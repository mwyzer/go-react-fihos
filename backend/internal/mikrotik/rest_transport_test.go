package mikrotik

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeRouterOS is an in-memory RouterOS REST stub. It keeps the hotpot profile
// list and active sessions so tests can exercise the full call flow.
type fakeRouterOS struct {
	mu        sync.Mutex
	profiles  []map[string]string
	active    []map[string]string
	authUser  string
	authPass  string
	requireOK bool
	reqLog    []string
}

func newFake() *fakeRouterOS {
	return &fakeRouterOS{
		profiles:  []map[string]string{},
		active:    []map[string]string{},
		requireOK: true,
	}
}

func (f *fakeRouterOS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	if f.authUser != "" && (!ok || user != f.authUser || pass != f.authPass) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"Invalid credentials"}`))
		return
	}
	f.mu.Lock()
	f.reqLog = append(f.reqLog, r.Method+" "+r.URL.Path)
	f.mu.Unlock()

	if !f.requireOK && r.URL.Path == "/rest/system/resource" {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"detail":"device busy"}`))
		return
	}

	switch {
	case r.URL.Path == "/rest/system/resource":
		_, _ = w.Write([]byte(`[{"uptime":"1w2d3h4m5s","cpu-load":"2","total-memory":"134217728"}]`))

	case r.URL.Path == "/rest/ip/hotspot/user/profile" && r.Method == http.MethodGet:
		f.mu.Lock()
		_ = json.NewEncoder(w).Encode(f.profiles)
		f.mu.Unlock()

	case r.URL.Path == "/rest/ip/hotspot/user/profile" && r.Method == http.MethodPost:
		var p map[string]string
		_ = json.NewDecoder(r.Body).Decode(&p)
		f.mu.Lock()
		p[".id"] = "*P" + itoa(len(f.profiles)+1)
		f.profiles = append(f.profiles, p)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(p)

	case strings.HasPrefix(r.URL.Path, "/rest/ip/hotspot/user/profile/") && r.Method == http.MethodPatch:
		id := strings.TrimPrefix(r.URL.Path, "/rest/ip/hotspot/user/profile/")
		var patch map[string]string
		_ = json.NewDecoder(r.Body).Decode(&patch)
		f.mu.Lock()
		for i := range f.profiles {
			if f.profiles[i][".id"] == id {
				for k, v := range patch {
					f.profiles[i][k] = v
				}
			}
		}
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{}`))

	case r.URL.Path == "/rest/ip/hotspot/active/print":
		f.mu.Lock()
		_ = json.NewEncoder(w).Encode(f.active)
		f.mu.Unlock()

	case strings.HasPrefix(r.URL.Path, "/rest/ip/hotspot/active/") && r.Method == http.MethodDelete:
		id := strings.TrimPrefix(r.URL.Path, "/rest/ip/hotspot/active/")
		f.mu.Lock()
		for i := range f.active {
			if f.active[i][".id"] == id {
				f.active = append(f.active[:i], f.active[i+1:]...)
				break
			}
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))

	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"no such command"}`))
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func newRESTClient(t *testing.T, fake *fakeRouterOS) (*restTransport, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	tr := newRestTransport(Credentials{Username: "admin", Password: "sekret"}, srv.Client())
	return tr, srv
}

func testRouter(srv *httptest.Server) Router {
	u, err := url.Parse(srv.URL)
	if err != nil {
		panic(err)
	}
	port, _ := strconv.Atoi(u.Port())
	if u.Port() == "" {
		port = 80
	}
	return Router{ID: 7, Name: "r7", IPAddress: u.Hostname(), APIPort: port}
}

func TestRestProbeOnline(t *testing.T) {
	fake := newFake()
	tr, srv := newRESTClient(t, fake)
	defer srv.Close()

	ok, ts, err := tr.Probe(testRouter(srv))
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if !ok {
		t.Fatal("expected online")
	}
	if ts.IsZero() {
		t.Fatal("expected last-seen timestamp")
	}
}

func TestRestProbeUnauthorized(t *testing.T) {
	fake := newFake()
	fake.authUser = "admin"
	fake.authPass = "wrong"
	tr, srv := newRESTClient(t, fake)
	defer srv.Close()

	if _, _, err := tr.Probe(testRouter(srv)); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestRestProbeOfflineDevice(t *testing.T) {
	fake := newFake()
	fake.requireOK = false
	closed := httptest.NewServer(fake)
	closed.Close()
	tr := newRestTransport(Credentials{}, nil)

	ok, _, err := tr.Probe(Router{ID: 1, IPAddress: "127.0.0.1", APIPort: 1})
	if err != ErrRouterOffline {
		t.Fatalf("expected ErrRouterOffline, got %v", err)
	}
	if ok {
		t.Fatal("expected offline")
	}
}

func TestRestApplyConfigCreatesProfile(t *testing.T) {
	fake := newFake()
	tr, srv := newRESTClient(t, fake)
	defer srv.Close()

	err := tr.ApplyConfig(testRouter(srv), HotspotConfig{Name: "wifi-demo", RxRate: 2_000_000, TxRate: 1_000_000, UptimeLimit: 7200})
	if err != nil {
		t.Fatalf("ApplyConfig error: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(fake.profiles))
	}
	p := fake.profiles[0]
	if p["name"] != "wifi-demo" {
		t.Fatalf("profile name = %q", p["name"])
	}
	if p["rate-limit"] != "16M/8000k" {
		t.Fatalf("rate-limit = %q, want 16M/8000k", p["rate-limit"])
	}
	if p["session-timeout"] != "02:00:00" {
		t.Fatalf("session-timeout = %q", p["session-timeout"])
	}
}

func TestRestApplyConfigUpdatesExisting(t *testing.T) {
	fake := newFake()
	fake.profiles = []map[string]string{{".id": "*1", "name": "wifi-demo", "rate-limit": "1M/1M"}}
	tr, srv := newRESTClient(t, fake)
	defer srv.Close()

	if err := tr.ApplyConfig(testRouter(srv), HotspotConfig{Name: "wifi-demo", RxRate: 512_000, TxRate: 512_000, UptimeLimit: 60}); err != nil {
		t.Fatalf("ApplyConfig error: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.profiles) != 1 {
		t.Fatalf("expected update not create, profiles = %d", len(fake.profiles))
	}
	if got := fake.profiles[0]["rate-limit"]; got != "4096k/4096k" {
		t.Fatalf("rate-limit = %q, want 4096k/4096k", got)
	}
	found := false
	for _, l := range fake.reqLog {
		if strings.HasPrefix(l, http.MethodPatch+" ") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected PATCH, got calls %v", fake.reqLog)
	}
}

func TestRestApplyRateMultiplier(t *testing.T) {
	fake := newFake()
	tr, srv := newRESTClient(t, fake)
	defer srv.Close()

	r := testRouter(srv)
	if err := tr.ApplyConfig(r, HotspotConfig{Name: "wifi-demo", RxRate: 1_000_000, TxRate: 1_000_000, UptimeLimit: 0}); err != nil {
		t.Fatalf("ApplyConfig error: %v", err)
	}
	if err := tr.ApplyRateMultiplier(r, "wifi-demo", 2); err != nil {
		t.Fatalf("ApplyRateMultiplier error: %v", err)
	}
	fake.mu.Lock()
	p := fake.profiles[0]["rate-limit"]
	fake.mu.Unlock()
	if p != "16M/16M" {
		t.Fatalf("rate-limit after x2 = %q, want 16M/16M", p)
	}
	if err := tr.ApplyRateMultiplier(r, "missing", 2); err != ErrNoHotspot {
		t.Fatalf("expected ErrNoHotspot, got %v", err)
	}
}

func TestRestListSessions(t *testing.T) {
	fake := newFake()
	fake.active = []map[string]string{
		{".id": "*1", "user": "7-abcd2345", "mac-address": "00:11:22:33:44:55", "address": "10.5.5.9", "bytes-in": "1,048,576 B", "bytes-out": "32 KiB"},
		{".id": "*2", "user": "7-efgh6789", "mac-address": "AA:BB:CC:DD:EE:FF", "address": "10.5.5.10", "bytes-in": "0", "bytes-out": "0"},
	}
	tr, srv := newRESTClient(t, fake)
	defer srv.Close()

	sess, err := tr.ListSessions(testRouter(srv), 30)
	if err != nil {
		t.Fatalf("ListSessions error: %v", err)
	}
	if len(sess) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sess))
	}
	if sess[0].Username != "7-abcd2345" || sess[0].BytesRX != 1048576 || sess[0].BytesTX != 32768 {
		t.Fatalf("session parse mismatch: %+v", sess[0])
	}
}

func TestRestDisconnectRemovesById(t *testing.T) {
	fake := newFake()
	fake.active = []map[string]string{{".id": "*9", "user": "7-abcd2345", "address": "10.5.5.9"}}
	tr, srv := newRESTClient(t, fake)
	defer srv.Close()

	if err := tr.Disconnect(testRouter(srv), "7-abcd2345"); err != nil {
		t.Fatalf("Disconnect error: %v", err)
	}
	fake.mu.Lock()
	if len(fake.active) != 0 {
		fake.mu.Unlock()
		t.Fatalf("session not removed: %+v", fake.active)
	}
	fake.mu.Unlock()
	deleted := false
	for _, l := range fake.reqLog {
		if strings.HasPrefix(l, http.MethodDelete+" ") && strings.HasSuffix(l, "*9") {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("expected DELETE by .id, calls=%v", fake.reqLog)
	}
	// Unknown user is a no-op (idempotent).
	if err := tr.Disconnect(testRouter(srv), "ghost"); err != nil {
		t.Fatalf("Disconnect unknown user error: %v (reqLog=%v)", err, fake.reqLog)
	}
}

func TestRestSendBasicAuth(t *testing.T) {
	fake := newFake()
	fake.authUser = "admin"
	fake.authPass = "sekret"
	tr, srv := newRESTClient(t, fake)
	defer srv.Close()

	if _, _, err := tr.Probe(testRouter(srv)); err != nil {
		t.Fatalf("Probe with correct auth error: %v", err)
	}
}

func wantBytes(f float64) int64 { return int64(f) }

func TestParseRouterBytes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"1,048,576 B", 1048576},
		{"32 KiB", 32768},
		{"3.4 MiB", wantBytes(3.4 * 1024 * 1024)},
		{"1.2 GiB", wantBytes(1.2 * 1024 * 1024 * 1024)},
		{"512 k", 512000},
		{"2 MB", 2000000},
		{"-", 0},
		{"", 0},
	} {
		if got := parseRouterBytes(tc.in); got != tc.want {
			t.Fatalf("parseRouterBytes(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestFmtRateLimitStrings(t *testing.T) {
	if got := fmtBandwidth(2_000_000); got != "16M" {
		t.Fatalf("fmtBandwidth = %q, want 16M", got)
	}
	if got := fmtBandwidth(512_000); got != "4096k" {
		t.Fatalf("fmtBandwidth = %q, want 4096k", got)
	}
	if got := fmtBandwidth(0); got != "unlimited" {
		t.Fatalf("fmtBandwidth(0) = %q", got)
	}
	if got := fmtDurationHMS(7325); got != "02:02:05" {
		t.Fatalf("fmtDurationHMS = %q", got)
	}
	if got := fmtDurationHMS(0); got != "00:00:00" {
		t.Fatalf("fmtDurationHMS(0) = %q", got)
	}
}

func TestNewModeRouting(t *testing.T) {
	sim := NewSimulator()
	if c := New(ModeSimulate, sim, Credentials{}, nil); c == nil || c.tr == nil {
		t.Fatal("simulate mode should produce a client")
	}
}
package mikrotik

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// restTransport talks to real RouterOS 6.47+/7.x devices over the JSON REST
// API exposed at /rest/... (basic auth). Counter/limit values follow RouterOS
// human-string conventions so the mapping is readable on the device.
type restTransport struct {
	creds  Credentials
	httpc  *http.Client
	mu     sync.Mutex
	baseRx map[string]int64
	baseTx map[string]int64
}

func newRestTransport(creds Credentials, httpc *http.Client) *restTransport {
	if httpc == nil {
		httpc = &http.Client{Timeout: 5 * time.Second}
	}
	return &restTransport{
		creds:  creds,
		httpc:  httpc,
		baseRx: map[string]int64{},
		baseTx: map[string]int64{},
	}
}

// baseURL builds the RouterOS REST root for a router. The stored APIPort is
// the legacy RouterOS API (8728/8729) port; the REST endpoint runs on the web
// interface port. When APIPort is the default (0 or 8728style), web is 80/443.
func (t *restTransport) baseURL(r Router) string {
	port := r.APIPort
	if port <= 0 || port == 8728 || port == 8729 {
		port = 80
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(r.IPAddress, strconv.Itoa(port)))
}

type restSession struct {
	DotID     string `json:".id"`
	User      string `json:"user"`
	MAC       string `json:"mac-address"`
	Address   string `json:"address"`
	BytesIn   string `json:"bytes-in"`
	BytesOut  string `json:"bytes-out"`
}

type restProfile struct {
	DotID   string `json:".id"`
	Name    string `json:"name"`
	Limits  string `json:"rate-limit"`
	Timeout string `json:"session-timeout"`
}

func (t *restTransport) do(ctx context.Context, r Router, method, path string, body any) (int, []byte, error) {
	target := t.baseURL(r) + path
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rd)
	if err != nil {
		return 0, nil, err
	}
	creds := t.creds.forRouter(r)
	if creds.Username != "" {
		req.SetBasicAuth(creds.Username, creds.Password)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := t.httpc.Do(req)
	if err != nil {
		return 0, nil, ErrRouterOffline
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return resp.StatusCode, raw, ErrUnauthorized
	}
	return resp.StatusCode, raw, nil
}

func (t *restTransport) get(ctx context.Context, r Router, path string) (int, []byte, error) {
	return t.do(ctx, r, http.MethodGet, path, nil)
}

// Register/Unregister are no-ops: devices are addressed directly by IP.
func (t *restTransport) Register(_ Router)                      {}
func (t *restTransport) Unregister(_ int64)                     {}

// Probe reports reachability via the resource endpoint.
func (t *restTransport) Probe(r Router) (bool, time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code, _, err := t.get(ctx, r, "/rest/system/resource")
	if err != nil {
		return false, time.Time{}, err
	}
	return code == http.StatusOK, time.Now().UTC(), nil
}

func (t *restTransport) ApplyConfig(r Router, cfg HotspotConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	code, raw, err := t.get(ctx, r, "/rest/ip/hotspot/user/profile")
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return ErrRouterOffline
	}
	var profiles []restProfile
	if err := json.Unmarshal(raw, &profiles); err != nil {
		return fmt.Errorf("decode profiles: %w", err)
	}

	profile := restProfile{}
	found := false
	for _, p := range profiles {
		if p.Name == cfg.Name {
			profile = p
			found = true
			break
		}
	}

	payload := map[string]string{
		"rate-limit":      fmtRateLimit(cfg.RxRate, cfg.TxRate),
		"session-timeout": fmtDurationHMS(cfg.UptimeLimit),
	}
	if found {
		code, raw, err = t.do(ctx, r, http.MethodPatch, "/rest/ip/hotspot/user/profile/"+url.PathEscape(profile.DotID), payload)
	} else {
		payload["name"] = cfg.Name
		code, raw, err = t.do(ctx, r, http.MethodPost, "/rest/ip/hotspot/user/profile", payload)
	}
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusCreated {
		return fmt.Errorf("apply config: device returned %d: %s", code, strings.TrimSpace(string(raw)))
	}

	key := profileKey(r.ID, cfg.Name)
	t.mu.Lock()
	t.baseRx[key] = cfg.RxRate
	t.baseTx[key] = cfg.TxRate
	t.mu.Unlock()
	return nil
}

func (t *restTransport) ApplyRateMultiplier(r Router, hotspotName string, multiplier float64) error {
	key := profileKey(r.ID, hotspotName)
	t.mu.Lock()
	baseRx, okRx := t.baseRx[key]
	baseTx, _ := t.baseTx[key]
	t.mu.Unlock()
	if !okRx {
		return ErrNoHotspot
	}
	if multiplier < 1 {
		multiplier = 1
	}
	rx := int64(float64(baseRx) * multiplier)
	tx := int64(float64(baseTx) * multiplier)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code, raw, err := t.get(ctx, r, "/rest/ip/hotspot/user/profile")
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return ErrRouterOffline
	}
	var profiles []restProfile
	if err := json.Unmarshal(raw, &profiles); err != nil {
		return fmt.Errorf("decode profiles: %w", err)
	}
	dotID := ""
	for _, p := range profiles {
		if p.Name == hotspotName {
			dotID = p.DotID
			break
		}
	}
	if dotID == "" {
		return ErrNoHotspot
	}
	code, raw, err = t.do(ctx, r, http.MethodPatch, "/rest/ip/hotspot/user/profile/"+url.PathEscape(dotID),
		map[string]string{"rate-limit": fmtRateLimit(rx, tx)})
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusCreated {
		return fmt.Errorf("apply multiplier: device returned %d: %s", code, strings.TrimSpace(string(raw)))
	}
	return nil
}

// SeedSessions/DropSessionsExcept are no-ops: the device owns live sessions.
func (t *restTransport) SeedSessions(_ Router, _ []SimSession)        {}
func (t *restTransport) DropSessionsExcept(_ Router, _ map[string]bool) {}

func (t *restTransport) ListSessions(r Router, _ float64) ([]SimSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code, raw, err := t.get(ctx, r, "/rest/ip/hotspot/active/print")
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, ErrRouterOffline
	}
	var list []restSession
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decode active sessions: %w", err)
	}
	out := make([]SimSession, 0, len(list))
	for _, s := range list {
		out = append(out, SimSession{
			Username: s.User,
			MAC:      s.MAC,
			IP:       s.Address,
			BytesRX:  parseRouterBytes(s.BytesIn),
			BytesTX:  parseRouterBytes(s.BytesOut),
		})
	}
	return out, nil
}

func (t *restTransport) Disconnect(r Router, username string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code, raw, err := t.get(ctx, r, "/rest/ip/hotspot/active/print")
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return ErrRouterOffline
	}
	var list []restSession
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("decode active sessions: %w", err)
	}
	dotID := ""
	for _, s := range list {
		if s.User == username {
			dotID = s.DotID
			break
		}
	}
	if dotID == "" {
		return nil
	}
	code, raw, err = t.do(ctx, r, http.MethodDelete, "/rest/ip/hotspot/active/"+url.PathEscape(dotID), nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusNoContent {
		return fmt.Errorf("disconnect: device returned %d: %s", code, strings.TrimSpace(string(raw)))
	}
	return nil
}

func profileKey(routerID int64, hotspot string) string {
	return strconv.FormatInt(routerID, 10) + "/" + hotspot
}

// fmtRateLimit renders rx/tx (bytes/sec) as a RouterOS rate-limit string,
// e.g. "1024k/512k" or "5M/5M".
func fmtRateLimit(rx, tx int64) string {
	return fmtBandwidth(rx) + "/" + fmtBandwidth(tx)
}

func fmtBandwidth(bps int64) string {
	if bps <= 0 {
		return "unlimited"
	}
	kbps := float64(bps*8) / 1000
	// Stay in k up to 10 Mbps for readability on the device ("4096k", "16M").
	if kbps >= 10000 {
		return trimDotZero(fmt.Sprintf("%.1f", kbps/1000)) + "M"
	}
	return fmt.Sprintf("%.0f", kbps) + "k"
}

func trimDotZero(s string) string {
	if strings.HasSuffix(s, ".0") {
		return s[:len(s)-2]
	}
	return s
}

// fmtDurationHMS renders seconds as RouterOS session-timeout ("HH:MM:SS").
func fmtDurationHMS(sec int) string {
	if sec <= 0 {
		return "00:00:00"
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// parseRouterBytes parses RouterOS human byte strings ("0", "1,024 B",
// "16 KiB", "3.4 MiB") into a byte count.
func parseRouterBytes(s string) int64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" || s == "-" {
		return 0
	}
	fields := strings.Fields(s)
	val, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	if len(fields) > 1 {
		switch strings.ToLower(fields[1]) {
		case "kib":
			val *= 1024
		case "k", "kb":
			val *= 1000
		case "mib":
			val *= 1024 * 1024
		case "m", "mb":
			val *= 1000 * 1000
		case "gib":
			val *= 1024 * 1024 * 1024
		case "g", "gb":
			val *= 1000 * 1000 * 1000
		}
	}
	return int64(val)
}

var (
	ErrUnauthorized = errors.New("routeros authentication failed")
)
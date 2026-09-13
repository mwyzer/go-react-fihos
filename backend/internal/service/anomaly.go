package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"time"

	"fihos/backend/internal/store"
)

// Alert types
const (
	TypeExcessTraffic   = "excess_traffic"
	TypeVoucherShared   = "voucher_shared"
	TypeTrafficSpike    = "traffic_spike"
	TypeConcurrencyGap  = "concurrency_gap"
	TypeOverLimit       = "over_limit_session"
	TypeMacHop          = "mac_hop"
)

// AnomalyEngine runs rule-based + z-score checks over session/traffic
// snapshots and persists alerts (deduped by fingerprint within 1h).
type AnomalyEngine struct {
	st *store.Store
}

func NewAnomalyEngine(st *store.Store) *AnomalyEngine {
	return &AnomalyEngine{st: st}
}

func fingerprint(tenantID int64, t, key string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", tenantID, t, key)))
	return fmt.Sprintf("%x", h)[:32]
}

// Run evaluates a single tenant and returns the alerts created.
func (e *AnomalyEngine) Run(ctx context.Context, tenantID int64) ([]store.AnomalyAlert, error) {
	now := time.Now().UTC()

	sessions, err := e.st.ActiveSessionsSnapshot(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	conc, err := e.st.HotspotConcurrency(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	hourly, err := e.st.HotspotHourlyUsage(ctx, tenantID, 7)
	if err != nil {
		return nil, err
	}
	macs, err := e.st.DistinctMACCounts(ctx, tenantID, 24)
	if err != nil {
		return nil, err
	}

	bucketByHour := indexHourly(hourly)

	var created []store.AnomalyAlert

	voucherCount := map[int64]int{}
	for _, s := range sessions {
		if s.VoucherID != nil {
			voucherCount[*s.VoucherID]++
		}
	}

	for _, s := range sessions {
		total := s.BytesRX + s.BytesTX
		elapsed := now.Sub(s.StartTime)

		if s.UptimeLimitMin > 0 && elapsed.Minutes() > float64(s.UptimeLimitMin)*1.05 {
			created = append(created, e.alert(tenantID, s, TypeOverLimit, "warning",
				elapsed.Minutes(), float64(s.UptimeLimitMin), fingerprint(tenantID, TypeOverLimit, fmt.Sprint(s.SessionID))))
		}

		baseRate := s.ProfileRXRate + s.ProfileTXRate
		if baseRate > 0 {
			allowed := float64(baseRate) * 125 * elapsed.Seconds() * 1.1
			if total > int64(allowed) {
				sev := "warning"
				if total > int64(allowed*2) {
					sev = "critical"
				}
				created = append(created, e.alert(tenantID, s, TypeExcessTraffic, sev,
					float64(total), allowed, fingerprint(tenantID, TypeExcessTraffic, fmt.Sprint(s.SessionID))))
			}
		}

		if s.VoucherID != nil && voucherCount[*s.VoucherID] > 1 {
			created = append(created, e.alert(tenantID, s, TypeVoucherShared, "critical",
				float64(voucherCount[*s.VoucherID]), 1, fingerprint(tenantID, TypeVoucherShared, fmt.Sprintf("v%d", *s.VoucherID))))
		}
	}

	for _, row := range macs {
		created = append(created, e.alert(tenantID, store.SessionSnapshot{}, TypeMacHop, "warning",
			float64(row.Macs), 3, fingerprint(tenantID, TypeMacHop, row.Username)))
	}

	for _, hs := range conc {
		buckets := bucketByHour[hs.HotspotID]

		if n := len(buckets); n >= 5 {
			var vals []float64
			for _, b := range buckets {
				vals = append(vals, float64(b.Bytes))
			}
			mean, std := meanStd(vals)
			if std > 0 {
				threshold := mean + 3*std
				curBucket := now.Truncate(time.Hour)
				var cur int64
				for _, b := range buckets {
					if b.Bucket.Equal(curBucket) {
						cur = b.Bytes
					}
				}
				if cur > int64(threshold) && cur > int64(mean*2) {
					created = append(created, e.alert(tenantID, withHotspotPre(store.SessionSnapshot{}, hs.HotspotID), TypeTrafficSpike, "warning",
						float64(cur), threshold, fingerprint(tenantID, TypeTrafficSpike, fmt.Sprintf("h%d:%s", hs.HotspotID, curBucket.Format("2006-01-02-15")))))
				}
			}
		}

		var avg int64
		if len(buckets) > 0 {
			var sum, n int64
			for _, b := range buckets {
				sum += int64(b.Sessions)
				n++
			}
			avg = sum / n
		}
		if avg > 0 && float64(hs.Active) > float64(avg)*3 {
			created = append(created, e.alert(tenantID, withHotspotPre(store.SessionSnapshot{}, hs.HotspotID), TypeConcurrencyGap, "warning",
				float64(hs.Active), float64(avg*3), fingerprint(tenantID, TypeConcurrencyGap, fmt.Sprintf("h%d", hs.HotspotID))))
		}
	}

	// Persist created alerts; collapse repeats via InsertAlert.
	var persisted []store.AnomalyAlert
	seen := map[string]bool{}
	for _, a := range created {
		if seen[a.Fingerprint] {
			continue
		}
		seen[a.Fingerprint] = true
		ins, err := e.st.InsertAlert(ctx, &a)
		if err != nil {
			return persisted, err
		}
		persisted = append(persisted, *ins)
	}

	e.upsertDigest(ctx, tenantID, persisted, now)

	// Resolve stale open alerts older than 24h automatically.
	return persisted, e.autoResolve(ctx, tenantID, now)
}

func (e *AnomalyEngine) alert(tenantID int64, s store.SessionSnapshot, t, severity string, value, baseline float64, fp string) store.AnomalyAlert {
	a := store.AnomalyAlert{
		TenantID:    tenantID,
		HotspotID:   idPtrIf(s.HotspotID),
		RouterID:    idPtrIf(s.RouterID),
		Type:        t,
		Severity:    severity,
		Status:      "open",
		Value:       value,
		Baseline:    baseline,
		Fingerprint: fp,
		RateWindowID: s.RateWindowID,
		CreatedAt:   time.Now().UTC(),
	}
	if s.SessionID > 0 {
		a.SessionID = &s.SessionID
	}
	return a
}

func idPtrIf(v int64) *int64 {
	if v == 0 {
		return nil
	}
	p := v
	return &p
}

func withHotspotPre(s store.SessionSnapshot, hid int64) store.SessionSnapshot {
	s.HotspotID = hid
	return s
}

func indexHourly(rows []store.HotspotHourly) map[int64][]store.HotspotHourly {
	m := map[int64][]store.HotspotHourly{}
	for _, r := range rows {
		m[r.HotspotID] = append(m[r.HotspotID], r)
	}
	return m
}

func meanStd(v []float64) (float64, float64) {
	if len(v) == 0 {
		return 0, 0
	}
	sort.Float64s(v)
	// Use median-IQR-ish robust center to survive hot-school-day outliers
	median := v[len(v)/2]
	var sum float64
	for _, x := range v {
		sum += (x - median) * (x - median)
	}
	std := math.Sqrt(sum / float64(len(v)))
	return median, std
}

func (e *AnomalyEngine) upsertDigest(ctx context.Context, tenantID int64, alerts []store.AnomalyAlert, now time.Time) {
	type key struct {
		hotspot  int64
		t        string
		severity string
	}
	agg := map[key]map[int]int{} // key -> status counts
	vals := map[key]float64{}
	first := map[key]time.Time{}
	last := map[key]time.Time{}
	for _, a := range alerts {
		k := key{}
		if a.HotspotID != nil {
			k.hotspot = *a.HotspotID
		}
		k.t, k.severity = a.Type, a.Severity
		if agg[k] == nil {
			agg[k] = map[int]int{}
			first[k] = a.CreatedAt
		}
		agg[k][statusCode(a.Status)]++
		vals[k] += a.Value
		if a.CreatedAt.After(last[k]) {
			last[k] = a.CreatedAt
		}
	}
	for k, counts := range agg {
		open := counts[0]
		total := counts[0] + counts[1] + counts[2] + counts[3]
		resolved := counts[2]
		_ = e.st.UpsertAnomalySummary(ctx, tenantID, k.hotspot, k.t, k.severity,
			now.Truncate(time.Hour), open, total, resolved, vals[k], first[k], last[k])
	}
}

func statusCode(s string) int {
	switch s {
	case "open":
		return 0
	case "acknowledged":
		return 1
	case "resolved":
		return 2
	case "dismissed":
		return 3
	}
	return 0
}

func (e *AnomalyEngine) autoResolve(ctx context.Context, tenantID int64, now time.Time) error {
	_, err := e.st.Exec(ctx, `
		UPDATE anomaly_alerts
		SET status='resolved', resolved_at=COALESCE(resolved_at, now()), updated_at=now()
		WHERE tenant_id=$1 AND status='open' AND created_at < now() - interval '24 hours'`, tenantID)
	return err
}
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"fihos/backend/internal/auth"
	"fihos/backend/internal/config"
	"fihos/backend/internal/database"
	"fihos/backend/internal/store"
)

var (
	firstNames = []string{"Budi", "Siti", "Agus", "Rina", "Dedi", "Lina", "Hendra", "Wati",
		"Rudi", "Dewi", "Eko", "Sari", "Bayu", "Nina", "Fajar", "Indah",
		"Joko", "Sri", "Andi", "Putri", "Rizal", "Maya", "Tono", "Yuni",
		"Wawan", "Tika", "Ferry", "Desi", "Iwan", "Lestari", "Doni", "Ratna"}
	lastNames = []string{"Santoso", "Wijaya", "Pratama", "Saputra", "Hidayat", "Nugroho",
		"Rahayu", "Kurniawan", "Setiawan", "Wulandari", "Gunawan", "Susanti",
		"Ramadhan", "Anggraini", "Permana", "Utami"}
)

type seededTenant struct {
	Name       string
	Slug       string
	Portal     string
	MonthlyFee float64
}

var tenants = []seededTenant{
	{Name: "Demo WiFi", Slug: "demo", Portal: "Free Wi-Fi", MonthlyFee: 150000},
	{Name: "Warnet Alfa", Slug: "warnet-alfa", Portal: "WiFi Alfa", MonthlyFee: 150000},
	{Name: "Warnet Beta", Slug: "warnet-beta", Portal: "WiFi Beta", MonthlyFee: 175000},
	{Name: "WiFi Gamma", Slug: "wifi-gamma", Portal: "Gamma Net", MonthlyFee: 200000},
	{Name: "Netz Delta", Slug: "netz-delta", Portal: "Delta Connected", MonthlyFee: 150000},
	{Name: "Kopi & WiFi Epsilon", Slug: "epsilon-net", Portal: "Epsilon Corner", MonthlyFee: 250000},
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer db.Close()
	st := store.New(db)

	for _, t := range tenants {
		if err := seedTenant(ctx, st, t); err != nil {
			log.Printf("seed %s: %v", t.Slug, err)
		}
	}
	fmt.Println("seeding done")
}

func seedTenant(ctx context.Context, st *store.Store, t seededTenant) error {
	// Load or create the tenant.
	tenant, err := st.TenantBySlug(ctx, t.Slug)
	if err == store.ErrNotFound {
		tenant, err = st.CreateTenant(ctx, t.Name, t.Slug, map[string]any{"color": "#3b82f6"})
		if err != nil {
			return err
		}
		fmt.Printf("[ok] tenant %s (id=%d)\n", t.Slug, tenant.ID)
	} else if err != nil {
		return err
	} else {
		fmt.Printf("[ok] tenant %s existing (id=%d)\n", t.Slug, tenant.ID)
	}

	// Ensure a hotspot exists (profile + router if needed).
	hotspots, err := st.ListHotspots(ctx, tenant.ID, 0, "")
	if err != nil {
		return err
	}
	var hot *store.Hotspot
	if len(hotspots) == 0 {
		profile, err := st.CreateProfile(ctx, tenant.ID, &store.Profile{
			Name: "Paket 10M", RxRate: 10_000_000, TxRate: 10_000_000, SessionUptimeLimit: 3600, KeepaliveTimeout: 90,
		})
		if err != nil {
			return err
		}
		router, err := st.CreateRouter(ctx, tenant.ID, &store.Router{
			Name: "RB1", IPAddress: "10.10.0.1", APIPort: 8728, Username: "admin",
		})
		if err != nil {
			return err
		}
		hot, err = st.CreateHotspot(ctx, tenant.ID, &store.Hotspot{
			RouterID: router.ID, ProfileID: profile.ID, Name: "Hotspot " + t.Name,
		})
		if err != nil {
			return err
		}
	} else {
		hot = &hotspots[0]
	}

	// Ensure owner login exists.
	email := "owner@" + t.Slug + ".dev"
	owner, err := st.UserByEmail(ctx, email)
	if err != nil {
		if err != store.ErrNotFound {
			return err
		}
		pass := randomPass()
		hash, err := auth.HashPassword(pass)
		if err != nil {
			return err
		}
		tid := tenant.ID
		owner, err = st.CreateUser(ctx, &store.User{
			TenantID: &tid, Email: email, FullName: "Owner " + t.Name, Role: "owner", IsActive: true,
		}, hash)
		if err != nil {
			return err
		}
		fmt.Printf("  login: owner account id=%d  %s / %s\n", owner.ID, email, pass)
	} else {
		fmt.Printf("  login owner existing: %s\n", email)
	}

	// Tenant settings with monthly fee.
	if _, err := st.Exec(ctx, `
		INSERT INTO settings (tenant_id, portal_title, portal_message, payment_config)
		VALUES ($1, $2, $3, jsonb_build_object('monthly_fee', $4::bigint))
		ON CONFLICT (tenant_id) DO UPDATE SET
			portal_title = COALESCE(NULLIF(EXCLUDED.portal_title,''), settings.portal_title),
			portal_message = COALESCE(NULLIF(EXCLUDED.portal_message,''), settings.portal_message),
			payment_config = EXCLUDED.payment_config,
			updated_at = now()`,
		tenant.ID, t.Portal, "Silakan gunakan WiFi "+t.Name, currencyFor(t.MonthlyFee)); err != nil {
		return err
	}

	// 50 customers (30 active, 10 token, 5 expired, 5 suspended) if not seeded yet.
	total, err := st.CountCustomersByStatus(ctx, tenant.ID)
	if err != nil {
		return err
	}
	seedCount := int64(0)
	for _, n := range total {
		seedCount += n
	}

	if seedCount == 0 {
		statuses := make([]string, 0, 50)
		for i := 0; i < 30; i++ {
			statuses = append(statuses, "active")
		}
		for i := 0; i < 10; i++ {
			statuses = append(statuses, "token")
		}
		for i := 0; i < 5; i++ {
			statuses = append(statuses, "expired")
		}
		for i := 0; i < 5; i++ {
			statuses = append(statuses, "suspended")
		}
		shuffle(statuses)

		for i, status := range statuses {
			_, err := st.CreateCustomer(ctx, tenant.ID, &store.Customer{
				HotspotID: &hot.ID,
				Name:      fmt.Sprintf("%s %s", randName(firstNames), randName(lastNames)),
				Phone:     fmt.Sprintf("08%09d", i+100000),
				Address:   fmt.Sprintf("Jl. Contoh No. %d, Kota", i+1),
				Status:    status,
			})
			if err != nil {
				return err
			}
		}
		fmt.Printf("  customers seeded: 50\n")
	}

	// Billing windows for token customers: last 2 months + current.
	tokenCustomers, _, err := st.ListCustomers(ctx, tenant.ID, "token", 1, 100)
	if err != nil {
		return err
	}
	fee := t.MonthlyFee
	windowCount := 0
	for _, c := range tokenCustomers {
		for off := 0; off >= -2; off-- {
			start := time.Now().AddDate(0, off, 0)
			start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
			end := start.AddDate(0, 1, -1)
			res, err := st.Exec(ctx, `
				INSERT INTO billing_windows (tenant_id, customer_id, period_start, period_end, amount, status, issued_at)
				VALUES ($1, $2, $3, $4, $5, 'issued', now())
				ON CONFLICT (customer_id, period_start) DO NOTHING`,
				tenant.ID, c.ID, start, end, fee)
			if err != nil {
				return err
			}
			windowCount += int(res.RowsAffected())
		}
	}
	if windowCount > 0 {
		// Mark the oldest (two months ago) window as paid for realism.
		cids := make([]int64, 0, len(tokenCustomers))
		for _, c := range tokenCustomers {
			cids = append(cids, c.ID)
		}
		_, _ = st.Exec(ctx, `
			UPDATE billing_windows SET status='paid', paid_at=now() - interval '1 month'
			WHERE tenant_id = $1 AND customer_id IN (SELECT unnest($2::bigint[]))
			  AND period_start = date_trunc('month', now() - interval '2 month')::date`,
			tenant.ID, cids)
	}
	fmt.Printf("[ok] %s (windows=%d, token_customers=%d)\n", t.Name, windowCount, len(tokenCustomers))

	if err := st.SetHotspotConfigured(ctx, hot.ID, hot.Name, 1); err != nil {
		log.Printf("  hotspot configure: %v", err)
	}
	return nil
}

func currencyFor(f float64) int64 { return int64(f) }

func randomPass() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("rand: %v", err)
	}
	return "owner-" + hex.EncodeToString(b)
}

func randName(list []string) string {
	b := make([]byte, 1)
	_, _ = rand.Read(b)
	return list[int(b[0])%len(list)]
}

func shuffle(s []string) {
	b := make([]byte, len(s))
	_, _ = rand.Read(b)
	for i := len(s) - 1; i > 0; i-- {
		j := int(b[i]) % (i + 1)
		s[i], s[j] = s[j], s[i]
	}
}

func init() {
	time.Local = time.UTC
}

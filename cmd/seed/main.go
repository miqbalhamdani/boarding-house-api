// Command seed populates the database with realistic dummy data for local
// development and testing: 5 owners, 10-20 tenants per owner, and 5-20 monthly
// billing cycles (bill -> optional gateway transaction -> payment) per tenant.
//
// It is idempotent-ish: it truncates the seeded tables first, so every run
// starts from a clean slate. It refuses to run when ENV=production.
//
// Usage:
//
//	make seed   # or: go run ./cmd/seed
package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/iqbal-hamdani/go-backend/internal/config"
	"github.com/iqbal-hamdani/go-backend/internal/database"
	"github.com/iqbal-hamdani/go-backend/pkg/logger"
)

const (
	ownerCount    = 5
	minTenants    = 10
	maxTenants    = 20
	minBills      = 5
	maxBills      = 20
	seedPassword  = "password123"
	gatewayVendor = "sandbox"
)

var (
	firstNames = []string{
		"Adi", "Budi", "Citra", "Dewi", "Eka", "Fajar", "Gita", "Hadi",
		"Indah", "Joko", "Kartika", "Lestari", "Maya", "Nanda", "Oki",
		"Putri", "Rian", "Sari", "Tono", "Umi", "Vina", "Wawan", "Yuni", "Zaki",
	}
	lastNames = []string{
		"Santoso", "Wijaya", "Pratama", "Nugroho", "Halim", "Kusuma",
		"Saputra", "Utami", "Hartono", "Wibowo", "Permana", "Anggraini",
		"Setiawan", "Maharani", "Firmansyah", "Cahyani",
	}
	businessSuffix = []string{"Kost", "Residence", "Boarding House", "Living", "House"}
	streets        = []string{"Melati", "Kenanga", "Anggrek", "Mawar", "Cempaka", "Flamboyan", "Dahlia"}
	paymentMethods = []string{"cash", "bank_transfer", "e_wallet", "virtual_account", "qris"}
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Env)
	slog.SetDefault(log)

	if strings.EqualFold(cfg.Env, "production") {
		return fmt.Errorf("refusing to seed: ENV=production")
	}

	ctx := context.Background()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.Info("connected to database")

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(seedPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	if err := clean(ctx, tx); err != nil {
		return err
	}

	stats := &stats{}
	now := time.Now()

	for o := 1; o <= ownerCount; o++ {
		if err := seedOwner(ctx, tx, string(passwordHash), o, now, stats); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	log.Info("seed complete",
		"owners", stats.owners,
		"tenants", stats.tenants,
		"rooms", stats.rooms,
		"bills", stats.bills,
		"payments", stats.payments,
		"gateway_transactions", stats.gatewayTx,
		"password", seedPassword,
	)
	return nil
}

type stats struct {
	owners    int
	tenants   int
	rooms     int
	bills     int
	payments  int
	gatewayTx int
}

// clean removes all previously seeded rows so re-runs start fresh. Child-to-parent
// order with CASCADE keeps FKs happy; RESTART IDENTITY is harmless (all PKs are UUIDs).
func clean(ctx context.Context, tx pgx.Tx) error {
	const q = `TRUNCATE payments, payment_gateway_transactions, bills,
		room_assignments, rooms, tenants, owner_users, owners
		RESTART IDENTITY CASCADE`
	if _, err := tx.Exec(ctx, q); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}
	return nil
}

func seedOwner(ctx context.Context, tx pgx.Tx, passwordHash string, seq int, now time.Time, st *stats) error {
	business := fmt.Sprintf("%s %s", pick(streets), pick(businessSuffix))
	fullName := randName()
	email := fmt.Sprintf("owner%d@example.com", seq)

	var ownerID string
	err := tx.QueryRow(ctx,
		`INSERT INTO owners (business_name, full_name, email, phone_number)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		business, fullName, email, randPhone(),
	).Scan(&ownerID)
	if err != nil {
		return fmt.Errorf("insert owner %d: %w", seq, err)
	}
	st.owners++

	// Login user for the owner.
	if _, err := tx.Exec(ctx,
		`INSERT INTO owner_users (owner_id, full_name, email, password_hash, status)
		 VALUES ($1, $2, $3, $4, 'active')`,
		ownerID, fullName, email, passwordHash,
	); err != nil {
		return fmt.Errorf("insert owner_user %d: %w", seq, err)
	}

	tenantCount := randRange(minTenants, maxTenants)
	billCounter := 0 // per-owner running counter for unique bill numbers

	for t := 1; t <= tenantCount; t++ {
		if err := seedTenant(ctx, tx, ownerID, seq, t, passwordHash, now, &billCounter, st); err != nil {
			return err
		}
	}
	return nil
}

func seedTenant(ctx context.Context, tx pgx.Tx, ownerID string, ownerSeq, tenantSeq int, passwordHash string, now time.Time, billCounter *int, st *stats) error {
	// One room per tenant so each can hold exactly one active assignment.
	roomNumber := fmt.Sprintf("R%02d-%03d", ownerSeq, tenantSeq)
	rent := randRange(16, 60) * 50_000 // 800,000 .. 3,000,000 IDR

	var roomID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO rooms (owner_id, room_number, room_name, monthly_rent, status)
		 VALUES ($1, $2, $3, $4, 'occupied') RETURNING id`,
		ownerID, roomNumber, "Kamar "+roomNumber, rent,
	).Scan(&roomID); err != nil {
		return fmt.Errorf("insert room %s: %w", roomNumber, err)
	}
	st.rooms++

	tenantName := randName()
	tenantEmail := fmt.Sprintf("tenant%d-%d@example.com", ownerSeq, tenantSeq)

	var tenantID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO tenants
		 (owner_id, full_name, phone_number, email, password_hash, identity_number,
		  emergency_contact_name, emergency_contact_phone, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active') RETURNING id`,
		ownerID, tenantName, randPhone(), tenantEmail, passwordHash,
		randIdentity(), randName(), randPhone(),
	).Scan(&tenantID); err != nil {
		return fmt.Errorf("insert tenant %s: %w", tenantEmail, err)
	}
	st.tenants++

	billCount := randRange(minBills, maxBills)
	dueDay := randRange(1, 10)
	// Assignment started billCount months ago so the oldest bill lines up with it.
	startMonth := addMonths(monthStart(now), -(billCount - 1))

	var assignmentID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO room_assignments
		 (owner_id, tenant_id, room_id, start_date, monthly_rent, payment_due_day, status)
		 VALUES ($1, $2, $3, $4, $5, $6, 'active') RETURNING id`,
		ownerID, tenantID, roomID, startMonth, rent, dueDay,
	).Scan(&assignmentID); err != nil {
		return fmt.Errorf("insert assignment for %s: %w", tenantEmail, err)
	}

	// Generate billCount consecutive months, oldest first.
	for i := 0; i < billCount; i++ {
		month := addMonths(startMonth, i)
		*billCounter++
		if err := seedBill(ctx, tx, billContext{
			ownerID:      ownerID,
			tenantID:     tenantID,
			roomID:       roomID,
			assignmentID: assignmentID,
			ownerSeq:     ownerSeq,
			billSeq:      *billCounter,
			month:        month,
			dueDay:       dueDay,
			amount:       rent,
			now:          now,
		}, st); err != nil {
			return err
		}
	}
	return nil
}

type billContext struct {
	ownerID      string
	tenantID     string
	roomID       string
	assignmentID string
	ownerSeq     int
	billSeq      int
	month        time.Time
	dueDay       int
	amount       int
	now          time.Time
}

func seedBill(ctx context.Context, tx pgx.Tx, b billContext, st *stats) error {
	periodStart := b.month
	periodEnd := addMonths(b.month, 1).AddDate(0, 0, -1)
	dueDate := b.month.AddDate(0, 0, b.dueDay-1)
	billMonth := b.month.Format("2006-01")
	billNumber := fmt.Sprintf("INV-%02d-%06d", b.ownerSeq, b.billSeq)

	// Status mix: current month is unpaid; past months are mostly paid, with a
	// minority left overdue.
	status := "paid"
	if !dueDate.Before(b.now) {
		status = "unpaid"
	} else if rand.IntN(100) < 15 {
		status = "overdue"
	}

	var paidAt *time.Time
	if status == "paid" {
		// Paid a few days around the due date.
		pt := dueDate.AddDate(0, 0, rand.IntN(6)-2)
		paidAt = &pt
	}

	var billID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO bills
		 (owner_id, tenant_id, room_id, room_assignment_id, bill_number, bill_type,
		  billing_month, billing_period_start, billing_period_end, amount, due_date,
		  status, paid_at)
		 VALUES ($1,$2,$3,$4,$5,'rent',$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
		b.ownerID, b.tenantID, b.roomID, b.assignmentID, billNumber,
		billMonth, periodStart, periodEnd, b.amount, dueDate, status, paidAt,
	).Scan(&billID); err != nil {
		return fmt.Errorf("insert bill %s: %w", billNumber, err)
	}
	st.bills++

	if status != "paid" {
		return nil
	}

	// A paid bill gets a payment. Half go through the gateway.
	source := "manual"
	var gatewayTxID *string
	if rand.IntN(2) == 0 {
		source = "gateway"
		id, err := seedGatewayTx(ctx, tx, b, billID, billNumber, *paidAt)
		if err != nil {
			return err
		}
		gatewayTxID = &id
		st.gatewayTx++
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO payments
		 (owner_id, bill_id, tenant_id, room_id, amount, payment_date,
		  payment_method, payment_source, gateway_transaction_id, reference_number)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		b.ownerID, billID, b.tenantID, b.roomID, b.amount, *paidAt,
		pick(paymentMethods), source, gatewayTxID, "PAY-"+billNumber,
	); err != nil {
		return fmt.Errorf("insert payment for %s: %w", billNumber, err)
	}
	st.payments++
	return nil
}

func seedGatewayTx(ctx context.Context, tx pgx.Tx, b billContext, billID, billNumber string, paidAt time.Time) (string, error) {
	orderID := billNumber + "-PG"
	extTxID := fmt.Sprintf("SBX-%06d", b.billSeq)
	raw := fmt.Sprintf(`{"provider":%q,"order_id":%q,"status":"paid","amount":%d}`,
		gatewayVendor, orderID, b.amount)

	var id string
	err := tx.QueryRow(ctx,
		`INSERT INTO payment_gateway_transactions
		 (owner_id, bill_id, tenant_id, provider, external_transaction_id,
		  external_order_id, amount, currency, status, paid_at, raw_create_response)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,'IDR','paid',$8,$9) RETURNING id`,
		b.ownerID, billID, b.tenantID, gatewayVendor, extTxID, orderID,
		b.amount, paidAt, raw,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert gateway tx %s: %w", orderID, err)
	}
	return id, nil
}

// --- helpers ---

func randRange(min, max int) int { return min + rand.IntN(max-min+1) }

func pick[T any](s []T) T { return s[rand.IntN(len(s))] }

func randName() string { return pick(firstNames) + " " + pick(lastNames) }

func randPhone() string { return fmt.Sprintf("08%d", randRange(1_000_000_000, 1_999_999_999)) }

func randIdentity() string { return fmt.Sprintf("32%014d", randRange(100_000_000, 999_999_999)) }

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func addMonths(t time.Time, n int) time.Time { return t.AddDate(0, n, 0) }

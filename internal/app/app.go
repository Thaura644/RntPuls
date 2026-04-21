package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Thaura644/RntPuls/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
)

type App struct {
	cfg    config.Config
	db     *pgxpool.Pool
	logger *slog.Logger
}

type authUser struct {
	ID             string
	OrganizationID string
	Email          string
	Role           string
	FullName       string
	TenantID       string
}

type ctxKey string

const userKey ctxKey = "user"

func New(cfg config.Config, db *pgxpool.Pool, logger *slog.Logger) *App {
	return &App{cfg: cfg, db: db, logger: logger}
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /api/plans", a.plans)
	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.Handle("GET /api/me", a.staffAuth(http.HandlerFunc(a.me)))
	mux.Handle("GET /api/dashboard", a.staffAuth(http.HandlerFunc(a.dashboard)))
	mux.Handle("GET /api/settings", a.staffAuth(http.HandlerFunc(a.settings)))
	mux.Handle("PUT /api/settings", a.staffAuth(http.HandlerFunc(a.updateSettings)))
	mux.Handle("GET /api/properties", a.staffAuth(http.HandlerFunc(a.listProperties)))
	mux.Handle("POST /api/properties", a.staffAuth(http.HandlerFunc(a.createProperty)))
	mux.Handle("GET /api/tenants", a.staffAuth(http.HandlerFunc(a.listTenants)))
	mux.Handle("POST /api/tenants", a.staffAuth(http.HandlerFunc(a.createTenant)))
	mux.Handle("POST /api/tenants/{tenantID}/access-link", a.staffAuth(http.HandlerFunc(a.createTenantAccessLink)))
	mux.Handle("POST /api/imports/tenants", a.staffAuth(http.HandlerFunc(a.importTenants)))
	mux.Handle("GET /api/exports/tenants.csv", a.staffAuth(http.HandlerFunc(a.exportTenantsCSV)))
	mux.Handle("GET /api/reports/monthly.xlsx", a.staffAuth(http.HandlerFunc(a.monthlyExcelReport)))
	mux.Handle("POST /api/payments/mark-paid", a.staffAuth(http.HandlerFunc(a.markPaid)))
	mux.Handle("POST /api/payments/verify", a.staffAuth(http.HandlerFunc(a.verifyPayment)))
	mux.Handle("POST /api/communications/reminders/run", a.staffAuth(http.HandlerFunc(a.runReminders)))
	mux.Handle("GET /api/tenant/me", a.tenantAuth(http.HandlerFunc(a.tenantMe)))
	mux.Handle("POST /api/tenant/uploads", a.tenantAuth(http.HandlerFunc(a.tenantUpload)))
	mux.Handle("POST /api/tenant/payments/mark-paid", a.tenantAuth(http.HandlerFunc(a.tenantMarkPaid)))
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(a.cfg.UploadDir))))
	return a.cors(a.recover(mux))
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) plans(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []map[string]any{
		{"id": "free", "name": "Free", "price_cents": 0, "unit_limit": 2, "features": []string{"Tenant directory", "Manual payment tracking"}},
		{"id": "starter", "name": "Starter", "price_cents": 50000, "unit_limit": 2, "features": []string{"Automated invoices", "Basic rent tracking", "CSV exports"}},
		{"id": "pro", "name": "Pro", "price_cents": 120000, "unit_limit": 10, "features": []string{"WhatsApp/SMS reminders", "M-Pesa verification workflow", "Excel reports", "Imports"}},
		{"id": "agency", "name": "Agency", "price_cents": nil, "unit_limit": nil, "features": []string{"Unlimited units", "Multi-user access", "Branding", "Priority support"}},
	})
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OrganizationName string `json:"organization_name"`
		FullName         string `json:"full_name"`
		Email            string `json:"email"`
		Phone            string `json:"phone"`
		Password         string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.OrganizationName == "" || in.FullName == "" || in.Email == "" || len(in.Password) < 8 {
		writeError(w, http.StatusBadRequest, "organization, name, email and an 8+ character password are required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password hashing failed")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction failed")
		return
	}
	defer tx.Rollback(r.Context())
	var orgID, userID string
	if err := tx.QueryRow(r.Context(), `INSERT INTO organizations (name) VALUES ($1) RETURNING id`, in.OrganizationName).Scan(&orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "organization create failed")
		return
	}
	_, _ = tx.Exec(r.Context(), `INSERT INTO organization_settings (organization_id) VALUES ($1)`, orgID)
	err = tx.QueryRow(r.Context(), `INSERT INTO users (organization_id,email,password_hash,full_name,phone,role) VALUES ($1,$2,$3,$4,$5,'owner') RETURNING id`, orgID, in.Email, string(hash), in.FullName, in.Phone).Scan(&userID)
	if err != nil {
		writeError(w, http.StatusConflict, "email is already registered")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "registration commit failed")
		return
	}
	token, _ := a.token(authUser{ID: userID, OrganizationID: orgID, Email: in.Email, Role: "owner", FullName: in.FullName})
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "user": map[string]string{"id": userID, "organization_id": orgID, "email": in.Email, "role": "owner", "full_name": in.FullName}})
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	var u authUser
	var hash string
	err := a.db.QueryRow(r.Context(), `SELECT id, organization_id, email, password_hash, full_name, role FROM users WHERE email=$1`, strings.ToLower(strings.TrimSpace(in.Email))).Scan(&u.ID, &u.OrganizationID, &u.Email, &hash, &u.FullName, &u.Role)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	token, _ := a.token(u)
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, mustUser(r))
}

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	_, _ = a.db.Exec(r.Context(), `INSERT INTO payment_intents (organization_id, lease_id, period_month, due_on, amount_cents, status)
		SELECT l.organization_id, l.id, date_trunc('month', now())::date,
		       (date_trunc('month', now())::date + (l.due_day - 1) * interval '1 day')::date,
		       l.rent_cents,
		       CASE WHEN (date_trunc('month', now())::date + (l.due_day - 1) * interval '1 day')::date < current_date THEN 'overdue' ELSE 'due' END
		FROM leases l WHERE l.organization_id=$1 AND l.status='active'
		ON CONFLICT (lease_id, period_month) DO NOTHING`, u.OrganizationID)
	var out struct {
		Units          int64 `json:"units"`
		OccupiedUnits  int64 `json:"occupied_units"`
		Tenants        int64 `json:"tenants"`
		TotalDueCents  int64 `json:"total_due_cents"`
		CollectedCents int64 `json:"collected_cents"`
		OverdueCents   int64 `json:"overdue_cents"`
		PendingCount   int64 `json:"pending_count"`
	}
	_ = a.db.QueryRow(r.Context(), `SELECT count(*), count(*) FILTER (WHERE status='occupied') FROM units WHERE organization_id=$1`, u.OrganizationID).Scan(&out.Units, &out.OccupiedUnits)
	_ = a.db.QueryRow(r.Context(), `SELECT count(*) FROM tenants WHERE organization_id=$1`, u.OrganizationID).Scan(&out.Tenants)
	_ = a.db.QueryRow(r.Context(), `SELECT COALESCE(sum(amount_cents),0), COALESCE(sum(amount_cents) FILTER (WHERE status='verified'),0), COALESCE(sum(amount_cents) FILTER (WHERE status='overdue'),0), count(*) FILTER (WHERE status IN ('due','tenant_marked_paid','overdue')) FROM payment_intents WHERE organization_id=$1 AND period_month=date_trunc('month', now())::date`, u.OrganizationID).Scan(&out.TotalDueCents, &out.CollectedCents, &out.OverdueCents, &out.PendingCount)
	writeJSON(w, http.StatusOK, out)
}

func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	row := a.db.QueryRow(r.Context(), `SELECT mpesa_paybill, mpesa_till, sms_sender_id, reminder_before_days, reminder_template, escalation_template FROM organization_settings WHERE organization_id=$1`, u.OrganizationID)
	var out struct {
		MPesaPaybill       string `json:"mpesa_paybill"`
		MPesaTill          string `json:"mpesa_till"`
		SMSSenderID        string `json:"sms_sender_id"`
		ReminderBeforeDays int    `json:"reminder_before_days"`
		ReminderTemplate   string `json:"reminder_template"`
		EscalationTemplate string `json:"escalation_template"`
	}
	if err := row.Scan(&out.MPesaPaybill, &out.MPesaTill, &out.SMSSenderID, &out.ReminderBeforeDays, &out.ReminderTemplate, &out.EscalationTemplate); err != nil {
		writeError(w, http.StatusNotFound, "settings not found")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) updateSettings(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var in struct {
		MPesaPaybill       string `json:"mpesa_paybill"`
		MPesaTill          string `json:"mpesa_till"`
		SMSSenderID        string `json:"sms_sender_id"`
		ReminderBeforeDays int    `json:"reminder_before_days"`
		ReminderTemplate   string `json:"reminder_template"`
		EscalationTemplate string `json:"escalation_template"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.ReminderBeforeDays <= 0 {
		in.ReminderBeforeDays = 3
	}
	if in.SMSSenderID == "" {
		in.SMSSenderID = "RentPulse"
	}
	_, err := a.db.Exec(r.Context(), `UPDATE organization_settings SET mpesa_paybill=$2, mpesa_till=$3, sms_sender_id=$4, reminder_before_days=$5, reminder_template=$6, escalation_template=$7, updated_at=now() WHERE organization_id=$1`, u.OrganizationID, in.MPesaPaybill, in.MPesaTill, in.SMSSenderID, in.ReminderBeforeDays, in.ReminderTemplate, in.EscalationTemplate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (a *App) listProperties(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	rows, err := a.db.Query(r.Context(), `SELECT id, name, address, city, created_at FROM properties WHERE organization_id=$1 ORDER BY created_at DESC`, u.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "properties query failed")
		return
	}
	defer rows.Close()
	writeJSON(w, http.StatusOK, collect(rows))
}

func (a *App) createProperty(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var in struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		City    string `json:"city"`
		Units   []struct {
			Label     string `json:"label"`
			RentCents int64  `json:"rent_cents"`
		} `json:"units"`
	}
	if !decode(w, r, &in) || in.Name == "" {
		writeError(w, http.StatusBadRequest, "property name is required")
		return
	}
	tx, _ := a.db.Begin(r.Context())
	defer tx.Rollback(r.Context())
	var id string
	if err := tx.QueryRow(r.Context(), `INSERT INTO properties (organization_id,name,address,city) VALUES ($1,$2,$3,$4) RETURNING id`, u.OrganizationID, in.Name, in.Address, in.City).Scan(&id); err != nil {
		writeError(w, http.StatusInternalServerError, "property create failed")
		return
	}
	for _, unit := range in.Units {
		if unit.Label != "" {
			_, _ = tx.Exec(r.Context(), `INSERT INTO units (organization_id,property_id,label,monthly_rent_cents) VALUES ($1,$2,$3,$4)`, u.OrganizationID, id, unit.Label, unit.RentCents)
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "property commit failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (a *App) listTenants(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	rows, err := a.db.Query(r.Context(), `SELECT t.id, t.full_name, t.phone, t.email, COALESCE(p.name,'') AS property_name, COALESCE(un.label,'') AS unit_label, COALESCE(l.rent_cents,0) AS rent_cents, COALESCE(l.status,'') AS lease_status, t.created_at
		FROM tenants t
		LEFT JOIN leases l ON l.tenant_id=t.id AND l.status='active'
		LEFT JOIN units un ON un.id=l.unit_id
		LEFT JOIN properties p ON p.id=un.property_id
		WHERE t.organization_id=$1 ORDER BY t.created_at DESC`, u.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenants query failed")
		return
	}
	defer rows.Close()
	writeJSON(w, http.StatusOK, collect(rows))
}

func (a *App) createTenant(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var in struct {
		FullName     string `json:"full_name"`
		Phone        string `json:"phone"`
		Email        string `json:"email"`
		NationalID   string `json:"national_id"`
		UnitID       string `json:"unit_id"`
		StartsOn     string `json:"starts_on"`
		DueDay       int    `json:"due_day"`
		RentCents    int64  `json:"rent_cents"`
		DepositCents int64  `json:"deposit_cents"`
	}
	if !decode(w, r, &in) || in.FullName == "" || in.Phone == "" {
		writeError(w, http.StatusBadRequest, "tenant name and phone are required")
		return
	}
	tx, _ := a.db.Begin(r.Context())
	defer tx.Rollback(r.Context())
	var tenantID string
	if err := tx.QueryRow(r.Context(), `INSERT INTO tenants (organization_id,full_name,phone,email,national_id) VALUES ($1,$2,$3,$4,$5) RETURNING id`, u.OrganizationID, in.FullName, in.Phone, in.Email, in.NationalID).Scan(&tenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "tenant create failed")
		return
	}
	if in.UnitID != "" {
		if in.DueDay == 0 {
			in.DueDay = 1
		}
		if in.StartsOn == "" {
			in.StartsOn = time.Now().Format("2006-01-02")
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO leases (organization_id,tenant_id,unit_id,starts_on,due_day,rent_cents,deposit_cents) VALUES ($1,$2,$3,$4,$5,$6,$7)`, u.OrganizationID, tenantID, in.UnitID, in.StartsOn, in.DueDay, in.RentCents, in.DepositCents); err != nil {
			writeError(w, http.StatusBadRequest, "lease create failed")
			return
		}
		_, _ = tx.Exec(r.Context(), `UPDATE units SET status='occupied' WHERE id=$1 AND organization_id=$2`, in.UnitID, u.OrganizationID)
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "tenant commit failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": tenantID})
}

func (a *App) createTenantAccessLink(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	tenantID := r.PathValue("tenantID")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant id is required")
		return
	}
	var exists bool
	if err := a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1 AND organization_id=$2)`, tenantID, u.OrganizationID).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	token, err := a.tenantToken(u.OrganizationID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant token create failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": strings.TrimRight(a.cfg.FrontendOrigin, "/") + "/tenant?token=" + url.QueryEscape(token), "expires_in_days": "30"})
}

func (a *App) importTenants(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "multipart file field is required")
		return
	}
	defer file.Close()
	records, err := readTenantImport(file, header)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, _ := a.db.Begin(r.Context())
	defer tx.Rollback(r.Context())
	imported := 0
	var errs []string
	for i, rec := range records {
		if rec[0] == "" || rec[1] == "" {
			errs = append(errs, fmt.Sprintf("row %d missing name or phone", i+2))
			continue
		}
		_, err := tx.Exec(r.Context(), `INSERT INTO tenants (organization_id,full_name,phone,email) VALUES ($1,$2,$3,$4)`, u.OrganizationID, rec[0], rec[1], rec[2])
		if err != nil {
			errs = append(errs, fmt.Sprintf("row %d: %v", i+2, err))
			continue
		}
		imported++
	}
	errJSON, _ := json.Marshal(errs)
	var jobID string
	_ = tx.QueryRow(r.Context(), `INSERT INTO import_jobs (organization_id,kind,filename,total_rows,imported_rows,failed_rows,errors) VALUES ($1,'tenants',$2,$3,$4,$5,$6) RETURNING id`, u.OrganizationID, header.Filename, len(records), imported, len(errs), errJSON).Scan(&jobID)
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "import commit failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": jobID, "total_rows": len(records), "imported_rows": imported, "failed_rows": len(errs), "errors": errs})
}

func (a *App) exportTenantsCSV(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	rows, err := a.db.Query(r.Context(), `SELECT full_name, phone, email, national_id, created_at FROM tenants WHERE organization_id=$1 ORDER BY full_name`, u.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export failed")
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="rentpulse-tenants.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"Full name", "Phone", "Email", "National ID", "Created at"})
	for rows.Next() {
		var name, phone, email, nid string
		var created time.Time
		_ = rows.Scan(&name, &phone, &email, &nid, &created)
		_ = cw.Write([]string{name, phone, email, nid, created.Format(time.RFC3339)})
	}
	cw.Flush()
}

func (a *App) monthlyExcelReport(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	rows, err := a.db.Query(r.Context(), `SELECT t.full_name, COALESCE(p.name,''), COALESCE(un.label,''), pi.due_on, pi.amount_cents, pi.status
		FROM payment_intents pi
		JOIN leases l ON l.id=pi.lease_id
		JOIN tenants t ON t.id=l.tenant_id
		JOIN units un ON un.id=l.unit_id
		JOIN properties p ON p.id=un.property_id
		WHERE pi.organization_id=$1 AND pi.period_month=date_trunc('month', now())::date
		ORDER BY pi.due_on, t.full_name`, u.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "report query failed")
		return
	}
	defer rows.Close()
	f := excelize.NewFile()
	sheet := "Monthly report"
	f.SetSheetName("Sheet1", sheet)
	headers := []any{"Tenant", "Property", "Unit", "Due on", "Amount KES", "Status"}
	_ = f.SetSheetRow(sheet, "A1", &headers)
	row := 2
	for rows.Next() {
		var tenant, property, unit, status string
		var due time.Time
		var cents int64
		_ = rows.Scan(&tenant, &property, &unit, &due, &cents, &status)
		values := []any{tenant, property, unit, due.Format("2006-01-02"), money(cents), status}
		_ = f.SetSheetRow(sheet, fmt.Sprintf("A%d", row), &values)
		row++
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		writeError(w, http.StatusInternalServerError, "report generation failed")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="rentpulse-monthly-report.xlsx"`)
	_, _ = w.Write(buf.Bytes())
}

func (a *App) markPaid(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var in struct {
		PaymentIntentID string `json:"payment_intent_id"`
		AmountCents     int64  `json:"amount_cents"`
		Provider        string `json:"provider"`
		TransactionRef  string `json:"transaction_ref"`
		EvidenceURL     string `json:"evidence_url"`
	}
	if !decode(w, r, &in) || in.PaymentIntentID == "" || in.TransactionRef == "" {
		writeError(w, http.StatusBadRequest, "payment intent and transaction reference are required")
		return
	}
	var tenantID string
	err := a.db.QueryRow(r.Context(), `SELECT l.tenant_id FROM payment_intents pi JOIN leases l ON l.id=pi.lease_id WHERE pi.id=$1 AND pi.organization_id=$2`, in.PaymentIntentID, u.OrganizationID).Scan(&tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "payment intent not found")
		return
	}
	_, err = a.db.Exec(r.Context(), `INSERT INTO payment_confirmations (organization_id,payment_intent_id,tenant_id,amount_cents,provider,transaction_ref,evidence_url) VALUES ($1,$2,$3,$4,$5,$6,$7);
		UPDATE payment_intents SET status='tenant_marked_paid' WHERE id=$2 AND organization_id=$1`, u.OrganizationID, in.PaymentIntentID, tenantID, in.AmountCents, in.Provider, in.TransactionRef, in.EvidenceURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "payment confirmation failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "pending_verification"})
}

func (a *App) tenantMe(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	rows, err := a.db.Query(r.Context(), `SELECT pi.id, pi.due_on, pi.amount_cents, pi.status, p.name AS property_name, un.label AS unit_label
		FROM payment_intents pi
		JOIN leases l ON l.id=pi.lease_id
		JOIN units un ON un.id=l.unit_id
		JOIN properties p ON p.id=un.property_id
		WHERE pi.organization_id=$1 AND l.tenant_id=$2 AND pi.status IN ('due','overdue','tenant_marked_paid','rejected')
		ORDER BY pi.due_on DESC`, u.OrganizationID, u.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant payment query failed")
		return
	}
	defer rows.Close()
	var tenant struct {
		ID       string           `json:"id"`
		FullName string           `json:"full_name"`
		Phone    string           `json:"phone"`
		Email    string           `json:"email"`
		Payments []map[string]any `json:"payments"`
	}
	if err := a.db.QueryRow(r.Context(), `SELECT id, full_name, phone, email FROM tenants WHERE id=$1 AND organization_id=$2`, u.TenantID, u.OrganizationID).Scan(&tenant.ID, &tenant.FullName, &tenant.Phone, &tenant.Email); err != nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	tenant.Payments = collect(rows)
	writeJSON(w, http.StatusOK, tenant)
}

func (a *App) tenantUpload(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "upload must be 8MB or smaller")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".pdf" {
		writeError(w, http.StatusBadRequest, "only jpg, png, or pdf evidence is accepted")
		return
	}
	token, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "file token failed")
		return
	}
	dir := filepath.Join(a.cfg.UploadDir, u.OrganizationID, u.TenantID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		writeError(w, http.StatusInternalServerError, "upload directory failed")
		return
	}
	path := filepath.Join(dir, token+ext)
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload create failed")
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, io.LimitReader(file, 8<<20)); err != nil {
		writeError(w, http.StatusInternalServerError, "upload write failed")
		return
	}
	publicPath := "/uploads/" + url.PathEscape(u.OrganizationID) + "/" + url.PathEscape(u.TenantID) + "/" + url.PathEscape(token+ext)
	writeJSON(w, http.StatusCreated, map[string]string{"url": strings.TrimRight(a.cfg.PublicBaseURL, "/") + publicPath, "path": publicPath})
}

func (a *App) tenantMarkPaid(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var in struct {
		PaymentIntentID string `json:"payment_intent_id"`
		AmountCents     int64  `json:"amount_cents"`
		Provider        string `json:"provider"`
		TransactionRef  string `json:"transaction_ref"`
		EvidenceURL     string `json:"evidence_url"`
	}
	if !decode(w, r, &in) || in.PaymentIntentID == "" || in.TransactionRef == "" {
		writeError(w, http.StatusBadRequest, "payment intent and transaction reference are required")
		return
	}
	var expectedAmount int64
	err := a.db.QueryRow(r.Context(), `SELECT pi.amount_cents
		FROM payment_intents pi
		JOIN leases l ON l.id=pi.lease_id
		WHERE pi.id=$1 AND pi.organization_id=$2 AND l.tenant_id=$3`, in.PaymentIntentID, u.OrganizationID, u.TenantID).Scan(&expectedAmount)
	if err != nil {
		writeError(w, http.StatusNotFound, "payment intent not found")
		return
	}
	if in.AmountCents <= 0 {
		in.AmountCents = expectedAmount
	}
	_, err = a.db.Exec(r.Context(), `INSERT INTO payment_confirmations (organization_id,payment_intent_id,tenant_id,amount_cents,provider,transaction_ref,evidence_url) VALUES ($1,$2,$3,$4,$5,$6,$7);
		UPDATE payment_intents SET status='tenant_marked_paid' WHERE id=$2 AND organization_id=$1`, u.OrganizationID, in.PaymentIntentID, u.TenantID, in.AmountCents, in.Provider, in.TransactionRef, in.EvidenceURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "payment confirmation failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "pending_landlord_verification"})
}

func (a *App) verifyPayment(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var in struct {
		PaymentIntentID string `json:"payment_intent_id"`
		TransactionRef  string `json:"transaction_ref"`
		Approve         bool   `json:"approve"`
	}
	if !decode(w, r, &in) || in.PaymentIntentID == "" {
		writeError(w, http.StatusBadRequest, "payment intent is required")
		return
	}
	if in.Approve && in.TransactionRef != "" && a.cfg.MPesaConsumerKey != "" {
		ok, err := a.verifyMPesaReference(r.Context(), in.TransactionRef)
		if err != nil {
			writeError(w, http.StatusBadGateway, "mpesa verification failed: "+err.Error())
			return
		}
		if !ok {
			writeError(w, http.StatusConflict, "mpesa reference could not be verified")
			return
		}
	}
	status := "rejected"
	intentStatus := "rejected"
	if in.Approve {
		status = "verified"
		intentStatus = "verified"
	}
	_, err := a.db.Exec(r.Context(), `UPDATE payment_confirmations SET verification_status=$3, verified_by=$4, verified_at=now() WHERE payment_intent_id=$1 AND organization_id=$2 AND verification_status='pending';
		UPDATE payment_intents SET status=$5 WHERE id=$1 AND organization_id=$2`, in.PaymentIntentID, u.OrganizationID, status, u.ID, intentStatus)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "verification update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": intentStatus})
}

func (a *App) runReminders(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	rows, err := a.db.Query(r.Context(), `SELECT pi.id, t.id, t.full_name, t.phone, p.name, un.label, pi.amount_cents, pi.due_on, pi.status, s.reminder_template, s.escalation_template
		FROM payment_intents pi
		JOIN leases l ON l.id=pi.lease_id
		JOIN tenants t ON t.id=l.tenant_id
		JOIN units un ON un.id=l.unit_id
		JOIN properties p ON p.id=un.property_id
		JOIN organization_settings s ON s.organization_id=pi.organization_id
		WHERE pi.organization_id=$1 AND pi.status IN ('due','overdue','tenant_marked_paid')`, u.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reminder query failed")
		return
	}
	defer rows.Close()
	sent, failed, skipped := 0, 0, 0
	for rows.Next() {
		var intentID, tenantID, tenant, phone, property, unit, status, template, escalation string
		var amount int64
		var due time.Time
		_ = rows.Scan(&intentID, &tenantID, &tenant, &phone, &property, &unit, &amount, &due, &status, &template, &escalation)
		if status == "tenant_marked_paid" {
			continue
		}
		body := template
		if status == "overdue" {
			body = escalation
		}
		link := ""
		if token, err := a.tenantToken(u.OrganizationID, tenantID); err == nil {
			link = strings.TrimRight(a.cfg.FrontendOrigin, "/") + "/tenant?token=" + url.QueryEscape(token)
		}
		body = render(body, map[string]string{"tenant": tenant, "amount": "KES " + money(amount), "unit": property + " " + unit, "due_date": due.Format("2 Jan 2006"), "tenant_link": link})
		msgID, sendErr := a.sendMessage(r.Context(), "sms", phone, body)
		commStatus, errText := "sent", ""
		if errors.Is(sendErr, errProviderNotConfigured) {
			commStatus = "skipped"
			errText = sendErr.Error()
			skipped++
		} else if sendErr != nil {
			commStatus = "failed"
			errText = sendErr.Error()
			failed++
		} else {
			sent++
		}
		_, _ = a.db.Exec(r.Context(), `INSERT INTO communications (organization_id,tenant_id,payment_intent_id,channel,recipient,body,status,provider_message_id,error,sent_at) VALUES ($1,$2,$3,'sms',$4,$5,$6,$7,$8,CASE WHEN $6='sent' THEN now() ELSE NULL END)`, u.OrganizationID, tenantID, intentID, phone, body, commStatus, msgID, errText)
	}
	writeJSON(w, http.StatusOK, map[string]int{"sent": sent, "failed": failed, "skipped": skipped})
}

var errProviderNotConfigured = errors.New("messaging provider is not configured")

func (a *App) sendMessage(ctx context.Context, channel, recipient, body string) (string, error) {
	if a.cfg.TwilioAccountSID == "" || a.cfg.TwilioAuthToken == "" || a.cfg.TwilioFromSMS == "" {
		return "", errProviderNotConfigured
	}
	form := url.Values{}
	form.Set("To", recipient)
	form.Set("From", a.cfg.TwilioFromSMS)
	form.Set("Body", body)
	endpoint := "https://api.twilio.com/2010-04-01/Accounts/" + a.cfg.TwilioAccountSID + "/Messages.json"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	req.SetBasicAuth(a.cfg.TwilioAccountSID, a.cfg.TwilioAuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		SID     string `json:"sid"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("twilio returned %d: %s", resp.StatusCode, out.Message)
	}
	return out.SID, nil
}

func (a *App) verifyMPesaReference(ctx context.Context, ref string) (bool, error) {
	tokenReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.MPesaBaseURL+"/oauth/v1/generate?grant_type=client_credentials", nil)
	tokenReq.SetBasicAuth(a.cfg.MPesaConsumerKey, a.cfg.MPesaSecret)
	tokenResp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		return false, err
	}
	defer tokenResp.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&token); err != nil {
		return false, err
	}
	if tokenResp.StatusCode >= 300 || token.AccessToken == "" {
		return false, fmt.Errorf("oauth status %d", tokenResp.StatusCode)
	}
	passwordSeed := a.cfg.MPesaShortCode + "RentPulse" + time.Now().Format("20060102150405")
	sum := hmac.New(sha256.New, []byte(a.cfg.MPesaSecret))
	sum.Write([]byte(passwordSeed))
	signature := base64.StdEncoding.EncodeToString(sum.Sum(nil))
	payload := map[string]string{"TransactionID": ref, "PartyA": a.cfg.MPesaShortCode, "IdentifierType": "4", "ResultURL": "https://example.com/mpesa/result", "QueueTimeOutURL": "https://example.com/mpesa/timeout", "Remarks": "RentPulse verification", "Occasion": "Rent verification", "SecurityCredential": signature, "CommandID": "TransactionStatusQuery"}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.MPesaBaseURL+"/mpesa/transactionstatus/v1/query", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		return false, fmt.Errorf("transaction status returned %d", resp.StatusCode)
	}
	code, _ := out["ResponseCode"].(string)
	return code == "0", nil
}

func (a *App) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		token, err := jwt.ParseWithClaims(strings.TrimPrefix(header, "Bearer "), jwt.MapClaims{}, func(token *jwt.Token) (any, error) {
			return []byte(a.cfg.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		claims := token.Claims.(jwt.MapClaims)
		u := authUser{ID: fmt.Sprint(claims["sub"]), OrganizationID: fmt.Sprint(claims["org"]), Email: fmt.Sprint(claims["email"]), Role: fmt.Sprint(claims["role"]), FullName: fmt.Sprint(claims["name"]), TenantID: fmt.Sprint(claims["tenant_id"])}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (a *App) tenantAuth(next http.Handler) http.Handler {
	return a.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := mustUser(r)
		if u.Role != "tenant" || u.TenantID == "" || u.TenantID == "<nil>" {
			writeError(w, http.StatusForbidden, "tenant portal token required")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (a *App) staffAuth(next http.Handler) http.Handler {
	return a.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := mustUser(r)
		if u.Role == "tenant" {
			writeError(w, http.StatusForbidden, "staff account required")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (a *App) token(u authUser) (string, error) {
	claims := jwt.MapClaims{"sub": u.ID, "org": u.OrganizationID, "email": u.Email, "role": u.Role, "name": u.FullName, "exp": time.Now().Add(24 * time.Hour).Unix(), "iat": time.Now().Unix()}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.cfg.JWTSecret))
}

func (a *App) tenantToken(orgID, tenantID string) (string, error) {
	claims := jwt.MapClaims{"sub": tenantID, "tenant_id": tenantID, "org": orgID, "role": "tenant", "exp": time.Now().Add(30 * 24 * time.Hour).Unix(), "iat": time.Now().Unix()}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.cfg.JWTSecret))
}

func (a *App) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", a.cfg.FrontendOrigin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				a.logger.Error("panic", "error", err)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func readTenantImport(file multipart.File, header *multipart.FileHeader) ([][3]string, error) {
	name := strings.ToLower(header.Filename)
	var rows [][]string
	if strings.HasSuffix(name, ".xlsx") {
		tmp, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}
		xl, err := excelize.OpenReader(bytes.NewReader(tmp))
		if err != nil {
			return nil, err
		}
		sheet := xl.GetSheetName(0)
		rows, err = xl.GetRows(sheet)
		if err != nil {
			return nil, err
		}
	} else {
		parsed, err := csv.NewReader(file).ReadAll()
		if err != nil {
			return nil, err
		}
		rows = parsed
	}
	if len(rows) < 2 {
		return nil, errors.New("file must include a header row and at least one tenant")
	}
	var out [][3]string
	for _, row := range rows[1:] {
		var rec [3]string
		for i := 0; i < len(row) && i < 3; i++ {
			rec[i] = strings.TrimSpace(row[i])
		}
		out = append(out, rec)
	}
	return out, nil
}

func collect(rows pgx.Rows) []map[string]any {
	fields := rows.FieldDescriptions()
	out := []map[string]any{}
	for rows.Next() {
		values, _ := rows.Values()
		item := map[string]any{}
		for i, f := range fields {
			item[string(f.Name)] = values[i]
		}
		out = append(out, item)
	}
	return out
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func mustUser(r *http.Request) authUser {
	return r.Context().Value(userKey).(authUser)
}

func money(cents int64) string {
	return strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
}

func render(template string, values map[string]string) string {
	for key, value := range values {
		template = strings.ReplaceAll(template, "{{"+key+"}}", value)
	}
	return template
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}

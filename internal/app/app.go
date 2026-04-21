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
	"sync"
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
	limits map[string]*rateBucket
	mu     sync.Mutex
}

type rateBucket struct {
	WindowStart time.Time
	Count       int
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
	return &App{cfg: cfg, db: db, logger: logger, limits: map[string]*rateBucket{}}
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
	mux.Handle("GET /api/units", a.staffAuth(http.HandlerFunc(a.listUnits)))
	mux.Handle("POST /api/units", a.staffAuth(http.HandlerFunc(a.createUnit)))
	mux.Handle("GET /api/tenants", a.staffAuth(http.HandlerFunc(a.listTenants)))
	mux.Handle("POST /api/tenants", a.staffAuth(http.HandlerFunc(a.createTenant)))
	mux.Handle("POST /api/tenants/{tenantID}/access-link", a.staffAuth(http.HandlerFunc(a.createTenantAccessLink)))
	mux.Handle("GET /api/imports/tenants/template.csv", a.staffAuth(http.HandlerFunc(a.tenantImportTemplate)))
	mux.Handle("POST /api/imports/tenants/preview", a.staffAuth(http.HandlerFunc(a.previewTenantImport)))
	mux.Handle("POST /api/imports/tenants", a.staffAuth(http.HandlerFunc(a.importTenants)))
	mux.Handle("GET /api/exports/tenants.csv", a.staffAuth(http.HandlerFunc(a.exportTenantsCSV)))
	mux.Handle("GET /api/reports/monthly.xlsx", a.staffAuth(http.HandlerFunc(a.monthlyExcelReport)))
	mux.Handle("GET /api/payments", a.staffAuth(http.HandlerFunc(a.listPayments)))
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
		{"id": "free", "name": "Free", "price_cents": 0, "unit_limit": 2, "requests_per_minute": 60, "features": []string{"Tenant directory", "Manual payment tracking"}},
		{"id": "starter", "name": "Starter", "price_cents": 50000, "unit_limit": 2, "requests_per_minute": 120, "features": []string{"Automated invoices", "Basic rent tracking", "CSV exports", "Import wizard"}},
		{"id": "pro", "name": "Pro", "price_cents": 120000, "unit_limit": 10, "requests_per_minute": 300, "features": []string{"WhatsApp/SMS reminders", "M-Pesa verification workflow", "Excel reports", "Imports"}},
		{"id": "agency", "name": "Agency", "price_cents": nil, "unit_limit": nil, "requests_per_minute": 1000, "features": []string{"Unlimited units", "Multi-user access", "Branding", "Priority support"}},
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
	row := a.db.QueryRow(r.Context(), `SELECT s.mpesa_paybill, s.mpesa_till, s.sms_sender_id, s.reminder_before_days, s.reminder_template, s.escalation_template, o.plan
		FROM organization_settings s
		JOIN organizations o ON o.id=s.organization_id
		WHERE s.organization_id=$1`, u.OrganizationID)
	var out struct {
		MPesaPaybill       string `json:"mpesa_paybill"`
		MPesaTill          string `json:"mpesa_till"`
		SMSSenderID        string `json:"sms_sender_id"`
		ReminderBeforeDays int    `json:"reminder_before_days"`
		ReminderTemplate   string `json:"reminder_template"`
		EscalationTemplate string `json:"escalation_template"`
		Plan               string `json:"plan"`
	}
	if err := row.Scan(&out.MPesaPaybill, &out.MPesaTill, &out.SMSSenderID, &out.ReminderBeforeDays, &out.ReminderTemplate, &out.EscalationTemplate, &out.Plan); err != nil {
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
	if len(in.Units) > 0 {
		if err := a.enforceUnitCapacity(r.Context(), u.OrganizationID, len(in.Units)); err != nil {
			writeError(w, http.StatusPaymentRequired, err.Error())
			return
		}
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

func (a *App) listUnits(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	rows, err := a.db.Query(r.Context(), `SELECT un.id, un.label, un.monthly_rent_cents, un.status, p.id AS property_id, p.name AS property_name
		FROM units un
		JOIN properties p ON p.id=un.property_id
		WHERE un.organization_id=$1
		ORDER BY p.name, un.label`, u.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "units query failed")
		return
	}
	defer rows.Close()
	writeJSON(w, http.StatusOK, collect(rows))
}

func (a *App) createUnit(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var in struct {
		PropertyID string `json:"property_id"`
		Label      string `json:"label"`
		RentCents  int64  `json:"rent_cents"`
	}
	if !decode(w, r, &in) || in.PropertyID == "" || in.Label == "" {
		writeError(w, http.StatusBadRequest, "property and unit label are required")
		return
	}
	if err := a.enforceUnitCapacity(r.Context(), u.OrganizationID, 1); err != nil {
		writeError(w, http.StatusPaymentRequired, err.Error())
		return
	}
	var id string
	err := a.db.QueryRow(r.Context(), `INSERT INTO units (organization_id,property_id,label,monthly_rent_cents) VALUES ($1,$2,$3,$4) RETURNING id`, u.OrganizationID, in.PropertyID, in.Label, in.RentCents).Scan(&id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unit create failed")
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

func (a *App) tenantImportTemplate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="rentpulse-tenant-import-template.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"full_name", "phone", "email", "national_id", "property_name", "property_address", "city", "unit_label", "monthly_rent_kes", "lease_start_date", "due_day", "deposit_kes"})
	samples := [][]string{
		{"Alice Wanjiku", "+254711111111", "alice@example.com", "12345678", "Nairobi Heights", "Kilimani Road", "Nairobi", "A-101", "45000", "2026-05-01", "5", "45000"},
		{"Samuel Mutua", "+254722222222", "samuel@example.com", "23456789", "Nairobi Heights", "Kilimani Road", "Nairobi", "A-102", "38000", "2026-05-01", "5", "38000"},
		{"Grace Njeri", "+254733333333", "grace@example.com", "34567890", "Skyline Apartments", "Mombasa Road", "Nairobi", "B-12", "52000", "2026-05-01", "1", "52000"},
		{"Kevin Otieno", "+254744444444", "kevin@example.com", "45678901", "Skyline Apartments", "Mombasa Road", "Nairobi", "B-14", "52000", "2026-05-01", "1", "52000"},
		{"Mary Achieng", "+254755555555", "mary@example.com", "56789012", "Meridian Heights", "Waiyaki Way", "Nairobi", "PH-1", "120000", "2026-05-01", "3", "120000"},
		{"John Kariuki", "+254766666666", "john@example.com", "67890123", "Meridian Heights", "Waiyaki Way", "Nairobi", "C-04", "65000", "2026-05-01", "3", "65000"},
		{"Faith Muthoni", "+254777777777", "faith@example.com", "78901234", "Garden Court", "Kiambu Road", "Nairobi", "G-07", "30000", "2026-05-01", "10", "30000"},
		{"Brian Ouma", "+254788888888", "brian@example.com", "89012345", "Garden Court", "Kiambu Road", "Nairobi", "G-08", "30000", "2026-05-01", "10", "30000"},
	}
	for _, sample := range samples {
		_ = cw.Write(sample)
	}
	cw.Flush()
}

func (a *App) previewTenantImport(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "multipart file field is required")
		return
	}
	defer file.Close()
	table, err := readImportTable(file, header)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	mapping := suggestTenantMapping(table.Headers)
	result := validateTenantImport(table, attachHeaders(mapping, table.Headers), 10)
	writeJSON(w, http.StatusOK, map[string]any{
		"filename":          header.Filename,
		"headers":           table.Headers,
		"sample_rows":       firstRows(table.Rows, 10),
		"total_rows":        len(table.Rows),
		"suggested_mapping": mapping,
		"required_fields":   []string{"full_name", "phone"},
		"optional_fields":   []string{"email", "national_id", "property_name", "property_address", "city", "unit_label", "monthly_rent_kes", "lease_start_date", "due_day", "deposit_kes"},
		"validation":        result,
	})
}

func (a *App) importTenants(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "multipart file field is required")
		return
	}
	defer file.Close()
	table, err := readImportTable(file, header)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	mapping := suggestTenantMapping(table.Headers)
	if rawMapping := r.FormValue("mapping"); rawMapping != "" {
		if err := json.Unmarshal([]byte(rawMapping), &mapping); err != nil {
			writeError(w, http.StatusBadRequest, "mapping must be valid json")
			return
		}
	}
	mapping = attachHeaders(mapping, table.Headers)
	validation := validateTenantImport(table, mapping, len(table.Rows))
	if validation.ValidRows == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "no valid tenant rows found", "validation": validation})
		return
	}
	unitsToCreate := 0
	for _, row := range table.Rows {
		rec := tenantImportRecord(row, mapping)
		if rec.PropertyName != "" && rec.UnitLabel != "" {
			unitsToCreate++
		}
	}
	if unitsToCreate > 0 {
		if err := a.enforceUnitCapacity(r.Context(), u.OrganizationID, unitsToCreate); err != nil {
			writeError(w, http.StatusPaymentRequired, err.Error())
			return
		}
	}
	tx, _ := a.db.Begin(r.Context())
	defer tx.Rollback(r.Context())
	imported := 0
	var errs []string
	for i, row := range table.Rows {
		rec := tenantImportRecord(row, mapping)
		if rec.FullName == "" || rec.Phone == "" {
			errs = append(errs, fmt.Sprintf("row %d missing full_name or phone", i+2))
			continue
		}
		var tenantID string
		err := tx.QueryRow(r.Context(), `INSERT INTO tenants (organization_id,full_name,phone,email,national_id) VALUES ($1,$2,$3,$4,$5) RETURNING id`, u.OrganizationID, rec.FullName, rec.Phone, rec.Email, rec.NationalID).Scan(&tenantID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("row %d: %v", i+2, err))
			continue
		}
		if rec.PropertyName != "" && rec.UnitLabel != "" {
			propertyID, err := ensureProperty(r.Context(), tx, u.OrganizationID, rec.PropertyName, rec.PropertyAddress, rec.City)
			if err != nil {
				errs = append(errs, fmt.Sprintf("row %d property: %v", i+2, err))
				continue
			}
			unitID, err := ensureUnit(r.Context(), tx, u.OrganizationID, propertyID, rec.UnitLabel, rec.MonthlyRentCents)
			if err != nil {
				errs = append(errs, fmt.Sprintf("row %d unit: %v", i+2, err))
				continue
			}
			startsOn := rec.LeaseStartDate
			if startsOn == "" {
				startsOn = time.Now().Format("2006-01-02")
			}
			dueDay := rec.DueDay
			if dueDay == 0 {
				dueDay = 1
			}
			if _, err := tx.Exec(r.Context(), `INSERT INTO leases (organization_id,tenant_id,unit_id,starts_on,due_day,rent_cents,deposit_cents) VALUES ($1,$2,$3,$4,$5,$6,$7)`, u.OrganizationID, tenantID, unitID, startsOn, dueDay, rec.MonthlyRentCents, rec.DepositCents); err != nil {
				errs = append(errs, fmt.Sprintf("row %d lease: %v", i+2, err))
				continue
			}
			_, _ = tx.Exec(r.Context(), `UPDATE units SET status='occupied' WHERE id=$1 AND organization_id=$2`, unitID, u.OrganizationID)
		}
		imported++
	}
	errJSON, _ := json.Marshal(errs)
	var jobID string
	_ = tx.QueryRow(r.Context(), `INSERT INTO import_jobs (organization_id,kind,filename,total_rows,imported_rows,failed_rows,errors) VALUES ($1,'tenants',$2,$3,$4,$5,$6) RETURNING id`, u.OrganizationID, header.Filename, len(table.Rows), imported, len(errs), errJSON).Scan(&jobID)
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "import commit failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": jobID, "total_rows": len(table.Rows), "imported_rows": imported, "failed_rows": len(errs), "errors": errs})
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

func (a *App) listPayments(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	rows, err := a.db.Query(r.Context(), `SELECT pi.id, pi.due_on, pi.period_month, pi.amount_cents, pi.status,
		       t.full_name AS tenant_name, t.phone AS tenant_phone, p.name AS property_name, un.label AS unit_label,
		       COALESCE(pc.provider,'') AS provider, COALESCE(pc.transaction_ref,'') AS transaction_ref, COALESCE(pc.evidence_url,'') AS evidence_url,
		       pc.created_at AS submitted_at
		FROM payment_intents pi
		JOIN leases l ON l.id=pi.lease_id
		JOIN tenants t ON t.id=l.tenant_id
		JOIN units un ON un.id=l.unit_id
		JOIN properties p ON p.id=un.property_id
		LEFT JOIN LATERAL (
			SELECT provider, transaction_ref, evidence_url, created_at
			FROM payment_confirmations
			WHERE payment_intent_id=pi.id
			ORDER BY created_at DESC
			LIMIT 1
		) pc ON true
		WHERE pi.organization_id=$1
		ORDER BY pi.due_on DESC, t.full_name`, u.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "payments query failed")
		return
	}
	defer rows.Close()
	writeJSON(w, http.StatusOK, collect(rows))
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
		if !a.allowRequest(r.Context(), u.OrganizationID) {
			writeError(w, http.StatusTooManyRequests, "plan request limit reached; retry in a minute or upgrade your plan")
			return
		}
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

type importTable struct {
	Headers []string
	Rows    [][]string
}

type tenantImport struct {
	FullName         string
	Phone            string
	Email            string
	NationalID       string
	PropertyName     string
	PropertyAddress  string
	City             string
	UnitLabel        string
	MonthlyRentCents int64
	LeaseStartDate   string
	DueDay           int
	DepositCents     int64
}

type importValidation struct {
	ValidRows   int      `json:"valid_rows"`
	InvalidRows int      `json:"invalid_rows"`
	Errors      []string `json:"errors"`
}

func readImportTable(file multipart.File, header *multipart.FileHeader) (importTable, error) {
	name := strings.ToLower(header.Filename)
	var rows [][]string
	if strings.HasSuffix(name, ".xlsx") {
		tmp, err := io.ReadAll(file)
		if err != nil {
			return importTable{}, err
		}
		xl, err := excelize.OpenReader(bytes.NewReader(tmp))
		if err != nil {
			return importTable{}, err
		}
		sheet := xl.GetSheetName(0)
		rows, err = xl.GetRows(sheet)
		if err != nil {
			return importTable{}, err
		}
	} else {
		parsed, err := csv.NewReader(file).ReadAll()
		if err != nil {
			return importTable{}, err
		}
		rows = parsed
	}
	if len(rows) < 2 {
		return importTable{}, errors.New("file must include a header row and at least one tenant")
	}
	headers := make([]string, len(rows[0]))
	for i, header := range rows[0] {
		headers[i] = strings.TrimSpace(header)
	}
	var data [][]string
	for _, row := range rows[1:] {
		normalized := make([]string, len(headers))
		empty := true
		for i := range headers {
			if i < len(row) {
				normalized[i] = strings.TrimSpace(row[i])
				if normalized[i] != "" {
					empty = false
				}
			}
		}
		if !empty {
			data = append(data, normalized)
		}
	}
	return importTable{Headers: headers, Rows: data}, nil
}

func suggestTenantMapping(headers []string) map[string]string {
	aliases := map[string][]string{
		"full_name":        {"full_name", "full name", "tenant", "tenant name", "name", "resident", "resident name"},
		"phone":            {"phone", "phone number", "mobile", "mobile number", "msisdn", "contact", "contact phone"},
		"email":            {"email", "email address", "mail"},
		"national_id":      {"national_id", "national id", "id number", "id", "passport"},
		"property_name":    {"property_name", "property name", "property", "building", "estate", "apartment"},
		"property_address": {"property_address", "property address", "address", "location"},
		"city":             {"city", "town"},
		"unit_label":       {"unit_label", "unit label", "unit", "unit no", "unit number", "house", "house number", "door"},
		"monthly_rent_kes": {"monthly_rent_kes", "monthly rent", "rent", "rent amount", "amount", "monthly_rent"},
		"lease_start_date": {"lease_start_date", "lease start", "start date", "starts on", "move in", "move-in date"},
		"due_day":          {"due_day", "due day", "rent due day", "due"},
		"deposit_kes":      {"deposit_kes", "deposit", "security deposit"},
	}
	mapping := map[string]string{}
	for field, names := range aliases {
		for _, header := range headers {
			clean := normalizeHeader(header)
			for _, name := range names {
				if clean == normalizeHeader(name) {
					mapping[field] = header
					break
				}
			}
			if mapping[field] != "" {
				break
			}
		}
	}
	return mapping
}

func validateTenantImport(table importTable, mapping map[string]string, limit int) importValidation {
	result := importValidation{Errors: []string{}}
	if mapping["full_name"] == "" {
		result.Errors = append(result.Errors, "Map a column to full_name")
	}
	if mapping["phone"] == "" {
		result.Errors = append(result.Errors, "Map a column to phone")
	}
	max := len(table.Rows)
	if limit > 0 && limit < max {
		max = limit
	}
	for i := 0; i < max; i++ {
		rec := tenantImportRecord(table.Rows[i], mapping)
		rowErrs := validateTenantRecord(rec)
		if len(rowErrs) > 0 {
			result.InvalidRows++
			for _, err := range rowErrs {
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: %s", i+2, err))
			}
			continue
		}
		result.ValidRows++
	}
	return result
}

func validateTenantRecord(rec tenantImport) []string {
	var errs []string
	if rec.FullName == "" {
		errs = append(errs, "full_name is required")
	}
	if rec.Phone == "" {
		errs = append(errs, "phone is required")
	}
	if rec.DueDay < 0 || rec.DueDay > 28 {
		errs = append(errs, "due_day must be between 1 and 28")
	}
	if rec.LeaseStartDate != "" {
		if _, err := time.Parse("2006-01-02", rec.LeaseStartDate); err != nil {
			errs = append(errs, "lease_start_date must use YYYY-MM-DD")
		}
	}
	if rec.UnitLabel != "" && rec.MonthlyRentCents <= 0 {
		errs = append(errs, "monthly_rent_kes is required when unit_label is provided")
	}
	if rec.PropertyName == "" && rec.UnitLabel != "" {
		errs = append(errs, "property_name is required when unit_label is provided")
	}
	return errs
}

func tenantImportRecord(row []string, mapping map[string]string) tenantImport {
	get := func(field string) string {
		header := mapping[field]
		if header == "" {
			return ""
		}
		index := -1
		for i, value := range mappingRowHeaders(mapping, field) {
			if value == header {
				index = i
				break
			}
		}
		if index < 0 || index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
	return tenantImport{
		FullName:         get("full_name"),
		Phone:            get("phone"),
		Email:            strings.ToLower(get("email")),
		NationalID:       get("national_id"),
		PropertyName:     get("property_name"),
		PropertyAddress:  get("property_address"),
		City:             get("city"),
		UnitLabel:        get("unit_label"),
		MonthlyRentCents: parseKESCents(get("monthly_rent_kes")),
		LeaseStartDate:   get("lease_start_date"),
		DueDay:           parseInt(get("due_day")),
		DepositCents:     parseKESCents(get("deposit_kes")),
	}
}

func mappingRowHeaders(mapping map[string]string, field string) []string {
	headersRaw, ok := mapping["__headers"]
	if !ok || headersRaw == "" {
		return []string{}
	}
	return strings.Split(headersRaw, "\x1f")
}

func attachHeaders(mapping map[string]string, headers []string) map[string]string {
	out := map[string]string{}
	for key, value := range mapping {
		out[key] = value
	}
	out["__headers"] = strings.Join(headers, "\x1f")
	return out
}

func firstRows(rows [][]string, n int) [][]string {
	if len(rows) <= n {
		return rows
	}
	return rows[:n]
}

func normalizeHeader(header string) string {
	header = strings.ToLower(strings.TrimSpace(header))
	header = strings.ReplaceAll(header, "_", " ")
	header = strings.ReplaceAll(header, "-", " ")
	header = strings.Join(strings.Fields(header), " ")
	return header
}

func parseKESCents(value string) int64 {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ",", "")
	value = strings.TrimPrefix(strings.ToLower(value), "kes")
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	amount, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return int64(amount * 100)
}

func parseInt(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	n, _ := strconv.Atoi(value)
	return n
}

func ensureProperty(ctx context.Context, tx pgx.Tx, orgID, name, address, city string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM properties WHERE organization_id=$1 AND lower(name)=lower($2) LIMIT 1`, orgID, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	err = tx.QueryRow(ctx, `INSERT INTO properties (organization_id,name,address,city) VALUES ($1,$2,$3,$4) RETURNING id`, orgID, name, address, city).Scan(&id)
	return id, err
}

func ensureUnit(ctx context.Context, tx pgx.Tx, orgID, propertyID, label string, rentCents int64) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM units WHERE organization_id=$1 AND property_id=$2 AND lower(label)=lower($3) LIMIT 1`, orgID, propertyID, label).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	err = tx.QueryRow(ctx, `INSERT INTO units (organization_id,property_id,label,monthly_rent_cents,status) VALUES ($1,$2,$3,$4,'vacant') RETURNING id`, orgID, propertyID, label, rentCents).Scan(&id)
	return id, err
}

func (a *App) allowRequest(ctx context.Context, orgID string) bool {
	plan := a.organizationPlan(ctx, orgID)
	limit := planRequestsPerMinute(plan)
	key := orgID + ":" + plan
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	bucket := a.limits[key]
	if bucket == nil || now.Sub(bucket.WindowStart) >= time.Minute {
		a.limits[key] = &rateBucket{WindowStart: now, Count: 1}
		return true
	}
	if bucket.Count >= limit {
		return false
	}
	bucket.Count++
	return true
}

func (a *App) enforceUnitCapacity(ctx context.Context, orgID string, additional int) error {
	limit := planUnitLimit(a.organizationPlan(ctx, orgID))
	if limit < 0 {
		return nil
	}
	var current int
	if err := a.db.QueryRow(ctx, `SELECT count(*) FROM units WHERE organization_id=$1`, orgID).Scan(&current); err != nil {
		return err
	}
	if current+additional > limit {
		return fmt.Errorf("current plan allows %d unit(s); upgrade in Settings to add more", limit)
	}
	return nil
}

func (a *App) organizationPlan(ctx context.Context, orgID string) string {
	var plan string
	if err := a.db.QueryRow(ctx, `SELECT plan FROM organizations WHERE id=$1`, orgID).Scan(&plan); err != nil || plan == "" {
		return "free"
	}
	return plan
}

func planUnitLimit(plan string) int {
	switch plan {
	case "agency":
		return -1
	case "pro":
		return 10
	default:
		return 2
	}
}

func planRequestsPerMinute(plan string) int {
	switch plan {
	case "agency":
		return 1000
	case "pro":
		return 300
	case "starter":
		return 120
	default:
		return 60
	}
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

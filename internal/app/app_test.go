package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Thaura644/RntPuls/internal/config"
)

func TestHealthEndpoint(t *testing.T) {
	// This test requires a real database, so we skip it in unit tests
	t.Skip("requires database connection")
}

func TestPlansEndpoint(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/api/plans", nil)
	w := httptest.NewRecorder()

	app.plans(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var plans []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&plans); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(plans) != 4 {
		t.Fatalf("expected 4 plans, got %d", len(plans))
	}

	planNames := []string{"free", "starter", "pro", "agency"}
	for i, name := range planNames {
		if plans[i]["id"] != name {
			t.Errorf("plan %d expected id %q, got %q", i, name, plans[i]["id"])
		}
	}
}

func TestRegisterValidation(t *testing.T) {
	app := &App{}

	tests := []struct {
		name   string
		body   string
		status int
	}{
		{
			name:   "empty body",
			body:   `{}`,
			status: http.StatusBadRequest,
		},
		{
			name:   "short password",
			body:   `{"organization_name":"Test","full_name":"User","email":"user@test.com","phone":"123","password":"short"}`,
			status: http.StatusBadRequest,
		},
		{
			name:   "missing email",
			body:   `{"organization_name":"Test","full_name":"User","password":"longenoughpassword"}`,
			status: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			app.register(w, req)

			if w.Code != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, w.Code)
			}
		})
	}
}

func TestLoginValidation(t *testing.T) {
	t.Skip("requires database connection")
}

func TestAuthMiddlewareMissingToken(t *testing.T) {
	app := &App{}

	handler := app.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	app := &App{cfg: loadTestConfig()}

	handler := app.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer invalidtoken")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestTenantAuthMiddleware(t *testing.T) {
	app := &App{cfg: loadTestConfig()}

	handler := app.tenantAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	token, _ := app.token(authUser{ID: "1", OrganizationID: "1", Email: "user@test.com", Role: "owner", FullName: "Test"})

	req := httptest.NewRequest(http.MethodGet, "/api/tenant/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestStaffAuthMiddleware(t *testing.T) {
	app := &App{cfg: loadTestConfig()}

	handler := app.staffAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	token, _ := app.tenantToken("org1", "tenant1")

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestCORSHeaders(t *testing.T) {
	app := &App{cfg: loadTestConfig()}

	handler := app.cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}

	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("expected CORS origin header, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestMoneyFormatting(t *testing.T) {
	tests := []struct {
		cents    int64
		expected string
	}{
		{0, "0.00"},
		{100, "1.00"},
		{12345, "123.45"},
		{8500000, "85000.00"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatMoney(tt.cents)
			if result != tt.expected {
				t.Errorf("formatMoney(%d) = %q, want %q", tt.cents, result, tt.expected)
			}
		})
	}
}

func TestRenderTemplate(t *testing.T) {
	template := "Hello {{tenant}}, your rent is {{amount}} due on {{due_date}}"
	values := map[string]string{
		"tenant":   "John",
		"amount":   "KES 50000.00",
		"due_date": "1 May 2026",
	}

	result := render(template, values)
	expected := "Hello John, your rent is KES 50000.00 due on 1 May 2026"

	if result != expected {
		t.Errorf("render() = %q, want %q", result, expected)
	}
}

func TestParseKESCents(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"", 0},
		{"100", 10000},
		{"1,000", 100000},
		{"KES 50000", 5000000},
		{"kes 12345.67", 1234567},
		{"invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseKESCents(tt.input)
			if result != tt.expected {
				t.Errorf("parseKESCents(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizePaymentMethod(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"bank", "bank"},
		{"BANK", "bank"},
		{"bank transfer", "bank"},
		{"mpesa", "mpesa_paybill"},
		{"M-Pesa Paybill", "m-pesa paybill"},
		{"paybill", "mpesa_paybill"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizePaymentMethod(tt.input)
			if result != tt.expected {
				t.Errorf("normalizePaymentMethod(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeHeader(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Full Name", "full name"},
		{"full_name", "full name"},
		{"FULL-NAME", "full name"},
		{"  Phone  Number  ", "phone number"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeHeader(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeHeader(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSuggestTenantMapping(t *testing.T) {
	headers := []string{"Full Name", "Phone Number", "Email", "Property Name", "Unit", "Rent Amount"}

	mapping := suggestTenantMapping(headers)

	if mapping["full_name"] != "Full Name" {
		t.Errorf("expected full_name mapping, got %q", mapping["full_name"])
	}
	if mapping["phone"] != "Phone Number" {
		t.Errorf("expected phone mapping, got %q", mapping["phone"])
	}
	if mapping["email"] != "Email" {
		t.Errorf("expected email mapping, got %q", mapping["email"])
	}
	if mapping["property_name"] != "Property Name" {
		t.Errorf("expected property_name mapping, got %q", mapping["property_name"])
	}
	if mapping["unit_label"] != "Unit" {
		t.Errorf("expected unit_label mapping, got %q", mapping["unit_label"])
	}
	if mapping["monthly_rent_kes"] != "Rent Amount" {
		t.Errorf("expected monthly_rent_kes mapping, got %q", mapping["monthly_rent_kes"])
	}
}

func TestValidateTenantRecord(t *testing.T) {
	tests := []struct {
		name   string
		rec    tenantImport
		valid  bool
	}{
		{
			name:  "valid record",
			rec:   tenantImport{FullName: "John", Phone: "+254700000000"},
			valid: true,
		},
		{
			name:  "missing name",
			rec:   tenantImport{Phone: "+254700000000"},
			valid: false,
		},
		{
			name:  "missing phone",
			rec:   tenantImport{FullName: "John"},
			valid: false,
		},
		{
			name:  "invalid due day",
			rec:   tenantImport{FullName: "John", Phone: "+254700000000", DueDay: 30},
			valid: false,
		},
		{
			name:  "unit without property",
			rec:   tenantImport{FullName: "John", Phone: "+254700000000", UnitLabel: "A1"},
			valid: false,
		},
		{
			name:  "bank without account",
			rec:   tenantImport{FullName: "John", Phone: "+254700000000", PaymentMethod: "bank"},
			valid: false,
		},
		{
			name:  "mpesa without paybill",
			rec:   tenantImport{FullName: "John", Phone: "+254700000000", PaymentMethod: "mpesa_paybill"},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateTenantRecord(tt.rec)
			if tt.valid && len(errs) > 0 {
				t.Errorf("expected valid record, got errors: %v", errs)
			}
			if !tt.valid && len(errs) == 0 {
				t.Error("expected invalid record, got no errors")
			}
		})
	}
}

func TestPlanUnitLimits(t *testing.T) {
	tests := []struct {
		plan   string
		limit  int
	}{
		{"free", 2},
		{"starter", 2},
		{"pro", 10},
		{"agency", -1},
		{"unknown", 2},
	}

	for _, tt := range tests {
		t.Run(tt.plan, func(t *testing.T) {
			result := planUnitLimit(tt.plan)
			if result != tt.limit {
				t.Errorf("planUnitLimit(%q) = %d, want %d", tt.plan, result, tt.limit)
			}
		})
	}
}

func TestPlanRequestsPerMinute(t *testing.T) {
	tests := []struct {
		plan  string
		limit int
	}{
		{"free", 60},
		{"starter", 120},
		{"pro", 300},
		{"agency", 1000},
		{"unknown", 60},
	}

	for _, tt := range tests {
		t.Run(tt.plan, func(t *testing.T) {
			result := planRequestsPerMinute(tt.plan)
			if result != tt.limit {
				t.Errorf("planRequestsPerMinute(%q) = %d, want %d", tt.plan, result, tt.limit)
			}
		})
	}
}

func TestGenerateID(t *testing.T) {
	id := generateID(16)
	if len(id) != 32 {
		t.Errorf("expected 32 char hex string, got %d", len(id))
	}

	id2 := generateID(16)
	if id == id2 {
		t.Error("expected unique IDs")
	}
}

func TestRandomHex(t *testing.T) {
	hex, err := randomHex(16)
	if err != nil {
		t.Fatalf("randomHex failed: %v", err)
	}
	if len(hex) != 32 {
		t.Errorf("expected 32 char hex string, got %d", len(hex))
	}
}

func TestFirstRows(t *testing.T) {
	rows := [][]string{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}}

	result := firstRows(rows, 2)
	if len(result) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result))
	}

	result = firstRows(rows, 10)
	if len(result) != 5 {
		t.Errorf("expected 5 rows, got %d", len(result))
	}
}

func TestJSONHelpers(t *testing.T) {
	result := toJSON([]string{"a", "b"})
	expected := `["a","b"]`
	if result != expected {
		t.Errorf("toJSON() = %q, want %q", result, expected)
	}

	result = toJSON(make(chan int))
	if result != "[]" {
		t.Errorf("toJSON(invalid) = %q, want []", result)
	}
}

func loadTestConfig() config.Config {
	return config.Config{
		JWTSecret:      "test-secret-for-unit-tests-only",
		FrontendOrigin: "http://localhost:5173",
	}
}

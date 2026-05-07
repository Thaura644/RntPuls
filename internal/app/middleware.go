package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type ctxKey string

const (
	userKey      ctxKey = "user"
	requestIDKey ctxKey = "request_id"
)

type authUser struct {
	ID             string
	OrganizationID string
	Email          string
	Role           string
	FullName       string
	TenantID       string
}

type pgxRows interface {
	Next() bool
	Values() ([]any, error)
	FieldDescriptions() []pgconn.FieldDescription
}

func newContext(ctx context.Context, key ctxKey, value any) context.Context {
	return context.WithValue(ctx, key, value)
}

func (a *App) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = generateID(16)
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(newContext(r.Context(), requestIDKey, id)))
	})
}

func (a *App) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		defer func() {
			elapsed := time.Since(start)
			requestID, _ := r.Context().Value(requestIDKey).(string)
			a.logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.statusCode,
				"duration_ms", elapsed.Milliseconds(),
				"remote_addr", r.RemoteAddr,
				"request_id", requestID,
			)
		}()

		next.ServeHTTP(rw, r)
	})
}

func (a *App) panicRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				requestID, _ := r.Context().Value(requestIDKey).(string)
				a.logger.Error("panic recovered",
					"error", err,
					"request_id", requestID,
					"path", r.URL.Path,
				)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isValidOrigin(origin, a.cfg.FrontendOrigin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		token, err := jwt.ParseWithClaims(tokenStr, jwt.MapClaims{}, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(a.cfg.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid token claims")
			return
		}

		u := authUser{
			ID:             toString(claims["sub"]),
			OrganizationID: toString(claims["org"]),
			Email:          toString(claims["email"]),
			Role:           toString(claims["role"]),
			FullName:       toString(claims["name"]),
			TenantID:       toString(claims["tenant_id"]),
		}

		if u.ID == "" || u.OrganizationID == "" {
			writeError(w, http.StatusUnauthorized, "invalid token: missing subject or org")
			return
		}

		if !a.allowRequest(r.Context(), u.OrganizationID) {
			writeError(w, http.StatusTooManyRequests, "plan request limit reached; retry in a minute or upgrade your plan")
			return
		}

		next.ServeHTTP(w, r.WithContext(newContext(r.Context(), userKey, u)))
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

func mustUser(r *http.Request) authUser {
	u, ok := r.Context().Value(userKey).(authUser)
	if !ok {
		return authUser{}
	}
	return u
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.Split(fwd, ",")[0]
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return real
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isValidOrigin(origin string, allowed string) bool {
	return origin == allowed || allowed == "*"
}

func generateID(length int) string {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(buf)
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write json response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func formatMoney(cents int64) string {
	return strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
}

func render(template string, values map[string]string) string {
	for key, value := range values {
		template = strings.ReplaceAll(template, "{{"+key+"}}", value)
	}
	return template
}

func collect(rows pgxRows) []map[string]any {
	fields := rows.FieldDescriptions()
	out := []map[string]any{}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			continue
		}
		item := map[string]any{}
		for i, f := range fields {
			item[f.Name] = values[i]
		}
		out = append(out, item)
	}
	return out
}

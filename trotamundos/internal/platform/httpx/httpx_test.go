package httpx

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDecodeToleratesEOFAndRejectsUnknownFields(t *testing.T) {
	var dst struct {
		A string `json:"a"`
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if err := Decode(req, &dst); err != nil {
		t.Fatalf("body vacío debía ser tolerado: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":"x"}`))
	if err := Decode(req, &dst); err != nil || dst.A != "x" {
		t.Fatalf("Decode válido: %v, %+v", err, dst)
	}

	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":"x","extra":1}`))
	if err := Decode(req, &dst); err == nil {
		t.Fatal("campos desconocidos debían ser rechazados")
	}
}

func TestRecoverDoesNotWrite500AfterHeader(t *testing.T) {
	handler := Recover(discardLogger())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("parcial"))
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, quería 202", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "internal_error") {
		t.Fatalf("no se debía escribir un 500 luego del header: %s", rec.Body.String())
	}

	handler = Recover(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom antes de escribir")
	}))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quería 500", rec.Code)
	}
}

func TestSafeChainPutsRecoverOutermost(t *testing.T) {
	panickingMiddleware := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("middleware roto") })
	}
	handler := SafeChain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), panickingMiddleware)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("SafeChain debió recuperar el panic del middleware, obtuve %d", rec.Code)
	}
}

func TestCORSWildcardAddsVaryAndMaxAge(t *testing.T) {
	handler := CORS("*")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q", got)
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("Vary = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got == "" {
		t.Fatal("falta Access-Control-Max-Age")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-API-Key") {
		t.Fatalf("Allow-Headers debe incluir X-API-Key, obtuve %q", got)
	}
}

func TestConfigureTrustedProxies(t *testing.T) {
	defer ResetTrustedProxies()
	t.Setenv("TRUSTED_PROXIES", "")

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.1.2.3:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	ConfigureTrustedProxies([]string{"10.0.0.0/8"})
	if got := ClientIP(r); got != "198.51.100.7" {
		t.Fatalf("ClientIP = %q", got)
	}
}

func TestClientIPOnlyTrustsConfiguredProxies(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.10:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")

	t.Setenv("TRUSTED_PROXIES", "")
	if got := ClientIP(r); got != "203.0.113.10" {
		t.Fatalf("sin proxies configurados debía usar RemoteAddr, obtuve %q", got)
	}

	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.10")
	r.RemoteAddr = "10.1.2.3:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.9")
	if got := ClientIP(r); got != "198.51.100.7" {
		t.Fatalf("ClientIP = %q", got)
	}

	r.Header.Set("X-Forwarded-For", "falso, 10.0.0.9")
	if got := ClientIP(r); got != "10.1.2.3" {
		t.Fatalf("un XFF inválido no debía ser aceptado, obtuve %q", got)
	}
}

type stubLimiter struct {
	allow  bool
	err    error
	window time.Duration
	keys   []string
}

func (s *stubLimiter) Allow(_ context.Context, key string) (bool, error) {
	s.keys = append(s.keys, key)
	return s.allow, s.err
}

func (s *stubLimiter) Window() time.Duration { return s.window }

func TestRateLimitResponses(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	limiter := &stubLimiter{allow: false, window: 90 * time.Second}
	handler := RateLimit(limiter, func(*http.Request) string { return "cliente-1" }, next)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "rate_limited") {
		t.Fatalf("esperaba 429 rate_limited, obtuve %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") != "90" {
		t.Fatalf("Retry-After = %q", rec.Header().Get("Retry-After"))
	}
	if len(limiter.keys) != 1 || limiter.keys[0] != "cliente-1" || nextCalled {
		t.Fatalf("claves = %q, nextCalled = %v", limiter.keys, nextCalled)
	}

	limiter = &stubLimiter{err: errors.New("redis caído")}
	handler = RateLimit(limiter, nil, next)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("fallo del limitador debe degradar a fail-open, obtuve %d %s", rec.Code, rec.Body.String())
	}

	limiter = &stubLimiter{allow: true}
	handler = RateLimit(limiter, nil, next)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNoContent || !nextCalled {
		t.Fatalf("el request permitido debía pasar, obtuve %d", rec.Code)
	}
}

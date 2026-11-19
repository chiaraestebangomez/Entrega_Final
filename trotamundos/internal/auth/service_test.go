package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/jwtx"
	"trotamundos/internal/platform/password"
)

type memoryUsers struct {
	mu    sync.Mutex
	users map[string]*domain.User
}

func newMemoryUsers() *memoryUsers {
	return &memoryUsers{users: map[string]*domain.User{}}
}

func (m *memoryUsers) Create(_ context.Context, u *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[strings.ToLower(u.Email)]; ok {
		return domain.ErrConflict
	}
	cp := *u
	m.users[strings.ToLower(u.Email)] = &cp
	return nil
}

func (m *memoryUsers) ByEmail(_ context.Context, email string) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[strings.ToLower(email)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *memoryUsers) ByID(_ context.Context, id string) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *memoryUsers) Upsert(_ context.Context, u *domain.User) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *u
	m.users[strings.ToLower(u.Email)] = &cp
	return &cp, nil
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, _, _ := newTestServiceWithDeps(t)
	return svc
}

func newTestServiceWithDeps(t *testing.T) (*Service, *MemoryRefreshStore, *jwtx.Signer) {
	t.Helper()
	priv, _, err := jwtx.Generate()
	if err != nil {
		t.Fatal(err)
	}
	signer := jwtx.NewSigner(priv, "test", 5*time.Minute)
	store := NewMemoryRefreshStore()
	return NewService(newMemoryUsers(), store, password.New(4), signer, time.Hour), store, signer
}

func TestRegisterLoginRefreshLogout(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	user, tokens, err := svc.Register(ctx, "Traveler@Example.com", "Ana Pérez", "secreto123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Email != "traveler@example.com" || user.Role != domain.RoleTraveler {
		t.Fatalf("usuario inesperado: %+v", user)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("tokens vacíos")
	}
	if _, _, err := svc.Register(ctx, "traveler@example.com", "Otra", "secreto123"); err != domain.ErrConflict {
		t.Fatalf("esperaba ErrConflict, obtuve %v", err)
	}
	if _, _, err := svc.Register(ctx, "bad", "Nombre", "secreto123"); err == nil {
		t.Fatal("esperaba error de validación")
	}

	if _, _, err := svc.Login(ctx, "traveler@example.com", "incorrecta"); err == nil {
		t.Fatal("esperaba credenciales inválidas")
	}
	if _, pair, err := svc.Login(ctx, "traveler@example.com", "secreto123"); err != nil || pair.RefreshToken == "" {
		t.Fatalf("Login: %v", err)
	}

	_, refreshed, err := svc.Refresh(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if refreshed.RefreshToken == tokens.RefreshToken {
		t.Fatal("el refresh debe rotar")
	}
	if _, _, err := svc.Refresh(ctx, tokens.RefreshToken); !errorsIsRefresh(err) {
		t.Fatalf("el refresh anterior debe quedar inválido, obtuve %v", err)
	}
	if err := svc.Logout(ctx, refreshed.RefreshToken, refreshed.AccessToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, _, err := svc.Refresh(ctx, refreshed.RefreshToken); !errorsIsRefresh(err) {
		t.Fatalf("el refresh revocado debe fallar, obtuve %v", err)
	}
}

func errorsIsRefresh(err error) bool {
	return errors.Is(err, ErrRefreshNotFound) || errors.Is(err, domain.ErrUnauthorized)
}

type failSaveStore struct {
	RefreshStore
	fail bool
}

func (s *failSaveStore) Rotate(ctx context.Context, oldHash, newHash, userID string, ttl time.Duration) error {
	if s.fail {
		return errors.New("rotate falló")
	}
	return s.RefreshStore.Rotate(ctx, oldHash, newHash, userID, ttl)
}

func TestRefreshSaveFailureKeepsOldTokenValid(t *testing.T) {
	svc, store, signer := newTestServiceWithDeps(t)
	failing := &failSaveStore{RefreshStore: store, fail: false}
	svc = NewService(newMemoryUsers(), failing, password.New(4), signer, time.Hour)
	ctx := context.Background()

	u, err := svc.EnsureUser(ctx, "rotate@example.com", "Rotación", "secreto123", domain.RoleTraveler)
	if err != nil {
		t.Fatal(err)
	}
	oldToken := "refresh-antiguo"
	if err := store.Save(ctx, hashToken(oldToken), u.ID, time.Hour); err != nil {
		t.Fatal(err)
	}

	failing.fail = true
	if _, _, err := svc.Refresh(ctx, oldToken); err == nil {
		t.Fatal("esperaba error de Save durante el rotate")
	}
	if _, err := store.Get(ctx, hashToken(oldToken)); err != nil {
		t.Fatalf("el refresh viejo debe seguir vigente si falla Save: %v", err)
	}

	failing.fail = false
	_, pair, err := svc.Refresh(ctx, oldToken)
	if err != nil {
		t.Fatalf("el rotate debe funcionar después: %v", err)
	}
	if pair.RefreshToken == oldToken {
		t.Fatal("el refresh debe rotar")
	}
	if _, _, err := svc.Refresh(ctx, oldToken); !errorsIsRefresh(err) {
		t.Fatalf("el refresh viejo debe quedar invalidado: %v", err)
	}
}

func TestLogoutRevokesAccessToken(t *testing.T) {
	svc, store, signer := newTestServiceWithDeps(t)
	ctx := context.Background()
	_, tokens, err := svc.Register(ctx, "logout@example.com", "Logout", "secreto123")
	if err != nil {
		t.Fatal(err)
	}

	auth := NewAuthenticatorWithRevocation(signer, store)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	guarded := auth.Require(next)

	check := func(wantStatus int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
		rec := httptest.NewRecorder()
		guarded.ServeHTTP(rec, req)
		if rec.Code != wantStatus {
			t.Fatalf("status = %d, quería %d; body: %s", rec.Code, wantStatus, rec.Body.String())
		}
		return rec
	}

	check(http.StatusNoContent)
	if err := svc.Logout(ctx, tokens.RefreshToken, tokens.AccessToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	rec := check(http.StatusUnauthorized)
	if !strings.Contains(rec.Body.String(), "token_revoked") {
		t.Fatalf("esperaba código token_revoked, obtuve %s", rec.Body.String())
	}
	if _, _, err := svc.Refresh(ctx, tokens.RefreshToken); !errorsIsRefresh(err) {
		t.Fatalf("el refresh debe quedar revocado: %v", err)
	}
}

type failingRevocationStore struct{ err error }

func (s failingRevocationStore) RevokeJTI(context.Context, string, time.Duration) error { return nil }
func (s failingRevocationStore) IsRevokedJTI(context.Context, string) (bool, error) {
	return false, s.err
}

func TestAuthenticatorFailsClosedOnRevocationError(t *testing.T) {
	svc, _, signer := newTestServiceWithDeps(t)
	_, tokens, err := svc.Register(context.Background(), "fail@example.com", "Falla", "secreto123")
	if err != nil {
		t.Fatal(err)
	}
	auth := NewAuthenticatorWithRevocation(signer, failingRevocationStore{err: errors.New("redis caído")})
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	rec := httptest.NewRecorder()
	auth.Require(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("no se debe llamar al handler si la denylist falla")
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "revocation_unavailable") {
		t.Fatalf("esperaba 503 revocation_unavailable, obtuve %d %s", rec.Code, rec.Body.String())
	}
}

func TestLogoutHandlerRejectsInvalidBodyButAllowsEmpty(t *testing.T) {
	h := NewHandler(newTestService(t))

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", strings.NewReader("{json inválido"))
	rec := httptest.NewRecorder()
	h.Logout(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_body") {
		t.Fatalf("esperaba 400 invalid_body, obtuve %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec = httptest.NewRecorder()
	h.Logout(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("body vacío debe ser tolerado, obtuve %d %s", rec.Code, rec.Body.String())
	}
}

func TestConcurrentRefreshOnlyRotatesOnce(t *testing.T) {
	svc, store, _ := newTestServiceWithDeps(t)
	ctx := context.Background()
	u, err := svc.EnsureUser(ctx, "concurrente@example.com", "Concurrente", "secreto123", domain.RoleTraveler)
	if err != nil {
		t.Fatal(err)
	}
	oldToken := "refresh-concurrente"
	if err := store.Save(ctx, hashToken(oldToken), u.ID, time.Hour); err != nil {
		t.Fatal(err)
	}

	const attempts = 8
	errs := make([]error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, errs[i] = svc.Refresh(ctx, oldToken)
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errorsIsRefresh(err):
		default:
			t.Fatalf("error inesperado al rotar en paralelo: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("sólo una rotación debía tener éxito, tuve %d", succeeded)
	}
}

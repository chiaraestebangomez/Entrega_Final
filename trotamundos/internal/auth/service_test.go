package auth

import (
	"context"
	"errors"
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
	priv, _, err := jwtx.Generate()
	if err != nil {
		t.Fatal(err)
	}
	signer := jwtx.NewSigner(priv, "test", 5*time.Minute)
	return NewService(newMemoryUsers(), NewMemoryRefreshStore(), password.New(4), signer, time.Hour)
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
	if err := svc.Logout(ctx, refreshed.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, _, err := svc.Refresh(ctx, refreshed.RefreshToken); !errorsIsRefresh(err) {
		t.Fatalf("el refresh revocado debe fallar, obtuve %v", err)
	}
}

func errorsIsRefresh(err error) bool {
	return errors.Is(err, ErrRefreshNotFound) || errors.Is(err, domain.ErrUnauthorized)
}

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/idgen"
	"trotamundos/internal/platform/jwtx"
	"trotamundos/internal/platform/password"
)

type TokenPair struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int64        `json:"expires_in"`
	User         *domain.User `json:"user"`
}

type Service struct {
	users      UserRepository
	store      RefreshStore
	hasher     password.Hasher
	tokens     *jwtx.Signer
	refreshTTL time.Duration
	now        func() time.Time
}

func NewService(users UserRepository, store RefreshStore, hasher password.Hasher, tokens *jwtx.Signer, refreshTTL time.Duration) *Service {
	return &Service{users: users, store: store, hasher: hasher, tokens: tokens, refreshTTL: refreshTTL, now: time.Now}
}

func (s *Service) Register(ctx context.Context, email, fullName, rawPassword string) (*domain.User, *TokenPair, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	fullName = strings.TrimSpace(fullName)
	if _, err := mail.ParseAddress(email); err != nil || !strings.Contains(email, "@") {
		return nil, nil, fmt.Errorf("%w: email inválido", domain.ErrValidation)
	}
	if len(fullName) < 2 {
		return nil, nil, fmt.Errorf("%w: el nombre es obligatorio", domain.ErrValidation)
	}
	if len(rawPassword) < 8 {
		return nil, nil, fmt.Errorf("%w: la contraseña debe tener al menos 8 caracteres", domain.ErrValidation)
	}
	if len(rawPassword) > 72 {
		return nil, nil, fmt.Errorf("%w: la contraseña no puede superar 72 caracteres", domain.ErrValidation)
	}
	hash, err := s.hasher.Hash(rawPassword)
	if err != nil {
		return nil, nil, err
	}
	now := s.now().UTC()
	u := &domain.User{
		ID:           idgen.UUID(),
		Email:        email,
		FullName:     fullName,
		PasswordHash: hash,
		Role:         domain.RoleTraveler,
		Locale:       "es",
		Region:       "AR",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, nil, err
	}
	pair, err := s.issue(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	return u, pair, nil
}

func (s *Service) Login(ctx context.Context, email, rawPassword string) (*domain.User, *TokenPair, error) {
	u, err := s.users.ByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil, domain.ErrUnauthorized
		}
		return nil, nil, err
	}
	if err := s.hasher.Verify(u.PasswordHash, rawPassword); err != nil {
		return nil, nil, domain.ErrUnauthorized
	}
	pair, err := s.issue(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	return u, pair, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*domain.User, *TokenPair, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, nil, domain.ErrUnauthorized
	}
	key := hashToken(refreshToken)
	userID, err := s.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrRefreshNotFound) {
			return nil, nil, domain.ErrUnauthorized
		}
		return nil, nil, err
	}
	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil, domain.ErrUnauthorized
		}
		return nil, nil, err
	}
	if err := s.store.Delete(ctx, key); err != nil {
		return nil, nil, err
	}
	pair, err := s.issue(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	return u, pair, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil
	}
	return s.store.Delete(ctx, hashToken(refreshToken))
}

func (s *Service) ByID(ctx context.Context, id string) (*domain.User, error) {
	return s.users.ByID(ctx, id)
}

func (s *Service) EnsureUser(ctx context.Context, email, fullName, rawPassword, role string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if existing, err := s.users.ByEmail(ctx, email); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	hash, err := s.hasher.Hash(rawPassword)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	u := &domain.User{
		ID: idgen.UUID(), Email: email, FullName: fullName, PasswordHash: hash,
		Role: role, Locale: "es", Region: "AR", CreatedAt: now, UpdatedAt: now,
	}
	return s.users.Upsert(ctx, u)
}

func (s *Service) issue(ctx context.Context, u *domain.User) (*TokenPair, error) {
	now := s.now().UTC()
	access, _, err := s.tokens.Issue(u.ID, u.Email, u.Role, now)
	if err != nil {
		return nil, err
	}
	refresh := idgen.Token(32)
	if err := s.store.Save(ctx, hashToken(refresh), u.ID, s.refreshTTL); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.tokens.TTL().Seconds()),
		User:         u,
	}, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

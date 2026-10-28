package auth

import (
	"context"
	"errors"
	"sync"
	"time"

	"trotamundos/internal/platform/redisx"
)

type RefreshStore interface {
	Save(ctx context.Context, tokenHash, userID string, ttl time.Duration) error
	Get(ctx context.Context, tokenHash string) (string, error)
	Delete(ctx context.Context, tokenHash string) error
}

var ErrRefreshNotFound = errors.New("refresh token not found")

type RedisRefreshStore struct {
	client *redisx.Client
	prefix string
}

func NewRedisRefreshStore(client *redisx.Client) *RedisRefreshStore {
	return &RedisRefreshStore{client: client, prefix: "auth:refresh:"}
}

func (s *RedisRefreshStore) Save(ctx context.Context, tokenHash, userID string, ttl time.Duration) error {
	return s.client.Set(ctx, s.prefix+tokenHash, userID, ttl)
}

func (s *RedisRefreshStore) Get(ctx context.Context, tokenHash string) (string, error) {
	v, err := s.client.Get(ctx, s.prefix+tokenHash)
	if err != nil {
		if errors.Is(err, redisx.ErrNil) {
			return "", ErrRefreshNotFound
		}
		return "", err
	}
	return v, nil
}

func (s *RedisRefreshStore) Delete(ctx context.Context, tokenHash string) error {
	return s.client.Delete(ctx, s.prefix+tokenHash)
}

type memEntry struct {
	userID    string
	expiresAt time.Time
}

type MemoryRefreshStore struct {
	mu   sync.Mutex
	data map[string]memEntry
	now  func() time.Time
}

func NewMemoryRefreshStore() *MemoryRefreshStore {
	return &MemoryRefreshStore{data: map[string]memEntry{}, now: time.Now}
}

func (s *MemoryRefreshStore) Save(_ context.Context, tokenHash, userID string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[tokenHash] = memEntry{userID: userID, expiresAt: s.now().Add(ttl)}
	return nil
}

func (s *MemoryRefreshStore) Get(_ context.Context, tokenHash string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[tokenHash]
	if !ok || s.now().After(e.expiresAt) {
		delete(s.data, tokenHash)
		return "", ErrRefreshNotFound
	}
	return e.userID, nil
}

func (s *MemoryRefreshStore) Delete(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, tokenHash)
	return nil
}

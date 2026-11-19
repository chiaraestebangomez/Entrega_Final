package auth

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"trotamundos/internal/platform/redisx"
)

type RefreshStore interface {
	Save(ctx context.Context, tokenHash, userID string, ttl time.Duration) error
	Get(ctx context.Context, tokenHash string) (string, error)
	Delete(ctx context.Context, tokenHash string) error
	// Rotate consume el refresh viejo y persiste el nuevo en una sola operación
	// atómica. Devuelve ErrRefreshNotFound si el viejo ya fue consumido (reuso).
	Rotate(ctx context.Context, oldHash, newHash, userID string, ttl time.Duration) error
}

// TokenRevocationStore es la denylist de JTI exigida por ADR-4.1. Las entradas
// se guardan sólo hasta el vencimiento del access token correspondiente.
type TokenRevocationStore interface {
	RevokeJTI(ctx context.Context, jti string, ttl time.Duration) error
	IsRevokedJTI(ctx context.Context, jti string) (bool, error)
}

var (
	_ RefreshStore         = (*RedisRefreshStore)(nil)
	_ TokenRevocationStore = (*RedisRefreshStore)(nil)
	_ RefreshStore         = (*MemoryRefreshStore)(nil)
	_ TokenRevocationStore = (*MemoryRefreshStore)(nil)
)

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

// rotateRefreshScript comprueba que el refresh viejo siga vigente, persiste el
// nuevo y recién después borra el viejo. Si el SET falla, el viejo queda intacto;
// si dos peticiones rotan el mismo token en paralelo, sólo la primera logra
// borrarlo y la segunda recibe ErrRefreshNotFound (detección de reuso).
const rotateRefreshScript = `
if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
redis.call('SET', KEYS[2], ARGV[1], 'PX', ARGV[2])
redis.call('DEL', KEYS[1])
return 1
`

func (s *RedisRefreshStore) Rotate(ctx context.Context, oldHash, newHash, userID string, ttl time.Duration) error {
	v, err := s.client.Eval(ctx, rotateRefreshScript, 2,
		s.prefix+oldHash, s.prefix+newHash, userID, strconv.FormatInt(ttl.Milliseconds(), 10))
	if err != nil {
		return err
	}
	n, ok := v.(int64)
	if !ok || n != 1 {
		return ErrRefreshNotFound
	}
	return nil
}

func (s *RedisRefreshStore) RevokeJTI(ctx context.Context, jti string, ttl time.Duration) error {
	if jti == "" {
		return nil
	}
	if ttl <= 0 {
		// El access token ya venció: no tiene sentido mantener la entrada.
		return s.client.Delete(ctx, "auth:jti:"+jti)
	}
	return s.client.Set(ctx, "auth:jti:"+jti, "1", ttl)
}

func (s *RedisRefreshStore) IsRevokedJTI(ctx context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}
	_, err := s.client.Get(ctx, "auth:jti:"+jti)
	if err != nil {
		if errors.Is(err, redisx.ErrNil) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

type memEntry struct {
	userID    string
	expiresAt time.Time
}

type MemoryRefreshStore struct {
	mu   sync.Mutex
	data map[string]memEntry
	jti  map[string]time.Time
	now  func() time.Time
}

func NewMemoryRefreshStore() *MemoryRefreshStore {
	return &MemoryRefreshStore{data: map[string]memEntry{}, jti: map[string]time.Time{}, now: time.Now}
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

func (s *MemoryRefreshStore) Rotate(_ context.Context, oldHash, newHash, userID string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[oldHash]
	if !ok || s.now().After(e.expiresAt) {
		delete(s.data, oldHash)
		return ErrRefreshNotFound
	}
	// Persistimos el nuevo antes de borrar el viejo, igual que el script Lua.
	s.data[newHash] = memEntry{userID: userID, expiresAt: s.now().Add(ttl)}
	delete(s.data, oldHash)
	return nil
}

func (s *MemoryRefreshStore) RevokeJTI(_ context.Context, jti string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jti == nil {
		s.jti = map[string]time.Time{}
	}
	if jti == "" {
		return nil
	}
	if ttl <= 0 {
		delete(s.jti, jti)
		return nil
	}
	s.jti[jti] = s.now().Add(ttl)
	return nil
}

func (s *MemoryRefreshStore) IsRevokedJTI(_ context.Context, jti string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if jti == "" || s.jti == nil {
		return false, nil
	}
	expiresAt, ok := s.jti[jti]
	if !ok {
		return false, nil
	}
	if !s.now().Before(expiresAt) {
		delete(s.jti, jti)
		return false, nil
	}
	return true, nil
}

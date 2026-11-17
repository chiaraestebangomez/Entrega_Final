package recommendation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"trotamundos/internal/platform/idgen"
	"trotamundos/internal/platform/redisx"
)

var ErrCacheMiss = errors.New("recommendation cache miss")

const releaseLockScript = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`

type Cache interface {
	Get(ctx context.Context, key string, dst any) error
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	AcquireLock(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key, token string) error
}

type RedisCache struct {
	client *redisx.Client
}

func NewRedisCache(client *redisx.Client) *RedisCache {
	return &RedisCache{client: client}
}

func (c *RedisCache) Get(ctx context.Context, key string, dst any) error {
	raw, err := c.client.Get(ctx, key)
	if errors.Is(err, redisx.ErrNil) {
		return ErrCacheMiss
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		return fmt.Errorf("decode cache %s: %w", key, err)
	}
	return nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, string(raw), ttl)
}

func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	return c.client.Delete(ctx, keys...)
}

func (c *RedisCache) AcquireLock(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		ttl = time.Minute
	}
	reply, err := c.client.Do(ctx, "SET", key, token, "NX", "PX", ttl.Milliseconds())
	if err != nil {
		return false, err
	}
	if reply == nil {
		return false, nil
	}
	s, ok := reply.(string)
	return ok && s == "OK", nil
}

func (c *RedisCache) ReleaseLock(ctx context.Context, key, token string) error {
	_, err := c.client.Do(ctx, "EVAL", releaseLockScript, 1, key, token)
	return err
}

type MemoryCache struct {
	mu     sync.Mutex
	values map[string][]byte
	locks  map[string]lockEntry
}

type lockEntry struct {
	token   string
	expires time.Time
}

func NewMemoryCache() *MemoryCache {
	return &MemoryCache{values: map[string][]byte{}, locks: map[string]lockEntry{}}
}

func (c *MemoryCache) Get(_ context.Context, key string, dst any) error {
	c.mu.Lock()
	raw, ok := c.values[key]
	c.mu.Unlock()
	if !ok {
		return ErrCacheMiss
	}
	return json.Unmarshal(raw, dst)
}

func (c *MemoryCache) Set(_ context.Context, key string, value any, _ time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.values[key] = raw
	c.mu.Unlock()
	return nil
}

func (c *MemoryCache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		delete(c.values, key)
	}
	return nil
}

func (c *MemoryCache) AcquireLock(_ context.Context, key, token string, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.locks[key]
	if ok && entry.expires.After(time.Now()) {
		return false, nil
	}
	if c.locks == nil {
		c.locks = map[string]lockEntry{}
	}
	c.locks[key] = lockEntry{token: token, expires: time.Now().Add(ttl)}
	return true, nil
}

func (c *MemoryCache) ReleaseLock(_ context.Context, key, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.locks[key]; ok && entry.token == token {
		delete(c.locks, key)
	}
	return nil
}

func newLockToken() string { return idgen.UUID() }

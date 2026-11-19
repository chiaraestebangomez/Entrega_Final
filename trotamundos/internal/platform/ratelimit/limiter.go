// Package ratelimit provee primitivas de limitación de tasa para el API.
package ratelimit

import (
	"context"
	"errors"
	"time"

	"trotamundos/internal/platform/redisx"
)

type Limiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

// incrAndExpireScript hace INCR y, sólo en la primera llave de la ventana,
// PEXPIRE. Al ejecutarse con EVAL todo el bloque es atómico en Redis.
const incrAndExpireScript = `local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return current`

type RedisLimiter struct {
	client *redisx.Client
	limit  int
	window time.Duration
	prefix string
}

func NewRedisLimiter(client *redisx.Client, limit int, window time.Duration) *RedisLimiter {
	return &RedisLimiter{client: client, limit: limit, window: window, prefix: "ratelimit:"}
}

func (l *RedisLimiter) Allow(ctx context.Context, key string) (bool, error) {
	if l == nil || l.client == nil {
		return false, errors.New("ratelimit: nil redis client")
	}
	if l.limit <= 0 || l.window <= 0 {
		// Configuración inválida: fail closed.
		return false, nil
	}
	windowMS := l.window.Milliseconds()
	if windowMS < 1 {
		windowMS = 1
	}

	reply, err := l.client.Do(ctx, "EVAL", incrAndExpireScript, 1, l.prefix+key, l.limit, windowMS)
	if err != nil {
		return false, err
	}
	current, ok := reply.(int64)
	if !ok {
		return false, errors.New("ratelimit: unexpected redis reply")
	}
	return current <= int64(l.limit), nil
}

// Window expone la ventana para que el middleware HTTP pueda calcular
// Retry-After.
func (l *RedisLimiter) Window() time.Duration {
	if l == nil {
		return 0
	}
	return l.window
}

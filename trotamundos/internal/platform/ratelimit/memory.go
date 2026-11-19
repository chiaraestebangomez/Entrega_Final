package ratelimit

import (
	"context"
	"sync"
	"time"
)

type memoryWindow struct {
	count   int
	resetAt time.Time
}

// MemoryLimiter es una implementación en proceso, útil para tests o
// despliegues de una sola réplica.
type MemoryLimiter struct {
	mu     sync.Mutex
	data   map[string]memoryWindow
	limit  int
	window time.Duration
	now    func() time.Time
}

func NewMemoryLimiter(limit int, window time.Duration) *MemoryLimiter {
	return &MemoryLimiter{
		data:   map[string]memoryWindow{},
		limit:  limit,
		window: window,
		now:    time.Now,
	}
}

func (m *MemoryLimiter) Allow(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if m.limit <= 0 || m.window <= 0 {
		return false, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	w, ok := m.data[key]
	if !ok || !now.Before(w.resetAt) {
		w = memoryWindow{resetAt: now.Add(m.window)}
	}
	if w.count >= m.limit {
		m.data[key] = w
		return false, nil
	}
	w.count++
	m.data[key] = w
	return true, nil
}

func (m *MemoryLimiter) Window() time.Duration { return m.window }

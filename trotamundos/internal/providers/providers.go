package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"trotamundos/internal/platform/breaker"
)

var (
	ErrNotFound    = errors.New("provider reservation not found")
	ErrUnavailable = errors.New("provider unavailable")
	ErrOpen        = errors.New("provider circuit open")
)

type Snapshot struct {
	Type        string         `json:"type"`
	Provider    string         `json:"provider"`
	Locator     string         `json:"locator"`
	Title       string         `json:"title"`
	Destination string         `json:"destination"`
	StartAt     *time.Time     `json:"start_at,omitempty"`
	EndAt       *time.Time     `json:"end_at,omitempty"`
	Status      string         `json:"status"`
	Details     map[string]any `json:"details,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type Adapter interface {
	Name() string
	GetReservation(ctx context.Context, locator string) (Snapshot, error)
}

type MockStore struct {
	mu        sync.RWMutex
	snapshots map[string]Snapshot
	failures  map[string]bool
}

func NewMockStore() *MockStore {
	return &MockStore{snapshots: map[string]Snapshot{}, failures: map[string]bool{}}
}

func key(provider, locator string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "|" + strings.TrimSpace(locator)
}

func (s *MockStore) Seed(provider, locator string, snap Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(provider, locator)
	if _, exists := s.snapshots[k]; exists {
		return
	}
	s.snapshots[k] = snap
}

func (s *MockStore) Set(provider, locator string, snap Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[key(provider, locator)] = snap
}

func (s *MockStore) Get(provider, locator string) (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap, ok := s.snapshots[key(provider, locator)]
	return snap, ok
}

func (s *MockStore) Cancel(provider, locator string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(provider, locator)
	snap, ok := s.snapshots[k]
	if !ok {
		return false
	}
	snap.Status = "canceled"
	snap.UpdatedAt = time.Now().UTC()
	s.snapshots[k] = snap
	return true
}

func (s *MockStore) SetFailure(provider string, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures[strings.ToLower(strings.TrimSpace(provider))] = enabled
}

func (s *MockStore) failing(provider string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.failures[strings.ToLower(strings.TrimSpace(provider))]
}

type mockAdapter struct {
	name  string
	store *MockStore
}

func (a *mockAdapter) Name() string { return a.name }

func (a *mockAdapter) GetReservation(_ context.Context, locator string) (Snapshot, error) {
	if a.store.failing(a.name) {
		return Snapshot{}, ErrUnavailable
	}
	if snap, ok := a.store.Get(a.name, locator); ok {
		return snap, nil
	}
	return Snapshot{}, ErrNotFound
}

type Registry struct {
	mu       sync.RWMutex
	store    *MockStore
	adapters map[string]Adapter
	now      func() time.Time
}

func NewMockRegistry(store *MockStore) *Registry {
	r := &Registry{store: store, adapters: map[string]Adapter{}, now: time.Now}
	for _, name := range []string{"flight", "hotel", "car", "other"} {
		r.adapters[name] = newResilient(name, &mockAdapter{name: name, store: store})
	}
	return r
}

func (r *Registry) Get(provider string) (Adapter, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	r.mu.RLock()
	a, ok := r.adapters[provider]
	r.mu.RUnlock()
	if ok {
		return a, true
	}
	if provider == "" || provider == "manual" {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.adapters[provider]; ok {
		return a, true
	}
	a = newResilient(provider, &mockAdapter{name: provider, store: r.store})
	r.adapters[provider] = a
	return a, true
}

func (r *Registry) Seed(provider, locator string, snap Snapshot) {
	if provider == "" || provider == "manual" || locator == "" {
		return
	}
	if snap.Provider == "" {
		snap.Provider = provider
	}
	if snap.Locator == "" {
		snap.Locator = locator
	}
	if snap.UpdatedAt.IsZero() {
		snap.UpdatedAt = r.now().UTC()
	}
	r.store.Seed(provider, locator, snap)
}

func (r *Registry) SetSnapshot(provider, locator string, snap Snapshot) {
	r.store.Set(provider, locator, snap)
}

func (r *Registry) Cancel(provider, locator string) bool { return r.store.Cancel(provider, locator) }

func (r *Registry) SetFailure(provider string, enabled bool) { r.store.SetFailure(provider, enabled) }

func (r *Registry) Cached(provider, locator string) (Snapshot, bool) {
	return r.store.Get(provider, locator)
}

type resilientAdapter struct {
	inner   Adapter
	breaker *breaker.Breaker
	sleep   func(context.Context, time.Duration)
}

func newResilient(name string, inner Adapter) *resilientAdapter {
	return &resilientAdapter{
		inner:   inner,
		breaker: breaker.New(3, 15*time.Second),
		sleep: func(ctx context.Context, d time.Duration) {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
			case <-t.C:
			}
		},
	}
}

func (a *resilientAdapter) Name() string { return a.inner.Name() }

func (a *resilientAdapter) GetReservation(ctx context.Context, locator string) (Snapshot, error) {
	if err := a.breaker.Allow(); err != nil {
		return Snapshot{}, ErrOpen
	}
	backoff := 100 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			a.sleep(ctx, backoff)
			backoff *= 3
			if ctx.Err() != nil {
				return Snapshot{}, ctx.Err()
			}
		}
		snap, err := a.inner.GetReservation(ctx, locator)
		if err == nil {
			a.breaker.Success()
			return snap, nil
		}
		if errors.Is(err, ErrNotFound) {
			a.breaker.Success()
			return Snapshot{}, err
		}
		lastErr = err
	}
	a.breaker.Failure()
	return Snapshot{}, fmt.Errorf("%w: %v", ErrUnavailable, lastErr)
}

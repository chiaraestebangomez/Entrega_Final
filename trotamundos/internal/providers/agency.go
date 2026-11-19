package providers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/breaker"
	"trotamundos/internal/platform/httpx"
)

var (
	ErrAgencyUnavailable = errors.New("agency unavailable")
	ErrAgencyOpen        = errors.New("agency circuit open")
	ErrAgencyNotFound    = errors.New("agency not found")

	// El circuito abierto de catálogo se distingue del de entrega: el primero
	// equivale a "sin datos frescos" y no debe confundirse con un fallo de envío.
	catalogOpenError = errors.New("agency catalog circuit open")
)

type AgencyCatalogStore interface {
	Catalog(ctx context.Context, agencyID string) ([]domain.AgencyCatalogItem, error)
}

type AgencyAdapter interface {
	Name() string
	Catalog(ctx context.Context, agencyID string) (domain.AgencyCatalogResult, error)
	SendRequest(ctx context.Context, req domain.AgencyRequest) (domain.AgencyDelivery, error)
}

func (s *MockStore) SetAgencyFailure(agencyID string, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agencyFailures == nil {
		s.agencyFailures = map[string]bool{}
	}
	s.agencyFailures[strings.TrimSpace(agencyID)] = enabled
}

func (s *MockStore) agencyFailing(agencyID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agencyFailures[strings.TrimSpace(agencyID)]
}

type agencyAdapter struct {
	name    string
	store   *MockStore
	catalog AgencyCatalogStore
}

func (a *agencyAdapter) Name() string { return a.name }

func (a *agencyAdapter) Catalog(ctx context.Context, agencyID string) (domain.AgencyCatalogResult, error) {
	if a.store.agencyFailing(agencyID) {
		return domain.AgencyCatalogResult{}, ErrAgencyUnavailable
	}
	items := []domain.AgencyCatalogItem{}
	if a.catalog != nil {
		var err error
		items, err = a.catalog.Catalog(ctx, agencyID)
		if err != nil {
			return domain.AgencyCatalogResult{}, err
		}
	}
	return domain.AgencyCatalogResult{Items: items, FetchedAt: time.Now().UTC()}, nil
}

func (a *agencyAdapter) SendRequest(_ context.Context, req domain.AgencyRequest) (domain.AgencyDelivery, error) {
	if a.store.agencyFailing(req.AgencyID) {
		return domain.AgencyDelivery{}, ErrAgencyUnavailable
	}
	return domain.AgencyDelivery{
		RequestID: req.ID,
		AgencyID:  req.AgencyID,
		Status:    "delivered",
		SentAt:    time.Now().UTC(),
	}, nil
}

type resilientAgencyAdapter struct {
	inner AgencyAdapter
	// Un circuit breaker por operación: una falla de entrega no debe servir
	// catálogo stale ni un fallo de catálogo debe bloquear las entregas.
	catalogBreaker  *breaker.Breaker
	deliveryBreaker *breaker.Breaker
	mu              sync.RWMutex
	last            map[string]domain.AgencyCatalogResult
}

func newResilientAgency(inner AgencyAdapter) *resilientAgencyAdapter {
	return &resilientAgencyAdapter{
		inner:           inner,
		catalogBreaker:  breaker.New(3, 15*time.Second),
		deliveryBreaker: breaker.New(3, 15*time.Second),
		last:            map[string]domain.AgencyCatalogResult{},
	}
}

func (a *resilientAgencyAdapter) Name() string { return a.inner.Name() }

func (a *resilientAgencyAdapter) Catalog(ctx context.Context, agencyID string) (domain.AgencyCatalogResult, error) {
	if err := a.catalogBreaker.Allow(); err != nil {
		// Errores distintos permiten distinguir "sin snapshot" de "circuito abierto".
		// Usamos un error propio para no confundir a los callers con fallos de entrega.
		if stale, ok := a.cached(agencyID); ok {
			return stale, nil
		}
		return domain.AgencyCatalogResult{}, catalogOpenError
	}
	result, err := a.inner.Catalog(ctx, agencyID)
	if err != nil {
		a.catalogBreaker.Failure()
		if stale, ok := a.cached(agencyID); ok {
			return stale, nil
		}
		return domain.AgencyCatalogResult{}, err
	}
	a.catalogBreaker.Success()
	a.remember(agencyID, result)
	return result, nil
}

func (a *resilientAgencyAdapter) SendRequest(ctx context.Context, req domain.AgencyRequest) (domain.AgencyDelivery, error) {
	if err := a.deliveryBreaker.Allow(); err != nil {
		return domain.AgencyDelivery{}, ErrAgencyOpen
	}
	delivery, err := a.inner.SendRequest(ctx, req)
	if err != nil {
		a.deliveryBreaker.Failure()
		return domain.AgencyDelivery{}, err
	}
	a.deliveryBreaker.Success()
	return delivery, nil
}

func (a *resilientAgencyAdapter) cached(agencyID string) (domain.AgencyCatalogResult, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result, ok := a.last[agencyID]
	if !ok {
		return result, false
	}
	result.Stale = true
	return result, true
}

func (a *resilientAgencyAdapter) remember(agencyID string, result domain.AgencyCatalogResult) {
	if result.FetchedAt.IsZero() {
		result.FetchedAt = time.Now().UTC()
	}
	items := make([]domain.AgencyCatalogItem, len(result.Items))
	copy(items, result.Items)
	result.Items = items
	a.mu.Lock()
	a.last[agencyID] = result
	a.mu.Unlock()
}

func (r *Registry) SetAgencyCatalog(store AgencyCatalogStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agencyCatalog = store
	r.agencies = map[string]AgencyAdapter{}
}

func (r *Registry) Agency(agencyID string) AgencyAdapter {
	id := strings.TrimSpace(agencyID)
	r.mu.RLock()
	adapter, ok := r.agencies[id]
	catalog := r.agencyCatalog
	r.mu.RUnlock()
	if ok {
		return adapter
	}
	if id == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if adapter, ok = r.agencies[id]; ok {
		return adapter
	}
	adapter = newResilientAgency(&agencyAdapter{name: id, store: r.store, catalog: catalog})
	r.agencies[id] = adapter
	return adapter
}

func (h *Handler) AgencyFail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	h.store.SetAgencyFailure(r.PathValue("id"), body.Enabled)
	httpx.JSON(w, http.StatusOK, map[string]any{"agency_id": r.PathValue("id"), "failing": body.Enabled})
}

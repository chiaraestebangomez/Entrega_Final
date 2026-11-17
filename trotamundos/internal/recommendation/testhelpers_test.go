package recommendation

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/providers"
)

type fakeStore struct {
	prefs          *domain.RecommendationPreferences
	discarded      map[string]bool
	interested     map[string]bool
	interactions   []string
	signals        []domain.AnonymizedSignal
	globalSignals  []domain.AnonymizedSignal
	deliveries     map[string]domain.AgencyDelivery
	preferencesSet int
}

func newFakeStore() *fakeStore {
	return &fakeStore{discarded: map[string]bool{}, interested: map[string]bool{}, deliveries: map[string]domain.AgencyDelivery{}}
}

func (s *fakeStore) GetPreferences(context.Context, string) (*domain.RecommendationPreferences, error) {
	return s.prefs, nil
}
func (s *fakeStore) UpsertPreferences(_ context.Context, p domain.RecommendationPreferences) error {
	copy := p
	s.prefs = &copy
	s.preferencesSet++
	return nil
}
func (s *fakeStore) DiscardedIDs(context.Context, string) (map[string]bool, error) {
	return s.discarded, nil
}
func (s *fakeStore) InterestedIDs(context.Context, string) (map[string]bool, error) {
	return s.interested, nil
}
func (s *fakeStore) RecordInteraction(_ context.Context, _, recommendationID, action string, _ any) error {
	s.interactions = append(s.interactions, recommendationID+":"+action)
	if action == domain.FeedbackDiscarded {
		s.discarded[recommendationID] = true
	}
	if action == domain.FeedbackInterested {
		s.interested[recommendationID] = true
	}
	return nil
}
func (s *fakeStore) WriteAnonymizedSignals(_ context.Context, signals []domain.AnonymizedSignal) error {
	s.signals = append(s.signals, signals...)
	return nil
}
func (s *fakeStore) GlobalSignals(context.Context, time.Time, int) ([]domain.AnonymizedSignal, error) {
	return s.globalSignals, nil
}
func (s *fakeStore) RecordDelivery(_ context.Context, d domain.AgencyDelivery) error {
	s.deliveries[d.RequestID] = d
	return nil
}
func (s *fakeStore) GetDelivery(_ context.Context, requestID string) (*domain.AgencyDelivery, error) {
	d, ok := s.deliveries[requestID]
	if !ok {
		return nil, nil
	}
	return &d, nil
}

type fakeAgencies struct {
	favorite  *domain.FavoriteAgency
	agencies  []domain.Agency
	recs      map[string]*domain.Recommendation
	requests  map[string]*domain.AgencyRequest
	requestID int
}

func newFakeAgencies() *fakeAgencies {
	return &fakeAgencies{
		favorite: &domain.FavoriteAgency{AgencyID: "ag1", Agency: &domain.Agency{ID: "ag1", Name: "Horizonte"}},
		agencies: []domain.Agency{{ID: "ag1", Name: "Horizonte"}},
		recs:     map[string]*domain.Recommendation{},
		requests: map[string]*domain.AgencyRequest{},
	}
}

func (f *fakeAgencies) ListAgencies(context.Context) ([]domain.Agency, error) { return f.agencies, nil }
func (f *fakeAgencies) Favorite(context.Context, string) (*domain.FavoriteAgency, error) {
	return f.favorite, nil
}
func (f *fakeAgencies) SetFavorite(_ context.Context, _, agencyID string) (*domain.FavoriteAgency, error) {
	f.favorite = &domain.FavoriteAgency{AgencyID: agencyID}
	return f.favorite, nil
}
func (f *fakeAgencies) DeleteFavorite(context.Context, string, bool) error {
	f.favorite = nil
	return nil
}
func (f *fakeAgencies) GetRecommendation(_ context.Context, id string) (*domain.Recommendation, error) {
	rec, ok := f.recs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return rec, nil
}
func (f *fakeAgencies) SaveRecommendation(_ context.Context, rec domain.Recommendation) error {
	copyRec := rec
	f.recs[rec.ID] = &copyRec
	return nil
}
func (f *fakeAgencies) Inquire(_ context.Context, userID, recommendationID, message string) (*domain.AgencyRequest, error) {
	rec := f.recs[recommendationID]
	f.requestID++
	id := "req" + string(rune('0'+f.requestID))
	req := &domain.AgencyRequest{ID: id, UserID: userID, AgencyID: rec.AgencyID, RecommendationID: recommendationID, Message: message, Status: domain.RequestOpen}
	f.requests[id] = req
	return req, nil
}
func (f *fakeAgencies) ContactAboutReservation(_ context.Context, userID, reservationID, message string) (*domain.AgencyRequest, error) {
	f.requestID++
	id := "req" + string(rune('0'+f.requestID))
	req := &domain.AgencyRequest{ID: id, UserID: userID, AgencyID: "ag1", ReservationID: reservationID, Message: message, Status: domain.RequestOpen}
	f.requests[id] = req
	return req, nil
}
func (f *fakeAgencies) Request(_ context.Context, _, id string) (*domain.AgencyRequest, error) {
	req, ok := f.requests[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return req, nil
}
func (f *fakeAgencies) Requests(context.Context, string) ([]domain.AgencyRequest, error) {
	out := []domain.AgencyRequest{}
	for _, req := range f.requests {
		out = append(out, *req)
	}
	return out, nil
}
func (f *fakeAgencies) CloseRequest(context.Context, string, string) error { return nil }

type fakeHistory struct{}

func (fakeHistory) ListTrips(context.Context, string, bool) ([]domain.Trip, error) {
	return []domain.Trip{}, nil
}

type fakeCatalog struct{ items []domain.AgencyCatalogItem }

func (f fakeCatalog) Catalog(context.Context, string) ([]domain.AgencyCatalogItem, error) {
	return f.items, nil
}

type failingCache struct{}

func (failingCache) Get(context.Context, string, any) error { return errors.New("redis down") }
func (failingCache) Set(context.Context, string, any, time.Duration) error {
	return errors.New("redis down")
}
func (failingCache) Delete(context.Context, ...string) error { return errors.New("redis down") }
func (failingCache) AcquireLock(context.Context, string, string, time.Duration) (bool, error) {
	return false, errors.New("redis down")
}
func (failingCache) ReleaseLock(context.Context, string, string) error {
	return errors.New("redis down")
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testService(t *testing.T, store Store, cache Cache, agencies AgencyService, catalog *fakeCatalog) *Service {
	t.Helper()
	mockStore := providers.NewMockStore()
	registry := providers.NewMockRegistry(mockStore)
	registry.SetAgencyCatalog(catalog)
	return NewService(store, cache, agencies, fakeHistory{}, registry, testLogger(), DefaultOptions())
}

func catalogItems() []domain.AgencyCatalogItem {
	return []domain.AgencyCatalogItem{
		{ID: "r1", AgencyID: "ag1", AgencyName: "Horizonte", Title: "Patagonia", Destination: "Bariloche", Type: domain.RecoTypeRoute, PriceCents: 100000, Currency: "USD"},
		{ID: "r2", AgencyID: "ag1", AgencyName: "Horizonte", Title: "Hotel centro", Destination: "Mendoza", Type: domain.RecoTypeLodging, PriceCents: 80000, Currency: "USD"},
		{ID: "r3", AgencyID: "ag1", AgencyName: "Horizonte", Title: "Vuelo norte", Destination: "Bariloche", Type: domain.RecoTypeTransport, PriceCents: 90000, Currency: "USD"},
	}
}

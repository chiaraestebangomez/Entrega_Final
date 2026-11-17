package recommendation

import (
	"context"
	"strings"
	"testing"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/providers"
)

func TestRankExcludesDiscardedAndOverBudget(t *testing.T) {
	candidates := FromCatalog(catalogItems())
	model := TrainModel([]domain.AnonymizedSignal{
		{Destination: "Bariloche", RecommendationType: domain.RecoTypeRoute, Action: domain.FeedbackInterested},
	}, time.Now())
	prefs := &domain.RecommendationPreferences{MaxBudgetCents: 95000}
	got := Rank(candidates, model, prefs, nil, map[string]bool{"r2": true}, map[string]bool{})
	if len(got) != 1 || got[0].ID != "r3" {
		t.Fatalf("rank = %+v", got)
	}
	if got[0].Score <= 0 || !strings.Contains(got[0].Reason, "afinidad") {
		t.Fatalf("score/reason = %+v", got[0])
	}
}

func TestRankPrioritizesInterestedAndHistory(t *testing.T) {
	candidates := FromCatalog(catalogItems())
	model := domain.ModelSnapshot{Affinities: map[string]float64{}}
	history := []domain.Trip{{Destination: "Mendoza", Reservations: []domain.Reservation{{Type: domain.RecoTypeLodging}}}}
	got := Rank(candidates, model, nil, history, map[string]bool{}, map[string]bool{"r1": true})
	if len(got) != 3 || got[0].ID != "r1" {
		t.Fatalf("rank = %+v", got)
	}
	if !got[0].Interested {
		t.Fatal("interés no reflejado")
	}
}

func TestFeedbackRefreshesAndAnonymizesSignal(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	agencies := newFakeAgencies()
	for _, item := range catalogItems() {
		rec := FromCatalog([]domain.AgencyCatalogItem{item})[0]
		agencies.recs[rec.ID] = &rec
	}
	cache := NewMemoryCache()
	svc := testService(t, store, cache, agencies, &fakeCatalog{items: catalogItems()})
	if _, err := svc.RunBatch(ctx); err != nil {
		t.Fatalf("batch: %v", err)
	}
	initial, err := svc.Recommendations(ctx, "user-1")
	if err != nil || len(initial) != 3 {
		t.Fatalf("inicial = %+v, err=%v", initial, err)
	}
	result, err := svc.Feedback(ctx, "user-1", "r1", domain.RecommendationFeedback{
		RecommendationID: "r1", Action: domain.FeedbackDiscarded,
	})
	if err != nil {
		t.Fatalf("feedback: %v", err)
	}
	if len(result.Recommendations) != 2 {
		t.Fatalf("refinada = %+v", result.Recommendations)
	}
	for _, rec := range result.Recommendations {
		if rec.ID == "r1" {
			t.Fatal("la descartada volvió a mostrarse")
		}
	}
	if len(store.signals) != 1 || store.signals[0].Action != domain.FeedbackDiscarded {
		t.Fatalf("señales = %+v", store.signals)
	}
	if store.signals[0].UserBucket == "user-1" || len(store.signals[0].UserBucket) != 16 {
		t.Fatalf("bucket no anonimizado: %q", store.signals[0].UserBucket)
	}
	if store.signals[0].Destination != "Bariloche" || store.signals[0].PriceBucket != "mid" {
		t.Fatalf("señal incompleta: %+v", store.signals[0])
	}
}

func TestFeedbackAdjustUpdatesPreferencesImmediately(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	agencies := newFakeAgencies()
	rec := FromCatalog([]domain.AgencyCatalogItem{catalogItems()[0]})[0]
	agencies.recs[rec.ID] = &rec
	cache := NewMemoryCache()
	svc := testService(t, store, cache, agencies, &fakeCatalog{items: catalogItems()})
	if _, err := svc.RunBatch(ctx); err != nil {
		t.Fatalf("batch: %v", err)
	}
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	result, err := svc.Feedback(ctx, "user-1", "r1", domain.RecommendationFeedback{
		RecommendationID: "r1", Action: domain.FeedbackAdjust,
		Adjustments: &domain.RecommendationPreferences{MaxBudgetCents: 95000, PreferredStartDate: &start},
	})
	if err != nil {
		t.Fatalf("feedback: %v", err)
	}
	if store.prefs == nil || store.prefs.MaxBudgetCents != 95000 || store.preferencesSet != 1 {
		t.Fatalf("preferencias = %+v", store.prefs)
	}
	if len(result.Recommendations) != 2 {
		t.Fatalf("lista ajustada = %+v", result.Recommendations)
	}
}

func TestBatchJobUsesDistributedLock(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	agencies := newFakeAgencies()
	cache := NewMemoryCache()
	svc := testService(t, store, cache, agencies, &fakeCatalog{items: catalogItems()})
	token := newLockToken()
	if ok, err := cache.AcquireLock(ctx, lockKey, token, time.Minute); err != nil || !ok {
		t.Fatalf("lock previo: ok=%v err=%v", ok, err)
	}
	snap, err := svc.RunBatch(ctx)
	if err != nil {
		t.Fatalf("batch con lock ajeno: %v", err)
	}
	if snap.Version != "" {
		t.Fatal("el batch no debió ejecutarse con el lock tomado")
	}
	if err := cache.ReleaseLock(ctx, lockKey, token); err != nil {
		t.Fatal(err)
	}
	snap, err = svc.RunBatch(ctx)
	if err != nil || snap.Version == "" || snap.Signals != 0 {
		t.Fatalf("batch = %+v, err=%v", snap, err)
	}
	var pool domain.CandidatePool
	if err := cache.Get(ctx, poolKey, &pool); err != nil || len(pool.Candidates) != 3 {
		t.Fatalf("pool = %+v, err=%v", pool, err)
	}
	if err := cache.ReleaseLock(ctx, lockKey, "otro-token"); err != nil {
		t.Fatal(err)
	}
	var locked string
	if err := cache.Get(ctx, lockKey, &locked); !strings.Contains(err.Error(), "miss") {
		t.Fatalf("lock liberado por token inválido: %v", err)
	}
}

func TestBatchJobFallsBackToStaleCatalog(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.globalSignals = []domain.AnonymizedSignal{
		{Destination: "Bariloche", RecommendationType: domain.RecoTypeRoute, Action: domain.FeedbackInterested},
	}
	agencies := newFakeAgencies()
	cache := NewMemoryCache()
	mockStore := providers.NewMockStore()
	registry := providers.NewMockRegistry(mockStore)
	registry.SetAgencyCatalog(&fakeCatalog{items: catalogItems()})
	svc := NewService(store, cache, agencies, fakeHistory{}, registry, testLogger(), DefaultOptions())
	if _, err := svc.RunBatch(ctx); err != nil {
		t.Fatalf("primer batch: %v", err)
	}
	mockStore.SetAgencyFailure("ag1", true)
	if _, err := svc.RunBatch(ctx); err != nil {
		t.Fatalf("batch con agencia caída: %v", err)
	}
	var pool domain.CandidatePool
	if err := cache.Get(ctx, poolKey, &pool); err != nil {
		t.Fatalf("pool: %v", err)
	}
	if len(pool.Candidates) != 3 || pool.Snapshot.Signals != 1 {
		t.Fatalf("pool = %+v", pool)
	}
	if pool.Snapshot.Affinities[affinityKey("Bariloche", domain.RecoTypeRoute)] <= 0 {
		t.Fatalf("modelo sin afinidades: %+v", pool.Snapshot.Affinities)
	}
}

func TestRecommendationsDegradeWithoutRedis(t *testing.T) {
	agencies := newFakeAgencies()
	svc := testService(t, newFakeStore(), failingCache{}, agencies, &fakeCatalog{items: catalogItems()})
	recs, err := svc.Recommendations(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("sin Redis no debe fallar: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("sin Redis debe responder sin recomendaciones: %+v", recs)
	}
}

func TestAgencyRequestUsesAdapterAndExposesDelivery(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	agencies := newFakeAgencies()
	rec := FromCatalog([]domain.AgencyCatalogItem{catalogItems()[0]})[0]
	agencies.recs[rec.ID] = &rec
	svc := testService(t, store, NewMemoryCache(), agencies, &fakeCatalog{items: catalogItems()})
	created, err := svc.CreateAgencyRequest(ctx, "user-1", "r1", "", "¿Tiene disponibilidad?")
	if err != nil || created.Delivery == nil || created.Delivery.Status != "delivered" {
		t.Fatalf("creada = %+v, err=%v", created, err)
	}
	got, err := svc.AgencyRequest(ctx, "user-1", created.ID)
	if err != nil || got.Delivery == nil || got.Delivery.RequestID != created.ID {
		t.Fatalf("detalle = %+v, err=%v", got, err)
	}
	if len(store.deliveries) != 1 {
		t.Fatalf("entregas = %+v", store.deliveries)
	}
}

func TestUpdatePreferencesRejectsInvalidRange(t *testing.T) {
	svc := testService(t, newFakeStore(), NewMemoryCache(), newFakeAgencies(), &fakeCatalog{})
	start := time.Date(2030, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2030, 4, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.UpdatePreferences(context.Background(), "user-1", domain.RecommendationPreferences{
		PreferredStartDate: &start, PreferredEndDate: &end,
	})
	if !strings.Contains(err.Error(), "validation") {
		t.Fatalf("err = %v", err)
	}
}

package recommendation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/idgen"
	"trotamundos/internal/providers"
)

const (
	snapshotKey  = "reco:model:global"
	poolKey      = "reco:pool:global"
	lockKey      = "reco:batch:lock"
	resultPrefix = "reco:result:"
)

type AgencyService interface {
	ListAgencies(ctx context.Context) ([]domain.Agency, error)
	Favorite(ctx context.Context, userID string) (*domain.FavoriteAgency, error)
	SetFavorite(ctx context.Context, userID, agencyID string) (*domain.FavoriteAgency, error)
	DeleteFavorite(ctx context.Context, userID string, confirm bool) error
	GetRecommendation(ctx context.Context, id string) (*domain.Recommendation, error)
	SaveRecommendation(ctx context.Context, rec domain.Recommendation) error
	Inquire(ctx context.Context, userID, recommendationID, message string) (*domain.AgencyRequest, error)
	ContactAboutReservation(ctx context.Context, userID, reservationID, message string) (*domain.AgencyRequest, error)
	Request(ctx context.Context, userID, id string) (*domain.AgencyRequest, error)
	Requests(ctx context.Context, userID string) ([]domain.AgencyRequest, error)
	CloseRequest(ctx context.Context, userID, id string) error
}

type HistoryService interface {
	ListTrips(ctx context.Context, userID string, archived bool) ([]domain.Trip, error)
}

type AgencyRegistry interface {
	Agency(agencyID string) providers.AgencyAdapter
}

type Options struct {
	ResultTTL    time.Duration
	SnapshotTTL  time.Duration
	PoolTTL      time.Duration
	LockTTL      time.Duration
	SignalWindow time.Duration
	BatchLimit   int
}

func DefaultOptions() Options {
	return Options{
		ResultTTL:    5 * time.Minute,
		SnapshotTTL:  24 * time.Hour,
		PoolTTL:      24 * time.Hour,
		LockTTL:      10 * time.Minute,
		SignalWindow: 30 * 24 * time.Hour,
		BatchLimit:   10000,
	}
}

type Service struct {
	store     Store
	cache     Cache
	agencies  AgencyService
	history   HistoryService
	providers AgencyRegistry
	log       *slog.Logger
	opts      Options
	now       func() time.Time
}

func NewService(store Store, cache Cache, agencies AgencyService, history HistoryService, registry AgencyRegistry, log *slog.Logger, opts Options) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, cache: cache, agencies: agencies, history: history, providers: registry, log: log, opts: opts, now: time.Now}
}

func (s *Service) Agencies(ctx context.Context) ([]domain.Agency, error) {
	return s.agencies.ListAgencies(ctx)
}

func (s *Service) SetFavorite(ctx context.Context, userID, agencyID string) (*domain.FavoriteAgency, error) {
	fav, err := s.agencies.SetFavorite(ctx, userID, agencyID)
	if err == nil {
		s.invalidate(ctx, userID)
	}
	return fav, err
}

func (s *Service) DeleteFavorite(ctx context.Context, userID string, confirm bool) error {
	if err := s.agencies.DeleteFavorite(ctx, userID, confirm); err != nil {
		return err
	}
	s.invalidate(ctx, userID)
	return nil
}

func (s *Service) Preferences(ctx context.Context, userID string) (*domain.RecommendationPreferences, error) {
	return s.store.GetPreferences(ctx, userID)
}

func (s *Service) UpdatePreferences(ctx context.Context, userID string, input domain.RecommendationPreferences) (*domain.RecommendationPreferences, error) {
	if input.MaxBudgetCents < 0 {
		return nil, fmt.Errorf("%w: max_budget_cents no puede ser negativo", domain.ErrValidation)
	}
	if input.PreferredStartDate != nil && input.PreferredEndDate != nil && input.PreferredStartDate.After(*input.PreferredEndDate) {
		return nil, fmt.Errorf("%w: preferred_start_date debe ser anterior a preferred_end_date", domain.ErrValidation)
	}
	input.UserID = userID
	input.UpdatedAt = s.now().UTC()
	input.LodgingTypes = normalizeValues(input.LodgingTypes)
	input.TransportTypes = normalizeValues(input.TransportTypes)
	input.PreferredDestinations = normalizeValues(input.PreferredDestinations)
	if err := s.store.UpsertPreferences(ctx, input); err != nil {
		return nil, err
	}
	s.invalidate(ctx, userID)
	return &input, nil
}

func (s *Service) Recommendations(ctx context.Context, userID string) ([]domain.Recommendation, error) {
	fav, err := s.agencies.Favorite(ctx, userID)
	if err != nil {
		return nil, err
	}
	if fav == nil {
		return []domain.Recommendation{}, nil
	}
	key := s.resultKey(ctx, userID)
	var cached []domain.Recommendation
	if err := s.cache.Get(ctx, key, &cached); err == nil {
		return cached, nil
	} else if !errors.Is(err, ErrCacheMiss) {
		s.log.Warn("recommendation cache unavailable", "error", err)
		return []domain.Recommendation{}, nil
	}
	var snapshot domain.ModelSnapshot
	var pool domain.CandidatePool
	if err := s.cache.Get(ctx, snapshotKey, &snapshot); err != nil {
		s.degrade(ctx, "snapshot", err)
		return []domain.Recommendation{}, nil
	}
	if err := s.cache.Get(ctx, poolKey, &pool); err != nil {
		s.degrade(ctx, "pool", err)
		return []domain.Recommendation{}, nil
	}
	prefs, err := s.store.GetPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	discarded, err := s.store.DiscardedIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	interested, err := s.store.InterestedIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	trips, err := s.history.ListTrips(ctx, userID, false)
	if err != nil {
		return nil, err
	}
	candidates := make([]domain.Recommendation, 0, len(pool.Candidates))
	for _, rec := range pool.Candidates {
		if rec.AgencyID == fav.AgencyID {
			candidates = append(candidates, rec)
		}
	}
	ranked := Rank(candidates, snapshot, prefs, trips, discarded, interested)
	if err := s.cache.Set(ctx, key, ranked, s.opts.ResultTTL); err != nil {
		s.log.Warn("no se pudo cachear recomendaciones", "error", err)
	}
	return ranked, nil
}

func (s *Service) Feedback(ctx context.Context, userID, recommendationID string, input domain.RecommendationFeedback) (*domain.FeedbackResult, error) {
	action := strings.ToLower(strings.TrimSpace(input.Action))
	switch action {
	case domain.FeedbackInterested, domain.FeedbackDiscarded, domain.FeedbackAdjust:
	default:
		return nil, fmt.Errorf("%w: acción de feedback inválida", domain.ErrValidation)
	}
	rec, err := s.agencies.GetRecommendation(ctx, recommendationID)
	if err != nil {
		return nil, err
	}
	var adjustments any
	if action == domain.FeedbackAdjust {
		if input.Adjustments == nil {
			return nil, fmt.Errorf("%w: adjust requiere adjustments", domain.ErrValidation)
		}
		params := *input.Adjustments
		current, err := s.store.GetPreferences(ctx, userID)
		if err != nil {
			return nil, err
		}
		merged := mergePreferences(current, params)
		prefs, err := s.UpdatePreferences(ctx, userID, merged)
		if err != nil {
			return nil, err
		}
		adjustments = prefs
	}
	signalID := idgen.UUID()
	if err := s.store.RecordInteraction(ctx, userID, rec.ID, action, adjustments); err != nil {
		return nil, err
	}
	signal := domain.AnonymizedSignal{
		ID: signalID, UserBucket: userBucket(userID), Destination: rec.Destination,
		RecommendationType: rec.Type, Action: action, PriceBucket: priceBucket(rec.PriceCents),
		CreatedAt: s.now().UTC(),
	}
	if err := s.store.WriteAnonymizedSignals(ctx, []domain.AnonymizedSignal{signal}); err != nil {
		return nil, err
	}
	s.invalidate(ctx, userID)
	recs, err := s.Recommendations(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &domain.FeedbackResult{SignalID: signalID, Recommendations: recs}, nil
}

func (s *Service) InfoRequest(ctx context.Context, userID, recommendationID, message string) (*domain.AgencyRequest, error) {
	req, err := s.agencies.Inquire(ctx, userID, recommendationID, message)
	if err != nil {
		return nil, err
	}
	s.deliver(ctx, req)
	return req, nil
}

func (s *Service) CreateAgencyRequest(ctx context.Context, userID, recommendationID, reservationID, message string) (*domain.AgencyRequest, error) {
	hasRec := strings.TrimSpace(recommendationID) != ""
	hasReservation := strings.TrimSpace(reservationID) != ""
	if hasRec == hasReservation {
		return nil, fmt.Errorf("%w: indicar recommendation_id o reservation_id", domain.ErrValidation)
	}
	var req *domain.AgencyRequest
	var err error
	if hasRec {
		req, err = s.agencies.Inquire(ctx, userID, recommendationID, message)
	} else {
		req, err = s.agencies.ContactAboutReservation(ctx, userID, reservationID, message)
	}
	if err != nil {
		return nil, err
	}
	s.deliver(ctx, req)
	return req, nil
}

func (s *Service) AgencyRequest(ctx context.Context, userID, id string) (*domain.AgencyRequest, error) {
	req, err := s.agencies.Request(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	delivery, err := s.store.GetDelivery(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	req.Delivery = delivery
	return req, nil
}

func (s *Service) Requests(ctx context.Context, userID string) ([]domain.AgencyRequest, error) {
	return s.agencies.Requests(ctx, userID)
}

func (s *Service) CloseRequest(ctx context.Context, userID, id string) error {
	return s.agencies.CloseRequest(ctx, userID, id)
}

func (s *Service) RunBatch(ctx context.Context) (domain.ModelSnapshot, error) {
	empty := domain.ModelSnapshot{Affinities: map[string]float64{}}
	token := idgen.UUID()
	acquired, err := s.cache.AcquireLock(ctx, lockKey, token, s.opts.LockTTL)
	if err != nil {
		return empty, err
	}
	if !acquired {
		return empty, nil
	}
	defer func() {
		if err := s.cache.ReleaseLock(ctx, lockKey, token); err != nil {
			s.log.Warn("no se pudo liberar lock del batch", "error", err)
		}
	}()
	now := s.now().UTC()
	signals, err := s.store.GlobalSignals(ctx, now.Add(-s.opts.SignalWindow), s.opts.BatchLimit)
	if err != nil {
		return empty, err
	}
	snapshot := TrainModel(signals, now)
	agencyList, err := s.agencies.ListAgencies(ctx)
	if err != nil {
		return empty, err
	}
	seen := map[string]bool{}
	candidates := []domain.Recommendation{}
	for _, agency := range agencyList {
		adapter := s.providers.Agency(agency.ID)
		if adapter == nil {
			continue
		}
		catalog, err := adapter.Catalog(ctx, agency.ID)
		if err != nil {
			s.log.Warn("catálogo de agencia no disponible", "agency_id", agency.ID, "error", err)
			continue
		}
		if catalog.Stale {
			s.log.Warn("usando catálogo en caché", "agency_id", agency.ID, "fetched_at", catalog.FetchedAt)
		}
		for _, rec := range FromCatalog(catalog.Items) {
			if seen[rec.ID] {
				continue
			}
			if err := s.agencies.SaveRecommendation(ctx, rec); err != nil {
				return empty, fmt.Errorf("guardar recomendación %s: %w", rec.ID, err)
			}
			seen[rec.ID] = true
			candidates = append(candidates, rec)
		}
	}
	pool := domain.CandidatePool{Snapshot: snapshot, Candidates: candidates, GeneratedAt: now}
	if err := s.cache.Set(ctx, snapshotKey, snapshot, s.opts.SnapshotTTL); err != nil {
		return empty, err
	}
	if err := s.cache.Set(ctx, poolKey, pool, s.opts.PoolTTL); err != nil {
		return empty, err
	}
	s.log.Info("modelo global publicado", "version", snapshot.Version, "signals", snapshot.Signals, "candidates", len(candidates))
	return snapshot, nil
}

func (s *Service) deliver(ctx context.Context, req *domain.AgencyRequest) {
	adapter := s.providers.Agency(req.AgencyID)
	if adapter == nil {
		return
	}
	delivery, err := adapter.SendRequest(ctx, domain.AgencyRequest{ID: req.ID, AgencyID: req.AgencyID})
	if err != nil {
		delivery = domain.AgencyDelivery{RequestID: req.ID, AgencyID: req.AgencyID, Status: "failed", Detail: err.Error(), SentAt: s.now().UTC()}
	}
	if err := s.store.RecordDelivery(ctx, delivery); err != nil {
		s.log.Warn("no se pudo registrar entrega a agencia", "error", err)
	}
	req.Delivery = &delivery
}

func (s *Service) invalidate(ctx context.Context, userID string) {
	_ = s.cache.Delete(ctx, s.resultKey(ctx, userID), resultPrefix+userID)
}

func (s *Service) resultKey(ctx context.Context, userID string) string {
	var snapshot domain.ModelSnapshot
	if err := s.cache.Get(ctx, snapshotKey, &snapshot); err == nil && snapshot.Version != "" {
		return resultPrefix + snapshot.Version + ":" + userID
	}
	return resultPrefix + userID
}

func (s *Service) degrade(ctx context.Context, resource string, err error) {
	if errors.Is(err, ErrCacheMiss) {
		s.log.Info("sin datos de recomendación en caché", "resource", resource)
		return
	}
	s.log.Warn("caché de recomendaciones no disponible", "resource", resource, "error", err)
}

func mergePreferences(current *domain.RecommendationPreferences, changes domain.RecommendationPreferences) domain.RecommendationPreferences {
	merged := changes
	if current == nil {
		return merged
	}
	if merged.MaxBudgetCents == 0 {
		merged.MaxBudgetCents = current.MaxBudgetCents
	}
	if len(merged.LodgingTypes) == 0 {
		merged.LodgingTypes = current.LodgingTypes
	}
	if len(merged.TransportTypes) == 0 {
		merged.TransportTypes = current.TransportTypes
	}
	if len(merged.PreferredDestinations) == 0 {
		merged.PreferredDestinations = current.PreferredDestinations
	}
	if merged.PreferredStartDate == nil {
		merged.PreferredStartDate = current.PreferredStartDate
	}
	if merged.PreferredEndDate == nil {
		merged.PreferredEndDate = current.PreferredEndDate
	}
	return merged
}

func normalizeValues(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func userBucket(userID string) string {
	sum := sha256.Sum256([]byte(userID))
	return hex.EncodeToString(sum[:])[:16]
}

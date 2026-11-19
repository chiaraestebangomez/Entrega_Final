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
	var havePool bool
	if err := s.cache.Get(ctx, snapshotKey, &snapshot); err != nil {
		s.degrade(ctx, "snapshot", err)
		// Cold start: sin modelo entrenado se rankea con pesos neutros.
		snapshot = domain.ModelSnapshot{Affinities: map[string]float64{}}
	}
	if err := s.cache.Get(ctx, poolKey, &pool); err != nil {
		s.degrade(ctx, "pool", err)
	} else {
		havePool = true
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
	var candidates []domain.Recommendation
	if havePool {
		candidates = make([]domain.Recommendation, 0, len(pool.Candidates))
		for _, rec := range pool.Candidates {
			if rec.AgencyID == fav.AgencyID {
				candidates = append(candidates, rec)
			}
		}
	} else {
		// Cold start: reconstruimos el catálogo de la agencia favorita en lugar
		// de devolver una lista vacía mientras corre el primer batch.
		candidates = s.coldStartCandidates(ctx, fav.AgencyID)
	}
	ranked := Rank(candidates, snapshot, prefs, trips, userBucket(userID), discarded, interested)
	if err := s.cache.Set(ctx, key, ranked, s.opts.ResultTTL); err != nil {
		s.log.Warn("no se pudo cachear recomendaciones", "error", err)
	}
	return ranked, nil
}

// coldStartCandidates obtiene las recomendaciones directamente del adaptador de
// la agencia favorita cuando todavía no corrió el batch que publica el pool.
func (s *Service) coldStartCandidates(ctx context.Context, agencyID string) []domain.Recommendation {
	adapter := s.providers.Agency(agencyID)
	if adapter == nil {
		return []domain.Recommendation{}
	}
	catalog, err := adapter.Catalog(ctx, agencyID)
	if err != nil {
		s.log.Warn("cold start: catálogo de agencia no disponible", "agency_id", agencyID, "error", err)
		return []domain.Recommendation{}
	}
	return FromCatalog(catalog.Items)
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
	if rec == nil {
		return nil, domain.ErrNotFound
	}
	// Misma validación que agencies.Service.assertRecommendation: la
	// recomendación debe pertenecer a la agencia favorita del usuario.
	fav, err := s.agencies.Favorite(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	if fav == nil || fav.AgencyID != rec.AgencyID {
		return nil, domain.ErrForbidden
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

// recordInquirySignal registra la consulta como señal anonimizada para que el
// modelo batch la tenga en cuenta, igual que el resto del feedback.
func (s *Service) recordInquirySignal(ctx context.Context, userID, recommendationID string) {
	rec, err := s.agencies.GetRecommendation(ctx, recommendationID)
	if err != nil || rec == nil {
		if err != nil {
			s.log.Warn("no se pudo resolver la recomendación consultada", "error", err)
		}
		return
	}
	signal := domain.AnonymizedSignal{
		ID: idgen.UUID(), UserBucket: userBucket(userID), Destination: rec.Destination,
		RecommendationType: rec.Type, Action: domain.FeedbackInquiry, PriceBucket: priceBucket(rec.PriceCents),
		CreatedAt: s.now().UTC(),
	}
	if err := s.store.WriteAnonymizedSignals(ctx, []domain.AnonymizedSignal{signal}); err != nil {
		s.log.Warn("no se pudo registrar la señal de consulta", "error", err)
	}
	s.invalidate(ctx, userID)
}

func (s *Service) InfoRequest(ctx context.Context, userID, recommendationID, message string) (*domain.AgencyRequest, error) {
	req, err := s.agencies.Inquire(ctx, userID, recommendationID, message)
	if err != nil {
		return nil, err
	}
	s.recordInquirySignal(ctx, userID, recommendationID)
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
	if hasRec {
		s.recordInquirySignal(ctx, userID, recommendationID)
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
	// Borramos la clave vigente, la clave histórica sin versión y, si el cache
	// sabe enumerar, cualquier versión intermedia (reco:result:<version>:<userID>).
	keys := []string{s.resultKey(ctx, userID), resultPrefix + userID}
	if lister, ok := s.cache.(KeyLister); ok {
		matched, err := lister.Keys(ctx, resultPrefix+"*")
		if err != nil {
			s.log.Warn("no se pudieron enumerar las claves de resultados", "error", err)
		} else {
			for _, key := range matched {
				if key == resultPrefix+userID || strings.HasSuffix(key, ":"+userID) {
					keys = append(keys, key)
				}
			}
		}
	}
	if deleter, ok := s.cache.(PatternDeleter); ok {
		pattern := resultPrefix + "*" + escapeGlob(userID)
		if _, err := deleter.DeletePattern(ctx, pattern); err != nil {
			s.log.Warn("no se pudieron borrar las claves de resultados por patrón", "pattern", pattern, "error", err)
		}
	}

	unique := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, key)
	}
	if err := s.cache.Delete(ctx, unique...); err != nil {
		s.log.Warn("no se pudo invalidar la caché de recomendaciones", "error", err)
	}
}

// escapeGlob mantiene el userID literal dentro de un patrón MATCH de Redis o
// path.Match, para que un ID con metacaracteres no invalide otras cuentas.
func escapeGlob(value string) string {
	return strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`, `]`, `\]`).Replace(value)
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

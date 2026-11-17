package recommendation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"trotamundos/internal/domain"
)

type Store interface {
	GetPreferences(ctx context.Context, userID string) (*domain.RecommendationPreferences, error)
	UpsertPreferences(ctx context.Context, prefs domain.RecommendationPreferences) error
	DiscardedIDs(ctx context.Context, userID string) (map[string]bool, error)
	InterestedIDs(ctx context.Context, userID string) (map[string]bool, error)
	RecordInteraction(ctx context.Context, userID, recommendationID, action string, adjustments any) error
	WriteAnonymizedSignals(ctx context.Context, signals []domain.AnonymizedSignal) error
	GlobalSignals(ctx context.Context, since time.Time, limit int) ([]domain.AnonymizedSignal, error)
	RecordDelivery(ctx context.Context, delivery domain.AgencyDelivery) error
	GetDelivery(ctx context.Context, requestID string) (*domain.AgencyDelivery, error)
}

type PostgresStore struct {
	oltp *pgxpool.Pool
	olap *pgxpool.Pool
}

func NewPostgresStore(oltp, olap *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{oltp: oltp, olap: olap}
}

func (s *PostgresStore) GetPreferences(ctx context.Context, userID string) (*domain.RecommendationPreferences, error) {
	const q = `SELECT user_id, max_budget_cents, lodging_types, transport_types,
		preferred_destinations, preferred_start_date, preferred_end_date, updated_at
		FROM recommendation.user_preferences WHERE user_id=$1`
	var p domain.RecommendationPreferences
	err := s.oltp.QueryRow(ctx, q, userID).Scan(&p.UserID, &p.MaxBudgetCents, &p.LodgingTypes, &p.TransportTypes,
		&p.PreferredDestinations, &p.PreferredStartDate, &p.PreferredEndDate, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *PostgresStore) UpsertPreferences(ctx context.Context, p domain.RecommendationPreferences) error {
	const q = `INSERT INTO recommendation.user_preferences
		(user_id, max_budget_cents, lodging_types, transport_types, preferred_destinations,
		 preferred_start_date, preferred_end_date, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (user_id) DO UPDATE SET
		 max_budget_cents=EXCLUDED.max_budget_cents,
		 lodging_types=EXCLUDED.lodging_types,
		 transport_types=EXCLUDED.transport_types,
		 preferred_destinations=EXCLUDED.preferred_destinations,
		 preferred_start_date=EXCLUDED.preferred_start_date,
		 preferred_end_date=EXCLUDED.preferred_end_date,
		 updated_at=EXCLUDED.updated_at`
	_, err := s.oltp.Exec(ctx, q, p.UserID, p.MaxBudgetCents, stringsOrEmpty(p.LodgingTypes),
		stringsOrEmpty(p.TransportTypes), stringsOrEmpty(p.PreferredDestinations),
		dateArg(p.PreferredStartDate), dateArg(p.PreferredEndDate), p.UpdatedAt.UTC())
	return err
}

func (s *PostgresStore) DiscardedIDs(ctx context.Context, userID string) (map[string]bool, error) {
	return s.interactionIDs(ctx, userID, domain.FeedbackDiscarded)
}

func (s *PostgresStore) InterestedIDs(ctx context.Context, userID string) (map[string]bool, error) {
	return s.interactionIDs(ctx, userID, domain.FeedbackInterested)
}

func (s *PostgresStore) interactionIDs(ctx context.Context, userID, action string) (map[string]bool, error) {
	rows, err := s.oltp.Query(ctx, `SELECT recommendation_id FROM recommendation.user_interactions WHERE user_id=$1 AND action=$2`, userID, action)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (s *PostgresStore) RecordInteraction(ctx context.Context, userID, recommendationID, action string, adjustments any) error {
	raw, err := json.Marshal(adjustments)
	if err != nil {
		return err
	}
	_, err = s.oltp.Exec(ctx, `INSERT INTO recommendation.user_interactions
		(id, user_id, recommendation_id, action, adjustments, created_at)
		VALUES (gen_random_uuid(),$1,$2,$3,$4,now())`, userID, recommendationID, action, string(raw))
	return err
}

func (s *PostgresStore) WriteAnonymizedSignals(ctx context.Context, signals []domain.AnonymizedSignal) error {
	for _, signal := range signals {
		_, err := s.oltp.Exec(ctx, `INSERT INTO recommendation.anonymized_signals
			(id, user_bucket, destination, recommendation_type, action, price_bucket, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`, signal.ID, signal.UserBucket, signal.Destination,
			signal.RecommendationType, signal.Action, signal.PriceBucket, signal.CreatedAt.UTC())
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) GlobalSignals(ctx context.Context, since time.Time, limit int) ([]domain.AnonymizedSignal, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	const q = `SELECT id, destination, recommendation_type, action, price_bucket, created_at
		FROM recommendation.anonymized_signals WHERE created_at >= $1
		ORDER BY created_at DESC LIMIT $2`
	rows, err := s.olap.Query(ctx, q, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AnonymizedSignal{}
	for rows.Next() {
		var signal domain.AnonymizedSignal
		if err := rows.Scan(&signal.ID, &signal.Destination, &signal.RecommendationType,
			&signal.Action, &signal.PriceBucket, &signal.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, signal)
	}
	return out, rows.Err()
}

func (s *PostgresStore) RecordDelivery(ctx context.Context, delivery domain.AgencyDelivery) error {
	_, err := s.oltp.Exec(ctx, `INSERT INTO recommendation.agency_deliveries
		(request_id, agency_id, status, detail, sent_at) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (request_id) DO UPDATE SET status=EXCLUDED.status, detail=EXCLUDED.detail, sent_at=EXCLUDED.sent_at`,
		delivery.RequestID, delivery.AgencyID, delivery.Status, delivery.Detail, delivery.SentAt.UTC())
	return err
}

func (s *PostgresStore) GetDelivery(ctx context.Context, requestID string) (*domain.AgencyDelivery, error) {
	const q = `SELECT request_id, agency_id, status, detail, sent_at FROM recommendation.agency_deliveries WHERE request_id=$1`
	var d domain.AgencyDelivery
	err := s.oltp.QueryRow(ctx, q, requestID).Scan(&d.RequestID, &d.AgencyID, &d.Status, &d.Detail, &d.SentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func dateArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format("2006-01-02")
}

func stringsOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

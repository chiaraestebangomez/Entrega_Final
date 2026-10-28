package agencies

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"trotamundos/internal/domain"
)

type Repository interface {
	ListAgencies(ctx context.Context) ([]domain.Agency, error)
	AgencyExists(ctx context.Context, id string) (bool, error)
	GetFavorite(ctx context.Context, userID string) (*domain.FavoriteAgency, error)
	SetFavorite(ctx context.Context, userID, agencyID string) error
	DeleteFavorite(ctx context.Context, userID string) error

	ListRecommendations(ctx context.Context, userID string) ([]domain.Recommendation, error)
	GetRecommendation(ctx context.Context, id string) (*domain.Recommendation, error)
	RecordResponse(ctx context.Context, userID, recommendationID, action string) error

	CreateRequest(ctx context.Context, req *domain.AgencyRequest) error
	ListRequests(ctx context.Context, userID string) ([]domain.AgencyRequest, error)
	GetRequest(ctx context.Context, userID, id string) (*domain.AgencyRequest, error)
	CloseRequest(ctx context.Context, userID, id string, at time.Time) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ListAgencies(ctx context.Context) ([]domain.Agency, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, email, phone, description FROM itinerary.agencies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Agency{}
	for rows.Next() {
		var a domain.Agency
		if err := rows.Scan(&a.ID, &a.Name, &a.Email, &a.Phone, &a.Description); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) AgencyExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM itinerary.agencies WHERE id=$1)`, id).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) GetFavorite(ctx context.Context, userID string) (*domain.FavoriteAgency, error) {
	const q = `SELECT f.user_id, f.agency_id, f.created_at, a.id, a.name, a.email, a.phone, a.description
		FROM itinerary.agency_favorites f JOIN itinerary.agencies a ON a.id=f.agency_id
		WHERE f.user_id=$1`
	var f domain.FavoriteAgency
	var a domain.Agency
	err := r.pool.QueryRow(ctx, q, userID).Scan(&f.UserID, &f.AgencyID, &f.CreatedAt, &a.ID, &a.Name, &a.Email, &a.Phone, &a.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	f.Agency = &a
	return &f, nil
}

func (r *PostgresRepository) SetFavorite(ctx context.Context, userID, agencyID string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO itinerary.agency_favorites (user_id, agency_id, created_at)
		VALUES ($1,$2,now()) ON CONFLICT (user_id) DO UPDATE SET agency_id=EXCLUDED.agency_id, created_at=now()`, userID, agencyID)
	return err
}

func (r *PostgresRepository) DeleteFavorite(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM itinerary.agency_favorites WHERE user_id=$1`, userID)
	return err
}

func (r *PostgresRepository) ListRecommendations(ctx context.Context, userID string) ([]domain.Recommendation, error) {
	const q = `SELECT r.id, r.agency_id, a.name, r.title, r.destination, r.description, r.price_cents, r.currency,
		r.start_date, r.end_date, r.active
		FROM itinerary.agency_recommendations r
		JOIN itinerary.agencies a ON a.id=r.agency_id
		WHERE r.agency_id=(SELECT agency_id FROM itinerary.agency_favorites WHERE user_id=$1)
		  AND r.active
		  AND NOT EXISTS (SELECT 1 FROM itinerary.recommendation_responses rr
		                  WHERE rr.user_id=$1 AND rr.recommendation_id=r.id AND rr.action='discarded')
		ORDER BY r.start_date NULLS LAST, r.title`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Recommendation{}
	for rows.Next() {
		var rec domain.Recommendation
		if err := rows.Scan(&rec.ID, &rec.AgencyID, &rec.AgencyName, &rec.Title, &rec.Destination, &rec.Description,
			&rec.PriceCents, &rec.Currency, &rec.StartDate, &rec.EndDate, &rec.Active); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetRecommendation(ctx context.Context, id string) (*domain.Recommendation, error) {
	const q = `SELECT r.id, r.agency_id, a.name, r.title, r.destination, r.description, r.price_cents, r.currency,
		r.start_date, r.end_date, r.active
		FROM itinerary.agency_recommendations r JOIN itinerary.agencies a ON a.id=r.agency_id WHERE r.id=$1`
	var rec domain.Recommendation
	err := r.pool.QueryRow(ctx, q, id).Scan(&rec.ID, &rec.AgencyID, &rec.AgencyName, &rec.Title, &rec.Destination,
		&rec.Description, &rec.PriceCents, &rec.Currency, &rec.StartDate, &rec.EndDate, &rec.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *PostgresRepository) RecordResponse(ctx context.Context, userID, recommendationID, action string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO itinerary.recommendation_responses (id, user_id, recommendation_id, action, created_at)
		VALUES (gen_random_uuid(), $1,$2,$3,now())
		ON CONFLICT (user_id, recommendation_id) DO UPDATE SET action=EXCLUDED.action, created_at=now()`,
		userID, recommendationID, action)
	return err
}

func (r *PostgresRepository) CreateRequest(ctx context.Context, req *domain.AgencyRequest) error {
	ctxJSON, err := json.Marshal(orEmpty(req.Context))
	if err != nil {
		return err
	}
	const q = `INSERT INTO itinerary.agency_requests
		(id, user_id, agency_id, reservation_id, recommendation_id, message, status, context, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	_, err = r.pool.Exec(ctx, q, req.ID, req.UserID, req.AgencyID, nullableUUID(req.ReservationID),
		nullableUUID(req.RecommendationID), req.Message, req.Status, ctxJSON, req.CreatedAt)
	return err
}

func (r *PostgresRepository) ListRequests(ctx context.Context, userID string) ([]domain.AgencyRequest, error) {
	const q = `SELECT q.id, q.user_id, q.agency_id, a.name, COALESCE(q.reservation_id::text,''),
		COALESCE(q.recommendation_id::text,''), q.message, q.status, q.context, q.created_at, q.closed_at
		FROM itinerary.agency_requests q JOIN itinerary.agencies a ON a.id=q.agency_id
		WHERE q.user_id=$1 ORDER BY q.created_at DESC`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AgencyRequest{}
	for rows.Next() {
		var req domain.AgencyRequest
		var raw []byte
		if err := rows.Scan(&req.ID, &req.UserID, &req.AgencyID, &req.AgencyName, &req.ReservationID, &req.RecommendationID,
			&req.Message, &req.Status, &raw, &req.CreatedAt, &req.ClosedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &req.Context)
		out = append(out, req)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetRequest(ctx context.Context, userID, id string) (*domain.AgencyRequest, error) {
	const q = `SELECT q.id, q.user_id, q.agency_id, a.name, COALESCE(q.reservation_id::text,''),
		COALESCE(q.recommendation_id::text,''), q.message, q.status, q.context, q.created_at, q.closed_at
		FROM itinerary.agency_requests q JOIN itinerary.agencies a ON a.id=q.agency_id
		WHERE q.id=$1 AND q.user_id=$2`
	var req domain.AgencyRequest
	var raw []byte
	err := r.pool.QueryRow(ctx, q, id, userID).Scan(&req.ID, &req.UserID, &req.AgencyID, &req.AgencyName, &req.ReservationID,
		&req.RecommendationID, &req.Message, &req.Status, &raw, &req.CreatedAt, &req.ClosedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(raw, &req.Context)
	return &req, nil
}

func (r *PostgresRepository) CloseRequest(ctx context.Context, userID, id string, at time.Time) error {
	tag, err := r.pool.Exec(ctx, `UPDATE itinerary.agency_requests SET status='closed', closed_at=$3
		WHERE id=$1 AND user_id=$2 AND status='open'`, id, userID, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func nullableUUID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

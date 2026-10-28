package sharing

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"trotamundos/internal/domain"
)

type Repository interface {
	ListByTrip(ctx context.Context, ownerUserID, tripID string) ([]domain.TripShare, error)
	Create(ctx context.Context, share *domain.TripShare, now time.Time) error
	Revoke(ctx context.Context, ownerUserID, shareID string, now time.Time) error
	ClaimInvitations(ctx context.Context, userID, email string) error
	ListSharedWithMe(ctx context.Context, userID, email string) ([]domain.TripShare, error)
	GetGuestShare(ctx context.Context, tripID, userID, email string) (*domain.TripShare, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ListByTrip(ctx context.Context, ownerUserID, tripID string) ([]domain.TripShare, error) {
	const q = `SELECT id, trip_id, owner_user_id, guest_email::text, COALESCE(guest_user_id::text,''), created_at, revoked_at
		FROM itinerary.trip_shares WHERE owner_user_id=$1 AND trip_id=$2 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, ownerUserID, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShares(rows)
}

func (r *PostgresRepository) Create(ctx context.Context, share *domain.TripShare, now time.Time) error {
	if _, err := r.pool.Exec(ctx, `UPDATE itinerary.trip_shares SET revoked_at=$3
		WHERE trip_id=$1 AND guest_email=$2 AND revoked_at IS NULL`, share.TripID, share.GuestEmail, now); err != nil {
		return err
	}
	const q = `INSERT INTO itinerary.trip_shares (id, trip_id, owner_user_id, guest_email, guest_user_id, created_at)
		VALUES ($1,$2,$3,$4::citext,(SELECT id FROM auth.users WHERE lower(email)=lower($4::citext)),$5)`
	_, err := r.pool.Exec(ctx, q, share.ID, share.TripID, share.OwnerUserID, share.GuestEmail, share.CreatedAt)
	return err
}

func (r *PostgresRepository) Revoke(ctx context.Context, ownerUserID, shareID string, now time.Time) error {
	tag, err := r.pool.Exec(ctx, `UPDATE itinerary.trip_shares SET revoked_at=$3
		WHERE id=$1 AND owner_user_id=$2 AND revoked_at IS NULL`, shareID, ownerUserID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ClaimInvitations(ctx context.Context, userID, email string) error {
	_, err := r.pool.Exec(ctx, `UPDATE itinerary.trip_shares SET guest_user_id=$1
		WHERE guest_user_id IS NULL AND lower(guest_email)=lower($2)`, userID, strings.TrimSpace(email))
	return err
}

func (r *PostgresRepository) ListSharedWithMe(ctx context.Context, userID, email string) ([]domain.TripShare, error) {
	const q = `SELECT id, trip_id, owner_user_id, guest_email::text, COALESCE(guest_user_id::text,''), created_at, revoked_at
		FROM itinerary.trip_shares
		WHERE revoked_at IS NULL AND (guest_user_id=$1 OR lower(guest_email)=lower($2))
		ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, userID, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShares(rows)
}

func (r *PostgresRepository) GetGuestShare(ctx context.Context, tripID, userID, email string) (*domain.TripShare, error) {
	const q = `SELECT id, trip_id, owner_user_id, guest_email::text, COALESCE(guest_user_id::text,''), created_at, revoked_at
		FROM itinerary.trip_shares
		WHERE trip_id=$1 AND revoked_at IS NULL AND (guest_user_id=$2 OR lower(guest_email)=lower($3))
		ORDER BY created_at DESC LIMIT 1`
	var s domain.TripShare
	err := r.pool.QueryRow(ctx, q, tripID, userID, email).Scan(&s.ID, &s.TripID, &s.OwnerUserID, &s.GuestEmail, &s.GuestUserID, &s.CreatedAt, &s.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func scanShares(rows pgx.Rows) ([]domain.TripShare, error) {
	out := []domain.TripShare{}
	for rows.Next() {
		var s domain.TripShare
		if err := rows.Scan(&s.ID, &s.TripID, &s.OwnerUserID, &s.GuestEmail, &s.GuestUserID, &s.CreatedAt, &s.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

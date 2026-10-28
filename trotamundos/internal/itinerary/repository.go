package itinerary

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
	CreateTrip(ctx context.Context, t *domain.Trip) error
	UpdateTrip(ctx context.Context, t *domain.Trip) error
	GetTrip(ctx context.Context, userID, tripID string) (*domain.Trip, error)
	ListTrips(ctx context.Context, userID string, archived bool) ([]domain.Trip, error)
	ListActiveTrips(ctx context.Context, userID string) ([]domain.Trip, error)
	DeleteTrip(ctx context.Context, userID, tripID string) error
	RefreshTripBounds(ctx context.Context, userID, tripID string) error
	ArchivePastTrips(ctx context.Context, now time.Time) (int64, error)

	CreateReservation(ctx context.Context, r *domain.Reservation) error
	UpdateReservation(ctx context.Context, r *domain.Reservation) error
	GetReservation(ctx context.Context, userID, id string) (*domain.Reservation, error)
	FindByProviderLocator(ctx context.Context, userID, provider, locator string) (*domain.Reservation, error)
	ListReservationsByTrip(ctx context.Context, userID, tripID string) ([]domain.Reservation, error)
	DeleteReservation(ctx context.Context, userID, id string) error
	ListReservationsForSync(ctx context.Context, limit int) ([]domain.Reservation, error)

	AddChange(ctx context.Context, c *domain.ReservationChange) error
	ListChanges(ctx context.Context, userID, reservationID string) ([]domain.ReservationChange, error)
	AddNotification(ctx context.Context, n *domain.Notification) error
	ListNotifications(ctx context.Context, userID string, limit int) ([]domain.Notification, error)
	MarkNotificationRead(ctx context.Context, userID, id string) error
	AddAudit(ctx context.Context, a *domain.AuditLog) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateTrip(ctx context.Context, t *domain.Trip) error {
	const q = `INSERT INTO itinerary.trips (id, user_id, name, destination, start_date, end_date, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	_, err := r.pool.Exec(ctx, q, t.ID, t.UserID, t.Name, t.Destination, dateArg(t.StartDate), dateArg(t.EndDate), t.Status, t.CreatedAt, t.UpdatedAt)
	return err
}

func (r *PostgresRepository) UpdateTrip(ctx context.Context, t *domain.Trip) error {
	const q = `UPDATE itinerary.trips SET name=$3, destination=$4, start_date=$5, end_date=$6, status=$7, updated_at=$8
		WHERE id=$1 AND user_id=$2`
	tag, err := r.pool.Exec(ctx, q, t.ID, t.UserID, t.Name, t.Destination, dateArg(t.StartDate), dateArg(t.EndDate), t.Status, t.UpdatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) GetTrip(ctx context.Context, userID, tripID string) (*domain.Trip, error) {
	const q = `SELECT id, user_id, name, destination, start_date, end_date, status, created_at, updated_at
		FROM itinerary.trips WHERE id=$1 AND user_id=$2`
	var t domain.Trip
	var start, end *time.Time
	err := r.pool.QueryRow(ctx, q, tripID, userID).Scan(&t.ID, &t.UserID, &t.Name, &t.Destination, &start, &end, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.StartDate, t.EndDate = normalizeDate(start), normalizeDate(end)
	return &t, nil
}

func (r *PostgresRepository) ListTrips(ctx context.Context, userID string, archived bool) ([]domain.Trip, error) {
	status := domain.TripActive
	if archived {
		status = domain.TripArchived
	}
	const q = `SELECT id, user_id, name, destination, start_date, end_date, status, created_at, updated_at
		FROM itinerary.trips WHERE user_id=$1 AND status=$2
		ORDER BY COALESCE(start_date, created_at::date) ASC`
	rows, err := r.pool.Query(ctx, q, userID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Trip{}
	for rows.Next() {
		var t domain.Trip
		var start, end *time.Time
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Destination, &start, &end, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.StartDate, t.EndDate = normalizeDate(start), normalizeDate(end)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListActiveTrips(ctx context.Context, userID string) ([]domain.Trip, error) {
	return r.ListTrips(ctx, userID, false)
}

func (r *PostgresRepository) DeleteTrip(ctx context.Context, userID, tripID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM itinerary.trips WHERE id=$1 AND user_id=$2`, tripID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) RefreshTripBounds(ctx context.Context, userID, tripID string) error {
	const q = `UPDATE itinerary.trips t
		SET start_date = s.min_start::date, end_date = s.max_end::date, updated_at = now()
		FROM (
			SELECT min(start_at) AS min_start, max(COALESCE(end_at, start_at)) AS max_end
			FROM itinerary.reservations
			WHERE user_id=$1 AND trip_id=$2 AND start_at IS NOT NULL
		) s
		WHERE t.id=$2 AND t.user_id=$1 AND s.min_start IS NOT NULL`
	_, err := r.pool.Exec(ctx, q, userID, tripID)
	return err
}

func (r *PostgresRepository) ArchivePastTrips(ctx context.Context, now time.Time) (int64, error) {
	const q = `UPDATE itinerary.trips SET status='archived', updated_at=now()
		WHERE status='active' AND end_date IS NOT NULL AND end_date < $1::date`
	tag, err := r.pool.Exec(ctx, q, now.UTC().Format("2006-01-02"))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *PostgresRepository) CreateReservation(ctx context.Context, res *domain.Reservation) error {
	const q = `INSERT INTO itinerary.reservations
		(id, user_id, trip_id, reservation_type, provider, provider_locator, title, destination, start_at, end_at,
		 status, source, source_ref, details, last_synced_at, stale, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`
	details, err := json.Marshal(orEmptyMap(res.Details))
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, q, res.ID, res.UserID, res.TripID, res.Type, res.Provider, res.ProviderLocator,
		res.Title, res.Destination, res.StartAt, res.EndAt, res.Status, res.Source, res.SourceRef, details,
		res.LastSyncedAt, res.Stale, res.CreatedAt, res.UpdatedAt)
	return err
}

func (r *PostgresRepository) UpdateReservation(ctx context.Context, res *domain.Reservation) error {
	const q = `UPDATE itinerary.reservations SET
		trip_id=$3, reservation_type=$4, provider=$5, provider_locator=$6, title=$7, destination=$8,
		start_at=$9, end_at=$10, status=$11, source_ref=$12, details=$13, last_synced_at=$14, stale=$15, updated_at=$16
		WHERE id=$1 AND user_id=$2`
	details, err := json.Marshal(orEmptyMap(res.Details))
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, q, res.ID, res.UserID, res.TripID, res.Type, res.Provider, res.ProviderLocator,
		res.Title, res.Destination, res.StartAt, res.EndAt, res.Status, res.SourceRef, details,
		res.LastSyncedAt, res.Stale, res.UpdatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) GetReservation(ctx context.Context, userID, id string) (*domain.Reservation, error) {
	const q = selectReservation + ` WHERE id=$1 AND user_id=$2`
	return scanReservation(r.pool.QueryRow(ctx, q, id, userID))
}

func (r *PostgresRepository) FindByProviderLocator(ctx context.Context, userID, provider, locator string) (*domain.Reservation, error) {
	const q = selectReservation + ` WHERE user_id=$1 AND provider=$2 AND provider_locator=$3`
	return scanReservation(r.pool.QueryRow(ctx, q, userID, provider, locator))
}

func (r *PostgresRepository) ListReservationsByTrip(ctx context.Context, userID, tripID string) ([]domain.Reservation, error) {
	const q = selectReservation + ` WHERE user_id=$1 AND trip_id=$2 ORDER BY start_at ASC NULLS LAST, created_at ASC`
	rows, err := r.pool.Query(ctx, q, userID, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Reservation{}
	for rows.Next() {
		res, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *res)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) DeleteReservation(ctx context.Context, userID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM itinerary.reservations WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListReservationsForSync(ctx context.Context, limit int) ([]domain.Reservation, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = selectReservation + ` WHERE status='confirmed' AND provider <> '' AND provider_locator <> ''
		ORDER BY last_synced_at ASC NULLS FIRST LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Reservation{}
	for rows.Next() {
		res, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *res)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) AddChange(ctx context.Context, c *domain.ReservationChange) error {
	prev, err := json.Marshal(orEmptyMap(c.Previous))
	if err != nil {
		return err
	}
	cur, err := json.Marshal(orEmptyMap(c.Current))
	if err != nil {
		return err
	}
	const q = `INSERT INTO itinerary.reservation_changes (id, user_id, reservation_id, source, changed_fields, previous, current, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`
	_, err = r.pool.Exec(ctx, q, c.ID, c.UserID, c.ReservationID, c.Source, c.ChangedFields, prev, cur, c.CreatedAt)
	return err
}

func (r *PostgresRepository) ListChanges(ctx context.Context, userID, reservationID string) ([]domain.ReservationChange, error) {
	const q = `SELECT id, user_id, reservation_id, source, changed_fields, previous, current, created_at
		FROM itinerary.reservation_changes WHERE user_id=$1 AND reservation_id=$2 ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, q, userID, reservationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ReservationChange{}
	for rows.Next() {
		var c domain.ReservationChange
		var prev, cur []byte
		if err := rows.Scan(&c.ID, &c.UserID, &c.ReservationID, &c.Source, &c.ChangedFields, &prev, &cur, &c.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(prev, &c.Previous)
		_ = json.Unmarshal(cur, &c.Current)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) AddNotification(ctx context.Context, n *domain.Notification) error {
	const q = `INSERT INTO itinerary.notifications (id, user_id, notification_type, title, body, resource_type, resource_id, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`
	_, err := r.pool.Exec(ctx, q, n.ID, n.UserID, n.Type, n.Title, n.Body, n.ResourceType, n.ResourceID, n.CreatedAt)
	return err
}

func (r *PostgresRepository) ListNotifications(ctx context.Context, userID string, limit int) ([]domain.Notification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const q = `SELECT id, user_id, notification_type, title, body, resource_type, resource_id, read_at, created_at
		FROM itinerary.notifications WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`
	rows, err := r.pool.Query(ctx, q, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Notification{}
	for rows.Next() {
		var n domain.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.ResourceType, &n.ResourceID, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) MarkNotificationRead(ctx context.Context, userID, id string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE itinerary.notifications SET read_at=now() WHERE id=$1 AND user_id=$2 AND read_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) AddAudit(ctx context.Context, a *domain.AuditLog) error {
	details, err := json.Marshal(orEmptyMap(a.Details))
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO itinerary.audit_logs (id, user_id, action, resource, resource_id, ip, details, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, a.ID, nullableUUID(a.UserID), a.Action, a.Resource, a.ResourceID, a.IP, details, a.CreatedAt)
	return err
}

const selectReservation = `SELECT id, user_id, trip_id, reservation_type, provider, provider_locator, title, destination,
	start_at, end_at, status, source, source_ref, details, last_synced_at, stale, created_at, updated_at
	FROM itinerary.reservations`

type scanner interface {
	Scan(dest ...any) error
}

func scanReservation(row scanner) (*domain.Reservation, error) {
	var res domain.Reservation
	var details []byte
	err := row.Scan(&res.ID, &res.UserID, &res.TripID, &res.Type, &res.Provider, &res.ProviderLocator, &res.Title,
		&res.Destination, &res.StartAt, &res.EndAt, &res.Status, &res.Source, &res.SourceRef, &details,
		&res.LastSyncedAt, &res.Stale, &res.CreatedAt, &res.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(details) > 0 {
		_ = json.Unmarshal(details, &res.Details)
	}
	if res.Details == nil {
		res.Details = map[string]any{}
	}
	return &res, nil
}

func dateArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format("2006-01-02")
}

func normalizeDate(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return &u
}

func orEmptyMap(m map[string]any) map[string]any {
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

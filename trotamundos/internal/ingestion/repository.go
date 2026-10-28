package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"trotamundos/internal/domain"
)

type Repository interface {
	CreateAccount(ctx context.Context, a *domain.MailAccount) error
	GetAccount(ctx context.Context, id string) (*domain.MailAccount, error)
	ListAccounts(ctx context.Context, userID string) ([]domain.MailAccount, error)
	RevokeAccount(ctx context.Context, userID, id string, now time.Time) error
	UpdateLastScan(ctx context.Context, id string, at time.Time) error

	ListWhitelist(ctx context.Context, userID string) ([]domain.WhitelistEntry, error)
	AddWhitelist(ctx context.Context, w *domain.WhitelistEntry) error
	DeleteWhitelist(ctx context.Context, userID, id string) error

	ListFilters(ctx context.Context, userID string) ([]domain.MailFilter, error)
	AddFilter(ctx context.Context, f *domain.MailFilter) error
	UpdateFilter(ctx context.Context, f *domain.MailFilter) error
	DeleteFilter(ctx context.Context, userID, id string) error

	CreateEvent(ctx context.Context, e *domain.IngestionEvent) (created bool, err error)
	ClaimEvents(ctx context.Context, limit int) ([]domain.IngestionEvent, error)
	MarkProcessed(ctx context.Context, id, status string, at time.Time) error
	MarkError(ctx context.Context, id, status string, next time.Time, msg string) error
	RecoverStale(ctx context.Context, olderThan time.Time) error
	ListEventsByUser(ctx context.Context, userID string, limit int) ([]domain.IngestionEvent, error)
	CountPending(ctx context.Context) (int64, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateAccount(ctx context.Context, a *domain.MailAccount) error {
	const q = `INSERT INTO itinerary.mail_accounts (id, user_id, provider, email, scopes, token_cipher, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`
	_, err := r.pool.Exec(ctx, q, a.ID, a.UserID, a.Provider, a.Email, a.Scopes, a.TokenCipher, a.Status, a.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrConflict
		}
	}
	return err
}

func (r *PostgresRepository) GetAccount(ctx context.Context, id string) (*domain.MailAccount, error) {
	const q = `SELECT id, user_id, provider, email::text, scopes, token_cipher, status, last_scanned_at, created_at, revoked_at
		FROM itinerary.mail_accounts WHERE id=$1`
	return scanAccount(r.pool.QueryRow(ctx, q, id))
}

func (r *PostgresRepository) ListAccounts(ctx context.Context, userID string) ([]domain.MailAccount, error) {
	const q = `SELECT id, user_id, provider, email::text, scopes, token_cipher, status, last_scanned_at, created_at, revoked_at
		FROM itinerary.mail_accounts WHERE user_id=$1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MailAccount{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) RevokeAccount(ctx context.Context, userID, id string, now time.Time) error {
	tag, err := r.pool.Exec(ctx, `UPDATE itinerary.mail_accounts SET status='revoked', revoked_at=$3, token_cipher=''
		WHERE id=$1 AND user_id=$2 AND status='active'`, id, userID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateLastScan(ctx context.Context, id string, at time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE itinerary.mail_accounts SET last_scanned_at=$2 WHERE id=$1`, id, at)
	return err
}

func (r *PostgresRepository) ListWhitelist(ctx context.Context, userID string) ([]domain.WhitelistEntry, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, user_id, pattern FROM itinerary.mail_whitelist WHERE user_id=$1 ORDER BY pattern`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WhitelistEntry{}
	for rows.Next() {
		var w domain.WhitelistEntry
		if err := rows.Scan(&w.ID, &w.UserID, &w.Pattern); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) AddWhitelist(ctx context.Context, w *domain.WhitelistEntry) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO itinerary.mail_whitelist (id, user_id, pattern) VALUES ($1,$2,$3)`, w.ID, w.UserID, w.Pattern)
	return err
}

func (r *PostgresRepository) DeleteWhitelist(ctx context.Context, userID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM itinerary.mail_whitelist WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListFilters(ctx context.Context, userID string) ([]domain.MailFilter, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, user_id, field, operator, value, active FROM itinerary.mail_filters WHERE user_id=$1 ORDER BY field, value`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MailFilter{}
	for rows.Next() {
		var f domain.MailFilter
		if err := rows.Scan(&f.ID, &f.UserID, &f.Field, &f.Operator, &f.Value, &f.Active); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) AddFilter(ctx context.Context, f *domain.MailFilter) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO itinerary.mail_filters (id, user_id, field, operator, value, active) VALUES ($1,$2,$3,$4,$5,$6)`,
		f.ID, f.UserID, f.Field, f.Operator, f.Value, f.Active)
	return err
}

func (r *PostgresRepository) UpdateFilter(ctx context.Context, f *domain.MailFilter) error {
	tag, err := r.pool.Exec(ctx, `UPDATE itinerary.mail_filters SET field=$3, operator=$4, value=$5, active=$6 WHERE id=$1 AND user_id=$2`,
		f.ID, f.UserID, f.Field, f.Operator, f.Value, f.Active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) DeleteFilter(ctx context.Context, userID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM itinerary.mail_filters WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CreateEvent(ctx context.Context, e *domain.IngestionEvent) (bool, error) {
	payload, err := json.Marshal(orEmpty(e.Payload))
	if err != nil {
		return false, err
	}
	const q = `INSERT INTO itinerary.ingestion_events
		(id, user_id, mailbox_id, channel, message_id, payload, status, attempts, received_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,0,$8,$9)
		ON CONFLICT (user_id, message_id) DO NOTHING`
	tag, err := r.pool.Exec(ctx, q, e.ID, e.UserID, nullableUUID(e.MailboxID), e.Channel, e.MessageID, payload, e.Status, e.ReceivedAt, e.CreatedAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresRepository) ClaimEvents(ctx context.Context, limit int) ([]domain.IngestionEvent, error) {
	if limit <= 0 {
		limit = 10
	}
	const q = `WITH picked AS (
			SELECT id FROM itinerary.ingestion_events
			WHERE status='pending' AND next_attempt_at <= now()
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		), upd AS (
			UPDATE itinerary.ingestion_events e
			SET status='processing', attempts=e.attempts+1, processing_started_at=now()
			FROM picked WHERE e.id=picked.id
			RETURNING e.id, e.user_id, e.mailbox_id, e.channel, e.message_id, e.payload, e.status, e.attempts,
				e.last_error, e.received_at, e.processed_at, e.created_at
		)
		SELECT * FROM upd`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.IngestionEvent{}
	for rows.Next() {
		var e domain.IngestionEvent
		var mailbox *string
		var payload []byte
		if err := rows.Scan(&e.ID, &e.UserID, &mailbox, &e.Channel, &e.MessageID, &payload, &e.Status, &e.Attempts,
			&e.LastError, &e.ReceivedAt, &e.ProcessedAt, &e.CreatedAt); err != nil {
			return nil, err
		}
		if mailbox != nil {
			e.MailboxID = *mailbox
		}
		_ = json.Unmarshal(payload, &e.Payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) MarkProcessed(ctx context.Context, id, status string, at time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE itinerary.ingestion_events SET status=$2, processed_at=$3, last_error='' WHERE id=$1`, id, status, at)
	return err
}

func (r *PostgresRepository) MarkError(ctx context.Context, id, status string, next time.Time, msg string) error {
	_, err := r.pool.Exec(ctx, `UPDATE itinerary.ingestion_events SET status=$2, next_attempt_at=$3, last_error=$4 WHERE id=$1`, id, status, next, msg)
	return err
}

func (r *PostgresRepository) RecoverStale(ctx context.Context, olderThan time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE itinerary.ingestion_events SET status='pending', next_attempt_at=now()
		WHERE status='processing' AND processing_started_at < $1`, olderThan)
	return err
}

func (r *PostgresRepository) ListEventsByUser(ctx context.Context, userID string, limit int) ([]domain.IngestionEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const q = `SELECT id, user_id, mailbox_id, channel, message_id, payload, status, attempts, last_error,
		received_at, processed_at, created_at
		FROM itinerary.ingestion_events WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`
	rows, err := r.pool.Query(ctx, q, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.IngestionEvent{}
	for rows.Next() {
		var e domain.IngestionEvent
		var mailbox *string
		var payload []byte
		if err := rows.Scan(&e.ID, &e.UserID, &mailbox, &e.Channel, &e.MessageID, &payload, &e.Status, &e.Attempts,
			&e.LastError, &e.ReceivedAt, &e.ProcessedAt, &e.CreatedAt); err != nil {
			return nil, err
		}
		if mailbox != nil {
			e.MailboxID = *mailbox
		}
		_ = json.Unmarshal(payload, &e.Payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CountPending(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM itinerary.ingestion_events WHERE status IN ('pending','processing')`).Scan(&n)
	return n, err
}

func scanAccount(row interface{ Scan(dest ...any) error }) (*domain.MailAccount, error) {
	var a domain.MailAccount
	if err := row.Scan(&a.ID, &a.UserID, &a.Provider, &a.Email, &a.Scopes, &a.TokenCipher, &a.Status, &a.LastScannedAt, &a.CreatedAt, &a.RevokedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &a, nil
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

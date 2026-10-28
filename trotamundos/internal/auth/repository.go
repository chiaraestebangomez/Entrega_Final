package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"trotamundos/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, u *domain.User) error
	ByEmail(ctx context.Context, email string) (*domain.User, error)
	ByID(ctx context.Context, id string) (*domain.User, error)
	Upsert(ctx context.Context, u *domain.User) (*domain.User, error)
}

type PostgresUsers struct {
	pool *pgxpool.Pool
}

func NewPostgresUsers(pool *pgxpool.Pool) *PostgresUsers { return &PostgresUsers{pool: pool} }

func (r *PostgresUsers) Create(ctx context.Context, u *domain.User) error {
	const q = `INSERT INTO auth.users (id, email, full_name, password_hash, role, locale, region, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING created_at, updated_at`
	err := r.pool.QueryRow(ctx, q, u.ID, strings.ToLower(u.Email), u.FullName, u.PasswordHash, u.Role, u.Locale, u.Region, u.CreatedAt, u.UpdatedAt).Scan(&u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrConflict
		}
		return err
	}
	return nil
}

func (r *PostgresUsers) ByEmail(ctx context.Context, email string) (*domain.User, error) {
	const q = `SELECT id, email::text, full_name, password_hash, role, locale, region, created_at, updated_at
		FROM auth.users WHERE email = $1`
	return scanUser(r.pool.QueryRow(ctx, q, strings.ToLower(strings.TrimSpace(email))))
}

func (r *PostgresUsers) ByID(ctx context.Context, id string) (*domain.User, error) {
	const q = `SELECT id, email::text, full_name, password_hash, role, locale, region, created_at, updated_at
		FROM auth.users WHERE id = $1`
	return scanUser(r.pool.QueryRow(ctx, q, id))
}

func (r *PostgresUsers) Upsert(ctx context.Context, u *domain.User) (*domain.User, error) {
	const q = `INSERT INTO auth.users (id, email, full_name, password_hash, role, locale, region, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (email) DO UPDATE SET full_name = EXCLUDED.full_name, role = EXCLUDED.role, updated_at = now()
		RETURNING id, email::text, full_name, password_hash, role, locale, region, created_at, updated_at`
	return scanUser(r.pool.QueryRow(ctx, q, u.ID, strings.ToLower(u.Email), u.FullName, u.PasswordHash, u.Role, u.Locale, u.Region, u.CreatedAt, u.UpdatedAt))
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*domain.User, error) {
	var u domain.User
	var created, updated time.Time
	if err := row.Scan(&u.ID, &u.Email, &u.FullName, &u.PasswordHash, &u.Role, &u.Locale, &u.Region, &created, &updated); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	u.CreatedAt, u.UpdatedAt = created, updated
	return &u, nil
}

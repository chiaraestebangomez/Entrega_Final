package pgxdb

import (
	"context"
	"embed"
	"fmt"
	"hash/fnv"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Shards struct {
	pools []*pgxpool.Pool
}

func NewShards(ctx context.Context, dsns []string) (*Shards, error) {
	if len(dsns) == 0 {
		return nil, fmt.Errorf("pgxdb: no shards")
	}
	s := &Shards{}
	for _, dsn := range dsns {
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("pgxdb: parse dsn: %w", err)
		}
		cfg.MaxConns = 20
		cfg.MinConns = 1
		cfg.MaxConnLifetime = 30 * time.Minute
		cfg.MaxConnIdleTime = 5 * time.Minute
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("pgxdb: connect: %w", err)
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			s.Close()
			return nil, fmt.Errorf("pgxdb: ping: %w", err)
		}
		s.pools = append(s.pools, pool)
	}
	return s, nil
}

func (s *Shards) PoolFor(userID string) *pgxpool.Pool {
	h := fnv.New32a()
	_, _ = h.Write([]byte(userID))
	return s.pools[int(h.Sum32())%len(s.pools)]
}

func (s *Shards) Directory() *pgxpool.Pool { return s.pools[0] }

func (s *Shards) Pools() []*pgxpool.Pool { return s.pools }

func (s *Shards) Migrate(ctx context.Context) error {
	for _, pool := range s.pools {
		if err := MigratePool(ctx, pool); err != nil {
			return err
		}
	}
	return nil
}

func (s *Shards) Close() {
	for _, p := range s.pools {
		if p != nil {
			p.Close()
		}
	}
}

func OpenReadOnly(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns <= 0 {
		maxConns = 10
	}
	cfg.MaxConns = maxConns
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET default_transaction_read_only = on")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func MigratePool(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS platform`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS platform.schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var applied bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.schema_migrations WHERE version=$1)`, name).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		for _, stmt := range splitStatements(string(body)) {
			if _, err := pool.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
		}
		if _, err := pool.Exec(ctx, `INSERT INTO platform.schema_migrations(version) VALUES($1)`, name); err != nil {
			return err
		}
	}
	return nil
}

func splitStatements(sql string) []string {
	parts := strings.Split(sql, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

package analytics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/httpx"
	"trotamundos/internal/platform/idgen"
)

type Repository interface {
	Publish(ctx context.Context, fact domain.AnalyticsFact) error
	Summary(ctx context.Context, from, to time.Time) (domain.AnalyticsSummary, error)
	ListFacts(ctx context.Context, from, to time.Time, limit int) ([]domain.AnalyticsFact, error)
}

type PostgresRepository struct {
	write *pgxpool.Pool
	read  *pgxpool.Pool
}

func NewPostgresRepository(write, read *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{write: write, read: read}
}

func (r *PostgresRepository) Publish(ctx context.Context, f domain.AnalyticsFact) error {
	const q = `INSERT INTO analytics.reservation_facts
		(id, destination, start_date, end_date, status, provider, fact_type, agency_id, occurred_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`
	_, err := r.write.Exec(ctx, q, f.ID, f.Destination, dateArg(f.StartDate), dateArg(f.EndDate), f.Status,
		f.Provider, f.Type, nullableUUID(f.AgencyID), f.OccurredAt, f.CreatedAt)
	return err
}

func (r *PostgresRepository) Summary(ctx context.Context, from, to time.Time) (domain.AnalyticsSummary, error) {
	out := domain.AnalyticsSummary{From: from, To: to, ByType: []domain.AnalyticsBucket{}, ByProvider: []domain.AnalyticsBucket{}, TopDestinations: []domain.AnalyticsBucket{}, Monthly: []domain.AnalyticsBucket{}}
	const totals = `SELECT count(*),
		count(*) FILTER (WHERE status='canceled'),
		count(DISTINCT (destination, start_date))
		FROM analytics.reservation_facts WHERE occurred_at >= $1 AND occurred_at < $2`
	if err := r.read.QueryRow(ctx, totals, from, to).Scan(&out.Reservations, &out.Canceled, &out.Trips); err != nil {
		return out, err
	}
	var err error
	if out.ByType, err = r.buckets(ctx, `SELECT fact_type, count(*) FROM analytics.reservation_facts
		WHERE occurred_at >= $1 AND occurred_at < $2 GROUP BY fact_type ORDER BY count(*) DESC`, from, to); err != nil {
		return out, err
	}
	if out.ByProvider, err = r.buckets(ctx, `SELECT provider, count(*) FROM analytics.reservation_facts
		WHERE occurred_at >= $1 AND occurred_at < $2 GROUP BY provider ORDER BY count(*) DESC`, from, to); err != nil {
		return out, err
	}
	if out.TopDestinations, err = r.buckets(ctx, `SELECT destination, count(*) FROM analytics.reservation_facts
		WHERE occurred_at >= $1 AND occurred_at < $2 AND destination <> ''
		GROUP BY destination ORDER BY count(*) DESC LIMIT 10`, from, to); err != nil {
		return out, err
	}
	if out.Monthly, err = r.buckets(ctx, `SELECT to_char(date_trunc('month', occurred_at), 'YYYY-MM'), count(*)
		FROM analytics.reservation_facts WHERE occurred_at >= $1 AND occurred_at < $2
		GROUP BY 1 ORDER BY 1 ASC`, from, to); err != nil {
		return out, err
	}
	return out, nil
}

func (r *PostgresRepository) buckets(ctx context.Context, q string, from, to time.Time) ([]domain.AnalyticsBucket, error) {
	rows, err := r.read.Query(ctx, q, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AnalyticsBucket{}
	for rows.Next() {
		var b domain.AnalyticsBucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListFacts(ctx context.Context, from, to time.Time, limit int) ([]domain.AnalyticsFact, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	const q = `SELECT id, destination, start_date, end_date, status, provider, fact_type, COALESCE(agency_id::text,''), occurred_at, created_at
		FROM analytics.reservation_facts WHERE occurred_at >= $1 AND occurred_at < $2 ORDER BY occurred_at DESC LIMIT $3`
	rows, err := r.read.Query(ctx, q, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AnalyticsFact{}
	for rows.Next() {
		var f domain.AnalyticsFact
		if err := rows.Scan(&f.ID, &f.Destination, &f.StartDate, &f.EndDate, &f.Status, &f.Provider, &f.Type, &f.AgencyID, &f.OccurredAt, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service { return &Service{repo: repo, now: time.Now} }

func (s *Service) PublishReservation(ctx context.Context, res *domain.Reservation) error {
	if res == nil {
		return nil
	}
	now := s.now().UTC()
	f := domain.AnalyticsFact{
		ID:          idgen.UUID(),
		Destination: res.Destination,
		StartDate:   dateOnly(res.StartAt),
		EndDate:     dateOnly(res.EndAt),
		Status:      res.Status,
		Provider:    res.Provider,
		Type:        res.Type,
		OccurredAt:  now,
		CreatedAt:   now,
	}
	return s.repo.Publish(ctx, f)
}

func (s *Service) Summary(ctx context.Context, from, to time.Time) (domain.AnalyticsSummary, error) {
	return s.repo.Summary(ctx, from, to)
}

func (s *Service) Facts(ctx context.Context, from, to time.Time, limit int) ([]domain.AnalyticsFact, error) {
	return s.repo.ListFacts(ctx, from, to, limit)
}

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	from, to, err := rangeParams(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}
	out, err := h.svc.Summary(r.Context(), from, to)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "no se pudo generar el reporte")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) Facts(w http.ResponseWriter, r *http.Request) {
	from, to, err := rangeParams(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}
	out, err := h.svc.Facts(r.Context(), from, to, 100)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "no se pudieron obtener los datos")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func rangeParams(r *http.Request) (time.Time, time.Time, error) {
	to := time.Now().UTC()
	from := to.AddDate(0, -3, 0)
	if v := r.URL.Query().Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return from, to, fmt.Errorf("parámetro 'to' inválido, se espera RFC3339")
		}
		to = t
	}
	if v := r.URL.Query().Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return from, to, fmt.Errorf("parámetro 'from' inválido, se espera RFC3339")
		}
		from = t
	}
	if !from.Before(to) {
		return from, to, fmt.Errorf("'from' debe ser anterior a 'to'")
	}
	return from, to, nil
}

func dateArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format("2006-01-02")
}

func dateOnly(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	d := t.UTC()
	day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	return &day
}

func nullableUUID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

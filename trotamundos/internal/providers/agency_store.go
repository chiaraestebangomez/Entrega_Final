package providers

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"trotamundos/internal/domain"
)

type PostgresAgencyCatalog struct {
	pool *pgxpool.Pool
}

func NewPostgresAgencyCatalog(pool *pgxpool.Pool) *PostgresAgencyCatalog {
	return &PostgresAgencyCatalog{pool: pool}
}

func (s *PostgresAgencyCatalog) Catalog(ctx context.Context, agencyID string) ([]domain.AgencyCatalogItem, error) {
	const q = `SELECT r.id, r.agency_id, a.name, r.title, r.destination, r.description,
		r.price_cents, r.currency, r.start_date, r.end_date
		FROM itinerary.agency_recommendations r JOIN itinerary.agencies a ON a.id=r.agency_id
		WHERE r.agency_id=$1 AND r.active
		ORDER BY r.start_date NULLS LAST, r.title`
	rows, err := s.pool.Query(ctx, q, agencyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AgencyCatalogItem{}
	for rows.Next() {
		var item domain.AgencyCatalogItem
		if err := rows.Scan(&item.ID, &item.AgencyID, &item.AgencyName, &item.Title, &item.Destination,
			&item.Description, &item.PriceCents, &item.Currency, &item.StartDate, &item.EndDate); err != nil {
			return nil, err
		}
		item.Type = CatalogType(item.Title + " " + item.Description)
		out = append(out, item)
	}
	return out, rows.Err()
}

func CatalogType(text string) string {
	text = strings.ToLower(text)
	switch {
	case strings.Contains(text, "hotel"), strings.Contains(text, "alojamiento"), strings.Contains(text, "noche"),
		strings.Contains(text, "resort"), strings.Contains(text, "cabaña"):
		return domain.RecoTypeLodging
	case strings.Contains(text, "vuelo"), strings.Contains(text, "flight"), strings.Contains(text, "aéreo"),
		strings.Contains(text, "auto"), strings.Contains(text, "car"), strings.Contains(text, "tren"),
		strings.Contains(text, "transporte"), strings.Contains(text, "crucero"):
		return domain.RecoTypeTransport
	case strings.Contains(text, "ruta"), strings.Contains(text, "trekking"), strings.Contains(text, "excursión"),
		strings.Contains(text, "circuito"), strings.Contains(text, "visita"):
		return domain.RecoTypeRoute
	default:
		return domain.RecoTypePackage
	}
}

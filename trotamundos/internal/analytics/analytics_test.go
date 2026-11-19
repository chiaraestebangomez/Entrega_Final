package analytics

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"trotamundos/internal/domain"
)

type fakeRepo struct {
	published []domain.AnalyticsFact
}

func (f *fakeRepo) Publish(_ context.Context, fact domain.AnalyticsFact) error {
	f.published = append(f.published, fact)
	return nil
}

func (f *fakeRepo) Summary(_ context.Context, from, to time.Time) (domain.AnalyticsSummary, error) {
	return domain.AnalyticsSummary{From: from, To: to}, nil
}

func (f *fakeRepo) ListFacts(_ context.Context, _, _ time.Time, _ int) ([]domain.AnalyticsFact, error) {
	return f.published, nil
}

func TestPublishReservationIsAnonymized(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	start := time.Date(2026, 12, 20, 8, 30, 0, 0, time.UTC)
	res := &domain.Reservation{
		ID: "r1", UserID: "user-1", Type: domain.TypeFlight, Provider: "aerolineas",
		Destination: "Bariloche", StartAt: &start, Status: domain.ReservationConfirmed,
		ProviderLocator: "AR7X2K", SourceRef: "email:abc",
	}
	if err := svc.PublishReservation(context.Background(), res); err != nil {
		t.Fatal(err)
	}
	if len(repo.published) != 1 {
		t.Fatalf("facts = %d", len(repo.published))
	}
	f := repo.published[0]
	if f.Destination != "Bariloche" || f.Provider != "aerolineas" || f.Type != domain.TypeFlight {
		t.Fatalf("fact inesperado: %+v", f)
	}
	if f.StartDate == nil || f.StartDate.Hour() != 0 || f.StartDate.Minute() != 0 {
		t.Fatalf("la fecha debe anonimizarse a día: %v", f.StartDate)
	}
}

func TestPublishNilReservation(t *testing.T) {
	svc := NewService(&fakeRepo{})
	if err := svc.PublishReservation(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestRangeParamsAcceptsDateAndRFC3339(t *testing.T) {
	cases := []struct {
		name, from, to string
	}{
		{"date", "2026-12-20", "2026-12-25"},
		{"rfc3339", "2026-12-20T08:30:00Z", "2026-12-25T20:10:00-03:00"},
		{"mixed", "2026-12-20T08:30:00Z", "2026-12-25"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/analytics/summary?from="+c.from+"&to="+c.to, nil)
			from, to, err := rangeParams(req)
			if err != nil {
				t.Fatal(err)
			}
			if from.Hour() != 0 || from.Minute() != 0 || from.Second() != 0 || from.Nanosecond() != 0 {
				t.Fatalf("from no normalizado: %v", from)
			}
			if to.Hour() != 23 || to.Minute() != 59 || to.Second() != 59 || to.Nanosecond() != 999000000 {
				t.Fatalf("to no normalizado: %v", to)
			}
			if from.Format(dateLayout) != "2026-12-20" || to.Format(dateLayout) != "2026-12-25" {
				t.Fatalf("rango = %v..%v", from, to)
			}
		})
	}
}

func TestRangeParamsRejectsInvalidValues(t *testing.T) {
	for _, query := range []string{"from=2026-13-01", "to=no-valido", "from=2026-12-25&to=2026-12-20"} {
		req := httptest.NewRequest("GET", "/analytics/summary?"+query, nil)
		if _, _, err := rangeParams(req); err == nil {
			t.Errorf("esperaba error para %q", query)
		}
	}
}

package analytics

import (
	"context"
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

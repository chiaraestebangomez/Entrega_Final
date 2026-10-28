package social

import (
	"context"
	"strings"
	"testing"
	"time"

	"trotamundos/internal/domain"
)

type fakeTrips struct{ trip *domain.Trip }

func (f *fakeTrips) GetTrip(_ context.Context, userID, tripID string) (*domain.Trip, error) {
	if f.trip == nil || f.trip.UserID != userID || f.trip.ID != tripID {
		return nil, domain.ErrNotFound
	}
	return f.trip, nil
}

func TestPreviewOmitsSensitiveData(t *testing.T) {
	start := time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 27, 0, 0, 0, 0, time.UTC)
	trip := &domain.Trip{
		ID: "trip-1", UserID: "user-1", Name: "Vacaciones familia Pérez",
		Destination: "Bariloche", StartDate: &start, EndDate: &end, Status: domain.TripActive,
		Reservations: []domain.Reservation{
			{Type: domain.TypeFlight, Status: domain.ReservationConfirmed, ProviderLocator: "AR7X2K"},
			{Type: domain.TypeHotel, Status: domain.ReservationConfirmed, ProviderLocator: "HT-9988"},
			{Type: domain.TypeCar, Status: domain.ReservationCanceled, ProviderLocator: "CAR-1"},
		},
	}
	svc := NewService(&fakeTrips{trip: trip})
	preview, err := svc.BuildPreview(context.Background(), "user-1", "trip-1", "x")
	if err != nil {
		t.Fatal(err)
	}
	if preview.ContainsPII {
		t.Fatal("el resumen no debe contener PII")
	}
	for _, forbidden := range []string{"AR7X2K", "HT-9988", "Pérez", "user-1"} {
		if strings.Contains(preview.Text, forbidden) {
			t.Fatalf("el texto contiene dato sensible %q: %s", forbidden, preview.Text)
		}
	}
	if !strings.Contains(preview.Text, "Bariloche") || !strings.Contains(preview.Text, "2 reservas confirmadas") {
		t.Fatalf("texto inesperado: %s", preview.Text)
	}
	if preview.Reservations != 2 {
		t.Fatalf("reservas confirmadas = %d", preview.Reservations)
	}
}

func TestPublishRejectsUnknownNetwork(t *testing.T) {
	svc := NewService(&fakeTrips{})
	if _, err := svc.Publish(context.Background(), "u", "t", "myspace"); err == nil {
		t.Fatal("esperaba error para red no soportada")
	}
}

func TestPublishStoresMockPost(t *testing.T) {
	start := time.Date(2027, 1, 10, 0, 0, 0, 0, time.UTC)
	trip := &domain.Trip{ID: "t", UserID: "u", Destination: "Mendoza", StartDate: &start, Status: domain.TripActive}
	svc := NewService(&fakeTrips{trip: trip})
	post, err := svc.Publish(context.Background(), "u", "t", "linkedin")
	if err != nil {
		t.Fatal(err)
	}
	if post.URL == "" || post.Network != "linkedin" {
		t.Fatalf("post inesperado: %+v", post)
	}
	if got := svc.Posts(); len(got) != 1 {
		t.Fatalf("posts = %d", len(got))
	}
}

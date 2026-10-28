package itinerary

import (
	"testing"
	"time"

	"trotamundos/internal/domain"
)

func dt(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

func TestSameDestination(t *testing.T) {
	if !SameDestination("Bariloche", "bariloche") {
		t.Fatal("debe ignorar mayúsculas")
	}
	if !SameDestination("San Carlos de Bariloche", "Bariloche") {
		t.Fatal("debe aceptar destinos contenidos")
	}
	if SameDestination("Bariloche", "Mendoza") {
		t.Fatal("destinos distintos no deben coincidir")
	}
}

func TestDatesCompatible(t *testing.T) {
	start, end := dt(2026, 12, 20), dt(2026, 12, 25)
	if !DatesCompatible(start, end, dt(2026, 12, 22), dt(2026, 12, 23), GroupWindowDays) {
		t.Fatal("fechas dentro del viaje deben ser compatibles")
	}
	if !DatesCompatible(start, end, dt(2026, 12, 27), nil, GroupWindowDays) {
		t.Fatal("fechas dentro de la ventana deben ser compatibles")
	}
	if DatesCompatible(start, end, dt(2027, 2, 1), nil, GroupWindowDays) {
		t.Fatal("fechas fuera de la ventana no deben ser compatibles")
	}
	if !DatesCompatible(start, end, nil, nil, GroupWindowDays) {
		t.Fatal("sin fecha de reserva debe ser compatible")
	}
}

func TestBestTripPicksClosest(t *testing.T) {
	trips := []domain.Trip{
		{ID: "far", Destination: "Bariloche", Status: domain.TripActive, StartDate: dt(2026, 12, 20), EndDate: dt(2026, 12, 25)},
		{ID: "near", Destination: "Bariloche", Status: domain.TripActive, StartDate: dt(2027, 1, 8), EndDate: dt(2027, 1, 15)},
		{ID: "other", Destination: "Mendoza", Status: domain.TripActive, StartDate: dt(2027, 1, 8)},
		{ID: "archived", Destination: "Bariloche", Status: domain.TripArchived, StartDate: dt(2027, 1, 8)},
	}
	best := BestTrip(trips, "Bariloche", dt(2027, 1, 9), dt(2027, 1, 12))
	if best == nil || best.ID != "near" {
		t.Fatalf("mejor viaje = %+v", best)
	}
}

func TestTripBounds(t *testing.T) {
	start1, end1 := dt(2026, 12, 20), dt(2026, 12, 21)
	start2, end2 := dt(2026, 12, 18), dt(2026, 12, 27)
	min, max := TripBounds([]domain.Reservation{
		{StartAt: start1, EndAt: end1},
		{StartAt: start2, EndAt: end2},
	})
	if min == nil || !min.Equal(*start2) {
		t.Fatalf("min = %v", min)
	}
	if max == nil || !max.Equal(*end2) {
		t.Fatalf("max = %v", max)
	}
}

func TestTripName(t *testing.T) {
	if got := TripName("Bariloche", "Vuelo"); got != "Bariloche" {
		t.Fatalf("TripName = %q", got)
	}
	if got := TripName("", "Vuelo"); got != "Vuelo" {
		t.Fatalf("TripName = %q", got)
	}
	if got := TripName("", ""); got != "Viaje" {
		t.Fatalf("TripName = %q", got)
	}
}

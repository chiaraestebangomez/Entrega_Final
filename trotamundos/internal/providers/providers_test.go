package providers

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMockRegistryReturnsSeededSnapshot(t *testing.T) {
	store := NewMockStore()
	reg := NewMockRegistry(store)
	start := time.Date(2026, 12, 20, 8, 0, 0, 0, time.UTC)
	reg.Seed("aerolineas", "AR7X2K", Snapshot{
		Type: "flight", Title: "Vuelo a Bariloche", Destination: "Bariloche",
		StartAt: &start, Status: "confirmed",
	})
	adapter, ok := reg.Get("Aerolineas")
	if !ok {
		t.Fatal("adapter no encontrado")
	}
	snap, err := adapter.GetReservation(context.Background(), "AR7X2K")
	if err != nil {
		t.Fatalf("GetReservation: %v", err)
	}
	if snap.Provider != "aerolineas" || snap.Locator != "AR7X2K" || snap.Destination != "Bariloche" {
		t.Fatalf("snapshot inesperado: %+v", snap)
	}
}

func TestMockAdapterNotFoundAndCancel(t *testing.T) {
	store := NewMockStore()
	reg := NewMockRegistry(store)
	adapter, _ := reg.Get("hotel")
	if _, err := adapter.GetReservation(context.Background(), "NOPE"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, obtuve %v", err)
	}
	reg.Seed("hotel", "HT1", Snapshot{Type: "hotel", Status: "confirmed"})
	if !reg.Cancel("hotel", "HT1") {
		t.Fatal("Cancel debía encontrar la reserva")
	}
	snap, err := adapter.GetReservation(context.Background(), "HT1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Status != "canceled" {
		t.Fatalf("estado = %q", snap.Status)
	}
}

package ingestion

import (
	"errors"
	"strings"
	"testing"
	"time"

	"trotamundos/internal/mailer"
)

func TestParseMessageKeyValueFlight(t *testing.T) {
	msg := mailer.Message{
		ID:      "m1",
		From:    "reservas@aerolineas.com",
		Subject: "Tu reserva",
		Body: `Provider: aerolineas
Locator: AR7X2K
Type: flight
Title: Vuelo Buenos Aires - Bariloche
Destination: Bariloche
Start: 2026-12-20T08:00:00Z
End: 2026-12-20T10:30:00Z
Details: equipaje incluido`,
		ReceivedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
	}
	p, err := ParseMessage(msg)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if p.Type != "flight" || p.Provider != "aerolineas" || p.Locator != "AR7X2K" {
		t.Fatalf("unexpected parsed values: %+v", p)
	}
	if p.Destination != "Bariloche" || p.StartAt == nil || p.EndAt == nil {
		t.Fatalf("missing dates/destination: %+v", p)
	}
	if p.StartAt.Hour() != 8 || p.EndAt.Hour() != 10 {
		t.Fatalf("unexpected times: %v %v", p.StartAt, p.EndAt)
	}
}

func TestParseMessageJSON(t *testing.T) {
	msg := mailer.Message{
		ID:         "m2",
		From:       "hotel@example.com",
		Subject:    "Confirmación",
		Body:       `{"provider":"hotel","locator":"HT-99","type":"hotel","title":"Hotel Centro","destination":"Mendoza","start":"2027-01-10","end":"2027-01-14"}`,
		ReceivedAt: time.Now(),
	}
	p, err := ParseMessage(msg)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if p.Type != "hotel" || p.Provider != "hotel" || p.Locator != "HT-99" || p.Destination != "Mendoza" {
		t.Fatalf("unexpected parsed values: %+v", p)
	}
}

func TestParseMessageUnparseable(t *testing.T) {
	_, err := ParseMessage(mailer.Message{})
	if !errors.Is(err, ErrUnparseable) {
		t.Fatalf("expected ErrUnparseable, got %v", err)
	}
}

func TestParseMessageInfersProviderAndDestination(t *testing.T) {
	msg := mailer.Message{
		From: "no-reply@booking.com", Subject: "Reserva confirmada",
		Body: "Locator: BK1234\nDestination: Ushuaia\nHotel en Ushuaia del 2026-11-02 al 2026-11-06",
	}
	p, err := ParseMessage(msg)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if !strings.EqualFold(p.Provider, "booking") {
		t.Fatalf("provider inferred as %q", p.Provider)
	}
	if p.Destination == "" {
		t.Fatalf("destination not inferred: %+v", p)
	}
}

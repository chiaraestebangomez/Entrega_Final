package ingestion

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/itinerary"
	"trotamundos/internal/mailer"
)

type privacyRepo struct {
	Repository
	event     *domain.IngestionEvent
	processed bool
}

func (r *privacyRepo) ListWhitelist(context.Context, string) ([]domain.WhitelistEntry, error) {
	return nil, nil
}

func (r *privacyRepo) ListFilters(context.Context, string) ([]domain.MailFilter, error) {
	return nil, nil
}

func (r *privacyRepo) CreateEvent(_ context.Context, e *domain.IngestionEvent) (bool, error) {
	copy := *e
	r.event = &copy
	return true, nil
}

func (r *privacyRepo) MarkProcessed(context.Context, string, string, time.Time) error {
	r.processed = true
	return nil
}

type captureApplier struct {
	in itinerary.ReservationInput
}

func (a *captureApplier) ApplyReservation(_ context.Context, _ string, in itinerary.ReservationInput, _, _ string) (*domain.Reservation, bool, error) {
	a.in = in
	return &domain.Reservation{}, true, nil
}

func TestEnqueueDoesNotPersistEmailBody(t *testing.T) {
	repo := &privacyRepo{}
	svc := NewService(repo, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	acc := &domain.MailAccount{ID: "mailbox-1", UserID: "user-1"}
	msg := mailer.Message{
		ID: "message-1", From: "booking@proveedor.com", Subject: "Vuelo a Bariloche ABC123",
		Body: `type: flight
provider: aerolinea
locator: ABC123
destination: Bariloche
start: 2027-01-03
end: 2027-01-10
contenido confidencial que no debe persistirse en el outbox`,
		ReceivedAt: time.Now().UTC(),
	}

	if _, _, err := svc.enqueue(context.Background(), acc, "webhook", msg); err != nil {
		t.Fatal(err)
	}
	payload := repo.event.Payload
	if _, ok := payload["body"]; ok {
		t.Fatalf("el outbox no debe guardar el cuerpo del correo: %+v", payload)
	}
	parsed, ok := payload["parsed"].(map[string]any)
	if !ok {
		t.Fatalf("falta la reserva canónica extraída: %+v", payload)
	}
	if parsed["locator"] != "ABC123" || parsed["destination"] != "Bariloche" {
		t.Fatalf("datos canónicos incorrectos: %+v", parsed)
	}
	if accepted, ok := payload["accepted"].(bool); !ok || !accepted {
		t.Fatalf("falta el resultado del filtrado: %+v", payload)
	}
}

func TestProcessEventUsesCanonicalPayloadWithoutBody(t *testing.T) {
	repo := &privacyRepo{}
	applier := &captureApplier{}
	svc := NewService(repo, applier, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ev := domain.IngestionEvent{
		ID: "event-1", UserID: "user-1", MessageID: "message-1",
		Payload: map[string]any{
			"accepted": true,
			"parsed": map[string]any{
				"type": "flight", "provider": "aerolinea", "locator": "ABC123",
				"destination": "Bariloche", "start_at": "2027-01-03T00:00:00Z",
				"end_at": "2027-01-10T00:00:00Z", "details": map[string]any{},
			},
		},
	}

	if err := svc.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if !repo.processed || applier.in.ProviderLocator != "ABC123" {
		t.Fatalf("processed=%v reserva=%+v", repo.processed, applier.in)
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(applier.in.Title)), "confidencial") {
		t.Fatalf("el procesamiento reintrodujo contenido del correo: %+v", applier.in)
	}
}

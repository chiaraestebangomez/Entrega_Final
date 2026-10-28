package mailer

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/idgen"
)

type Message struct {
	ID         string    `json:"id"`
	From       string    `json:"from"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	ReceivedAt time.Time `json:"received_at"`
}

type Connector interface {
	Name() string
	FetchNew(ctx context.Context, account domain.MailAccount) ([]Message, error)
}

type MockConnector struct {
	mu    sync.Mutex
	inbox map[string][]Message
	now   func() time.Time
}

func NewMockConnector() *MockConnector {
	return &MockConnector{inbox: map[string][]Message{}, now: time.Now}
}

func (m *MockConnector) Name() string { return "mock" }

func (m *MockConnector) FetchNew(_ context.Context, account domain.MailAccount) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(account.Email)
	msgs, ok := m.inbox[key]
	if !ok {
		msgs = sampleMessages(m.now().UTC())
		m.inbox[key] = msgs
	}
	out := make([]Message, len(msgs))
	copy(out, msgs)
	sort.Slice(out, func(i, j int) bool { return out[i].ReceivedAt.Before(out[j].ReceivedAt) })
	return out, nil
}

func (m *MockConnector) Add(email string, msg Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(strings.TrimSpace(email))
	if msg.ID == "" {
		msg.ID = idgen.UUID()
	}
	if msg.ReceivedAt.IsZero() {
		msg.ReceivedAt = m.now().UTC()
	}
	if _, ok := m.inbox[key]; !ok {
		m.inbox[key] = sampleMessages(m.now().UTC())
	}
	m.inbox[key] = append(m.inbox[key], msg)
}

func sampleMessages(now time.Time) []Message {
	start := now.AddDate(0, 0, 45).Truncate(time.Minute)
	end := start.Add(2*time.Hour + 30*time.Minute)
	hotelEnd := start.AddDate(0, 0, 5)

	flight := fmt.Sprintf(`Provider: aerolineas
Locator: AR7X2K
Type: flight
Title: Vuelo Buenos Aires - Bariloche
Destination: Bariloche
Start: %s
End: %s
Details: vuelo directo, equipaje incluido`, start.Format(time.RFC3339), end.Format(time.RFC3339))

	hotel := fmt.Sprintf(`Provider: hotelplaza
Locator: HTL998
Type: hotel
Title: Hotel Plaza Bariloche
Destination: Bariloche
Start: %s
End: %s
Details: habitación doble con desayuno`, start.Format(time.RFC3339), hotelEnd.Format(time.RFC3339))

	return []Message{
		{ID: "mock-flight-" + start.Format("20060102"), From: "reservas@aerolineas.example", Subject: "Tu vuelo a Bariloche está confirmado", Body: flight, ReceivedAt: now},
		{ID: "mock-hotel-" + start.Format("20060102"), From: "booking@hotelplaza.example", Subject: "Confirmación de hotel en Bariloche", Body: hotel, ReceivedAt: now.Add(time.Second)},
	}
}

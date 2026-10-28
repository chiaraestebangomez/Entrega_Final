package social

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/idgen"
)

var allowedNetworks = map[string]string{
	"x": "X (Twitter)", "twitter": "X (Twitter)", "facebook": "Facebook",
	"linkedin": "LinkedIn", "instagram": "Instagram", "threads": "Threads",
}

type TripReader interface {
	GetTrip(ctx context.Context, userID, tripID string) (*domain.Trip, error)
}

type Preview struct {
	Network      string         `json:"network"`
	NetworkName  string         `json:"network_name"`
	TripID       string         `json:"trip_id"`
	Text         string         `json:"text"`
	Destination  string         `json:"destination"`
	StartDate    *time.Time     `json:"start_date,omitempty"`
	EndDate      *time.Time     `json:"end_date,omitempty"`
	Reservations int            `json:"reservations"`
	Types        map[string]int `json:"types"`
	ContainsPII  bool           `json:"contains_pii"`
}

type Post struct {
	ID        string    `json:"id"`
	Network   string    `json:"network"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	trips TripReader
	mu    sync.Mutex
	posts []Post
	now   func() time.Time
}

func NewService(trips TripReader) *Service {
	return &Service{trips: trips, now: time.Now}
}

func (s *Service) Networks() map[string]string {
	out := make(map[string]string, len(allowedNetworks))
	for k, v := range allowedNetworks {
		out[k] = v
	}
	return out
}

func (s *Service) BuildPreview(ctx context.Context, userID, tripID, network string) (*Preview, error) {
	name, ok := allowedNetworks[strings.ToLower(strings.TrimSpace(network))]
	if !ok {
		return nil, fmt.Errorf("%w: red social no soportada", domain.ErrValidation)
	}
	trip, err := s.trips.GetTrip(ctx, userID, tripID)
	if err != nil {
		return nil, err
	}
	types := map[string]int{}
	confirmed := 0
	for _, r := range trip.Reservations {
		types[r.Type]++
		if r.Status == domain.ReservationConfirmed {
			confirmed++
		}
	}
	text := summaryText(trip, confirmed)
	return &Preview{
		Network: strings.ToLower(strings.TrimSpace(network)), NetworkName: name, TripID: trip.ID,
		Text: text, Destination: trip.Destination, StartDate: trip.StartDate, EndDate: trip.EndDate,
		Reservations: confirmed, Types: types, ContainsPII: false,
	}, nil
}

func (s *Service) Publish(ctx context.Context, userID, tripID, network string) (*Post, error) {
	preview, err := s.BuildPreview(ctx, userID, tripID, network)
	if err != nil {
		return nil, err
	}
	post := Post{
		ID: idgen.UUID(), Network: preview.Network, Text: preview.Text,
		URL: "https://social.mock/" + preview.Network + "/" + idgen.Token(12), CreatedAt: s.now().UTC(),
	}
	s.mu.Lock()
	s.posts = append(s.posts, post)
	s.mu.Unlock()
	return &post, nil
}

func (s *Service) Posts() []Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Post, len(s.posts))
	copy(out, s.posts)
	return out
}

func summaryText(trip *domain.Trip, confirmed int) string {
	destination := strings.TrimSpace(trip.Destination)
	if destination == "" {
		destination = "un nuevo destino"
	}
	var b strings.Builder
	b.WriteString("Mi próximo viaje a ")
	b.WriteString(destination)
	if trip.StartDate != nil && trip.EndDate != nil {
		b.WriteString(" del ")
		b.WriteString(trip.StartDate.Format("02/01/2006"))
		b.WriteString(" al ")
		b.WriteString(trip.EndDate.Format("02/01/2006"))
	} else if trip.StartDate != nil {
		b.WriteString(" desde el ")
		b.WriteString(trip.StartDate.Format("02/01/2006"))
	}
	if confirmed > 0 {
		b.WriteString(fmt.Sprintf(" con %d reservas confirmadas", confirmed))
	}
	b.WriteString(". #Trotamundos")
	return b.String()
}

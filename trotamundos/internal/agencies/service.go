package agencies

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/idgen"
)

type ReservationReader interface {
	GetReservation(ctx context.Context, userID, reservationID string) (*domain.Reservation, error)
}

type Service struct {
	repo         Repository
	reservations ReservationReader
	now          func() time.Time
}

func NewService(repo Repository, reservations ReservationReader) *Service {
	return &Service{repo: repo, reservations: reservations, now: time.Now}
}

func (s *Service) ListAgencies(ctx context.Context) ([]domain.Agency, error) {
	return s.repo.ListAgencies(ctx)
}

func (s *Service) Favorite(ctx context.Context, userID string) (*domain.FavoriteAgency, error) {
	f, err := s.repo.GetFavorite(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return f, err
}

func (s *Service) SetFavorite(ctx context.Context, userID, agencyID string) (*domain.FavoriteAgency, error) {
	agencyID = strings.TrimSpace(agencyID)
	if agencyID == "" {
		return nil, fmt.Errorf("%w: agency_id es obligatorio", domain.ErrValidation)
	}
	ok, err := s.repo.AgencyExists(ctx, agencyID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrNotFound
	}
	if err := s.repo.SetFavorite(ctx, userID, agencyID); err != nil {
		return nil, err
	}
	return s.repo.GetFavorite(ctx, userID)
}

func (s *Service) DeleteFavorite(ctx context.Context, userID string, confirm bool) error {
	if !confirm {
		return fmt.Errorf("%w: se requiere confirmación para quitar la agencia favorita", domain.ErrValidation)
	}
	if _, err := s.repo.GetFavorite(ctx, userID); err != nil {
		return err
	}
	return s.repo.DeleteFavorite(ctx, userID)
}

func (s *Service) Recommendations(ctx context.Context, userID string) ([]domain.Recommendation, error) {
	if _, err := s.repo.GetFavorite(ctx, userID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return []domain.Recommendation{}, nil
		}
		return nil, err
	}
	return s.repo.ListRecommendations(ctx, userID)
}

func (s *Service) Discard(ctx context.Context, userID, recommendationID string) error {
	if _, err := s.assertRecommendation(ctx, userID, recommendationID); err != nil {
		return err
	}
	return s.repo.RecordResponse(ctx, userID, recommendationID, "discarded")
}

func (s *Service) Inquire(ctx context.Context, userID, recommendationID, message string) (*domain.AgencyRequest, error) {
	rec, err := s.assertRecommendation(ctx, userID, recommendationID)
	if err != nil {
		return nil, err
	}
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		return nil, fmt.Errorf("%w: el mensaje no puede superar los 2000 caracteres", domain.ErrValidation)
	}
	req := &domain.AgencyRequest{
		ID: idgen.UUID(), UserID: userID, AgencyID: rec.AgencyID, AgencyName: rec.AgencyName,
		RecommendationID: rec.ID, Message: message, Status: "open", CreatedAt: s.now().UTC(),
		Context: map[string]any{
			"recommendation_id": rec.ID, "title": rec.Title, "destination": rec.Destination,
			"price_cents": rec.PriceCents, "currency": rec.Currency, "start_date": rec.StartDate,
			"end_date": rec.EndDate,
		},
	}
	if err := s.repo.CreateRequest(ctx, req); err != nil {
		return nil, err
	}
	if err := s.repo.RecordResponse(ctx, userID, recommendationID, "inquiry"); err != nil {
		return nil, err
	}
	return req, nil
}

func (s *Service) ContactAboutReservation(ctx context.Context, userID, reservationID, message string) (*domain.AgencyRequest, error) {
	fav, err := s.repo.GetFavorite(ctx, userID)
	if err != nil {
		return nil, err
	}
	res, err := s.reservations.GetReservation(ctx, userID, reservationID)
	if err != nil {
		return nil, err
	}
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		return nil, fmt.Errorf("%w: el mensaje no puede superar los 2000 caracteres", domain.ErrValidation)
	}
	req := &domain.AgencyRequest{
		ID: idgen.UUID(), UserID: userID, AgencyID: fav.AgencyID, AgencyName: fav.Agency.Name,
		ReservationID: res.ID, Message: message, Status: "open", CreatedAt: s.now().UTC(),
		Context: map[string]any{
			"reservation_id": res.ID, "type": res.Type, "title": res.Title, "destination": res.Destination,
			"provider": res.Provider, "provider_locator": res.ProviderLocator,
			"start_at": res.StartAt, "end_at": res.EndAt, "status": res.Status,
		},
	}
	if err := s.repo.CreateRequest(ctx, req); err != nil {
		return nil, err
	}
	return req, nil
}

func (s *Service) Requests(ctx context.Context, userID string) ([]domain.AgencyRequest, error) {
	return s.repo.ListRequests(ctx, userID)
}

func (s *Service) Request(ctx context.Context, userID, id string) (*domain.AgencyRequest, error) {
	return s.repo.GetRequest(ctx, userID, id)
}

func (s *Service) CloseRequest(ctx context.Context, userID, id string) error {
	return s.repo.CloseRequest(ctx, userID, id, s.now().UTC())
}

func (s *Service) assertRecommendation(ctx context.Context, userID, recommendationID string) (*domain.Recommendation, error) {
	rec, err := s.repo.GetRecommendation(ctx, recommendationID)
	if err != nil {
		return nil, err
	}
	fav, err := s.repo.GetFavorite(ctx, userID)
	if err != nil {
		return nil, err
	}
	if fav.AgencyID != rec.AgencyID {
		return nil, domain.ErrForbidden
	}
	return rec, nil
}

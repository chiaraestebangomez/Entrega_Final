package sharing

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/idgen"
)

type TripReader interface {
	GetTrip(ctx context.Context, userID, tripID string) (*domain.Trip, error)
}

type SharedTrip struct {
	Share domain.TripShare `json:"share"`
	Trip  *domain.Trip     `json:"trip"`
}

type Service struct {
	repo  Repository
	trips TripReader
	now   func() time.Time
}

func NewService(repo Repository, trips TripReader) *Service {
	return &Service{repo: repo, trips: trips, now: time.Now}
}

func (s *Service) Share(ctx context.Context, ownerID, ownerEmail, tripID, guestEmail string) (*domain.TripShare, error) {
	if _, err := s.trips.GetTrip(ctx, ownerID, tripID); err != nil {
		return nil, err
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(guestEmail))
	if err != nil || addr.Address == "" {
		return nil, fmt.Errorf("%w: email de invitado inválido", domain.ErrValidation)
	}
	email := strings.ToLower(addr.Address)
	if ownerEmail != "" && strings.EqualFold(email, ownerEmail) {
		return nil, fmt.Errorf("%w: no podés compartir el viaje con vos mismo", domain.ErrValidation)
	}
	share := &domain.TripShare{
		ID: idgen.UUID(), TripID: tripID, OwnerUserID: ownerID,
		GuestEmail: email, CreatedAt: s.now().UTC(),
	}
	if err := s.repo.Create(ctx, share, share.CreatedAt); err != nil {
		return nil, err
	}
	return share, nil
}

func (s *Service) ListByTrip(ctx context.Context, ownerID, tripID string) ([]domain.TripShare, error) {
	if _, err := s.trips.GetTrip(ctx, ownerID, tripID); err != nil {
		return nil, err
	}
	return s.repo.ListByTrip(ctx, ownerID, tripID)
}

func (s *Service) Revoke(ctx context.Context, ownerID, shareID string) error {
	return s.repo.Revoke(ctx, ownerID, shareID, s.now().UTC())
}

func (s *Service) SharedWithMe(ctx context.Context, userID, email string) ([]SharedTrip, error) {
	if err := s.repo.ClaimInvitations(ctx, userID, email); err != nil {
		return nil, err
	}
	shares, err := s.repo.ListSharedWithMe(ctx, userID, email)
	if err != nil {
		return nil, err
	}
	out := make([]SharedTrip, 0, len(shares))
	for _, sh := range shares {
		trip, err := s.trips.GetTrip(ctx, sh.OwnerUserID, sh.TripID)
		if err != nil {
			continue
		}
		out = append(out, SharedTrip{Share: sh, Trip: trip})
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, userID, email, tripID string) (*SharedTrip, error) {
	if err := s.repo.ClaimInvitations(ctx, userID, email); err != nil {
		return nil, err
	}
	sh, err := s.repo.GetGuestShare(ctx, tripID, userID, email)
	if err != nil {
		return nil, err
	}
	trip, err := s.trips.GetTrip(ctx, sh.OwnerUserID, tripID)
	if err != nil {
		return nil, err
	}
	return &SharedTrip{Share: *sh, Trip: trip}, nil
}

package sharing

import (
	"context"
	"testing"
	"time"

	"trotamundos/internal/domain"
)

type fakeRepo struct {
	shares []domain.TripShare
}

func (f *fakeRepo) ListByTrip(_ context.Context, ownerUserID, tripID string) ([]domain.TripShare, error) {
	out := []domain.TripShare{}
	for _, s := range f.shares {
		if s.OwnerUserID == ownerUserID && s.TripID == tripID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeRepo) Create(_ context.Context, share *domain.TripShare, _ time.Time) error {
	f.shares = append(f.shares, *share)
	return nil
}

func (f *fakeRepo) Revoke(_ context.Context, ownerUserID, shareID string, _ time.Time) error {
	for i := range f.shares {
		if f.shares[i].ID == shareID && f.shares[i].OwnerUserID == ownerUserID {
			now := time.Now()
			f.shares[i].RevokedAt = &now
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeRepo) ClaimInvitations(_ context.Context, userID, email string) error {
	for i := range f.shares {
		if f.shares[i].GuestUserID == "" && f.shares[i].GuestEmail == email {
			f.shares[i].GuestUserID = userID
		}
	}
	return nil
}

func (f *fakeRepo) ListSharedWithMe(_ context.Context, userID, email string) ([]domain.TripShare, error) {
	out := []domain.TripShare{}
	for _, s := range f.shares {
		if (s.GuestUserID == userID || s.GuestEmail == email) && s.RevokedAt == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeRepo) GetGuestShare(_ context.Context, tripID, userID, email string) (*domain.TripShare, error) {
	for i := range f.shares {
		s := &f.shares[i]
		if s.TripID == tripID && s.RevokedAt == nil && (s.GuestUserID == userID || s.GuestEmail == email) {
			return s, nil
		}
	}
	return nil, domain.ErrNotFound
}

type fakeTrips struct{ trip *domain.Trip }

func (f *fakeTrips) GetTrip(_ context.Context, userID, tripID string) (*domain.Trip, error) {
	if f.trip == nil || f.trip.UserID != userID || f.trip.ID != tripID {
		return nil, domain.ErrNotFound
	}
	return f.trip, nil
}

func TestShareNormalizesEmailAndRejectsSelf(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, &fakeTrips{trip: &domain.Trip{ID: "t1", UserID: "owner"}})
	share, err := svc.Share(context.Background(), "owner", "owner@example.com", "t1", "Guest@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	if share.GuestEmail != "guest@example.com" {
		t.Fatalf("email = %q", share.GuestEmail)
	}
	if _, err := svc.Share(context.Background(), "owner", "owner@example.com", "t1", "OWNER@example.com"); err == nil {
		t.Fatal("esperaba error al compartir con uno mismo")
	}
	if _, err := svc.Share(context.Background(), "owner", "owner@example.com", "t1", "no-es-email"); err == nil {
		t.Fatal("esperaba error con email inválido")
	}
}

func TestSharedWithMeClaimsAndRevoke(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, &fakeTrips{trip: &domain.Trip{ID: "t1", UserID: "owner"}})
	share, err := svc.Share(context.Background(), "owner", "", "t1", "guest@example.com")
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.SharedWithMe(context.Background(), "guest-id", "guest@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Trip == nil {
		t.Fatalf("compartidos = %+v", out)
	}
	if err := svc.Revoke(context.Background(), "owner", share.ID); err != nil {
		t.Fatal(err)
	}
	out, _ = svc.SharedWithMe(context.Background(), "guest-id", "guest@example.com")
	if len(out) != 0 {
		t.Fatal("el viaje revocado no debe aparecer")
	}
}

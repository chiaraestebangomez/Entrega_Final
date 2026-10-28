package itinerary

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/geo"
	"trotamundos/internal/platform/idgen"
	"trotamundos/internal/providers"
)

type FactPublisher interface {
	PublishReservation(ctx context.Context, res *domain.Reservation) error
}

type Service struct {
	repo      Repository
	providers *providers.Registry
	facts     FactPublisher
	now       func() time.Time
}

func NewService(repo Repository, providerRegistry *providers.Registry, facts FactPublisher) *Service {
	return &Service{repo: repo, providers: providerRegistry, facts: facts, now: time.Now}
}

type ReservationInput struct {
	Type            string         `json:"type"`
	Provider        string         `json:"provider"`
	ProviderLocator string         `json:"provider_locator"`
	Title           string         `json:"title"`
	Destination     string         `json:"destination"`
	StartAt         *time.Time     `json:"start_at"`
	EndAt           *time.Time     `json:"end_at"`
	Status          string         `json:"status"`
	Details         map[string]any `json:"details"`
	TripID          string         `json:"trip_id"`
}

func (s *Service) ListTrips(ctx context.Context, userID string, archived bool) ([]domain.Trip, error) {
	if !archived {
		_, _ = s.repo.ArchivePastTrips(ctx, s.now().UTC())
	}
	trips, err := s.repo.ListTrips(ctx, userID, archived)
	if err != nil {
		return nil, err
	}
	for i := range trips {
		res, err := s.repo.ListReservationsByTrip(ctx, userID, trips[i].ID)
		if err != nil {
			return nil, err
		}
		trips[i].Reservations = res
	}
	return trips, nil
}

func (s *Service) GetTrip(ctx context.Context, userID, tripID string) (*domain.Trip, error) {
	t, err := s.repo.GetTrip(ctx, userID, tripID)
	if err != nil {
		return nil, err
	}
	res, err := s.repo.ListReservationsByTrip(ctx, userID, tripID)
	if err != nil {
		return nil, err
	}
	t.Reservations = res
	return t, nil
}

func (s *Service) GetReservation(ctx context.Context, userID, reservationID string) (*domain.Reservation, error) {
	return s.repo.GetReservation(ctx, userID, reservationID)
}

func (s *Service) DeleteTrip(ctx context.Context, userID, tripID string, confirm bool) error {
	if !confirm {
		return fmt.Errorf("%w: se requiere confirmación para eliminar el viaje", domain.ErrValidation)
	}
	t, err := s.repo.GetTrip(ctx, userID, tripID)
	if err != nil {
		return err
	}
	if t.Status != domain.TripArchived {
		return fmt.Errorf("%w: solo se pueden eliminar viajes archivados", domain.ErrValidation)
	}
	return s.repo.DeleteTrip(ctx, userID, tripID)
}

func (s *Service) CreateReservation(ctx context.Context, userID string, in ReservationInput) (*domain.Reservation, error) {
	res, _, err := s.ApplyReservation(ctx, userID, in, domain.SourceManual, "manual")
	return res, err
}

func (s *Service) UpdateReservation(ctx context.Context, userID, id string, in ReservationInput) (*domain.Reservation, error) {
	cur, err := s.repo.GetReservation(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	updated, changed := mergeReservation(cur, normalizeInput(in), domain.SourceManual)
	if in.TripID != "" && in.TripID != cur.TripID {
		t, err := s.repo.GetTrip(ctx, userID, in.TripID)
		if err != nil {
			return nil, err
		}
		updated.TripID = t.ID
		changed = appendUnique(changed, "trip_id")
	}
	if len(changed) == 0 {
		return cur, nil
	}
	now := s.now().UTC()
	updated.UpdatedAt = now
	if !MatchesTrip(&domain.Trip{Destination: cur.Destination, StartDate: cur.StartAt, EndDate: cur.EndAt, Status: domain.TripActive}, updated.Destination, updated.StartAt, updated.EndAt) || updated.TripID == "" {
		if err := s.regroup(ctx, userID, &updated); err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdateReservation(ctx, &updated); err != nil {
		return nil, err
	}
	if err := s.recordChange(ctx, cur, &updated, domain.SourceManual, changed); err != nil {
		return nil, err
	}
	s.notify(ctx, userID, "reservation_updated", "Reserva actualizada", "Se actualizaron datos de "+displayTitle(&updated), "reservation", updated.ID)
	s.publishFact(ctx, &updated)
	return &updated, nil
}

func (s *Service) DeleteReservation(ctx context.Context, userID, id string, confirm bool) error {
	if !confirm {
		return fmt.Errorf("%w: se requiere confirmación para eliminar la reserva", domain.ErrValidation)
	}
	res, err := s.repo.GetReservation(ctx, userID, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteReservation(ctx, userID, id); err != nil {
		return err
	}
	return s.repo.RefreshTripBounds(ctx, userID, res.TripID)
}

func (s *Service) ApplyReservation(ctx context.Context, userID string, in ReservationInput, source, sourceRef string) (*domain.Reservation, bool, error) {
	in = normalizeInput(in)
	if err := validateInput(in); err != nil {
		return nil, false, err
	}
	if in.Provider != "" && in.ProviderLocator != "" {
		existing, err := s.repo.FindByProviderLocator(ctx, userID, in.Provider, in.ProviderLocator)
		if err == nil {
			return s.updateFromInput(ctx, userID, existing, in, source, sourceRef)
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, false, err
		}
	}
	trip, err := s.assignTrip(ctx, userID, in)
	if err != nil {
		return nil, false, err
	}
	now := s.now().UTC()
	res := &domain.Reservation{
		ID:              idgen.UUID(),
		UserID:          userID,
		TripID:          trip.ID,
		Type:            in.Type,
		Provider:        in.Provider,
		ProviderLocator: in.ProviderLocator,
		Title:           in.Title,
		Destination:     in.Destination,
		StartAt:         cloneTime(in.StartAt),
		EndAt:           cloneTime(in.EndAt),
		Status:          in.Status,
		Source:          source,
		SourceRef:       sourceRef,
		Details:         cloneMap(in.Details),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.CreateReservation(ctx, res); err != nil {
		return nil, false, err
	}
	if err := s.repo.RefreshTripBounds(ctx, userID, trip.ID); err != nil {
		return nil, false, err
	}
	change := &domain.ReservationChange{
		ID:            idgen.UUID(),
		UserID:        userID,
		ReservationID: res.ID,
		Source:        source,
		ChangedFields: []string{"created"},
		Previous:      map[string]any{},
		Current:       reservationToMap(res),
		CreatedAt:     now,
	}
	_ = s.repo.AddChange(ctx, change)
	s.notify(ctx, userID, "reservation_created", "Nueva reserva", displayTitle(res)+" se agregó a tu viaje a "+displayDestination(res), "reservation", res.ID)
	s.seedProvider(res)
	s.publishFact(ctx, res)
	return res, true, nil
}

func (s *Service) updateFromInput(ctx context.Context, userID string, cur *domain.Reservation, in ReservationInput, source, sourceRef string) (*domain.Reservation, bool, error) {
	updated, changed := mergeReservation(cur, in, source)
	if len(changed) == 0 {
		return cur, false, nil
	}
	now := s.now().UTC()
	updated.UpdatedAt = now
	if sourceRef != "" {
		updated.SourceRef = sourceRef
	}
	if !MatchesTrip(&domain.Trip{Destination: cur.Destination, StartDate: cur.StartAt, EndDate: cur.EndAt, Status: domain.TripActive}, updated.Destination, updated.StartAt, updated.EndAt) {
		if err := s.regroup(ctx, userID, &updated); err != nil {
			return nil, false, err
		}
	}
	if err := s.repo.UpdateReservation(ctx, &updated); err != nil {
		return nil, false, err
	}
	if err := s.repo.RefreshTripBounds(ctx, userID, cur.TripID); err != nil {
		return nil, false, err
	}
	if updated.TripID != cur.TripID {
		_ = s.repo.RefreshTripBounds(ctx, userID, updated.TripID)
	}
	if err := s.recordChange(ctx, cur, &updated, source, changed); err != nil {
		return nil, false, err
	}
	if updated.Status == domain.ReservationCanceled && cur.Status != domain.ReservationCanceled {
		s.notify(ctx, userID, "reservation_canceled", "Reserva cancelada", displayTitle(&updated)+" fue cancelada por el proveedor", "reservation", updated.ID)
	} else {
		s.notify(ctx, userID, "reservation_updated", "Reserva actualizada", "Cambios en "+displayTitle(&updated)+": "+strings.Join(changed, ", "), "reservation", updated.ID)
	}
	s.seedProvider(&updated)
	s.publishFact(ctx, &updated)
	return &updated, true, nil
}

func (s *Service) SyncReservation(ctx context.Context, userID, id string) (*domain.Reservation, error) {
	res, err := s.repo.GetReservation(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if res.Provider == "" || res.ProviderLocator == "" {
		return nil, fmt.Errorf("%w: la reserva no tiene proveedor y localizador para sincronizar", domain.ErrValidation)
	}
	adapter, ok := s.providers.Get(res.Provider)
	if !ok {
		return nil, fmt.Errorf("%w: proveedor no soportado", domain.ErrValidation)
	}
	now := s.now().UTC()
	snap, err := adapter.GetReservation(ctx, res.ProviderLocator)
	if err != nil {
		res.Stale = true
		res.UpdatedAt = now
		_ = s.repo.UpdateReservation(ctx, res)
		return res, nil
	}
	in := ReservationInput{
		Type:            snap.Type,
		Provider:        snap.Provider,
		ProviderLocator: snap.Locator,
		Title:           snap.Title,
		Destination:     snap.Destination,
		StartAt:         snap.StartAt,
		EndAt:           snap.EndAt,
		Status:          snap.Status,
		Details:         snap.Details,
	}
	if in.Provider == "" {
		in.Provider = res.Provider
	}
	if in.ProviderLocator == "" {
		in.ProviderLocator = res.ProviderLocator
	}
	updated, _, err := s.ApplyReservation(ctx, userID, in, domain.SourceProvider, "provider:"+res.Provider)
	if err != nil {
		return nil, err
	}
	updated.LastSyncedAt = &now
	updated.Stale = false
	updated.UpdatedAt = now
	if err := s.repo.UpdateReservation(ctx, updated); err != nil {
		return nil, err
	}
	s.providers.SetSnapshot(res.Provider, res.ProviderLocator, snap)
	return updated, nil
}

func (s *Service) SyncDueReservations(ctx context.Context, limit int) (int, int) {
	reservations, err := s.repo.ListReservationsForSync(ctx, limit)
	if err != nil {
		return 0, 0
	}
	ok, failed := 0, 0
	for i := range reservations {
		res := reservations[i]
		if _, err := s.SyncReservation(ctx, res.UserID, res.ID); err != nil {
			failed++
			continue
		}
		ok++
	}
	return ok, failed
}

func (s *Service) ArchivePastTrips(ctx context.Context) (int64, error) {
	return s.repo.ArchivePastTrips(ctx, s.now().UTC())
}

func (s *Service) ListChanges(ctx context.Context, userID, reservationID string) ([]domain.ReservationChange, error) {
	if _, err := s.repo.GetReservation(ctx, userID, reservationID); err != nil {
		return nil, err
	}
	return s.repo.ListChanges(ctx, userID, reservationID)
}

func (s *Service) ListNotifications(ctx context.Context, userID string, limit int) ([]domain.Notification, error) {
	return s.repo.ListNotifications(ctx, userID, limit)
}

func (s *Service) MarkNotificationRead(ctx context.Context, userID, id string) error {
	return s.repo.MarkNotificationRead(ctx, userID, id)
}

func (s *Service) RecordAudit(ctx context.Context, userID, action, resource, resourceID, ip string, details map[string]any) {
	_ = s.repo.AddAudit(ctx, &domain.AuditLog{
		ID: idgen.UUID(), UserID: userID, Action: action, Resource: resource,
		ResourceID: resourceID, IP: ip, Details: details, CreatedAt: s.now().UTC(),
	})
}

type LocationContext struct {
	Available     bool    `json:"available"`
	Reason        string  `json:"reason,omitempty"`
	Destination   string  `json:"destination,omitempty"`
	DistanceKM    float64 `json:"distance_km,omitempty"`
	NavigationURL string  `json:"navigation_url,omitempty"`
}

func (s *Service) LocationContext(ctx context.Context, userID, tripID string, consent bool, lat, lng *float64) (LocationContext, error) {
	t, err := s.repo.GetTrip(ctx, userID, tripID)
	if err != nil {
		return LocationContext{}, err
	}
	if !consent || lat == nil || lng == nil {
		return LocationContext{Available: false, Reason: "location_consent_required", Destination: t.Destination}, nil
	}
	dest, ok := geo.Lookup(t.Destination)
	if !ok {
		return LocationContext{Available: false, Reason: "destination_without_coordinates", Destination: t.Destination}, nil
	}
	from := geo.Coord{Lat: *lat, Lng: *lng}
	return LocationContext{
		Available:     true,
		Destination:   t.Destination,
		DistanceKM:    geo.DistanceKM(from, dest),
		NavigationURL: geo.NavigationURL(from, dest),
	}, nil
}

func (s *Service) assignTrip(ctx context.Context, userID string, in ReservationInput) (*domain.Trip, error) {
	if in.TripID != "" {
		return s.repo.GetTrip(ctx, userID, in.TripID)
	}
	trips, err := s.repo.ListActiveTrips(ctx, userID)
	if err != nil {
		return nil, err
	}
	if t := BestTrip(trips, in.Destination, in.StartAt, in.EndAt); t != nil {
		return t, nil
	}
	now := s.now().UTC()
	start := dateOnlyPtr(in.StartAt)
	end := dateOnlyPtr(in.EndAt)
	t := &domain.Trip{
		ID:          idgen.UUID(),
		UserID:      userID,
		Name:        TripName(in.Destination, in.Title),
		Destination: in.Destination,
		StartDate:   start,
		EndDate:     end,
		Status:      domain.TripActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.CreateTrip(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) regroup(ctx context.Context, userID string, res *domain.Reservation) error {
	oldTripID := res.TripID
	trips, err := s.repo.ListActiveTrips(ctx, userID)
	if err != nil {
		return err
	}
	filtered := trips[:0]
	for _, t := range trips {
		if t.ID != oldTripID {
			filtered = append(filtered, t)
		}
	}
	if best := BestTrip(filtered, res.Destination, res.StartAt, res.EndAt); best != nil {
		res.TripID = best.ID
	} else {
		now := s.now().UTC()
		t := &domain.Trip{
			ID: idgen.UUID(), UserID: userID, Name: TripName(res.Destination, res.Title),
			Destination: res.Destination, StartDate: dateOnlyPtr(res.StartAt), EndDate: dateOnlyPtr(res.EndAt),
			Status: domain.TripActive, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.repo.CreateTrip(ctx, t); err != nil {
			return err
		}
		res.TripID = t.ID
	}
	if res.TripID != oldTripID {
		return s.repo.RefreshTripBounds(ctx, userID, oldTripID)
	}
	return nil
}

func (s *Service) recordChange(ctx context.Context, before, after *domain.Reservation, source string, changed []string) error {
	return s.repo.AddChange(ctx, &domain.ReservationChange{
		ID:            idgen.UUID(),
		UserID:        before.UserID,
		ReservationID: before.ID,
		Source:        source,
		ChangedFields: changed,
		Previous:      reservationToMap(before),
		Current:       reservationToMap(after),
		CreatedAt:     s.now().UTC(),
	})
}

func (s *Service) notify(ctx context.Context, userID, typ, title, body, resourceType, resourceID string) {
	_ = s.repo.AddNotification(ctx, &domain.Notification{
		ID: idgen.UUID(), UserID: userID, Type: typ, Title: title, Body: body,
		ResourceType: resourceType, ResourceID: resourceID, CreatedAt: s.now().UTC(),
	})
}

func (s *Service) publishFact(ctx context.Context, res *domain.Reservation) {
	if s.facts != nil {
		_ = s.facts.PublishReservation(ctx, res)
	}
}

func (s *Service) seedProvider(res *domain.Reservation) {
	if s.providers == nil || res.Provider == "" || res.ProviderLocator == "" {
		return
	}
	s.providers.Seed(res.Provider, res.ProviderLocator, providers.Snapshot{
		Type: res.Type, Provider: res.Provider, Locator: res.ProviderLocator, Title: res.Title,
		Destination: res.Destination, StartAt: res.StartAt, EndAt: res.EndAt, Status: res.Status,
		Details: res.Details, UpdatedAt: res.UpdatedAt,
	})
}

func normalizeInput(in ReservationInput) ReservationInput {
	in.Type = strings.ToLower(strings.TrimSpace(in.Type))
	if in.Type == "" {
		in.Type = domain.TypeOther
	}
	in.Provider = strings.ToLower(strings.TrimSpace(in.Provider))
	if in.Provider == "" {
		in.Provider = domain.SourceManual
	}
	in.ProviderLocator = strings.TrimSpace(in.ProviderLocator)
	in.Title = strings.TrimSpace(in.Title)
	in.Destination = strings.TrimSpace(in.Destination)
	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	if in.Status == "" {
		in.Status = domain.ReservationConfirmed
	}
	if in.Details == nil {
		in.Details = map[string]any{}
	}
	in.StartAt = cloneTime(in.StartAt)
	in.EndAt = cloneTime(in.EndAt)
	return in
}

func validateInput(in ReservationInput) error {
	switch in.Type {
	case domain.TypeFlight, domain.TypeHotel, domain.TypeCar, domain.TypeOther:
	default:
		return fmt.Errorf("%w: tipo de reserva inválido", domain.ErrValidation)
	}
	switch in.Status {
	case domain.ReservationConfirmed, domain.ReservationCanceled:
	default:
		return fmt.Errorf("%w: estado de reserva inválido", domain.ErrValidation)
	}
	if in.EndAt != nil && in.StartAt != nil && in.EndAt.Before(*in.StartAt) {
		return fmt.Errorf("%w: la fecha de fin no puede ser anterior al inicio", domain.ErrValidation)
	}
	return nil
}

func mergeReservation(cur *domain.Reservation, in ReservationInput, source string) (domain.Reservation, []string) {
	upd := *cur
	fillOnly := (cur.Source == domain.SourceProvider && source != domain.SourceProvider) ||
		(cur.Source == domain.SourceManual && source == domain.SourceEmail)
	changed := []string{}

	if in.Type != "" && (!fillOnly || upd.Type == "") && upd.Type != in.Type {
		upd.Type = in.Type
		changed = appendUnique(changed, "type")
	}
	if in.Provider != "" && in.Provider != domain.SourceManual && upd.Provider != in.Provider && (!fillOnly || upd.Provider == "" || upd.Provider == domain.SourceManual) {
		upd.Provider = in.Provider
		changed = appendUnique(changed, "provider")
	}
	if in.ProviderLocator != "" && upd.ProviderLocator != in.ProviderLocator && (!fillOnly || upd.ProviderLocator == "") {
		upd.ProviderLocator = in.ProviderLocator
		changed = appendUnique(changed, "provider_locator")
	}
	if in.Title != "" && upd.Title != in.Title && (!fillOnly || upd.Title == "") {
		upd.Title = in.Title
		changed = appendUnique(changed, "title")
	}
	if in.Destination != "" && upd.Destination != in.Destination && (!fillOnly || upd.Destination == "") {
		upd.Destination = in.Destination
		changed = appendUnique(changed, "destination")
	}
	if in.StartAt != nil && (!fillOnly || upd.StartAt == nil) && !sameTime(upd.StartAt, in.StartAt) {
		upd.StartAt = cloneTime(in.StartAt)
		changed = appendUnique(changed, "start_at")
	}
	if in.EndAt != nil && (!fillOnly || upd.EndAt == nil) && !sameTime(upd.EndAt, in.EndAt) {
		upd.EndAt = cloneTime(in.EndAt)
		changed = appendUnique(changed, "end_at")
	}
	if in.Status != "" && upd.Status != in.Status && (!fillOnly || in.Status == domain.ReservationConfirmed) {
		upd.Status = in.Status
		changed = appendUnique(changed, "status")
	}
	if len(in.Details) > 0 {
		merged := cloneMap(upd.Details)
		for k, v := range in.Details {
			if _, exists := merged[k]; exists && fillOnly {
				continue
			}
			if !reflect.DeepEqual(merged[k], v) {
				merged[k] = v
				changed = appendUnique(changed, "details."+k)
			}
		}
		upd.Details = merged
	}
	return upd, changed
}

func reservationToMap(r *domain.Reservation) map[string]any {
	return map[string]any{
		"type": r.Type, "provider": r.Provider, "provider_locator": r.ProviderLocator,
		"title": r.Title, "destination": r.Destination, "start_at": r.StartAt, "end_at": r.EndAt,
		"status": r.Status, "source": r.Source, "trip_id": r.TripID,
	}
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func displayTitle(r *domain.Reservation) string {
	if strings.TrimSpace(r.Title) != "" {
		return r.Title
	}
	if strings.TrimSpace(r.Destination) != "" {
		return "Reserva en " + r.Destination
	}
	return "Reserva"
}

func displayDestination(r *domain.Reservation) string {
	if strings.TrimSpace(r.Destination) != "" {
		return r.Destination
	}
	return "destino"
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

func dateOnlyPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	d := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	return &d
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

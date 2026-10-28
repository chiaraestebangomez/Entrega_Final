package ingestion

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/itinerary"
	"trotamundos/internal/mailer"
	"trotamundos/internal/platform/idgen"
	"trotamundos/internal/platform/secrets"
)

type ReservationApplier interface {
	ApplyReservation(ctx context.Context, userID string, in itinerary.ReservationInput, source, sourceRef string) (*domain.Reservation, bool, error)
}

type Service struct {
	repo      Repository
	applier   ReservationApplier
	connector mailer.Connector
	box       *secrets.Box
	log       *slog.Logger
	now       func() time.Time
	queue     chan string
}

func NewService(repo Repository, applier ReservationApplier, connector mailer.Connector, box *secrets.Box, log *slog.Logger) *Service {
	return &Service{
		repo: repo, applier: applier, connector: connector, box: box, log: log,
		now: time.Now, queue: make(chan string, 1024),
	}
}

type LinkMailboxInput struct {
	Provider    string   `json:"provider"`
	Email       string   `json:"email"`
	AccessToken string   `json:"access_token"`
	Scopes      []string `json:"scopes"`
}

func (s *Service) LinkMailbox(ctx context.Context, userID string, in LinkMailboxInput) (*domain.MailAccount, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, fmt.Errorf("%w: email inválido", domain.ErrValidation)
	}
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if provider == "" {
		return nil, fmt.Errorf("%w: proveedor de correo requerido", domain.ErrValidation)
	}
	if len(in.Scopes) == 0 {
		in.Scopes = []string{"mail.read"}
	}
	for _, sc := range in.Scopes {
		if sc != "mail.read" && sc != "mail.readonly" {
			return nil, fmt.Errorf("%w: solo se permite el permiso mínimo de lectura", domain.ErrValidation)
		}
	}
	tokenCipher, err := s.box.Encrypt(in.AccessToken)
	if err != nil {
		return nil, err
	}
	acc := &domain.MailAccount{
		ID: idgen.UUID(), UserID: userID, Provider: provider, Email: email,
		Scopes: in.Scopes, TokenCipher: tokenCipher, Status: "active", CreatedAt: s.now().UTC(),
	}
	if err := s.repo.CreateAccount(ctx, acc); err != nil {
		return nil, err
	}
	return acc, nil
}

func (s *Service) ListAccounts(ctx context.Context, userID string) ([]domain.MailAccount, error) {
	return s.repo.ListAccounts(ctx, userID)
}

func (s *Service) RevokeMailbox(ctx context.Context, userID, id string) error {
	return s.repo.RevokeAccount(ctx, userID, id, s.now().UTC())
}

func (s *Service) ScanMailbox(ctx context.Context, userID, accountID string) (int, error) {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if acc.UserID != userID {
		return 0, domain.ErrForbidden
	}
	if acc.Status != "active" {
		return 0, fmt.Errorf("%w: la cuenta de correo está revocada", domain.ErrValidation)
	}
	msgs, err := s.connector.FetchNew(ctx, *acc)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, m := range msgs {
		if acc.LastScannedAt != nil && !m.ReceivedAt.After(*acc.LastScannedAt) {
			continue
		}
		ev, ok, err := s.enqueue(ctx, acc, "imap", m)
		if err != nil {
			return created, err
		}
		if ok {
			created++
			s.Notify(ev.ID)
		}
	}
	if err := s.repo.UpdateLastScan(ctx, acc.ID, s.now().UTC()); err != nil {
		return created, err
	}
	return created, nil
}

type InboundEmail struct {
	MailboxID  string    `json:"mailbox_id"`
	MessageID  string    `json:"message_id"`
	From       string    `json:"from"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	ReceivedAt time.Time `json:"received_at"`
}

func (s *Service) ReceiveWebhook(ctx context.Context, in InboundEmail) (*domain.IngestionEvent, error) {
	if strings.TrimSpace(in.MailboxID) == "" {
		return nil, fmt.Errorf("%w: mailbox_id es obligatorio", domain.ErrValidation)
	}
	acc, err := s.repo.GetAccount(ctx, in.MailboxID)
	if err != nil {
		return nil, err
	}
	if acc.Status != "active" {
		return nil, fmt.Errorf("%w: la cuenta de correo está revocada", domain.ErrValidation)
	}
	msg := mailer.Message{
		ID: in.MessageID, From: in.From, Subject: in.Subject, Body: in.Body, ReceivedAt: in.ReceivedAt,
	}
	if msg.ID == "" {
		msg.ID = idgen.UUID()
	}
	if msg.ReceivedAt.IsZero() {
		msg.ReceivedAt = s.now().UTC()
	}
	ev, created, err := s.enqueue(ctx, acc, "webhook", msg)
	if err != nil {
		return nil, err
	}
	if created {
		s.Notify(ev.ID)
	}
	return ev, nil
}

func (s *Service) ListEvents(ctx context.Context, userID string, limit int) ([]domain.IngestionEvent, error) {
	return s.repo.ListEventsByUser(ctx, userID, limit)
}

func (s *Service) ListWhitelist(ctx context.Context, userID string) ([]domain.WhitelistEntry, error) {
	return s.repo.ListWhitelist(ctx, userID)
}

func (s *Service) AddWhitelist(ctx context.Context, userID, pattern string) (*domain.WhitelistEntry, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, fmt.Errorf("%w: el patrón es obligatorio", domain.ErrValidation)
	}
	w := &domain.WhitelistEntry{ID: idgen.UUID(), UserID: userID, Pattern: pattern}
	if err := s.repo.AddWhitelist(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

func (s *Service) DeleteWhitelist(ctx context.Context, userID, id string) error {
	return s.repo.DeleteWhitelist(ctx, userID, id)
}

func (s *Service) ListFilters(ctx context.Context, userID string) ([]domain.MailFilter, error) {
	return s.repo.ListFilters(ctx, userID)
}

func (s *Service) AddFilter(ctx context.Context, userID string, in domain.MailFilter) (*domain.MailFilter, error) {
	in.ID = idgen.UUID()
	in.UserID = userID
	if err := validateFilter(&in); err != nil {
		return nil, err
	}
	if err := s.repo.AddFilter(ctx, &in); err != nil {
		return nil, err
	}
	return &in, nil
}

func (s *Service) UpdateFilter(ctx context.Context, userID, id string, in domain.MailFilter) (*domain.MailFilter, error) {
	in.ID = id
	in.UserID = userID
	if err := validateFilter(&in); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateFilter(ctx, &in); err != nil {
		return nil, err
	}
	return &in, nil
}

func (s *Service) DeleteFilter(ctx context.Context, userID, id string) error {
	return s.repo.DeleteFilter(ctx, userID, id)
}

func (s *Service) Notify(eventID string) {
	select {
	case s.queue <- eventID:
	default:
	}
}

func (s *Service) Queue() <-chan string { return s.queue }

func (s *Service) enqueue(ctx context.Context, acc *domain.MailAccount, channel string, msg mailer.Message) (*domain.IngestionEvent, bool, error) {
	payload := map[string]any{
		"from": msg.From, "subject": msg.Subject, "body": msg.Body, "received_at": msg.ReceivedAt,
	}
	ev := &domain.IngestionEvent{
		ID: idgen.UUID(), UserID: acc.UserID, MailboxID: acc.ID, Channel: channel,
		MessageID: msg.ID, Payload: payload, Status: domain.EventPending,
		ReceivedAt: &msg.ReceivedAt, CreatedAt: s.now().UTC(),
	}
	created, err := s.repo.CreateEvent(ctx, ev)
	if err != nil {
		return nil, false, err
	}
	if !created {
		existing, lerr := s.findEvent(ctx, acc.UserID, msg.ID)
		if lerr == nil {
			return existing, false, nil
		}
	}
	return ev, created, nil
}

func (s *Service) findEvent(ctx context.Context, userID, messageID string) (*domain.IngestionEvent, error) {
	events, err := s.repo.ListEventsByUser(ctx, userID, 200)
	if err != nil {
		return nil, err
	}
	for i := range events {
		if events[i].MessageID == messageID {
			return &events[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func validateFilter(f *domain.MailFilter) error {
	switch f.Field {
	case "sender", "subject", "body":
	default:
		return fmt.Errorf("%w: campo de filtro inválido", domain.ErrValidation)
	}
	switch f.Operator {
	case "contains", "equals", "not_contains", "starts_with":
	default:
		return fmt.Errorf("%w: operador de filtro inválido", domain.ErrValidation)
	}
	if strings.TrimSpace(f.Value) == "" {
		return fmt.Errorf("%w: el valor del filtro es obligatorio", domain.ErrValidation)
	}
	return nil
}

func messageFromPayload(payload map[string]any) mailer.Message {
	get := func(k string) string {
		if v, ok := payload[k].(string); ok {
			return v
		}
		return ""
	}
	msg := mailer.Message{From: get("from"), Subject: get("subject"), Body: get("body")}
	if v, ok := payload["received_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			msg.ReceivedAt = t
		}
	}
	if msg.ReceivedAt.IsZero() {
		msg.ReceivedAt = time.Now().UTC()
	}
	return msg
}

func (s *Service) ProcessEvent(ctx context.Context, ev domain.IngestionEvent) error {
	msg := messageFromPayload(ev.Payload)
	if ev.MessageID != "" {
		msg.ID = ev.MessageID
	}
	whitelist, err := s.repo.ListWhitelist(ctx, ev.UserID)
	if err != nil {
		return err
	}
	filters, err := s.repo.ListFilters(ctx, ev.UserID)
	if err != nil {
		return err
	}
	if !ShouldProcess(whitelist, filters, msg) {
		return s.repo.MarkProcessed(ctx, ev.ID, domain.EventIgnored, s.now().UTC())
	}
	parsed, err := ParseMessage(msg)
	if err != nil {
		if errors.Is(err, ErrUnparseable) {
			return s.repo.MarkProcessed(ctx, ev.ID, domain.EventIgnored, s.now().UTC())
		}
		return err
	}
	in := itinerary.ReservationInput{
		Type: parsed.Type, Provider: parsed.Provider, ProviderLocator: parsed.Locator,
		Title: parsed.Title, Destination: parsed.Destination, StartAt: parsed.StartAt,
		EndAt: parsed.EndAt, Status: domain.ReservationConfirmed, Details: parsed.Details,
	}
	if _, _, err := s.applier.ApplyReservation(ctx, ev.UserID, in, domain.SourceEmail, "email:"+ev.MessageID); err != nil {
		return err
	}
	return s.repo.MarkProcessed(ctx, ev.ID, domain.EventProcessed, s.now().UTC())
}

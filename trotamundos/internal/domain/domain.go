package domain

import "time"

const (
	RoleTraveler = "traveler"
	RoleAnalyst  = "analyst"
	RoleAdmin    = "admin"

	TripActive   = "active"
	TripArchived = "archived"

	ReservationConfirmed = "confirmed"
	ReservationCanceled  = "canceled"

	SourceEmail    = "email"
	SourceProvider = "provider"
	SourceManual   = "manual"

	TypeFlight = "flight"
	TypeHotel  = "hotel"
	TypeCar    = "car"
	TypeOther  = "other"

	EventPending    = "pending"
	EventProcessing = "processing"
	EventProcessed  = "processed"
	EventIgnored    = "ignored"
	EventError      = "error"

	RequestOpen   = "open"
	RequestClosed = "closed"
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	FullName     string    `json:"full_name"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	Locale       string    `json:"locale"`
	Region       string    `json:"region"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Trip struct {
	ID           string        `json:"id"`
	UserID       string        `json:"user_id,omitempty"`
	Name         string        `json:"name"`
	Destination  string        `json:"destination"`
	StartDate    *time.Time    `json:"start_date,omitempty"`
	EndDate      *time.Time    `json:"end_date,omitempty"`
	Status       string        `json:"status"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	Reservations []Reservation `json:"reservations,omitempty"`
}

type Reservation struct {
	ID              string              `json:"id"`
	UserID          string              `json:"user_id,omitempty"`
	TripID          string              `json:"trip_id"`
	Type            string              `json:"type"`
	Provider        string              `json:"provider"`
	ProviderLocator string              `json:"provider_locator,omitempty"`
	Title           string              `json:"title"`
	Destination     string              `json:"destination"`
	StartAt         *time.Time          `json:"start_at,omitempty"`
	EndAt           *time.Time          `json:"end_at,omitempty"`
	Status          string              `json:"status"`
	Source          string              `json:"source"`
	SourceRef       string              `json:"source_ref,omitempty"`
	Details         map[string]any      `json:"details,omitempty"`
	LastSyncedAt    *time.Time          `json:"last_synced_at,omitempty"`
	Stale           bool                `json:"stale"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
	Changes         []ReservationChange `json:"changes,omitempty"`
}

type ReservationChange struct {
	ID            string         `json:"id"`
	UserID        string         `json:"user_id,omitempty"`
	ReservationID string         `json:"reservation_id"`
	Source        string         `json:"source"`
	ChangedFields []string       `json:"changed_fields"`
	Previous      map[string]any `json:"previous"`
	Current       map[string]any `json:"current"`
	CreatedAt     time.Time      `json:"created_at"`
}

type Notification struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id,omitempty"`
	Type         string     `json:"type"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	ResourceType string     `json:"resource_type"`
	ResourceID   string     `json:"resource_id"`
	ReadAt       *time.Time `json:"read_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type IngestionEvent struct {
	ID          string         `json:"id"`
	UserID      string         `json:"user_id"`
	MailboxID   string         `json:"mailbox_id,omitempty"`
	Channel     string         `json:"channel"`
	MessageID   string         `json:"message_id"`
	Payload     map[string]any `json:"payload"`
	Status      string         `json:"status"`
	Attempts    int            `json:"attempts"`
	LastError   string         `json:"last_error,omitempty"`
	ReceivedAt  *time.Time     `json:"received_at,omitempty"`
	ProcessedAt *time.Time     `json:"processed_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

type MailAccount struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id,omitempty"`
	Provider      string     `json:"provider"`
	Email         string     `json:"email"`
	Scopes        []string   `json:"scopes"`
	TokenCipher   string     `json:"-"`
	Status        string     `json:"status"`
	LastScannedAt *time.Time `json:"last_scanned_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}

type WhitelistEntry struct {
	ID      string `json:"id"`
	UserID  string `json:"user_id,omitempty"`
	Pattern string `json:"pattern"`
}

type MailFilter struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id,omitempty"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
	Active   bool   `json:"active"`
}

type TripShare struct {
	ID          string     `json:"id"`
	TripID      string     `json:"trip_id"`
	OwnerUserID string     `json:"owner_user_id,omitempty"`
	GuestEmail  string     `json:"guest_email"`
	GuestUserID string     `json:"guest_user_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

type Agency struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Description string `json:"description"`
}

type FavoriteAgency struct {
	UserID    string    `json:"user_id,omitempty"`
	AgencyID  string    `json:"agency_id"`
	CreatedAt time.Time `json:"created_at"`
	Agency    *Agency   `json:"agency,omitempty"`
}

type Recommendation struct {
	ID          string     `json:"id"`
	AgencyID    string     `json:"agency_id"`
	AgencyName  string     `json:"agency_name,omitempty"`
	Title       string     `json:"title"`
	Destination string     `json:"destination"`
	Description string     `json:"description"`
	PriceCents  int64      `json:"price_cents"`
	Currency    string     `json:"currency"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
	Active      bool       `json:"active"`
	Discarded   bool       `json:"discarded"`
}

type AgencyRequest struct {
	ID               string         `json:"id"`
	UserID           string         `json:"user_id,omitempty"`
	AgencyID         string         `json:"agency_id"`
	AgencyName       string         `json:"agency_name,omitempty"`
	ReservationID    string         `json:"reservation_id,omitempty"`
	RecommendationID string         `json:"recommendation_id,omitempty"`
	Message          string         `json:"message"`
	Status           string         `json:"status"`
	Context          map[string]any `json:"context,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	ClosedAt         *time.Time     `json:"closed_at,omitempty"`
}

type AuditLog struct {
	ID         string         `json:"id"`
	UserID     string         `json:"user_id,omitempty"`
	Action     string         `json:"action"`
	Resource   string         `json:"resource"`
	ResourceID string         `json:"resource_id"`
	IP         string         `json:"ip"`
	Details    map[string]any `json:"details,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type AnalyticsFact struct {
	ID          string     `json:"id"`
	Destination string     `json:"destination"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
	Status      string     `json:"status"`
	Provider    string     `json:"provider"`
	Type        string     `json:"type"`
	AgencyID    string     `json:"agency_id,omitempty"`
	OccurredAt  time.Time  `json:"occurred_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type AnalyticsSummary struct {
	From            time.Time         `json:"from"`
	To              time.Time         `json:"to"`
	Trips           int64             `json:"trips"`
	Reservations    int64             `json:"reservations"`
	Canceled        int64             `json:"canceled"`
	ByType          []AnalyticsBucket `json:"by_type"`
	ByProvider      []AnalyticsBucket `json:"by_provider"`
	TopDestinations []AnalyticsBucket `json:"top_destinations"`
	Monthly         []AnalyticsBucket `json:"monthly"`
}

type AnalyticsBucket struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"

	"trotamundos/internal/agencies"
	"trotamundos/internal/analytics"
	"trotamundos/internal/auth"
	"trotamundos/internal/ingestion"
	"trotamundos/internal/itinerary"
	"trotamundos/internal/platform/httpx"
	"trotamundos/internal/providers"
	"trotamundos/internal/recommendation"
	"trotamundos/internal/sharing"
	"trotamundos/internal/social"
	"trotamundos/internal/web"
)

type Deps struct {
	Authenticator  *auth.Authenticator
	Auth           *auth.Handler
	Itinerary      *itinerary.Handler
	Ingestion      *ingestion.Handler
	Agencies       *agencies.Handler
	Recommendation *recommendation.Handler
	Sharing        *sharing.Handler
	Social         *social.Handler
	Analytics      *analytics.Handler
	Providers      *providers.Handler
	MockKey        string
	APIKeys        []string
	WebAPIKey      string
	RateLimiter    httpx.RateLimiter
	Health         func(r *http.Request) error
	Logger         *slog.Logger
	AllowedOrigin  string
}

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	protected := d.Authenticator.Require
	analyst := func(h http.HandlerFunc) http.Handler {
		return protected(auth.RequireRoles("analyst", "admin")(h))
	}
	guard := func(h http.HandlerFunc) http.Handler { return protected(h) }

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if d.Health != nil {
			if err := d.Health(r); err != nil {
				httpx.Error(w, http.StatusServiceUnavailable, "not_ready", "dependencias no disponibles")
				return
			}
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	mux.HandleFunc("POST /auth/register", d.Auth.Register)
	mux.HandleFunc("POST /auth/login", d.Auth.Login)
	mux.HandleFunc("POST /auth/refresh", d.Auth.Refresh)
	mux.HandleFunc("POST /auth/logout", d.Auth.Logout)
	mux.Handle("GET /auth/me", guard(d.Auth.Me))

	mux.Handle("GET /trips", guard(d.Itinerary.ListTrips))
	mux.Handle("GET /trips/{id}", guard(d.Itinerary.GetTrip))
	mux.Handle("DELETE /trips/{id}", guard(d.Itinerary.DeleteTrip))
	mux.Handle("POST /trips/{id}/reservations", guard(d.Itinerary.CreateReservation))
	mux.Handle("POST /trips/{id}/context-location", guard(d.Itinerary.LocationContext))
	mux.Handle("GET /trips/{id}/shares", guard(d.Sharing.ListShares))
	mux.Handle("POST /trips/{id}/shares", guard(d.Sharing.Share))
	mux.Handle("POST /trips/{id}/social/preview", guard(d.Social.Preview))
	mux.Handle("POST /trips/{id}/social/share", guard(d.Social.Publish))
	mux.Handle("DELETE /shares/{id}", guard(d.Sharing.Revoke))
	mux.Handle("GET /shared-with-me", guard(d.Sharing.SharedWithMe))
	mux.Handle("GET /shared/{id}", guard(d.Sharing.GetSharedTrip))

	mux.Handle("POST /reservations", guard(d.Itinerary.CreateReservation))
	mux.Handle("GET /reservations/{id}", guard(d.Itinerary.GetReservation))
	mux.Handle("PATCH /reservations/{id}", guard(d.Itinerary.UpdateReservation))
	mux.Handle("DELETE /reservations/{id}", guard(d.Itinerary.DeleteReservation))
	mux.Handle("POST /reservations/{id}/sync", guard(d.Itinerary.SyncReservation))
	mux.Handle("GET /reservations/{id}/changes", guard(d.Itinerary.ListChanges))
	mux.Handle("POST /reservations/{id}/agency-request", guard(d.Agencies.ContactReservation))

	mux.Handle("GET /notifications", guard(d.Itinerary.ListNotifications))
	mux.Handle("POST /notifications/{id}/read", guard(d.Itinerary.MarkNotificationRead))

	mux.Handle("GET /mail/accounts", guard(d.Ingestion.ListAccounts))
	mux.Handle("POST /mail/accounts", guard(d.Ingestion.LinkMailbox))
	mux.Handle("DELETE /mail/accounts/{id}", guard(d.Ingestion.RevokeMailbox))
	mux.Handle("POST /mail/accounts/{id}/scan", guard(d.Ingestion.ScanMailbox))
	mux.Handle("GET /mail/events", guard(d.Ingestion.ListEvents))
	mux.Handle("GET /mail/whitelist", guard(d.Ingestion.ListWhitelist))
	mux.Handle("POST /mail/whitelist", guard(d.Ingestion.AddWhitelist))
	mux.Handle("DELETE /mail/whitelist/{id}", guard(d.Ingestion.DeleteWhitelist))
	mux.Handle("GET /mail/filters", guard(d.Ingestion.ListFilters))
	mux.Handle("POST /mail/filters", guard(d.Ingestion.AddFilter))
	mux.Handle("PATCH /mail/filters/{id}", guard(d.Ingestion.UpdateFilter))
	mux.Handle("DELETE /mail/filters/{id}", guard(d.Ingestion.DeleteFilter))
	mux.HandleFunc("POST /webhooks/mail", d.Ingestion.Webhook)
	mux.HandleFunc("POST /mock/mail/{email}/messages", requireKey(d.MockKey, d.Ingestion.MockAddMail))
	mux.HandleFunc("PUT /mock/providers/{provider}/reservations/{locator}", requireKey(d.MockKey, d.Providers.Seed))
	mux.HandleFunc("DELETE /mock/providers/{provider}/reservations/{locator}", requireKey(d.MockKey, d.Providers.Cancel))
	mux.HandleFunc("POST /mock/providers/{provider}/fail", requireKey(d.MockKey, d.Providers.Fail))
	mux.HandleFunc("POST /mock/agencies/{id}/fail", requireKey(d.MockKey, d.Providers.AgencyFail))

	mux.Handle("GET /agencies", guard(d.Recommendation.Agencies))
	mux.Handle("GET /agencies/favorite", guard(d.Recommendation.Favorite))
	mux.Handle("PUT /agencies/favorite", guard(d.Recommendation.SetFavorite))
	mux.Handle("DELETE /agencies/favorite", guard(d.Recommendation.DeleteFavorite))
	mux.Handle("GET /me/favorite-agency", guard(d.Recommendation.Favorite))
	mux.Handle("PUT /me/favorite-agency", guard(d.Recommendation.SetFavorite))
	mux.Handle("DELETE /me/favorite-agency", guard(d.Recommendation.DeleteFavorite))
	mux.Handle("GET /me/preferences", guard(d.Recommendation.Preferences))
	mux.Handle("PUT /me/preferences", guard(d.Recommendation.UpdatePreferences))
	mux.Handle("GET /recommendations", guard(d.Recommendation.Recommendations))
	mux.Handle("POST /recommendations/{id}/feedback", guard(d.Recommendation.Feedback))
	mux.Handle("POST /recommendations/{id}/info-request", guard(d.Recommendation.InfoRequest))
	mux.Handle("POST /recommendations/{id}/discard", guard(d.Recommendation.Discard))
	mux.Handle("POST /recommendations/{id}/inquire", guard(d.Recommendation.Inquire))
	mux.Handle("GET /agency-requests", guard(d.Agencies.ListRequests))
	mux.Handle("POST /agency-requests", guard(d.Recommendation.CreateAgencyRequest))
	mux.Handle("GET /agency-requests/{id}", guard(d.Recommendation.GetRequest))
	mux.Handle("POST /agency-requests/{id}/close", guard(d.Agencies.CloseRequest))
	mux.HandleFunc("GET /social/networks", d.Social.Networks)

	mux.Handle("GET /analytics/summary", analyst(d.Analytics.Summary))
	mux.Handle("GET /analytics/facts", analyst(d.Analytics.Facts))

	mux.Handle("GET /assets/", web.Assets())
	mux.Handle("GET /api-config.js", web.Config(d.WebAPIKey))
	mux.HandleFunc("GET /", web.Index)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, http.StatusNotFound, "not_found", "endpoint no encontrado")
	})

	handler := httpx.Chain(mux,
		httpx.Recover(d.Logger),
		httpx.RequestID,
		httpx.Logger(d.Logger),
		httpx.CORS(d.AllowedOrigin),
	)
	handler = withAPIKey(d.APIKeys, handler)
	handler = withRateLimit(d.RateLimiter, handler)
	return handler
}

// withAPIKey aplica CON-01: cuando hay claves configuradas, los controladores
// exigen X-API-Key además del JWT. En desarrollo la lista puede estar vacía.
func withAPIKey(keys []string, next http.Handler) http.Handler {
	if len(keys) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicRoute(r) {
			next.ServeHTTP(w, r)
			return
		}
		presented := strings.TrimSpace(r.Header.Get("X-API-Key"))
		for _, key := range keys {
			if subtle.ConstantTimeCompare([]byte(presented), []byte(key)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}
		httpx.Error(w, http.StatusUnauthorized, "invalid_api_key", "API key inválida")
	})
}

func withRateLimit(limiter httpx.RateLimiter, next http.Handler) http.Handler {
	if limiter == nil {
		return next
	}
	limited := httpx.RateLimit(limiter, func(r *http.Request) string {
		return httpx.ClientIP(r) + "|" + r.Method + " " + r.URL.Path
	}, next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicRoute(r) {
			next.ServeHTTP(w, r)
			return
		}
		limited.ServeHTTP(w, r)
	})
}

func isPublicRoute(r *http.Request) bool {
	p := r.URL.Path
	return p == "/healthz" || p == "/readyz" || p == "/" || p == "/api-config.js" ||
		strings.HasPrefix(p, "/assets/") || strings.HasPrefix(p, "/mock/") ||
		p == "/webhooks/mail" || p == "/social/networks" ||
		strings.HasPrefix(p, "/auth/")
}

func requireKey(key string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if key == "" || r.Header.Get("X-Mock-Key") != key {
			httpx.Error(w, http.StatusUnauthorized, "invalid_mock_key", "clave de mock inválida")
			return
		}
		next(w, r)
	}
}

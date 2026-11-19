package httpx

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"
)

// RateLimiter es la abstracción mínima que necesita el middleware HTTP. La
// implementación Redis vive en internal/platform/ratelimit.
type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

// RateLimit responde 429 {"error":"rate_limited"} cuando el limitador rechaza
// el request. keyFunc permite elegir la clave (por defecto, la IP del cliente).
// Si el limitador falla se aplica fail-open (QA-05): un problema transitorio de
// Redis no debe caer todo el servicio. La decisión se registra en el middleware
// de logging; el ataque sostenido se mitigaa con la cuota cuando Redis vuelve.
func RateLimit(l RateLimiter, keyFunc func(*http.Request) string, next http.Handler) http.Handler {
	if l == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := ""
		if keyFunc != nil {
			key = keyFunc(r)
		}
		if key == "" {
			key = ClientIP(r)
		}

		allowed, err := l.Allow(r.Context(), key)
		if err != nil {
			// Fail-open deliberado por QA-05 (disponibilidad).
			next.ServeHTTP(w, r)
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(l)))
			Error(w, http.StatusTooManyRequests, "rate_limited", "demasiadas solicitudes")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// retryAfterSeconds usa la ventana del limitador cuando la expone (así el
// cliente sabe cuándo reintentar) y un valor conservador de 1s en caso
// contrario.
func retryAfterSeconds(l RateLimiter) int {
	if wl, ok := l.(interface{ Window() time.Duration }); ok && wl.Window() > 0 {
		seconds := int(math.Ceil(wl.Window().Seconds()))
		if seconds < 1 {
			return 1
		}
		return seconds
	}
	return 1
}

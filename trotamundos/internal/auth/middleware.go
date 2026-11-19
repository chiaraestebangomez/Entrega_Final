package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"trotamundos/internal/platform/httpx"
	"trotamundos/internal/platform/jwtx"
)

type Principal struct {
	UserID string
	Email  string
	Role   string
	JTI    string
}

type principalKey struct{}

type Authenticator struct {
	verifier *jwtx.Signer
	rev      TokenRevocationStore
	now      func() time.Time
}

func NewAuthenticator(verifier *jwtx.Signer) *Authenticator {
	return &Authenticator{verifier: verifier, now: time.Now}
}

// NewAuthenticatorWithRevocation habilita la denylist de JTI (ADR-4.1).
//
// Decisión sobre fallos de la denylist: cualquier error de IsRevokedJTI se
// maneja fall-closed respondiendo 503 "revocation_unavailable". No se hace
// fail-open ni siquiera para errores de conexión transitorios: dejar pasar el
// request podría aceptar un access token revocado en el logout. El costo es
// indisponibilidad temporal mientras la denylist no esté accesible.
func NewAuthenticatorWithRevocation(verifier *jwtx.Signer, rev TokenRevocationStore) *Authenticator {
	return &Authenticator{verifier: verifier, rev: rev, now: time.Now}
}

// auditDeny registra todo acceso rechazado con usuario, recurso, motivo e IP,
// de modo que QA-15 ("bloquea y audita el 100 % de los accesos no autorizados")
// sea verificable en los logs, no sólo en la respuesta HTTP.
func auditDeny(r *http.Request, reason string, userID string) {
	slog.Warn("auth access denied",
		"event", "auth.denied",
		"reason", reason,
		"user_id", userID,
		"resource", r.URL.Path,
		"method", r.Method,
		"ip", httpx.ClientIP(r),
	)
}

func (a *Authenticator) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerFromHeader(r.Header.Get("Authorization"))
		if token == "" {
			auditDeny(r, "missing_token", "")
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "token de acceso requerido")
			return
		}
		claims, err := a.verifier.Parse(token, a.now())
		if err != nil {
			if errors.Is(err, jwtx.ErrExpiredToken) {
				auditDeny(r, "token_expired", "")
				httpx.Error(w, http.StatusUnauthorized, "token_expired", "el token de acceso expiró")
				return
			}
			auditDeny(r, "invalid_token", "")
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "token de acceso inválido")
			return
		}
		if a.rev != nil {
			revoked, err := a.rev.IsRevokedJTI(r.Context(), claims.JTI)
			if err != nil {
				auditDeny(r, "revocation_unavailable", claims.Subject)
				httpx.Error(w, http.StatusServiceUnavailable, "revocation_unavailable", "no se pudo verificar la revocación del token")
				return
			}
			if revoked {
				auditDeny(r, "token_revoked", claims.Subject)
				httpx.Error(w, http.StatusUnauthorized, "token_revoked", "el token de acceso fue revocado")
				return
			}
		}

		p := Principal{UserID: claims.Subject, Email: claims.Email, Role: claims.Role, JTI: claims.JTI}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

func RequireRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok || !allowed[p.Role] {
				auditDeny(r, "forbidden_role", func() string {
					if ok {
						return p.UserID
					}
					return ""
				}())
				httpx.Error(w, http.StatusForbidden, "forbidden", "no tenés permisos para esta operación")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

func MustPrincipal(ctx context.Context) Principal {
	p, _ := PrincipalFrom(ctx)
	return p
}

func bearerFromHeader(header string) string {
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

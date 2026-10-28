package auth

import (
	"context"
	"errors"
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
	now      func() time.Time
}

func NewAuthenticator(verifier *jwtx.Signer) *Authenticator {
	return &Authenticator{verifier: verifier, now: time.Now}
}

func (a *Authenticator) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "token de acceso requerido")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		claims, err := a.verifier.Parse(token, a.now())
		if err != nil {
			if errors.Is(err, jwtx.ErrExpiredToken) {
				httpx.Error(w, http.StatusUnauthorized, "token_expired", "el token de acceso expiró")
				return
			}
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "token de acceso inválido")
			return
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

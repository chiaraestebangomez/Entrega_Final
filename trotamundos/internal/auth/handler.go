package auth

import (
	"errors"
	"net/http"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Password string `json:"password"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	user, pair, err := h.svc.Register(r.Context(), body.Email, body.FullName, body.Password)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"user": user, "tokens": pair})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	user, pair, err := h.svc.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": user, "tokens": pair})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	user, pair, err := h.svc.Refresh(r.Context(), body.RefreshToken)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": user, "tokens": pair})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = httpx.Decode(r, &body)
	if err := h.svc.Logout(r.Context(), body.RefreshToken); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "no se pudo cerrar la sesión")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p := MustPrincipal(r.Context())
	user, err := h.svc.ByID(r.Context(), p.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found", "usuario no encontrado")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "no se pudo obtener el usuario")
		return
	}
	httpx.JSON(w, http.StatusOK, user)
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		httpx.Error(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, domain.ErrConflict):
		httpx.Error(w, http.StatusConflict, "email_already_registered", "el email ya está registrado")
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, ErrRefreshNotFound):
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", "credenciales inválidas")
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "error inesperado")
	}
}

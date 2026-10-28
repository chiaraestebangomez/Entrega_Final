package sharing

import (
	"errors"
	"log/slog"
	"net/http"

	"trotamundos/internal/auth"
	"trotamundos/internal/domain"
	"trotamundos/internal/platform/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) ListShares(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	out, err := h.svc.ListByTrip(r.Context(), p.UserID, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) Share(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		GuestEmail string `json:"guest_email"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	out, err := h.svc.Share(r.Context(), p.UserID, p.Email, r.PathValue("id"), body.GuestEmail)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	if err := h.svc.Revoke(r.Context(), p.UserID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SharedWithMe(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	out, err := h.svc.SharedWithMe(r.Context(), p.UserID, p.Email)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) GetSharedTrip(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	out, err := h.svc.Get(r.Context(), p.UserID, p.Email, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "recurso no encontrado")
	case errors.Is(err, domain.ErrValidation):
		httpx.Error(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, domain.ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden", "no tenés acceso a este viaje")
	default:
		slog.Error("sharing error", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "error inesperado")
	}
}

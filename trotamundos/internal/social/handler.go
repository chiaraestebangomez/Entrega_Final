package social

import (
	"errors"
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

func (h *Handler) Networks(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"data": h.svc.Networks()})
}

func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	network, err := decodeNetwork(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	out, err := h.svc.BuildPreview(r.Context(), p.UserID, r.PathValue("id"), network)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	network, err := decodeNetwork(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	out, err := h.svc.Publish(r.Context(), p.UserID, r.PathValue("id"), network)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func decodeNetwork(r *http.Request) (string, error) {
	var body struct {
		Network string `json:"network"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		return "", err
	}
	return body.Network, nil
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "viaje no encontrado")
	case errors.Is(err, domain.ErrValidation):
		httpx.Error(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "error inesperado")
	}
}

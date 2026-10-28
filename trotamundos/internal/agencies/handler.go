package agencies

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

func (h *Handler) ListAgencies(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListAgencies(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) GetFavorite(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	f, err := h.svc.Favorite(r.Context(), p.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"favorite": f})
}

func (h *Handler) SetFavorite(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		AgencyID string `json:"agency_id"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	f, err := h.svc.SetFavorite(r.Context(), p.UserID, body.AgencyID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, f)
}

func (h *Handler) DeleteFavorite(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	confirm := r.URL.Query().Get("confirm") == "true"
	if err := h.svc.DeleteFavorite(r.Context(), p.UserID, confirm); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListRecommendations(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	out, err := h.svc.Recommendations(r.Context(), p.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) Discard(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	if err := h.svc.Discard(r.Context(), p.UserID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Inquire(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		Message string `json:"message"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	req, err := h.svc.Inquire(r.Context(), p.UserID, r.PathValue("id"), body.Message)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, req)
}

func (h *Handler) ContactReservation(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		Message string `json:"message"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	req, err := h.svc.ContactAboutReservation(r.Context(), p.UserID, r.PathValue("id"), body.Message)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, req)
}

func (h *Handler) ListRequests(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	out, err := h.svc.Requests(r.Context(), p.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) GetRequest(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	out, err := h.svc.Request(r.Context(), p.UserID, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) CloseRequest(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	if err := h.svc.CloseRequest(r.Context(), p.UserID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "recurso no encontrado")
	case errors.Is(err, domain.ErrValidation):
		httpx.Error(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, domain.ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden", "la recomendación no pertenece a tu agencia favorita")
	case errors.Is(err, domain.ErrConflict):
		httpx.Error(w, http.StatusConflict, "conflict", "operación en conflicto")
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "error inesperado")
	}
}

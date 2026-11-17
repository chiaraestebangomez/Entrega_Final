package recommendation

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

func (h *Handler) Agencies(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Agencies(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) Favorite(w http.ResponseWriter, r *http.Request) {
	userID := auth.MustPrincipal(r.Context()).UserID
	fav, err := h.svc.agencies.Favorite(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"favorite": fav})
}

func (h *Handler) SetFavorite(w http.ResponseWriter, r *http.Request) {
	userID := auth.MustPrincipal(r.Context()).UserID
	var body struct {
		AgencyID string `json:"agency_id"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	fav, err := h.svc.SetFavorite(r.Context(), userID, body.AgencyID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, fav)
}

func (h *Handler) DeleteFavorite(w http.ResponseWriter, r *http.Request) {
	userID := auth.MustPrincipal(r.Context()).UserID
	if r.URL.Query().Get("confirm") != "true" {
		httpx.Error(w, http.StatusBadRequest, "confirmation_required", "se requiere confirmación")
		return
	}
	if err := h.svc.DeleteFavorite(r.Context(), userID, true); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Preferences(w http.ResponseWriter, r *http.Request) {
	userID := auth.MustPrincipal(r.Context()).UserID
	prefs, err := h.svc.Preferences(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"preferences": prefs})
}

func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := auth.MustPrincipal(r.Context()).UserID
	var prefs domain.RecommendationPreferences
	if err := httpx.Decode(r, &prefs); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	out, err := h.svc.UpdatePreferences(r.Context(), userID, prefs)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) Recommendations(w http.ResponseWriter, r *http.Request) {
	userID := auth.MustPrincipal(r.Context()).UserID
	out, err := h.svc.Recommendations(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) Feedback(w http.ResponseWriter, r *http.Request) {
	userID := auth.MustPrincipal(r.Context()).UserID
	var body domain.RecommendationFeedback
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	out, err := h.svc.Feedback(r.Context(), userID, r.PathValue("id"), body)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) InfoRequest(w http.ResponseWriter, r *http.Request) {
	userID := auth.MustPrincipal(r.Context()).UserID
	var body struct {
		Message string `json:"message"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	req, err := h.svc.InfoRequest(r.Context(), userID, r.PathValue("id"), body.Message)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, req)
}

func (h *Handler) CreateAgencyRequest(w http.ResponseWriter, r *http.Request) {
	userID := auth.MustPrincipal(r.Context()).UserID
	var body struct {
		RecommendationID string `json:"recommendation_id"`
		ReservationID    string `json:"reservation_id"`
		Message          string `json:"message"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	req, err := h.svc.CreateAgencyRequest(r.Context(), userID, body.RecommendationID, body.ReservationID, body.Message)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, req)
}

func (h *Handler) GetRequest(w http.ResponseWriter, r *http.Request) {
	userID := auth.MustPrincipal(r.Context()).UserID
	req, err := h.svc.AgencyRequest(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, req)
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "recurso no encontrado")
	case errors.Is(err, domain.ErrValidation):
		httpx.Error(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, domain.ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden", "recurso no disponible para este usuario")
	case errors.Is(err, domain.ErrConflict):
		httpx.Error(w, http.StatusConflict, "conflict", "operación en conflicto")
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "error inesperado")
	}
}

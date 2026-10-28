package itinerary

import (
	"errors"
	"net/http"
	"strconv"

	"trotamundos/internal/auth"
	"trotamundos/internal/domain"
	"trotamundos/internal/platform/httpx"
)

type principal = auth.Principal

func principalFrom(r *http.Request) (auth.Principal, bool) {
	return auth.PrincipalFrom(r.Context())
}

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) ListTrips(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	archived, _ := strconv.ParseBool(r.URL.Query().Get("archived"))
	trips, err := h.svc.ListTrips(r.Context(), p.UserID, archived)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": trips})
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	trip, err := h.svc.GetTrip(r.Context(), p.UserID, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, trip)
}

func (h *Handler) DeleteTrip(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	confirm, _ := strconv.ParseBool(r.URL.Query().Get("confirm"))
	if err := h.svc.DeleteTrip(r.Context(), p.UserID, r.PathValue("id"), confirm); err != nil {
		writeError(w, err)
		return
	}
	h.svc.RecordAudit(r.Context(), p.UserID, "trip.delete", "trip", r.PathValue("id"), httpx.ClientIP(r), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateReservation(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	var in ReservationInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	if tripID := r.PathValue("id"); tripID != "" {
		in.TripID = tripID
	}
	res, err := h.svc.CreateReservation(r.Context(), p.UserID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	h.svc.RecordAudit(r.Context(), p.UserID, "reservation.create", "reservation", res.ID, httpx.ClientIP(r), nil)
	httpx.JSON(w, http.StatusCreated, res)
}

func (h *Handler) GetReservation(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	res, err := h.svc.repo.GetReservation(r.Context(), p.UserID, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	changes, _ := h.svc.ListChanges(r.Context(), p.UserID, res.ID)
	res.Changes = changes
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) UpdateReservation(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	var in ReservationInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	res, err := h.svc.UpdateReservation(r.Context(), p.UserID, r.PathValue("id"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	h.svc.RecordAudit(r.Context(), p.UserID, "reservation.update", "reservation", res.ID, httpx.ClientIP(r), nil)
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) DeleteReservation(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	confirm, _ := strconv.ParseBool(r.URL.Query().Get("confirm"))
	id := r.PathValue("id")
	if err := h.svc.DeleteReservation(r.Context(), p.UserID, id, confirm); err != nil {
		writeError(w, err)
		return
	}
	h.svc.RecordAudit(r.Context(), p.UserID, "reservation.delete", "reservation", id, httpx.ClientIP(r), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SyncReservation(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	res, err := h.svc.SyncReservation(r.Context(), p.UserID, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) ListChanges(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	out, err := h.svc.ListChanges(r.Context(), p.UserID, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	out, err := h.svc.ListNotifications(r.Context(), p.UserID, 50)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	if err := h.svc.MarkNotificationRead(r.Context(), p.UserID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) LocationContext(w http.ResponseWriter, r *http.Request) {
	p := mustPrincipal(r)
	consent, _ := strconv.ParseBool(r.URL.Query().Get("consent"))
	var lat, lng *float64
	if v := r.URL.Query().Get("lat"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			lat = &f
		}
	}
	if v := r.URL.Query().Get("lng"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			lng = &f
		}
	}
	out, err := h.svc.LocationContext(r.Context(), p.UserID, r.PathValue("id"), consent, lat, lng)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func mustPrincipal(r *http.Request) principal {
	p, _ := principalFrom(r)
	return p
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "recurso no encontrado")
	case errors.Is(err, domain.ErrValidation):
		httpx.Error(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, domain.ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden", "no tenés permisos para esta operación")
	case errors.Is(err, domain.ErrConflict):
		httpx.Error(w, http.StatusConflict, "conflict", "conflicto de datos")
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "error inesperado")
	}
}

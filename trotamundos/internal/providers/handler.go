package providers

import (
	"net/http"
	"time"

	"trotamundos/internal/platform/httpx"
)

type Handler struct {
	store *MockStore
	now   func() time.Time
}

func NewHandler(store *MockStore) *Handler {
	return &Handler{store: store, now: time.Now}
}

func (h *Handler) Seed(w http.ResponseWriter, r *http.Request) {
	var body Snapshot
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	provider := r.PathValue("provider")
	locator := r.PathValue("locator")
	body.Provider = provider
	body.Locator = locator
	if body.Status == "" {
		body.Status = "confirmed"
	}
	if body.UpdatedAt.IsZero() {
		body.UpdatedAt = h.now().UTC()
	}
	h.store.Set(provider, locator, body)
	httpx.JSON(w, http.StatusOK, body)
}

func (h *Handler) Fail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	h.store.SetFailure(r.PathValue("provider"), body.Enabled)
	httpx.JSON(w, http.StatusOK, map[string]any{"provider": r.PathValue("provider"), "failing": body.Enabled})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	if !h.store.Cancel(r.PathValue("provider"), r.PathValue("locator")) {
		httpx.Error(w, http.StatusNotFound, "not_found", "reserva mock no encontrada")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

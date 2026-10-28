package ingestion

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"trotamundos/internal/auth"
	"trotamundos/internal/domain"
	"trotamundos/internal/mailer"
	"trotamundos/internal/platform/httpx"
)

type Handler struct {
	svc        *Service
	connector  *mailer.MockConnector
	webhookKey string
}

func NewHandler(svc *Service, connector *mailer.MockConnector, webhookKey string) *Handler {
	return &Handler{svc: svc, connector: connector, webhookKey: webhookKey}
}

func (h *Handler) LinkMailbox(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var in LinkMailboxInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	acc, err := h.svc.LinkMailbox(r.Context(), p.UserID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, acc)
}

func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	out, err := h.svc.ListAccounts(r.Context(), p.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) RevokeMailbox(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	if err := h.svc.RevokeMailbox(r.Context(), p.UserID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ScanMailbox(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	n, err := h.svc.ScanMailbox(r.Context(), p.UserID, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]any{"events_created": n})
}

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	out, err := h.svc.ListEvents(r.Context(), p.UserID, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) ListWhitelist(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	out, err := h.svc.ListWhitelist(r.Context(), p.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) AddWhitelist(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		Pattern string `json:"pattern"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	out, err := h.svc.AddWhitelist(r.Context(), p.UserID, body.Pattern)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) DeleteWhitelist(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	if err := h.svc.DeleteWhitelist(r.Context(), p.UserID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListFilters(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	out, err := h.svc.ListFilters(r.Context(), p.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) AddFilter(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var in domain.MailFilter
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	out, err := h.svc.AddFilter(r.Context(), p.UserID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) UpdateFilter(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var in domain.MailFilter
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	out, err := h.svc.UpdateFilter(r.Context(), p.UserID, r.PathValue("id"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) DeleteFilter(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	if err := h.svc.DeleteFilter(r.Context(), p.UserID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	if !secureEqual(r.Header.Get("X-Webhook-Key"), h.webhookKey) {
		httpx.Error(w, http.StatusUnauthorized, "invalid_webhook_key", "clave de webhook inválida")
		return
	}
	var in InboundEmail
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	ev, err := h.svc.ReceiveWebhook(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"event_id": ev.ID, "status": ev.Status})
}

func (h *Handler) MockAddMail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From       string `json:"from"`
		Subject    string `json:"subject"`
		Body       string `json:"body"`
		ReceivedAt string `json:"received_at"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "cuerpo JSON inválido")
		return
	}
	msg := mailer.Message{From: body.From, Subject: body.Subject, Body: body.Body}
	if body.ReceivedAt != "" {
		if t := parseDate(body.ReceivedAt); t != nil {
			msg.ReceivedAt = *t
		}
	}
	h.connector.Add(r.PathValue("email"), msg)
	httpx.JSON(w, http.StatusCreated, map[string]any{"status": "queued_in_mock_inbox"})
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
		httpx.Error(w, http.StatusConflict, "conflict", "la cuenta de correo ya está vinculada")
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "error inesperado")
	}
}

func secureEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(a)), []byte(strings.TrimSpace(b))) == 1
}

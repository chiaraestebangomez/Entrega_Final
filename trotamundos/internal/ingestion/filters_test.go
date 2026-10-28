package ingestion

import (
	"testing"

	"trotamundos/internal/domain"
	"trotamundos/internal/mailer"
)

func TestSenderMatches(t *testing.T) {
	cases := []struct {
		pattern string
		sender  string
		want    bool
	}{
		{"*@aerolineas.com", "Reservas <no-reply@aerolineas.com>", true},
		{"aerolineas.com", "no-reply@mail.aerolineas.com", true},
		{"*@booking.com", "no-reply@aerolineas.com", false},
		{"persona@example.com", "Persona <persona@example.com>", true},
	}
	for _, c := range cases {
		if got := SenderMatches(c.pattern, c.sender); got != c.want {
			t.Errorf("SenderMatches(%q,%q)=%v want %v", c.pattern, c.sender, got, c.want)
		}
	}
}

func TestShouldProcessWhitelistAndFilters(t *testing.T) {
	msg := mailer.Message{
		From: "reservas@aerolineas.com", Subject: "Confirmación de vuelo",
		Body: "Tu vuelo a Bariloche fue confirmado. Locator AR7X2K",
	}
	if !ShouldProcess(nil, nil, msg) {
		t.Fatal("sin reglas el mensaje debe procesarse")
	}
	if !ShouldProcess([]domain.WhitelistEntry{{Pattern: "*@aerolineas.com"}}, nil, msg) {
		t.Fatal("remitente whitelisted debe procesarse")
	}
	if ShouldProcess([]domain.WhitelistEntry{{Pattern: "*@booking.com"}}, nil, msg) {
		t.Fatal("remitente no whitelisted no debe procesarse")
	}
	filters := []domain.MailFilter{
		{Field: "subject", Operator: "contains", Value: "vuelo", Active: true},
		{Field: "body", Operator: "not_contains", Value: "cancelado", Active: true},
	}
	if !ShouldProcess(nil, filters, msg) {
		t.Fatal("filtros compatibles deben permitir el mensaje")
	}
	filters[0].Value = "hotel"
	if ShouldProcess(nil, filters, msg) {
		t.Fatal("un filtro que no coincide debe descartar el mensaje")
	}
	inactive := []domain.MailFilter{{Field: "subject", Operator: "contains", Value: "hotel", Active: false}}
	if !ShouldProcess(nil, inactive, msg) {
		t.Fatal("los filtros inactivos deben ignorarse")
	}
}

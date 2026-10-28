package ingestion

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/mailer"
)

var ErrUnparseable = errors.New("email sin datos de reserva reconocibles")

type ParsedReservation struct {
	Type        string
	Provider    string
	Locator     string
	Title       string
	Destination string
	StartAt     *time.Time
	EndAt       *time.Time
	Details     map[string]any
}

func ParseMessage(msg mailer.Message) (ParsedReservation, error) {
	body := strings.TrimSpace(msg.Body)
	var p ParsedReservation
	if strings.HasPrefix(body, "{") {
		p = parseJSONBody(body)
	} else {
		p = parseKeyValueBody(body)
	}
	if p.Type == "" {
		p.Type = inferType(msg.Subject + " " + msg.Body)
	}
	if p.Provider == "" {
		p.Provider = inferProvider(msg.From)
	}
	if p.Title == "" {
		p.Title = strings.TrimSpace(msg.Subject)
	}
	if p.Destination == "" {
		p.Destination = inferDestination(p.Title)
	}
	if p.Details == nil {
		p.Details = map[string]any{}
	}
	if msg.From != "" {
		p.Details["sender"] = msg.From
	}
	if msg.Subject != "" {
		p.Details["subject"] = msg.Subject
	}
	if p.Locator == "" {
		p.Locator = inferLocator(msg.Subject + "\n" + msg.Body)
	}
	if p.Provider == "" && p.Locator == "" && p.Title == "" {
		return ParsedReservation{}, ErrUnparseable
	}
	if p.Type == "" {
		p.Type = domain.TypeOther
	}
	return p, nil
}

func parseJSONBody(body string) ParsedReservation {
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return ParsedReservation{}
	}
	get := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
		return ""
	}
	p := ParsedReservation{
		Type:        get("type", "tipo"),
		Provider:    get("provider", "proveedor"),
		Locator:     get("locator", "localizador", "code", "codigo"),
		Title:       get("title", "titulo"),
		Destination: get("destination", "destino"),
		Details:     map[string]any{},
	}
	p.StartAt = parseDate(get("start_at", "start", "inicio", "from"))
	p.EndAt = parseDate(get("end_at", "end", "fin", "to"))
	if d, ok := raw["details"].(map[string]any); ok {
		p.Details = d
	}
	return p
}

func parseKeyValueBody(body string) ParsedReservation {
	p := ParsedReservation{Details: map[string]any{}}
	for _, line := range strings.Split(body, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		switch key {
		case "provider", "proveedor", "aerolinea", "aerolínea", "hotel", "rentadora":
			if p.Provider == "" {
				p.Provider = value
			}
		case "locator", "localizador", "codigo", "código", "reserva", "confirmation":
			if p.Locator == "" {
				p.Locator = value
			}
		case "type", "tipo":
			p.Type = strings.ToLower(value)
		case "title", "titulo", "título":
			p.Title = value
		case "destination", "destino", "ciudad":
			p.Destination = value
		case "start", "start_at", "inicio", "desde", "check-in", "checkin", "departure":
			p.StartAt = parseDate(value)
		case "end", "end_at", "fin", "hasta", "check-out", "checkout", "arrival":
			p.EndAt = parseDate(value)
		case "details", "detalle", "detalles":
			p.Details["notes"] = value
		}
	}
	return p
}

func parseDate(v string) *time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"02/01/2006 15:04",
		"02/01/2006",
		"02-01-2006",
	}
	for _, layout := range layouts {
		var t time.Time
		var err error
		if strings.Contains(layout, "Z07") {
			t, err = time.Parse(layout, v)
		} else {
			t, err = time.ParseInLocation(layout, v, time.UTC)
		}
		if err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}

func inferType(text string) string {
	t := strings.ToLower(text)
	switch {
	case containsAny(t, "vuelo", "flight", "boarding", "aerolinea", "aerolínea"):
		return domain.TypeFlight
	case containsAny(t, "hotel", "habitaci", "check-in", "check in", "alojamiento"):
		return domain.TypeHotel
	case containsAny(t, "auto", "car ", "vehiculo", "vehículo", "rentadora", "rental"):
		return domain.TypeCar
	default:
		return domain.TypeOther
	}
}

func inferProvider(from string) string {
	from = strings.ToLower(strings.TrimSpace(from))
	if at := strings.LastIndex(from, "@"); at >= 0 {
		from = from[at+1:]
	}
	from = strings.Trim(from, "> ")
	parts := strings.Split(from, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

var locatorPattern = regexp.MustCompile(`\b[A-Z0-9]{6}\b`)

func inferLocator(text string) string {
	for _, candidate := range locatorPattern.FindAllString(text, -1) {
		if strings.IndexFunc(candidate, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0 {
			return candidate
		}
	}
	return ""
}

func inferDestination(title string) string {
	if i := strings.LastIndex(title, " - "); i >= 0 {
		return strings.TrimSpace(title[i+3:])
	}
	if i := strings.LastIndex(strings.ToLower(title), " a "); i >= 0 {
		return strings.TrimSpace(title[i+3:])
	}
	return ""
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func (p ParsedReservation) String() string {
	return fmt.Sprintf("%s %s %s", p.Type, p.Provider, p.Locator)
}

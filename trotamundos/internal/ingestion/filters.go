package ingestion

import (
	"strings"

	"trotamundos/internal/domain"
	"trotamundos/internal/mailer"
)

func ShouldProcess(whitelist []domain.WhitelistEntry, filters []domain.MailFilter, msg mailer.Message) bool {
	// Los patrones vacíos se ignoran: no pueden bloquear todos los correos.
	patterns := 0
	allowed := false
	for _, w := range whitelist {
		if strings.TrimSpace(w.Pattern) == "" {
			continue
		}
		patterns++
		if SenderMatches(w.Pattern, msg.From) {
			allowed = true
			break
		}
	}
	if patterns > 0 && !allowed {
		return false
	}
	for _, f := range filters {
		if !f.Active {
			continue
		}
		if !FilterMatches(f, msg) {
			return false
		}
	}
	return true
}

func SenderMatches(pattern, sender string) bool {
	p := strings.ToLower(strings.TrimSpace(pattern))
	s := strings.ToLower(strings.TrimSpace(sender))
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "mailto:") {
		p = strings.TrimPrefix(p, "mailto:")
	}
	email := s
	if i := strings.LastIndex(s, "<"); i >= 0 {
		email = strings.Trim(s[i+1:], "<> ")
	}
	domain := email
	if i := strings.LastIndex(email, "@"); i >= 0 {
		domain = email[i+1:]
	}
	if strings.HasPrefix(p, "*@") || strings.HasPrefix(p, "@") {
		d := strings.TrimPrefix(strings.TrimPrefix(p, "*"), "@")
		return domain == d || strings.HasSuffix(domain, "."+d)
	}
	if strings.Contains(p, "@") {
		return email == p
	}
	return domain == p || strings.HasSuffix(domain, "."+p)
}

// FilterMatches devuelve true (match neutral) para filtros inválidos o
// inactivos: un campo desconocido o un patrón vacío no debe descartar correos.
func FilterMatches(f domain.MailFilter, msg mailer.Message) bool {
	if !f.Active {
		return true
	}
	want := strings.TrimSpace(f.Value)
	if want == "" {
		return true
	}
	var value string
	switch strings.ToLower(strings.TrimSpace(f.Field)) {
	case "sender":
		value = msg.From
	case "subject":
		value = msg.Subject
	case "body":
		value = msg.Body
	default:
		return true
	}
	value = strings.ToLower(value)
	want = strings.ToLower(want)
	switch strings.ToLower(strings.TrimSpace(f.Operator)) {
	case "contains":
		return strings.Contains(value, want)
	case "not_contains":
		return !strings.Contains(value, want)
	case "equals":
		return strings.TrimSpace(value) == strings.TrimSpace(want)
	case "starts_with":
		return strings.HasPrefix(value, want)
	default:
		return true
	}
}

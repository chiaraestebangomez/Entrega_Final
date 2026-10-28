package ingestion

import (
	"strings"

	"trotamundos/internal/domain"
	"trotamundos/internal/mailer"
)

func ShouldProcess(whitelist []domain.WhitelistEntry, filters []domain.MailFilter, msg mailer.Message) bool {
	if len(whitelist) > 0 {
		allowed := false
		for _, w := range whitelist {
			if SenderMatches(w.Pattern, msg.From) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	active := 0
	for _, f := range filters {
		if !f.Active {
			continue
		}
		active++
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

func FilterMatches(f domain.MailFilter, msg mailer.Message) bool {
	var value string
	switch strings.ToLower(f.Field) {
	case "sender":
		value = msg.From
	case "subject":
		value = msg.Subject
	case "body":
		value = msg.Body
	default:
		return false
	}
	value = strings.ToLower(value)
	want := strings.ToLower(f.Value)
	switch strings.ToLower(f.Operator) {
	case "contains":
		return strings.Contains(value, want)
	case "not_contains":
		return !strings.Contains(value, want)
	case "equals":
		return strings.TrimSpace(value) == strings.TrimSpace(want)
	case "starts_with":
		return strings.HasPrefix(value, want)
	default:
		return false
	}
}

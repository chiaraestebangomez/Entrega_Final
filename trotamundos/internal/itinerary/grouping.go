package itinerary

import (
	"math"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"trotamundos/internal/domain"
)

const GroupWindowDays = 3

type tripRank struct {
	date        float64
	destination int
	status      int
	id          string
}

func (r tripRank) less(other tripRank) bool {
	if r.date != other.date {
		return r.date < other.date
	}
	if r.destination != other.destination {
		return r.destination < other.destination
	}
	if r.status != other.status {
		return r.status < other.status
	}
	return r.id < other.id
}

func BestTrip(trips []domain.Trip, destination string, start, end *time.Time) *domain.Trip {
	var best *domain.Trip
	var bestRank tripRank
	for i := range trips {
		t := &trips[i]
		if t.Status != domain.TripActive {
			continue
		}
		if !SameDestination(t.Destination, destination) {
			continue
		}
		if !DatesCompatible(t.StartDate, t.EndDate, start, end, GroupWindowDays) {
			continue
		}
		rank := tripRank{
			date:        dateScore(t.StartDate, start),
			destination: destinationDistance(t.Destination, destination),
			status:      statusRank(t.Status),
			id:          t.ID,
		}
		if best == nil || rank.less(bestRank) {
			best = t
			bestRank = rank
		}
	}
	return best
}

func statusRank(status string) int {
	if status == domain.TripActive {
		return 0
	}
	return 1
}

func destinationDistance(a, b string) int {
	na, nb := normalizeDestination(a), normalizeDestination(b)
	if na == "" || nb == "" {
		return 1 << 30
	}
	if na == nb {
		return 0
	}
	ta, tb := destinationTokens(na), destinationTokens(nb)
	if !tokensContain(ta, tb) && !tokensContain(tb, ta) {
		return 1 << 29
	}
	diff := len(ta) - len(tb)
	if diff < 0 {
		diff = -diff
	}
	return diff
}

func SameDestination(a, b string) bool {
	na, nb := normalizeDestination(a), normalizeDestination(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	ta, tb := destinationTokens(na), destinationTokens(nb)
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}
	// Igualdad tolerante: todos los tokens del destino más corto deben estar
	// en el más largo (evita falsos positivos de "contains" parcial).
	return tokensContain(ta, tb) || tokensContain(tb, ta)
}

func normalizeDestination(s string) string {
	decomposed := norm.NFD.String(strings.ToLower(strings.TrimSpace(s)))
	plain := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, decomposed)
	var b strings.Builder
	for _, r := range strings.TrimSpace(plain) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func destinationTokens(normalized string) []string {
	if normalized == "" {
		return nil
	}
	return strings.Fields(normalized)
}

func tokensContain(haystack, needle []string) bool {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}
	set := make(map[string]struct{}, len(haystack))
	for _, token := range haystack {
		set[token] = struct{}{}
	}
	for _, token := range needle {
		if _, ok := set[token]; !ok {
			return false
		}
	}
	return true
}

func DatesCompatible(tripStart, tripEnd, resStart, resEnd *time.Time, windowDays int) bool {
	if resStart == nil || tripStart == nil {
		return true
	}
	rs, re := normalizeRange(resStart, resEnd)
	ts, te := normalizeRange(tripStart, tripEnd)
	window := time.Duration(windowDays) * 24 * time.Hour
	return !rs.After(te.Add(window)) && !re.Before(ts.Add(-window))
}

func MatchesTrip(t *domain.Trip, destination string, start, end *time.Time) bool {
	return SameDestination(t.Destination, destination) && DatesCompatible(t.StartDate, t.EndDate, start, end, GroupWindowDays)
}

func TripBounds(reservations []domain.Reservation) (*time.Time, *time.Time) {
	var min, max *time.Time
	for i := range reservations {
		r := reservations[i]
		if r.StartAt == nil {
			continue
		}
		s := dateOnly(r.StartAt)
		e := s
		if r.EndAt != nil {
			e = dateOnly(r.EndAt)
		}
		if min == nil || s.Before(*min) {
			min = &s
		}
		if max == nil || e.After(*max) {
			max = &e
		}
	}
	return min, max
}

func TripName(destination, title string) string {
	if d := strings.TrimSpace(destination); d != "" {
		return d
	}
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	return "Viaje"
}

func normalizeRange(start, end *time.Time) (time.Time, time.Time) {
	s := dateOnly(start)
	if end == nil {
		return s, s
	}
	e := dateOnly(end)
	if e.Before(s) {
		return e, s
	}
	return s, e
}

func dateOnly(t *time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

func dateScore(a, b *time.Time) float64 {
	if a == nil || b == nil {
		return float64(GroupWindowDays * 24)
	}
	return math.Abs(dateOnly(a).Sub(dateOnly(b)).Hours())
}

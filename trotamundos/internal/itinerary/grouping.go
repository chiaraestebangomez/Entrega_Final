package itinerary

import (
	"math"
	"strings"
	"time"

	"trotamundos/internal/domain"
)

const GroupWindowDays = 3

func BestTrip(trips []domain.Trip, destination string, start, end *time.Time) *domain.Trip {
	var best *domain.Trip
	bestScore := math.MaxFloat64
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
		score := dateScore(t.StartDate, start)
		if best == nil || score < bestScore {
			best = t
			bestScore = score
		}
	}
	return best
}

func SameDestination(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return true
	}
	return a == b || strings.Contains(a, b) || strings.Contains(b, a)
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

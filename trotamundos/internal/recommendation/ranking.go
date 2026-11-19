package recommendation

import (
	"math"
	"sort"
	"strings"
	"time"

	"trotamundos/internal/domain"
	"trotamundos/internal/platform/idgen"
)

func affinityKey(destination, recType string) string {
	return strings.ToLower(strings.TrimSpace(destination)) + "|" + strings.ToLower(strings.TrimSpace(recType))
}

// profileAffinityKey agrega afinidades por destino y perfil anonimizado, tal
// como describe la Iteración 5 del ADD. El bucket es seudónimo, nunca el user_id.
func profileAffinityKey(bucket, destination, recType string) string {
	return bucket + "|" + affinityKey(destination, recType)
}

func TrainModel(signals []domain.AnonymizedSignal, now time.Time) domain.ModelSnapshot {
	affinities := map[string]float64{}
	for _, signal := range signals {
		destination := strings.TrimSpace(signal.Destination)
		if destination == "" {
			continue
		}
		weight := 0.5
		switch signal.Action {
		case domain.FeedbackInterested:
			weight = 1.5
		case domain.FeedbackDiscarded:
			weight = -0.75
		case domain.FeedbackAdjust, domain.FeedbackInquiry:
			weight = 0.75
		}
		affinities[affinityKey(destination, signal.RecommendationType)] += weight
		affinities[affinityKey(destination, "*")] += weight * 0.4
		if bucket := strings.TrimSpace(signal.UserBucket); bucket != "" {
			affinities[profileAffinityKey(bucket, destination, signal.RecommendationType)] += weight
			affinities[profileAffinityKey(bucket, destination, "*")] += weight * 0.4
		}
	}
	return domain.ModelSnapshot{
		Version:    idgen.UUID(),
		TrainedAt:  now.UTC(),
		Affinities: affinities,
		Signals:    int64(len(signals)),
	}
}

func FromCatalog(items []domain.AgencyCatalogItem) []domain.Recommendation {
	out := make([]domain.Recommendation, 0, len(items))
	for _, item := range items {
		out = append(out, domain.Recommendation{
			ID: item.ID, AgencyID: item.AgencyID, AgencyName: item.AgencyName,
			Title: item.Title, Destination: item.Destination, Description: item.Description,
			PriceCents: item.PriceCents, Currency: item.Currency,
			StartDate: item.StartDate, EndDate: item.EndDate, Active: true, Type: item.Type,
		})
	}
	return out
}

func Rank(candidates []domain.Recommendation, model domain.ModelSnapshot, prefs *domain.RecommendationPreferences, history []domain.Trip, profileBucket string, discarded, interested map[string]bool) []domain.Recommendation {
	destinations, types := historyAffinities(history)
	out := make([]domain.Recommendation, 0, len(candidates))
	for _, candidate := range candidates {
		if discarded[candidate.ID] {
			continue
		}
		if prefs != nil && prefs.MaxBudgetCents > 0 && candidate.PriceCents > prefs.MaxBudgetCents {
			continue
		}
		recType := candidate.Type
		if recType == "" {
			recType = domain.RecoTypePackage
		}
		score := 1.0
		reasons := []string{}
		if profileBucket != "" {
			if value, ok := model.Affinities[profileAffinityKey(profileBucket, candidate.Destination, recType)]; ok {
				score += clamp(value, -2, 4) * 0.5
				reasons = append(reasons, "afinidad por perfil")
			} else if value, ok := model.Affinities[profileAffinityKey(profileBucket, candidate.Destination, "*")]; ok {
				score += clamp(value, -2, 4) * 0.3
				reasons = append(reasons, "afinidad de destino del perfil")
			} else if value, ok := model.Affinities[affinityKey(candidate.Destination, recType)]; ok {
				score += clamp(value, -2, 4) * 0.5
				reasons = append(reasons, "afinidad global")
			} else if value, ok := model.Affinities[affinityKey(candidate.Destination, "*")]; ok {
				score += clamp(value, -2, 4) * 0.3
				reasons = append(reasons, "afinidad de destino")
			}
		} else if value, ok := model.Affinities[affinityKey(candidate.Destination, recType)]; ok {
			score += clamp(value, -2, 4) * 0.5
			reasons = append(reasons, "afinidad global")
		} else if value, ok := model.Affinities[affinityKey(candidate.Destination, "*")]; ok {
			score += clamp(value, -2, 4) * 0.3
			reasons = append(reasons, "afinidad de destino")
		}
		if destinations[normalize(candidate.Destination)] > 0 {
			score += math.Min(float64(destinations[normalize(candidate.Destination)]), 3) * 0.6
			reasons = append(reasons, "destino del historial")
		}
		if types[recType] > 0 {
			score += math.Min(float64(types[recType]), 3) * 0.4
			reasons = append(reasons, "tipo del historial")
		}
		if prefs != nil {
			if containsFold(prefs.PreferredDestinations, candidate.Destination) {
				score += 1.5
				reasons = append(reasons, "destino preferido")
			}
			if matchesPreferredTypes(prefs, recType) {
				score += 1.2
				reasons = append(reasons, "tipo preferido")
			}
			if overlapPreference(prefs, candidate) {
				score += 0.8
				reasons = append(reasons, "fechas preferidas")
			} else if prefs.PreferredStartDate != nil || prefs.PreferredEndDate != nil {
				score -= 0.5
			}
		}
		if interested[candidate.ID] {
			score += 3
			reasons = append(reasons, "te interesó")
		}
		candidate.Score = math.Round(score*100) / 100
		candidate.Reason = strings.Join(reasons, ", ")
		candidate.Interested = interested[candidate.ID]
		candidate.Discarded = false
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func historyAffinities(trips []domain.Trip) (map[string]int, map[string]int) {
	destinations := map[string]int{}
	types := map[string]int{}
	for _, trip := range trips {
		if trip.Destination != "" {
			destinations[normalize(trip.Destination)]++
		}
		for _, res := range trip.Reservations {
			if res.Destination != "" {
				destinations[normalize(res.Destination)]++
			}
			if res.Type != "" {
				types[res.Type]++
			}
		}
	}
	return destinations, types
}

func matchesPreferredTypes(prefs *domain.RecommendationPreferences, recType string) bool {
	if prefs == nil {
		return false
	}
	if recType == domain.RecoTypeLodging {
		return containsFold(prefs.LodgingTypes, recType) || contains(prefs.LodgingTypes, "hotel")
	}
	if recType == domain.RecoTypeTransport {
		return containsFold(prefs.TransportTypes, recType) || contains(prefs.TransportTypes, domain.RecoTypeTransport)
	}
	return false
}

func overlapPreference(prefs *domain.RecommendationPreferences, rec domain.Recommendation) bool {
	if prefs == nil || (prefs.PreferredStartDate == nil && prefs.PreferredEndDate == nil) {
		return false
	}
	if rec.StartDate == nil && rec.EndDate == nil {
		return false
	}
	from := prefs.PreferredStartDate
	to := prefs.PreferredEndDate
	if rec.EndDate != nil && from != nil && rec.EndDate.Before(*from) {
		return false
	}
	if rec.StartDate != nil && to != nil && rec.StartDate.After(*to) {
		return false
	}
	return true
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if normalize(value) == normalize(target) {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func priceBucket(cents int64) string {
	switch {
	case cents <= 0:
		return "unknown"
	case cents < 50000:
		return "low"
	case cents < 150000:
		return "mid"
	default:
		return "high"
	}
}

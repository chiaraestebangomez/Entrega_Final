package recommendation

import (
	"os"
	"strings"
	"testing"
)

func TestOpenAPIContractIncludesRecommendationEndpoints(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("openapi.yaml: %v", err)
	}
	spec := string(body)
	required := []string{
		"  /agencies:",
		"  /me/favorite-agency:",
		"  /me/preferences:",
		"  /recommendations:",
		"  /recommendations/{id}/feedback:",
		"  /recommendations/{id}/info-request:",
		"  /agency-requests:",
		"  /agency-requests/{id}:",
		"    RecommendationPreferences:",
		"    RecommendationFeedback:",
		"    ModelSnapshot:",
	}
	for _, item := range required {
		if !strings.Contains(spec, item) {
			t.Fatalf("falta en el contrato OpenAPI: %s", item)
		}
	}
}

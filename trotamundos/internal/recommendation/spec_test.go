package recommendation

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type openAPIDocument struct {
	OpenAPI    string                    `yaml:"openapi"`
	Paths      map[string]map[string]any `yaml:"paths"`
	Components struct {
		Schemas map[string]any `yaml:"schemas"`
	} `yaml:"components"`
}

type asyncAPIDocument struct {
	AsyncAPI string         `yaml:"asyncapi"`
	Channels map[string]any `yaml:"channels"`
}

var (
	httpMethods   = map[string]bool{"get": true, "put": true, "post": true, "delete": true, "options": true, "head": true, "patch": true, "trace": true}
	routerRouteRE = regexp.MustCompile(`mux\.Handle(?:Func)?\(\s*"([A-Z]+)\s+([^"]+)"`)
)

func readSpec(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return body
}

func loadOpenAPI(t *testing.T) openAPIDocument {
	t.Helper()
	var doc openAPIDocument
	if err := yaml.Unmarshal(readSpec(t, "../../api/openapi.yaml"), &doc); err != nil {
		t.Fatalf("openapi.yaml no es YAML válido: %v", err)
	}
	return doc
}

func TestSpecsParseAsYAML(t *testing.T) {
	openapi := loadOpenAPI(t)
	if openapi.OpenAPI == "" {
		t.Fatal("openapi.yaml no declara la versión 'openapi'")
	}
	var asyncapi asyncAPIDocument
	if err := yaml.Unmarshal(readSpec(t, "../../api/asyncapi.yaml"), &asyncapi); err != nil {
		t.Fatalf("asyncapi.yaml no es YAML válido: %v", err)
	}
	if asyncapi.AsyncAPI == "" {
		t.Fatal("asyncapi.yaml no declara la versión 'asyncapi'")
	}
}

func TestOpenAPIContractIncludesRecommendationEndpoints(t *testing.T) {
	doc := loadOpenAPI(t)
	requiredPaths := []string{
		"/agencies", "/me/favorite-agency", "/me/preferences", "/recommendations",
		"/recommendations/{id}/feedback", "/recommendations/{id}/info-request",
		"/agency-requests", "/agency-requests/{id}",
	}
	for _, path := range requiredPaths {
		operations, ok := doc.Paths[path]
		if !ok {
			t.Errorf("falta en el contrato OpenAPI: %s", path)
		}
		if len(operations) == 0 {
			t.Errorf("el path %s no declara operaciones HTTP", path)
		}
	}
	for _, schema := range []string{"RecommendationPreferences", "RecommendationFeedback", "ModelSnapshot"} {
		if _, ok := doc.Components.Schemas[schema]; !ok {
			t.Errorf("falta el schema en el contrato OpenAPI: %s", schema)
		}
	}
}

func TestOpenAPIOperationsDeclareSuccessResponses(t *testing.T) {
	doc := loadOpenAPI(t)
	for path, operations := range doc.Paths {
		for method, raw := range operations {
			if !httpMethods[strings.ToLower(method)] {
				continue
			}
			op, ok := raw.(map[string]any)
			if !ok {
				t.Errorf("%s %s: operación no es un objeto", method, path)
				continue
			}
			responses, ok := op["responses"].(map[string]any)
			if !ok || len(responses) == 0 {
				t.Errorf("%s %s: no declara respuestas", method, path)
				continue
			}
			hasSuccess := false
			for code := range responses {
				if strings.HasPrefix(code, "2") {
					hasSuccess = true
					break
				}
			}
			if !hasSuccess {
				t.Errorf("%s %s: no declara ninguna respuesta 2xx", method, path)
			}
		}
	}
}

func TestOpenAPIMatchesRouter(t *testing.T) {
	doc := loadOpenAPI(t)

	specRoutes := map[string]bool{}
	for path, operations := range doc.Paths {
		for method := range operations {
			lower := strings.ToLower(method)
			if !httpMethods[lower] {
				continue // parameters, summary, description, etc. a nivel path
			}
			specRoutes[strings.ToUpper(lower)+" "+path] = true
		}
	}

	routerBody, err := os.ReadFile("../../internal/api/router.go")
	if err != nil {
		t.Fatalf("router.go: %v", err)
	}
	routerRoutes := map[string]bool{}
	for _, match := range routerRouteRE.FindAllStringSubmatch(string(routerBody), -1) {
		routerRoutes[match[1]+" "+match[2]] = true
	}

	for route := range specRoutes {
		if !routerRoutes[route] {
			t.Errorf("OpenAPI declara %q pero el router no lo registra", route)
		}
	}
	for route := range routerRoutes {
		if !specRoutes[route] {
			t.Errorf("el router registra %q pero no está declarado en OpenAPI", route)
		}
	}
	if len(routerRoutes) == 0 {
		t.Fatal("no se encontraron rutas en router.go; revisar el regex de extracción")
	}
}

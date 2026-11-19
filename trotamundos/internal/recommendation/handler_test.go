package recommendation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"trotamundos/internal/auth"
	"trotamundos/internal/domain"
	"trotamundos/internal/platform/jwtx"
)

func authenticatedRoute(t *testing.T, route string, handler http.HandlerFunc) (http.Handler, string) {
	t.Helper()
	priv, pub, err := jwtx.Generate()
	if err != nil {
		t.Fatal(err)
	}
	signer := jwtx.NewSigner(priv, "test-issuer", time.Hour)
	token, _, err := signer.Issue("user-1", "user@example.com", "user", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	authenticator := auth.NewAuthenticator(jwtx.NewVerifier(pub, "test-issuer"))
	mux.Handle(route, authenticator.Require(handler))
	return mux, token
}

func TestDiscardWrapperRecordsDiscardedFeedback(t *testing.T) {
	store := newFakeStore()
	agencies := newFakeAgencies()
	rec := FromCatalog([]domain.AgencyCatalogItem{catalogItems()[0]})[0]
	agencies.recs[rec.ID] = &rec
	h := NewHandler(testService(t, store, NewMemoryCache(), agencies, &fakeCatalog{}))

	mux, token := authenticatedRoute(t, "POST /recommendations/{id}/discard", h.Discard)
	req := httptest.NewRequest(http.MethodPost, "/recommendations/r1/discard", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if len(store.interactions) != 1 || store.interactions[0] != "r1:discarded" {
		t.Fatalf("interacciones = %+v", store.interactions)
	}
	if len(store.signals) != 1 || store.signals[0].Action != domain.FeedbackDiscarded {
		t.Fatalf("señales = %+v", store.signals)
	}
}

func TestInquireWrapperMatchesInfoRequest(t *testing.T) {
	agencies := newFakeAgencies()
	rec := FromCatalog([]domain.AgencyCatalogItem{catalogItems()[0]})[0]
	agencies.recs[rec.ID] = &rec
	h := NewHandler(testService(t, newFakeStore(), NewMemoryCache(), agencies, &fakeCatalog{}))

	mux, token := authenticatedRoute(t, "POST /recommendations/{id}/inquire", h.Inquire)
	req := httptest.NewRequest(http.MethodPost, "/recommendations/r1/inquire", strings.NewReader(`{"message":"¿Hay disponibilidad?"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var created domain.AgencyRequest
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Message != "¿Hay disponibilidad?" || created.RecommendationID != "r1" {
		t.Fatalf("pedido = %+v", created)
	}
	if len(agencies.requests) != 1 {
		t.Fatalf("pedidos = %+v", agencies.requests)
	}
}

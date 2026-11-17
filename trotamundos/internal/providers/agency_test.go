package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	"trotamundos/internal/domain"
)

type fakeAgencyAdapter struct {
	calls   int
	failing bool
}

func (f *fakeAgencyAdapter) Name() string { return "fake" }
func (f *fakeAgencyAdapter) Catalog(context.Context, string) (domain.AgencyCatalogResult, error) {
	f.calls++
	if f.failing {
		return domain.AgencyCatalogResult{}, ErrAgencyUnavailable
	}
	return domain.AgencyCatalogResult{Items: []domain.AgencyCatalogItem{{ID: "r1", Title: "Patagonia"}}, FetchedAt: time.Now().UTC()}, nil
}
func (f *fakeAgencyAdapter) SendRequest(context.Context, domain.AgencyRequest) (domain.AgencyDelivery, error) {
	if f.failing {
		return domain.AgencyDelivery{}, ErrAgencyUnavailable
	}
	return domain.AgencyDelivery{Status: "delivered"}, nil
}

func TestResilientAgencyFallsBackToLastCatalog(t *testing.T) {
	inner := &fakeAgencyAdapter{}
	adapter := newResilientAgency(inner)
	ctx := context.Background()
	first, err := adapter.Catalog(ctx, "ag1")
	if err != nil || len(first.Items) != 1 || first.Stale {
		t.Fatalf("primera consulta = %+v, err=%v", first, err)
	}
	inner.failing = true
	second, err := adapter.Catalog(ctx, "ag1")
	if err != nil || len(second.Items) != 1 || !second.Stale {
		t.Fatalf("fallback = %+v, err=%v", second, err)
	}
	if _, err := adapter.SendRequest(ctx, domain.AgencyRequest{AgencyID: "ag1"}); !errors.Is(err, ErrAgencyUnavailable) {
		t.Fatalf("envío = %v", err)
	}
}

func TestResilientAgencyBreakerAvoidsRepeatedCalls(t *testing.T) {
	inner := &fakeAgencyAdapter{failing: true}
	adapter := newResilientAgency(inner)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := adapter.Catalog(ctx, "ag1"); !errors.Is(err, ErrAgencyUnavailable) {
			t.Fatalf("intento %d = %v", i, err)
		}
	}
	calls := inner.calls
	if _, err := adapter.Catalog(ctx, "ag1"); !errors.Is(err, ErrAgencyOpen) || inner.calls != calls {
		t.Fatalf("circuit breaker: err=%v calls=%d", err, inner.calls)
	}
}

func TestCatalogType(t *testing.T) {
	cases := map[string]string{
		"Hotel céntrico con desayuno": domain.RecoTypeLodging,
		"Vuelo a Bariloche":           domain.RecoTypeTransport,
		"Ruta del vino con trekking":  domain.RecoTypeRoute,
		"Paquete completo":            domain.RecoTypePackage,
	}
	for text, want := range cases {
		if got := CatalogType(text); got != want {
			t.Fatalf("CatalogType(%q) = %q, esperado %q", text, got, want)
		}
	}
}

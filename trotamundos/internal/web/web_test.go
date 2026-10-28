package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndex(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	Index(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("content-type = %q, want text/html", got)
	}
	if !strings.Contains(rec.Body.String(), "/assets/js/app.js") {
		t.Fatal("index does not reference the frontend bundle")
	}
}

func TestIndexRejectsUnknownPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/viajes", nil)
	rec := httptest.NewRecorder()

	Index(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAssets(t *testing.T) {
	tests := []struct {
		path        string
		wantStatus  int
		wantContent string
	}{
		{path: "/assets/styles.css", wantStatus: http.StatusOK, wantContent: "text/css"},
		{path: "/assets/js/app.js", wantStatus: http.StatusOK, wantContent: "javascript"},
		{path: "/assets/favicon.svg", wantStatus: http.StatusOK, wantContent: "image/svg+xml"},
		{path: "/assets/missing.css", wantStatus: http.StatusNotFound},
	}

	handler := Assets()
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantContent != "" && !strings.Contains(rec.Header().Get("Content-Type"), tt.wantContent) {
				t.Fatalf("content-type = %q, want %q", rec.Header().Get("Content-Type"), tt.wantContent)
			}
		})
	}
}

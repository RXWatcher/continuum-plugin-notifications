package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIReportsNotConfiguredInsteadOfPanicking(t *testing.T) {
	h := New(Deps{}).Handler()

	for _, path := range []string{"/api/admin/status", "/api/v1/capabilities"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want %d; body=%s", path, rec.Code, http.StatusServiceUnavailable, rec.Body.String())
		}
	}
}

func TestComputeBaseHref(t *testing.T) {
	tests := map[string]string{
		"/admin":                   "./",
		"/admin/":                  "../",
		"/admin/registry":          "../",
		"/admin/registry/":         "../../",
		"/admin/registry/123/edit": "../../../",
	}
	for path, want := range tests {
		if got := computeBaseHref(path); got != want {
			t.Fatalf("computeBaseHref(%q) = %q, want %q", path, got, want)
		}
	}
}

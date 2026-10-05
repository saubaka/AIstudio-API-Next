package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The deployed root serves both API namespaces, never the SPA fallback.
func TestRootRoutesChannelNamespacesToAPI(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-API-Path", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	root := rootHandler(api)
	for _, path := range []string{"/playground/v1/models", "/playground/v1beta/models", "/build/v1/chat/completions", "/build/v1beta/models/fixture:generateContent"} {
		w := httptest.NewRecorder()
		root.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusNoContent || w.Header().Get("X-Test-API-Path") != path {
			t.Fatalf("API path served SPA: %s %d", path, w.Code)
		}
	}
}

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebHandlerServesIndexAndSPAFallback(t *testing.T) {
	handler := NewWebHandler()
	for _, target := range []string{"/", "/rooms/example"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d", target, response.Code)
		}
		if !strings.Contains(response.Body.String(), `<div id="root"></div>`) {
			t.Fatalf("GET %s did not return the web application", target)
		}
		if response.Header().Get("Content-Security-Policy") == "" {
			t.Fatalf("GET %s missing content security policy", target)
		}
	}
}

func TestWebHandlerDoesNotMaskUnknownAPI(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	response := httptest.NewRecorder()
	NewWebHandler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", response.Code)
	}
}

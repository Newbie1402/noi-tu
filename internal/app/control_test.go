package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsToggle(t *testing.T) {
	s := server{collector: &collectorManager{}, settings: newRuntimeSettings()}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/settings/suggestions", strings.NewReader(`{"enabled":false}`))
	s.toggleSuggestions(recorder, request)
	if recorder.Code != http.StatusOK || s.suggestionsEnabled() || !strings.Contains(recorder.Body.String(), `"suggestions_enabled":false`) {
		t.Fatalf("status=%d enabled=%t body=%s", recorder.Code, s.suggestionsEnabled(), recorder.Body.String())
	}
}

func TestProtectedEndpointRequiresAPIKey(t *testing.T) {
	s := server{apiKey: "secret", requireAPIKey: true, allowedOrigins: map[string]bool{"https://www.noitu.fun": true}, settings: newRuntimeSettings(), collector: &collectorManager{}}
	request := httptest.NewRequest(http.MethodGet, "/settings", nil)
	recorder := httptest.NewRecorder()
	s.settingsHandler(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing key status=%d", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/settings", nil)
	request.Header.Set("X-API-Key", "secret")
	recorder = httptest.NewRecorder()
	s.settingsHandler(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("valid key status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCollectorTokenEndpointStoresToken(t *testing.T) {
	s := server{tokenStore: &tokenBridge{ready: make(chan struct{})}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/collector/token", strings.NewReader(`{"token":"01234567890123456789"}`))
	s.collectorToken(recorder, request)
	if recorder.Code != http.StatusNoContent || s.tokenStore.get() != "01234567890123456789" {
		t.Fatalf("status=%d token=%q", recorder.Code, s.tokenStore.get())
	}
}

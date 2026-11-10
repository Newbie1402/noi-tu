package app

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAPISpec(t *testing.T) {
	spec := openAPISpec()
	paths, ok := spec["paths"].(map[string]any)
	if !ok || paths["/suggest"] == nil || paths["/session/suggestions"] == nil || paths["/collector/toggle"] == nil || paths["/collector/token"] == nil || paths["/settings"] == nil || paths["/settings/suggestions"] == nil {
		t.Fatalf("missing documented paths: %#v", paths)
	}
	if _, err := json.Marshal(spec); err != nil {
		t.Fatal(err)
	}
}

func TestSwaggerHandlers(t *testing.T) {
	jsonResponse := httptest.NewRecorder()
	openAPIHandler(jsonResponse, httptest.NewRequest("GET", "/openapi.json", nil))
	if jsonResponse.Code != 200 || !strings.Contains(jsonResponse.Body.String(), `"openapi":"3.0.3"`) {
		t.Fatalf("openapi response: %d %s", jsonResponse.Code, jsonResponse.Body.String())
	}
	htmlResponse := httptest.NewRecorder()
	swaggerUIHandler(htmlResponse, httptest.NewRequest("GET", "/swagger", nil))
	if htmlResponse.Code != 200 || !strings.Contains(htmlResponse.Body.String(), "SwaggerUIBundle") {
		t.Fatalf("swagger response: %d", htmlResponse.Code)
	}
}

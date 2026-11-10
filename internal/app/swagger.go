package app

import (
	"encoding/json"
	"net/http"
)

func openAPISpec() map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Nối Từ Helper API",
			"version":     "1.1.0",
			"description": "API gợi ý nước đi, điều khiển plugin và collector VPS.",
		},
		"servers":  []map[string]string{{"url": "/"}},
		"security": []map[string][]string{{"ApiKeyAuth": {}}},
		"paths": map[string]any{
			"/healthz": map[string]any{
				"get": map[string]any{"summary": "Health check", "responses": map[string]any{"204": map[string]string{"description": "Backend is running"}}},
			},
			"/suggest": map[string]any{
				"get": map[string]any{
					"summary": "Stream suggestions by syllable",
					"parameters": []map[string]any{
						{"name": "key", "in": "query", "required": true, "schema": map[string]string{"type": "string"}, "description": "Syllable that starts the next two-syllable word."},
						{"name": "used", "in": "query", "required": false, "schema": map[string]any{"type": "array", "items": map[string]string{"type": "string"}}, "description": "Repeated parameter for words already used."},
					},
					"responses": map[string]any{"200": map[string]string{"description": "Server-sent events: suggestion, ranking, done."}, "400": map[string]string{"description": "Invalid key."}, "503": map[string]string{"description": "Database or dictionary unavailable."}},
				},
			},
			"/observations": map[string]any{
				"post": map[string]any{"summary": "Queue a validation observation", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Observation"}}}}, "responses": map[string]any{"202": map[string]string{"description": "Queued for persistence."}, "400": map[string]string{"description": "Invalid request."}, "503": map[string]string{"description": "Queue is full."}}},
			},
			"/collector/toggle": map[string]any{
				"post": map[string]any{"summary": "Enable or disable the automatic collector", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/CollectorToggle"}, "example": map[string]bool{"enabled": true}}}}, "responses": map[string]any{"200": map[string]any{"description": "Collector state.", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/CollectorStatus"}}}}, "400": map[string]string{"description": "Invalid request."}, "503": map[string]string{"description": "Collector failed to start or stopped with an error."}}},
			},
			"/collector/token": map[string]any{
				"post": map[string]any{"summary": "Send the browser guest token to the VPS collector", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/CollectorToken"}}}}, "responses": map[string]string{"204": "Token accepted in memory.", "400": "Invalid token.", "401": "API key required."}},
			},
			"/settings": map[string]any{
				"get": map[string]any{"summary": "Read plugin runtime settings", "responses": map[string]any{"200": map[string]any{"description": "Current settings.", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Settings"}}}}, "401": map[string]string{"description": "API key required."}}},
			},
			"/settings/suggestions": map[string]any{
				"post": map[string]any{"summary": "Enable or disable suggestions", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/CollectorToggle"}}}}, "responses": map[string]any{"200": map[string]string{"description": "Settings updated."}, "400": map[string]string{"description": "Invalid request."}, "401": map[string]string{"description": "API key required."}}},
			},
			"/session/suggestions": map[string]any{
				"post": map[string]any{"summary": "Read game state and return ranked suggestions", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/SessionState"}}}}, "responses": map[string]any{"200": map[string]any{"description": "Ranked suggestions.", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/SessionSuggestions"}}}}, "400": map[string]string{"description": "Invalid state."}, "403": map[string]string{"description": "Origin is not allowed."}, "503": map[string]string{"description": "Database or dictionary unavailable."}}},
			},
		},
		"components": map[string]any{"securitySchemes": map[string]any{"ApiKeyAuth": map[string]string{"type": "apiKey", "in": "header", "name": "X-API-Key"}}, "schemas": map[string]any{
			"Observation":        map[string]any{"type": "object", "required": []string{"phrase", "accepted"}, "properties": map[string]any{"phrase": map[string]string{"type": "string", "example": "bình minh"}, "accepted": map[string]string{"type": "boolean", "example": "true"}}},
			"CollectorToggle":    map[string]any{"type": "object", "required": []string{"enabled"}, "properties": map[string]any{"enabled": map[string]string{"type": "boolean", "description": "true để chạy cron, false để dừng cron."}}},
			"CollectorToken":     map[string]any{"type": "object", "required": []string{"token"}, "properties": map[string]any{"token": map[string]any{"type": "string", "writeOnly": true}}},
			"CollectorStatus":    map[string]any{"type": "object", "properties": map[string]any{"enabled": map[string]string{"type": "boolean"}, "running": map[string]string{"type": "boolean"}, "last_error": map[string]string{"type": "string"}}},
			"Settings":           map[string]any{"type": "object", "properties": map[string]any{"suggestions_enabled": map[string]string{"type": "boolean"}, "collector": map[string]any{"$ref": "#/components/schemas/CollectorStatus"}}},
			"SessionState":       map[string]any{"type": "object", "properties": map[string]any{"current_word": map[string]string{"type": "string", "example": "ao cá"}, "key": map[string]string{"type": "string", "example": "cá"}, "used": map[string]any{"type": "array", "items": map[string]string{"type": "string"}}, "words": map[string]any{"type": "array", "items": map[string]string{"type": "string"}}}},
			"RankedWord":         map[string]any{"type": "object", "properties": map[string]any{"word": map[string]string{"type": "string"}, "verified": map[string]string{"type": "boolean"}, "opponent_replies": map[string]string{"type": "integer"}, "worst_next_moves": map[string]string{"type": "integer"}, "assessment": map[string]any{"type": "string", "enum": []string{"possible_kill", "has_reply", "risk"}}}},
			"SessionSuggestions": map[string]any{"type": "object", "properties": map[string]any{"current_word": map[string]string{"type": "string"}, "key": map[string]string{"type": "string"}, "suggestions": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/RankedWord"}}}},
		}},
	}
}

func openAPIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(openAPISpec())
}

func swaggerUIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Nối Từ API Swagger</title><link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script><script>window.onload=()=>SwaggerUIBundle({url:'/openapi.json',dom_id:'#swagger-ui',deepLinking:true});</script></body></html>`))
}

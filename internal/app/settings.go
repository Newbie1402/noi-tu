package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
)

type runtimeSettings struct {
	suggestions atomic.Bool
}

func newRuntimeSettings() *runtimeSettings {
	settings := &runtimeSettings{}
	settings.suggestions.Store(true)
	return settings
}

func ensureRuntimeSettings(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS runtime_settings (
		id smallint PRIMARY KEY CHECK (id = 1),
		suggestions_enabled boolean NOT NULL DEFAULT true,
		collector_enabled boolean NOT NULL DEFAULT false,
		updated_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO runtime_settings(id) VALUES(1) ON CONFLICT(id) DO NOTHING`)
	return err
}

func loadRuntimeSettings(ctx context.Context, db *sql.DB) (suggestionsEnabled, collectorEnabled bool, err error) {
	if db == nil {
		return true, false, nil
	}
	err = db.QueryRowContext(ctx, `SELECT suggestions_enabled, collector_enabled FROM runtime_settings WHERE id=1`).Scan(&suggestionsEnabled, &collectorEnabled)
	return
}

func (s *server) persistSuggestions(enabled bool) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`UPDATE runtime_settings SET suggestions_enabled=$1, updated_at=now() WHERE id=1`, enabled)
	return err
}

func (s *server) persistCollector(enabled bool) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`UPDATE runtime_settings SET collector_enabled=$1, updated_at=now() WHERE id=1`, enabled)
	return err
}

func (s *server) suggestionsEnabled() bool {
	if s.settings == nil {
		return true
	}
	return s.settings.suggestions.Load()
}

type settingsResponse struct {
	SuggestionsEnabled bool            `json:"suggestions_enabled"`
	Collector          collectorStatus `json:"collector"`
}

type toggleRequest struct {
	Enabled *bool `json:"enabled"`
}

func (s *server) settingsHandler(w http.ResponseWriter, r *http.Request) {
	if !s.protected(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, settingsResponse{SuggestionsEnabled: s.suggestionsEnabled(), Collector: s.collectorStatus()})
}

func (s *server) toggleSuggestions(w http.ResponseWriter, r *http.Request) {
	if !s.protected(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var request toggleRequest
	if !decodeStrictJSON(w, r, &request) {
		return
	}
	if request.Enabled == nil {
		http.Error(w, `body must be {"enabled":true|false}`, http.StatusBadRequest)
		return
	}
	if s.settings == nil {
		s.settings = newRuntimeSettings()
	}
	if err := s.persistSuggestions(*request.Enabled); err != nil {
		http.Error(w, "settings unavailable", http.StatusServiceUnavailable)
		return
	}
	s.settings.suggestions.Store(*request.Enabled)
	writeJSON(w, settingsResponse{SuggestionsEnabled: *request.Enabled, Collector: s.collectorStatus()})
}

func (s *server) collectorStatus() collectorStatus {
	if s.collector == nil {
		return collectorStatus{}
	}
	return s.collector.status()
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func decodeStrictJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return false
	}
	return true
}

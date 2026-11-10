package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type collectorManager struct {
	mu         sync.Mutex
	cancel     context.CancelFunc
	running    bool
	generation uint64
	lastError  string
}

type collectorToggleRequest struct {
	Enabled *bool `json:"enabled"`
}

type collectorStatus struct {
	Enabled   bool   `json:"enabled"`
	Running   bool   `json:"running"`
	LastError string `json:"last_error,omitempty"`
}

func (m *collectorManager) statusLocked() collectorStatus {
	return collectorStatus{Enabled: m.running, Running: m.running, LastError: m.lastError}
}

func (m *collectorManager) status() collectorStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusLocked()
}

func (m *collectorManager) set(enabled bool, run func(context.Context) error) collectorStatus {
	m.mu.Lock()
	if !enabled {
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		m.running = false
		m.lastError = ""
		m.generation++
		status := m.statusLocked()
		m.mu.Unlock()
		return status
	}
	if m.running {
		status := m.statusLocked()
		m.mu.Unlock()
		return status
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.lastError = ""
	m.generation++
	generation := m.generation
	m.mu.Unlock()

	go func() {
		err := run(ctx)
		m.mu.Lock()
		defer m.mu.Unlock()
		if generation != m.generation {
			return
		}
		m.running = false
		m.cancel = nil
		if err != nil && !errors.Is(err, context.Canceled) {
			m.lastError = err.Error()
		}
	}()
	return m.status()
}

func (s *server) collectorToggle(w http.ResponseWriter, r *http.Request) {
	if !s.protected(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var request collectorToggleRequest
	if !decodeStrictJSON(w, r, &request) {
		return
	}
	if request.Enabled == nil {
		http.Error(w, `body must be {"enabled":true|false}`, http.StatusBadRequest)
		return
	}
	if *request.Enabled && s.collectorConfigError != "" {
		writeCollectorError(w, http.StatusServiceUnavailable, s.collectorConfigError)
		return
	}
	if *request.Enabled && os.Getenv("NOITU_ACCESS_TOKEN") == "" && os.Getenv("COLLECT_BROWSER") != "1" && (s.tokenStore == nil || s.tokenStore.get() == "") {
		writeCollectorError(w, http.StatusServiceUnavailable, "NOITU_ACCESS_TOKEN is required, or start with COLLECT_BROWSER=1")
		return
	}
	if err := s.persistCollector(*request.Enabled); err != nil {
		writeCollectorError(w, http.StatusServiceUnavailable, "collector settings unavailable")
		return
	}
	if s.collector == nil {
		s.collector = &collectorManager{}
	}
	status := s.collector.set(*request.Enabled, s.runCollector)
	if status.LastError != "" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	writeJSON(w, status)
}

func writeCollectorError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	writeJSON(w, collectorStatus{LastError: message})
}

func (s *server) collectorToken(w http.ResponseWriter, r *http.Request) {
	if !s.protected(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Token string `json:"token"`
	}
	if !decodeStrictJSON(w, r, &request) {
		return
	}
	if len(request.Token) < 20 || len(request.Token) > 12000 || strings.ContainsAny(request.Token, "\r\n") {
		http.Error(w, "valid token is required", http.StatusBadRequest)
		return
	}
	if s.tokenStore == nil {
		s.tokenStore = &tokenBridge{ready: make(chan struct{})}
	}
	s.tokenStore.set(request.Token)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) runCollector(ctx context.Context) error {
	token := os.Getenv("NOITU_ACCESS_TOKEN")
	if s.tokenStore != nil {
		if token == "" {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-s.tokenStore.ready:
			}
		}
		if bridgeToken := s.tokenStore.get(); bridgeToken != "" {
			token = bridgeToken
		}
	}
	if token == "" {
		return fmt.Errorf("NOITU_ACCESS_TOKEN is required, or start with COLLECT_BROWSER=1")
	}
	loadCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	index, err := s.dictionary(loadCtx)
	cancel()
	if err != nil {
		return err
	}
	save := func(ctx context.Context, word string) error {
		key, ok := parts(word)
		if !ok {
			return fmt.Errorf("invalid observed word")
		}
		_, err := s.db.ExecContext(ctx, `INSERT INTO word_observations(phrase,key_word,accepted) VALUES($1,$2,true) ON CONFLICT(phrase) DO UPDATE SET accepted=true, observed_at=now()`, word, key)
		return err
	}
	err = runCollectorSchedule(ctx, s.collectorInterval, func(parent context.Context) error {
		if s.tokenStore != nil {
			if bridgeToken := s.tokenStore.get(); bridgeToken != "" {
				token = bridgeToken
			}
		}
		session, err := newSessionID()
		if err != nil {
			return err
		}
		gameCtx, cancel := context.WithTimeout(parent, 2*time.Minute)
		err = collectGame(gameCtx, s.client, "https://api.noitu.fun/api/v1", token, session, index, 10, 3*time.Second, save)
		cancel()
		if err != nil {
			return err
		}
		log.Print("collector finished")
		return nil
	})
	return err
}

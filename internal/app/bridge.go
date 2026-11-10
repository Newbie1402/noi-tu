package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

type tokenBridge struct {
	mu    sync.RWMutex
	token string
	ready chan struct{}
	once  sync.Once
}

func (b *tokenBridge) get() string { b.mu.RLock(); defer b.mu.RUnlock(); return b.token }

func (b *tokenBridge) set(token string) {
	b.mu.Lock()
	b.token = token
	b.mu.Unlock()
	b.once.Do(func() { close(b.ready) })
}

func (b *tokenBridge) serve(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if !strings.HasPrefix(origin, "chrome-extension://") || strings.Contains(strings.TrimPrefix(origin, "chrome-extension://"), "/") {
		http.Error(w, "extension origin required", http.StatusForbidden)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "JSON POST required", http.StatusBadRequest)
		return
	}
	var data struct {
		Token string `json:"token"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	dec.DisallowUnknownFields()
	if dec.Decode(&data) != nil || len(data.Token) < 20 || len(data.Token) > 12000 || strings.ContainsAny(data.Token, "\r\n") {
		http.Error(w, "invalid token", http.StatusBadRequest)
		return
	}
	b.set(data.Token)
	w.WriteHeader(http.StatusNoContent)
}

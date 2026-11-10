package app

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/unicode/norm"
	"noitu/internal/config"
	"noitu/internal/domain"
	"noitu/internal/infrastructure/database"
)

type observation = domain.Observation
type suggestion = domain.Suggestion
type server struct {
	db                   *sql.DB
	client               *http.Client
	mu                   sync.Mutex
	words                map[string][]string
	queue                chan observation
	collector            *collectorManager
	tokenStore           *tokenBridge
	collectorInterval    time.Duration
	collectorConfigError string
	staticToken          string
	apiKey               string
	requireAPIKey        bool
	allowedOrigins       map[string]bool
	settings             *runtimeSettings
}

type sessionState = domain.SessionState

type sessionSuggestionResponse struct {
	CurrentWord string       `json:"current_word"`
	Key         string       `json:"key"`
	Suggestions []rankedWord `json:"suggestions"`
}

func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(norm.NFC.String(s))), " ")
}
func parts(s string) (string, bool) {
	p := strings.Fields(normalize(s))
	if len(p) != 2 {
		return "", false
	}
	return p[0], true
}
func (s *server) dictionary(ctx context.Context) (map[string][]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.words != nil {
		return s.words, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.vuanoitu.duckdns.org/full_dictionary.json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dictionary HTTP %d", resp.StatusCode)
	}
	var list []string
	if err = json.NewDecoder(io.LimitReader(resp.Body, 20<<20)).Decode(&list); err != nil {
		return nil, err
	}
	index := make(map[string][]string)
	seen := make(map[string]bool)
	for _, w := range list {
		w = normalize(w)
		key, ok := parts(w)
		if ok && !seen[w] {
			index[key] = append(index[key], w)
			seen[w] = true
		}
	}
	s.words = index
	return index, nil
}
func emit(w http.ResponseWriter, event string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	if err == nil {
		w.(http.Flusher).Flush()
	}
	return err
}

func (s *server) allowCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin != "" && !s.allowedOrigin(origin) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return false
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}

func (s *server) allowedOrigin(origin string) bool {
	if strings.HasPrefix(origin, "chrome-extension://") && !strings.Contains(strings.TrimPrefix(origin, "chrome-extension://"), "/") {
		return true
	}
	if len(s.allowedOrigins) == 0 && origin == "https://www.noitu.fun" {
		return true
	}
	return s.allowedOrigins[origin]
}

func (s *server) authorized(w http.ResponseWriter, r *http.Request) bool {
	if !s.requireAPIKey {
		return true
	}
	provided := r.Header.Get("X-API-Key")
	if provided == "" {
		provided = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(s.apiKey)) != 1 {
		http.Error(w, "API key required", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *server) protected(w http.ResponseWriter, r *http.Request) bool {
	return s.allowCORS(w, r) && s.authorized(w, r)
}

func (s *server) sessionSuggestions(w http.ResponseWriter, r *http.Request) {
	if !s.protected(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !s.suggestionsEnabled() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var state sessionState
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(&state) != nil || dec.Decode(&struct{}{}) != io.EOF {
		http.Error(w, "invalid session state", http.StatusBadRequest)
		return
	}
	state.CurrentWord = normalize(state.CurrentWord)
	key := normalize(state.Key)
	if key == "" {
		fields := strings.Fields(state.CurrentWord)
		if len(fields) > 0 {
			key = fields[len(fields)-1]
		}
	}
	if key == "" || len(strings.Fields(key)) != 1 {
		http.Error(w, "current word or key is required", http.StatusBadRequest)
		return
	}
	used := make(map[string]bool, len(state.Used))
	for _, word := range state.Used {
		word = normalize(word)
		if _, ok := parts(word); ok {
			used[word] = true
		}
	}
	for _, word := range state.Words {
		word = normalize(word)
		if _, ok := parts(word); ok {
			used[word] = true
			accepted := true
			select {
			case s.queue <- observation{Phrase: word, Accepted: &accepted}:
			default:
			}
		}
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT phrase, accepted FROM word_observations ORDER BY phrase")
	if err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	observed := map[string]bool{}
	for rows.Next() {
		var phrase string
		var accepted bool
		if err = rows.Scan(&phrase, &accepted); err != nil {
			break
		}
		observed[normalize(phrase)] = accepted
	}
	rows.Close()
	if err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	index, err := s.dictionary(r.Context())
	if err != nil {
		http.Error(w, "dictionary unavailable", http.StatusServiceUnavailable)
		return
	}
	ranking := rankWords(key, index, observed, used)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(sessionSuggestionResponse{CurrentWord: state.CurrentWord, Key: key, Suggestions: ranking})
}
func (s *server) suggest(w http.ResponseWriter, r *http.Request) {
	if !s.protected(w, r) {
		return
	}
	if !s.suggestionsEnabled() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	key := normalize(r.URL.Query().Get("key"))
	if key == "" || len(strings.Fields(key)) != 1 {
		http.Error(w, "key must be one syllable", 400)
		return
	}
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT phrase, accepted FROM word_observations ORDER BY phrase")
	if err != nil {
		http.Error(w, "database unavailable", 503)
		return
	}
	observed := map[string]bool{}
	used := map[string]bool{}
	for _, word := range r.URL.Query()["used"] {
		used[normalize(word)] = true
	}
	for rows.Next() {
		var phrase string
		var accepted bool
		if err = rows.Scan(&phrase, &accepted); err != nil {
			break
		}
		observed[phrase] = accepted
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		http.Error(w, "database unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	for phrase, accepted := range observed {
		first, valid := parts(phrase)
		if accepted && valid && first == key && !used[phrase] {
			if emit(w, "suggestion", suggestion{Word: phrase, Verified: true}) != nil {
				return
			}
		}
	}
	// Flush even if there are no verified words so clients receive headers promptly.
	w.(http.Flusher).Flush()
	index, err := s.dictionary(r.Context())
	if err != nil {
		emit(w, "source_error", map[string]string{"message": "dictionary unavailable"})
		emit(w, "done", struct{}{})
		return
	}
	ranking := rankWords(key, index, observed, used)
	for _, item := range ranking {
		if item.Verified {
			continue
		}
		if emit(w, "suggestion", suggestion{Word: item.Word, Verified: false}) != nil {
			return
		}
	}
	if emit(w, "ranking", ranking) != nil {
		return
	}
	emit(w, "done", struct{}{})
}
func (s *server) record(w http.ResponseWriter, r *http.Request) {
	if !s.protected(w, r) {
		return
	}
	var o observation
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if dec.Decode(&o) != nil || o.Accepted == nil {
		http.Error(w, "phrase and accepted are required", 400)
		return
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		http.Error(w, "invalid JSON body", 400)
		return
	}
	o.Phrase = normalize(o.Phrase)
	if _, ok := parts(o.Phrase); !ok || len(o.Phrase) > 256 {
		http.Error(w, "phrase must have two syllables", 400)
		return
	}
	select {
	case s.queue <- o:
		w.WriteHeader(http.StatusAccepted)
	default:
		http.Error(w, "queue full", 503)
	}
}
func (s *server) worker() {
	for o := range s.queue {
		key, _ := parts(o.Phrase)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := s.db.ExecContext(ctx, `INSERT INTO word_observations(phrase,key_word,accepted) VALUES($1,$2,$3) ON CONFLICT(phrase) DO UPDATE SET accepted=EXCLUDED.accepted, observed_at=now()`, o.Phrase, key, *o.Accepted)
		cancel()
		if err != nil {
			log.Printf("observation save failed: %v", err)
		}
	}
}

func Run() {
	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatal(err)
	}
	dsn := cfg.DatabaseURL
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	dbContext, cancelDB := context.WithTimeout(context.Background(), 5*time.Second)
	db, err := database.Open(dbContext, dsn)
	cancelDB()
	if err != nil {
		log.Fatal(err)
	}
	settingsContext, cancelSettings := context.WithTimeout(context.Background(), 5*time.Second)
	if err := ensureRuntimeSettings(settingsContext, db); err != nil {
		cancelSettings()
		log.Fatal(err)
	}
	suggestionsEnabled, collectorEnabled, err := loadRuntimeSettings(settingsContext, db)
	cancelSettings()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	tokenStore := &tokenBridge{ready: make(chan struct{})}
	runtimeSettings := newRuntimeSettings()
	runtimeSettings.suggestions.Store(suggestionsEnabled)
	s := &server{db: db, client: &http.Client{Timeout: 15 * time.Second}, queue: make(chan observation, 256), collector: &collectorManager{}, tokenStore: tokenStore, collectorInterval: cfg.CollectorInterval, apiKey: cfg.APIKey, requireAPIKey: cfg.RequireAPIKey, allowedOrigins: cfg.AllowedOrigins, settings: runtimeSettings, staticToken: cfg.StaticToken}
	if cfg.CollectorBrowser {
		listener := &http.Server{Addr: "127.0.0.1:8090", Handler: http.HandlerFunc(tokenStore.serve), ReadHeaderTimeout: 5 * time.Second}
		socket, err := net.Listen("tcp", listener.Addr)
		if err != nil {
			log.Printf("collector bridge stopped: %v", err)
		} else {
			go func() {
				if err := listener.Serve(socket); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Printf("session bridge failed: %v", err)
				}
			}()
			log.Print("browser collector bridge listening on 127.0.0.1:8090")
		}
	}
	autoStart := cfg.CollectorAutostart || cfg.CollectorOnce
	if autoStart || collectorEnabled {
		s.collector.set(true, s.runCollector)
	}
	go s.worker()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /suggest", s.suggest)
	mux.HandleFunc("POST /observations", s.record)
	mux.HandleFunc("POST /session/suggestions", s.sessionSuggestions)
	mux.HandleFunc("POST /collector/toggle", s.collectorToggle)
	mux.HandleFunc("OPTIONS /collector/toggle", s.collectorToggle)
	mux.HandleFunc("POST /collector/token", s.collectorToken)
	mux.HandleFunc("OPTIONS /collector/token", s.collectorToken)
	mux.HandleFunc("GET /settings", s.settingsHandler)
	mux.HandleFunc("POST /settings/suggestions", s.toggleSuggestions)
	mux.HandleFunc("OPTIONS /settings/suggestions", s.toggleSuggestions)
	mux.HandleFunc("OPTIONS /session/suggestions", s.sessionSuggestions)
	mux.HandleFunc("GET /openapi.json", openAPIHandler)
	mux.HandleFunc("GET /swagger", swaggerUIHandler)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	log.Printf("listening on %s", cfg.Address)
	if err := http.ListenAndServe(cfg.Address, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

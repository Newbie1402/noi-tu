package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var schema string
	if err = tx.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	t.Logf("connected to schema %s", schema)
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE word_observations (phrase text PRIMARY KEY, key_word text NOT NULL, accepted boolean NOT NULL, observed_at timestamptz NOT NULL DEFAULT now()) ON COMMIT DROP`)
	if err != nil {
		t.Fatal(err)
	}
	query := `INSERT INTO word_observations(phrase,key_word,accepted) VALUES($1,$2,$3) ON CONFLICT(phrase) DO UPDATE SET accepted=EXCLUDED.accepted, observed_at=now()`
	for _, accepted := range []bool{true, false} {
		if _, err = tx.ExecContext(ctx, query, "bình minh", "bình", accepted); err != nil {
			t.Fatal(err)
		}
	}
	var accepted bool
	if err = tx.QueryRowContext(ctx, "SELECT accepted FROM word_observations WHERE key_word=$1 AND phrase=$2", "bình", "bình minh").Scan(&accepted); err != nil || accepted {
		t.Fatalf("accepted=%v err=%v", accepted, err)
	}
}

func TestRuntimeSettingsPersistence(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err = db.ExecContext(ctx, `CREATE TEMP TABLE runtime_settings (
		id smallint PRIMARY KEY CHECK (id = 1),
		suggestions_enabled boolean NOT NULL DEFAULT true,
		collector_enabled boolean NOT NULL DEFAULT false,
		updated_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		t.Fatal(err)
	}
	s := server{db: db}
	if err = ensureRuntimeSettings(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = s.persistSuggestions(false); err != nil {
		t.Fatal(err)
	}
	if err = s.persistCollector(true); err != nil {
		t.Fatal(err)
	}
	suggestions, collector, err := loadRuntimeSettings(ctx, db)
	if err != nil || suggestions || !collector {
		t.Fatalf("suggestions=%t collector=%t err=%v", suggestions, collector, err)
	}
}

func TestPostgresHTTPFlow(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`CREATE TEMP TABLE word_observations (phrase text PRIMARY KEY, key_word text NOT NULL, accepted boolean NOT NULL, observed_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	s := server{db: db, queue: make(chan observation, 4), words: map[string][]string{"bình": {"bình minh", "bình an", "bình yên"}}, client: &http.Client{}}
	done := make(chan struct{})
	go func() { s.worker(); close(done) }()
	defer func() { close(s.queue); <-done }()
	for _, body := range []string{`{"phrase":"bình minh","accepted":true}`, `{"phrase":"bình an","accepted":false}`} {
		w := httptest.NewRecorder()
		s.record(w, httptest.NewRequest("POST", "/observations", strings.NewReader(body)))
		if w.Code != 202 {
			t.Fatalf("record: %d", w.Code)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var count int
		if err = db.QueryRow("SELECT count(*) FROM word_observations").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not persist observations")
		}
		time.Sleep(25 * time.Millisecond)
	}
	w := httptest.NewRecorder()
	s.suggest(w, httptest.NewRequest("GET", "/suggest?key=bình", nil))
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, `"word":"bình minh","verified":true`) || !strings.Contains(body, `"word":"bình yên","verified":false`) || strings.Contains(body, "bình an") || !strings.Contains(body, "event: done") {
		t.Fatalf("stream: %d %s", w.Code, body)
	}
}

func TestInitializeSchema(t *testing.T) {
	if os.Getenv("INIT_SCHEMA") != "1" {
		t.Skip("INIT_SCHEMA not set")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL is required to initialize schema")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var existing sql.NullString
	if err = db.QueryRowContext(ctx, "SELECT to_regclass('word_observations')::text").Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing.Valid {
		t.Log("table already exists; preserving existing schema")
		return
	}
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	t.Log("created word_observations and index")
}

func TestSaveObservedGameWords(t *testing.T) {
	if os.Getenv("SAVE_OBSERVED_WORDS") != "1" {
		t.Skip("SAVE_OBSERVED_WORDS not set")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := server{db: db, queue: make(chan observation, 16)}
	done := make(chan struct{})
	go func() { s.worker(); close(done) }()
	for _, body := range []string{`{"phrase":"nạt nộ","accepted":true}`, `{"phrase":"nạt xyz","accepted":false}`, `{"phrase":"kính lúp","accepted":true}`, `{"phrase":"lúp xúp","accepted":true}`, `{"phrase":"xúp cua","accepted":true}`, `{"phrase":"cua biển","accepted":true}`, `{"phrase":"biển hiệu","accepted":true}`, `{"phrase":"hiệu quả","accepted":true}`, `{"phrase":"quả báo","accepted":true}`, `{"phrase":"báo xyz","accepted":false}`, `{"phrase":"báo chí","accepted":true}`, `{"phrase":"chí mạng","accepted":true}`} {
		w := httptest.NewRecorder()
		s.record(w, httptest.NewRequest("POST", "/observations", strings.NewReader(body)))
		if w.Code != http.StatusAccepted {
			t.Fatalf("record: %d", w.Code)
		}
	}
	close(s.queue)
	<-done
	for phrase, want := range map[string]bool{"nạt nộ": true, "nạt xyz": false, "kính lúp": true, "lúp xúp": true, "xúp cua": true, "cua biển": true, "biển hiệu": true, "hiệu quả": true, "quả báo": true, "báo xyz": false, "báo chí": true, "chí mạng": true} {
		var got bool
		if err := db.QueryRow("SELECT accepted FROM word_observations WHERE phrase=$1", phrase).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: %v %v", phrase, got, err)
		}
	}
	s.client = &http.Client{Timeout: 20 * time.Second}
	w := httptest.NewRecorder()
	s.suggest(w, httptest.NewRequest("GET", "/suggest?key=nạt", nil))
	stream := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(stream, `"word":"nạt nộ","verified":true`) || strings.Contains(stream, "nạt xyz") || strings.Contains(stream, "event: source_error") || !strings.Contains(stream, "event: done") {
		t.Fatalf("unexpected stream: status=%d body=%s", w.Code, stream)
	}
}

func TestInspectPersistentData(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var database, schema string
	var count int
	if err := db.QueryRow("SELECT current_database(), current_schema(), (SELECT count(*) FROM public.word_observations)").Scan(&database, &schema, &count); err != nil {
		t.Fatal(err)
	}
	t.Logf("database=%s schema=%s table=public.word_observations rows=%d", database, schema, count)
	for phrase, want := range map[string]bool{"nạt nộ": true, "nạt xyz": false} {
		var got bool
		var key string
		if err := db.QueryRow("SELECT key_word, accepted FROM public.word_observations WHERE phrase=$1", phrase).Scan(&key, &got); err != nil {
			t.Fatal(err)
		}
		if got != want || key != "nạt" {
			t.Fatalf("unexpected row %q: key=%q accepted=%t", phrase, key, got)
		}
		t.Logf("phrase=%q key=%q accepted=%t", phrase, key, got)
	}
}

func TestSessionSuggestionsEndpoint(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`CREATE TEMP TABLE word_observations (phrase text PRIMARY KEY, key_word text NOT NULL, accepted boolean NOT NULL, observed_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO word_observations(phrase,key_word,accepted) VALUES ('nạt nộ','nạt',true),('nạt xyz','nạt',false)`); err != nil {
		t.Fatal(err)
	}
	s := server{db: db, client: &http.Client{}, queue: make(chan observation, 4), words: map[string][]string{"nạt": {"nạt nộ", "nạt dậy", "nạt xyz"}}}
	req := httptest.NewRequest(http.MethodPost, "/session/suggestions", strings.NewReader(`{"current_word":"ao cá","key":"nạt","used":[],"words":["ao cá"]}`))
	req.Header.Set("Origin", "https://www.noitu.fun")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.sessionSuggestions(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"word":"nạt nộ"`) || strings.Contains(w.Body.String(), `"word":"nạt xyz"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

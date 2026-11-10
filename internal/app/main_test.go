package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeAndParts(t *testing.T) {
	for _, tc := range []struct {
		input, key string
		ok         bool
	}{{"  BÌNH   Minh ", "bình", true}, {"a b c", "", false}, {"single", "", false}} {
		key, ok := parts(tc.input)
		if key != tc.key || ok != tc.ok {
			t.Fatalf("parts(%q) = %q, %v", tc.input, key, ok)
		}
	}
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDictionaryDeduplicatesAndFilters(t *testing.T) {
	calls := 0
	s := server{client: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: ioNop(`["Bình Minh","bình minh","bình an","bình an vui"]`)}, nil
	})}}
	for i := 0; i < 2; i++ {
		idx, err := s.dictionary(context.Background())
		if err != nil || len(idx["bình"]) != 2 {
			t.Fatalf("index %v err %v", idx, err)
		}
	}
	if calls != 1 {
		t.Fatalf("calls: %d", calls)
	}
}

type readCloser struct{ *strings.Reader }

func (readCloser) Close() error { return nil }
func ioNop(s string) readCloser { return readCloser{strings.NewReader(s)} }
func TestRecordValidatesAndQueues(t *testing.T) {
	s := server{queue: make(chan observation, 1)}
	for _, tc := range []struct {
		body   string
		status int
	}{{`{"phrase":" Bình Minh ","accepted":true}`, 202}, {`{"phrase":"bình minh","accepted":false}`, 503}, {`{"phrase":"a b c","accepted":true}`, 400}, {`{"phrase":"a b"}`, 400}, {`{"phrase":"a b","accepted":true} garbage`, 400}} {
		w := httptest.NewRecorder()
		s.record(w, httptest.NewRequest("POST", "/observations", strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.body, w.Code)
		}
	}
	if o := <-s.queue; o.Phrase != "bình minh" || !*o.Accepted {
		t.Fatalf("observation: %+v", o)
	}
}

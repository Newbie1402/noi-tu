package app

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBridgeRejectsWebAndAcceptsExtension(t *testing.T) {
	b := &tokenBridge{ready: make(chan struct{})}
	for _, tc := range []struct {
		origin, body string
		status       int
	}{{"https://www.noitu.fun", `{"token":"abcdefghijklmnopqrstuv"}`, 403}, {"chrome-extension://abc", `{"token":"short"}`, 400}, {"chrome-extension://abc", `{"token":"abcdefghijklmnopqrstuv"}`, 204}} {
		req := httptest.NewRequest("POST", "/token", strings.NewReader(tc.body))
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		b.serve(w, req)
		if w.Code != tc.status {
			t.Fatalf("origin %s status %d", tc.origin, w.Code)
		}
	}
	if b.get() != "abcdefghijklmnopqrstuv" {
		t.Fatal("token not retained")
	}
	select {
	case <-b.ready:
	default:
		t.Fatal("collector not notified")
	}
}

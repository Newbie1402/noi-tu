package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollectorManagerToggleCancelsRun(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	manager := &collectorManager{}

	manager.set(true, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})
	<-started
	if !manager.status().Running {
		t.Fatal("collector should be running")
	}
	status := manager.set(false, nil)
	if status.Running {
		t.Fatal("collector should be stopped")
	}
	<-stopped
}

func TestCollectorToggleValidatesJSON(t *testing.T) {
	t.Setenv("NOITU_ACCESS_TOKEN", "")
	s := server{collector: &collectorManager{}}
	for _, body := range []string{`{}`, `{"enabled":true} {"enabled":false}`} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/collector/toggle", strings.NewReader(body))
		s.collectorToggle(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d", body, recorder.Code)
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/collector/toggle", strings.NewReader(`{"enabled":false}`))
	s.collectorToggle(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"enabled":false`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/collector/toggle", strings.NewReader(`{"enabled":true}`))
	s.collectorToggle(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "NOITU_ACCESS_TOKEN") {
		t.Fatalf("missing token status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

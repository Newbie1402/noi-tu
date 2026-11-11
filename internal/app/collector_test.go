package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestCollectGameStopsOnRejectedAnswer(t *testing.T) {
	var answers []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Error("missing authorization")
		}
		if r.URL.Path == "/word-link/start" {
			w.Write([]byte(`{"word":"bắt nạt"}`))
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		answers = append(answers, body["answer"])
		if len(answers) == 1 {
			w.Write([]byte(`{"isSuccessful":true,"wordDescription":{"word":"nộ nạt"}}`))
		} else {
			w.Write([]byte(`{"isSuccessful":false,"message":"invalid"}`))
		}
	}))
	defer upstream.Close()
	var saved []string
	err := collectGame(context.Background(), upstream.Client(), upstream.URL, "test", "s", map[string][]string{"nạt": {"nạt nộ", "nạt xyz"}}, 10, 0, func(_ context.Context, w string) error { saved = append(saved, w); return nil })
	if err != nil || !reflect.DeepEqual(saved, []string{"bắt nạt", "nạt nộ", "nộ nạt"}) || !reflect.DeepEqual(answers, []string{"nạt nộ", "nạt xyz"}) {
		t.Fatalf("saved=%v answers=%v err=%v", saved, answers, err)
	}
}

func TestCollectorScheduleRepeatsAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := runCollectorSchedule(ctx, time.Millisecond, func(context.Context) error {
		calls++
		if calls == 2 {
			cancel()
		}
		return nil
	})
	if err != context.Canceled || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
func TestCollectGameRequiresExplicitSuccess(t *testing.T) {
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/word-link/start" {
			w.Write([]byte(`{"word":"a b"}`))
		} else {
			w.Write([]byte(`{}`))
		}
	}))
	defer u.Close()
	var saved []string
	err := collectGame(context.Background(), u.Client(), u.URL, "token", "id", map[string][]string{"b": {"b c"}}, 1, 0, func(_ context.Context, w string) error { saved = append(saved, w); return nil })
	if err == nil || !reflect.DeepEqual(saved, []string{"a b"}) {
		t.Fatalf("saved=%v err=%v", saved, err)
	}
}

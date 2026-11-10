package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type gameWord struct {
	Word string `json:"word"`
}
type gameResponse struct {
	Word            string   `json:"word"`
	WordDescription gameWord `json:"wordDescription"`
	Successful      *bool    `json:"isSuccessful"`
	Finished        bool     `json:"isFinished"`
}

func (r gameResponse) currentWord() string {
	if r.Word != "" {
		return normalize(r.Word)
	}
	return normalize(r.WordDescription.Word)
}

func runCollectorSchedule(ctx context.Context, interval time.Duration, run func(context.Context) error) error {
	for {
		if err := run(ctx); err != nil {
			return err
		}
		if interval == 0 {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func gameRequest(ctx context.Context, client *http.Client, method, endpoint, token string, body io.Reader) (gameResponse, error) {
	var result gameResponse
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return result, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return result, fmt.Errorf("noitu.fun returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result)
	return result, err
}

func collectGame(ctx context.Context, client *http.Client, base, token, session string, index map[string][]string, maxMoves int, delay time.Duration, save func(context.Context, string) error) error {
	state, err := gameRequest(ctx, client, "GET", base+"/word-link/init?sessionId="+url.QueryEscape(session), token, nil)
	if err != nil {
		return err
	}
	word := state.currentWord()
	if _, ok := parts(word); !ok {
		return fmt.Errorf("start response missing two-syllable word")
	}
	if err = save(ctx, word); err != nil {
		return err
	}
	used := map[string]bool{word: true}
	for move := 0; move < maxMoves; move++ {
		key := strings.Fields(word)[1]
		var candidate string
		for _, w := range index[key] {
			if !used[w] {
				candidate = w
				break
			}
		}
		if candidate == "" {
			return nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		payload, err := json.Marshal(map[string]string{"answer": candidate, "sessionId": session})
		if err != nil {
			return err
		}
		state, err = gameRequest(ctx, client, "POST", base+"/word-link/answer", token, strings.NewReader(string(payload)))
		if err != nil {
			return err
		}
		if state.Successful == nil {
			return fmt.Errorf("answer response missing isSuccessful")
		}
		if !*state.Successful {
			return nil
		}
		if err = save(ctx, candidate); err != nil {
			return err
		}
		used[candidate] = true
		if state.Finished {
			return nil
		}
		word = state.currentWord()
		if _, ok := parts(word); !ok {
			return fmt.Errorf("answer response missing next word")
		}
		if err = save(ctx, word); err != nil {
			return err
		}
		used[word] = true
	}
	return nil
}

func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 0x40
	b[8] = (b[8] & 63) | 0x80
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}

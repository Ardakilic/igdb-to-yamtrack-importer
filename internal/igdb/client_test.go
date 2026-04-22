package igdb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ardakilic/igdb-yamtrack-importer/internal/igdbcsv"
	"github.com/ardakilic/igdb-yamtrack-importer/internal/mapping"
)

// fastTicker returns a high-frequency ticker channel so tests are not slowed down
// by the 250 ms production rate limit.
func fastTicker() <-chan time.Time {
	ch := make(chan time.Time, 1)
	go func() {
		for {
			ch <- time.Now()
			time.Sleep(time.Millisecond)
		}
	}()
	return ch
}

// newTestClient creates a client wired to a test server with a fast ticker.
func newTestClient(t *testing.T, oauthURL, apiBase string) *Client {
	t.Helper()
	return newClientWithTicker("cid", "csec", oauthURL, apiBase, nil, fastTicker())
}

// oauthResponse writes a canned Twitch token response.
func oauthResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokenResponse{
		AccessToken: "testtoken",
		ExpiresIn:   3600,
		TokenType:   "bearer",
	})
}

func TestEnrichBatch_Empty(t *testing.T) {
	c := newClientWithTicker("cid", "csec", "http://noserver", "http://noserver", nil, fastTicker())
	warns, err := c.EnrichBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %v", warns)
	}
}

func TestEnrichBatch_HappyPath(t *testing.T) {
	gamesResp := []map[string]any{
		{"id": 1942, "name": "1942 Enriched"},
		{"id": 76882, "name": "The Witcher 3"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "token"):
			oauthResponse(w)
		default:
			if r.Header.Get("Client-ID") == "" {
				t.Error("missing Client-ID header")
			}
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				t.Error("missing Bearer Authorization header")
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(gamesResp)
		}
	}))
	defer srv.Close()

	r1 := 80.0
	games := []igdbcsv.Game{
		{IGDBID: 1942, Title: "old 1942", Status: mapping.StatusCompleted, UserRating: &r1},
		{IGDBID: 76882, Title: "old tw3", Status: mapping.StatusCompleted},
	}

	c := newTestClient(t, srv.URL+"/token", srv.URL)
	warns, err := c.EnrichBatch(context.Background(), games)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if games[0].Title != "1942 Enriched" {
		t.Errorf("title not updated: %q", games[0].Title)
	}
	if games[1].Title != "The Witcher 3" {
		t.Errorf("title not updated: %q", games[1].Title)
	}
}

func TestEnrichBatch_MissingIDWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			oauthResponse(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1942, "name": "1942"},
		})
	}))
	defer srv.Close()

	games := []igdbcsv.Game{
		{IGDBID: 1942, Title: "1942", Status: mapping.StatusCompleted},
		{IGDBID: 99999, Title: "Ghost", Status: mapping.StatusCompleted},
	}

	c := newTestClient(t, srv.URL+"/token", srv.URL)
	warns, err := c.EnrichBatch(context.Background(), games)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 1 {
		t.Fatalf("expected 1 warning for missing ID, got %d: %v", len(warns), warns)
	}
	if !strings.Contains(warns[0], "99999") {
		t.Errorf("warning should mention ID 99999: %q", warns[0])
	}
}

func TestEnrichBatch_TokenRefreshOnExpiry(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			callCount++
			// Return a token expiring in 4 minutes (below the 5-minute buffer → always refreshes).
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenResponse{
				AccessToken: "expiring-soon",
				ExpiresIn:   240, // 4 minutes
				TokenType:   "bearer",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1942, "name": "1942"}})
	}))
	defer srv.Close()

	games := []igdbcsv.Game{{IGDBID: 1942, Title: "1942", Status: mapping.StatusCompleted}}

	c := newTestClient(t, srv.URL+"/token", srv.URL)
	if _, err := c.EnrichBatch(context.Background(), games); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := c.EnrichBatch(context.Background(), games); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 token fetches (token expires within buffer each time), got %d", callCount)
	}
}

func Test429WithRetryAfterHeader(t *testing.T) {
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			oauthResponse(w)
			return
		}
		attempt++
		if attempt < 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1942, "name": "1942"}})
	}))
	defer srv.Close()

	games := []igdbcsv.Game{{IGDBID: 1942, Title: "1942", Status: mapping.StatusCompleted}}
	c := newTestClient(t, srv.URL+"/token", srv.URL)
	if _, err := c.EnrichBatch(context.Background(), games); err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
	if attempt < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempt)
	}
}

func Test5xxRetry(t *testing.T) {
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			oauthResponse(w)
			return
		}
		attempt++
		if attempt < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1942, "name": "1942"}})
	}))
	defer srv.Close()

	games := []igdbcsv.Game{{IGDBID: 1942, Title: "1942", Status: mapping.StatusCompleted}}
	c := newTestClient(t, srv.URL+"/token", srv.URL)
	if _, err := c.EnrichBatch(context.Background(), games); err != nil {
		t.Fatalf("expected 5xx retry to succeed: %v", err)
	}
}

func Test5xxExhaustsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			oauthResponse(w)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	games := []igdbcsv.Game{{IGDBID: 1942, Title: "1942", Status: mapping.StatusCompleted}}
	c := newTestClient(t, srv.URL+"/token", srv.URL)
	if _, err := c.EnrichBatch(context.Background(), games); err == nil {
		t.Fatal("expected error after exhausted retries")
	}
}

func TestContextCancellationDuringEnrich(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			oauthResponse(w)
			return
		}
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	games := []igdbcsv.Game{{IGDBID: 1942, Title: "1942", Status: mapping.StatusCompleted}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	c := newTestClient(t, srv.URL+"/token", srv.URL)
	if _, err := c.EnrichBatch(ctx, games); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestOAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("invalid credentials"))
	}))
	defer srv.Close()

	games := []igdbcsv.Game{{IGDBID: 1942, Title: "1942", Status: mapping.StatusCompleted}}
	c := newTestClient(t, srv.URL+"/token", srv.URL)
	if _, err := c.EnrichBatch(context.Background(), games); err == nil {
		t.Fatal("expected OAuth failure error")
	}
}

func TestRequestBodyContainsIDs(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			oauthResponse(w)
			return
		}
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "name": "Game 1"},
			{"id": 2, "name": "Game 2"},
		})
	}))
	defer srv.Close()

	games := []igdbcsv.Game{
		{IGDBID: 1, Status: mapping.StatusCompleted},
		{IGDBID: 2, Status: mapping.StatusCompleted},
	}
	c := newTestClient(t, srv.URL+"/token", srv.URL)
	if _, err := c.EnrichBatch(context.Background(), games); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedBody, "1") || !strings.Contains(capturedBody, "2") {
		t.Errorf("request body should contain IDs 1 and 2, got: %q", capturedBody)
	}
}

func TestNonOKResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			oauthResponse(w)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad query"))
	}))
	defer srv.Close()

	games := []igdbcsv.Game{{IGDBID: 1, Status: mapping.StatusCompleted}}
	c := newTestClient(t, srv.URL+"/token", srv.URL)
	if _, err := c.EnrichBatch(context.Background(), games); err == nil {
		t.Fatal("expected error on 400 response")
	}
}

func TestNewClient_DefaultHTTPClient(t *testing.T) {
	// Verify the exported NewClient constructor does not panic and sets a non-nil httpClient.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			oauthResponse(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "name": "G"}})
	}))
	defer srv.Close()

	// NewClient creates a 250ms ticker; we can't test quickly with it, but we can
	// verify it wires up correctly by issuing a request after draining the first tick.
	c := NewClient("cid", "csec", srv.URL+"/token", srv.URL, nil)
	// Replace the ticker with a fast one so the test doesn't take 250ms+.
	c.rateTicker = fastTicker()

	games := []igdbcsv.Game{{IGDBID: 1, Status: mapping.StatusCompleted}}
	if _, err := c.EnrichBatch(context.Background(), games); err != nil {
		t.Fatalf("NewClient happy path failed: %v", err)
	}
}

func TestRetryAfterNonIntegerValue(t *testing.T) {
	// When Retry-After is not a plain integer, retryAfter should fall back to defaultDelay.
	resp := &http.Response{
		Header: http.Header{"Retry-After": []string{"Thu, 01 Jan 2099 00:00:00 GMT"}},
	}
	got := retryAfter(resp, 42*time.Millisecond)
	if got != 42*time.Millisecond {
		t.Errorf("expected fallback delay 42ms, got %v", got)
	}
}

func TestRetryAfterMissingHeader(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	got := retryAfter(resp, 10*time.Millisecond)
	if got != 10*time.Millisecond {
		t.Errorf("expected fallback delay 10ms, got %v", got)
	}
}

func TestBearerTokenCacheHit(t *testing.T) {
	// First call fetches the token; second call within the same client should NOT re-fetch
	// (token has a 1-hour expiry which is well above the 5-minute buffer).
	tokenFetchCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			tokenFetchCount++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenResponse{
				AccessToken: "cached-token",
				ExpiresIn:   3600,
				TokenType:   "bearer",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "name": "G"}})
	}))
	defer srv.Close()

	games := []igdbcsv.Game{{IGDBID: 1, Status: mapping.StatusCompleted}}
	c := newTestClient(t, srv.URL+"/token", srv.URL)
	if _, err := c.EnrichBatch(context.Background(), games); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := c.EnrichBatch(context.Background(), games); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if tokenFetchCount != 1 {
		t.Errorf("expected exactly 1 token fetch (cache hit on second call), got %d", tokenFetchCount)
	}
}

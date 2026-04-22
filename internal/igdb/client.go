// Package igdb provides an IGDB API client that handles Twitch OAuth2 authentication,
// token caching, rate limiting, and batch game enrichment.
package igdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ardakilic/igdb-yamtrack-importer/internal/igdbcsv"
)

const (
	// batchSize is the IGDB maximum number of IDs per query.
	batchSize = 500
	// defaultRateInterval enforces the 4 req/s IGDB rate limit (1 tick per 250 ms).
	defaultRateInterval = 250 * time.Millisecond
	// maxConcurrent is the IGDB maximum number of open requests at a time.
	maxConcurrent = 8
	// maxRetries is the number of times to retry 429 / 5xx responses.
	maxRetries = 5
)

// tokenResponse is the Twitch OAuth2 token endpoint response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// gameRecord is a partial IGDB game response used for enrichment.
type gameRecord struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	FirstReleaseDate int64  `json:"first_release_date"` // Unix timestamp
}

// Client is an IGDB REST API client with token caching and rate limiting.
type Client struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	oauthURL     string
	apiBase      string

	tokenMu  sync.Mutex
	token    string
	tokenExp time.Time

	// rateTicker serialises requests to ≤4/s. Semaphore limits concurrent in-flight.
	rateTicker <-chan time.Time
	semaphore  chan struct{}
}

// NewClient constructs an IGDB client. oauthURL and apiBase are overridable for tests.
func NewClient(clientID, clientSecret, oauthURL, apiBase string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	ticker := time.NewTicker(defaultRateInterval)
	return newClient(clientID, clientSecret, oauthURL, apiBase, httpClient, ticker.C)
}

// newClientWithTicker is used by tests to inject a fast ticker, avoiding 250ms waits per batch.
func newClientWithTicker(clientID, clientSecret, oauthURL, apiBase string, httpClient *http.Client, ticker <-chan time.Time) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return newClient(clientID, clientSecret, oauthURL, apiBase, httpClient, ticker)
}

func newClient(clientID, clientSecret, oauthURL, apiBase string, httpClient *http.Client, ticker <-chan time.Time) *Client {
	return &Client{
		httpClient:   httpClient,
		clientID:     clientID,
		clientSecret: clientSecret,
		oauthURL:     oauthURL,
		apiBase:      strings.TrimRight(apiBase, "/"),
		rateTicker:   ticker,
		semaphore:    make(chan struct{}, maxConcurrent),
	}
}

// EnrichBatch validates and enriches the provided games slice in place using the IGDB API.
// Games not returned by IGDB (deleted or merged IDs) are noted via the returned warnings slice.
// The method respects the 4 req/s + 8 concurrent IGDB rate limits.
func (c *Client) EnrichBatch(ctx context.Context, games []igdbcsv.Game) (warnings []string, err error) {
	if len(games) == 0 {
		return nil, nil
	}

	// Index games by IGDB ID for O(1) update after each batch response.
	gamesByID := make(map[int]*igdbcsv.Game, len(games))
	for i := range games {
		gamesByID[games[i].IGDBID] = &games[i]
	}

	// Collect unique IDs.
	ids := make([]int, 0, len(gamesByID))
	for id := range gamesByID {
		ids = append(ids, id)
	}

	// Process in batches of batchSize.
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]

		records, batchErr := c.fetchBatch(ctx, batch)
		if batchErr != nil {
			return warnings, batchErr
		}

		found := make(map[int]bool, len(records))
		for _, rec := range records {
			found[rec.ID] = true
			if g, ok := gamesByID[rec.ID]; ok && rec.Name != "" {
				g.Title = rec.Name
			}
		}

		for _, id := range batch {
			if !found[id] {
				warnings = append(warnings, fmt.Sprintf("IGDB ID %d not found (deleted or merged)", id))
			}
		}
	}

	return warnings, nil
}

// fetchBatch requests game metadata for a slice of IDs. It enforces rate limiting.
func (c *Client) fetchBatch(ctx context.Context, ids []int) ([]gameRecord, error) {
	if err := c.acquireSlot(ctx); err != nil {
		return nil, err
	}
	defer c.releaseSlot()

	token, err := c.bearerToken(ctx)
	if err != nil {
		return nil, err
	}

	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = strconv.Itoa(id)
	}
	body := fmt.Sprintf("fields name,first_release_date; where id = (%s); limit %d;",
		strings.Join(idStrs, ","), batchSize)

	return c.doQuery(ctx, token, "/games", body)
}

// doQuery posts an Apicalypse body to endpoint with exponential backoff on 429/5xx.
func (c *Client) doQuery(ctx context.Context, token, endpoint, body string) ([]gameRecord, error) {
	reqURL := c.apiBase + endpoint
	delay := 100 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("building IGDB request: %w", err)
		}
		req.Header.Set("Client-ID", c.clientID)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "text/plain")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("IGDB request failed: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			wait := retryAfter(resp, delay)
			if attempt == maxRetries {
				return nil, fmt.Errorf("IGDB rate limit exceeded after %d retries", maxRetries)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			delay = minDuration(delay*2, 1600*time.Millisecond)
			continue
		}

		if resp.StatusCode >= 500 {
			resp.Body.Close()
			if attempt == maxRetries {
				return nil, fmt.Errorf("IGDB server error %d after %d retries", resp.StatusCode, maxRetries)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			delay = minDuration(delay*2, 1600*time.Millisecond)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("IGDB returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
		}

		var records []gameRecord
		if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding IGDB response: %w", err)
		}
		resp.Body.Close()
		return records, nil
	}
	return nil, fmt.Errorf("IGDB request exhausted retries")
}

// bearerToken returns a valid bearer token, refreshing if near expiry.
func (c *Client) bearerToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.token != "" && time.Until(c.tokenExp) > 5*time.Minute {
		return c.token, nil
	}

	params := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"client_credentials"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthURL+"?"+params.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("building OAuth request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("OAuth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OAuth returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decoding OAuth response: %w", err)
	}

	c.token = tr.AccessToken
	// Subtract 5 minutes to refresh before actual expiry.
	c.tokenExp = time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second - 5*time.Minute)
	return c.token, nil
}

// acquireSlot blocks until a rate-tick has fired and a concurrency slot is free.
func (c *Client) acquireSlot(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.rateTicker:
	}
	select {
	case c.semaphore <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (c *Client) releaseSlot() {
	<-c.semaphore
}

// retryAfter reads the Retry-After header; falls back to defaultDelay.
func retryAfter(resp *http.Response, defaultDelay time.Duration) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return defaultDelay
}

// minDuration returns the smaller of two durations.
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

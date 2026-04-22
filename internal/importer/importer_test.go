package importer

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardakilic/igdb-yamtrack-importer/internal/config"
)

func igdbServer(t *testing.T, gamesResp any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "token") {
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "tok",
				"expires_in":   3600,
				"token_type":   "bearer",
			})
			return
		}
		json.NewEncoder(w).Encode(gamesResp)
	}))
}

func TestRun_NoCreds_NoEnrichment(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "out.csv")
	cfg := &config.Config{
		Input:      "../../testdata/igdb/combined.csv",
		OutputFile: outFile,
		EnrichMode: config.EnrichAuto,
	}

	res, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.GamesRead != 3 {
		t.Errorf("expected 3 games read, got %d", res.GamesRead)
	}
	if res.GamesEnriched != 0 {
		t.Errorf("expected 0 enriched (no creds), got %d", res.GamesEnriched)
	}

	rows := readOutputCSV(t, outFile)
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
}

func TestRun_WithEnrichment(t *testing.T) {
	srv := igdbServer(t, []map[string]any{
		{"id": 1942, "name": "1942 API"},
		{"id": 119388, "name": "Elden Ring API"},
		{"id": 2155, "name": "Cyberpunk API"},
	})
	defer srv.Close()

	outFile := filepath.Join(t.TempDir(), "out.csv")
	cfg := &config.Config{
		Input:            "../../testdata/igdb/combined.csv",
		OutputFile:       outFile,
		EnrichMode:       config.EnrichAlways,
		IGDBClientID:     "cid",
		IGDBClientSecret: "csec",
		IGDBAPIBase:      srv.URL,
		IGDBOAuthURL:     srv.URL + "/token",
	}

	res, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.GamesRead != 3 {
		t.Errorf("expected 3 games read, got %d", res.GamesRead)
	}
	if res.GamesEnriched != 3 {
		t.Errorf("expected 3 enriched, got %d", res.GamesEnriched)
	}

	rows := readOutputCSV(t, outFile)
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	titleIdx := headerIndex(t, rows[0], "title")
	if !strings.Contains(rows[1][titleIdx], "API") &&
		!strings.Contains(rows[2][titleIdx], "API") &&
		!strings.Contains(rows[3][titleIdx], "API") {
		t.Error("expected at least one title updated with 'API' suffix from enrichment")
	}
}

func TestRun_DirMode(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "out.csv")
	cfg := &config.Config{
		Input:      "../../testdata/igdb",
		InputIsDir: true,
		OutputFile: outFile,
		EnrichMode: config.EnrichNever,
	}

	res, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.GamesRead == 0 {
		t.Fatal("expected games from directory, got none")
	}

	rows := readOutputCSV(t, outFile)
	if len(rows) < 2 {
		t.Fatal("expected at least one data row in output")
	}
}

func TestRun_EnrichmentWithWarning(t *testing.T) {
	srv := igdbServer(t, []map[string]any{
		{"id": 1942, "name": "1942"},
	})
	defer srv.Close()

	outFile := filepath.Join(t.TempDir(), "out.csv")
	cfg := &config.Config{
		Input:            "../../testdata/igdb/combined.csv",
		OutputFile:       outFile,
		EnrichMode:       config.EnrichAlways,
		IGDBClientID:     "cid",
		IGDBClientSecret: "csec",
		IGDBAPIBase:      srv.URL,
		IGDBOAuthURL:     srv.URL + "/token",
	}

	res, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Error("expected warnings for missing IGDB IDs, got none")
	}
}

func TestRun_InvalidCSVPath(t *testing.T) {
	cfg := &config.Config{
		Input:      "/nonexistent/file.csv",
		OutputFile: filepath.Join(t.TempDir(), "out.csv"),
		EnrichMode: config.EnrichNever,
	}
	_, err := Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for missing CSV file")
	}
}

func TestRun_OutputPath(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir", "nested")
	outFile := filepath.Join(subDir, "out.csv")
	cfg := &config.Config{
		Input:      "../../testdata/igdb/combined.csv",
		OutputFile: outFile,
		EnrichMode: config.EnrichNever,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validation error: %v", err)
	}
	res, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.GamesRead == 0 {
		t.Error("expected games read")
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1)
	defer cancel()

	cfg := &config.Config{
		Input:            "../../testdata/igdb/combined.csv",
		OutputFile:       filepath.Join(t.TempDir(), "out.csv"),
		EnrichMode:       config.EnrichAlways,
		IGDBClientID:     "cid",
		IGDBClientSecret: "csec",
		IGDBAPIBase:      srv.URL,
		IGDBOAuthURL:     srv.URL + "/token",
	}

	_, err := Run(ctx, cfg)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestRun_WithStatusFlag(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "out.csv")
	cfg := &config.Config{
		Input:      "../../testdata/igdb/combined.csv",
		Status:     "playing",
		OutputFile: outFile,
		EnrichMode: config.EnrichNever,
	}

	res, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.GamesRead != 3 {
		t.Errorf("expected 3 games, got %d", res.GamesRead)
	}
}

func readOutputCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening output csv: %v", err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("reading output csv: %v", err)
	}
	return rows
}

func headerIndex(t *testing.T, row []string, col string) int {
	t.Helper()
	for i, h := range row {
		if h == col {
			return i
		}
	}
	t.Fatalf("column %q not found in header %v", col, row)
	return -1
}

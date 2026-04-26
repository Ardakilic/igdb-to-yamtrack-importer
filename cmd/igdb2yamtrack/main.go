package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ardakilic/igdb-yamtrack-importer/internal/config"
	"github.com/ardakilic/igdb-yamtrack-importer/internal/importer"
)

func main() {
	os.Exit(run())
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `igdb2yamtrack - Import IGDB CSV exports to YamTrack format

Usage: igdb2yamtrack <input> [flags]

Arguments:
  <input>   Path to CSV file or directory containing CSV files

Flags:
  --status string          Status for single-list CSV (played, playing, want_to_play, etc.)
  --output string          Output file path (default "./yamtrack-import.csv")
  --enrich-mode string     API enrichment: auto, always, or never (default "auto")
  --igdb-client-id string  Twitch/IGDB OAuth client ID
  --igdb-client-secret string  Twitch/IGDB OAuth client secret
  --igdb-api-base string  IGDB REST API base URL (default "https://api.igdb.com/v4")
  --igdb-oauth-url string  Twitch OAuth2 token endpoint (default "https://id.twitch.tv/oauth2/token")
  --log-level string       Log level: debug, info, warn, error (default "info")

Examples:
  # Import a single CSV file
  igdb2yamtrack ./my-igdb-export.csv

  # Import all CSV files from a directory
  igdb2yamtrack ./igdb-exports/

  # Import with IGDB enrichment
  igdb2yamtrack ./my-igdb-export.csv \
    --igdb-client-id YOUR_CLIENT_ID \
    --igdb-client-secret YOUR_CLIENT_SECRET \
    --enrich-mode always

  # Docker: build image first
  docker build -t igdb2yamtrack .

  # Docker: run directly after build
  docker run --rm -v "$(pwd):/data" igdb2yamtrack /data/igdb-export.csv \
    --output /data/output.csv \
    --igdb-client-id YOUR_CLIENT_ID \
    --igdb-client-secret YOUR_CLIENT_SECRET

  # Docker: single CSV with status and API enrichment
  docker run --rm \
    -v ~/igdb-exports:/data:ro \
    -v ~/output:/output \
    igdb2yamtrack /data/played.csv \
    --status played \
    --output /output/yamtrack-import.csv \
    --igdb-client-id YOUR_CLIENT_ID \
    --igdb-client-secret YOUR_CLIENT_SECRET \
    --enrich-mode always
`)
}

func run() int {
	args := os.Args[1:]

	// Handle help flag before parsing
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		printUsage()
		return 0
	}

	cfg, err := config.ParseFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n Try --help for usage.\n", err)
		return 1
	}

	setupLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "validation error: %v\n", err)
		return 1
	}

	res, err := importer.Run(ctx, cfg)
	if err != nil {
		slog.Error("import failed", "error", err)
		return 1
	}

	fmt.Printf("Done: %d games read", res.GamesRead)
	if res.GamesEnriched > 0 {
		fmt.Printf(", %d enriched via IGDB API", res.GamesEnriched)
	}
	if len(res.Warnings) > 0 {
		fmt.Printf(", %d warnings", len(res.Warnings))
	}
	fmt.Printf("\nOutput: %s\n", res.OutputFile)
	return 0
}

func setupLogger(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})))
}

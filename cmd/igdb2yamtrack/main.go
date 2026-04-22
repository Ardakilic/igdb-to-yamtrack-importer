package main

import (
	"context"
	"flag"
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

func run() int {
	args := os.Args[1:]

	// Handle help flag before parsing
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		flag.Usage()
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

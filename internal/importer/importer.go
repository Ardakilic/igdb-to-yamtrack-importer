// Package importer orchestrates the full IGDB → Yamtrack migration pipeline.
package importer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ardakilic/igdb-yamtrack-importer/internal/config"
	"github.com/ardakilic/igdb-yamtrack-importer/internal/igdb"
	"github.com/ardakilic/igdb-yamtrack-importer/internal/igdbcsv"
	"github.com/ardakilic/igdb-yamtrack-importer/internal/mapping"
	"github.com/ardakilic/igdb-yamtrack-importer/internal/yamtrack"
)

// Result summarises what happened during an import run.
type Result struct {
	GamesRead     int
	GamesEnriched int
	Warnings      []string
	OutputFile    string
}

// Run executes the full pipeline: parse CSV → (optional) enrich → write Yamtrack CSV.
func Run(ctx context.Context, cfg *config.Config) (*Result, error) {
	slog.Info("reading IGDB CSV exports")

	var games []igdbcsv.Game
	var err error

	input := cfg.Input
	if cfg.InputIsDir {
		games, err = igdbcsv.ReadAll(input, "")
	} else {
		games, err = igdbcsv.ReadAll("", input)
		if cfg.Status != "" && len(games) > 0 {
			status, err := mapping.FromListName(cfg.Status)
			if err == nil {
				for i := range games {
					games[i].Status = status
				}
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("parsing IGDB CSV: %w", err)
	}
	slog.Info("IGDB CSV parsed", "count", len(games))

	res := &Result{
		GamesRead:  len(games),
		OutputFile: cfg.OutputFile,
	}

	if cfg.ShouldEnrich() {
		slog.Info("enriching games via IGDB API")
		client := igdb.NewClient(
			cfg.IGDBClientID,
			cfg.IGDBClientSecret,
			cfg.IGDBOAuthURL,
			cfg.IGDBAPIBase,
			nil,
		)
		warnings, err := client.EnrichBatch(ctx, games)
		if err != nil {
			return nil, fmt.Errorf("IGDB API enrichment: %w", err)
		}
		for _, w := range warnings {
			slog.Warn("enrichment warning", "detail", w)
		}
		res.GamesEnriched = len(games)
		res.Warnings = warnings
		slog.Info("enrichment complete", "enriched", res.GamesEnriched, "warnings", len(warnings))
	}

	slog.Info("writing Yamtrack CSV", "path", cfg.OutputFile)
	if err := yamtrack.WriteCSV(cfg.OutputFile, games); err != nil {
		return nil, fmt.Errorf("writing Yamtrack CSV: %w", err)
	}

	return res, nil
}

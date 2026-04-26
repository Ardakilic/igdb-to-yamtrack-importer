package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type EnrichMode string

const (
	EnrichAuto   EnrichMode = "auto"
	EnrichAlways EnrichMode = "always"
	EnrichNever  EnrichMode = "never"
)

type Config struct {
	Input            string
	InputIsDir       bool
	Status           string
	OutputFile       string
	IGDBClientID     string
	IGDBClientSecret string
	IGDBAPIBase      string
	IGDBOAuthURL     string
	EnrichMode       EnrichMode
	LogLevel         string
}

type flags struct {
	status           string
	output           string
	enrichMode       string
	igdbClientID     string
	igdbClientSecret string
	igdbAPIBase      string
	igdbOAuthURL     string
	logLevel         string
}

func ParseFlags(args []string) (*Config, error) {
	fl := &flags{}
	fs := flag.NewFlagSet("igdb2yamtrack", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	fs.StringVar(&fl.status, "status", "", "Status for single-list CSV (played, playing, want_to_play, etc.)")
	fs.StringVar(&fl.output, "output", "./yamtrack-import.csv", "Output file path")
	fs.StringVar(&fl.enrichMode, "enrich-mode", "auto", "API enrichment: auto, always, or never")
	fs.StringVar(&fl.igdbClientID, "igdb-client-id", "", "Twitch/IGDB OAuth client ID")
	fs.StringVar(&fl.igdbClientSecret, "igdb-client-secret", "", "Twitch/IGDB OAuth client secret")
	fs.StringVar(&fl.igdbAPIBase, "igdb-api-base", "https://api.igdb.com/v4", "IGDB REST API base URL")
	fs.StringVar(&fl.igdbOAuthURL, "igdb-oauth-url", "https://id.twitch.tv/oauth2/token", "Twitch OAuth2 token endpoint")
	fs.StringVar(&fl.logLevel, "log-level", "info", "Log level: debug, info, warn, error")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Try --help for usage.\n")
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if fs.NArg() < 1 {
		return nil, fmt.Errorf("missing required input argument (CSV file or directory)")
	}

	input := fs.Arg(0)
	inputIsDir, err := isDirectory(input)
	if err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	mode := strings.ToLower(fl.enrichMode)
	if mode != "auto" && mode != "always" && mode != "never" {
		return nil, fmt.Errorf("--enrich-mode must be auto, always, or never; got %q", fl.enrichMode)
	}

	cfg := &Config{
		Input:            input,
		InputIsDir:       inputIsDir,
		Status:           fl.status,
		OutputFile:       fl.output,
		IGDBClientID:     fl.igdbClientID,
		IGDBClientSecret: fl.igdbClientSecret,
		IGDBAPIBase:      fl.igdbAPIBase,
		IGDBOAuthURL:     fl.igdbOAuthURL,
		LogLevel:         fl.logLevel,
		EnrichMode:       EnrichMode(mode),
	}

	if cfg.EnrichMode == EnrichAlways && (cfg.IGDBClientID == "" || cfg.IGDBClientSecret == "") {
		return nil, fmt.Errorf("--igdb-client-id and --igdb-client-secret are required when --enrich-mode=always")
	}

	return cfg, nil
}

func isDirectory(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("input path does not exist: %s", path)
		}
		return false, err
	}
	return fi.IsDir(), nil
}

func (c *Config) ShouldEnrich() bool {
	switch c.EnrichMode {
	case EnrichAlways:
		return true
	case EnrichNever:
		return false
	default:
		return c.IGDBClientID != "" && c.IGDBClientSecret != ""
	}
}

func (c *Config) Validate() error {
	if c.OutputFile == "" {
		return fmt.Errorf("--output is required")
	}
	dir := filepath.Dir(c.OutputFile)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create output directory: %w", err)
		}
	}
	return nil
}

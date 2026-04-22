package config

import (
	"path/filepath"
	"testing"
)

func TestParseFlags_MissingInput(t *testing.T) {
	_, err := ParseFlags([]string{})
	if err == nil {
		t.Fatal("expected error for missing input")
	}
}

func TestParseFlags_FileMode(t *testing.T) {
	cfg, err := ParseFlags([]string{"../../testdata/igdb/combined.csv"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Input != "../../testdata/igdb/combined.csv" {
		t.Errorf("expected Input=../../testdata/igdb/combined.csv, got %q", cfg.Input)
	}
	if cfg.InputIsDir {
		t.Error("expected InputIsDir=false for file")
	}
}

func TestParseFlags_DirMode(t *testing.T) {
	cfg, err := ParseFlags([]string{"../../testdata/igdb"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.InputIsDir {
		t.Error("expected InputIsDir=true for directory")
	}
}

func TestShouldEnrich(t *testing.T) {
	tests := []struct {
		mode   EnrichMode
		id     string
		secret string
		want   bool
	}{
		{EnrichAuto, "", "", false},
		{EnrichAuto, "id", "sec", true},
		{EnrichAuto, "id", "", false},
		{EnrichAlways, "", "", true},
		{EnrichAlways, "id", "sec", true},
		{EnrichNever, "id", "sec", false},
		{EnrichNever, "", "", false},
	}
	for _, tt := range tests {
		cfg := &Config{
			EnrichMode:       tt.mode,
			IGDBClientID:     tt.id,
			IGDBClientSecret: tt.secret,
		}
		if got := cfg.ShouldEnrich(); got != tt.want {
			t.Errorf("ShouldEnrich(%q, id=%q, sec=%q) = %v, want %v",
				tt.mode, tt.id, tt.secret, got, tt.want)
		}
	}
}

func TestConfig_Validate(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "out.csv")

	cfg := &Config{
		OutputFile: outFile,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}

	emptyCfg := &Config{
		OutputFile: "",
	}
	if err := emptyCfg.Validate(); err == nil {
		t.Error("expected error for empty output")
	}
}

func TestConfig_ValidateCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "subdir", "out.csv")

	cfg := &Config{
		OutputFile: outFile,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

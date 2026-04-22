package main

import (
	"testing"
)

func TestSetupLogger(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error", "unknown"} {
		setupLogger(level)
	}
}

func TestSetupLoggerLevels(t *testing.T) {
	tests := []string{"debug", "info", "warn", "error"}
	for _, tt := range tests {
		setupLogger(tt)
	}
	setupLogger("unknown")
}

func TestLogLevels(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping log test in short mode")
	}
}

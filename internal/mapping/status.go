// Package mapping converts IGDB list names to Yamtrack tracking statuses.
package mapping

import (
	"fmt"
	"regexp"
	"strings"
)

// Status is a Yamtrack-compatible media tracking status string.
type Status string

const (
	StatusCompleted  Status = "Completed"
	StatusInProgress Status = "In progress"
	StatusPlanning   Status = "Planning"
	StatusPaused     Status = "Paused"
	StatusDropped    Status = "Dropped"
)

// nonAlnum strips everything that is not a lowercase ASCII letter or digit.
var nonAlnum = regexp.MustCompile(`[^a-z0-9]`)

// normalize converts arbitrary list name strings to a canonical lookup key.
func normalize(s string) string {
	return nonAlnum.ReplaceAllString(strings.ToLower(s), "")
}

// listToStatus maps normalised IGDB list name tokens to Yamtrack statuses.
var listToStatus = map[string]Status{
	// Completed variants
	"played":    StatusCompleted,
	"completed": StatusCompleted,
	"finished":  StatusCompleted,
	// In progress variants
	"playing":          StatusInProgress,
	"inprogress":       StatusInProgress,
	"currentlyplaying": StatusInProgress,
	// Planning variants
	"wanttoplay": StatusPlanning,
	"wishlist":   StatusPlanning,
	"planning":   StatusPlanning,
	"planned":    StatusPlanning,
	// Paused variants
	"paused": StatusPaused,
	"onhold": StatusPaused,
	// Dropped variants
	"dropped":   StatusDropped,
	"abandoned": StatusDropped,
}

// FromListName maps an IGDB list name (e.g. a CSV filename without extension, or the
// value of a 'list' column) to a Yamtrack Status. Returns an error for unknown tokens.
func FromListName(name string) (Status, error) {
	key := normalize(name)
	if s, ok := listToStatus[key]; ok {
		return s, nil
	}
	return "", fmt.Errorf("unknown IGDB list name %q (normalized: %q)", name, key)
}

// Package yamtrack writes game records in the Yamtrack CSV import format.
//
// Yamtrack CSV columns:
//
//	media_id, source, media_type, title, image, season_number, episode_number,
//	score, status, notes, start_date, end_date, progress
//
// For games, source is always "igdb" and media_type is always "game".
// Yamtrack auto-fetches title and image from IGDB when media_id is supplied, so
// those fields are left blank when not available.
package yamtrack

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/ardakilicdagi/igdb-yamtrack-importer/internal/igdbcsv"
)

// header is the fixed column order required by Yamtrack's CSV importer.
var header = []string{
	"media_id", "source", "media_type", "title", "image",
	"season_number", "episode_number", "score", "status",
	"notes", "start_date", "end_date", "progress",
}

// WriteCSV writes games to the file at path in Yamtrack's CSV import format.
// The file is created or truncated on each call.
func WriteCSV(path string, games []igdbcsv.Game) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating output file %q: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return fmt.Errorf("writing CSV header: %w", err)
	}

	for i := range games {
		row := gameToRow(&games[i])
		if err := w.Write(row); err != nil {
			return fmt.Errorf("writing CSV row for IGDB ID %d: %w", games[i].IGDBID, err)
		}
	}

	w.Flush()
	return w.Error()
}

// gameToRow converts a Game to a Yamtrack CSV row matching the header order.
func gameToRow(g *igdbcsv.Game) []string {
	score := ""
	if g.UserRating != nil {
		// Convert from IGDB 0–100 scale to Yamtrack 0–10 scale.
		converted := *g.UserRating / 10.0
		score = strconv.FormatFloat(converted, 'f', 1, 64)
	}

	return []string{
		strconv.Itoa(g.IGDBID), // media_id
		"igdb",                 // source
		"game",                 // media_type
		g.Title,                // title (may be blank; Yamtrack auto-fetches)
		"",                     // image (always blank; Yamtrack auto-fetches)
		"",                     // season_number (not applicable for games)
		"",                     // episode_number (not applicable for games)
		score,                  // score
		string(g.Status),       // status
		"",                     // notes
		"",                     // start_date
		"",                     // end_date
		"",                     // progress
	}
}

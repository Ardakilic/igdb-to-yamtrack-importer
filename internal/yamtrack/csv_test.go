package yamtrack

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardakilic/igdb-yamtrack-importer/internal/igdbcsv"
	"github.com/ardakilic/igdb-yamtrack-importer/internal/mapping"
)

func rating(v float64) *float64 { return &v }

func TestWriteCSV_HappyPath(t *testing.T) {
	games := []igdbcsv.Game{
		{IGDBID: 1942, Title: "1942", Status: mapping.StatusCompleted, UserRating: rating(80)},
		{IGDBID: 76882, Title: "The Witcher 3", Status: mapping.StatusInProgress, UserRating: nil},
		{IGDBID: 2155, Title: "Cyberpunk 2077", Status: mapping.StatusPlanning},
	}

	path := filepath.Join(t.TempDir(), "out.csv")
	if err := WriteCSV(path, games); err != nil {
		t.Fatalf("WriteCSV failed: %v", err)
	}

	rows := readCSV(t, path)

	if len(rows) != 4 { // 1 header + 3 data
		t.Fatalf("expected 4 rows (header + 3 games), got %d", len(rows))
	}

	// Verify header.
	if strings.Join(rows[0], ",") != strings.Join(header, ",") {
		t.Errorf("wrong header: %v", rows[0])
	}

	// Row 1: 1942 with rating 80 → score 8.0.
	assertRow(t, rows[1], "1942", "igdb", "game", "1942", "8.0", "Completed")

	// Row 2: Witcher 3 with no rating → blank score.
	assertRow(t, rows[2], "76882", "igdb", "game", "The Witcher 3", "", "In progress")

	// Row 3: Cyberpunk with no title.
	assertRow(t, rows[3], "2155", "igdb", "game", "Cyberpunk 2077", "", "Planning")
}

func TestWriteCSV_ScoreConversion(t *testing.T) {
	tests := []struct {
		igdbRating float64
		wantScore  string
	}{
		{100, "10.0"},
		{0, "0.0"},
		{50, "5.0"},
		{75, "7.5"},
		{95, "9.5"},
	}

	for _, tt := range tests {
		games := []igdbcsv.Game{
			{IGDBID: 1, Title: "G", Status: mapping.StatusCompleted, UserRating: rating(tt.igdbRating)},
		}
		path := filepath.Join(t.TempDir(), "out.csv")
		if err := WriteCSV(path, games); err != nil {
			t.Fatalf("WriteCSV: %v", err)
		}
		rows := readCSV(t, path)
		if len(rows) < 2 {
			t.Fatal("no data row")
		}
		scoreIdx := colIndex("score")
		if rows[1][scoreIdx] != tt.wantScore {
			t.Errorf("IGDB rating %.0f → score %q, want %q", tt.igdbRating, rows[1][scoreIdx], tt.wantScore)
		}
	}
}

func TestWriteCSV_EmptyGames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	if err := WriteCSV(path, nil); err != nil {
		t.Fatalf("WriteCSV with no games: %v", err)
	}
	rows := readCSV(t, path)
	if len(rows) != 1 {
		t.Fatalf("expected only header row, got %d rows", len(rows))
	}
}

func TestWriteCSV_AllStatuses(t *testing.T) {
	statuses := []mapping.Status{
		mapping.StatusCompleted,
		mapping.StatusInProgress,
		mapping.StatusPlanning,
		mapping.StatusPaused,
		mapping.StatusDropped,
	}
	games := make([]igdbcsv.Game, len(statuses))
	for i, s := range statuses {
		games[i] = igdbcsv.Game{IGDBID: i + 1, Title: "G", Status: s}
	}
	path := filepath.Join(t.TempDir(), "out.csv")
	if err := WriteCSV(path, games); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	rows := readCSV(t, path)
	statusIdx := colIndex("status")
	for i, s := range statuses {
		if rows[i+1][statusIdx] != string(s) {
			t.Errorf("row %d: status %q, want %q", i+1, rows[i+1][statusIdx], s)
		}
	}
}

func TestWriteCSV_MediaIDAndSourceType(t *testing.T) {
	games := []igdbcsv.Game{{IGDBID: 99, Title: "Test", Status: mapping.StatusCompleted}}
	path := filepath.Join(t.TempDir(), "out.csv")
	if err := WriteCSV(path, games); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	rows := readCSV(t, path)
	row := rows[1]
	if row[colIndex("media_id")] != "99" {
		t.Errorf("wrong media_id: %q", row[colIndex("media_id")])
	}
	if row[colIndex("source")] != "igdb" {
		t.Errorf("wrong source: %q", row[colIndex("source")])
	}
	if row[colIndex("media_type")] != "game" {
		t.Errorf("wrong media_type: %q", row[colIndex("media_type")])
	}
}

func TestWriteCSV_InvalidPath(t *testing.T) {
	err := WriteCSV("/nonexistent/dir/out.csv", nil)
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestWriteCSV_ZeroRating(t *testing.T) {
	games := []igdbcsv.Game{{IGDBID: 1, Title: "G", Status: mapping.StatusCompleted, UserRating: rating(0)}}
	path := filepath.Join(t.TempDir(), "out.csv")
	if err := WriteCSV(path, games); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	rows := readCSV(t, path)
	if len(rows) < 2 {
		t.Fatal("no data row")
	}
	if rows[1][colIndex("score")] != "0.0" {
		t.Errorf("zero rating: got %q, want 0.0", rows[1][colIndex("score")])
	}
}

func TestWriteCSV_ColumnCount(t *testing.T) {
	games := []igdbcsv.Game{{IGDBID: 42, Title: "Test", Status: mapping.StatusPaused}}
	path := filepath.Join(t.TempDir(), "out.csv")
	if err := WriteCSV(path, games); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	rows := readCSV(t, path)
	if len(rows[0]) != len(header) {
		t.Errorf("header has %d cols, want %d", len(rows[0]), len(header))
	}
	if len(rows[1]) != len(header) {
		t.Errorf("data row has %d cols, want %d", len(rows[1]), len(header))
	}
}

// readCSV is a test helper that reads a CSV file and returns all rows.
func readCSV(t *testing.T, path string) [][]string {
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

// colIndex returns the position of col in the Yamtrack header.
func colIndex(col string) int {
	for i, h := range header {
		if h == col {
			return i
		}
	}
	panic("unknown column: " + col)
}

// assertRow checks selected columns of a data row.
func assertRow(t *testing.T, row []string, mediaID, source, mediaType, title, wantScore, status string) {
	t.Helper()
	if row[colIndex("media_id")] != mediaID {
		t.Errorf("media_id: got %q, want %q", row[colIndex("media_id")], mediaID)
	}
	if row[colIndex("source")] != source {
		t.Errorf("source: got %q, want %q", row[colIndex("source")], source)
	}
	if row[colIndex("media_type")] != mediaType {
		t.Errorf("media_type: got %q, want %q", row[colIndex("media_type")], mediaType)
	}
	if row[colIndex("title")] != title {
		t.Errorf("title: got %q, want %q", row[colIndex("title")], title)
	}
	if row[colIndex("score")] != wantScore {
		t.Errorf("score: got %q, want %q", row[colIndex("score")], wantScore)
	}
	if row[colIndex("status")] != status {
		t.Errorf("status: got %q, want %q", row[colIndex("status")], status)
	}
}

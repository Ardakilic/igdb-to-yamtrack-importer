// Package igdbcsv parses IGDB list CSV exports into a normalised Game slice.
//
// IGDB exports CSVs with the following columns:
//
//	id, game, url, rating, category, release_date, platforms, genres, themes, companies, description
//
// Two input modes are supported:
//
//   - Directory mode (IGDB_CSV_DIR): each .csv file in the directory is read; the
//     filename without extension maps to a Yamtrack status via mapping.FromListName.
//
//   - Single-file mode (IGDB_CSV_FILE): a single CSV file whose rows carry a "list"
//     column (or "status" column) indicating which IGDB list each row belongs to.
package igdbcsv

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ardakilicdagi/igdb-yamtrack-importer/internal/mapping"
)

// Game represents one IGDB game entry after parsing.
type Game struct {
	// IGDBID is the unique IGDB integer identifier.
	IGDBID int
	// Title is the game name from the CSV.
	Title string
	// UserRating is the user's personal rating on a 0–100 IGDB scale.
	// Nil means the field was absent or blank.
	UserRating *float64
	// Status is the mapped Yamtrack tracking status.
	Status mapping.Status
	// ReleaseDate is the raw release_date string from IGDB (may be empty).
	ReleaseDate string
	// Platforms is the raw platforms string from IGDB (may be empty).
	Platforms string
}

// ReadAll loads games from either a directory of per-list CSVs or a single
// consolidated CSV, according to which of csvDir / csvFile is non-empty.
func ReadAll(csvDir, csvFile string) ([]Game, error) {
	if csvDir != "" {
		return readDir(csvDir)
	}
	return readFile(csvFile, "")
}

// readDir reads every .csv file in dir. The filename without extension is used
// to derive the Yamtrack status via mapping.FromListName.
func readDir(dir string) ([]Game, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading IGDB CSV directory %q: %w", dir, err)
	}

	var all []Game
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.EqualFold(filepath.Ext(name), ".csv") {
			continue
		}
		listName := strings.TrimSuffix(name, filepath.Ext(name))
		path := filepath.Join(dir, name)
		games, err := readFile(path, listName)
		if err != nil {
			return nil, err
		}
		all = append(all, games...)
	}
	return all, nil
}

// readFile parses a single IGDB CSV file. When listNameOverride is non-empty it is
// used as the status source for every row; otherwise each row must carry a "list"
// or "status" column.
func readFile(path, listNameOverride string) ([]Game, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %q: %w", path, err)
	}
	defer f.Close()
	return parseReader(f, listNameOverride)
}

// ParseReader is exported for tests that supply an in-memory io.Reader.
func ParseReader(r io.Reader, listNameOverride string) ([]Game, error) {
	return parseReader(r, listNameOverride)
}

func parseReader(r io.Reader, listNameOverride string) ([]Game, error) {
	cr := csv.NewReader(stripBOM(r))
	cr.TrimLeadingSpace = true
	cr.LazyQuotes = true

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("reading CSV header: %w", err)
	}

	idx := buildIndex(header)

	var games []Game
	lineNum := 1
	for {
		lineNum++
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading CSV line %d: %w", lineNum, err)
		}

		g, skip, parseErr := parseRecord(record, idx, listNameOverride, lineNum)
		if parseErr != nil {
			return nil, parseErr
		}
		if skip {
			continue
		}
		games = append(games, g)
	}
	return games, nil
}

// columnIndex maps canonical lowercase column names to their CSV positions.
type columnIndex struct {
	id          int
	game        int
	rating      int
	releaseDate int
	platforms   int
	list        int // single-file mode: "list" or "status" column
}

const absent = -1

func buildIndex(header []string) columnIndex {
	idx := columnIndex{
		id: absent, game: absent, rating: absent,
		releaseDate: absent, platforms: absent, list: absent,
	}
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "id", "game_id":
			idx.id = i
		case "game", "name", "title":
			idx.game = i
		case "rating", "user_rating", "score":
			idx.rating = i
		case "release_date", "release_dates", "releasedate":
			idx.releaseDate = i
		case "platforms", "platform":
			idx.platforms = i
		case "list", "status":
			idx.list = i
		}
	}
	return idx
}

func parseRecord(record []string, idx columnIndex, listNameOverride string, line int) (Game, bool, error) {
	get := func(i int) string {
		if i == absent || i >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[i])
	}

	// Resolve status.
	listName := listNameOverride
	if listName == "" {
		listName = get(idx.list)
	}
	if listName == "" {
		// Row cannot be mapped; silently skip (single-file mode without list column).
		return Game{}, true, nil
	}
	status, err := mapping.FromListName(listName)
	if err != nil {
		// Unknown status: skip with no error so the tool keeps running.
		return Game{}, true, nil
	}

	// Parse IGDB ID.
	rawID := get(idx.id)
	if rawID == "" {
		return Game{}, true, nil
	}
	id, err := strconv.Atoi(rawID)
	if err != nil || id <= 0 {
		return Game{}, true, nil
	}

	g := Game{
		IGDBID:      id,
		Title:       get(idx.game),
		Status:      status,
		ReleaseDate: get(idx.releaseDate),
		Platforms:   get(idx.platforms),
	}

	// Parse optional user rating (IGDB scale 0–100).
	if rawRating := get(idx.rating); rawRating != "" {
		r, err := strconv.ParseFloat(rawRating, 64)
		if err == nil && r >= 0 && r <= 100 {
			g.UserRating = &r
		}
	}

	return g, false, nil
}

// stripBOM returns r with a leading UTF-8 BOM removed if present.
func stripBOM(r io.Reader) io.Reader {
	buf := make([]byte, 3)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		// Unreadable; return original reader (will fail at CSV parse).
		return r
	}
	buf = buf[:n]
	if len(buf) == 3 && buf[0] == 0xEF && buf[1] == 0xBB && buf[2] == 0xBF {
		return io.MultiReader(strings.NewReader(""), r)
	}
	return io.MultiReader(strings.NewReader(string(buf)), r)
}

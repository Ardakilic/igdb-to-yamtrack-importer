package igdbcsv

import (
	"strings"
	"testing"

	"github.com/ardakilicdagi/igdb-yamtrack-importer/internal/mapping"
)

const samplePlayed = `id,game,url,rating,category,release_date,platforms,genres,themes,companies,description
1942,1942,https://www.igdb.com/games/1942,80,0,1982,Arcade,Shooter,,Capcom,Vertical scrolling
76882,The Witcher 3: Wild Hunt,https://www.igdb.com/games/tw3,95,0,2015-05-19,PC,RPG,Fantasy,CD Projekt Red,Open world RPG
`

const sampleCombined = `id,game,url,rating,category,release_date,platforms,genres,themes,companies,description,list
1942,1942,https://www.igdb.com/games/1942,80,0,1982,Arcade,Shooter,,Capcom,shooter,played
119388,Elden Ring,https://www.igdb.com/games/elden-ring,90,0,2022-02-25,PC,RPG,Fantasy,FromSoftware,rpg,playing
2155,Cyberpunk 2077,https://www.igdb.com/games/cyberpunk-2077,,0,2020-12-10,PC,RPG,SF,CDPR,rpg,want_to_play
`

func TestParseReader_DirectMode(t *testing.T) {
	games, err := ParseReader(strings.NewReader(samplePlayed), "played")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("expected 2 games, got %d", len(games))
	}

	g := games[0]
	if g.IGDBID != 1942 {
		t.Errorf("expected IGDBID=1942, got %d", g.IGDBID)
	}
	if g.Status != mapping.StatusCompleted {
		t.Errorf("expected Completed, got %q", g.Status)
	}
	if g.UserRating == nil || *g.UserRating != 80 {
		t.Errorf("expected rating=80, got %v", g.UserRating)
	}
	if g.Title != "1942" {
		t.Errorf("expected title=1942, got %q", g.Title)
	}

	g2 := games[1]
	if g2.IGDBID != 76882 {
		t.Errorf("expected IGDBID=76882, got %d", g2.IGDBID)
	}
	if g2.UserRating == nil || *g2.UserRating != 95 {
		t.Errorf("expected rating=95, got %v", g2.UserRating)
	}
}

func TestParseReader_SingleFileMode(t *testing.T) {
	games, err := ParseReader(strings.NewReader(sampleCombined), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("expected 3 games, got %d", len(games))
	}
	if games[0].Status != mapping.StatusCompleted {
		t.Errorf("row 0: expected Completed, got %q", games[0].Status)
	}
	if games[1].Status != mapping.StatusInProgress {
		t.Errorf("row 1: expected In progress, got %q", games[1].Status)
	}
	if games[2].Status != mapping.StatusPlanning {
		t.Errorf("row 2: expected Planning, got %q", games[2].Status)
	}
}

func TestParseReader_MissingRating(t *testing.T) {
	const csv = `id,game,url,rating,category,release_date,platforms,genres,themes,companies,description
1020,Baldur's Gate,https://igdb.com,,0,1998,PC,RPG,,BioWare,Classic
`
	games, err := ParseReader(strings.NewReader(csv), "played")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(games))
	}
	if games[0].UserRating != nil {
		t.Errorf("expected nil rating, got %v", games[0].UserRating)
	}
}

func TestParseReader_SkipsBlankID(t *testing.T) {
	const csv = `id,game,url,rating,category,release_date,platforms,genres,themes,companies,description
,Missing Game,https://igdb.com,50,0,2000,PC,,,Company,desc
1942,Valid Game,https://igdb.com,80,0,1982,Arcade,,,Capcom,desc
`
	games, err := ParseReader(strings.NewReader(csv), "played")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 game (blank ID skipped), got %d", len(games))
	}
}

func TestParseReader_SkipsNegativeID(t *testing.T) {
	const csv = `id,game,url,rating,category,release_date,platforms,genres,themes,companies,description
-5,Bad Game,https://igdb.com,50,0,2000,PC,,,Company,desc
`
	games, err := ParseReader(strings.NewReader(csv), "played")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("expected 0 games, got %d", len(games))
	}
}

func TestParseReader_SkipsUnknownList(t *testing.T) {
	const csv = `id,game,url,rating,category,release_date,platforms,genres,themes,companies,description,list
1942,1942,https://igdb.com,80,0,1982,Arcade,,,Capcom,desc,backlog
`
	games, err := ParseReader(strings.NewReader(csv), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("expected 0 games (unknown list skipped), got %d", len(games))
	}
}

func TestParseReader_SkipsMissingListColumn(t *testing.T) {
	// No list override, no list column → all rows skipped.
	const csv = `id,game,url,rating
1942,1942,https://igdb.com,80
`
	games, err := ParseReader(strings.NewReader(csv), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("expected 0 games, got %d", len(games))
	}
}

func TestParseReader_BOMPrefix(t *testing.T) {
	bom := "\xEF\xBB\xBF"
	csv := bom + "id,game,url,rating,category,release_date,platforms,genres,themes,companies,description\n" +
		"1942,1942,https://igdb.com,80,0,1982,Arcade,,,Capcom,desc\n"
	games, err := ParseReader(strings.NewReader(csv), "played")
	if err != nil {
		t.Fatalf("unexpected error with BOM: %v", err)
	}
	if len(games) != 1 || games[0].IGDBID != 1942 {
		t.Errorf("BOM handling failed: got %v", games)
	}
}

func TestParseReader_CRLFLineEndings(t *testing.T) {
	csv := "id,game,url,rating,category,release_date,platforms,genres,themes,companies,description\r\n" +
		"1942,1942,https://igdb.com,80,0,1982,Arcade,,,Capcom,desc\r\n"
	games, err := ParseReader(strings.NewReader(csv), "played")
	if err != nil {
		t.Fatalf("unexpected error with CRLF: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(games))
	}
}

func TestParseReader_EmptyFile(t *testing.T) {
	const csv = `id,game,url,rating,category,release_date,platforms,genres,themes,companies,description
`
	games, err := ParseReader(strings.NewReader(csv), "played")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("expected 0 games, got %d", len(games))
	}
}

func TestParseReader_AlternativeColumnNames(t *testing.T) {
	const csv = `game_id,name,url,user_rating,category,release_dates,platform,genres,themes,companies,description
42,Test Game,https://igdb.com,75.5,0,2010,PC,,,Dev,desc
`
	games, err := ParseReader(strings.NewReader(csv), "completed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(games))
	}
	g := games[0]
	if g.IGDBID != 42 {
		t.Errorf("wrong ID: %d", g.IGDBID)
	}
	if g.UserRating == nil || *g.UserRating != 75.5 {
		t.Errorf("wrong rating: %v", g.UserRating)
	}
}

func TestReadAll_DirMode(t *testing.T) {
	games, err := ReadAll("../../testdata/igdb", "")
	if err != nil {
		t.Fatalf("ReadAll dir mode failed: %v", err)
	}
	// played.csv has 3 rows, playing.csv has 1, want_to_play.csv has 1, combined.csv has 3 (with list col)
	if len(games) == 0 {
		t.Fatal("expected games from testdata directory, got none")
	}
	// Verify combined.csv rows are also included (has list column)
	statuses := map[mapping.Status]int{}
	for _, g := range games {
		statuses[g.Status]++
	}
	if statuses[mapping.StatusCompleted] == 0 {
		t.Error("expected at least one Completed game from testdata")
	}
	if statuses[mapping.StatusInProgress] == 0 {
		t.Error("expected at least one In progress game from testdata")
	}
}

func TestReadAll_FileMode(t *testing.T) {
	games, err := ReadAll("", "../../testdata/igdb/combined.csv")
	if err != nil {
		t.Fatalf("ReadAll file mode failed: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("expected 3 games, got %d", len(games))
	}
}

func TestReadAll_MissingDir(t *testing.T) {
	_, err := ReadAll("/nonexistent/dir", "")
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestReadAll_MissingFile(t *testing.T) {
	_, err := ReadAll("", "/nonexistent/file.csv")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

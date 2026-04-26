# igdb-yamtrack-importer

A CLI tool that converts your [IGDB](https://www.igdb.com) game list CSV exports into the [Yamtrack](https://github.com/FuzzyGrim/Yamtrack) CSV import format, so you can migrate your play history to a self-hosted Yamtrack instance.

## Why this exists

Yamtrack does not natively import IGDB data yet — see [FuzzyGrim/Yamtrack#1272](https://github.com/FuzzyGrim/Yamtrack/issues/1272). A pull request upstream would have taken longer to review and merge than building this standalone tool, so here we are.

## How it works

```
IGDB CSV export(s)
       │
       ▼
  [igdb2yamtrack]
       │  ├── Parses per-list CSVs (played.csv, playing.csv, …)
       │  └── (Optional) enriches titles via IGDB API
       │
       ▼
 yamtrack-import.csv
       │
       ▼
  Yamtrack web UI
  Settings → Import → "Yamtrack CSV"
```

The IGDB public API contains no user-specific data (your lists live only in the CSV export), so the CSV is the required input. The optional IGDB API step validates game IDs and updates titles to their canonical API names.

## Installing

### go install

```bash
go install github.com/ardakilic/igdb-yamtrack-importer/cmd/igdb2yamtrack@latest
```

### Build from source

```bash
git clone https://github.com/ardakilic/igdb-yamtrack-importer
cd igdb-to-yamtrack-importer
make build
./bin/igdb2yamtrack
```

### Docker

```bash
docker run --rm \
  -v /path/to/igdb-exports:/data:ro \
  -v /path/to/output:/output \
  igdb2yamtrack /data --output /output/yamtrack-import.csv
```

## Exporting your IGDB data

1. Log in at [igdb.com](https://www.igdb.com).
2. Open your profile and go to **Lists**.
3. For each list (Played, Playing, Want to Play, etc.) click the list name, then use the **Export** or **Download CSV** button.
4. Save the files in a single directory, e.g. `~/igdb-exports/`. The filename determines the Yamtrack status (see [Status mapping](#status-mapping)).

## Usage

### Command-line arguments

```
igdb2yamtrack <input> [flags]
```

**Arguments:**
- `<input>` — Required. Path to a CSV file or a directory containing CSV files.

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `--output` | `./yamtrack-import.csv` | Output file path |
| `--status` | (derived from filename) | Status for single-list CSV (played, playing, want_to_play, etc.) |
| `--enrich-mode` | `auto` | API enrichment: `auto`, `always`, or `never` |
| `--igdb-client-id` | — | Twitch/IGDB OAuth client ID |
| `--igdb-client-secret` | — | Twitch/IGDB OAuth client secret |
| `--igdb-api-base` | `https://api.igdb.com/v4` | IGDB REST API base URL |
| `--igdb-oauth-url` | `https://id.twitch.tv/oauth2/token` | Twitch OAuth2 token endpoint |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |

### Examples

```bash
# Directory of per-list CSVs, no API enrichment:
igdb2yamtrack ~/igdb-exports --output ~/yamtrack-import.csv

# Single CSV file with status override:
igdb2yamtrack played.csv --status played --output ~/yamtrack-import.csv

# Single CSV with list column:
igdb2yamtrack all.csv --output ~/yamtrack-import.csv

# With API enrichment:
igdb2yamtrack ~/igdb-exports \
  --igdb-client-id your_client_id \
  --igdb-client-secret your_client_secret \
  --output ~/yamtrack-import.csv
```

### Docker

Build the image first:

```bash
docker build -t igdb2yamtrack .
```

Then run with your data:

```bash
# Directory of per-list CSVs with API enrichment:
docker run --rm \
  -v ~/igdb-exports:/data:ro \
  -v ~/output:/output \
  igdb2yamtrack /data --output /output/yamtrack-import.csv \
  --igdb-client-id your_id \
  --igdb-client-secret your_secret

# Single CSV with status and API enrichment:
docker run --rm \
  -v ~/igdb-exports:/data:ro \
  -v ~/output:/output \
  igdb2yamtrack /data/played.csv \
  --status played \
  --output /output/yamtrack-import.csv \
  --igdb-client-id your_id \
  --igdb-client-secret your_secret \
  --enrich-mode always
```

## Status mapping

The Yamtrack status is derived from the IGDB list name (the CSV filename without extension in directory mode, or the `--status` flag value in single-file mode, or the `list` column value in single-file mode). Matching is case-insensitive and ignores spaces, hyphens, and underscores.

| IGDB list name | Yamtrack status |
|---|---|
| `played`, `completed`, `finished` | Completed |
| `playing`, `in_progress`, `in-progress`, `currently playing` | In progress |
| `want_to_play`, `want-to-play`, `wishlist`, `planning`, `planned` | Planning |
| `paused`, `on_hold`, `on-hold` | Paused |
| `dropped`, `abandoned` | Dropped |

Rows with unrecognised list names are skipped with a warning log.

## Importing into Yamtrack

1. Open your Yamtrack instance.
2. Go to **Settings → Import**.
3. Select **Yamtrack CSV**.
4. Upload the generated `yamtrack-import.csv`.
5. Choose mode:
   - **new** — only add items that do not already exist.
   - **overwrite** — replace existing items with data from the CSV.
6. Click **Import**.

Yamtrack will automatically fetch game titles, cover images, and other metadata from IGDB using the `media_id` in the CSV.

## GitHub Actions CI

The project includes a GitHub Actions workflow (`.github/workflows/ci.yml`) that runs on every pull request and push to `main`. It includes:

- **Lint**: `go vet` and `gofmt` checks
- **Test**: Tests with race detector and coverage enforcement (≥90%)
- **Build**: Cross-compilation for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64

## Development

### Prerequisites

- Go 1.23+
- Docker (for `make build-docker` / `make test-docker`)

### Project layout

```
cmd/igdb2yamtrack/     CLI entrypoint
internal/
  config/              CLI flag parsing and validation
  igdbcsv/            IGDB CSV parser (per-list dir or single file)
  igdb/               IGDB REST API client (OAuth, rate limiting, enrichment)
  mapping/            IGDB list name → Yamtrack status conversion
  yamtrack/           Yamtrack CSV writer
  importer/           Pipeline orchestrator
testdata/igdb/          CSV fixtures for tests
.github/              GitHub Actions and sample data for CI
```

### Running tests

```bash
make test
```

### Coverage

```bash
make test-coverage
```

### Linting

```bash
make lint
```

Checks `go vet` and `gofmt`. No external linter dependencies.

### Cross-compilation

```bash
make build-cross
# Produces binaries in bin/ for: linux/amd64, linux/arm64,
#   darwin/amd64, darwin/arm64, windows/amd64, windows/arm64
```

## Docker

The multi-stage `Dockerfile` uses two Debian-family base images:

| Stage | Image | Purpose |
|---|---|---|
| `build` | `golang:bookworm` (Debian 12 Bookworm) | Compile the binary with the official Go toolchain on Debian. |
| `test` | `golang:bookworm` | Run `go test` in an isolated container (`make test-docker`). |
| `runtime` | `gcr.io/distroless/base-debian12:nonroot` | Final image with no shell, no package manager, minimal CVE surface. |

The binary is compiled with `CGO_ENABLED=0` so it runs without glibc and works in the distroless image.

## Make targets reference

| Target | Description |
|---|---|
| `make lint` | Run `go vet` and check `gofmt` formatting. |
| `make build` | Compile `bin/igdb2yamtrack` for the current platform. |
| `make build-cross` | Cross-compile for all six supported platforms into `bin/`. |
| `make build-docker` | Build the Docker image `igdb2yamtrack:local`. |
| `make test` | Run all tests with race detector. |
| `make test-coverage` | Run tests and fail if coverage is below 90%. |
| `make test-docker` | Run tests inside the Docker `test` stage. |
| `make run` | Run the tool via `go run`. |
| `make clean` | Delete `bin/` and `coverage.out`. |
| `make ci` | Run lint + test + build (local CI check). |

## License

[MIT](LICENSE)
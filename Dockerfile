# syntax=docker/dockerfile:1

# ─── Build stage ─────────────────────────────────────────────────────────────
# golang:bookworm is the official Go image based on Debian 12 (Bookworm), which
# is the smallest Debian-based Go build environment available.
FROM golang:bookworm AS build

WORKDIR /src

# Download module dependencies in a separate layer so they are cached when only
# source files change.
COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w" \
    -o /out/igdb2yamtrack \
    ./cmd/igdb2yamtrack

# ─── Test stage ──────────────────────────────────────────────────────────────
# Used by `make test-docker`. Runs the full test suite inside the container so
# results are reproducible regardless of the host Go version.
FROM golang:bookworm AS test

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .

CMD ["go", "test", "-race", "-count=1", "./..."]

# ─── Runtime stage ───────────────────────────────────────────────────────────
# gcr.io/distroless/base-debian12:nonroot provides a minimal Debian 12 base
# with no shell, no package manager, and a pre-created nonroot user (uid 65532).
# The statically linked binary runs without any additional dependencies.
FROM gcr.io/distroless/base-debian12:nonroot AS runtime

COPY --from=build /out/igdb2yamtrack /usr/local/bin/igdb2yamtrack

USER nonroot

ENTRYPOINT ["/usr/local/bin/igdb2yamtrack"]

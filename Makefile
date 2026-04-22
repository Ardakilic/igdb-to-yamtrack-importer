BINARY     := igdb2yamtrack
BUILD_DIR  := bin
CMD_PATH   := ./cmd/igdb2yamtrack
GOFLAGS    := -trimpath -ldflags "-s -w"
COVER_FILE := coverage.out
COVER_MIN  := 90.0
IMAGE      := igdb2yamtrack:local

# Cross-compilation targets: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64,
# windows/amd64, windows/arm64.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: all lint build build-cross build-docker test test-coverage test-docker run ci clean

all: lint test build

ci: lint test build

## lint: Run go vet and check formatting.
lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "The following files need formatting:"; gofmt -l .; exit 1)

## build: Compile the binary for the current platform.
build:
	mkdir -p $(BUILD_DIR)
	go build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY) $(CMD_PATH)

## build-cross: Cross-compile for all supported platforms.
build-cross:
	mkdir -p $(BUILD_DIR)
	$(foreach PLATFORM,$(PLATFORMS), \
		$(eval OS   := $(word 1,$(subst /, ,$(PLATFORM)))) \
		$(eval ARCH := $(word 2,$(subst /, ,$(PLATFORM)))) \
		$(eval EXT  := $(if $(filter windows,$(OS)),.exe,)) \
		GOOS=$(OS) GOARCH=$(ARCH) CGO_ENABLED=0 \
			go build $(GOFLAGS) \
			-o $(BUILD_DIR)/$(BINARY)-$(OS)-$(ARCH)$(EXT) \
			$(CMD_PATH); \
	)

## build-docker: Build the Docker image (runtime stage).
build-docker:
	docker build -t $(IMAGE) .

## test: Run the full test suite with race detector.
test:
	go test -race -count=1 ./...

## test-coverage: Run tests and enforce ≥ $(COVER_MIN)% line coverage.
test-coverage:
	go test -race -count=1 -covermode=atomic -coverprofile=$(COVER_FILE) ./...
	@go tool cover -func=$(COVER_FILE) | tail -1
	@TOTAL=$$(go tool cover -func=$(COVER_FILE) | tail -1 | awk '{print $$NF}' | tr -d '%'); \
	echo "Total coverage: $${TOTAL}%"; \
	awk -v total="$${TOTAL}" -v min="$(COVER_MIN)" \
		'BEGIN { if (total+0 < min+0) { print "Coverage " total "% is below minimum " min "%"; exit 1 } }'

## test-docker: Run the test suite inside the Docker test stage.
test-docker:
	docker build --target test -t igdb2yamtrack:test .
	docker run --rm igdb2yamtrack:test

## run: Run the binary directly with go run (uses environment variables already set).
run:
	go run $(CMD_PATH)

## clean: Remove build artifacts.
clean:
	rm -rf $(BUILD_DIR) $(COVER_FILE)

.PHONY: test test-unit test-contract test-race lint

# Run all unit tests
test-unit:
	go test ./internal/... -short -v

# Run only contract tests (Tool, Channel, Provider)
test-contract:
	go test ./internal/... -run Contract -v

# Run all tests with race detection
test-race:
	go test ./internal/... -race -v

# Run all tests
test:
	go test ./... -v

# Coverage report
coverage:
	go test ./internal/... -coverprofile=cover.out
	go tool cover -html=cover.out -o cover.html

# Lint
lint:
	golangci-lint run ./...

# Build binary
build:
	go build -ldflags "-X github.com/dayuer/nanobot-go/cmd.Version=$$(git describe --tags --always)" -o bin/nanobot .

# Sync upstream python project
sync-upstream:
	cd upstream && git pull origin main
	@echo "Run 'make diff-upstream' to check for breaking changes"

# Diff upstream changes
diff-upstream:
	@cd upstream && git diff HEAD~1 --stat

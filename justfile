default := "build"

# Build the CLI binary
build:
	go build -o gdocs-cli cmd/gdocs-cli/main.go

# Run all tests
test:
	go test ./...

# Run tests with coverage
test-cover:
	go test ./... -cover

# Run tests with verbose output
test-v:
	go test ./... -v

# Clean up dependencies
tidy:
	go mod tidy

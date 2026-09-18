default := "build"

install_dir := env_var('HOME') + "/.local/bin"

# Build stamp from git (works for plain git and colocated jj checkouts).
version := `git describe --always --dirty --abbrev=12 2>/dev/null || echo dev`

# Build the CLI binary
build:
	go build -ldflags "-X main.version={{version}}" -o gdocs-cli cmd/gdocs-cli/main.go

# Install the CLI binary into ~/.local/bin
install: build
	install -d {{install_dir}}
	install -m 0755 gdocs-cli {{install_dir}}/

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

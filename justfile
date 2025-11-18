# AgentMesh Development Tasks
# Run with: just <target>
# Install just: https://github.com/casey/just

# Default recipe - show available commands
default:
    @just --list

# Run all tests with race detection (default)
test:
    go test ./pkg/... ./internal/... ./integration_test/... -race -count=1

# Run tests without race detection (faster, for quick iterations)
test-fast:
    go test ./pkg/... ./internal/... ./integration_test/...

# Run tests with race detection (explicit alias for CI)
test-race:
    go test ./pkg/... ./internal/... ./integration_test/... -race -count=1

# Run linter (only core library - examples excluded)
lint:
    golangci-lint run ./pkg/... ./internal/... --config .golangci.yml --timeout=2m

# Generate test coverage report
cover:
    go test ./... -coverprofile=coverage.out
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report generated: coverage.html"

# Run all quality checks (CI-ready)
check: test lint cover
    @echo "All quality checks completed"

# Clean generated files
clean:
    rm -f coverage.out coverage.html

# Install development dependencies
install-deps:
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Quick development cycle - test and lint with race detector
dev: test lint
    @echo "Development checks completed"

# Fast development cycle - skip race detector for speed
dev-fast: test-fast lint
    @echo "Fast development checks completed"

# Run tests with verbose output
test-verbose:
    go test ./... -v

# Run specific package tests (usage: just test-pkg agent)
test-pkg package:
    go test ./{{package}}/... -v

# Show test coverage by package
cover-summary:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out | grep -E "(total|\.go:)"

# Serve documentation site locally with live reload
docs-serve:
    cd docs && bundle config set --local path 'vendor/bundle'
    cd docs && bundle install
    cd docs && bundle exec jekyll serve --livereload --host 0.0.0.0 --port 4000 --config _config.yml,_config.dev.yml

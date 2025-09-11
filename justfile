# AgentMesh Development Tasks
# Run with: just <target>
# Install just: https://github.com/casey/just

# Default recipe - show available commands
default:
    @just --list

# Run all tests
test:
    go test ./...

# Run tests with race detection
test-race:
    go test ./... -race -count=1

# Run linter
lint:
    golangci-lint run --config .golangci.yml --timeout=2m

# Generate test coverage report
cover:
    go test ./... -coverprofile=coverage.out
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report generated: coverage.html"

# Run all quality checks
check: test-race lint cover
    @echo "All quality checks completed"

# Clean generated files
clean:
    rm -f coverage.out coverage.html

# Install development dependencies
install-deps:
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Quick development cycle - test and lint
dev: test lint
    @echo "Development checks completed"

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

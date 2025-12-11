# Contributing to Hypasis Sync Protocol

Thank you for your interest in contributing to Hypasis! We welcome contributions from everyone.

## Code of Conduct

This project adheres to a code of conduct. By participating, you are expected to uphold this code. Please report unacceptable behavior to team@hypasis.io.

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check existing issues. When creating a bug report, include:

- **Clear title and description**
- **Steps to reproduce**
- **Expected vs actual behavior**
- **System information** (OS, Go version, etc.)
- **Logs and error messages**

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. When creating an enhancement suggestion:

- **Use a clear and descriptive title**
- **Provide a detailed description** of the suggested enhancement
- **Explain why this enhancement would be useful**

### Pull Requests

1. Fork the repo and create your branch from `main`
2. If you've added code, add tests
3. Ensure the test suite passes
4. Make sure your code follows the existing style
5. Write a clear commit message

## Development Setup

### Prerequisites

- Go 1.21+
- Git
- Make

### Getting Started

1. Clone your fork:
   ```bash
   git clone https://github.com/YOUR-USERNAME/sync-protocol.git
   cd sync-protocol
   ```

2. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/hypasis/sync-protocol.git
   ```

3. Install dependencies:
   ```bash
   go mod download
   ```

4. Build:
   ```bash
   make build
   ```

5. Run tests:
   ```bash
   make test
   ```

## Code Style

### Go Code Style

We follow standard Go conventions:

- Run `gofmt` on your code
- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use meaningful variable and function names
- Add comments for exported functions
- Keep functions small and focused

### Commit Messages

- Use present tense ("Add feature" not "Added feature")
- Use imperative mood ("Move cursor to..." not "Moves cursor to...")
- Limit first line to 72 characters
- Reference issues and PRs liberally

Example:
```
Add backward sync throttling

- Implement bandwidth throttling for backward sync
- Add configuration options for throttle limits
- Update documentation

Fixes #123
```

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific package
go test ./pkg/sync/...
```

### Writing Tests

- Write tests for all new code
- Aim for >80% code coverage
- Use table-driven tests where appropriate
- Test edge cases and error conditions

Example:
```go
func TestGapTracker(t *testing.T) {
    tests := []struct {
        name     string
        ranges   []BlockRange
        expected int
    }{
        {"empty", []BlockRange{}, 0},
        {"single", []BlockRange{{0, 100}}, 0},
        {"gap", []BlockRange{{0, 100}, {200, 300}}, 1},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

## Project Structure

```
sync-protocol/
├── cmd/                  # Command-line applications
├── pkg/                  # Public libraries
│   ├── checkpoint/      # Checkpoint management
│   ├── sync/            # Sync coordination
│   ├── storage/         # Storage layer
│   ├── p2p/             # P2P networking
│   ├── api/             # API server
│   └── config/          # Configuration
├── internal/            # Private libraries
├── docs/                # Documentation
├── deployments/         # Deployment configs
└── test/                # Integration tests
```

## Documentation

- Update README.md for user-facing changes
- Update ARCHITECTURE.md for design changes
- Add inline code comments
- Update API documentation

## Release Process

1. Update version in code
2. Update CHANGELOG.md
3. Create git tag
4. Build release binaries
5. Publish Docker images
6. Announce release

## Community

- **Discord**: https://discord.gg/hypasis
- **Twitter**: https://twitter.com/hypasis
- **Forum**: https://forum.hypasis.io

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

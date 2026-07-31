# Contributing to Hypasis Sync Protocol

Thank you for your interest in contributing to **Hypasis Sync Protocol**! We welcome contributions from developers, security researchers, and node operators.

---

## 🚀 Getting Started

### Prerequisites

- **Go**: `v1.21` or higher
- **Make**: For running build automation commands
- **Docker**: For running multi-container cloud tests

### Quick Setup

```bash
# 1. Clone repository
git clone https://github.com/Hypasis/sync-protocol.git
cd sync-protocol

# 2. Install Go dependencies
go mod download

# 3. Build binary
make build

# 4. Run unit tests
go test -v ./...
```

---

## 🛠 Development Guidelines

### Coding Style & Formatting
- All Go code must be formatted using standard `gofmt` and `goimports`.
- Run linter checks locally before submitting a PR:
  ```bash
  golangci-lint run
  ```

### Branch Naming Conventions
- `feat/feature-name` - For new features
- `fix/bug-description` - For bug fixes
- `docs/update-readme` - For documentation changes
- `ci/workflow-fix` - For CI/CD pipeline changes

### Submitting a Pull Request (PR)

1. Fork the repository and create your feature branch.
2. Ensure all tests pass (`go test ./...`) and the binary builds (`make build`).
3. Open a Pull Request on GitHub using our [Pull Request Template](.github/PULL_REQUEST_TEMPLATE.md).
4. Maintainers will review your PR and provide feedback.

---

## 🛡️ Security Vulnerabilities

Please **do not** report security vulnerabilities through public GitHub issues. See our [Security Policy](SECURITY.md) for details on how to report security issues responsibly via `security@hypasis.io`.

## 📜 Code of Conduct

All contributors are expected to adhere to our [Code of Conduct](.github/CODE_OF_CONDUCT.md).

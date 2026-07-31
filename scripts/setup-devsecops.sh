#!/usr/bin/env bash
set -euo pipefail

echo "🛡️ Setting up Hypasis DevSecOps Local Security Environment..."

# 1. Check for Go
if ! command -v go &> /dev/null; then
    echo "❌ Go is required but not installed."
    exit 1
fi

# 2. Check for pre-commit
if ! command -v pre-commit &> /dev/null; then
    echo "⚠️  pre-commit tool not found. Installing via python/pip or brew if available..."
    if command -v brew &> /dev/null; then
        brew install pre-commit gitleaks golangci-lint
    elif command -v pip3 &> /dev/null; then
        pip3 install pre-commit
    else
        echo "Please install pre-commit manually: https://pre-commit.com/#install"
        exit 1
    fi
fi

# 3. Install Go security tools
echo "📦 Installing Go security tools (gosec, goimports)..."
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install golang.org/x/tools/cmd/goimports@latest

# 4. Install pre-commit hooks
echo "🔗 Installing git pre-commit hooks..."
pre-commit install

echo "✅ DevSecOps local environment setup complete!"
echo "   Every git commit will now automatically run local secret scanning, GoSec AST analysis, and linting."

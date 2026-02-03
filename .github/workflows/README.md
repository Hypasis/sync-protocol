# GitHub Actions Workflows

This directory contains GitHub Actions workflows for CI/CD and security scanning.

## Workflows

### 1. Gitleaks Security Scan (`gitleaks.yml`)

**Purpose**: Scans the codebase for hardcoded secrets, API keys, and sensitive data.

**Triggers**:
- Push to `main` or `develop` branches
- Pull requests to `main` or `develop` branches
- Manual trigger via GitHub UI

**What it does**:
- Checks out the entire repository history
- Runs Gitleaks with the custom configuration (`.gitleaks.toml`)
- Fails the build if secrets are detected
- Respects `.gitleaksignore` for false positives

**Configuration**:
- Uses `.gitleaks.toml` for custom rules
- Uses `.gitleaksignore` for approved exceptions

### 2. CI Pipeline (`ci.yml`)

**Purpose**: Comprehensive continuous integration pipeline.

**Jobs**:

#### a. Gitleaks Security Scan
- Same as the standalone Gitleaks workflow
- Runs in parallel with other jobs

#### b. Lint and Format Check
- Checks Go code formatting with `gofmt`
- Runs `go vet` for suspicious constructs
- Runs `golangci-lint` for comprehensive linting

#### c. Build and Test
- Builds the Go application
- Runs all tests with race detection
- Generates coverage report
- Uploads coverage to Codecov (optional)

#### d. Pre-commit Checks
- Runs all pre-commit hooks in CI
- Ensures local pre-commit setup matches CI

**Triggers**:
- Push to `main` or `develop` branches
- Pull requests to `main` or `develop` branches

## Setup Instructions

### 1. Enable GitHub Actions

GitHub Actions are enabled by default for public repositories. For private repositories:

1. Go to repository **Settings** → **Actions** → **General**
2. Enable "Allow all actions and reusable workflows"

### 2. Required Secrets (Optional)

For Gitleaks Pro (optional):
- `GITLEAKS_LICENSE`: Your Gitleaks Pro license key

For Codecov (optional):
- `CODECOV_TOKEN`: Your Codecov upload token

Add secrets at: **Settings** → **Secrets and variables** → **Actions** → **New repository secret**

### 3. Branch Protection Rules

Recommended branch protection settings for `main`:

1. Go to **Settings** → **Branches** → **Add rule**
2. Branch name pattern: `main`
3. Enable:
   - ✓ Require a pull request before merging
   - ✓ Require status checks to pass before merging
   - ✓ Required checks:
     - `Gitleaks Security Scan`
     - `Lint and Format Check`
     - `Build and Test`

## Workflow Status Badges

Add these to your README.md:

```markdown
![Gitleaks](https://github.com/hypasis/sync-protocol/workflows/Gitleaks%20Security%20Scan/badge.svg)
![CI Pipeline](https://github.com/hypasis/sync-protocol/workflows/CI%20Pipeline/badge.svg)
```

## Troubleshooting

### Gitleaks Fails in CI but Passes Locally

**Cause**: Different Gitleaks versions or configuration.

**Solution**:
```bash
# Run the exact same command as CI
gitleaks detect --config .gitleaks.toml --verbose --no-git
```

### Pre-commit Checks Fail in CI

**Cause**: Pre-commit cache differences or missing tools.

**Solution**: The CI workflow installs all required tools. If it still fails:
1. Check the workflow logs
2. Update pre-commit hook versions: `pre-commit autoupdate`
3. Commit and push the changes

### Lint Job Fails but Code Looks Fine

**Cause**: Different golangci-lint versions.

**Solution**: Pin the version in CI (already done in `ci.yml`):
```yaml
curl -sSfL ... | sh -s -- -b $(go env GOPATH)/bin v1.55.2
```

### Tests Pass Locally but Fail in CI

**Cause**: Race conditions, environment differences, or missing dependencies.

**Solution**:
1. Run tests locally with race detector: `go test -race ./...`
2. Check for hardcoded paths or OS-specific code
3. Review CI logs for missing environment variables

## Customization

### Adding More Workflows

Create new workflow files in this directory:

```yaml
name: My Custom Workflow
on: [push, pull_request]
jobs:
  my-job:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: My step
        run: echo "Hello"
```

### Modifying Triggers

Change when workflows run:

```yaml
on:
  push:
    branches: [main, develop, feature/*]
  pull_request:
    branches: [main]
  schedule:
    - cron: '0 0 * * *'  # Daily at midnight
```

### Adding Slack/Discord Notifications

Add notification step to any job:

```yaml
- name: Notify on failure
  if: failure()
  uses: 8398a7/action-slack@v3
  with:
    status: ${{ job.status }}
    webhook_url: ${{ secrets.SLACK_WEBHOOK }}
```

## Performance Optimization

### Caching

The CI pipeline already caches:
- Go modules: `~/go/pkg/mod`
- Pre-commit environments

### Parallel Execution

Jobs run in parallel by default. Sequential execution:

```yaml
jobs:
  job1:
    runs-on: ubuntu-latest
    steps: [...]

  job2:
    runs-on: ubuntu-latest
    needs: job1  # Wait for job1
    steps: [...]
```

## Security Best Practices

1. **Never commit secrets** - Use GitHub Secrets instead
2. **Pin action versions** - Use `@v4` not `@main`
3. **Review third-party actions** - Check source before using
4. **Limit permissions** - Add `permissions:` block when needed
5. **Use GITHUB_TOKEN** - Prefer over personal access tokens

## More Information

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Gitleaks Action](https://github.com/gitleaks/gitleaks-action)
- [Pre-commit CI](https://pre-commit.ci/)

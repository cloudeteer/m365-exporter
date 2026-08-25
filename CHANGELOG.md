# Changelog

All notable changes to this project will be documented in this file.

## v3.9.3

## Fixes

- [#103](https://github.com/cloudeteer/m365-exporter/issues/103) agent users are not requested from the agent API anymore

## v3.9.2

### Build and Release
- Updated CI build job to run on `ubuntu-24.04-arm` and set `GOMAXPROCS=6` to improve build stability.
- Updated GoReleaser GitHub Action to `v7.2.3` and GoReleaser binary version in CI snapshot builds to `v2.17.1`.
- Updated release workflow actions for checkout, setup-go, cosign installer, and docker login.

### Dependencies and Toolchain
- Updated Go version in `go.mod` from `1.24.0` to `1.25.0`.
- Updated Go toolchain directive from `go1.25.6` to `go1.26.5`.
- Updated key runtime dependencies, including:
  - `github.com/microsoftgraph/msgraph-sdk-go` to `v1.100.0`
  - `github.com/microsoftgraph/msgraph-sdk-go-core` to `v1.4.1`
  - `github.com/Azure/azure-sdk-for-go/sdk/azcore` to `v1.22.0`
  - `github.com/Azure/azure-sdk-for-go/sdk/azidentity` to `v1.14.0`
  - `github.com/prometheus/client_golang` to `v1.24.1`
- Refreshed indirect dependencies in `go.sum` and `go.mod` (including OpenTelemetry, x/*, protobuf, and Prometheus ecosystem modules).

### Collectors and Metrics
- Standardized Prometheus label key usage in multiple collectors by introducing local label constants (for example `tenant`, `collector`, `appleId`, and OneDrive label keys), reducing repeated string literals.
- Updated Application collector pagination typing to use `models.Applicationable` and added nil guards for `displayName` and `appId` during iteration.
- Improved Application collector warning logging for missing password credential end dates by using context-aware logging and normalized log field names.
- Updated OneDrive Graph select fields to reuse shared label constants where applicable.

### CI and Linting Actions
- Updated pinned versions of several GitHub Actions used by CI and lint jobs:
  - `actions/checkout` to `v7.0.1`
  - `actions/setup-go` to `v7.0.0`
  - `codecov/codecov-action` to `v7.0.0`
  - `docker/login-action` to `v4.6.0`
  - `golangci/golangci-lint-action` to `v9.3.0`
  - `super-linter/super-linter/slim` to `v8.7.0`

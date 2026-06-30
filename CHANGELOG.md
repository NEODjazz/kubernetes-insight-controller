# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

No unreleased changes.

## [0.1.1] - 2026-06-30

### Added

- Release install manifest for installing the controller without cloning the repository.
- README installation, sample, rollout check, and uninstall commands based on release URLs.

### Changed

- Default deployment image tag now points to `ghcr.io/neodjazz/kubernetes-insight-controller:0.1.1`.

## [0.1.0] - 2026-06-30

### Added

- Initial Kubernetes Insight Controller implementation.
- GitHub Actions workflows for CI tests and tagged releases.
- Build-time version metadata exposed with the manager `--version` flag.
- Apache License 2.0 project license.

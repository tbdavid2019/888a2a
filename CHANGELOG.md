# Changelog

All notable changes to 888a2a are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses [Semantic Versioning](https://semver.org/) for releases.

## [Unreleased]

### Added

- Added the 888a2a project README and Traditional Chinese README with the
  project direction, upstream attribution, roadmap, and major TODO areas.
- Added GitHub Actions CI for Agent Network naming checks, Go lint/tests/build,
  and frontend format/lint/type-check/tests/build.
- Added Node.js 24 as the frontend CI and release workflow runtime.
- Added SLSA generic provenance generation for release binaries.

### Changed

- Established `tbdavid2019/888a2a` as the public repository while retaining
  `Ranxy/laelia` as the upstream source.
- Normalized existing frontend imports so the Biome CI check passes.

### Fixed

- Made the tampered Agent JWT test modify the payload reliably instead of
  changing a Base64URL signature character whose unused bits may decode to the
  same bytes.

## How to update this file

Add every meaningful change to the `[Unreleased]` section before committing.
Move the unreleased entries into a dated version section when creating a
release.

# TakeBinder Product Specification

[日本語](SPECIFICATION.ja.md)

- Document version: 0.2.5
- Status: Draft
- Target release: MVP
- Last updated: 2026-08-15

The specification is split by subject. Use this file as the entry point and update only the relevant document when making a change.

| Document | Contents |
| --- | --- |
| [Product overview](product.md) | Goals, terminology, scope, UI, and open questions |
| [Functional requirements](functional-requirements.md) | Scanning, classification, joining, concurrency, output, and errors |
| [Osmo Pocket / DJI support](osmo-pocket-support.md) | Research sources, experimental individual-file copying, and validation gates for future joining |
| [Architecture](architecture.md) | Electron, Go backend, external tools, and communication protocol |
| [Quality and testing](quality-and-testing.md) | Non-functional requirements, acceptance criteria, testing, and implementation phases |
| [Releasing](releasing.md) | Version tags, release validation, artifacts, and current signing limitations |
| [Security exceptions](security-exceptions.md) | Time-limited supply-chain policy exceptions |

## Critical requirements

1. Do not re-encode video or audio.
2. Preserve GoPro GPMF telemetry present in the input.
3. Do not modify, move, or delete input files.
4. Do not report output as successful unless GPMF preservation can be verified.
5. Copy a single-file capture without remuxing it.
6. Do not implicitly run install-time scripts from Yarn dependencies, and reject lockfile inconsistencies in development, CI, and release environments.

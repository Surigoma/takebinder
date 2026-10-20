# Architecture

[日本語](architecture.ja.md)

## 1. System structure

```text
Electron Renderer
  └─ UI, settings, and progress display
       │ IPC (approved APIs only)
Electron Main
  └─ Window, directory selection, and Go process management
       │ JSON Lines over stdin/stdout
Go Backend
  ├─ File scanning and group detection
  ├─ Job queue and concurrency control
  ├─ Copying, hashing, and report generation
  ├─ Media-tool execution and cancellation
  └─ Output verification
       ├─ FFmpeg (stream-copy joining)
       ├─ ffprobe (stream inspection)
       ├─ mp4-merge (Pocket 4 MP4 table joining)
       └─ GPMF parser (telemetry verification candidate)
```

## 2. Electron

- Use TypeScript.
- The Renderer uses React, Material UI, i18next, and react-i18next, bundled by Vite as browser-targeted ES modules.
- Electron Main and preload are compiled separately by TypeScript Compiler as Node.js-targeted CommonJS.
- The Renderer does not access Node.js APIs directly.
- Enable `contextIsolation` and disable `nodeIntegration`.
- Expose only the minimum required API through preload.
- Do not place file-processing logic in the Renderer.
- A Renderer run request contributes group IDs only. The Go backend restores file paths and destinations from its latest scan result and rejects unscanned, duplicate, or review-required groups.

## 3. Go backend

- Build one executable for each OS/CPU target.
- Communicate with Electron using JSON Lines over standard input and output.
- Write only machine-readable events to stdout; write diagnostics to stderr.
- Stop jobs and child processes through context cancellation.
- Limit concurrency with a semaphore.
- Pass external commands as an executable and argument array; never assemble shell command strings.
- During scanning, use ffprobe to inspect video, GoPro-specific tags, and `gpmd`. Prefer Media ID, Capture ID, and Chapter tags for grouping when available; otherwise fall back to GoPro filename conventions and return the decision basis and confidence.
- Experimental DJI discovery uses Pocket model tags or DJI metadata/filename hints. Only Pocket 4 files satisfying the verified split-continuity contract form a joined group; all others remain independent byte-copy units. See [support status](osmo-pocket-support.md).
- To preserve originals, open input read-only, write a temporary copy on the output filesystem, verify it with SHA-256, and only then finalize its name. Before starting, use OS APIs to estimate capacity for finished videos and source copies; return `E_DISK_FULL` when space is insufficient.
- Set the first chapter's `creation_time` on joined output and verify it with ffprobe before finalizing. After completion, set the file modification time and, on Windows, creation time. Timestamp failures on unsupported operating systems or filesystems are recorded in warnings and reports and do not mark verified media as corrupt.
- The Go backend determines default output names and relative input subdirectories during scanning, then restores them from the saved scan plan at execution time. When date directories are enabled, use `date/input-relative-directory/video`.

## 4. Native media tools

Electron and Go code are built for each OS/CPU target. FFmpeg, ffprobe, and mp4-merge are not bundled with the application; pinned standalone binaries explicitly downloaded by the user through the UI are stored in application-managed storage.

Distributions are generated in native environments using `package:windows`, `package:macos`, and `package:linux`, with each task verified by the CI OS matrix.

The selected tools must satisfy all of the following through technical validation with real GoPro samples:

- Concatenate `gpmd` tracks without dropping data
- Reconstruct timestamps and sample tables correctly
- Do not re-encode H.264 or H.265
- Meet licensing requirements for commercial distribution

Multiple chapters are passed to FFmpeg's concat demuxer in order. Video, audio, and `gpmd` are mapped explicitly and muxed with `-c copy`. A successful command alone is not completion: ffprobe compares input/output video and audio properties, total duration, and the SHA-256, order, and size of every packet. For `gpmd`, it also verifies a time range covering the full video and preservation of major input keys (`GYRO`, `ACCL`, `GPS5`). Real GoPro samples have been verified for matching video, audio, and `gpmd` packet payloads and preservation of major GPMF keys.

Pocket 4 split groups are passed to pinned mp4-merge 0.1.11 in chapter order. It rewrites MP4 sample tables without decoding media. mp4-merge preserves only the first `tmcd` sample and attached MJPEG thumbnail. TakeBinder retains the SMPTE start payload and extends its MP4 `stts` duration across the joined video after validating chapter-to-chapter timecode continuity. Before atomic rename, ffprobe verifies the full seven-track layout, summed duration, creation time, exact concatenated packet hashes/counts/bytes for HEVC, both AAC tracks, `djmd`, and `dbgi`, the unchanged first `tmcd` and MJPEG payloads, and a timecode range covering the joined video.

## 5. Backend communication protocol

### 5.1 Commands

| Command | Purpose |
| --- | --- |
| `scan` | Scan the input directory and return candidate capture groups |
| `run` | Execute a reviewed processing plan |
| `cancel` | Stop current processing |
| `status` | Return backend and managed-tool status |
| `install-tools` | Download and verify tools from the pinned manifest |
| `remove-tools` | Remove only downloaded tool trees from managed storage; payload must be `{}` |

### 5.2 Events

| Event | Purpose |
| --- | --- |
| `scan.progress` | File-scan progress |
| `scan.completed` | Grouping results |
| `job.started` | Group processing started |
| `job.progress` | Group processing progress |
| `job.warning` | Recoverable warning |
| `job.completed` | Group processing succeeded |
| `job.failed` | Group processing failed |
| `run.completed` | All processing finished |
| `tools.install.progress` | Tool-download byte progress |
| `tools.install.completed` | FFmpeg, ffprobe, and mp4-merge download results |
| `tools.remove.completed` | Removal succeeded; payload contains refreshed `tools` status |

Every message contains `protocolVersion`, `requestId`, `type`, and `payload`.

Tool status includes `available` and `removable` independently: an external PATH tool can be available after managed deletion, and a corrupt or old managed installation can still be removable. A backend read/write mutex prevents tool mutation during scans/runs and serializes installation/removal. Removal uses `os.Root` to confine deletion to the reserved tool trees and invalidates the scan plan. Errors use `E_TOOLS_BUSY` or `E_TOOL_REMOVE`; removal failure is followed by refreshed `status.completed`.

## 6. Software supply-chain protection

### 6.1 Yarn dependencies

- Pin Yarn 4.17.1 through `packageManager` and use Corepack for consistent CI and development environments.
- Commit `yarn.lock`; CI and release builds use `corepack yarn install --immutable`.
- Pin direct dependencies to exact versions and transitive dependencies through the lockfile.
- Review necessity, maintenance status, publisher, repository, license, known vulnerabilities, and install-time scripts before adding a dependency.
- Dependencies from Git URLs, arbitrary tarballs, or local paths are prohibited by default; use exact versions from the npm registry.
- Fail CI on lockfile differences or inconsistencies. Keep dependency-only PRs separate from application changes.
- Do not use `yarn dlx` to download unpinned packages in CI or documentation. Pin tasks in `Taskfile.yml`.
- Set `npmMinimalAgeGate` to seven days to avoid newly published dependencies.

### 6.2 Install-time scripts

- Disable dependency install-time scripts with `enableScripts: false` in `.yarnrc.yml`.
- When lifecycle processing is required, provide an explicit task after reviewing its content and version. Electron 43 is downloaded only by `task install:electron`.
- Before adding an exception, review the script, download destination, and generated artifacts. Blanket script approval is prohibited.

### 6.3 Vulnerabilities and updates

- Run `yarn npm audit --recursive` and dependency-diff review in PRs. High/critical findings block merge unless impact is assessed and a time-limited exception is recorded.
- Do not perform unreviewed bulk updates with `yarn up`.
- Electron includes Chromium and Node.js, so use a supported version and prioritize security updates.
- Automated update PRs are allowed but are not merged automatically. Review the lockfile, install-time scripts, and test results.

### 6.4 CI and releases

- Pin Task to v3.51.1 in CI and use `Taskfile.yml` as the shared entry point for development and CI.
- Pin third-party GitHub Actions to full commit SHAs.
- Minimize workflow `permissions` per job; routine tests receive read-only access.
- Do not expose signing keys, publication tokens, or other release secrets to external-contributor PRs.
- Produce releases from protected tags or approval-gated environments using a clean checkout and immutable install.
- Do not expose code-signing keys during dependency installation or tests; provide them only to the signing stage for verified artifacts.
- Generate the CycloneDX 1.6 JSON SBOM from pinned Yarn `yarn info -A -R --json` output without downloading additional dependencies. Reconcile it with the lockfile-derived dependency graph during packaging and include it in the distribution.

### 6.5 Go and managed binaries

- Commit `go.mod` and `go.sum`, and review Go dependency changes in separate PRs.
- Pin versions and sources for downloaded FFmpeg, ffprobe, and similar artifacts; verify them against published checksums and repository-managed SHA-256 values.
- Record provenance, version, license, and checksums in the release manifest.
- Do not download a “latest” version during builds.

### 6.6 References

- [Yarn installation](https://yarnpkg.com/getting-started/install)
- [Yarn Corepack](https://yarnpkg.com/corepack)
- [Task installation](https://taskfile.dev/docs/installation)
- [Electron Security](https://www.electronjs.org/docs/latest/tutorial/security)
- [GitHub Actions hardening](https://docs.github.com/en/code-security/tutorials/secure-your-organization/protect-against-threats)

### 6.7 Native-tool download

- Pin download URLs, versions, filenames, sizes, and SHA-256 hashes in the Go binary. Do not replace them at runtime with a remote server manifest.
- Allow HTTPS only and validate both initial and redirected hosts against an allowlist.
- Download into a size-limited temporary file and finalize it in application-managed storage only after size and SHA-256 match.
- Download standalone FFmpeg and ffprobe binaries from pinned Shaka Project releases and mp4-merge from its pinned Gyroflow release. Verify exact size and SHA-256, then run a non-mutating startup check. Never execute an installer.
- Current targets are Windows x64, macOS x64/arm64, and Linux x64/arm64.

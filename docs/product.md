# Product Overview

[日本語](product.ja.md)

## 1. Overview

TakeBinder is a desktop application that scans a selected directory for chapters created when a GoPro splits one recording because of file-size limits or similar constraints, then combines each recording into one MP4 and organizes the results into predictable folders.

It does not re-encode video or audio and preserves telemetry such as gyroscope, accelerometer, and GPS data stored in GoPro-specific GPMF tracks. Independent capture groups are processed in parallel. A group containing only one file is output unchanged without media processing.

## 2. Terminology

Experimental Osmo Pocket / DJI support joins only conservatively identified Pocket 4 long-recording splits validated with real footage. Other DJI MP4s are copied individually. See [scope, evidence, and safety rules](osmo-pocket-support.md).

| Term | Definition |
| --- | --- |
| Capture | One recording produced between pressing record and stop on a GoPro |
| Chapter | One file in a capture split across multiple MP4 files |
| Capture group | One or more chapters belonging to the same capture |
| GPMF | GoPro Metadata Format, time-series metadata containing gyroscope, accelerometer, GPS, camera information, and similar data |
| Pass-through | Copying an input file to the output while preserving its bytes and performing no media conversion |
| Stream copy | Placing streams in a new MP4 container without decoding or re-encoding video or audio |

The phrase “per caption” in the original request is interpreted as “per capture (capture group).”

## 3. Goals

- Organize large collections of GoPro videos by recording without manual selection.
- Preserve video and audio quality exactly.
- Preserve GoPro GPMF telemetry for downstream tools such as Gyroflow.
- Provide the same user experience on Windows, macOS, and Linux.
- Reduce processing time by handling independent capture groups in parallel.
- Process safely without modifying or deleting input files.

## 4. Scope

### 4.1 Included in the MVP

- Input and output directory selection
- Recursive MP4 scanning
- GoPro-file detection
- Automatic grouping of chapters from the same capture
- Automatic chapter ordering
- Parallel processing by capture group
- Lossless joining of multiple chapters
- Pass-through copying of single files
- GPMF preservation and post-processing verification
- Progress, success, warning, and failure-reason display
- Processing cancellation
- Result report storage
- Output directories grouped by capture date
- Optional copying of source videos into `original` within the output
- Preservation of the first chapter's capture time on output
- Distribution for Windows, macOS, and Linux
- Supply-chain protection for Yarn dependencies, CI, and bundled binaries

### 4.2 Excluded from the MVP

- H.264/H.265 conversion
- Resolution, frame-rate, or bitrate changes
- Stabilization
- Frame-level editing or arbitrary cuts
- Video preview or timeline editing
- Cloud uploads
- Moving or deleting input files
- Stitching 360-degree video
- Operating a private package registry or custom security-monitoring platform

## 5. UI requirements

The MVP has two primary pages: Convert and Settings. Convert contains routine actions; detailed behavior and tool management belong in Settings. Input/output directories and Settings values are restored on the next launch.

1. Input directory selection by dialog or drag and drop, preserving the previous value
2. Output directory selection by dialog or drag and drop, preserving the previous value
3. Scanning and output options on the Settings page
   - Include subdirectories
   - Flatten or preserve the input directory structure
   - Strict SHA-256 verification
   - Maximum parallelism
   - Capture-date directories and their format (`{YYYY}`, `{MM}`, `{DD}`; default `{YYYY}-{MM}-{DD}`)
   - Enable or disable JSON reports
   - Copy source videos into `original`
   - Preserve the first chapter's capture time on output
4. Scan button
5. Capture-group list
   - Planned output name
   - Chapter count
   - Total size and duration
   - Capture time and resolution
   - GPMF presence
   - Classification status
6. Start button
7. Progress bar for each capture group
8. Log and completion summary

The user reviews scan results before starting processing in a two-stage workflow.

## 6. Open questions

- Oldest supported GoPro generation
- Whether 360-degree video can be treated as a simple track-concatenation target
- Whether non-GoPro MP4 files should be eligible for pass-through
- Final default for flat output versus preserving subdirectories
- Whether SHA-256 verification should be enabled by default
- Whether the first release should include macOS Intel
- Whether FFmpeg can safely join GPMF for every target model (verified with supplied samples)
- Final icon, signing/notarization, and distribution method

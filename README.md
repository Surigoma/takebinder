# TakeBinder

[日本語](README.ja.md)

TakeBinder joins the chapter files created by a GoPro into one MP4 per recording and organizes the results into predictable folders. It keeps the original video and audio streams without re-encoding and preserves GoPro GPMF telemetry used by tools such as Gyroflow.

See the [changelog](CHANGELOG.md) for release history.

Experimental Osmo Pocket / DJI discovery supports copying and date-based organization of individual MP4 files. **DJI split recordings are not automatically joined.** Filename candidates cannot confirm the camera model; real Pocket footage has not yet been validated. See [support scope and research](docs/osmo-pocket-support.md).

## Highlights

- Automatically finds and orders chapters from the same recording.
- Joins multiple chapters without reducing video or audio quality.
- Preserves and verifies GPMF data, including gyroscope, accelerometer, and GPS metadata.
- Copies a single-file recording byte-for-byte instead of remuxing it.
- Processes independent recordings in parallel.
- Runs on Windows, macOS, and Linux.

## Install and launch

TakeBinder is distributed as a portable application. Download the package for your operating system from [GitHub Releases](https://github.com/Surigoma/takebinder/releases), verify it with `SHA256SUMS.txt`, extract it, and keep every file in the extracted directory together.

- Windows: launch `TakeBinder.exe` inside `TakeBinder-win32-x64`.
- macOS: launch `TakeBinder.app`.
- Linux: launch `takebinder` inside `TakeBinder-linux-x64`.

On first use, open **Settings** and select **Download verified tools**. TakeBinder downloads pinned standalone FFmpeg, ffprobe, and mp4-merge binaries, verifies their size and SHA-256 hash, and stores them in the application's managed data directory. An internet connection is needed for this download; video processing itself is local.

## Quick start

1. Open **Settings** and choose your language and output preferences.
2. Download the verified media tools if they are not already available.
3. Return to **Convert** and select the directory containing your GoPro MP4 files.
4. Select an output directory.
5. Select **Scan folder**.
6. Review the detected captures. Only captures marked **Ready** are processed automatically.
7. Select **Start** and keep the application open until processing finishes.

The activity log reports completed files, warnings, skipped files, and failures. When JSON reporting is enabled, a detailed report is also written to the output directory.

## Your source videos are protected

TakeBinder is designed never to modify, move, or delete input videos.

- Existing output files are not silently overwritten; a numbered name is chosen instead.
- Work is written to a temporary file and finalized only after verification succeeds.
- Video and audio are stream-copied, not decoded and re-encoded.
- A one-file capture is copied without media processing.
- Output with unverifiable GPMF preservation is not reported as successful.
- Cancelling does not delete files that already completed.

Videos stay on your computer. The application does not upload them to a cloud service.

## Main settings

| Setting | Purpose |
| --- | --- |
| Filename format | Builds names with `{YYYY}`, `{MM}`, `{DD}`, `{hh}`, `{mm}`, `{ss}`, and `{NAME}` |
| Preserve input subfolders | Recreates the input directory structure below the output directory |
| Create folders by capture date | Groups output using `{YYYY}`, `{MM}`, and `{DD}` |
| Copy source videos | Copies source chapters into the date directory's `original` folder |
| JSON report | Writes a machine-readable processing report |
| Include subfolders | Scans directories below the selected input directory |
| Strict SHA-256 verification | Verifies byte-for-byte copies with SHA-256 |
| Concurrent capture groups | Limits parallel processing; leave blank for automatic selection |

Use **Restore defaults** to reset the language and conversion settings. Saved input and output directory selections are retained.

## Output behavior

- The default filename is similar to `2026-08-09_143052_GX012345.mp4`.
- Date directories use `2026-08-09` by default.
- A capture containing multiple compatible chapters is joined into one MP4.
- A capture containing one file is copied unchanged.
- Incompatible, incomplete, ambiguous, non-GoPro, or unreadable input is skipped or failed with a reason rather than converted automatically.

## Current limitations

- The application does not convert between H.264 and H.265 or change resolution, frame rate, bitrate, or HDR properties.
- It does not provide trimming, timeline editing, preview, stabilization, cloud upload, or 360-degree stitching.
- Stream-incompatible chapters fail instead of falling back to re-encoding.
- Real GoPro footage has passed packet-level video, audio, and `gpmd` verification, but every GoPro model and recording mode has not yet been validated.
- The project is currently an MVP. Treat irreplaceable footage as valuable source material and keep an independent backup.

## Trademark notice

TakeBinder is an independent, fan-made application. It is not affiliated with, endorsed by, or in any way associated with GoPro, Inc. or its products and services.

GoPro, HERO, MAX, MISSION, and their respective logos are trademarks or registered trademarks of GoPro, Inc. in the United States and other countries.

## Building from source

Development requires Node.js 24, Corepack, Yarn 4.17.1, Go 1.26, and Task 3.x.

```powershell
go install github.com/go-task/task/v3/cmd/task@v3.51.1
task install
task install:electron
task dev
```

See the [product specification](docs/SPECIFICATION.md) for implementation details and safety requirements.

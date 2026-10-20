# Functional Requirements

[日本語](functional-requirements.ja.md)

## FR-01 Directory selection

1. The user can select input and output directories in the GUI.
2. Input and output directories can be selected through a dialog or drag and drop, and previous values are restored on the next launch.
3. Recursive scanning is enabled by default and can be disabled in Settings.
4. If the input and output directories are identical, or output is below input, previously generated output is not scanned again.
5. Input files are treated as read-only and are never overwritten, moved, or deleted.
6. Values changed on the Settings page are saved and restored on the next launch.
7. After confirmation, the user can restore the display language and conversion settings to their defaults. Saved input and output directories are retained.

## FR-02 File detection

1. Files with a `.mp4` or `.MP4` extension are candidates.
2. MP4 track structure and metadata are inspected to determine whether a file originated from a GoPro.
3. Each result is classified as one of the following:
   - GoPro video with GPMF
   - GoPro video without GPMF
   - Osmo Pocket identified by model metadata (`osmo-pocket`, copy only)
   - DJI candidate identified by manufacturer or filename hints (`dji-candidate`, copy only)
   - Non-GoPro MP4
   - Corrupt or unreadable
4. GoPro videos with GPMF are processed by default. Experimental DJI support joins only Pocket 4 splits that satisfy the verified continuity contract; other discovered DJI MP4s are copied independently. Other files are skipped with a displayed reason.
5. DJI model metadata or filename hints produce `osmo-pocket` or `dji-candidate` respectively. Filename hints do not establish the camera model or recording boundaries. See [Osmo Pocket / DJI support](osmo-pocket-support.md) for matching rules, sources, and unverified behavior.

## FR-03 Capture-group detection

Capture groups are determined using the following sources in descending order of confidence:

1. Media ID, Capture ID, and Chapter data in GPMF or MP4 metadata
2. Capture and chapter numbers in GoPro filename conventions
3. Supporting evidence from camera model, codec, resolution, frame rate, creation time, and continuity

If different captures might be joined incorrectly, the group is marked as requiring review and is not joined automatically. When a filename-only heuristic is uncertain, the UI displays its basis and confidence.

DJI candidates are independent low-confidence `DJI file copy` units even when generic Capture ID/Chapter tags exist. Confirmed Pocket 4 files are grouped only when exact names, sequential counters, split size, filename-time continuity, SMPTE timecode continuity, directory, and complete stream layout all match; those units use basis `DJI Pocket 4 split continuity`. Other multi-file DJI units are rejected with `E_GROUP_AMBIGUOUS`.

## FR-04 Chapter order

1. Explicit chapter numbers take priority.
2. MP4 start time and filename are used only when chapter numbers are unavailable.
3. Missing or duplicate numbers and reversed chronology produce warnings.
4. A group whose order cannot be established is not processed automatically.

## FR-05 Compatibility preflight

Before joining, compare at least the following across every chapter in a group:

- Video codec and profile
- Resolution
- Frame rate and time base
- Color space and HDR metadata
- Audio codec, sample rate, and channel layout
- Presence of the GPMF `gpmd` track
- Timecode and continuity of every track

A combination that cannot be safely joined by stream copy fails without automatically falling back to re-encoding.

## FR-06 Joining multiple chapters

1. Video and audio are stream-copied and never re-encoded.
2. GPMF payloads and timing information are concatenated in chapter order.
3. Timecode tracks are concatenated or reconstructed when required.
4. The finished file is MP4.
5. Output is written to a temporary file and receives its final name only after successful verification.
6. An incomplete file is never left as a finished output after failure.

## FR-07 Single-file captures

1. A capture group containing one file is not joined or remuxed.
2. If input and output locations differ, the file is copied byte-for-byte.
3. File size is compared after copying; SHA-256 is also compared when strict verification is enabled.
4. Modification time is preserved where possible.
5. If input and output refer to the same file, no copy is made and the operation completes as unchanged.

## FR-08 Parallel processing

1. The unit of parallelism is the capture group.
2. Chapter order is preserved within each group.
3. Default concurrency is `min(4, half the logical CPU count, capture-group count)`.
4. The user can choose a value from 1 through 8.
5. To limit storage load, the default is capped at two when input and output are on the same storage device. An explicit user value from 1 through 8 takes priority.
6. The UI displays progress per capture group.
7. Failure of one group does not stop independent groups.

## FR-09 Output

1. The default filename is generated from the original capture identifier and capture time.
2. Example: `2026-08-09_143052_GX012345.mp4`
3. Existing files are not overwritten; `_2`, `_3`, and similar suffixes are added.
4. The user can preserve the input subdirectory structure or flatten output into the output root. Flat output is the default.
5. Characters unsupported in filenames are safely replaced for each OS.
6. The user can group output in capture-date directories. Directory names use the same brace syntax as filenames: `{YYYY}`, `{MM}`, and `{DD}`, with `{YYYY}-{MM}-{DD}` as the default. Text outside braces is not substituted. Groups captured on the same date share a directory. Formats containing path separators or other unsafe directory syntax are rejected. Previously saved legacy formats are migrated to brace syntax.
7. Capture time is taken from the first chapter's media metadata when available, then falls back to that chapter's file modification time. The selected source is recorded in the report.
8. The user can copy each date's source videos into an `original` directory directly below that date directory. This option is disabled by default.
9. Source preservation uses copying only; input files are never moved, modified, or deleted. Additional required capacity is displayed before processing, and processing does not start when free space is insufficient.
10. An original-copy name collision never causes an implicit overwrite. The implementation verifies identity with the existing file or chooses a safe numbered name.
11. Output videos preserve the first chapter's capture time by default. The container capture time and file modification time are set, and file creation time is set on supported operating systems.
12. Timestamp support depends on the OS and filesystem. Fields that cannot be set are recorded as warnings and in the report. Timestamp-setting failure alone does not mark verified media as corrupt.
13. When date directories and preservation of the input directory structure are both enabled, output is placed at `date/input-relative-directory/video`. `original` remains directly below the date directory.
14. The user can configure filenames with `{YYYY}`, `{MM}`, `{DD}`, `{hh}`, `{mm}`, `{ss}`, and `{NAME}` (the first chapter's filename without its extension). The default is `{YYYY}-{MM}-{DD}_{hh}{mm}{ss}_{NAME}`. Text outside braces is not substituted. `.mp4` is appended automatically, and the setting is saved for the next launch. Formats containing path separators are rejected; characters unsupported by the OS are safely replaced.

## FR-10 Post-processing verification

Joined output is verified for all of the following:

- The output MP4 can be parsed.
- Required video, audio, and GPMF tracks exist.
- Video and audio codecs match the input.
- Output duration matches the sum of input chapter durations within tolerance.
- GPMF samples cover the entire video duration.
- Major GPMF keys present in input, such as `GYRO`, `ACCL`, and `GPS5`, remain present.
- Input and output video/audio packets prove that no re-encoding occurred.

Output that fails verification is not treated as complete. Source files are always retained.

## FR-11 Cancellation and reruns

1. The user can cancel all processing.
2. Running external processes are stopped safely and temporary files remain incomplete.
3. Completed files are not deleted.
4. On rerun, existing reports and output are inspected so completed groups can be skipped.

## FR-12 Reports

Report generation is enabled by default and can be disabled in Settings. When enabled, a JSON report is written to the output directory after processing. When disabled, no report file is created. Reports contain:

- Application and backend versions
- Start and end times
- Input and output directories
- Detected-file and capture-group counts
- Input files, order, and decision basis for every group
- Join, pass-through, skip, and failure results
- Input and output stream information
- GPMF verification results
- Capture-time source and timestamp warnings
- Error codes and diagnostic messages

## Error policy

| Code | Condition | Behavior |
| --- | --- | --- |
| `E_INPUT_UNREADABLE` | Input cannot be read | Mark the item failed and continue |
| `E_NOT_GOPRO` | Not a GoPro video | Skip |
| `E_GROUP_AMBIGUOUS` | Group detection is uncertain | Require review; do not process automatically |
| `E_CHAPTER_GAP` | A chapter is missing | Warn and do not process by default |
| `E_STREAM_MISMATCH` | Streams are incompatible | Fail without re-encoding |
| `E_GPMF_MISSING` | Required GPMF is absent | Fail or warn according to input state |
| `E_GPMF_VERIFY` | Output GPMF verification failed | Do not treat output as complete |
| `E_OUTPUT_EXISTS` | Output name collision | Choose a safe numbered name |
| `E_DISK_FULL` | Insufficient free space | Stop new jobs and report the error |
| `E_CANCELLED` | User cancellation | Do not treat temporary output as complete |
| `E_TOOLS_BUSY` | Tools are in use or being changed | Reject install/removal and retry after activity finishes |
| `E_TOOL_REMOVE` | Managed tools cannot be fully removed | Report failure and refresh status |

## FR-13 External-tool download

1. The user can download pinned standalone `ffmpeg`, `ffprobe`, and `mp4-merge` binaries.
2. URLs, versions, expected sizes, and SHA-256 hashes are fixed in the application; user-provided or remote manifests are not accepted.
3. Only HTTPS and approved hosts are allowed, including after redirects.
4. A file with a mismatched size or SHA-256 hash is not used and its temporary file is deleted.
5. Tools in application-managed storage take priority.
6. FFmpeg and ffprobe become available only after their `-version` commands succeed. mp4-merge becomes available only after an unmodified no-argument startup check succeeds. Installers and package managers are never executed.
7. Settings offers removal of downloaded FFmpeg, ffprobe, and mp4-merge after confirmation. Remove only the reserved `ffmpeg`, `ffprobe`, and `mp4_merge` trees within application-managed tool storage, including old versions and partial downloads. Never delete PATH-installed tools, input/output videos, or other storage entries. Constrain filesystem traversal to the managed root, including symbolic links.
8. Tool installation and removal are mutually exclusive and cannot overlap scanning or processing. Reject conflicting requests with `E_TOOLS_BUSY`; disable these actions in the UI while busy. `remove-tools` accepts only an empty object, never paths, and emits `tools.remove.completed` with refreshed tool status. The status field `removable` indicates whether a managed tool entry exists, independently of availability or checksum validity.
9. Removal invalidates the scan plan. Refresh tool status after success or failure; report failures with `E_TOOL_REMOVE` and never display partial removal as complete. The user can download the pinned tools again. External tools may remain available after managed removal.

## FR-14 Display language

1. The UI supports Japanese and English.
2. On first launch without a saved selection, Japanese is used when the OS language is Japanese; otherwise English is used.
3. A selected language is applied immediately and saved for the next launch.

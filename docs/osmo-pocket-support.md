# Osmo Pocket / DJI support status

## Scope

Initial Osmo Pocket, DJI Pocket 2, Osmo Pocket 3, and Osmo Pocket 4 are targets for experimental discovery and **individual-file copying and organization only**. Automatic joining of DJI recordings is not implemented. No real Pocket footage was available for this implementation; recognition across firmware versions and camera generations remains unverified.

## Research (2026-10-18)

| Subject | Confirmed information | Implementation consequence |
| --- | --- | --- |
| Split size | DJI documents 4 GB splitting for the original Pocket / Pocket 2 and approximately 16 GB for Pocket 3 / Pocket 4 series. | File size alone does not establish a recording boundary. No size threshold or timestamp-continuity heuristic is used to join files. |
| Naming | Pocket 3 lets users change folder and file naming rules. The manual does not specify a reliable cross-generation filename-to-recording mapping. | Names are discovery hints only. A `DJI_` name does not establish a Pocket model, capture ID, or chapter number. |
| Video / audio | Pocket 3 documents H.264 / HEVC, Normal / HLG / D-Log M, and recording modes with separate audio files. | Copy the entire MP4 without remuxing. Sidecar audio, `.LRF`, `.SRT`, still images, and other non-MP4 files are outside this feature's scope and remain in the input. |
| Telemetry | ExifTool's DJI parser documents DJI `djmd` / `dbgi` timed metadata and the Pocket 3 `dvtm_PP-101.proto` protocol. This does not establish a Pocket 4 metadata schema or a safe concat method. | Do not substitute GoPro `gpmd` verification for DJI telemetry verification. Byte copying retains all embedded bytes, including unknown metadata. |

Sources:

- [DJI: Osmo Action / Osmo Pocket series video split logic](https://repair.dji.com/help/content?customId=01700006948&lang=en&paperDocType=ARTICLE&re=US&spaceId=17)
- [DJI: Pocket 3 user manual](https://dl.djicdn.com/downloads/DJI_Osmo_Pocket_3/UM/20250826/DJI_Osmo_Pocket_3_User_Manual_v1.0_en.pdf), Naming Management, Video Compression, recording modes, and color modes
- [ExifTool: DJI parser source](https://github.com/exiftool/exiftool/blob/master/lib/Image/ExifTool/DJI.pm), timed metadata and Pocket 3 protocol mapping

## Discovery and safety contract

1. Inspect MP4s using the existing ffprobe path. Files without a video stream or unreadable files remain excluded.
2. Recognize explicit Pocket model names in format or stream `model`, `camera_model`, or `com.apple.quicktime.model` tags as `osmo-pocket`. Matching ignores spaces, underscores, hyphens, and case; only the listed generations are recognized. These are conservative application matching rules, not a claim that every camera exposes these tags through ffprobe.
3. Otherwise, a `DJI` make/manufacturer tag or the case-insensitive filename hint `DJI_[A-Z0-9_]+.MP4` produces `dji-candidate`. This includes other DJI cameras and may include renamed files. Do not label these as confirmed Pocket footage. Custom names without identifying tags cannot be recognized.
4. Each DJI file has its own path-based identity and chapter value 1. Do not merge files with adjacent counters, equal times, generic Media/Capture IDs, or matching names in different folders. These units are files, not verified complete captures. Confidence is low and basis is `DJI file copy`.
5. Use the existing atomic copy, collision avoidance, date folders, optional original copying, timestamp handling, and strict SHA-256 verification. No video/audio encoder or remuxer is invoked. The container's original bytes and capture timestamp remain unchanged.
6. Group `hasGpmf` reflects actual input `gpmd` presence; absence is permitted for DJI copying. `gpmfVerification: byte-identical` denotes the existing copy verification result, not the presence of a GPMF track. Reports retain the classification, per-file decision basis, and actual GPMF-presence flag.
7. Reject any multi-file processing unit containing a DJI classification with `E_GROUP_AMBIGUOUS`, before creating directories or copying originals. Existing GoPro joining and GPMF checks remain in force.
8. The UI marks these rows as Copy only and explains experimental discovery, unverified recording boundaries, and the lack of automatic joining in Japanese and English.

## Verification and remaining work

Automated synthetic cases cover model names for four generations, filename candidates, unrelated videos, actual GPMF flags, adjacent files and duplicate names, generic-ID collisions, byte-preserving copies, and rejecting DJI multi-file units. Synthetic metadata is not evidence of real-camera compatibility.

Before enabling DJI joining, obtain unedited consecutive split chapters and neighboring separate recordings for each generation and firmware under test. Establish recording identity, missing-chapter detection, and order from actual metadata. Verify video/audio packet payloads, all DJI telemetry/debug data and timing, color/HDR metadata, timecode, and downstream playback. Unknown or unverifiable metadata must prevent a joined output from being reported as complete.

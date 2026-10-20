# Osmo Pocket / DJI support status

## Scope

Osmo Pocket generations and DJI-named MP4s are discovered experimentally. TakeBinder can losslessly join a conservative subset of **Osmo Pocket 4** long-recording splits validated with real camera footage. Other Pocket generations, renamed files, and generic DJI candidates remain individual byte-for-byte copies.

## Evidence (2026-10-18)

Three unedited Pocket 4 chapters from one 1 h 52 min recording were inspected and joined: two approximately 16 GiB splits and one remainder. Each input contains HEVC, two AAC tracks, `djmd`, `dbgi`, `tmcd`, and an attached MJPEG track. The format encoder is `DJI OsmoPocket4`; counters, filename timestamps, container creation times, and timecodes are continuous. Gyroflow `mp4-merge` 0.1.11 produced a 43.7 GB output with all seven tracks and the expected duration. TakeBinder compares every concatenated packet payload, order, count, and byte total for HEVC, both AAC tracks, `djmd`, and `dbgi`. mp4-merge retains the first chapter's attached MJPEG thumbnail. For `tmcd`, TakeBinder preserves the first chapter's four-byte SMPTE start payload and extends that sample's MP4 duration across the joined video after confirming chapter timecode continuity.

This evidence covers the supplied Pocket 4 sample only. Other firmware, modes, resolutions, frame rates, HDR profiles, naming settings, and Pocket generations are not claimed compatible.

Research sources:

- [DJI: Osmo Action / Osmo Pocket series video split logic](https://repair.dji.com/help/content?customId=01700006948&lang=en&paperDocType=ARTICLE&re=US&spaceId=17)
- [DJI: Pocket 3 user manual](https://dl.djicdn.com/downloads/DJI_Osmo_Pocket_3/UM/20250826/DJI_Osmo_Pocket_3_User_Manual_v1.0_en.pdf)
- [ExifTool DJI parser](https://github.com/exiftool/exiftool/blob/master/lib/Image/ExifTool/DJI.pm)
- [Gyroflow mp4-merge](https://github.com/gyroflow/mp4-merge), MIT or Apache-2.0

## Discovery, grouping, and safety contract

1. ffprobe must identify a video stream and the exact Pocket model through `model`, `camera_model`, QuickTime model, or `encoder`. The real Pocket 4 sample uses `encoder=DJI OsmoPocket4`.
2. Automatic joining requires the exact name `DJI_YYYYMMDDhhmmss_NNNN_D.MP4`, the same directory, sequential counters, identical complete stream layouts, a preceding file of at least 15 GiB, a next filename timestamp within three seconds of the preceding media end, and SMPTE drop-frame timecode continuity within two frames. Failure of any condition starts an independent copy unit; the app never guesses across a gap.
3. A joined unit has basis `DJI Pocket 4 split continuity` and high confidence. Generic `DJI_` names remain low-confidence `DJI file copy` units even if generic capture/chapter tags exist.
4. `mp4-merge` is invoked directly with an argument array, never through a shell. It rewrites MP4 sample tables without decoding or encoding media. Version 0.1.11 standalone binaries, URLs, sizes, and SHA-256 values are pinned in `backend/tools.go`; installers are never executed.
5. Output is written to a temporary file. ffprobe must confirm the complete stream layout, summed duration, and first chapter creation time. SHA-256 packet fingerprints must match the concatenated inputs for HEVC, both AAC tracks, `djmd`, and `dbgi`. The MJPEG and `tmcd` payloads must match the first chapter, and the extended `tmcd` duration must cover the joined video. Any mismatch returns `E_DJI_VERIFY`, deletes the temporary file, and does not create the final name.
6. Single files continue through atomic byte copy. Inputs are never changed, moved, or deleted, and existing outputs are never overwritten.
7. `gpmfVerification: all-tracks-verified` is retained in the existing protocol field for a successfully joined DJI group; it means all DJI media/data tracks and the continuous `tmcd` semantics were verified, not that DJI contains GoPro GPMF.

## Remaining validation

Collect consecutive splits and neighboring separate recordings for each additional generation, firmware, and recording mode before widening the rules. Playback interoperability should also be exercised in target editors. Unknown or unverifiable metadata must continue to prevent a joined output from being reported as complete.

# Quality and Testing

[日本語](quality-and-testing.ja.md)

## 1. Non-functional requirements

### NFR-01 Quality preservation

- Do not invoke video or audio encoders.
- Output video and audio codec information matches the input.
- Pass-through files have matching SHA-256 hashes when strict verification is enabled.

### NFR-02 Performance

- Do not block UI interaction while scanning a directory containing 10,000 files.
- Stream file copies and joins without loading entire videos into memory.
- Keep memory use approximately constant relative to configured parallelism.

### NFR-03 Safety

- Do not modify input.
- Do not implicitly overwrite output.
- Distinguish temporary files from completed files.
- Prevent path traversal, command injection, and unintended writes through symbolic links.
- Minimize Electron privileges.

### NFR-04 Supported environments

- Windows 11 x64
- Currently supported macOS versions (prioritize Apple Silicon; decide Intel support before distribution)
- Ubuntu LTS x64

Official OS/CPU support is limited to combinations for which CI and physical-device test environments are available.

### NFR-05 Supply-chain safety

- Produce reproducible immutable installs from the same `yarn.lock`.
- Do not implicitly run install-time scripts from Yarn dependencies.
- Do not track mutable references for third-party CI actions, Go dependencies, or managed binaries.
- Do not provide release secrets to routine PR verification.
- Make dependency and managed-binary versions and checksums traceable from release artifacts.

## 2. Acceptance criteria

### AC-01 Multiple chapters

Given three GoPro MP4 chapters from the same capture, one correctly ordered MP4 is generated without video/audio re-encoding and with GPMF `GYRO` present.

### AC-02 Single file

A one-file capture group is copied without passing through media tools. Input and output SHA-256 hashes match when strict verification is enabled.

### AC-03 Parallel processing

Given four independent capture groups and maximum parallelism of two, no more than two groups run simultaneously and chapter order is preserved within each group.

### AC-04 Partial failure

If one group is corrupt, other independent groups complete and the result report records success and failure separately.

### AC-05 Input protection

Input contents, names, and locations remain unchanged after success, failure, or cancellation.

### AC-06 GPMF-loss prevention

If GPMF exists in input but is absent from output, the application does not display success and reports a verification failure.

### AC-07 Yarn supply-chain protection

CI fails when the lockfile and manifest disagree, and dependency install-time scripts are not run by default. Release artifacts include artifact checksums and managed-tool versions and checksums. Yarn-compatible SBOM generation is added after selection.

### AC-08 Output organization by capture time

When date directories and original copying are enabled, completed videos and source files under `original` are generated in the specified date directory (default `{YYYY}-{MM}-{DD}`), while input contents, names, and locations remain unchanged. Custom brace-token formats expand correctly and unsafe formats are rejected. The completed video's container capture time and file modification time match the first chapter's capture time; creation time also matches on supported operating systems. Reports record the timestamp source and fields that could not be set.

## 3. Test strategy

- Unit-test Go grouping, ordering, naming, and collision avoidance.
- Test capture-time source precedence, date-directory organization, `original` copies, and timestamp preservation on each OS.
- Test the JSON Lines protocol contract.
- Launch the real backend from Node and verify valid responses as well as continued communication after malformed JSON, protocol mismatch, and unknown commands.
- Verify that the backend rejects Renderer-modified input paths, unscanned groups, and different output locations.
- Verify that disabling reports produces no JSON report.
- Join samples covering H.264/H.265, standard color/HDR, and different GoPro generations.
- Compare SHA-256 for every input/output packet and verify GPMF keys, time ranges, packet counts, and payloads.
- Test output in major downstream applications such as Gyroflow on physical systems.
- Build in Windows, macOS, and Linux CI and run smoke tests on each OS.
- Run full CI for pull requests and release builds. Do not duplicate ordinary CI after merging to `main`; require up-to-date PR status checks through branch protection. Keep the main-push Release Please job for release orchestration.
- On every CI OS, assemble the Electron runtime, UI, and same-OS Go backend in a distribution directory, then launch the packaged application with `--smoke-test`.
- Test thousands of files, long recordings, low disk space, cancellation, and abnormal termination.
- Detect `package.json`/lockfile inconsistencies, unapproved install-time scripts, and high/critical vulnerabilities in CI.
- Review that GitHub Actions use full commit SHAs and workflows declare permissions explicitly.
- Verify the SBOM and checksums of every distributed binary at release time.

## 4. Implementation phases

1. Technical validation: verify FFmpeg stream copying, ffprobe, and GPMF-verification methods with real GoPro footage.
2. Go CLI: implement scanning, grouping, joining, copying, verification, and reports.
3. Electron MVP: implement directory selection, scan results, execution, progress, and cancellation.
4. Cross-platform distribution: assemble OS-specific binaries, sign, and package.
5. Physical compatibility testing: verify camera generations, codecs, HDR, and long recordings.

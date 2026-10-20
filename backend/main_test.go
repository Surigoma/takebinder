package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestConcatEntry(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{"spaces", `C:\GoPro clips\GX010001.MP4`, "file '" + filepath.ToSlash(`C:\GoPro clips\GX010001.MP4`) + "'\n", false},
		{"apostrophe", `/media/camera's/GX010001.MP4`, "file '/media/camera'\\''s/GX010001.MP4'\n", false},
		{"line break", "bad\npath.mp4", "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := concatEntry(videoFile{Path: test.path, Name: filepath.Base(test.path)})
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("got %q, err=%v; want %q, wantErr=%t", got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestStatusToolContractUsesFFmpeg(t *testing.T) {
	var output bytes.Buffer
	a := &app{out: json.NewEncoder(&output)}
	a.handleStatus("status-test")
	var got struct {
		Payload struct {
			Tools map[string]json.RawMessage `json:"tools"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Payload.Tools["ffmpeg"]; !ok {
		t.Fatal("status response has no ffmpeg tool")
	}
	if _, ok := got.Payload.Tools["mp4_merge"]; !ok {
		t.Fatal("status response has no mp4_merge tool")
	}
	if _, ok := got.Payload.Tools["mp4box"]; ok {
		t.Fatal("status response still exposes mp4box")
	}
}

func TestToolDownloadURLPolicy(t *testing.T) {
	for _, raw := range []string{
		"http://github.com/owner/file",
		"https://github.com.evil.example/file",
		"https://user:pass@github.com/file",
	} {
		if err := validateAssetURL(raw); err == nil {
			t.Fatalf("expected URL to be rejected: %s", raw)
		}
	}
	if err := validateAssetURL("https://github.com/owner/file"); err != nil {
		t.Fatalf("expected pinned host to be accepted: %v", err)
	}
}

func TestFileMatchesAsset(t *testing.T) {
	content := []byte("verified tool")
	digest := sha256.Sum256(content)
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
	asset := toolAsset{Size: int64(len(content)), SHA256: hex.EncodeToString(digest[:])}
	valid, err := fileMatchesAsset(path, asset)
	if err != nil || !valid {
		t.Fatalf("expected valid asset, got valid=%t err=%v", valid, err)
	}
	asset.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	valid, err = fileMatchesAsset(path, asset)
	if err == nil || valid {
		t.Fatalf("expected hash mismatch, got valid=%t err=%v", valid, err)
	}
}

func TestParseGoProName(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		chapter int
		ok      bool
	}{
		{"GX010123.MP4", "modern-x-0123", 1, true},
		{"GH020123.mp4", "modern-h-0123", 2, true},
		{"GOPR1234.MP4", "legacy-1234", 0, true},
		{"GP011234.MP4", "legacy-1234", 1, true},
		{"holiday.mp4", "", 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, chapter, ok := parseGoProName(test.name)
			if key != test.key || chapter != test.chapter || ok != test.ok {
				t.Fatalf("got (%q, %d, %t)", key, chapter, ok)
			}
		})
	}
}

func TestMediaClassificationAndGroupingIdentity(t *testing.T) {
	goPro := probeResult{
		Format:  probeFormat{Tags: map[string]string{"firmware": "H21.01.01.62.00", "media_id": "Capture-42", "chapter": "3"}},
		Streams: []probeStream{{CodecType: "video", Tags: map[string]string{"handler_name": "GoPro H.265"}}, {CodecType: "data", CodecTagString: "gpmd"}},
	}
	classification, gpmf := classifyMedia(goPro, "clip.mp4")
	if classification != "gopro-gpmf" || !gpmf {
		t.Fatalf("got %s, gpmf=%t", classification, gpmf)
	}
	key, chapter, basis, confidence, ok := groupingIdentity(goPro, "clip.mp4")
	if !ok || key != "metadata-capture-42" || chapter != 3 || basis != "Media/Capture ID" || confidence != "high" {
		t.Fatalf("got (%q, %d, %q, %q, %t)", key, chapter, basis, confidence, ok)
	}
	withoutGPMF := goPro
	withoutGPMF.Streams = withoutGPMF.Streams[:1]
	if classification, _ := classifyMedia(withoutGPMF, "GX010001.MP4"); classification != "gopro-no-gpmf" {
		t.Fatalf("got %s", classification)
	}
	notGoPro := probeResult{Streams: []probeStream{{CodecType: "video"}}}
	if classification, _ := classifyMedia(notGoPro, "holiday.mp4"); classification != "not-gopro" {
		t.Fatalf("got %s", classification)
	}
}

func TestValidateChapters(t *testing.T) {
	tests := []struct {
		name   string
		files  []videoFile
		status string
	}{
		{"modern complete", []videoFile{{Name: "GX010001.MP4", Chapter: 1}, {Name: "GX020001.MP4", Chapter: 2}}, "ready"},
		{"legacy complete", []videoFile{{Name: "GOPR0001.MP4", Chapter: 0}, {Name: "GP010001.MP4", Chapter: 1}}, "ready"},
		{"missing first", []videoFile{{Name: "GX020001.MP4", Chapter: 2}}, "ambiguous"},
		{"gap", []videoFile{{Name: "GX010001.MP4", Chapter: 1}, {Name: "GX030001.MP4", Chapter: 3}}, "ambiguous"},
		{"time reversal", []videoFile{{Name: "GX010001.MP4", Chapter: 1, Captured: "2026-08-11T02:00:00Z"}, {Name: "GX020001.MP4", Chapter: 2, Captured: "2026-08-11T01:00:00Z"}}, "ambiguous"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, _ := validateChapters(test.files)
			if status != test.status {
				t.Fatalf("got %s, want %s", status, test.status)
			}
		})
	}
}

func TestCopyAtomic(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mp4")
	target := filepath.Join(dir, "target.mp4")
	content := []byte("gopro-test-data")
	if err := os.WriteFile(source, content, 0o644); err != nil {
		t.Fatal(err)
	}
	lastProgress := 0.0
	hash, err := copyAtomic(context.Background(), source, target, true, func(percent float64) { lastProgress = percent })
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("expected hash")
	}
	if lastProgress != 100 {
		t.Fatalf("progress ended at %v", lastProgress)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("copied content differs: %q", got)
	}
}

func TestCopyAtomicCancellationLeavesNoOutput(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.mp4"), filepath.Join(dir, "target.mp4")
	if err := os.WriteFile(source, bytes.Repeat([]byte("x"), 8*1024*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := copyAtomic(ctx, source, target, true, func(float64) {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled output exists: %v", err)
	}
	partials, _ := filepath.Glob(filepath.Join(dir, ".takebinder-*.partial"))
	if len(partials) != 0 {
		t.Fatalf("partial outputs remain: %v", partials)
	}
}

func TestMediaProgress(t *testing.T) {
	tests := []struct {
		name      string
		seconds   float64
		durations []float64
		want      float64
	}{
		{"first chapter", 5, []float64{10, 20}, 95.0 / 6},
		{"second chapter", 15, []float64{10, 20}, 47.5},
		{"complete capped for verification", 30, []float64{10, 20}, 95},
		{"missing durations", 5, []float64{0, 0}, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := mediaProgress(test.seconds, test.durations)
			if math.Abs(got-test.want) > 0.001 {
				t.Fatalf("got %.3f", got)
			}
		})
	}
}

func TestGPMFPacketCount(t *testing.T) {
	probe := probeResult{Streams: []probeStream{{CodecTagString: "gpmd", ReadPackets: "42"}}}
	count, ok := gpmfPackets(probe)
	if !ok || count != 42 {
		t.Fatalf("got (%d, %t)", count, ok)
	}
}

func TestParsePacketInfo(t *testing.T) {
	line := "stream_index=3|pts_time=1.250000|duration_time=0.500000|size=42|data_hash=SHA256:" + strings.Repeat("a", 64)
	packet, err := parsePacketInfo(line)
	if err != nil || packet.streamIndex != 3 || packet.size != 42 || packet.pts != 1.25 || packet.duration != 0.5 || !packet.hasTiming {
		t.Fatalf("got %#v, err=%v", packet, err)
	}
	if _, err := parsePacketInfo("stream_index=3|size=42"); err == nil {
		t.Fatal("missing packet hash was accepted")
	}
}

func TestGPMFVerificationHelpers(t *testing.T) {
	keys, err := findGPMFMajorKeys(iotest.OneByteReader(strings.NewReader("xxGYROyyACCLzzGPS5")))
	if err != nil || !keys["GYRO"] || !keys["ACCL"] || !keys["GPS5"] {
		t.Fatalf("got keys=%v err=%v", keys, err)
	}
	layout := []probeStream{{CodecType: "video"}, {CodecType: "data", CodecTagString: "gpmd"}}
	fingerprints := newPacketFingerprints(2)
	fingerprints[0].firstPTS, fingerprints[0].endPTS, fingerprints[0].hasTiming = 0, 10, true
	fingerprints[1].firstPTS, fingerprints[1].endPTS, fingerprints[1].hasTiming = 0.5, 9.5, true
	if !gpmfCoversVideo(layout, fingerprints) {
		t.Fatal("expected gpmd range within boundary tolerance to cover video")
	}
	fingerprints[1].endPTS = 8
	if gpmfCoversVideo(layout, fingerprints) {
		t.Fatal("short gpmd range was accepted")
	}
	inputs := []probeResult{{Format: probeFormat{Duration: "5"}}, {Format: probeFormat{Duration: "5"}}}
	if !durationMatches(inputs, probeResult{Format: probeFormat{Duration: "10.2"}}) || durationMatches(inputs, probeResult{Format: probeFormat{Duration: "11"}}) {
		t.Fatal("duration tolerance is incorrect")
	}
	expected, actual := newPacketFingerprints(1), newPacketFingerprints(1)
	for _, fingerprint := range []*packetFingerprint{&expected[0], &actual[0]} {
		fingerprint.count, fingerprint.bytes = 1, 4
		_, _ = fingerprint.hash.Write([]byte("packet"))
	}
	if !samePacketFingerprint(expected[0], actual[0]) {
		t.Fatal("identical packet fingerprints did not match")
	}
	actual[0].bytes++
	if samePacketFingerprint(expected[0], actual[0]) {
		t.Fatal("altered packet fingerprint was accepted")
	}
}

func TestRealGoProPacketVerification(t *testing.T) {
	rawInputs, output := os.Getenv("TAKEBINDER_SAMPLE_INPUTS"), os.Getenv("TAKEBINDER_SAMPLE_OUTPUT")
	if rawInputs == "" || output == "" {
		t.Skip("set TAKEBINDER_SAMPLE_INPUTS and TAKEBINDER_SAMPLE_OUTPUT to run")
	}
	ffmpeg, err := findTool("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	ffprobe, err := findTool("ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	paths := filepath.SplitList(rawInputs)
	files, probes := make([]videoFile, len(paths)), make([]probeResult, len(paths))
	for index, path := range paths {
		files[index] = videoFile{Path: path, Name: filepath.Base(path)}
		probes[index], err = probeFile(context.Background(), ffprobe, path)
		if err != nil {
			t.Fatal(err)
		}
	}
	outputProbe, err := probeFile(context.Background(), ffprobe, output)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyPacketPayloads(context.Background(), ffprobe, ffmpeg, files, probes, output, outputProbe); err != nil {
		t.Fatal(err)
	}
}

func TestVideoDetails(t *testing.T) {
	tests := []struct {
		name         string
		probe        probeResult
		wantCaptured string
		wantDuration float64
		wantWidth    int
		wantHeight   int
	}{
		{
			name: "format metadata",
			probe: probeResult{
				Format:  probeFormat{Duration: "65.5", Tags: map[string]string{"creation_time": "2026-08-11T01:02:03.456Z"}},
				Streams: []probeStream{{CodecType: "video", Width: 3840, Height: 2160}},
			},
			wantCaptured: "2026-08-11T01:02:03.456Z", wantDuration: 65.5, wantWidth: 3840, wantHeight: 2160,
		},
		{
			name:         "file timestamp fallback",
			probe:        probeResult{Streams: []probeStream{{CodecType: "video", Width: 1920, Height: 1080}}},
			wantCaptured: "2026-08-10T00:00:00Z", wantWidth: 1920, wantHeight: 1080,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			captured, duration, width, height := videoDetails(test.probe, "2026-08-10T00:00:00Z")
			if captured != test.wantCaptured || duration != test.wantDuration || width != test.wantWidth || height != test.wantHeight {
				t.Fatalf("got (%q, %v, %d, %d)", captured, duration, width, height)
			}
		})
	}
}

func TestOutputTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	captured := "2026-08-09T22:23:28.123Z"
	warnings := applyOutputTimestamp(path, captured)
	if runtime.GOOS == "windows" && len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := time.Parse(time.RFC3339Nano, captured)
	if math.Abs(info.ModTime().Sub(want).Seconds()) > 0.01 {
		t.Fatalf("got %s, want %s", info.ModTime(), want)
	}
	probe := probeResult{Format: probeFormat{Tags: map[string]string{"creation_time": "2026-08-09T22:23:28.123000Z"}}}
	if !creationTimeMatches(probe, captured) || creationTimeMatches(probe, "2026-08-09T22:24:00Z") {
		t.Fatal("creation_time comparison is incorrect")
	}
}

func TestDateFolderName(t *testing.T) {
	tests := []struct {
		name     string
		captured string
		format   string
		want     string
		wantErr  bool
	}{
		{"default", "2026-08-09T22:23:28Z", "", "2026-08-09", false},
		{"Japanese", "2026-08-09T22:23:28Z", "{YYYY}年{MM}月{DD}日", "2026年08月09日", false},
		{"prefix", "2026-08-09T22:23:28Z", "GoPro_{YYYY}{MM}{DD}", "GoPro_20260809", false},
		{"offset", "2026-08-10T01:02:03+09:00", "{YYYY}-{MM}-{DD}", "2026-08-10", false},
		{"bare token is literal", "2026-08-09T22:23:28Z", "YYYY_{MM}", "YYYY_08", false},
		{"invalid timestamp", "", "{YYYY}-{MM}-{DD}", "", true},
		{"path separator", "2026-08-09T22:23:28Z", "{YYYY}/{MM}/{DD}", "", true},
		{"no token", "2026-08-09T22:23:28Z", "videos", "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := dateFolderName(test.captured, test.format)
			if got != test.want || (err != nil) != test.wantErr {
				t.Fatalf("got %q, err=%v", got, err)
			}
		})
	}
}

func TestProcessGroupUsesDailyFolder(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	output := filepath.Join(root, "output")
	if err := os.WriteFile(source, []byte("video bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := group{
		ID: "daily", Status: "ready", OutputName: "joined.mp4",
		Files: []videoFile{{Path: source, Name: "source.mp4", Captured: "2026-08-09T22:23:28Z"}},
	}
	result := (&app{}).processGroup(context.Background(), output, g, false, true, "{YYYY}-{MM}-{DD}", false, false, func(jobProgress) {})
	want := filepath.Join(output, "2026-08-09", "joined.mp4")
	if result.Status != "completed" || result.OutputPath != want {
		t.Fatalf("got status=%s path=%q, want %q", result.Status, result.OutputPath, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
}

func TestProcessGroupCopiesOriginal(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "GX010001.MP4")
	output := filepath.Join(root, "output")
	content := []byte("original video bytes")
	if err := os.WriteFile(source, content, 0o644); err != nil {
		t.Fatal(err)
	}
	g := group{
		ID: "original", Status: "ready", OutputName: "joined.mp4", RelativeDir: "camera",
		Files: []videoFile{{Path: source, Name: filepath.Base(source), Size: int64(len(content)), Captured: "2026-08-09T22:23:28Z"}},
	}
	result := (&app{}).processGroup(context.Background(), output, g, false, true, "{YYYY}-{MM}-{DD}", true, true, func(jobProgress) {})
	wantOutput := filepath.Join(output, "2026-08-09", "camera", "joined.mp4")
	wantOriginal := filepath.Join(output, "2026-08-09", "original", filepath.Base(source))
	if result.Status != "completed" || result.OutputPath != wantOutput || len(result.Originals) != 1 || result.Originals[0] != wantOriginal {
		t.Fatalf("unexpected result: %#v", result)
	}
	for _, path := range []string{source, wantOutput, wantOriginal} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, content) {
			t.Fatalf("%s differs: %q, err=%v", path, got, err)
		}
	}
}

func TestDefaultOutputName(t *testing.T) {
	tests := []struct {
		name, captured, source, format, want string
		wantErr                              bool
	}{
		{"default", "2026-08-09T14:30:52Z", "GX012345.MP4", "", "2026-08-09_143052_GX012345.mp4", false},
		{"legacy default", "2026-08-09T14:30:52Z", "GX012345.MP4", "YYYY-MM-DD_hhmmss_NAME", "2026-08-09_143052_GX012345.mp4", false},
		{"custom", "2026-08-09T14:30:52+09:00", "GX012345.MP4", "{NAME}_{YYYY}{MM}{DD}_{hh}-{mm}-{ss}", "GX012345_20260809_14-30-52.mp4", false},
		{"literal token name", "2026-08-09T14:30:52Z", "GX012345.MP4", "literal_NAME_{NAME}", "literal_NAME_GX012345.mp4", false},
		{"bare token is literal", "2026-08-09T14:30:52Z", "GX012345.MP4", "NAME", "NAME.mp4", false},
		{"unsafe character replaced", "2026-08-09T14:30:52Z", "GX012345.MP4", "{NAME}:{hh}", "GX012345_14.mp4", false},
		{"optional extension", "2026-08-09T14:30:52Z", "GX012345.MP4", "{NAME}.mp4", "GX012345.mp4", false},
		{"reserved name", "2026-08-09T14:30:52Z", "GX012345.MP4", "CON", "_CON.mp4", false},
		{"path separator", "2026-08-09T14:30:52Z", "GX012345.MP4", "folder/{NAME}", "", true},
		{"invalid time", "", "GX012345.MP4", "", "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := defaultOutputName(test.captured, test.source, test.format)
			if got != test.want || (err != nil) != test.wantErr {
				t.Fatalf("got %q, err=%v", got, err)
			}
		})
	}
}

func TestProcessGroupPreservesSubfolder(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	output := filepath.Join(root, "output")
	if err := os.WriteFile(source, []byte("video bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := group{
		ID: "nested", Status: "ready", OutputName: "joined.mp4", RelativeDir: filepath.Join("camera", "day1"),
		Files: []videoFile{{Path: source, Name: "source.mp4", Captured: "2026-08-09T22:23:28Z"}},
	}
	result := (&app{}).processGroup(context.Background(), output, g, false, false, "", false, true, func(jobProgress) {})
	want := filepath.Join(output, "camera", "day1", "joined.mp4")
	if result.Status != "completed" || result.OutputPath != want {
		t.Fatalf("unexpected result: %#v", result)
	}

	g.RelativeDir = filepath.Join("..", "escape")
	result = (&app{}).processGroup(context.Background(), output, g, false, false, "", false, true, func(jobProgress) {})
	if result.Status != "failed" || result.Code != "E_BAD_REQUEST" {
		t.Fatalf("unsafe relative directory was accepted: %#v", result)
	}
}

func TestDiskSpaceEstimate(t *testing.T) {
	groups := []group{{Files: []videoFile{{Size: 100}}}}
	if got, want := estimatedRunBytes(groups, true), uint64(202+64*1024*1024); got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
	if available, err := availableDiskSpace(t.TempDir()); err != nil || available == 0 {
		t.Fatalf("available=%d err=%v", available, err)
	}
	if err := diskSpaceError(99, 100); err == nil {
		t.Fatal("low disk space was accepted")
	}
	if err := diskSpaceError(100, 100); err != nil {
		t.Fatalf("sufficient disk space was rejected: %v", err)
	}
}

func TestWriteReportCanBeDisabled(t *testing.T) {
	dir := t.TempDir()
	path, err := writeReportIfEnabled(dir, runReport{}, false)
	if err != nil || path != "" {
		t.Fatalf("got path=%q err=%v", path, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("report was created while disabled: %#v", entries)
	}
}

func TestRunReportIncludesExecutionDetails(t *testing.T) {
	started := time.Date(2026, 8, 11, 1, 2, 3, 0, time.UTC)
	finished := started.Add(time.Minute)
	captureGroup := group{
		ID: "capture", Basis: "Media/Capture ID", Confidence: "high",
		Files: []videoFile{{Name: "GX010001.MP4", Chapter: 1, Captured: "2026-08-11T01:02:03Z", Streams: []streamSummary{{Type: "video", Codec: "hevc"}}}},
	}
	report := buildRunReport(
		runRequest{InputDir: `C:\input`, OutputDir: `D:\output`, Groups: []group{captureGroup}},
		[]jobResult{{GroupID: "capture", Status: "completed", GPMFVerification: "verified", Warnings: []string{"timestamp warning"}}},
		started, finished,
	)
	if report.StartedAt != started.Format(time.RFC3339Nano) || report.FinishedAt != finished.Format(time.RFC3339Nano) || report.FileCount != 1 || report.GroupCount != 1 {
		t.Fatalf("unexpected report summary: %#v", report)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"GX010001.MP4", "Media/Capture ID", "hevc", "verified", "timestamp warning"} {
		if !bytes.Contains(data, []byte(value)) {
			t.Fatalf("report does not contain %q: %s", value, data)
		}
	}
}

func TestReusableResultsRequireMatchingInputsAndOutput(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "joined.mp4")
	if err := os.WriteFile(output, []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	captureGroup := group{ID: "capture", Files: []videoFile{{Path: filepath.Join(dir, "GX010001.MP4"), Size: 10, Captured: "2026-08-11T01:02:03Z", Chapter: 1}}}
	outputInfo, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	report := runReport{Results: []reportResult{{Group: captureGroup, Result: jobResult{GroupID: captureGroup.ID, Status: "completed", OutputPath: output, OutputSize: 4, OutputModified: outputInfo.ModTime().UTC().Format(time.RFC3339Nano)}}}}
	if !safeCompletedOutput(dir, report.Results[0].Result) {
		t.Fatalf("valid completed output was rejected: %#v", report.Results[0].Result)
	}
	if _, err := writeReport(dir, report); err != nil {
		t.Fatal(err)
	}
	pending, reused := reusableResults(dir, []group{captureGroup})
	if len(pending) != 0 || len(reused) != 1 || reused[0].Status != "skipped" {
		t.Fatalf("expected reuse, pending=%#v reused=%#v", pending, reused)
	}
	if err := os.WriteFile(output, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	pending, reused = reusableResults(dir, []group{captureGroup})
	if len(pending) != 1 || len(reused) != 0 {
		t.Fatalf("changed output was reused, pending=%#v reused=%#v", pending, reused)
	}
}

func TestScanFolderGroupsAndDetectsGap(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"GX010123.MP4", "GX020123.MP4", "GX010999.MP4", "GX030999.MP4", "notes.mp4"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	progressCount := 0
	result, err := scanFolderWithInspectorProgress(scanRequest{InputDir: dir, Recursive: true}, func(_ context.Context, path string) (probeResult, error) {
		if filepath.Base(path) == "notes.mp4" {
			return probeResult{Streams: []probeStream{{CodecType: "video"}}}, nil
		}
		return probeResult{
			Format:  probeFormat{Duration: "1", Tags: map[string]string{"firmware": "test"}},
			Streams: []probeStream{{CodecType: "video", Width: 1920, Height: 1080}, {CodecType: "data", CodecTagString: "gpmd"}},
		}, nil
	}, func(scanned int, _ string) { progressCount = scanned })
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 2 || len(result.Skipped) != 1 {
		t.Fatalf("got %d groups and %d skipped", len(result.Groups), len(result.Skipped))
	}
	if progressCount != 5 {
		t.Fatalf("got %d progress updates", progressCount)
	}
	for _, captureGroup := range result.Groups {
		for _, file := range captureGroup.Files {
			if file.Size != int64(len(file.Name)) {
				t.Fatalf("input size was not recorded for %s: %d", file.Name, file.Size)
			}
		}
	}
	statuses := map[string]int{}
	for _, group := range result.Groups {
		statuses[group.Status]++
	}
	if statuses["ready"] != 1 || statuses["ambiguous"] != 1 {
		t.Fatalf("unexpected statuses: %#v", statuses)
	}
	for _, group := range result.Groups {
		if !strings.HasSuffix(group.OutputName, "_"+strings.TrimSuffix(group.Files[0].Name, filepath.Ext(group.Files[0].Name))+".mp4") {
			t.Fatalf("unexpected output name: %q", group.OutputName)
		}
	}
}

func TestUniqueOutputReservesConcurrentNames(t *testing.T) {
	a := &app{}
	dir := t.TempDir()
	first, err := a.uniqueOutput(dir, "GX010001.MP4")
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.uniqueOutput(dir, "GX010001.MP4")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Base(second) != "GX010001_2.MP4" {
		t.Fatalf("unexpected reserved outputs: %s, %s", first, second)
	}
}

func TestAuthorizeRunUsesLatestScanPlan(t *testing.T) {
	input, output := t.TempDir(), t.TempDir()
	canonicalInput, err := canonicalDirectory(input)
	if err != nil {
		t.Fatal(err)
	}
	canonicalOutput, err := canonicalDirectory(output)
	if err != nil {
		t.Fatal(err)
	}
	canonical := group{ID: "safe", Status: "ready", Files: []videoFile{{Path: filepath.Join(canonicalInput, "safe.mp4")}}}
	a := &app{plan: &scanPlan{InputDir: canonicalInput, OutputDir: canonicalOutput, Groups: map[string]group{"safe": canonical}}}
	req, err := a.authorizeRun(runRequest{OutputDir: output, Groups: []group{{ID: "safe", Files: []videoFile{{Path: `C:\sensitive.txt`}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if req.Groups[0].Files[0].Path != canonical.Files[0].Path {
		t.Fatalf("renderer path was trusted: %q", req.Groups[0].Files[0].Path)
	}
	if _, err := a.authorizeRun(runRequest{OutputDir: output, Groups: []group{{ID: "unknown"}}}); err == nil {
		t.Fatal("unknown group was authorized")
	}
	if _, err := a.authorizeRun(runRequest{OutputDir: input, Groups: []group{{ID: "safe"}}}); err == nil {
		t.Fatal("different output directory was authorized")
	}
}

func TestDefaultParallel(t *testing.T) {
	tests := []struct {
		cpu, groups int
		sameDrive   bool
		want        int
	}{
		{16, 10, false, 4},
		{16, 3, false, 3},
		{16, 10, true, 2},
		{2, 10, false, 1},
		{16, 0, false, 1},
	}
	for _, test := range tests {
		if got := defaultParallel(test.cpu, test.groups, test.sameDrive); got != test.want {
			t.Fatalf("defaultParallel(%d, %d, %t)=%d, want %d", test.cpu, test.groups, test.sameDrive, got, test.want)
		}
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDJIClassification(t *testing.T) {
	tests := []struct {
		name, filename, classification string
		tags                           map[string]string
	}{
		{"original", "renamed.mp4", "osmo-pocket", map[string]string{"model": "Osmo Pocket"}},
		{"pocket 2", "renamed.mp4", "osmo-pocket", map[string]string{"camera_model": "DJI Pocket 2"}},
		{"pocket 3", "renamed.mp4", "osmo-pocket", map[string]string{"com.apple.quicktime.model": "Osmo Pocket 3"}},
		{"pocket 4", "renamed.mp4", "osmo-pocket", map[string]string{"Model": "DJI Osmo Pocket 4", "firmware": "test"}},
		{"pocket 4 encoder", "renamed.mp4", "osmo-pocket", map[string]string{"encoder": "DJI OsmoPocket4"}},
		{"legacy hint", "DJI_0001.MP4", "dji-candidate", nil},
		{"modern hint", "dji_20261018123000_0001_d.mp4", "dji-candidate", nil},
		{"manufacturer", "custom.mp4", "dji-candidate", map[string]string{"make": "DJI", "model": "Osmo Action 4"}},
		{"not a confirmed model", "DJI_0001.MP4", "dji-candidate", map[string]string{"model": "Osmo Pocket 40"}},
		{"unrelated", "holiday.mp4", "not-gopro", nil},
		{"other pocket camera", "holiday.mp4", "not-gopro", map[string]string{"model": "Blackmagic Pocket Cinema Camera"}},
		{"lookalike prefix", "DJI_holiday!.mp4", "not-gopro", nil},
		{"LRF excluded", "DJI_0001.LRF", "not-gopro", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := probeResult{Format: probeFormat{Tags: test.tags}, Streams: []probeStream{{CodecType: "video"}}}
			got, gpmf := classifyMedia(probe, test.filename)
			if got != test.classification || gpmf {
				t.Fatalf("got %q, gpmf=%t", got, gpmf)
			}
		})
	}
	probe := probeResult{Streams: []probeStream{{CodecType: "video", Tags: map[string]string{"model": "Osmo Pocket 4"}}, {CodecType: "data", CodecTagString: "gpmd"}}}
	if got, gpmf := classifyMedia(probe, "custom.mp4"); got != "osmo-pocket" || !gpmf {
		t.Fatalf("stream model/GPMF classification: %q, %t", got, gpmf)
	}
}

func TestPocket4SplitGroupingRequiresContinuity(t *testing.T) {
	probe := probeResult{
		Format:  probeFormat{Tags: map[string]string{"encoder": "DJI OsmoPocket4"}},
		Streams: []probeStream{{CodecType: "video", CodecName: "hevc", CodecTagString: "hvc1", FrameRate: "60000/1001"}, {CodecType: "data", CodecTagString: "djmd"}},
	}
	makeCandidate := func(name, timecode string, size int64, duration float64) groupCandidate {
		candidateProbe := probe
		candidateProbe.Format.Tags = map[string]string{"encoder": "DJI OsmoPocket4", "timecode": timecode}
		return groupCandidate{probe: candidateProbe, file: videoFile{Path: filepath.Join("C:\\clips", name), Name: name, Size: size, Duration: duration, Classification: "osmo-pocket"}}
	}
	first := makeCandidate("DJI_20260824210407_0030_D.MP4", "21:04:36;30", 17190133305, 2650.090667)
	second := makeCandidate("DJI_20260824214819_0031_D.MP4", "21:48:46;36", 17189903106, 2647.338667)
	third := makeCandidate("DJI_20260824223226_0032_D.MP4", "22:32:53;52", 9289912204, 1449.92)
	grouped := map[string][]groupCandidate{"a": {first}, "b": {second}, "c": {third}}
	regroupPocket4Candidates(grouped)
	if len(grouped) != 1 {
		t.Fatalf("got %d groups, want one: %#v", len(grouped), grouped)
	}
	for _, chain := range grouped {
		if len(chain) != 3 || chain[0].basis != "DJI Pocket 4 split continuity" || chain[2].file.Chapter != 3 {
			t.Fatalf("unexpected chain: %#v", chain)
		}
	}

	broken := map[string][]groupCandidate{"a": {first}, "c": {third}}
	regroupPocket4Candidates(broken)
	if len(broken) != 2 {
		t.Fatalf("counter gap was grouped: %#v", broken)
	}
	short := first
	short.file.Size = 1024
	broken = map[string][]groupCandidate{"a": {short}, "b": {second}}
	regroupPocket4Candidates(broken)
	if len(broken) != 2 {
		t.Fatalf("non-split-sized predecessor was grouped: %#v", broken)
	}
	pocket3First, pocket3Second := first, second
	pocket3First.probe.Format.Tags["encoder"] = "DJI OsmoPocket3"
	pocket3Second.probe.Format.Tags["encoder"] = "DJI OsmoPocket3"
	broken = map[string][]groupCandidate{"a": {pocket3First}, "b": {pocket3Second}}
	regroupPocket4Candidates(broken)
	if len(broken) != 2 {
		t.Fatalf("Pocket 3 was grouped by Pocket 4 rules: %#v", broken)
	}
}

func TestRealPocket4PacketVerification(t *testing.T) {
	rawInputs, output := os.Getenv("TAKEBINDER_DJI_SAMPLE_INPUTS"), os.Getenv("TAKEBINDER_DJI_SAMPLE_OUTPUT")
	if rawInputs == "" || output == "" {
		t.Skip("set TAKEBINDER_DJI_SAMPLE_INPUTS and TAKEBINDER_DJI_SAMPLE_OUTPUT to run")
	}
	ffprobe, err := findTool("ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	paths := filepath.SplitList(rawInputs)
	files, probes := make([]videoFile, len(paths)), make([]probeResult, len(paths))
	for index, path := range paths {
		files[index] = videoFile{Path: path, Name: filepath.Base(path)}
		probes[index], err = inspectFile(context.Background(), ffprobe, path)
		if err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("TAKEBINDER_DJI_EXTEND_TIMECODE") == "1" {
		if err := extendMergedTimecode(output, files); err != nil {
			t.Fatal(err)
		}
	}
	outputProbe, err := inspectFile(context.Background(), ffprobe, output)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyAllPacketPayloads(context.Background(), ffprobe, files, probes, output, outputProbe); err != nil {
		t.Fatal(err)
	}
}

func TestDJIScanCopiesFilesIndependently(t *testing.T) {
	input, output := t.TempDir(), t.TempDir()
	// Includes adjacent counters, equal timestamps, a repeated filename in a
	// different directory, a renamed file, and unsupported sidecars.
	names := []string{"DJI_0001.MP4", "DJI_0002.MP4", "other/DJI_0001.MP4", "renamed.mp4", "DJI_0001.LRF", "DJI_0001.SRT", "holiday.mp4"}
	contents := []byte("video\x00audio\x00djmd\x00dbgi\x00unknown metadata")
	for _, name := range names {
		path := filepath.Join(input, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result, err := scanFolderWithInspector(scanRequest{InputDir: input, OutputDir: output, Recursive: true}, func(_ context.Context, path string) (probeResult, error) {
		tags := map[string]string{"creation_time": "2026-10-18T12:30:00Z", "capture_id": "shared", "chapter": "2"}
		if filepath.Base(path) == "renamed.mp4" {
			tags["model"] = "Osmo Pocket 4"
		}
		return probeResult{Format: probeFormat{Tags: tags, Duration: "1"}, Streams: []probeStream{{CodecType: "video", CodecName: "hevc"}, {CodecType: "data", CodecTagString: "djmd"}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 4 || len(result.Skipped) != 1 || result.Skipped[0].Classification != "not-gopro" {
		t.Fatalf("unexpected discovery: %#v", result)
	}
	a := &app{reserved: map[string]bool{}}
	ids := map[string]bool{}
	for _, g := range result.Groups {
		if len(g.Files) != 1 || g.HasGPMF || g.Status != "ready" || g.Basis != "DJI file copy" || g.Confidence != "low" || ids[g.ID] {
			t.Fatalf("unsafe file group: %#v", g)
		}
		ids[g.ID] = true
		job := a.processGroup(context.Background(), output, g, true, true, "{YYYY}-{MM}-{DD}", false, true, func(jobProgress) {})
		if job.Status != "completed" || job.SHA256 == "" || job.GPMFVerification != "byte-identical" {
			t.Fatalf("copy failed: %#v", job)
		}
		copied, err := os.ReadFile(job.OutputPath)
		if err != nil || !bytes.Equal(copied, contents) {
			t.Fatalf("copy lost payload or metadata: %v", err)
		}
		source, err := os.ReadFile(g.Files[0].Path)
		if err != nil || !bytes.Equal(source, contents) {
			t.Fatalf("input changed: %v", err)
		}
	}
	// Verify the existing JSON contract carries the new classifications and
	// accurate GPMF absence without adding protocol fields.
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded scanResult
	if err := json.Unmarshal(encoded, &decoded); err != nil || len(decoded.Groups) != 4 || decoded.Groups[0].HasGPMF {
		t.Fatalf("scan contract failed: %s, %v", encoded, err)
	}
}

func TestDJIMultiFileProcessingRejectedBeforeWriting(t *testing.T) {
	for _, classification := range []string{"osmo-pocket", "dji-candidate"} {
		t.Run(classification, func(t *testing.T) {
			dir := t.TempDir()
			g := group{ID: "dji", Status: "ready", Files: []videoFile{{Classification: "gopro-gpmf"}, {Classification: classification}}}
			a := &app{}
			result := a.processGroup(context.Background(), dir, g, true, true, "{YYYY}", true, true, func(jobProgress) {})
			if result.Code != "E_GROUP_AMBIGUOUS" || result.Status != "failed" {
				t.Fatalf("DJI join was not rejected: %#v", result)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("rejected join wrote output: %v, %v", entries, err)
			}
		})
	}
}

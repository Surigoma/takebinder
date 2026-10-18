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

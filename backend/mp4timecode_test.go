package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestExtendMergedTimecode(t *testing.T) {
	directory := t.TempDir()
	inputDeltas := []uint32{158998840, 158833675, 86993907}
	inputs := make([]videoFile, len(inputDeltas))
	inputContents := make([][]byte, len(inputDeltas))
	for index, delta := range inputDeltas {
		path := filepath.Join(directory, string(rune('a'+index))+".mp4")
		content := syntheticTimecodeMP4(delta)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
		inputs[index] = videoFile{Path: path, Name: filepath.Base(path)}
		inputContents[index] = content
	}
	output := filepath.Join(directory, "joined.mp4")
	if err := os.WriteFile(output, syntheticTimecodeMP4(inputDeltas[0]), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extendMergedTimecode(output, inputs); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	table, err := findTimecodeTable(file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if table.sampleCount != 1 || table.delta != 404826422 {
		t.Fatalf("unexpected output timecode table: %#v", table)
	}
	for index, input := range inputs {
		content, err := os.ReadFile(input.Path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(content, inputContents[index]) {
			t.Fatalf("input %d was modified", index)
		}
	}
}

func TestExtendMergedTimecodeRejectsOverflow(t *testing.T) {
	directory := t.TempDir()
	inputs := make([]videoFile, 2)
	for index, delta := range []uint32{^uint32(0), 1} {
		path := filepath.Join(directory, string(rune('a'+index))+".mp4")
		if err := os.WriteFile(path, syntheticTimecodeMP4(delta), 0o644); err != nil {
			t.Fatal(err)
		}
		inputs[index] = videoFile{Path: path, Name: filepath.Base(path)}
	}
	output := filepath.Join(directory, "joined.mp4")
	if err := os.WriteFile(output, syntheticTimecodeMP4(^uint32(0)), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := extendMergedTimecode(output, inputs); err == nil {
		t.Fatal("expected overflow error")
	}
	after, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("output changed after rejected extension")
	}
}

func syntheticTimecodeMP4(delta uint32) []byte {
	stsdPayload := make([]byte, 16)
	binary.BigEndian.PutUint32(stsdPayload[4:8], 1)
	binary.BigEndian.PutUint32(stsdPayload[8:12], 8)
	copy(stsdPayload[12:16], "tmcd")
	sttsPayload := make([]byte, 16)
	binary.BigEndian.PutUint32(sttsPayload[4:8], 1)
	binary.BigEndian.PutUint32(sttsPayload[8:12], 1)
	binary.BigEndian.PutUint32(sttsPayload[12:16], delta)
	return mp4TestBox("moov", mp4TestBox("trak", mp4TestBox("mdia", mp4TestBox("minf", mp4TestBox("stbl", append(mp4TestBox("stsd", stsdPayload), mp4TestBox("stts", sttsPayload)...))))))
}

func mp4TestBox(boxType string, payload []byte) []byte {
	result := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(result[:4], uint32(len(result)))
	copy(result[4:8], boxType)
	copy(result[8:], payload)
	return result
}

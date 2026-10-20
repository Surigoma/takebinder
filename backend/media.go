package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func joinGroup(ctx context.Context, g group, target string, progress func(jobProgress)) jobResult {
	ffmpeg, err := findTool("ffmpeg")
	if err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_TOOL_MISSING", Message: "ffmpeg is not available"}
	}
	ffprobe, err := findTool("ffprobe")
	if err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_TOOL_MISSING", Message: "ffprobe is not available"}
	}
	inputProbes := make([]probeResult, 0, len(g.Files))
	for _, file := range g.Files {
		probe, err := probeFile(ctx, ffprobe, file.Path)
		if err != nil {
			return jobResult{GroupID: g.ID, Status: "failed", Code: "E_INPUT_UNREADABLE", Message: err.Error()}
		}
		if !hasGPMF(probe) {
			return jobResult{GroupID: g.ID, Status: "failed", Code: "E_GPMF_MISSING", Message: file.Name + " has no gpmd track"}
		}
		inputProbes = append(inputProbes, probe)
	}
	if !compatible(inputProbes) {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_STREAM_MISMATCH", Message: "video or audio streams differ"}
	}

	temp, err := os.CreateTemp(filepath.Dir(target), ".takebinder-*.partial.mp4")
	if err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_OUTPUT_UNWRITABLE", Message: err.Error()}
	}
	tempPath := temp.Name()
	_ = temp.Close()
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)
	concatPath, err := writeConcatFile(filepath.Dir(target), g.Files)
	if err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_JOIN", Message: err.Error()}
	}
	defer os.Remove(concatPath)
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-progress", "pipe:1", "-nostats", "-f", "concat", "-safe", "0", "-i", concatPath}
	for _, stream := range inputProbes[0].Streams {
		if stream.CodecType == "video" || stream.CodecType == "audio" || strings.EqualFold(stream.CodecTagString, "gpmd") {
			args = append(args, "-map", "0:"+strconv.Itoa(stream.Index))
		}
	}
	args = append(args, "-c", "copy", "-copy_unknown", "-metadata", "creation_time="+g.Files[0].Captured, "-n", tempPath)
	command := exec.CommandContext(ctx, ffmpeg, args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_JOIN", Message: err.Error()}
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_JOIN", Message: err.Error()}
	}
	durations := make([]float64, len(inputProbes))
	for index, probe := range inputProbes {
		durations[index], _ = strconv.ParseFloat(probe.Format.Duration, 64)
	}
	scanner := bufio.NewScanner(stdout)
	lastPercent := -1
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if key != "out_time_us" || !ok {
			continue
		}
		microseconds, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			continue
		}
		percent := mediaProgress(float64(microseconds)/1_000_000, durations)
		rounded := int(percent)
		if rounded == lastPercent {
			continue
		}
		lastPercent = rounded
		progress(jobProgress{Progress: percent})
	}
	if scanErr := scanner.Err(); scanErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_JOIN", Message: scanErr.Error()}
	}
	err = command.Wait()
	if err != nil {
		code := "E_JOIN"
		if errors.Is(ctx.Err(), context.Canceled) {
			code = "E_CANCELLED"
		}
		return jobResult{GroupID: g.ID, Status: "failed", Code: code, Message: strings.TrimSpace(stderr.String())}
	}
	outputProbe, err := probeFile(ctx, ffprobe, tempPath)
	if err != nil || !hasGPMF(outputProbe) {
		message := "output gpmd verification failed"
		if err != nil {
			message = err.Error()
		}
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_GPMF_VERIFY", Message: message}
	}
	if !sameAVSignature(inputProbes[0], outputProbe) {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_STREAM_MISMATCH", Message: "output streams differ from input"}
	}
	if !creationTimeMatches(outputProbe, g.Files[0].Captured) {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_GPMF_VERIFY", Message: "output creation_time differs from the first chapter"}
	}
	if !durationMatches(inputProbes, outputProbe) {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_GPMF_VERIFY", Message: "output duration differs from input total"}
	}
	if err := verifyPacketPayloads(ctx, ffprobe, ffmpeg, g.Files, inputProbes, tempPath, outputProbe); err != nil {
		code := "E_GPMF_VERIFY"
		if errors.Is(ctx.Err(), context.Canceled) {
			code = "E_CANCELLED"
		}
		return jobResult{GroupID: g.ID, Status: "failed", Code: code, Message: err.Error()}
	}
	if err := os.Rename(tempPath, target); err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_OUTPUT_UNWRITABLE", Message: err.Error()}
	}
	progress(jobProgress{Progress: 100})
	return jobResult{GroupID: g.ID, Status: "completed", OutputPath: target, Warnings: applyOutputTimestamp(target, g.Files[0].Captured), OutputStreams: summarizeStreams(outputProbe), GPMFVerification: "verified"}
}

func joinPocket4Group(ctx context.Context, g group, target string, progress func(jobProgress)) jobResult {
	mp4Merge, err := findTool("mp4_merge")
	if err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_TOOL_MISSING", Message: "mp4_merge is not available"}
	}
	ffprobe, err := findTool("ffprobe")
	if err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_TOOL_MISSING", Message: "ffprobe is not available"}
	}
	inputProbes := make([]probeResult, 0, len(g.Files))
	for _, file := range g.Files {
		probe, err := inspectFile(ctx, ffprobe, file.Path)
		if err != nil {
			return jobResult{GroupID: g.ID, Status: "failed", Code: "E_INPUT_UNREADABLE", Message: err.Error()}
		}
		if !isPocket4Probe(probe) {
			return jobResult{GroupID: g.ID, Status: "failed", Code: "E_GROUP_AMBIGUOUS", Message: file.Name + " is not a confirmed Osmo Pocket 4 file"}
		}
		inputProbes = append(inputProbes, probe)
	}
	for index := 1; index < len(inputProbes); index++ {
		if allStreamSignature(inputProbes[0]) != allStreamSignature(inputProbes[index]) {
			return jobResult{GroupID: g.ID, Status: "failed", Code: "E_STREAM_MISMATCH", Message: "DJI stream layouts differ"}
		}
		previous := groupCandidate{file: g.Files[index-1], probe: inputProbes[index-1]}
		current := groupCandidate{file: g.Files[index], probe: inputProbes[index]}
		for _, candidate := range []*groupCandidate{&previous, &current} {
			info, statErr := os.Stat(candidate.file.Path)
			if statErr != nil {
				return jobResult{GroupID: g.ID, Status: "failed", Code: "E_INPUT_UNREADABLE", Message: statErr.Error()}
			}
			candidate.file.Size = info.Size()
			candidate.file.Duration, _ = strconv.ParseFloat(candidate.probe.Format.Duration, 64)
		}
		if !pocket4Continues(previous, current) {
			return jobResult{GroupID: g.ID, Status: "failed", Code: "E_GROUP_AMBIGUOUS", Message: "Pocket 4 split continuity changed after scanning"}
		}
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".takebinder-*.partial.mp4")
	if err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_OUTPUT_UNWRITABLE", Message: err.Error()}
	}
	tempPath := temp.Name()
	_ = temp.Close()
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)
	args := make([]string, 0, len(g.Files)+2)
	for _, file := range g.Files {
		args = append(args, file.Path)
	}
	args = append(args, "--out", tempPath)
	progress(jobProgress{Progress: 5})
	command := exec.CommandContext(ctx, mp4Merge, args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_JOIN", Message: err.Error()}
	}
	var stderr, joinLog bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_JOIN", Message: err.Error()}
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Split(scanCRLF)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		value := strings.TrimSuffix(strings.TrimPrefix(line, "Merging... "), "%")
		if percent, parseErr := strconv.ParseFloat(value, 64); parseErr == nil && strings.HasPrefix(line, "Merging... ") {
			progress(jobProgress{Progress: 5 + min(max(percent, 0), 100)*0.85})
		} else if line != "" {
			joinLog.WriteString(line + "\n")
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_JOIN", Message: scanErr.Error()}
	}
	if err := command.Wait(); err != nil {
		code := "E_JOIN"
		if errors.Is(ctx.Err(), context.Canceled) {
			code = "E_CANCELLED"
		}
		return jobResult{GroupID: g.ID, Status: "failed", Code: code, Message: strings.TrimSpace(joinLog.String() + stderr.String())}
	}
	if err := extendMergedTimecode(tempPath, g.Files); err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_DJI_VERIFY", Message: err.Error()}
	}
	progress(jobProgress{Progress: 90})
	outputProbe, err := inspectFile(ctx, ffprobe, tempPath)
	if err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_DJI_VERIFY", Message: err.Error()}
	}
	if allStreamSignature(inputProbes[0]) != allStreamSignature(outputProbe) {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_DJI_VERIFY", Message: "output DJI stream layout differs from input"}
	}
	if !creationTimeMatches(outputProbe, g.Files[0].Captured) || !durationMatches(inputProbes, outputProbe) {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_DJI_VERIFY", Message: "output DJI duration or creation time differs from inputs"}
	}
	if err := verifyAllPacketPayloads(ctx, ffprobe, g.Files, inputProbes, tempPath, outputProbe); err != nil {
		code := "E_DJI_VERIFY"
		if errors.Is(ctx.Err(), context.Canceled) {
			code = "E_CANCELLED"
		}
		return jobResult{GroupID: g.ID, Status: "failed", Code: code, Message: err.Error()}
	}
	if err := os.Rename(tempPath, target); err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_OUTPUT_UNWRITABLE", Message: err.Error()}
	}
	progress(jobProgress{Progress: 100})
	return jobResult{GroupID: g.ID, Status: "completed", OutputPath: target, Warnings: applyOutputTimestamp(target, g.Files[0].Captured), OutputStreams: summarizeStreams(outputProbe), GPMFVerification: "all-tracks-verified"}
}

func scanCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for index, value := range data {
		if value == '\r' || value == '\n' {
			advance = index + 1
			for advance < len(data) && (data[advance] == '\r' || data[advance] == '\n') {
				advance++
			}
			return advance, data[:index], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func verifyAllPacketPayloads(ctx context.Context, ffprobe string, files []videoFile, inputProbes []probeResult, outputPath string, outputProbe probeResult) error {
	inputLayout := inputProbes[0].Streams
	outputLayout := outputProbe.Streams
	if allStreamSignature(inputProbes[0]) != allStreamSignature(outputProbe) {
		return errors.New("output stream layout differs from input")
	}
	expected := newPacketFingerprints(len(inputLayout))
	firstChapter := newPacketFingerprints(len(inputLayout))
	for index, file := range files {
		if allStreamSignature(inputProbes[index]) != allStreamSignature(inputProbes[0]) {
			return fmt.Errorf("%s stream layout differs", file.Name)
		}
		if err := addPacketFingerprints(ctx, ffprobe, file.Path, inputProbes[index].Streams, expected); err != nil {
			return err
		}
		if index == 0 {
			firstChapter = snapshotPacketFingerprints(expected)
		}
	}
	actual := newPacketFingerprints(len(outputLayout))
	if err := addPacketFingerprints(ctx, ffprobe, outputPath, outputLayout, actual); err != nil {
		return err
	}
	for index := range expected {
		wanted := expected[index]
		if firstChapterOnlyPocketStream(inputLayout[index]) {
			wanted = firstChapter[index]
		}
		if !samePacketFingerprint(wanted, actual[index]) {
			return fmt.Errorf("%s packet payload mismatch (input count=%d bytes=%d hash=%x, output count=%d bytes=%d hash=%x)",
				streamLabel(inputLayout[index]), wanted.count, wanted.bytes, packetFingerprintDigest(wanted),
				actual[index].count, actual[index].bytes, packetFingerprintDigest(actual[index]))
		}
	}
	if !timecodeCoversVideo(outputLayout, actual) {
		return errors.New("tmcd time range does not cover the joined video")
	}
	return nil
}

func firstChapterOnlyPocketStream(stream probeStream) bool {
	return strings.EqualFold(stream.CodecTagString, "tmcd") ||
		(stream.CodecType == "video" && strings.EqualFold(stream.CodecName, "mjpeg"))
}

func timecodeCoversVideo(layout []probeStream, fingerprints []packetFingerprint) bool {
	videoEnd, timecodeStart, timecodeEnd := math.Inf(-1), math.Inf(1), math.Inf(-1)
	for index, stream := range layout {
		fingerprint := fingerprints[index]
		if !fingerprint.hasTiming {
			continue
		}
		if stream.CodecType == "video" && !strings.EqualFold(stream.CodecName, "mjpeg") {
			videoEnd = max(videoEnd, fingerprint.endPTS)
		}
		if strings.EqualFold(stream.CodecTagString, "tmcd") {
			timecodeStart = min(timecodeStart, fingerprint.firstPTS)
			timecodeEnd = max(timecodeEnd, fingerprint.endPTS)
		}
	}
	return !math.IsInf(videoEnd, -1) && !math.IsInf(timecodeStart, 1) && timecodeStart <= 0.01 && timecodeEnd+0.05 >= videoEnd
}

func creationTimeMatches(probe probeResult, captured string) bool {
	expected, err := time.Parse(time.RFC3339Nano, captured)
	if err != nil {
		return false
	}
	for _, candidate := range []string{probe.Format.Tags["creation_time"], firstVideoCreationTime(probe)} {
		actual, err := time.Parse(time.RFC3339Nano, candidate)
		if err == nil && math.Abs(actual.Sub(expected).Seconds()) < 0.001 {
			return true
		}
	}
	return false
}

type packetFingerprint struct {
	hash      hash.Hash
	digest    []byte
	count     int64
	bytes     int64
	firstPTS  float64
	endPTS    float64
	hasTiming bool
}

type packetInfo struct {
	streamIndex int
	size        int64
	dataHash    string
	pts         float64
	duration    float64
	hasTiming   bool
}

func verifyPacketPayloads(ctx context.Context, ffprobe, ffmpeg string, files []videoFile, inputProbes []probeResult, outputPath string, outputProbe probeResult) error {
	inputLayout := mappedStreams(inputProbes[0])
	outputLayout := mappedStreams(outputProbe)
	if mappedLayoutSignature(inputLayout) != mappedLayoutSignature(outputLayout) {
		return errors.New("output stream layout differs from input")
	}
	expected := newPacketFingerprints(len(inputLayout))
	expectedKeys := map[string]bool{}
	for index, file := range files {
		layout := mappedStreams(inputProbes[index])
		if mappedLayoutSignature(layout) != mappedLayoutSignature(inputLayout) {
			return fmt.Errorf("%s stream layout differs", file.Name)
		}
		if err := addPacketFingerprints(ctx, ffprobe, file.Path, layout, expected); err != nil {
			return err
		}
		keys, err := gpmfMajorKeys(ctx, ffmpeg, file.Path, layout)
		if err != nil {
			return err
		}
		for key := range keys {
			expectedKeys[key] = true
		}
	}
	actual := newPacketFingerprints(len(outputLayout))
	if err := addPacketFingerprints(ctx, ffprobe, outputPath, outputLayout, actual); err != nil {
		return err
	}
	for index := range expected {
		if !samePacketFingerprint(expected[index], actual[index]) {
			if inputLayout[index].CodecType == "video" && strings.EqualFold(inputLayout[index].CodecName, "h264") {
				matches, err := normalizedH264Matches(ctx, ffmpeg, files, inputProbes, outputPath, outputProbe, index)
				if err != nil {
					return err
				}
				if matches {
					continue
				}
			}
			return fmt.Errorf("%s packet payload mismatch", streamLabel(inputLayout[index]))
		}
	}
	if !gpmfCoversVideo(outputLayout, actual) {
		return errors.New("gpmd time range does not cover the video")
	}
	actualKeys, err := gpmfMajorKeys(ctx, ffmpeg, outputPath, outputLayout)
	if err != nil {
		return err
	}
	for key := range expectedKeys {
		if !actualKeys[key] {
			return fmt.Errorf("gpmd key %s is missing from output", key)
		}
	}
	return nil
}

func normalizedH264Matches(ctx context.Context, ffmpeg string, files []videoFile, inputProbes []probeResult, outputPath string, outputProbe probeResult, position int) (bool, error) {
	expected, actual := sha256.New(), sha256.New()
	for index, file := range files {
		layout := mappedStreams(inputProbes[index])
		if position >= len(layout) {
			return false, errors.New("H.264 stream position is missing")
		}
		if err := hashAnnexBStream(ctx, ffmpeg, file.Path, layout[position].Index, expected); err != nil {
			return false, err
		}
	}
	outputLayout := mappedStreams(outputProbe)
	if position >= len(outputLayout) {
		return false, errors.New("output H.264 stream position is missing")
	}
	if err := hashAnnexBStream(ctx, ffmpeg, outputPath, outputLayout[position].Index, actual); err != nil {
		return false, err
	}
	return bytes.Equal(expected.Sum(nil), actual.Sum(nil)), nil
}

func hashAnnexBStream(ctx context.Context, ffmpeg, path string, streamIndex int, destination io.Writer) error {
	command := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-nostdin", "-i", path, "-map", "0:"+strconv.Itoa(streamIndex), "-c", "copy", "-bsf:v", "h264_mp4toannexb", "-f", "h264", "pipe:1")
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return err
	}
	if _, err := io.Copy(destination, stdout); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}
	if err := command.Wait(); err != nil {
		return fmt.Errorf("normalize H.264 bitstream: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func samePacketFingerprint(expected, actual packetFingerprint) bool {
	return expected.count > 0 && expected.count == actual.count && expected.bytes == actual.bytes && bytes.Equal(packetFingerprintDigest(expected), packetFingerprintDigest(actual))
}

func packetFingerprintDigest(fingerprint packetFingerprint) []byte {
	if fingerprint.digest != nil {
		return fingerprint.digest
	}
	return fingerprint.hash.Sum(nil)
}

func mappedStreams(probe probeResult) []probeStream {
	streams := []probeStream{}
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" || stream.CodecType == "audio" || strings.EqualFold(stream.CodecTagString, "gpmd") {
			streams = append(streams, stream)
		}
	}
	return streams
}

func mappedLayoutSignature(streams []probeStream) string {
	parts := make([]string, len(streams))
	for index, stream := range streams {
		parts[index] = fmt.Sprintf("%s|%s|%s", stream.CodecType, stream.CodecName, stream.CodecTagString)
	}
	return strings.Join(parts, "|")
}

func newPacketFingerprints(count int) []packetFingerprint {
	result := make([]packetFingerprint, count)
	for index := range result {
		result[index].hash = sha256.New()
	}
	return result
}

func snapshotPacketFingerprints(source []packetFingerprint) []packetFingerprint {
	result := newPacketFingerprints(len(source))
	for index, fingerprint := range source {
		result[index].digest = append([]byte(nil), fingerprint.hash.Sum(nil)...)
		result[index].count = fingerprint.count
		result[index].bytes = fingerprint.bytes
		result[index].firstPTS = fingerprint.firstPTS
		result[index].endPTS = fingerprint.endPTS
		result[index].hasTiming = fingerprint.hasTiming
	}
	return result
}

func addPacketFingerprints(ctx context.Context, ffprobe, path string, streams []probeStream, fingerprints []packetFingerprint) error {
	positions := make(map[int]int, len(streams))
	for position, stream := range streams {
		positions[stream.Index] = position
	}
	command := exec.CommandContext(ctx, ffprobe, "-v", "error", "-show_packets", "-show_entries", "packet=stream_index,pts_time,duration_time,size,data_hash", "-show_data_hash", "sha256", "-of", "compact=p=0:nk=0", path)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		packet, err := parsePacketInfo(scanner.Text())
		if err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return fmt.Errorf("ffprobe packet data for %s: %w", filepath.Base(path), err)
		}
		position, wanted := positions[packet.streamIndex]
		if !wanted {
			continue
		}
		fingerprint := &fingerprints[position]
		fingerprint.count++
		fingerprint.bytes += packet.size
		_, _ = io.WriteString(fingerprint.hash, packet.dataHash+"\n")
		if packet.hasTiming {
			end := packet.pts + max(packet.duration, 0)
			if !fingerprint.hasTiming || packet.pts < fingerprint.firstPTS {
				fingerprint.firstPTS = packet.pts
			}
			if !fingerprint.hasTiming || end > fingerprint.endPTS {
				fingerprint.endPTS = end
			}
			fingerprint.hasTiming = true
		}
	}
	if err := scanner.Err(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}
	if err := command.Wait(); err != nil {
		return fmt.Errorf("ffprobe packet data for %s: %w: %s", filepath.Base(path), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func parsePacketInfo(line string) (packetInfo, error) {
	values := map[string]string{}
	for _, field := range strings.Split(line, "|") {
		key, value, ok := strings.Cut(field, "=")
		if ok {
			values[key] = value
		}
	}
	streamIndex, err := strconv.Atoi(values["stream_index"])
	if err != nil {
		return packetInfo{}, errors.New("missing stream index")
	}
	size, err := strconv.ParseInt(values["size"], 10, 64)
	if err != nil || size < 0 {
		return packetInfo{}, errors.New("invalid packet size")
	}
	dataHash := values["data_hash"]
	if !strings.HasPrefix(dataHash, "SHA256:") || len(dataHash) != len("SHA256:")+sha256.Size*2 {
		return packetInfo{}, errors.New("invalid packet hash")
	}
	packet := packetInfo{streamIndex: streamIndex, size: size, dataHash: dataHash}
	pts, ptsErr := strconv.ParseFloat(values["pts_time"], 64)
	duration, durationErr := strconv.ParseFloat(values["duration_time"], 64)
	if ptsErr == nil && durationErr == nil {
		packet.pts, packet.duration, packet.hasTiming = pts, duration, true
	}
	return packet, nil
}

func gpmfCoversVideo(layout []probeStream, fingerprints []packetFingerprint) bool {
	videoStart, videoEnd, gpmdStart, gpmdEnd := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for index, stream := range layout {
		fingerprint := fingerprints[index]
		if !fingerprint.hasTiming {
			continue
		}
		if stream.CodecType == "video" {
			videoStart = min(videoStart, fingerprint.firstPTS)
			videoEnd = max(videoEnd, fingerprint.endPTS)
		}
		if strings.EqualFold(stream.CodecTagString, "gpmd") {
			gpmdStart = min(gpmdStart, fingerprint.firstPTS)
			gpmdEnd = max(gpmdEnd, fingerprint.endPTS)
		}
	}
	return !math.IsInf(videoStart, 1) && !math.IsInf(gpmdStart, 1) && gpmdStart <= videoStart+1 && gpmdEnd+1 >= videoEnd
}

func durationMatches(inputs []probeResult, output probeResult) bool {
	var expected float64
	for _, probe := range inputs {
		duration, err := strconv.ParseFloat(probe.Format.Duration, 64)
		if err != nil || duration <= 0 {
			return false
		}
		expected += duration
	}
	actual, err := strconv.ParseFloat(output.Format.Duration, 64)
	return err == nil && math.Abs(actual-expected) <= max(0.5, float64(len(inputs))*0.1)
}

func gpmfMajorKeys(ctx context.Context, ffmpeg, path string, layout []probeStream) (map[string]bool, error) {
	streamIndex := -1
	for _, stream := range layout {
		if strings.EqualFold(stream.CodecTagString, "gpmd") {
			streamIndex = stream.Index
			break
		}
	}
	if streamIndex < 0 {
		return nil, errors.New("gpmd stream is missing")
	}
	command := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-nostdin", "-i", path, "-map", "0:"+strconv.Itoa(streamIndex), "-c", "copy", "-f", "data", "pipe:1")
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	keys, readErr := findGPMFMajorKeys(stdout)
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, readErr
	}
	if err := command.Wait(); err != nil {
		return nil, fmt.Errorf("read gpmd keys from %s: %w: %s", filepath.Base(path), err, strings.TrimSpace(stderr.String()))
	}
	return keys, nil
}

func findGPMFMajorKeys(reader io.Reader) (map[string]bool, error) {
	keys := map[string]bool{}
	wanted := []string{"GYRO", "ACCL", "GPS5"}
	buffer := make([]byte, 64*1024)
	tail := []byte{}
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			data := append(append([]byte{}, tail...), buffer[:count]...)
			for _, key := range wanted {
				if bytes.Contains(data, []byte(key)) {
					keys[key] = true
				}
			}
			start := max(0, len(data)-3)
			tail = append(tail[:0], data[start:]...)
		}
		if errors.Is(err, io.EOF) {
			return keys, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func streamLabel(stream probeStream) string {
	if strings.EqualFold(stream.CodecTagString, "gpmd") {
		return "gpmd"
	}
	return stream.CodecType
}

func mediaProgress(seconds float64, durations []float64) float64 {
	var total float64
	for _, duration := range durations {
		total += duration
	}
	if total <= 0 || len(durations) == 0 {
		return 0
	}
	seconds = min(max(seconds, 0), total)
	return min(seconds/total*95, 95)
}

func writeConcatFile(dir string, files []videoFile) (string, error) {
	list, err := os.CreateTemp(dir, ".takebinder-*.ffconcat")
	if err != nil {
		return "", err
	}
	path := list.Name()
	keep := false
	defer func() {
		_ = list.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if _, err := list.WriteString("ffconcat version 1.0\n"); err != nil {
		return "", err
	}
	for _, file := range files {
		entry, err := concatEntry(file)
		if err != nil {
			return "", err
		}
		if _, err := list.WriteString(entry); err != nil {
			return "", err
		}
	}
	if err := list.Close(); err != nil {
		return "", err
	}
	keep = true
	return path, nil
}

func concatEntry(file videoFile) (string, error) {
	if strings.ContainsAny(file.Path, "\r\n") {
		return "", fmt.Errorf("input path contains a line break: %s", file.Name)
	}
	escaped := strings.ReplaceAll(filepath.ToSlash(file.Path), "'", "'\\''")
	return fmt.Sprintf("file '%s'\n", escaped), nil
}

func probeFile(ctx context.Context, ffprobe, path string) (probeResult, error) {
	command := exec.CommandContext(ctx, ffprobe, "-v", "error", "-count_packets", "-show_streams", "-show_format", "-of", "json", path)
	data, err := command.Output()
	if err != nil {
		return probeResult{}, fmt.Errorf("ffprobe %s: %w", filepath.Base(path), err)
	}
	var result probeResult
	if err := json.Unmarshal(data, &result); err != nil {
		return probeResult{}, err
	}
	return result, nil
}

func inspectFile(ctx context.Context, ffprobe, path string) (probeResult, error) {
	command := exec.CommandContext(ctx, ffprobe, "-v", "error", "-show_streams", "-show_format", "-of", "json", path)
	data, err := command.Output()
	if err != nil {
		return probeResult{}, err
	}
	var result probeResult
	if err := json.Unmarshal(data, &result); err != nil {
		return probeResult{}, err
	}
	return result, nil
}

func videoDetails(probe probeResult, fallback string) (captured string, duration float64, width, height int) {
	captured = fallback
	for _, candidate := range []string{probe.Format.Tags["creation_time"], firstVideoCreationTime(probe)} {
		if _, err := time.Parse(time.RFC3339Nano, candidate); err == nil {
			captured = candidate
			break
		}
	}
	duration, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			return captured, duration, stream.Width, stream.Height
		}
	}
	return captured, duration, 0, 0
}

func firstVideoCreationTime(probe probeResult) string {
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			return stream.Tags["creation_time"]
		}
	}
	return ""
}

func hasGPMF(probe probeResult) bool {
	for _, stream := range probe.Streams {
		if strings.EqualFold(stream.CodecTagString, "gpmd") {
			return true
		}
	}
	return false
}

func gpmfPackets(probe probeResult) (int64, bool) {
	for _, stream := range probe.Streams {
		if strings.EqualFold(stream.CodecTagString, "gpmd") {
			count, err := strconv.ParseInt(stream.ReadPackets, 10, 64)
			return count, err == nil && count > 0
		}
	}
	return 0, false
}

func compatible(probes []probeResult) bool {
	if len(probes) < 2 {
		return true
	}
	for i := 1; i < len(probes); i++ {
		if !sameAVSignature(probes[0], probes[i]) {
			return false
		}
	}
	return true
}

func sameAVSignature(a, b probeResult) bool {
	return strings.Join(avSignature(a), "|") == strings.Join(avSignature(b), "|")
}

func avSignature(probe probeResult) []string {
	result := []string{}
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" || stream.CodecType == "audio" {
			result = append(result, fmt.Sprintf("%s|%s|%s|%s|%d|%d|%s|%s|%s|%s|%s|%s|%s|%d|%s",
				stream.CodecType, stream.CodecName, stream.CodecTagString, stream.Profile,
				stream.Width, stream.Height, stream.PixelFormat, stream.ColorSpace,
				stream.ColorTransfer, stream.ColorPrimaries, stream.FrameRate, stream.TimeBase,
				stream.SampleRate, stream.Channels, stream.ChannelLayout))
		}
	}
	return result
}

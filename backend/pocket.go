package main

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DJI filenames are only a discovery hint: other DJI cameras use them too,
// and adjacent counters do not establish that files belong to one recording.
var djiName = regexp.MustCompile(`(?i)^DJI_[A-Z0-9_]+\.MP4$`)
var pocket4SplitName = regexp.MustCompile(`(?i)^DJI_(\d{14})_(\d{4})_D\.MP4$`)
var pocketTimecode = regexp.MustCompile(`^(\d{2}):(\d{2}):(\d{2});(\d{2})$`)

const pocket4MinimumSplitSize = int64(15 * 1024 * 1024 * 1024)

func djiClassification(probe probeResult, name string) string {
	normalized := djiModel(probe)
	switch normalized {
	case "osmopocket", "djiosmopocket", "osmopocket1", "djiosmopocket1",
		"pocket2", "djipocket2", "osmopocket2", "djiosmopocket2",
		"osmopocket3", "djiosmopocket3", "osmopocket4", "djiosmopocket4":
		return "osmo-pocket"
	}
	make := metadataValue(probe, "make", "manufacturer", "com.apple.quicktime.make")
	if strings.EqualFold(make, "DJI") || djiName.MatchString(name) {
		return "dji-candidate"
	}
	return ""
}

func djiModel(probe probeResult) string {
	model := metadataValue(probe, "model", "camera_model", "com.apple.quicktime.model", "encoder")
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(model))
}

func isPocket4Probe(probe probeResult) bool {
	model := djiModel(probe)
	return model == "osmopocket4" || model == "djiosmopocket4"
}

func regroupPocket4Candidates(grouped map[string][]groupCandidate) {
	byDirectory := map[string][]groupCandidate{}
	for key, candidates := range grouped {
		if len(candidates) != 1 || candidates[0].file.Classification != "osmo-pocket" || !isPocket4Probe(candidates[0].probe) {
			continue
		}
		if _, _, ok := parsePocket4SplitName(candidates[0].file.Name); !ok {
			continue
		}
		delete(grouped, key)
		dir := strings.ToLower(filepath.Clean(filepath.Dir(candidates[0].file.Path)))
		byDirectory[dir] = append(byDirectory[dir], candidates[0])
	}
	for dir, candidates := range byDirectory {
		sort.Slice(candidates, func(i, j int) bool {
			_, left, _ := parsePocket4SplitName(candidates[i].file.Name)
			_, right, _ := parsePocket4SplitName(candidates[j].file.Name)
			return left < right
		})
		chains := [][]groupCandidate{}
		for _, candidate := range candidates {
			if len(chains) == 0 || !pocket4Continues(chains[len(chains)-1][len(chains[len(chains)-1])-1], candidate) {
				chains = append(chains, []groupCandidate{candidate})
			} else {
				chains[len(chains)-1] = append(chains[len(chains)-1], candidate)
			}
		}
		for _, chain := range chains {
			if len(chain) == 1 {
				candidate := chain[0]
				key, _, basis, confidence := djiFileIdentity(candidate.file.Path)
				candidate.key, candidate.basis, candidate.confidence = key, basis, confidence
				candidate.file.Chapter = 1
				grouped[key] = []groupCandidate{candidate}
				continue
			}
			_, firstCounter, _ := parsePocket4SplitName(chain[0].file.Name)
			key := fmt.Sprintf("pocket4-split|%s|%04d", dir, firstCounter)
			for index := range chain {
				chain[index].key = key
				chain[index].basis = "DJI Pocket 4 split continuity"
				chain[index].confidence = "high"
				chain[index].file.Chapter = index + 1
			}
			grouped[key] = chain
		}
	}
}

func pocket4Continues(previous, current groupCandidate) bool {
	_, previousCounter, previousOK := parsePocket4SplitName(previous.file.Name)
	currentTime, currentCounter, currentOK := parsePocket4SplitName(current.file.Name)
	previousTime, _, _ := parsePocket4SplitName(previous.file.Name)
	if !previousOK || !currentOK || currentCounter != previousCounter+1 || previous.file.Size < pocket4MinimumSplitSize {
		return false
	}
	if allStreamSignature(previous.probe) != allStreamSignature(current.probe) {
		return false
	}
	expected := previousTime.Add(time.Duration(previous.file.Duration * float64(time.Second)))
	return math.Abs(currentTime.Sub(expected).Seconds()) <= 3 && pocketTimecodeContinues(previous, current)
}

func pocketTimecodeContinues(previous, current groupCandidate) bool {
	previousFrames, framesPerSecond, ok := pocketTimecodeFrames(previous.probe)
	if !ok {
		return false
	}
	currentFrames, currentRate, ok := pocketTimecodeFrames(current.probe)
	if !ok || currentRate != framesPerSecond {
		return false
	}
	framesPerDay := int64(24 * 60 * 60 * framesPerSecond)
	delta := currentFrames - previousFrames
	if delta < 0 {
		delta += framesPerDay
	}
	expected := int64(math.Round(previous.file.Duration * pocketFrameRate(previous.probe)))
	return math.Abs(float64(delta-expected)) <= 2
}

func pocketTimecodeFrames(probe probeResult) (int64, int, bool) {
	timecode := metadataValue(probe, "timecode")
	match := pocketTimecode.FindStringSubmatch(timecode)
	if match == nil {
		return 0, 0, false
	}
	rate := pocketFrameRate(probe)
	nominal, drop := 0, 0
	switch {
	case math.Abs(rate-60000.0/1001.0) < 0.001:
		nominal, drop = 60, 4
	case math.Abs(rate-30000.0/1001.0) < 0.001:
		nominal, drop = 30, 2
	default:
		return 0, 0, false
	}
	values := make([]int, 4)
	for index := range values {
		values[index], _ = strconv.Atoi(match[index+1])
	}
	if values[1] >= 60 || values[2] >= 60 || values[3] >= nominal {
		return 0, 0, false
	}
	totalMinutes := values[0]*60 + values[1]
	frames := int64(((values[0]*3600+values[1]*60+values[2])*nominal + values[3]) - drop*(totalMinutes-totalMinutes/10))
	return frames, nominal, true
}

func pocketFrameRate(probe probeResult) float64 {
	for _, stream := range probe.Streams {
		if stream.CodecType != "video" {
			continue
		}
		numerator, denominator, ok := strings.Cut(stream.FrameRate, "/")
		if !ok {
			return 0
		}
		n, nErr := strconv.ParseFloat(numerator, 64)
		d, dErr := strconv.ParseFloat(denominator, 64)
		if nErr == nil && dErr == nil && d > 0 {
			return n / d
		}
	}
	return 0
}

func parsePocket4SplitName(name string) (time.Time, int, bool) {
	match := pocket4SplitName.FindStringSubmatch(name)
	if match == nil {
		return time.Time{}, 0, false
	}
	stamp, err := time.Parse("20060102150405", match[1])
	if err != nil {
		return time.Time{}, 0, false
	}
	counter, err := strconv.Atoi(match[2])
	return stamp, counter, err == nil
}

func allStreamSignature(probe probeResult) string {
	parts := make([]string, len(probe.Streams))
	for index, stream := range probe.Streams {
		profile := stream.Profile
		// ffprobe reports AAC as either LC or unknown across otherwise identical
		// Pocket 4 chapters, depending on which packet exposes the config first.
		if stream.CodecType == "audio" {
			profile = ""
		}
		parts[index] = fmt.Sprintf("%s|%s|%s|%s|%dx%d|%s|%s|%s|%s|%s|%s|%s|%d", stream.CodecType, stream.CodecName, stream.CodecTagString, profile, stream.Width, stream.Height, stream.PixelFormat, stream.ColorSpace, stream.ColorTransfer, stream.ColorPrimaries, stream.FrameRate, stream.TimeBase, stream.SampleRate+"|"+stream.ChannelLayout, stream.Channels)
	}
	return strings.Join(parts, "||")
}

func isDJIClassification(classification string) bool {
	return classification == "osmo-pocket" || classification == "dji-candidate"
}

func djiFileIdentity(path string) (key string, chapter int, basis, confidence string) {
	// Use the full path, not a filename counter, timestamp, or generic Capture ID.
	// Every DJI file is an independent copy until capture boundaries and DJI
	// metadata preservation during remuxing have been validated with real media.
	return "dji-file|" + filepath.Clean(path), 1, "DJI file copy", "low"
}

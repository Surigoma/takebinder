package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var errProbeUnavailable = fmt.Errorf("ffprobe is required for media classification")

const (
	defaultNameFormat       = "{YYYY}-{MM}-{DD}_{hh}{mm}{ss}_{NAME}"
	legacyDefaultNameFormat = "YYYY-MM-DD_hhmmss_NAME"
)

func canonicalDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("directory is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		info, statErr := os.Lstat(absolute)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
			return "", err
		}
		resolved = absolute
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("directory is not accessible: %s", path)
	}
	return filepath.Clean(resolved), nil
}

var (
	modernName = regexp.MustCompile(`(?i)^G([HX])(\d{2})(\d{4})\.MP4$`)
	legacyRoot = regexp.MustCompile(`(?i)^GOPR(\d{4})\.MP4$`)
	legacyPart = regexp.MustCompile(`(?i)^GP(\d{2})(\d{4})\.MP4$`)
)

func scanFolder(req scanRequest, progress func(int, string)) (scanResult, error) {
	ffprobe, err := findTool("ffprobe")
	if err != nil {
		return scanResult{}, errProbeUnavailable
	}
	return scanFolderWithInspectorProgress(req, func(ctx context.Context, path string) (probeResult, error) {
		return inspectFile(ctx, ffprobe, path)
	}, progress)
}

type groupCandidate struct {
	file       videoFile
	probe      probeResult
	key        string
	basis      string
	confidence string
}

func scanFolderWithInspector(req scanRequest, inspect func(context.Context, string) (probeResult, error)) (scanResult, error) {
	return scanFolderWithInspectorProgress(req, inspect, nil)
}

func scanFolderWithInspectorProgress(req scanRequest, inspect func(context.Context, string) (probeResult, error), progress func(int, string)) (scanResult, error) {
	input, err := filepath.Abs(req.InputDir)
	if err != nil {
		return scanResult{}, err
	}
	info, err := os.Stat(input)
	if err != nil || !info.IsDir() {
		return scanResult{}, fmt.Errorf("input directory is not readable: %w", err)
	}
	output := ""
	if req.OutputDir != "" {
		output, _ = filepath.Abs(req.OutputDir)
	}
	grouped := map[string][]groupCandidate{}
	skipped := []skippedFile{}
	scanned := 0

	err = filepath.WalkDir(input, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			skipped = append(skipped, skippedFile{Path: path, Reason: walkErr.Error()})
			return nil
		}
		if path != input && entry.IsDir() {
			if !req.Recursive || (output != "" && sameFilePath(path, output)) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".mp4") {
			return nil
		}
		scanned++
		if progress != nil {
			progress(scanned, path)
		}
		fileInfo, err := entry.Info()
		if err != nil {
			skipped = append(skipped, skippedFile{Path: path, Reason: "E_INPUT_UNREADABLE"})
			return nil
		}
		modified := fileInfo.ModTime().UTC().Format(time.RFC3339)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		probe, probeErr := inspect(ctx, path)
		cancel()
		if probeErr != nil || !hasVideo(probe) {
			skipped = append(skipped, skippedFile{Path: path, Reason: "E_INPUT_UNREADABLE", Classification: "broken"})
			return nil
		}
		classification, hasGPMF := classifyMedia(probe, entry.Name())
		if classification != "gopro-gpmf" && !isDJIClassification(classification) {
			reason := "E_NOT_GOPRO"
			if classification == "gopro-no-gpmf" {
				reason = "E_GPMF_MISSING"
			}
			skipped = append(skipped, skippedFile{Path: path, Reason: reason, Classification: classification})
			return nil
		}
		key, chapter, basis, confidence, ok := groupingIdentity(probe, entry.Name())
		if isDJIClassification(classification) {
			key, chapter, basis, confidence = djiFileIdentity(path)
			ok = true
		}
		if !ok {
			skipped = append(skipped, skippedFile{Path: path, Reason: "E_GROUP_AMBIGUOUS", Classification: classification})
			return nil
		}
		if confidence != "high" {
			dir, _ := filepath.Abs(filepath.Dir(path))
			key = strings.ToLower(filepath.Clean(dir)) + "|" + key
		}
		captured, duration, width, height := videoDetails(probe, modified)
		capturedSource := "file.modified"
		if captured != modified {
			capturedSource = "metadata.creation_time"
		}
		candidate := groupCandidate{key: key, basis: basis, confidence: confidence, probe: probe, file: videoFile{
			Path: path, Name: entry.Name(), Size: fileInfo.Size(),
			Modified: modified, Captured: captured, CapturedSource: capturedSource, Duration: duration,
			Width: width, Height: height, Chapter: chapter, HasGPMF: hasGPMF, Classification: classification,
			Streams: summarizeStreams(probe),
		}}
		grouped[key] = append(grouped[key], candidate)
		return nil
	})
	if err != nil {
		return scanResult{}, err
	}
	regroupPocket4Candidates(grouped)

	groups := make([]group, 0, len(grouped))
	for key, candidates := range grouped {
		files := make([]videoFile, len(candidates))
		for index := range candidates {
			files[index] = candidates[index].file
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Chapter < files[j].Chapter })
		status, reason := validateChapters(files)
		outputName, nameErr := defaultOutputName(files[0].Captured, files[0].Name, req.OutputNameFormat)
		if nameErr != nil {
			status, reason = "ambiguous", "E_BAD_REQUEST: "+nameErr.Error()
		}
		relativeDir, relErr := filepath.Rel(input, filepath.Dir(files[0].Path))
		if relErr != nil {
			status, reason = "ambiguous", "E_BAD_REQUEST: input subfolder cannot be resolved"
		} else if relativeDir == "." {
			relativeDir = ""
		}
		groupHasGPMF := true
		for _, file := range files {
			groupHasGPMF = groupHasGPMF && file.HasGPMF
		}
		groups = append(groups, group{
			ID: shortID(key), Key: key, Files: files, Status: status, Confidence: candidates[0].confidence,
			Basis: candidates[0].basis, HasGPMF: groupHasGPMF, Reason: reason, OutputName: outputName, RelativeDir: relativeDir,
		})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Files[0].Path < groups[j].Files[0].Path })
	return scanResult{Groups: groups, Skipped: skipped}, nil
}

func summarizeStreams(probe probeResult) []streamSummary {
	result := make([]streamSummary, 0, len(probe.Streams))
	for _, stream := range probe.Streams {
		result = append(result, streamSummary{
			Type: stream.CodecType, Codec: stream.CodecName, CodecTag: stream.CodecTagString,
			Profile: stream.Profile, Width: stream.Width, Height: stream.Height, PixelFormat: stream.PixelFormat,
			ColorTransfer: stream.ColorTransfer, FrameRate: stream.FrameRate,
			SampleRate: stream.SampleRate, Channels: stream.Channels,
		})
	}
	return result
}

func defaultOutputName(captured, sourceName, format string) (string, error) {
	value, err := time.Parse(time.RFC3339Nano, captured)
	if err != nil {
		return "", fmt.Errorf("capture time is invalid: %w", err)
	}
	format, err = validateOutputNameFormat(format)
	if err != nil {
		return "", err
	}
	identifier := strings.TrimSuffix(filepath.Base(sourceName), filepath.Ext(sourceName))
	expanded := strings.NewReplacer(
		"{YYYY}", value.Format("2006"), "{MM}", value.Format("01"), "{DD}", value.Format("02"),
		"{hh}", value.Format("15"), "{mm}", value.Format("04"), "{ss}", value.Format("05"), "{NAME}", identifier,
	).Replace(format)
	if strings.HasSuffix(strings.ToLower(expanded), ".mp4") {
		expanded = expanded[:len(expanded)-4]
	}
	expanded = strings.Map(func(char rune) rune {
		if char < ' ' || strings.ContainsRune(`<>:"/\|?*`, char) {
			return '_'
		}
		return char
	}, expanded)
	expanded = strings.TrimRight(expanded, ". ")
	runes := []rune(expanded)
	if len(runes) > 180 {
		expanded = string(runes[:180])
	}
	if expanded == "" {
		return "", errors.New("output name format produces an empty name")
	}
	reserved := strings.ToUpper(expanded)
	if reserved == "CON" || reserved == "PRN" || reserved == "AUX" || reserved == "NUL" || (len(reserved) == 4 && (strings.HasPrefix(reserved, "COM") || strings.HasPrefix(reserved, "LPT")) && reserved[3] >= '1' && reserved[3] <= '9') {
		expanded = "_" + expanded
	}
	return expanded + ".mp4", nil
}

func validateOutputNameFormat(format string) (string, error) {
	if format == "" || format == legacyDefaultNameFormat {
		format = defaultNameFormat
	}
	if len(format) > 120 || strings.ContainsAny(format, `/\`) {
		return "", errors.New("invalid output name format")
	}
	for _, char := range format {
		if char < ' ' {
			return "", errors.New("invalid output name format")
		}
	}
	return format, nil
}

func classifyMedia(probe probeResult, name string) (string, bool) {
	gpmf := hasGPMF(probe)
	if classification := djiClassification(probe, name); classification != "" {
		return classification, gpmf
	}
	_, _, goProName := parseGoProName(name)
	goPro := goProName && gpmf
	for _, tags := range append([]map[string]string{probe.Format.Tags}, streamTags(probe)...) {
		for key, value := range tags {
			if strings.EqualFold(key, "firmware") || strings.Contains(strings.ToLower(value), "gopro") {
				goPro = true
			}
		}
	}
	if !goPro {
		return "not-gopro", gpmf
	}
	if !gpmf {
		return "gopro-no-gpmf", false
	}
	return "gopro-gpmf", true
}

func groupingIdentity(probe probeResult, name string) (key string, chapter int, basis, confidence string, ok bool) {
	mediaID := metadataValue(probe, "media_id", "mediaid", "capture_id", "captureid")
	metadataChapter := metadataValue(probe, "chapter", "chapter_number", "chapternumber")
	if mediaID != "" && metadataChapter != "" {
		value, err := strconv.Atoi(metadataChapter)
		if err == nil && value >= 0 {
			return "metadata-" + strings.ToLower(mediaID), value, "Media/Capture ID", "high", true
		}
	}
	key, chapter, ok = parseGoProName(name)
	return key, chapter, "GoProファイル名規則", "medium", ok
}

func metadataValue(probe probeResult, wanted ...string) string {
	for _, tags := range append([]map[string]string{probe.Format.Tags}, streamTags(probe)...) {
		for key, value := range tags {
			normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(key))
			for _, candidate := range wanted {
				candidate = strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(candidate))
				if normalized == candidate {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return ""
}

func streamTags(probe probeResult) []map[string]string {
	result := make([]map[string]string, len(probe.Streams))
	for index := range probe.Streams {
		result[index] = probe.Streams[index].Tags
	}
	return result
}

func hasVideo(probe probeResult) bool {
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			return true
		}
	}
	return false
}

func parseGoProName(name string) (string, int, bool) {
	if match := modernName.FindStringSubmatch(name); match != nil {
		return "modern-" + strings.ToLower(match[1]) + "-" + match[3], atoi2(match[2]), true
	}
	if match := legacyRoot.FindStringSubmatch(name); match != nil {
		return "legacy-" + match[1], 0, true
	}
	if match := legacyPart.FindStringSubmatch(name); match != nil {
		return "legacy-" + match[2], atoi2(match[1]), true
	}
	return "", 0, false
}

func atoi2(value string) int {
	return int(value[0]-'0')*10 + int(value[1]-'0')
}

func validateChapters(files []videoFile) (string, string) {
	if len(files) == 0 {
		return "ambiguous", "no files"
	}
	firstExpected := 1
	if strings.HasPrefix(strings.ToUpper(files[0].Name), "GOPR") {
		firstExpected = 0
	}
	if files[0].Chapter != firstExpected {
		return "ambiguous", "E_CHAPTER_GAP: first chapter is missing"
	}
	for i := 1; i < len(files); i++ {
		if files[i].Chapter != files[i-1].Chapter+1 {
			return "ambiguous", "E_CHAPTER_GAP: chapter sequence has a gap or duplicate"
		}
		previous, previousErr := time.Parse(time.RFC3339Nano, files[i-1].Captured)
		current, currentErr := time.Parse(time.RFC3339Nano, files[i].Captured)
		if previousErr == nil && currentErr == nil && current.Before(previous) {
			return "ambiguous", "E_CHAPTER_ORDER: capture time moves backwards"
		}
	}
	return "ready", ""
}

func shortID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

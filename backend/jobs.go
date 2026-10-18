package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

func (a *app) executeRun(ctx context.Context, requestID string, req runRequest) {
	startedAt := time.Now().UTC()
	pending, reused := reusableResults(req.OutputDir, req.Groups)
	for _, result := range reused {
		a.emit(requestID, "job.completed", result)
	}
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		a.emitError(requestID, "E_OUTPUT_UNWRITABLE", err)
		return
	}
	required := estimatedRunBytes(pending, req.CopyOriginals)
	available, err := availableDiskSpace(req.OutputDir)
	if err != nil {
		a.emitError(requestID, "E_OUTPUT_UNWRITABLE", err)
		return
	}
	if err := diskSpaceError(available, required); err != nil {
		a.emitError(requestID, "E_DISK_FULL", err)
		return
	}
	sem := make(chan struct{}, req.MaxParallel)
	results := make(chan jobResult, len(pending))
	var wg sync.WaitGroup
	for _, g := range pending {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- jobResult{GroupID: g.ID, Status: "failed", Code: "E_CANCELLED", Message: "cancelled", GPMFVerification: "not-run"}
				return
			}
			a.emit(requestID, "job.started", map[string]string{"groupId": g.ID})
			progress := func(update jobProgress) {
				update.GroupID = g.ID
				a.emit(requestID, "job.progress", update)
			}
			result := a.processGroup(ctx, req.OutputDir, g, req.StrictHash, req.DateFolders, req.DateFolderFormat, req.CopyOriginals, req.PreserveFolders, progress)
			if result.GPMFVerification == "" {
				if result.Code == "E_GPMF_VERIFY" {
					result.GPMFVerification = "failed"
				} else {
					result.GPMFVerification = "not-run"
				}
			}
			if result.Status == "completed" {
				if info, err := os.Stat(result.OutputPath); err == nil {
					result.OutputSize = info.Size()
					result.OutputModified = info.ModTime().UTC().Format(time.RFC3339Nano)
				}
			}
			if len(g.Files) > 0 {
				result.Captured = g.Files[0].Captured
				result.CapturedSource = g.Files[0].CapturedSource
			}
			results <- result
			for _, warning := range result.Warnings {
				a.emit(requestID, "job.warning", map[string]string{"groupId": g.ID, "message": warning})
			}
			if result.Status == "completed" {
				a.emit(requestID, "job.completed", result)
			} else {
				a.emit(requestID, "job.failed", result)
			}
		}()
	}
	wg.Wait()
	close(results)
	all := append([]jobResult{}, reused...)
	for result := range results {
		all = append(all, result)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].GroupID < all[j].GroupID })
	reportPath, reportErr := writeReportIfEnabled(req.OutputDir, buildRunReport(req, all, startedAt, time.Now().UTC()), req.WriteReport)
	payload := map[string]any{"results": all, "reportPath": reportPath}
	if reportErr != nil {
		payload["reportError"] = reportErr.Error()
	}
	a.emit(requestID, "run.completed", payload)
}

func diskSpaceError(available, required uint64) error {
	if available < required {
		return fmt.Errorf("insufficient disk space: %d bytes available, %d required", available, required)
	}
	return nil
}

func (a *app) processGroup(ctx context.Context, outputDir string, g group, strictHash, dateFolders bool, dateFolderFormat string, copyOriginals, preserveFolders bool, progress func(jobProgress)) jobResult {
	if g.Status != "ready" {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_GROUP_AMBIGUOUS", Message: g.Reason}
	}
	if len(g.Files) == 0 {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_BAD_REQUEST", Message: "group has no files"}
	}
	if len(g.Files) > 1 {
		for _, file := range g.Files {
			if isDJIClassification(file.Classification) {
				return jobResult{GroupID: g.ID, Status: "failed", Code: "E_GROUP_AMBIGUOUS", Message: "DJI files support individual copying only; capture boundaries and joined metadata preservation are not verified"}
			}
		}
	}
	if dateFolders {
		folder, err := dateFolderName(g.Files[0].Captured, dateFolderFormat)
		if err != nil {
			return jobResult{GroupID: g.ID, Status: "failed", Code: "E_BAD_REQUEST", Message: err.Error()}
		}
		outputDir = filepath.Join(outputDir, folder)
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return jobResult{GroupID: g.ID, Status: "failed", Code: "E_OUTPUT_UNWRITABLE", Message: err.Error()}
		}
	}
	originalsDir := outputDir
	if preserveFolders && g.RelativeDir != "" {
		relativeDir, err := safeRelativeDir(g.RelativeDir)
		if err != nil {
			return jobResult{GroupID: g.ID, Status: "failed", Code: "E_BAD_REQUEST", Message: err.Error()}
		}
		outputDir = filepath.Join(outputDir, relativeDir)
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return jobResult{GroupID: g.ID, Status: "failed", Code: "E_OUTPUT_UNWRITABLE", Message: err.Error()}
		}
	}
	originals := []string{}
	contentProgress := progress
	if copyOriginals {
		var err error
		originals, err = a.copyOriginalFiles(ctx, originalsDir, g.Files, progress)
		if err != nil {
			code := "E_COPY"
			if errors.Is(err, context.Canceled) {
				code = "E_CANCELLED"
			}
			return jobResult{GroupID: g.ID, Status: "failed", Code: code, Message: err.Error(), Originals: originals}
		}
		contentProgress = func(update jobProgress) {
			update.Progress = 20 + update.Progress*0.8
			progress(update)
		}
	}
	rawTarget := filepath.Join(outputDir, filepath.Base(g.OutputName))
	if len(g.Files) == 1 && sameFilePath(g.Files[0].Path, rawTarget) {
		contentProgress(jobProgress{Progress: 100})
		return jobResult{GroupID: g.ID, Status: "completed", OutputPath: rawTarget, Message: "unchanged", Originals: originals, OutputStreams: g.Files[0].Streams, GPMFVerification: "byte-identical"}
	}
	target, err := a.uniqueOutput(outputDir, g.OutputName)
	if err != nil {
		return jobResult{GroupID: g.ID, Status: "failed", Code: "E_OUTPUT_UNWRITABLE", Message: err.Error(), Originals: originals}
	}
	if len(g.Files) == 1 {
		hash, err := copyAtomic(ctx, g.Files[0].Path, target, strictHash, func(percent float64) {
			contentProgress(jobProgress{Progress: percent})
		})
		if err != nil {
			code := "E_COPY"
			if errors.Is(err, context.Canceled) {
				code = "E_CANCELLED"
			}
			return jobResult{GroupID: g.ID, Status: "failed", Code: code, Message: err.Error(), Originals: originals}
		}
		warnings := applyOutputTimestamp(target, g.Files[0].Captured)
		if g.Files[0].CapturedSource == "file.modified" {
			warnings = append(warnings, "単独ファイルはバイトコピーのためコンテナ撮影日時を変更していません")
		}
		return jobResult{GroupID: g.ID, Status: "completed", OutputPath: target, SHA256: hash, Originals: originals, Warnings: warnings, OutputStreams: g.Files[0].Streams, GPMFVerification: "byte-identical"}
	}
	result := joinGroup(ctx, g, target, contentProgress)
	result.Originals = originals
	return result
}

func buildRunReport(req runRequest, results []jobResult, startedAt, finishedAt time.Time) runReport {
	byID := make(map[string]jobResult, len(results))
	for _, result := range results {
		byID[result.GroupID] = result
	}
	items := make([]reportResult, 0, len(req.Groups))
	fileCount := 0
	for _, group := range req.Groups {
		fileCount += len(group.Files)
		items = append(items, reportResult{Group: group, Result: byID[group.ID]})
	}
	return runReport{
		Version: appVersion, StartedAt: startedAt.Format(time.RFC3339Nano), FinishedAt: finishedAt.Format(time.RFC3339Nano),
		InputDir: req.InputDir, OutputDir: req.OutputDir, FileCount: fileCount, GroupCount: len(req.Groups), Results: items,
	}
}

func safeRelativeDir(value string) (string, error) {
	clean := filepath.Clean(value)
	if clean == "." {
		return "", nil
	}
	if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid input subfolder")
	}
	return clean, nil
}

func (a *app) copyOriginalFiles(ctx context.Context, outputDir string, files []videoFile, progress func(jobProgress)) ([]string, error) {
	dir := filepath.Join(outputDir, "original")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var total int64
	for _, file := range files {
		total += file.Size
	}
	var done int64
	paths := make([]string, 0, len(files))
	for _, file := range files {
		target, err := a.uniqueOutput(dir, file.Name)
		if err != nil {
			return paths, err
		}
		_, err = copyAtomic(ctx, file.Path, target, true, func(percent float64) {
			if total > 0 {
				progress(jobProgress{Progress: (float64(done) + float64(file.Size)*percent/100) / float64(total) * 20})
			}
		})
		if err != nil {
			return paths, err
		}
		paths = append(paths, target)
		done += file.Size
	}
	return paths, nil
}

func estimatedRunBytes(groups []group, copyOriginals bool) uint64 {
	var mediaBytes uint64
	for _, group := range groups {
		for _, file := range group.Files {
			if file.Size > 0 {
				mediaBytes += uint64(file.Size)
			}
		}
	}
	required := mediaBytes
	if copyOriginals {
		required += mediaBytes
	}
	if required == 0 {
		return 0
	}
	return required + required/100 + 64*1024*1024
}

func dateFolderName(captured, format string) (string, error) {
	if format == "" {
		format = "{YYYY}-{MM}-{DD}"
	}
	if len(format) > 100 || strings.ContainsAny(format, `<>:"/\|?*`) {
		return "", fmt.Errorf("invalid date folder format")
	}
	if !strings.Contains(format, "{YYYY}") && !strings.Contains(format, "{MM}") && !strings.Contains(format, "{DD}") {
		return "", fmt.Errorf("date folder format must contain {YYYY}, {MM}, or {DD}")
	}
	for _, char := range format {
		if char < ' ' {
			return "", fmt.Errorf("invalid date folder format")
		}
	}
	value, err := time.Parse(time.RFC3339Nano, captured)
	if err != nil {
		return "", fmt.Errorf("capture date is unavailable: %w", err)
	}
	name := strings.NewReplacer(
		"{YYYY}", fmt.Sprintf("%04d", value.Year()),
		"{MM}", fmt.Sprintf("%02d", value.Month()),
		"{DD}", fmt.Sprintf("%02d", value.Day()),
	).Replace(format)
	if name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return "", fmt.Errorf("invalid date folder format")
	}
	return name, nil
}

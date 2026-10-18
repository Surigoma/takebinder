package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
)

func (a *app) serve(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var msg message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			a.emit("", "error", map[string]string{"code": "E_BAD_REQUEST", "message": err.Error()})
			continue
		}
		if msg.ProtocolVersion != protocolVersion {
			a.emit(msg.RequestID, "error", map[string]string{"code": "E_PROTOCOL", "message": "unsupported protocol version"})
			continue
		}
		switch msg.Type {
		case "status":
			a.handleStatus(msg.RequestID)
		case "install-tools":
			a.handleInstallTools(msg.RequestID)
		case "remove-tools":
			a.handleRemoveTools(msg)
		case "scan":
			a.handleScan(msg)
		case "run":
			a.handleRun(msg)
		case "cancel":
			a.handleCancel(msg.RequestID)
		default:
			a.emit(msg.RequestID, "error", map[string]string{"code": "E_BAD_REQUEST", "message": "unknown command"})
		}
	}
	return scanner.Err()
}

func (a *app) handleStatus(requestID string) {
	a.emit(requestID, "status.completed", map[string]any{
		"version": appVersion,
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
		"tools":   currentToolsStatus(),
	})
}

func (a *app) handleScan(msg message) {
	if !a.toolsMu.TryRLock() {
		a.emitError(msg.RequestID, "E_TOOLS_BUSY", errors.New("media tools are being changed"))
		return
	}
	defer a.toolsMu.RUnlock()
	var req scanRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		a.emitError(msg.RequestID, "E_BAD_REQUEST", err)
		return
	}
	if _, err := validateOutputNameFormat(req.OutputNameFormat); err != nil {
		a.emitError(msg.RequestID, "E_BAD_REQUEST", err)
		return
	}
	a.planMu.Lock()
	a.plan = nil
	a.planMu.Unlock()
	inputDir, err := canonicalDirectory(req.InputDir)
	if err != nil {
		a.emitError(msg.RequestID, "E_INPUT_UNREADABLE", err)
		return
	}
	outputDir, err := canonicalDirectory(req.OutputDir)
	if err != nil {
		a.emitError(msg.RequestID, "E_OUTPUT_UNWRITABLE", err)
		return
	}
	req.InputDir, req.OutputDir = inputDir, outputDir
	result, err := scanFolder(req, func(scanned int, path string) {
		a.emit(msg.RequestID, "scan.progress", map[string]any{"scanned": scanned, "path": path})
	})
	if err != nil {
		if errors.Is(err, errProbeUnavailable) {
			a.emitError(msg.RequestID, "E_TOOL_MISSING", err)
			return
		}
		a.emitError(msg.RequestID, "E_INPUT_UNREADABLE", err)
		return
	}
	groups := make(map[string]group, len(result.Groups))
	for _, candidate := range result.Groups {
		groups[candidate.ID] = candidate
	}
	a.planMu.Lock()
	a.plan = &scanPlan{InputDir: inputDir, OutputDir: outputDir, Groups: groups}
	a.planMu.Unlock()
	a.emit(msg.RequestID, "scan.completed", result)
}

func (a *app) handleRun(msg message) {
	var req runRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		a.emitError(msg.RequestID, "E_BAD_REQUEST", err)
		return
	}
	req, err := a.authorizeRun(req)
	if err != nil {
		a.emitError(msg.RequestID, "E_BAD_REQUEST", err)
		return
	}
	if req.MaxParallel < 1 {
		req.MaxParallel = defaultParallel(runtime.NumCPU(), len(req.Groups), sameStorage(req.InputDir, req.OutputDir))
	}
	if req.MaxParallel > 8 {
		req.MaxParallel = 8
	}

	if !a.toolsMu.TryRLock() {
		a.emitError(msg.RequestID, "E_TOOLS_BUSY", errors.New("media tools are being changed"))
		return
	}
	a.runMu.Lock()
	if a.cancelRun != nil {
		a.runMu.Unlock()
		a.toolsMu.RUnlock()
		a.emitError(msg.RequestID, "E_RUN_ACTIVE", errors.New("another run is active"))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelRun = cancel
	a.nameMu.Lock()
	a.reserved = map[string]bool{}
	a.nameMu.Unlock()
	a.runMu.Unlock()

	go func() {
		defer a.toolsMu.RUnlock()
		defer func() {
			a.runMu.Lock()
			a.cancelRun = nil
			a.runMu.Unlock()
		}()
		a.executeRun(ctx, msg.RequestID, req)
	}()
}

func defaultParallel(cpuCount, groupCount int, sameDrive bool) int {
	value := max(1, min(4, cpuCount/2, groupCount))
	if sameDrive {
		value = min(value, 2)
	}
	return value
}

func (a *app) authorizeRun(req runRequest) (runRequest, error) {
	outputDir, err := canonicalDirectory(req.OutputDir)
	if err != nil {
		return runRequest{}, err
	}
	a.planMu.Lock()
	defer a.planMu.Unlock()
	if a.plan == nil || !sameFilePath(a.plan.OutputDir, outputDir) {
		return runRequest{}, errors.New("run does not match the latest scan")
	}
	if req.CopyOriginals && !req.DateFolders {
		return runRequest{}, errors.New("original copies require date folders")
	}
	if len(req.Groups) == 0 {
		return runRequest{}, errors.New("run has no groups")
	}
	authorized := make([]group, 0, len(req.Groups))
	seen := map[string]bool{}
	for _, requested := range req.Groups {
		candidate, ok := a.plan.Groups[requested.ID]
		if !ok || candidate.Status != "ready" || seen[requested.ID] {
			return runRequest{}, fmt.Errorf("group %q is not authorized", requested.ID)
		}
		seen[requested.ID] = true
		authorized = append(authorized, candidate)
	}
	req.InputDir, req.OutputDir, req.Groups = a.plan.InputDir, outputDir, authorized
	return req, nil
}

func (a *app) handleCancel(requestID string) {
	a.runMu.Lock()
	cancel := a.cancelRun
	a.runMu.Unlock()
	if cancel == nil {
		a.emit(requestID, "cancel.completed", map[string]bool{"cancelled": false})
		return
	}
	cancel()
	a.emit(requestID, "cancel.completed", map[string]bool{"cancelled": true})
}

func (a *app) emit(requestID, eventType string, payload any) {
	a.emitMu.Lock()
	defer a.emitMu.Unlock()
	if err := a.out.Encode(event{protocolVersion, requestID, eventType, payload}); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func (a *app) emitError(requestID, code string, err error) {
	a.emit(requestID, "error", map[string]string{"code": code, "message": err.Error()})
}

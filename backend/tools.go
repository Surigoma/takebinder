package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const toolsDirectoryEnvironment = "TAKEBINDER_TOOLS_DIR"

type toolAsset struct {
	Tool     string
	Version  string
	URL      string
	SHA256   string
	Size     int64
	FileName string
}

type toolInstallResult struct {
	Available      bool   `json:"available"`
	Removable      bool   `json:"removable"`
	Path           string `json:"path,omitempty"`
	Version        string `json:"version,omitempty"`
	ExpectedSHA256 string `json:"expectedSha256,omitempty"`
}

var ffprobeAssets = map[string]toolAsset{
	"windows/amd64": {"ffprobe", "8.1.2-1", "https://github.com/shaka-project/static-ffmpeg-binaries/releases/download/n8.1.2-1/ffprobe-win-x64.exe", "fc37ca23d31ee08bb8f7e108edf3822f6ef3efc1a8d306bbe0b779190230710b", 53558272, "ffprobe.exe"},
	"linux/amd64":   {"ffprobe", "8.1.2-1", "https://github.com/shaka-project/static-ffmpeg-binaries/releases/download/n8.1.2-1/ffprobe-linux-x64", "065d3c56926052a76e884c4e4b51b7d95248da9391ab7effdcca6b94ceab98cf", 48090488, "ffprobe"},
	"linux/arm64":   {"ffprobe", "8.1.2-1", "https://github.com/shaka-project/static-ffmpeg-binaries/releases/download/n8.1.2-1/ffprobe-linux-arm64", "fd2aca1456f0261cabef4514b6d97a70fa342003347f51b39c473dd364328089", 36326648, "ffprobe"},
	"darwin/amd64":  {"ffprobe", "8.1.2-1", "https://github.com/shaka-project/static-ffmpeg-binaries/releases/download/n8.1.2-1/ffprobe-osx-x64", "d530823f480a3c7eb6334f18a00197d1e9f1070e86172b9aa89c4bf4022bd879", 42555344, "ffprobe"},
	"darwin/arm64":  {"ffprobe", "8.1.2-1", "https://github.com/shaka-project/static-ffmpeg-binaries/releases/download/n8.1.2-1/ffprobe-osx-arm64", "ded4c698b8ff38d0bc1fd30fcc5e768dc46f58bc15a8dfd61f98615ba49cde5c", 33882408, "ffprobe"},
}

var ffmpegAssets = map[string]toolAsset{
	"windows/amd64": {"ffmpeg", "8.1.2-1", "https://github.com/shaka-project/static-ffmpeg-binaries/releases/download/n8.1.2-1/ffmpeg-win-x64.exe", "4044b3924c977ad31229d504c5d5b8685f9553124fbaff6e9c99048b42830341", 53763072, "ffmpeg.exe"},
	"linux/amd64":   {"ffmpeg", "8.1.2-1", "https://github.com/shaka-project/static-ffmpeg-binaries/releases/download/n8.1.2-1/ffmpeg-linux-x64", "9eac5b2b5076db5ff853a6fa0dcd6b8de7d0cac8481eadda6c47cd935825f1ee", 48299480, "ffmpeg"},
	"linux/arm64":   {"ffmpeg", "8.1.2-1", "https://github.com/shaka-project/static-ffmpeg-binaries/releases/download/n8.1.2-1/ffmpeg-linux-arm64", "6e7b1d7d1aa8c35e3fedd78a140aa0968717aeb7386ecfb0ee00773d9f0a4503", 36523320, "ffmpeg"},
	"darwin/amd64":  {"ffmpeg", "8.1.2-1", "https://github.com/shaka-project/static-ffmpeg-binaries/releases/download/n8.1.2-1/ffmpeg-osx-x64", "62c87854d851f202fc4a29bdda0fe7b6ebcddd37b863482ce1bdc81151b03fe4", 42745472, "ffmpeg"},
	"darwin/arm64":  {"ffmpeg", "8.1.2-1", "https://github.com/shaka-project/static-ffmpeg-binaries/releases/download/n8.1.2-1/ffmpeg-osx-arm64", "e7b9fcd97f95f333512d6e8b8ac24d9dbc08f189f36047695499bd7b57214b22", 34074040, "ffmpeg"},
}

var allowedDownloadHosts = map[string]bool{
	"github.com":                           true,
	"release-assets.githubusercontent.com": true,
	"objects.githubusercontent.com":        true,
}

func platformKey() string { return runtime.GOOS + "/" + runtime.GOARCH }

func managedToolsDir() string { return os.Getenv(toolsDirectoryEnvironment) }

func managedToolPath(name string) string {
	dir := managedToolsDir()
	if dir == "" {
		return ""
	}
	asset, ok := toolAssetFor(name)
	if ok {
		return filepath.Join(dir, asset.Tool, asset.Version, asset.FileName)
	}
	return ""
}

func findTool(name string) (string, error) {
	if managed := managedToolPath(name); managed != "" {
		asset, _ := toolAssetFor(name)
		valid, err := fileMatchesAsset(managed, asset)
		if err != nil {
			return "", err
		}
		if valid {
			return managed, nil
		}
	}
	return exec.LookPath(name)
}

func toolAssetFor(name string) (toolAsset, bool) {
	assets := ffprobeAssets
	if name == "ffmpeg" {
		assets = ffmpegAssets
	} else if name != "ffprobe" {
		return toolAsset{}, false
	}
	asset, ok := assets[platformKey()]
	return asset, ok
}

func currentToolStatus(name string) toolInstallResult {
	path, err := findTool(name)
	asset, _ := toolAssetFor(name)
	removable := false
	if dir := managedToolsDir(); dir != "" {
		_, statErr := os.Lstat(filepath.Join(dir, name))
		removable = statErr == nil
	}
	return toolInstallResult{Available: err == nil, Removable: removable, Path: path, Version: asset.Version, ExpectedSHA256: asset.SHA256}
}

func currentToolsStatus() map[string]toolInstallResult {
	return map[string]toolInstallResult{
		"ffmpeg":  currentToolStatus("ffmpeg"),
		"ffprobe": currentToolStatus("ffprobe"),
	}
}

func (a *app) handleInstallTools(requestID string) {
	if !a.toolsMu.TryLock() {
		a.emitError(requestID, "E_TOOLS_BUSY", errors.New("media tools are in use or being changed"))
		return
	}
	go func() {
		defer a.toolsMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		result := currentToolsStatus()
		for _, name := range []string{"ffmpeg", "ffprobe"} {
			if result[name].Available {
				continue
			}
			asset, ok := toolAssetFor(name)
			if !ok {
				a.emitError(requestID, "E_TOOL_PLATFORM", fmt.Errorf("%s download is not available for %s", name, platformKey()))
				return
			}
			path, err := installAsset(ctx, asset, a.downloadProgress(requestID))
			if err != nil {
				a.emitError(requestID, "E_TOOL_DOWNLOAD", err)
				return
			}
			if output, err := exec.CommandContext(ctx, path, "-version").CombinedOutput(); err != nil {
				a.emitError(requestID, "E_TOOL_INVALID", fmt.Errorf("downloaded %s failed its version check: %w: %s", name, err, strings.TrimSpace(string(output))))
				return
			}
			result[name] = toolInstallResult{Available: true, Removable: true, Path: path, Version: asset.Version, ExpectedSHA256: asset.SHA256}
		}
		a.emit(requestID, "tools.install.completed", map[string]any{"tools": result})
	}()
}

func removeManagedTools() error {
	dir := managedToolsDir()
	if dir == "" {
		return errors.New("managed tools directory is not configured")
	}
	// Root confines traversal to application-managed storage, including if a
	// version directory is a symlink. Only these two reserved names are removed;
	// PATH tools and arbitrary paths from IPC are never deletion targets.
	root, err := os.OpenRoot(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	var failures []error
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if err := root.RemoveAll(name); err != nil {
			failures = append(failures, fmt.Errorf("remove managed %s: %w", name, err))
		}
	}
	return errors.Join(failures...)
}

func (a *app) handleRemoveTools(msg message) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(msg.Payload, &payload); err != nil || payload == nil || len(payload) != 0 {
		a.emitError(msg.RequestID, "E_BAD_REQUEST", errors.New("remove-tools requires an empty object; paths are not accepted"))
		return
	}
	if !a.toolsMu.TryLock() {
		a.emitError(msg.RequestID, "E_TOOLS_BUSY", errors.New("media tools are in use or being changed"))
		return
	}
	defer a.toolsMu.Unlock()
	err := removeManagedTools()
	// A partial removal also invalidates the plan and requires fresh tool status.
	a.planMu.Lock()
	a.plan = nil
	a.planMu.Unlock()
	if err != nil {
		a.emitError(msg.RequestID, "E_TOOL_REMOVE", err)
		a.handleStatus(msg.RequestID)
		return
	}
	a.emit(msg.RequestID, "tools.remove.completed", map[string]any{"tools": currentToolsStatus()})
}

func (a *app) downloadProgress(requestID string) func(tool string, downloaded, total int64) {
	return func(tool string, downloaded, total int64) {
		a.emit(requestID, "tools.install.progress", map[string]any{"tool": tool, "downloaded": downloaded, "total": total})
	}
}

func installAsset(ctx context.Context, asset toolAsset, progress func(string, int64, int64)) (string, error) {
	dir := managedToolsDir()
	if dir == "" {
		return "", errors.New("managed tools directory is not configured")
	}
	destination := filepath.Join(dir, asset.Tool, asset.Version, asset.FileName)
	if valid, err := fileMatchesAsset(destination, asset); err != nil {
		return "", err
	} else if valid {
		return destination, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", err
	}
	if err := validateAssetURL(asset.URL); err != nil {
		return "", err
	}
	client := &http.Client{
		Timeout: 30 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return validateAssetURL(req.URL.String())
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != asset.Size {
		return "", fmt.Errorf("unexpected content length: got %d, want %d", response.ContentLength, asset.Size)
	}
	temporary := destination + ".partial"
	if err := os.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("remove stale partial download: %w", err)
	}
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return "", fmt.Errorf("create partial download: %w", err)
	}
	keep := false
	defer func() {
		file.Close()
		if !keep {
			_ = os.Remove(temporary)
		}
	}()
	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	var downloaded int64
	reader := io.LimitReader(response.Body, asset.Size+1)
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			downloaded += int64(n)
			if _, err := file.Write(buffer[:n]); err != nil {
				return "", err
			}
			_, _ = hash.Write(buffer[:n])
			progress(asset.Tool, downloaded, asset.Size)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	if downloaded != asset.Size {
		return "", fmt.Errorf("unexpected size: got %d, want %d", downloaded, asset.Size)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != asset.SHA256 {
		return "", fmt.Errorf("SHA-256 mismatch: got %s", got)
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, destination); err != nil {
		return "", err
	}
	keep = true
	return destination, nil
}

func validateAssetURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" || parsed.User != nil || !allowedDownloadHosts[strings.ToLower(parsed.Hostname())] {
		return fmt.Errorf("download URL is not allowed: %s", raw)
	}
	return nil
}

func fileMatchesAsset(path string, asset toolAsset) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() != asset.Size {
		return false, fmt.Errorf("existing managed file has an unexpected size: %s", path)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, err
	}
	if hex.EncodeToString(hash.Sum(nil)) != asset.SHA256 {
		return false, fmt.Errorf("existing managed file failed SHA-256 verification: %s", path)
	}
	return true, nil
}

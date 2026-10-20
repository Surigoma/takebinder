package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveManagedTools(t *testing.T) {
	for _, layout := range []string{"absent", "versions and partials", "unconfigured"} {
		t.Run(layout, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(toolsDirectoryEnvironment, dir)
			if layout == "absent" {
				t.Setenv(toolsDirectoryEnvironment, filepath.Join(dir, "missing"))
			}
			if layout == "unconfigured" {
				t.Setenv(toolsDirectoryEnvironment, "")
			}
			for _, name := range []string{"ffmpeg/old/ffmpeg.exe", "ffprobe/current/ffprobe", "ffprobe/current/ffprobe.partial", "mp4_merge/current/mp4_merge.exe", "notes.txt", "videos/source.mp4"} {
				path := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("keep input intact"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := removeManagedTools()
			if (err != nil) != (layout == "unconfigured") {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, name := range []string{"notes.txt", "videos/source.mp4"} {
				if content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name))); err != nil || string(content) != "keep input intact" {
					t.Fatalf("unrelated file changed: %s: %v", name, err)
				}
			}
			if layout == "versions and partials" {
				for _, name := range []string{"ffmpeg", "ffprobe", "mp4_merge"} {
					if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
						t.Fatalf("tool remains: %s: %v", name, err)
					}
					if currentToolStatus(name).Removable {
						t.Fatalf("removed tool is still removable: %s", name)
					}
				}
				if err := removeManagedTools(); err != nil {
					t.Fatalf("repeat removal failed: %v", err)
				}
			}
		})
	}
}

func TestRemoveManagedToolsDoesNotFollowSymlink(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	t.Setenv(toolsDirectoryEnvironment, dir)
	source := filepath.Join(outside, "source.mp4")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "ffmpeg")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := removeManagedTools(); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(source); err != nil || string(content) != "source" {
		t.Fatalf("external target changed: %v", err)
	}
}

func TestRemoveToolsProtocol(t *testing.T) {
	for _, payload := range []string{`{}`, `{"path":"/outside"}`, `null`, `[]`} {
		t.Run(payload, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(toolsDirectoryEnvironment, dir)
			toolDir := filepath.Join(dir, "ffmpeg")
			if err := os.Mkdir(toolDir, 0o755); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			a := &app{out: json.NewEncoder(&out), plan: &scanPlan{}}
			if err := a.serve(strings.NewReader(`{"protocolVersion":"1","requestId":"remove","type":"remove-tools","payload":` + payload + "}\n")); err != nil {
				t.Fatal(err)
			}
			if payload == `{}` {
				if !strings.Contains(out.String(), `"type":"tools.remove.completed"`) || a.plan != nil {
					t.Fatalf("removal contract failed: %s", &out)
				}
				if _, err := os.Stat(toolDir); !os.IsNotExist(err) {
					t.Fatalf("tool not removed: %v", err)
				}
			} else {
				if !strings.Contains(out.String(), `"code":"E_BAD_REQUEST"`) || a.plan == nil {
					t.Fatalf("invalid request accepted: %s", &out)
				}
				if _, err := os.Stat(toolDir); err != nil {
					t.Fatalf("invalid request deleted tool: %v", err)
				}
			}
		})
	}
}

func TestToolRemovalBusy(t *testing.T) {
	for _, activity := range []string{"scan or run", "install or removal"} {
		t.Run(activity, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(toolsDirectoryEnvironment, dir)
			if err := os.Mkdir(filepath.Join(dir, "ffprobe"), 0o755); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			a := &app{out: json.NewEncoder(&out)}
			if activity == "scan or run" {
				a.toolsMu.RLock()
				defer a.toolsMu.RUnlock()
			} else {
				a.toolsMu.Lock()
				defer a.toolsMu.Unlock()
			}
			a.handleRemoveTools(message{RequestID: "remove", Payload: json.RawMessage(`{}`)})
			a.handleInstallTools("install")
			if strings.Count(out.String(), `"code":"E_TOOLS_BUSY"`) != 2 {
				t.Fatalf("tool mutation was accepted: %s", &out)
			}
			if _, err := os.Stat(filepath.Join(dir, "ffprobe")); err != nil {
				t.Fatalf("busy tool removed: %v", err)
			}
		})
	}
}

func TestToolRemovalFailureReportsFreshStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(toolsDirectoryEnvironment, path)
	var out bytes.Buffer
	a := &app{out: json.NewEncoder(&out), plan: &scanPlan{}}
	a.handleRemoveTools(message{RequestID: "remove", Payload: json.RawMessage(`{}`)})
	if !strings.Contains(out.String(), `"code":"E_TOOL_REMOVE"`) || !strings.Contains(out.String(), `"type":"status.completed"`) || a.plan != nil {
		t.Fatalf("failure contract failed: %s", &out)
	}
}

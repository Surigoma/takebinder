import { app, BrowserWindow, dialog, ipcMain } from "electron";
import { ChildProcessWithoutNullStreams, spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { existsSync, statSync } from "node:fs";
import path from "node:path";
import readline from "node:readline";
import { isBackendCommand, type BackendEvent } from "./types";

type BackendMessage = BackendEvent & { protocolVersion: "1" };

let window: BrowserWindow | null = null;
let backend: ChildProcessWithoutNullStreams | null = null;
const smokeTest = process.argv.includes("--smoke-test");
if (smokeTest) {
  app.disableHardwareAcceleration();
  app.setPath("userData", path.join(app.getPath("temp"), "takebinder-smoke"));
}

function sendToWindow(message: BackendMessage): void {
  if (!window || window.isDestroyed() || window.webContents.isDestroyed()) return;
  window.webContents.send("backend:event", message);
}

function backendPath(): string {
  const executable = process.platform === "win32" ? "takebinder-backend.exe" : "takebinder-backend";
  return app.isPackaged
    ? path.join(process.resourcesPath, "bin", executable)
    : path.join(__dirname, "..", "bin", executable);
}

function startBackend(): ChildProcessWithoutNullStreams {
  if (backend && backend.exitCode === null) return backend;
  const executable = backendPath();
  if (!existsSync(executable)) throw new Error(`Go backend not found: ${executable}`);
  backend = spawn(executable, [], {
    stdio: ["pipe", "pipe", "pipe"],
    windowsHide: true,
    shell: false,
    env: { ...process.env, TAKEBINDER_TOOLS_DIR: path.join(app.getPath("userData"), "tools") }
  });
  const lines = readline.createInterface({ input: backend.stdout });
  lines.on("line", (line) => {
    try {
      const message = JSON.parse(line) as BackendMessage;
      sendToWindow(message);
    } catch {
      sendToWindow({
        protocolVersion: "1",
        requestId: "",
        type: "error",
        payload: { code: "E_PROTOCOL", message: "Invalid backend response" }
      } satisfies BackendMessage);
    }
  });
  backend.stderr.on("data", (data: Buffer) => console.error(`[backend] ${data.toString().trimEnd()}`));
  backend.on("exit", (code) => {
    sendToWindow({
      protocolVersion: "1",
      requestId: "",
      type: "backend.exited",
      payload: { code }
    } satisfies BackendMessage);
    backend = null;
  });
  return backend;
}

async function createWindow(): Promise<void> {
  window = new BrowserWindow({
    width: 1040,
    height: 760,
    minWidth: 760,
    minHeight: 600,
    show: !smokeTest,
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true
    }
  });
  window.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
  window.on("closed", () => { window = null; });
  await window.loadFile(path.join(__dirname, "ui", "index.html"));
  window.webContents.on("will-navigate", (event) => event.preventDefault());
  if (smokeTest) {
    const interactive = await window.webContents.executeJavaScript(`new Promise((resolve) => {
      const button = document.querySelector('[data-testid="scan"]');
      const dropZone = document.querySelector('[data-drop-zone]');
      const event = new Event("dragover", { bubbles: true, cancelable: true });
      dropZone?.dispatchEvent(event);
      const settings = document.querySelector('[data-testid="settings-tab"]');
      settings?.click();
      setTimeout(() => {
        const select = document.querySelector('[data-testid="language-select"] [role="combobox"]');
        const target = document.documentElement.lang === "ja" ? "en" : "ja";
        select?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
        setTimeout(() => {
          const label = target === "ja" ? "日本語" : "English";
          const option = [...document.querySelectorAll('[role="option"]')].find((item) => item.textContent === label);
          option?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
          setTimeout(() => {
            const expected = target === "ja" ? "ファイル名の形式" : "Filename format";
            const languageChanged = document.documentElement.lang === target && localStorage.getItem("takeBinder.language") === target && document.body.innerText.includes(expected);
            window.confirm = () => true;
            document.querySelector('[data-testid="reset-settings"]')?.click();
            setTimeout(() => {
              const defaultLanguage = navigator.language.toLowerCase().startsWith("ja") ? "ja" : "en";
              resolve(Boolean(button) && Boolean(dropZone) && event.defaultPrevented && languageChanged && document.documentElement.lang === defaultLanguage && localStorage.getItem("takeBinder.language") === defaultLanguage);
            }, 50);
          }, 50);
        }, 50);
      }, 50);
    })`);
    if (!interactive) throw new Error("Renderer interaction smoke test failed");
    app.quit();
  }
}

ipcMain.handle("dialog:directory", async (event) => {
  if (event.sender !== window?.webContents) throw new Error("Untrusted IPC sender");
  const options: Electron.OpenDialogOptions = { properties: ["openDirectory"] };
  const result = window ? await dialog.showOpenDialog(window, options) : await dialog.showOpenDialog(options);
  return result.canceled ? null : (result.filePaths[0] ?? null);
});

ipcMain.handle("path:directory", (event, candidate: unknown) => {
  if (event.sender !== window?.webContents) throw new Error("Untrusted IPC sender");
  if (typeof candidate !== "string" || !path.isAbsolute(candidate)) return null;
  try {
    return statSync(candidate).isDirectory() ? path.normalize(candidate) : null;
  } catch {
    return null;
  }
});

ipcMain.handle("backend:command", (event, command: unknown) => {
  if (event.sender !== window?.webContents) throw new Error("Untrusted IPC sender");
  if (!isBackendCommand(command)) throw new Error("Invalid backend command");
  const requestId = randomUUID();
  const message: BackendMessage = { protocolVersion: "1", requestId, type: command.type, payload: command.payload };
  startBackend().stdin.write(`${JSON.stringify(message)}\n`);
  return requestId;
});

app.whenReady().then(createWindow).catch((error: unknown) => {
  console.error(error);
  app.exit(1);
});
app.on("window-all-closed", () => {
  if (process.platform !== "darwin") app.quit();
});
app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) void createWindow();
});
app.on("before-quit", () => backend?.kill());

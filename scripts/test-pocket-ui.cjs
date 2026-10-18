// Exercise the built Renderer through the real sandboxed preload/IPC bridge.
const path = require("node:path");
const { mkdirSync, writeFileSync } = require("node:fs");
const root = path.resolve(__dirname, "..");
const work = path.join(root, ".cache", "pocket-ui");

if (!process.versions.electron) {
  const { spawnSync } = require("node:child_process");
  const result = spawnSync(require("electron"), [__filename], { stdio: "inherit", shell: false, windowsHide: true });
  if (result.error) throw result.error;
  process.exit(result.status ?? 1);
} else {
  const { app, BrowserWindow, ipcMain } = require("electron");
  mkdirSync(work, { recursive: true });
  app.setPath("userData", path.join(work, "profile"));
  app.disableHardwareAcceleration();
  const timeout = setTimeout(() => { console.error("Pocket UI contract timed out"); app.exit(1); }, 30000);
  const groups = ["osmo-pocket", "dji-candidate", "gopro-gpmf"].map((classification, index) => ({
    id: `file-${index}`, status: "ready", confidence: index < 2 ? "low" : "medium",
    basis: index < 2 ? "DJI file copy" : "GoProファイル名規則", hasGpmf: index === 2,
    outputName: `2026-10-18_123000_${index < 2 ? "DJI_000" : "GX01000"}${index}.mp4`, relativeDir: "",
    files: [{ path: path.join(work, `${index}.mp4`), name: `${index}.mp4`, size: 1024, modified: "2026-10-18T12:30:00Z", captured: "2026-10-18T12:30:00Z", capturedSource: "metadata.creation_time", duration: 60, width: 3840, height: 2160, chapter: 1, hasGpmf: index === 2, classification }]
  }));
  let win;
  ipcMain.handle("backend:command", (_event, command) => {
    const response = command.type === "scan" ? { groups, skipped: [] } : { tools: { ffmpeg: { available: true }, ffprobe: { available: true } } };
    setTimeout(() => win.webContents.send("backend:event", { protocolVersion: "1", requestId: "test", type: `${command.type}.completed`, payload: response }), 10);
    return "test";
  });
  ipcMain.handle("dialog:directory", () => work);
  app.whenReady().then(async () => {
    win = new BrowserWindow({ width: 1040, height: 900, show: false, webPreferences: { preload: path.join(root, "dist", "preload.js"), contextIsolation: true, nodeIntegration: false, sandbox: true } });
    await win.loadFile(path.join(root, "dist", "ui", "index.html"));
    const js = (source) => win.webContents.executeJavaScript(source);
    const waitFor = async (source) => {
      for (let attempt = 0; attempt < 100; attempt++) {
        if (await js(source)) return;
        await new Promise((resolve) => setTimeout(resolve, 25));
      }
      throw new Error(`UI condition failed: ${source}`);
    };
    await waitFor('Boolean(document.querySelector("[data-testid=scan]"))');
    for (const language of ["ja", "en"]) {
      await js('document.querySelector("[data-testid=settings-tab]").click()');
      await waitFor('Boolean(document.querySelector("[data-testid=language-select] [role=combobox]"))');
      await js('document.querySelector("[data-testid=language-select] [role=combobox]").dispatchEvent(new MouseEvent("mousedown", {bubbles: true}))');
      const label = language === "ja" ? "日本語" : "English";
      await waitFor('Boolean(document.querySelector("[role=option]"))');
      await js(`[...document.querySelectorAll('[role=option]')].find(e => e.textContent === ${JSON.stringify(label)}).click()`);
      await waitFor(`document.documentElement.lang === ${JSON.stringify(language)}`);
      await js('document.querySelector("[role=tab]").click()');
      await waitFor('Boolean(document.querySelector("[data-testid=scan]"))');
      // Select input and output through the same handlers used in the app.
      await js('[...document.querySelectorAll("[data-drop-zone] button")].forEach(e => e.click())');
      await waitFor('Array.from(document.querySelectorAll("[data-drop-zone] input")).every(e => e.value.length > 0)');
      await js('document.querySelector("[data-testid=scan]").click()');
      const copyOnly = language === "ja" ? "コピーのみ" : "Copy only";
      const notice = language === "ja" ? "自動結合しません" : "without automatic joining";
      await waitFor(`[...document.querySelectorAll("tbody tr")].filter(e => e.textContent.includes(${JSON.stringify(copyOnly)})).length === 2 && document.body.innerText.includes(${JSON.stringify(notice)})`);
      const text = await js('document.body.innerText');
      if (!text.includes(language === "ja" ? "GoProファイル名規則" : "GoPro filename pattern")) throw new Error("GoPro row lost its identity");
      await js('document.querySelector("tbody").closest(".MuiPaper-root").scrollIntoView(); new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))');
      await new Promise((resolve) => setTimeout(resolve, 100));
      writeFileSync(path.join(work, `${language}.png`), (await win.webContents.capturePage()).toPNG());
    }
    win.setSize(760, 760);
    await js('new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))');
    await new Promise((resolve) => setTimeout(resolve, 100));
    writeFileSync(path.join(work, "minimum-width.png"), (await win.webContents.capturePage()).toPNG());
    clearTimeout(timeout);
    console.log("Pocket Renderer/preload contract: copy-only rows, limitation notice, mixed GoPro rows, scan interaction, and Japanese/English switching OK");
    app.quit();
  }).catch((error) => { console.error(error); app.exit(1); });
}

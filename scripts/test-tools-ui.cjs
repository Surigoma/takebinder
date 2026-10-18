// Verify removal UX through the built Renderer and sandboxed preload.
const path = require("node:path");
const { mkdirSync, writeFileSync } = require("node:fs");
const root = path.resolve(__dirname, "..");
const work = path.join(root, ".cache", "tools-ui");

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
  const timeout = setTimeout(() => { console.error("Tool UI test timed out"); app.exit(1); }, 30000);
  let win, removals = 0, failRemoval = false;
  let tools = { ffmpeg: { available: true, removable: true }, ffprobe: { available: true, removable: true } };
  const send = (type, payload) => win.webContents.send("backend:event", { protocolVersion: "1", requestId: "test", type, payload });
  ipcMain.handle("backend:command", (_event, command) => {
    if (command.type === "status") setTimeout(() => send("status.completed", { tools }), 10);
    else if (command.type === "remove-tools") {
      if (JSON.stringify(command.payload) !== "{}") throw new Error("Renderer sent a removal path");
      removals++;
      setTimeout(() => {
        if (failRemoval) {
          send("error", { code: "E_TOOL_REMOVE", message: "test removal failure" });
          send("status.completed", { tools });
        } else {
          tools = { ffmpeg: { available: false, removable: false }, ffprobe: { available: true, removable: false } };
          send("tools.remove.completed", { tools });
        }
      }, 150);
    } else if (command.type === "install-tools") {
      send("tools.install.progress", { tool: "ffmpeg", downloaded: 1, total: 2 });
      setTimeout(() => {
        tools = { ffmpeg: { available: true, removable: true }, ffprobe: { available: true, removable: true } };
        send("tools.install.completed", { tools });
      }, 150);
    }
    return "test";
  });
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
    await waitFor('Boolean(document.querySelector("[data-testid=settings-tab]"))');
    await js('document.querySelector("[data-testid=settings-tab]").click()');
    for (const language of ["ja", "en"]) {
      await waitFor('Boolean(document.querySelector("[data-testid=language-select] [role=combobox]"))');
      await js('document.querySelector("[data-testid=language-select] [role=combobox]").dispatchEvent(new MouseEvent("mousedown", {bubbles: true}))');
      await waitFor('Boolean(document.querySelector("[role=option]"))');
      await js(`[...document.querySelectorAll('[role=option]')].find(e => e.textContent === ${JSON.stringify(language === "ja" ? "日本語" : "English")}).click()`);
      await waitFor(`document.documentElement.lang === ${JSON.stringify(language)}`);
      await waitFor('document.querySelector("[data-testid=remove-tools]").disabled === false');
      await js('window.confirm = () => false; document.querySelector("[data-testid=remove-tools]").click()');
      if (removals !== (language === "ja" ? 0 : 2)) throw new Error("Cancel triggered removal");
      failRemoval = true;
      await js('window.confirm = text => { window.testConfirmation = text; return true; }; document.querySelector("[data-testid=remove-tools]").click()');
      await waitFor('document.querySelector("[data-testid=remove-tools]").disabled && document.querySelector("[data-testid=install-tools]").disabled');
      const confirmText = await js('window.testConfirmation');
      if (!confirmText.includes("FFmpeg") || !confirmText.includes("ffprobe")) throw new Error("Confirmation omitted targets");
      await waitFor(`[...document.querySelectorAll('[role=alert]')].some(e => e.textContent.includes(${JSON.stringify(language === "ja" ? "メディアツール削除エラー" : "Media tool removal error")}))`);
      await waitFor('document.querySelector("[data-testid=remove-tools]").disabled === false');
      failRemoval = false;
      await js('document.querySelector("[data-testid=remove-tools]").click()');
      await waitFor('document.querySelector("[data-testid=install-tools]").disabled === false && document.querySelector("[data-testid=remove-tools]").disabled');
      if (await js('document.querySelectorAll("[role=alert]").length !== 0')) throw new Error("Stale removal error remains");
      // One external tool remains available while removal is disabled.
      if (!(await js('document.body.innerText.includes("ffprobe OK")'))) throw new Error("External tool availability lost");
      await js('document.querySelector("[data-testid=remove-tools]").closest(".MuiPaper-root").scrollIntoView(); new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))');
      await new Promise((resolve) => setTimeout(resolve, 100));
      writeFileSync(path.join(work, `${language}-removed.png`), (await win.webContents.capturePage()).toPNG());
      await js('document.querySelector("[data-testid=install-tools]").click()');
      await waitFor('document.querySelector("[data-testid=remove-tools]").disabled && document.querySelector("[data-testid=install-tools]").disabled');
      await waitFor('document.querySelector("[data-testid=remove-tools]").disabled === false');
    }
    win.setSize(760, 760);
    await js('document.querySelector("[data-testid=remove-tools]").focus(); new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))');
    await new Promise((resolve) => setTimeout(resolve, 100));
    writeFileSync(path.join(work, "minimum-width.png"), (await win.webContents.capturePage()).toPNG());
    clearTimeout(timeout);
    console.log("Media tools UI: Japanese/English confirmation, cancellation, busy controls, failure recovery, removal, external tool status, and re-download flow OK");
    app.quit();
  }).catch((error) => { console.error(error); app.exit(1); });
}

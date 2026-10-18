import { useEffect, useMemo, useState } from "react";
import { Alert, Box, Container, Paper, Stack, Tab, Tabs, Typography } from "@mui/material";
import { useTranslation } from "react-i18next";
import { ConversionPanel, GroupsPanel, LogPanel, PreferencesPanel, ToolsPanel } from "./components";
import { defaultLanguage, type Language, type Translate } from "./i18n";
import type { Group, SkippedFile } from "./types";

export function App() {
  const api = window.takeBinder;
  const { t, i18n } = useTranslation();
  const language: Language = i18n.resolvedLanguage === "ja" ? "ja" : "en";
  const setLanguage = (value: Language) => { void i18n.changeLanguage(value); };
  const [inputDir, setInputDir] = useState(() => savedValue("inputDir", ""));
  const [outputDir, setOutputDir] = useState(() => savedValue("outputDir", ""));
  const [recursive, setRecursive] = useState(() => savedBoolean("recursive", true));
  const [strictHash, setStrictHash] = useState(() => savedBoolean("strictHash", true));
  const [parallel, setParallel] = useState(() => savedValue("parallel", ""));
  const [dateFolders, setDateFolders] = useState(() => savedBoolean("dateFolders", true));
  const [dateFolderFormat, setDateFolderFormat] = useState(savedDateFolderFormat);
  const [outputNameFormat, setOutputNameFormat] = useState(savedOutputNameFormat);
  const [copyOriginals, setCopyOriginals] = useState(() => savedBoolean("copyOriginals", false));
  const [preserveFolders, setPreserveFolders] = useState(() => savedBoolean("preserveFolders", false));
  const [writeReport, setWriteReport] = useState(() => savedBoolean("writeReport", true));
  const [page, setPage] = useState(0);
  const [groups, setGroups] = useState<Group[]>([]);
  const [busy, setBusy] = useState(false);
  const [running, setRunning] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [toolError, setToolError] = useState("");
  const [tools, setTools] = useState({ ffmpeg: false, ffprobe: false, removable: false, label: t("checkingTools"), progress: 0 });
  const [logs, setLogs] = useState<string[]>([]);
  const [scanProgress, setScanProgress] = useState("");
  const readyGroups = useMemo(() => groups.filter((group) => group.status === "ready"), [groups]);
  const originalBytes = useMemo(() => readyGroups.reduce((total, group) => total + group.files.reduce((sum, file) => sum + file.size, 0), 0), [readyGroups]);
  const appendLog = (message: string) => setLogs((current) => [...current.slice(-199), `${new Date().toLocaleTimeString(language === "ja" ? "ja-JP" : "en-US")} ${message}`]);

  useEffect(() => { saveValue("language", language); document.documentElement.lang = language; }, [language]);
  useEffect(() => saveValue("inputDir", inputDir), [inputDir]);
  useEffect(() => saveValue("outputDir", outputDir), [outputDir]);
  useEffect(() => {
    saveValue("recursive", String(recursive));
    saveValue("strictHash", String(strictHash));
    saveValue("parallel", parallel);
    saveValue("dateFolders", String(dateFolders));
    saveValue("dateFolderFormat", dateFolderFormat);
    saveValue("outputNameFormat", outputNameFormat);
    saveValue("copyOriginals", String(copyOriginals));
    saveValue("preserveFolders", String(preserveFolders));
    saveValue("writeReport", String(writeReport));
  }, [recursive, strictHash, parallel, dateFolders, dateFolderFormat, outputNameFormat, copyOriginals, preserveFolders, writeReport]);

  useEffect(() => {
    if (!api) return;
    const unsubscribe = api.onEvent((event) => {
      const payload = record(event.payload);
      switch (event.type) {
        case "status.completed": {
          const state = record(payload.tools);
          const ffmpeg = record(state.ffmpeg).available === true;
          const ffprobe = record(state.ffprobe).available === true;
          setTools({ ffmpeg, ffprobe, removable: record(state.ffmpeg).removable === true || record(state.ffprobe).removable === true, progress: 0, label: `FFmpeg ${ffmpeg ? "OK" : t("missing")} / ffprobe ${ffprobe ? "OK" : t("missing")}` });
          break;
        }
        case "tools.install.progress": {
          const downloaded = Number(payload.downloaded ?? 0);
          const total = Number(payload.total ?? 0);
          setTools((current) => ({ ...current, label: t("downloading", { tool: String(payload.tool ?? "tool") }), progress: total > 0 ? downloaded / total * 100 : 0 }));
          break;
        }
        case "tools.install.completed": {
          const installed = record(payload.tools);
          const ffmpeg = record(installed.ffmpeg);
          const ffprobe = record(installed.ffprobe);
          appendLog(`FFmpeg: ${ffmpeg.available === true ? t("available") : t("unavailable")}`);
          appendLog(`ffprobe: ${ffprobe.available === true ? t("available") : t("unavailable")}`);
          setInstalling(false);
          void api.command({ type: "status", payload: {} });
          break;
        }
        case "tools.remove.completed": {
          setRemoving(false);
          setGroups([]);
          appendLog(t("toolsRemoved"));
          void api.command({ type: "status", payload: {} });
          break;
        }
        case "scan.completed": {
          setGroups(Array.isArray(payload.groups) ? payload.groups as Group[] : []);
          setBusy(false);
          setScanProgress("");
          appendLog(t("scanComplete"));
          const skipped = Array.isArray(payload.skipped) ? payload.skipped as SkippedFile[] : [];
          for (const file of skipped) appendLog(`${t("skipped")} [${classificationLabel(file.classification, t)}] ${fileName(file.path)}: ${file.reason}`);
          break;
        }
        case "scan.progress": setScanProgress(t("scanProgress", { count: Number(payload.scanned ?? 0), file: fileName(String(payload.path ?? "")) })); break;
        case "job.started": appendLog(`${t("started")}: ${String(payload.groupId ?? "")}`); break;
        case "job.progress": {
          const groupId = String(payload.groupId ?? "");
          const progress = clampPercent(Number(payload.progress ?? 0));
          setGroups((current) => current.map((group) => group.id === groupId ? { ...group, progress } : group));
          break;
        }
        case "job.completed": {
          const groupId = String(payload.groupId ?? "");
          setGroups((current) => current.map((group) => group.id === groupId ? { ...group, progress: 100 } : group));
          appendLog(`${t("completed")}: ${String(payload.outputPath ?? payload.groupId ?? "")}`);
          break;
        }
        case "job.failed": appendLog(`${t("failed")} [${String(payload.code ?? "")}] ${String(payload.message ?? "")}`); break;
        case "job.warning": appendLog(`${t("warning")}: ${String(payload.message ?? "")}`); break;
        case "run.completed":
          setRunning(false);
          setBusy(false);
          appendLog(payload.reportPath ? t("allCompletedReport", { path: String(payload.reportPath) }) : t("allCompleted"));
          break;
        case "error":
          setBusy(false);
          setScanProgress("");
          setInstalling(false);
          setRemoving(false);
          if (payload.code === "E_TOOL_REMOVE") { setToolError(t("removeToolsError")); setGroups([]); }
          if (payload.code === "E_TOOLS_BUSY") setToolError(t("toolsBusy"));
          appendLog(`${t("error")} [${String(payload.code ?? "")}] ${String(payload.message ?? "")}`);
          break;
        case "backend.exited":
          setBusy(false);
          setRunning(false);
          setInstalling(false);
          setRemoving(false);
          appendLog(t("backendExited"));
          break;
      }
    });
    api.command({ type: "status", payload: {} }).catch((error: unknown) => appendLog(`${t("startupError")}: ${errorMessage(error)}`));
    return unsubscribe;
  }, [api, t]);

  const choose = async (setter: (path: string) => void) => {
    if (!api) return;
    try {
      const selected = await api.chooseDirectory();
      if (selected) setter(selected);
    } catch (error) { appendLog(`${t("chooseFolderError")}: ${errorMessage(error)}`); }
  };

  const drop = async (file: File, setter: (path: string) => void) => {
    if (!api) return;
    try {
      const selected = await api.droppedDirectory(file);
      if (selected) setter(selected);
      else appendLog(t("dropFolder"));
    } catch (error) { appendLog(`${t("folderError")}: ${errorMessage(error)}`); }
  };

  const scan = async () => {
    if (!api || !inputDir || !outputDir) return appendLog(t("chooseFolders"));
    setBusy(true);
    setScanProgress(t("searching"));
    setGroups([]);
    try { await api.command({ type: "scan", payload: { inputDir, outputDir, recursive, outputNameFormat } }); }
    catch (error) { setBusy(false); appendLog(`${t("scanStartError")}: ${errorMessage(error)}`); }
  };

  const run = async () => {
    if (!api || readyGroups.length === 0) return;
    setBusy(true);
    setRunning(true);
    setGroups((current) => current.map((group) => group.status === "ready" ? { ...group, progress: 0 } : group));
    try { await api.command({ type: "run", payload: { outputDir, maxParallel: Number(parallel), strictHash, dateFolders, dateFolderFormat, copyOriginals: dateFolders && copyOriginals, preserveFolders, writeReport, groups: readyGroups } }); }
    catch (error) { setBusy(false); setRunning(false); appendLog(`${t("runStartError")}: ${errorMessage(error)}`); }
  };

  const installTools = async () => {
    if (!api || busy || installing || removing) return;
    setInstalling(true);
    setToolError("");
    appendLog(t("toolDownloadStart"));
    try { await api.command({ type: "install-tools", payload: {} }); }
    catch (error) { setInstalling(false); appendLog(`${t("toolDownloadError")}: ${errorMessage(error)}`); }
  };

  const removeTools = async () => {
    if (!api || busy || installing || removing || !tools.removable) return;
    if (!window.confirm(t("removeToolsConfirm"))) return;
    setRemoving(true);
    setToolError("");
    try { await api.command({ type: "remove-tools", payload: {} }); }
    catch (error) { setRemoving(false); setToolError(t("removeToolsError")); appendLog(`${t("removeToolsError")}: ${errorMessage(error)}`); }
  };

  const resetSettings = () => {
    if (!window.confirm(t("resetSettingsConfirm"))) return;
    setLanguage(defaultLanguage());
    setRecursive(true);
    setStrictHash(true);
    setParallel("");
    setDateFolders(true);
    setDateFolderFormat("{YYYY}-{MM}-{DD}");
    setOutputNameFormat("{YYYY}-{MM}-{DD}_{hh}{mm}{ss}_{NAME}");
    setCopyOriginals(false);
    setPreserveFolders(false);
    setWriteReport(true);
    appendLog(t("settingsReset"));
  };

  if (!api) return <Container sx={{ py: 6 }}><Alert severity="error">{t("preloadError")}</Alert></Container>;

  return <Box sx={{ minHeight: "100vh", background: "radial-gradient(circle at 85% 0%, #173b4c 0, #071019 38%)", py: { xs: 2, md: 4 } }}><Container maxWidth="lg"><Stack spacing={3}>
    <Box><Typography variant="overline" color="primary" sx={{ fontWeight: 800, letterSpacing: 3 }}>LOSSLESS WORKFLOW</Typography><Typography variant="h2" sx={{ fontWeight: 800, fontSize: { xs: 38, md: 58 }, letterSpacing: "-0.045em" }}>TakeBinder</Typography><Typography color="text.secondary">{t("tagline")}</Typography></Box>
    <Paper><Tabs value={page} onChange={(_event, value: number) => setPage(value)} aria-label={t("pages")}><Tab label={t("conversion")} /><Tab data-testid="settings-tab" label={t("settings")} disabled={running} /></Tabs></Paper>
    {page === 0 ? <>
      <ConversionPanel t={t} inputDir={inputDir} outputDir={outputDir} busy={busy || installing || removing} running={running} canRun={readyGroups.length > 0} copyOriginals={dateFolders && copyOriginals} originalBytes={originalBytes} scanProgress={scanProgress} onChooseInput={() => void choose(setInputDir)} onChooseOutput={() => void choose(setOutputDir)} onDropInput={(file) => void drop(file, setInputDir)} onDropOutput={(file) => void drop(file, setOutputDir)} onScan={() => void scan()} onRun={() => void run()} onCancel={() => void api.command({ type: "cancel", payload: {} })} />
      <GroupsPanel t={t} groups={groups} readyCount={readyGroups.length} />
      <LogPanel t={t} logs={logs} />
    </> : <>
      <PreferencesPanel t={t} language={language} recursive={recursive} strictHash={strictHash} parallel={parallel} dateFolders={dateFolders} dateFolderFormat={dateFolderFormat} outputNameFormat={outputNameFormat} copyOriginals={copyOriginals} preserveFolders={preserveFolders} writeReport={writeReport} disabled={busy} onLanguage={setLanguage} onRecursive={setRecursive} onStrictHash={setStrictHash} onParallel={setParallel} onDateFolders={setDateFolders} onDateFolderFormat={setDateFolderFormat} onOutputNameFormat={setOutputNameFormat} onCopyOriginals={setCopyOriginals} onPreserveFolders={setPreserveFolders} onWriteReport={setWriteReport} onReset={resetSettings} />
      <ToolsPanel t={t} tools={tools} installing={installing} removing={removing} busy={busy} error={toolError} onInstall={() => void installTools()} onRemove={() => void removeTools()} />
    </>}
  </Stack></Container></Box>;
}

function record(value: unknown): Record<string, unknown> { return value && typeof value === "object" ? value as Record<string, unknown> : {}; }
function errorMessage(value: unknown): string { return value instanceof Error ? value.message : String(value); }
function clampPercent(value: number): number { return Number.isFinite(value) ? Math.min(100, Math.max(0, value)) : 0; }
function savedValue(key: string, fallback: string): string { try { return localStorage.getItem(`takeBinder.${key}`) ?? fallback; } catch { return fallback; } }
function savedBoolean(key: string, fallback: boolean): boolean { return savedValue(key, String(fallback)) === "true"; }
function savedOutputNameFormat(): string {
  const value = savedValue("outputNameFormat", "{YYYY}-{MM}-{DD}_{hh}{mm}{ss}_{NAME}");
  return value === "YYYY-MM-DD_hhmmss_NAME" ? "{YYYY}-{MM}-{DD}_{hh}{mm}{ss}_{NAME}" : value;
}
function savedDateFolderFormat(): string {
  return savedValue("dateFolderFormat", "{YYYY}-{MM}-{DD}").replace(/\{?(YYYY|MM|DD)\}?/g, "{$1}");
}
function saveValue(key: string, value: string): void { try { localStorage.setItem(`takeBinder.${key}`, value); } catch { /* storage unavailable */ } }
function fileName(path: string): string { return path.split(/[\\/]/).pop() ?? path; }
function classificationLabel(value: string, t: Translate): string { return ({ broken: t("broken"), "not-gopro": t("notGoPro"), "gopro-no-gpmf": t("noGpmf") } as Record<string, string>)[value] ?? value; }

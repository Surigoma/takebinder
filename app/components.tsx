import {
  Alert, Box, Button, Chip, CircularProgress, Divider, FormControlLabel, LinearProgress, MenuItem,
  Paper, Stack, Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  Switch, TextField, Typography
} from "@mui/material";
import type { Group, ToolState } from "./types";
import type { Language, Translate } from "./i18n";

export function ConversionPanel(props: {
  t: Translate;
  inputDir: string;
  outputDir: string;
  busy: boolean;
  running: boolean;
  canRun: boolean;
  copyOriginals: boolean;
  originalBytes: number;
  scanProgress: string;
  onChooseInput(): void;
  onChooseOutput(): void;
  onDropInput(file: File): void;
  onDropOutput(file: File): void;
  onScan(): void;
  onRun(): void;
  onCancel(): void;
}) {
  return <Paper sx={{ p: { xs: 2, md: 3 } }}><Stack spacing={2.5}>
    <Typography variant="h6" sx={{ fontWeight: 800 }}>{props.t("convertFolders")}</Typography>
    <PathField t={props.t} label={props.t("inputFolder")} value={props.inputDir} onChoose={props.onChooseInput} onDropPath={props.onDropInput} />
    <PathField t={props.t} label={props.t("outputFolder")} value={props.outputDir} onChoose={props.onChooseOutput} onDropPath={props.onDropOutput} />
    {props.copyOriginals && props.originalBytes > 0 && <Typography variant="body2" color="text.secondary">{props.t("originalExtra", { size: formatBytes(props.originalBytes) })}</Typography>}
    {props.scanProgress && <Typography variant="body2" color="text.secondary" aria-live="polite">{props.scanProgress}</Typography>}
    <Stack direction="row" spacing={1.5} useFlexGap sx={{ flexWrap: "wrap" }}>
      <Button data-testid="scan" variant="contained" disabled={props.busy} onClick={props.onScan}>{props.t("analyze")}</Button>
      <Button variant="contained" color="secondary" disabled={props.busy || !props.canRun} onClick={props.onRun}>{props.t("run")}</Button>
      <Button variant="outlined" color="error" disabled={!props.running} onClick={props.onCancel}>{props.t("cancel")}</Button>
    </Stack>
  </Stack></Paper>;
}

export function PreferencesPanel(props: {
  t: Translate;
  language: Language;
  recursive: boolean;
  strictHash: boolean;
  parallel: string;
  dateFolders: boolean;
  dateFolderFormat: string;
  outputNameFormat: string;
  copyOriginals: boolean;
  preserveFolders: boolean;
  writeReport: boolean;
  disabled: boolean;
  onLanguage(value: Language): void;
  onRecursive(value: boolean): void;
  onStrictHash(value: boolean): void;
  onParallel(value: string): void;
  onDateFolders(value: boolean): void;
  onDateFolderFormat(value: string): void;
  onOutputNameFormat(value: string): void;
  onCopyOriginals(value: boolean): void;
  onPreserveFolders(value: boolean): void;
  onWriteReport(value: boolean): void;
  onReset(): void;
}) {
  return <Paper sx={{ p: { xs: 2.5, md: 3.5 } }}><Stack spacing={3} divider={<Divider flexItem />}>
    <Stack direction={{ xs: "column", sm: "row" }} spacing={2} sx={{ justifyContent: "space-between", alignItems: { sm: "center" } }}>
      <Box><Typography variant="h6" sx={{ fontWeight: 800 }}>{props.t("settingsTitle")}</Typography><Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>{props.t("settingsHint")}</Typography></Box>
      <TextField data-testid="language-select" select disabled={props.disabled} label={props.t("language")} value={props.language} onChange={(event) => props.onLanguage(event.target.value as Language)} size="small" sx={{ width: { xs: "100%", sm: 220 }, flexShrink: 0 }}><MenuItem value="ja">{props.t("japanese")}</MenuItem><MenuItem value="en">{props.t("english")}</MenuItem></TextField>
    </Stack>
    <Box><Typography variant="subtitle1" sx={{ fontWeight: 700, mb: 0.5 }}>{props.t("output")}</Typography><Stack>
      <TextField disabled={props.disabled} label={props.t("filenameFormat")} value={props.outputNameFormat} onChange={(event) => props.onOutputNameFormat(event.target.value)} size="small" helperText={props.t("filenameHelp")} sx={{ mb: 1, maxWidth: 520 }} />
      <FormControlLabel control={<Switch disabled={props.disabled} checked={props.preserveFolders} onChange={(event) => props.onPreserveFolders(event.target.checked)} />} label={props.t("preserveFolders")} />
      <FormControlLabel control={<Switch disabled={props.disabled} checked={props.dateFolders} onChange={(event) => props.onDateFolders(event.target.checked)} />} label={props.t("dateFolders")} />
      <TextField disabled={props.disabled || !props.dateFolders} label={props.t("dateFormat")} value={props.dateFolderFormat} onChange={(event) => props.onDateFolderFormat(event.target.value)} size="small" helperText={props.t("dateHelp")} sx={{ ml: 6, mb: 1, maxWidth: 440 }} />
      <FormControlLabel control={<Switch disabled={props.disabled || !props.dateFolders} checked={props.copyOriginals} onChange={(event) => props.onCopyOriginals(event.target.checked)} />} label={props.t("copyOriginals")} />
      <FormControlLabel control={<Switch disabled={props.disabled} checked={props.writeReport} onChange={(event) => props.onWriteReport(event.target.checked)} />} label={props.t("writeReport")} />
    </Stack></Box>
    <Box><Typography variant="subtitle1" sx={{ fontWeight: 700, mb: 0.5 }}>{props.t("searchVerify")}</Typography><Stack>
      <FormControlLabel control={<Switch disabled={props.disabled} checked={props.recursive} onChange={(event) => props.onRecursive(event.target.checked)} />} label={props.t("recursive")} />
      <FormControlLabel control={<Switch disabled={props.disabled} checked={props.strictHash} onChange={(event) => props.onStrictHash(event.target.checked)} />} label={props.t("strictHash")} />
    </Stack></Box>
    <Box><Typography variant="subtitle1" sx={{ fontWeight: 700 }}>{props.t("performance")}</Typography><Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>{props.t("performanceHelp")}</Typography>
      <TextField disabled={props.disabled} label={props.t("parallel")} value={props.parallel} onChange={(event) => props.onParallel(event.target.value)} type="number" size="small" slotProps={{ htmlInput: { min: 1, max: 8 } }} sx={{ width: 220 }} />
    </Box>
    <Box><Button data-testid="reset-settings" variant="outlined" color="warning" disabled={props.disabled} onClick={props.onReset}>{props.t("resetSettings")}</Button><Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>{props.t("resetSettingsHint")}</Typography></Box>
  </Stack></Paper>;
}

function PathField({ t, label, value, onChoose, onDropPath }: { t: Translate; label: string; value: string; onChoose(): void; onDropPath(file: File): void }) {
  return <Stack data-drop-zone={label} direction="row" spacing={1} onDragOver={(event) => event.preventDefault()} onDrop={(event) => { event.preventDefault(); const file = event.dataTransfer.files[0]; if (file) onDropPath(file); }}>
    <TextField fullWidth label={label} value={value} placeholder={t("dropPlaceholder")} slotProps={{ htmlInput: { readOnly: true } }} /><Button variant="outlined" onClick={onChoose}>{t("choose")}</Button>
  </Stack>;
}

export function ToolsPanel({ t, tools, installing, onInstall }: { t: Translate; tools: ToolState; installing: boolean; onInstall(): void }) {
  return <Paper sx={{ p: { xs: 2, md: 3 } }}>
    <Stack direction={{ xs: "column", sm: "row" }} spacing={2} sx={{ justifyContent: "space-between", alignItems: { sm: "center" } }}>
      <Box><Typography variant="h6" sx={{ fontWeight: 800 }}>{t("mediaTools")}</Typography><Stack direction="row" spacing={1} sx={{ mt: 1 }}>
        <Chip label={`FFmpeg ${tools.ffmpeg ? "OK" : t("missing")}`} color={tools.ffmpeg ? "success" : "warning"} variant="outlined" />
        <Chip label={`ffprobe ${tools.ffprobe ? "OK" : t("missing")}`} color={tools.ffprobe ? "success" : "warning"} variant="outlined" />
      </Stack></Box>
      <Button variant="outlined" disabled={installing || (tools.ffmpeg && tools.ffprobe)} onClick={onInstall} startIcon={installing ? <CircularProgress size={16} /> : undefined}>{t("downloadTools")}</Button>
    </Stack>
    {installing && <Box sx={{ mt: 2 }}><Typography variant="caption" color="text.secondary">{tools.label}</Typography><LinearProgress variant="determinate" value={tools.progress} sx={{ mt: 1 }} /></Box>}
  </Paper>;
}

export function GroupsPanel({ t, groups, readyCount }: { t: Translate; groups: Group[]; readyCount: number }) {
  return <Paper sx={{ overflow: "hidden" }}>
    <Stack direction="row" sx={{ justifyContent: "space-between", alignItems: "center", p: 2.5 }}><Typography variant="h6" sx={{ fontWeight: 800 }}>{t("captureGroups")}</Typography><Chip label={t("groupSummary", { total: groups.length, ready: readyCount })} /></Stack>
    {groups.some((group) => group.basis === "DJI file copy") && <Alert severity="info" sx={{ mx: 2.5, mb: 2 }}>{t("djiCopyNotice")}</Alert>}
    <TableContainer sx={{ maxHeight: 430 }}><Table stickyHeader size="small">
      <TableHead><TableRow><TableCell>{t("outputName")}</TableCell><TableCell>{t("chapters")}</TableCell><TableCell>{t("size")}</TableCell><TableCell>{t("status")}</TableCell><TableCell>{t("videoInfo")}</TableCell><TableCell>{t("progress")}</TableCell></TableRow></TableHead>
      <TableBody>{groups.length === 0
        ? <TableRow><TableCell colSpan={6} align="center" sx={{ py: 6, color: "text.secondary" }}>{t("emptyGroups")}</TableCell></TableRow>
        : groups.map((group) => <TableRow key={group.id} hover><TableCell>{group.outputName}</TableCell><TableCell>{group.files.length}</TableCell><TableCell>{formatBytes(group.files.reduce((sum, file) => sum + file.size, 0))}</TableCell><TableCell><Stack spacing={0.5} sx={{ alignItems: "flex-start" }}><Chip size="small" label={group.status === "ready" ? t(group.basis === "DJI file copy" ? "copyOnly" : "ready") : group.reason ?? t("review")} color={group.status === "ready" ? "success" : "warning"} /><Typography variant="caption" color="text.secondary">{group.hasGpmf ? t("gpmfYes") : t("gpmfNo")} / {confidenceLabel(group.confidence, t)} / {group.basis === "GoProファイル名規則" ? t("basisFilename") : group.basis === "DJI file copy" ? t("basisDjiCopy") : group.basis}</Typography></Stack></TableCell><TableCell sx={{ whiteSpace: "nowrap" }}>{formatVideoInfo(group, t)}</TableCell><TableCell><ProgressCell group={group} t={t} /></TableCell></TableRow>)}</TableBody>
    </Table></TableContainer>
  </Paper>;
}

function ProgressCell({ group, t }: { group: Group; t: Translate }) {
  return <Stack direction="row" spacing={1} sx={{ minWidth: 150, alignItems: "center" }}>
    <LinearProgress aria-label={t("progressLabel", { name: group.outputName })} variant="determinate" value={group.progress ?? 0} sx={{ flex: 1 }} />
    <Typography variant="caption" sx={{ minWidth: 32, textAlign: "right" }}>{Math.round(group.progress ?? 0)}%</Typography>
  </Stack>;
}

function formatVideoInfo(group: Group, t: Translate): string {
  const first = group.files[0];
  const captured = formatCaptured(first?.captured, t);
  const resolution = first?.width && first.height ? `${first.width}×${first.height}` : t("resolutionUnknown");
  const seconds = group.files.reduce((sum, file) => sum + (file.duration || 0), 0);
  const duration = seconds > 0 ? t("duration", { minutes: Math.floor(seconds / 60), seconds: Math.round(seconds % 60) }) : t("durationUnknown");
  return `${captured} / ${resolution} / ${duration}`;
}

function formatCaptured(value: string | undefined, t: Translate): string {
  const match = value?.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})/);
  return match ? `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}:${match[6]}` : t("dateUnknown");
}

function confidenceLabel(value: string, t: Translate): string { return ({ high: t("confidenceHigh"), medium: t("confidenceMedium"), low: t("confidenceLow") } as Record<string, string>)[value] ?? value; }

export function LogPanel({ t, logs }: { t: Translate; logs: string[] }) {
  return <Paper sx={{ p: 2.5 }}><Typography variant="h6" sx={{ fontWeight: 800, mb: 1.5 }}>{t("log")}</Typography><Box component="pre" aria-live="polite" sx={{ height: 150, m: 0, overflow: "auto", color: "text.secondary", font: '12px/1.7 "Cascadia Mono", Consolas, monospace', whiteSpace: "pre-wrap" }}>{logs.length ? logs.join("\n") : t("noLogs")}</Box></Paper>;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = bytes / 1024;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) { value /= 1024; index++; }
  return `${value.toFixed(1)} ${units[index]}`;
}

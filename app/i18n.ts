import i18n, { type TFunction } from "i18next";
import { initReactI18next } from "react-i18next";

export type Language = "ja" | "en";
export type Translate = TFunction;

const ja = {
  toolsBusy: "動画の解析・処理またはツールの取得・削除が終わってから、もう一度お試しください。",
  removeTools: "取得したツールを削除", removingTools: "削除中…", removeToolsHelp: "アプリが取得したFFmpeg・ffprobeを削除できます。必要になったら再取得できます。別途インストールしたツールと動画は削除しません。", removeToolsConfirm: "アプリが取得したFFmpeg・ffprobe（旧バージョンを含む）を削除しますか？ 必要になったら再取得できます。動画や別途インストールしたツールは削除しません。", toolsRemoved: "アプリが取得したメディアツールを削除しました。", removeToolsError: "メディアツール削除エラー",
  copyOnly: "コピーのみ", basisDjiCopy: "DJI候補・ファイル単位", djiCopyNotice: "Osmo Pocket / DJI候補は試験対応です。録画の区切りは未確認のため、分割動画も1ファイルずつコピー・整理し、自動結合しません。ファイル名だけでは機種を確定できません。",
  checkingTools: "結合ツールを確認中…", missing: "未検出", downloading: "{{tool}} を取得中", available: "利用可能", unavailable: "未取得",
  scanComplete: "フォルダ解析が完了しました。", skipped: "スキップ", scanProgress: "{{count}}件目を解析中: {{file}}", started: "開始", completed: "完了", failed: "失敗", warning: "警告",
  allCompletedReport: "全処理完了。レポート: {{path}}", allCompleted: "全処理完了。", error: "エラー", backendExited: "バックエンドが終了しました。", startupError: "起動エラー",
  chooseFolderError: "フォルダ選択エラー", dropFolder: "フォルダをドロップしてください。", folderError: "フォルダ指定エラー", chooseFolders: "入力・出力フォルダを指定してください。",
  searching: "動画を検索中…", scanStartError: "解析開始エラー", runStartError: "処理開始エラー", toolDownloadStart: "検証済みツールの取得を開始します。", toolDownloadError: "ツール取得エラー",
  preloadError: "preload APIを読み込めません。アプリを再ビルドしてください。", tagline: "画質とGPMFジャイロデータを保ったまま、分割動画を撮影単位にまとめます。", pages: "ページ", conversion: "変換", settings: "設定",
  broken: "破損/解析不能", notGoPro: "GoPro以外", noGpmf: "GoPro・GPMFなし",
  convertFolders: "変換するフォルダ", inputFolder: "入力フォルダ", outputFolder: "出力フォルダ", originalExtra: "元動画コピーの追加必要容量: {{size}}", analyze: "フォルダを解析", run: "処理開始", cancel: "キャンセル",
  settingsTitle: "変換設定", settingsHint: "通常は既定値のままで利用できます。", language: "表示言語", japanese: "日本語", english: "English", output: "出力",
  filenameFormat: "ファイル名の形式", filenameHelp: "{YYYY}・{MM}・{DD}・{hh}・{mm}・{ss}・{NAME}を使用できます。拡張子は自動です。", preserveFolders: "入力のサブフォルダ構造を維持",
  dateFolders: "撮影日ごとのフォルダに出力", dateFormat: "日付フォルダ名の形式", dateHelp: "{YYYY}・{MM}・{DD}を使用できます。例: GoPro_{YYYY}{MM}{DD}", copyOriginals: "日付フォルダの original に元動画をコピー", writeReport: "処理結果のJSONレポートを生成",
  searchVerify: "検索と検証", recursive: "サブフォルダを含める", strictHash: "単独コピーをSHA-256検証", performance: "パフォーマンス", performanceHelp: "空欄の場合はPCに合わせて自動設定します。", parallel: "同時処理するグループ数",
  resetSettings: "初期設定に戻す", resetSettingsHint: "表示言語と変換設定を初期値へ戻します。入力・出力フォルダは保持されます。", resetSettingsConfirm: "表示言語と変換設定を初期値へ戻しますか？", settingsReset: "設定を初期値へ戻しました。",
  dropPlaceholder: "フォルダをここへドロップ、または選択", choose: "選択", mediaTools: "メディアツール", downloadTools: "検証済みツールを取得",
  captureGroups: "撮影グループ", groupSummary: "{{total}}件 / 処理可能 {{ready}}件", outputName: "出力名", chapters: "章数", size: "容量", status: "状態", videoInfo: "動画情報", progress: "進捗",
  emptyGroups: "フォルダを解析すると、ここに撮影グループが表示されます。", ready: "処理可能", review: "要確認", gpmfYes: "GPMFあり", gpmfNo: "GPMFなし", progressLabel: "{{name}}の進捗",
  resolutionUnknown: "解像度不明", durationUnknown: "時間不明", duration: "{{minutes}}分{{seconds}}秒", dateUnknown: "日時不明", confidenceHigh: "高信頼", confidenceMedium: "中信頼", confidenceLow: "低信頼", basisFilename: "GoProファイル名規則",
  log: "ログ", noLogs: "操作ログはここに表示されます。"
} as const;

const en: Record<keyof typeof ja, string> = {
  toolsBusy: "Wait for video scanning, processing, or tool changes to finish, then try again.",
  removeTools: "Remove downloaded tools", removingTools: "Removing…", removeToolsHelp: "Remove FFmpeg and ffprobe downloaded by this app. You can download them again when needed. Separately installed tools and videos are kept.", removeToolsConfirm: "Remove FFmpeg and ffprobe downloaded by this app, including older versions? You can download them again. Videos and separately installed tools are kept.", toolsRemoved: "Downloaded media tools were removed.", removeToolsError: "Media tool removal error",
  copyOnly: "Copy only", basisDjiCopy: "DJI candidate / individual file", djiCopyNotice: "Experimental Osmo Pocket / DJI discovery. Recording boundaries are unverified: split videos are copied and organized individually without automatic joining. Filenames alone cannot identify the camera model.",
  checkingTools: "Checking media tools…", missing: "Not found", downloading: "Downloading {{tool}}", available: "Available", unavailable: "Not installed",
  scanComplete: "Folder scan completed.", skipped: "Skipped", scanProgress: "Scanning item {{count}}: {{file}}", started: "Started", completed: "Completed", failed: "Failed", warning: "Warning",
  allCompletedReport: "All processing completed. Report: {{path}}", allCompleted: "All processing completed.", error: "Error", backendExited: "The backend has exited.", startupError: "Startup error",
  chooseFolderError: "Folder selection error", dropFolder: "Please drop a folder.", folderError: "Folder error", chooseFolders: "Select input and output folders.",
  searching: "Searching for videos…", scanStartError: "Scan error", runStartError: "Processing error", toolDownloadStart: "Downloading verified tools.", toolDownloadError: "Tool download error",
  preloadError: "Could not load the preload API. Rebuild the app.", tagline: "Join split GoPro videos by capture without changing image quality or GPMF gyro data.", pages: "Pages", conversion: "Convert", settings: "Settings",
  broken: "Broken/unreadable", notGoPro: "Not a GoPro video", noGpmf: "GoPro without GPMF",
  convertFolders: "Folders", inputFolder: "Input folder", outputFolder: "Output folder", originalExtra: "Additional space for original copies: {{size}}", analyze: "Scan folder", run: "Start", cancel: "Cancel",
  settingsTitle: "Conversion settings", settingsHint: "The defaults work for most cases.", language: "Language", japanese: "日本語", english: "English", output: "Output",
  filenameFormat: "Filename format", filenameHelp: "Available tokens: {YYYY}, {MM}, {DD}, {hh}, {mm}, {ss}, {NAME}. The extension is automatic.", preserveFolders: "Preserve input subfolder structure",
  dateFolders: "Create folders by capture date", dateFormat: "Date folder format", dateHelp: "Available tokens: {YYYY}, {MM}, {DD}. Example: GoPro_{YYYY}{MM}{DD}", copyOriginals: "Copy source videos to the date folder's original directory", writeReport: "Generate a JSON report",
  searchVerify: "Search and verification", recursive: "Include subfolders", strictHash: "Verify single-file copies with SHA-256", performance: "Performance", performanceHelp: "Leave blank to choose automatically for this PC.", parallel: "Concurrent capture groups",
  resetSettings: "Restore defaults", resetSettingsHint: "Restore the language and conversion settings. Input and output folders are kept.", resetSettingsConfirm: "Restore the language and conversion settings to their defaults?", settingsReset: "Settings restored to defaults.",
  dropPlaceholder: "Drop a folder here or choose one", choose: "Choose", mediaTools: "Media tools", downloadTools: "Download verified tools",
  captureGroups: "Captures", groupSummary: "{{total}} total / {{ready}} ready", outputName: "Output name", chapters: "Chapters", size: "Size", status: "Status", videoInfo: "Video info", progress: "Progress",
  emptyGroups: "Scan a folder to show captures here.", ready: "Ready", review: "Review required", gpmfYes: "GPMF present", gpmfNo: "No GPMF", progressLabel: "Progress for {{name}}",
  resolutionUnknown: "Unknown resolution", durationUnknown: "Unknown duration", duration: "{{minutes}}m {{seconds}}s", dateUnknown: "Unknown date", confidenceHigh: "High confidence", confidenceMedium: "Medium confidence", confidenceLow: "Low confidence", basisFilename: "GoPro filename pattern",
  log: "Log", noLogs: "Activity will appear here."
};

export function defaultLanguage(): Language {
  return navigator.language.toLowerCase().startsWith("ja") ? "ja" : "en";
}

function initialLanguage(): Language {
  try {
    const saved = localStorage.getItem("takeBinder.language");
    if (saved === "ja" || saved === "en") return saved;
  } catch { /* storage unavailable */ }
  return defaultLanguage();
}

void i18n.use(initReactI18next).init({
  resources: { ja: { translation: ja }, en: { translation: en } },
  lng: initialLanguage(),
  fallbackLng: "en",
  supportedLngs: ["ja", "en"],
  interpolation: { escapeValue: false },
  initAsync: false
});

export default i18n;

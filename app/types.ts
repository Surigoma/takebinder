export type VideoFile = { path: string; name: string; size: number; modified: string; captured: string; capturedSource: string; duration: number; width: number; height: number; chapter: number; hasGpmf: boolean; classification: string };
export type Group = { id: string; files: VideoFile[]; status: string; confidence: string; basis: string; hasGpmf: boolean; reason?: string; outputName: string; relativeDir: string; progress?: number };
export type SkippedFile = { path: string; reason: string; classification: string };
export type BackendEvent = { protocolVersion: string; requestId: string; type: string; payload: unknown };
export type CommandType = "status" | "install-tools" | "remove-tools" | "scan" | "run" | "cancel";
export type API = {
  chooseDirectory(): Promise<string | null>;
  droppedDirectory(file: File): Promise<string | null>;
  command(command: { type: CommandType; payload: unknown }): Promise<string>;
  onEvent(listener: (event: BackendEvent) => void): () => void;
};
export type ToolState = { ffmpeg: boolean; ffprobe: boolean; removable: boolean; label: string; progress: number };

declare global { interface Window { takeBinder?: API } }

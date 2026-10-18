import { contextBridge, ipcRenderer, webUtils } from "electron";

type Command = { type: "status" | "install-tools" | "remove-tools" | "scan" | "run" | "cancel"; payload: unknown };
type BackendEvent = { protocolVersion: string; requestId: string; type: string; payload: unknown };

contextBridge.exposeInMainWorld("takeBinder", {
  chooseDirectory: (): Promise<string | null> => ipcRenderer.invoke("dialog:directory"),
  droppedDirectory: (file: File): Promise<string | null> => ipcRenderer.invoke("path:directory", webUtils.getPathForFile(file)),
  command: (command: Command): Promise<string> => ipcRenderer.invoke("backend:command", command),
  onEvent: (listener: (event: BackendEvent) => void): (() => void) => {
    const handler = (_electronEvent: Electron.IpcRendererEvent, event: BackendEvent) => listener(event);
    ipcRenderer.on("backend:event", handler);
    return () => ipcRenderer.removeListener("backend:event", handler);
  }
});

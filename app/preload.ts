import { contextBridge, ipcRenderer, webUtils } from "electron";
import type { BackendCommand, BackendEvent } from "./types";

contextBridge.exposeInMainWorld("takeBinder", {
  chooseDirectory: (): Promise<string | null> => ipcRenderer.invoke("dialog:directory"),
  droppedDirectory: (file: File): Promise<string | null> => ipcRenderer.invoke("path:directory", webUtils.getPathForFile(file)),
  command: (command: BackendCommand): Promise<string> => ipcRenderer.invoke("backend:command", command),
  onEvent: (listener: (event: BackendEvent) => void): (() => void) => {
    const handler = (_electronEvent: Electron.IpcRendererEvent, event: BackendEvent) => listener(event);
    ipcRenderer.on("backend:event", handler);
    return () => ipcRenderer.removeListener("backend:event", handler);
  }
});

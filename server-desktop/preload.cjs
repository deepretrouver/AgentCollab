const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("agentCollabServer", {
  getState: () => ipcRenderer.invoke("server:get-state"),
  configure: (input) => ipcRenderer.invoke("server:configure", input),
  restart: () => ipcRenderer.invoke("server:restart"),
  openClient: () => ipcRenderer.invoke("server:open-client"),
  openHub: () => ipcRenderer.invoke("server:open-hub"),
  openAdmin: () => ipcRenderer.invoke("server:open-admin"),
  openDownloads: () => ipcRenderer.invoke("server:open-downloads"),
  copyTeamURL: () => ipcRenderer.invoke("server:copy-team-url"),
  openData: () => ipcRenderer.invoke("server:open-data"),
  openLogs: () => ipcRenderer.invoke("server:open-logs"),
  openSettings: () => ipcRenderer.invoke("server:open-settings"),
  quit: () => ipcRenderer.invoke("server:quit"),
  onStatus: (listener) => {
    const handler = (_event, state) => listener(state);
    ipcRenderer.on("server:status", handler);
    return () => ipcRenderer.removeListener("server:status", handler);
  }
});

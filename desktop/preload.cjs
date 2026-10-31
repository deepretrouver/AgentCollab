const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("agentCollabDesktop", {
  isDesktop: true,
  platform: process.platform,
  getState: () => ipcRenderer.invoke("desktop:get-state"),
  configure: (input) => ipcRenderer.invoke("desktop:configure", input),
  openTeamLink: (inviteLink) => ipcRenderer.invoke("desktop:open-team-link", inviteLink),
  connectCurrentComputer: (input) => ipcRenderer.invoke("desktop:connect-current", input),
  reconnect: () => ipcRenderer.invoke("desktop:reconnect"),
  openSettings: () => ipcRenderer.invoke("desktop:open-settings"),
  listProjectBindings: () => ipcRenderer.invoke("desktop:list-project-bindings"),
  bindProjectStep: (input) => ipcRenderer.invoke("desktop:bind-project-step", input),
  unbindProjectStep: (input) => ipcRenderer.invoke("desktop:unbind-project-step", input),
  openProjectStep: (input) => ipcRenderer.invoke("desktop:open-project-step", input),
  onStatus: (listener) => {
    const handler = (_event, state) => listener(state);
    ipcRenderer.on("desktop:status", handler);
    return () => ipcRenderer.removeListener("desktop:status", handler);
  }
});

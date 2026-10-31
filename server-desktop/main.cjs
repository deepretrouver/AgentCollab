const { app, BrowserWindow, Menu, Tray, clipboard, ipcMain, nativeImage, shell } = require("electron");
const { spawn, spawnSync } = require("node:child_process");
const { randomUUID } = require("node:crypto");
const dgram = require("node:dgram");
const fs = require("node:fs");
const http = require("node:http");
const net = require("node:net");
const os = require("node:os");
const path = require("node:path");
const { listLANAddresses, primaryLANURL, replaceURLPort, shouldUseAutomaticAddress } = require("./network.cjs");
const { shouldRevealInitialWindow } = require("./lifecycle.cjs");
const { versionedUserDataName } = require("./major-version.cjs");

const APP_NAME = "AgentCollab Server";
const APP_VERSION = app.getVersion();
const SETTINGS_VERSION = 4;
const DEFAULT_PORT = Number(process.env.AGENTCOLLAB_SERVER_PORT || 8080);
const DISCOVERY_PORT = 41729;
const ADDRESS_STABILITY_POLLS = 3;

let mainWindow;
let tray;
let settings;
let hubProcess;
let hubHealthy = false;
let restartTimer;
let networkTimer;
let addressRefreshRunning = false;
let maintenanceRunning = false;
let discoverySocket;
let pendingPublicUrl = "";
let pendingPublicUrlPolls = 0;
let lastHealthyAt = "";
let quitting = false;
let quitPromise;

app.setName(APP_NAME);
const isolatedData = process.env.AGENTCOLLAB_SERVER_DATA_DIR;
if (isolatedData && !path.isAbsolute(isolatedData)) throw new Error("AGENTCOLLAB_SERVER_DATA_DIR must be absolute");
app.setPath("userData", isolatedData || path.join(app.getPath("appData"), versionedUserDataName(APP_NAME, APP_VERSION)));

function userDataPath(...parts) {
  return path.join(app.getPath("userData"), ...parts);
}

function settingsPath() {
  return userDataPath("server-settings.json");
}

function runtimeExecutable() {
  const suffix = process.platform === "win32" ? ".exe" : "";
  const base = app.isPackaged ? path.join(process.resourcesPath, "runtime") : path.join(__dirname, "runtime");
  return path.join(base, `agentcollab-hub${suffix}`);
}

function normalizeHubURL(value) {
  const raw = String(value ?? "").trim().replace(/\/+$/, "");
  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(raw) ? raw : `http://${raw}`;
  const parsed = new URL(withScheme);
  if (!["http:", "https:"].includes(parsed.protocol) || !parsed.host || parsed.username || parsed.password || parsed.search || parsed.hash) {
    throw new Error("团队地址必须是 HTTP 或 HTTPS 地址，且不能包含账号、查询参数或片段");
  }
  parsed.pathname = parsed.pathname.replace(/\/+$/, "");
  return parsed.toString().replace(/\/$/, "");
}

function currentNetworkURLs(port = DEFAULT_PORT) {
  return listLANAddresses(os.networkInterfaces(), port).map((entry) => entry.url);
}

function defaultPublicURL(port = DEFAULT_PORT) {
  return primaryLANURL(os.networkInterfaces(), port) || `http://127.0.0.1:${port}`;
}

function defaultState() {
  return {
    configured: false,
    running: false,
    hubName: "AgentCollab 本地协作",
    publicUrl: defaultPublicURL(DEFAULT_PORT),
    addressMode: "auto",
    networkUrls: currentNetworkURLs(DEFAULT_PORT),
    storageRoot: userDataPath("storage"),
    port: DEFAULT_PORT,
    version: app.getVersion(),
  };
}

function loadSettings() {
  try {
    const loaded = JSON.parse(fs.readFileSync(settingsPath(), "utf8"));
    if (loaded?.version !== SETTINGS_VERSION) return undefined;
    const port = Number(loaded.port || DEFAULT_PORT);
    const storedPublicUrl = normalizeHubURL(loaded.publicUrl);
    const addressMode = loaded.addressMode === "manual" || loaded.addressMode === "auto"
      ? loaded.addressMode
      : shouldUseAutomaticAddress(storedPublicUrl) ? "auto" : "manual";
    const detectedPublicUrl = addressMode === "auto" ? primaryLANURL(os.networkInterfaces(), port) : "";
    const publicUrl = detectedPublicUrl || storedPublicUrl;
    const changed = publicUrl !== storedPublicUrl;
    const normalized = {
      ...loaded,
      version: SETTINGS_VERSION,
      hubId: String(loaded.hubId || randomUUID()),
      hubName: String(loaded.hubName || "AgentCollab").trim(),
      publicUrl,
      addressMode,
      storageRoot: path.resolve(String(loaded.storageRoot)),
      port,
      autoStart: false,
      previousPublicUrl: changed ? storedPublicUrl : String(loaded.previousPublicUrl || ""),
      addressChangedAt: changed ? new Date().toISOString() : String(loaded.addressChangedAt || ""),
    };
    saveSettings(normalized);
    return normalized;
  } catch {
    return undefined;
  }
}

function saveSettings(next) {
  fs.mkdirSync(path.dirname(settingsPath()), { recursive: true, mode: 0o700 });
  const temporary = `${settingsPath()}.tmp`;
  fs.writeFileSync(temporary, `${JSON.stringify(next, null, 2)}\n`, { mode: 0o600 });
  fs.renameSync(temporary, settingsPath());
}

function yamlString(value) {
  return JSON.stringify(String(value));
}

function configPath() {
  return userDataPath("hub.yaml");
}

function writeHubConfig(next) {
  const runnerURLs = next.addressMode === "auto" ? currentNetworkURLs(next.port) : [next.publicUrl];
  const usableRunnerURLs = runnerURLs.length > 0 ? runnerURLs : [next.publicUrl];
  const yaml = [
    `storage_root: ${yamlString(next.storageRoot)}`,
    `listen_address: ${yamlString(`0.0.0.0:${next.port}`)}`,
    `hub_id: ${yamlString(next.hubId)}`,
    `hub_name: ${yamlString(next.hubName)}`,
    `public_url: ${yamlString(next.publicUrl)}`,
    "runner_public_urls:",
    ...usableRunnerURLs.map((url) => `  - ${yamlString(url)}`),
    `database_path: ${yamlString("data/agentcollab.db")}`,
    `log_path: ${yamlString("logs/hub.jsonl")}`,
    "require_smb: false",
    "",
  ].join("\n");
  fs.mkdirSync(next.storageRoot, { recursive: true });
  fs.writeFileSync(configPath(), yaml, { mode: 0o600 });
}

function bundledPackageDirectory(name) {
  return app.isPackaged ? path.join(process.resourcesPath, name) : path.join(__dirname, name);
}

function publishBundledPackageDirectory(next, name) {
  const source = bundledPackageDirectory(name);
  if (!fs.existsSync(source)) return 0;
  const destination = path.join(next.storageRoot, "artifacts", name);
  fs.mkdirSync(destination, { recursive: true });
  let copied = 0;
  for (const entry of fs.readdirSync(source, { withFileTypes: true })) {
    if (!entry.isFile() || entry.name.startsWith(".")) continue;
    fs.copyFileSync(path.join(source, entry.name), path.join(destination, entry.name));
    copied += 1;
  }
  return copied;
}

function publishBundledPackages(next) {
  const clients = publishBundledPackageDirectory(next, "client-packages");
  const runners = publishBundledPackageDirectory(next, "runner-packages");
  appendLog(`published packages clients=${clients} runners=${runners}`);
}

function logPath(kind) {
  const directory = userDataPath("logs");
  fs.mkdirSync(directory, { recursive: true });
  return path.join(directory, `${kind}.log`);
}

function appendLog(message) {
  fs.appendFileSync(logPath("server"), `${new Date().toISOString()} ${message}\n`);
}

function healthCheck(timeoutMs = 1200, port = settings?.port) {
  return new Promise((resolve) => {
    if (!port) return resolve(false);
    const request = http.get(`http://127.0.0.1:${port}/healthz`, { timeout: timeoutMs }, (response) => {
      const chunks = [];
      response.on("data", (chunk) => chunks.push(chunk));
      response.on("end", () => {
        try {
          const body = JSON.parse(Buffer.concat(chunks).toString("utf8"));
          const healthy = response.statusCode === 200 && body?.status === "ok" && body?.service === "agentcollab-hub" && (!settings?.hubId || body?.hub_id === settings.hubId);
          if (healthy) lastHealthyAt = new Date().toISOString();
          resolve(healthy);
        } catch {
          resolve(false);
        }
      });
    });
    request.on("timeout", () => { request.destroy(); resolve(false); });
    request.on("error", () => resolve(false));
  });
}

function portAvailable(port) {
  return new Promise((resolve) => {
    const probe = net.createServer();
    probe.unref();
    probe.once("error", () => resolve(false));
    probe.listen({ host: "0.0.0.0", port, exclusive: true }, () => probe.close(() => resolve(true)));
  });
}

async function findAvailablePort(preferredPort, attempts = 20) {
  for (let offset = 0; offset < attempts; offset += 1) {
    const candidate = preferredPort + offset;
    if (candidate > 65535) break;
    if (await portAvailable(candidate)) return candidate;
  }
  return 0;
}

async function ensureServerPort() {
  if (await healthCheck(700, settings.port) || await portAvailable(settings.port)) return;
  if (settings.addressMode !== "auto") {
    throw new Error(`端口 ${settings.port} 已被其他程序占用；固定地址模式不会自动更换端口`);
  }
  const nextPort = await findAvailablePort(settings.port + 1);
  if (!nextPort) throw new Error(`端口 ${settings.port} 已被占用，且没有找到可用备用端口`);
  const previousPublicUrl = settings.publicUrl;
  settings = {
    ...settings,
    port: nextPort,
    publicUrl: replaceURLPort(defaultPublicURL(nextPort), nextPort),
    previousPublicUrl,
    addressChangedAt: new Date().toISOString(),
  };
  saveSettings(settings);
  writeHubConfig(settings);
  appendLog(`port conflict recovered previous=${previousPublicUrl} current=${settings.publicUrl}`);
}

function discoveryURLs() {
  return Array.from(new Set([settings?.publicUrl, ...currentNetworkURLs(settings?.port || DEFAULT_PORT)].filter(Boolean)));
}

function startDiscoveryResponder() {
  if (discoverySocket || quitting) return;
  const socket = dgram.createSocket({ type: "udp4", reuseAddr: true });
  discoverySocket = socket;
  socket.on("message", (message, remote) => {
    if (message.toString("utf8").trim() !== "agentcollab-discover-v1" || !settings) return;
    const response = Buffer.from(JSON.stringify({ service: "agentcollab-hub", hub_id: settings.hubId, name: settings.hubName, urls: discoveryURLs() }));
    socket.send(response, remote.port, remote.address, (error) => {
      if (error) appendLog(`LAN discovery response failed: ${error.message}`);
    });
  });
  socket.on("error", (error) => {
    appendLog(`LAN discovery unavailable: ${error.message}`);
    try { socket.close(); } catch {}
    if (discoverySocket === socket) discoverySocket = undefined;
  });
  socket.bind(DISCOVERY_PORT, "0.0.0.0", () => appendLog(`LAN discovery listening udp=${DISCOVERY_PORT}`));
}

async function waitForHub(attempts = 40) {
  for (let index = 0; index < attempts; index += 1) {
    if (await healthCheck()) return true;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  return false;
}

function hubOutputOptions() {
  const output = fs.openSync(logPath("hub"), "a");
  return { cwd: app.getPath("userData"), windowsHide: true, stdio: ["ignore", output, output] };
}

async function startHub() {
  if (!settings || hubProcess?.exitCode === null) return;
  if (await healthCheck()) {
    hubHealthy = true;
    appendLog(`using existing Hub on port ${settings.port}`);
    startDiscoveryResponder();
    notifyStatus();
    return;
  }
  await ensureServerPort();
  const executable = runtimeExecutable();
  if (!fs.existsSync(executable)) throw new Error("Server 安装不完整：缺少 agentcollab-hub");
  hubProcess = spawn(executable, ["serve", "--config", configPath()], hubOutputOptions());
  appendLog(`hub started pid=${hubProcess.pid}`);
  hubProcess.on("error", (error) => appendLog(`hub process error: ${error.stack || error.message}`));
  hubProcess.on("exit", (code, signal) => {
    appendLog(`hub stopped code=${code} signal=${signal}`);
    hubProcess = undefined;
    hubHealthy = false;
    notifyStatus();
    if (!quitting && code !== 0) {
      clearTimeout(restartTimer);
      restartTimer = setTimeout(() => void startHub().catch((error) => appendLog(`restart failed: ${error.message}`)), 3000);
    }
  });
  if (!(await waitForHub())) throw new Error("Server 启动失败，请打开运行日志查看原因");
  hubHealthy = true;
  startDiscoveryResponder();
  notifyStatus();
}

async function stopHub() {
  clearTimeout(restartTimer);
  hubHealthy = false;
  if (!hubProcess || hubProcess.exitCode !== null) {
    notifyStatus();
    return;
  }
  await new Promise((resolve) => {
    const timer = setTimeout(() => { try { hubProcess.kill("SIGKILL"); } catch {} resolve(); }, 3000);
    hubProcess.once("exit", () => { clearTimeout(timer); resolve(); });
    try { hubProcess.kill("SIGTERM"); } catch { clearTimeout(timer); resolve(); }
  });
  hubProcess = undefined;
}

function requestQuit() {
  if (quitPromise) return quitPromise;
  quitting = true;
  clearInterval(networkTimer);
  clearTimeout(restartTimer);
  if (discoverySocket) {
    try { discoverySocket.close(); } catch {}
    discoverySocket = undefined;
  }
  quitPromise = stopHub().catch((error) => {
    appendLog(`stop during quit failed: ${error.stack || error.message}`);
  }).finally(() => {
    quitting = "stopped";
    // app.exit avoids re-entering the asynchronous before-quit path after the
    // managed Hub has already stopped.
    app.exit(0);
  });
  return quitPromise;
}

function state() {
  if (!settings) return defaultState();
  return {
    configured: true,
    running: hubHealthy || Boolean(hubProcess && hubProcess.exitCode === null),
    hubId: settings.hubId,
    hubName: settings.hubName,
    publicUrl: settings.publicUrl,
    addressMode: settings.addressMode,
    networkUrls: currentNetworkURLs(settings.port),
    previousPublicUrl: settings.previousPublicUrl || "",
    addressChangedAt: settings.addressChangedAt || "",
    networkUrls: discoveryURLs(),
    lastHealthyAt,
    localUrl: `http://127.0.0.1:${settings.port}`,
    storageRoot: settings.storageRoot,
    port: settings.port,
    version: app.getVersion(),
  };
}

function notifyStatus() {
  if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.send("server:status", state());
  updateTrayMenu();
}

async function refreshAutomaticAddress() {
  if (!settings || settings.addressMode !== "auto" || addressRefreshRunning || quitting) return false;
  const detected = primaryLANURL(os.networkInterfaces(), settings.port);
  if (!detected || detected === settings.publicUrl) {
    pendingPublicUrl = "";
    pendingPublicUrlPolls = 0;
    return false;
  }
  if (pendingPublicUrl !== detected) {
    pendingPublicUrl = detected;
    pendingPublicUrlPolls = 1;
    appendLog(`candidate team address detected current=${settings.publicUrl} candidate=${detected}`);
    return false;
  }
  pendingPublicUrlPolls += 1;
  if (pendingPublicUrlPolls < ADDRESS_STABILITY_POLLS) return false;
  addressRefreshRunning = true;
  try {
    const previousPublicUrl = settings.publicUrl;
    settings = { ...settings, publicUrl: detected, previousPublicUrl, addressChangedAt: new Date().toISOString() };
    saveSettings(settings);
    writeHubConfig(settings);
    // The Hub listens on 0.0.0.0, so changing the advertised LAN address does
    // not require restarting it. Restarting here used to drop every active
    // Client connection whenever DHCP/Wi-Fi changed the machine's address.
    // Keep the process alive; the discovery responder immediately advertises
    // the new address and Clients can migrate without an avoidable restart.
    appendLog(`team address changed without hub restart previous=${previousPublicUrl} current=${detected}`);
    pendingPublicUrl = "";
    pendingPublicUrlPolls = 0;
    notifyStatus();
    return true;
  } finally {
    addressRefreshRunning = false;
  }
}

function startNetworkMonitor() {
  clearInterval(networkTimer);
  networkTimer = setInterval(() => {
    if (maintenanceRunning || quitting) return;
    maintenanceRunning = true;
    void (async () => {
      const changed = await refreshAutomaticAddress();
      if (!changed && !(await healthCheck(1800))) {
        appendLog("health monitor detected an unavailable Hub; restarting");
        await stopHub();
        await startHub();
      }
    })().catch((error) => appendLog(`network maintenance failed: ${error.stack || error.message}`)).finally(() => {
      maintenanceRunning = false;
    });
  }, 5000);
}

function createInitialAdmin(next, input) {
  const databasePath = path.join(next.storageRoot, "data", "agentcollab.db");
  if (fs.existsSync(databasePath)) return;
  const password = String(input.password ?? "");
  if (!password) throw new Error("请设置 owner 密码");
  const executable = runtimeExecutable();
  const result = spawnSync(executable, [
    "admin", "create", "--config", configPath(), "--username", "owner", "--display-name", String(input.displayName || "Owner").trim(),
  ], {
    cwd: app.getPath("userData"),
    env: { ...process.env, AGENTCOLLAB_ADMIN_PASSWORD: password },
    encoding: "utf8",
    windowsHide: true,
  });
  if (result.status !== 0) throw new Error((result.stderr || result.stdout || "无法创建 owner 管理员").trim());
}

async function configure(input) {
  const hubName = String(input?.hubName ?? "").trim();
  const storageRoot = path.resolve(String(input?.storageRoot ?? "").trim());
  const port = Number(input?.port || DEFAULT_PORT);
  const addressMode = input?.addressMode === "manual" ? "manual" : "auto";
  const publicUrl = addressMode === "auto" ? defaultPublicURL(port) : normalizeHubURL(input?.publicUrl);
  if (!hubName) throw new Error("请填写协作中心名称");
  if (!storageRoot) throw new Error("请设置数据目录");
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error("服务端口无效");
  const next = { version: SETTINGS_VERSION, hubId: settings?.hubId || randomUUID(), hubName, publicUrl, addressMode, storageRoot, port, autoStart: false, previousPublicUrl: "", addressChangedAt: "" };
  await stopHub();
  writeHubConfig(next);
  createInitialAdmin(next, input);
  publishBundledPackages(next);
  settings = next;
  saveSettings(next);
  if (!isolatedData) app.setLoginItemSettings({ openAtLogin: false });
  await startHub();
  startNetworkMonitor();
  await loadDashboard();
  return state();
}

function trustedLocalPage(event) {
  const value = event.senderFrame?.url ?? "";
  if (!value.startsWith("file:")) throw new Error("拒绝来自非 Server 管理页面的操作");
}

function localHubURL(pathname = "/") {
  if (!settings) throw new Error("Server 尚未完成设置");
  return new URL(pathname, `http://127.0.0.1:${settings.port}/`).toString();
}

async function openLocalHub(pathname = "/") {
  if (!(await healthCheck())) throw new Error("Server 尚未就绪，请先重新启动 Server");
  const target = localHubURL(pathname);
  await shell.openExternal(target);
  return { url: target };
}

function installedClientCandidates() {
  if (process.platform === "darwin") {
    return ["/Applications/AgentCollab.app", path.join(os.homedir(), "Applications", "AgentCollab.app")];
  }
  if (process.platform === "win32") {
    return [
      path.join(process.env.LOCALAPPDATA || "", "Programs", "AgentCollab", "AgentCollab.exe"),
      path.join(process.env.PROGRAMFILES || "", "AgentCollab", "AgentCollab.exe"),
    ].filter(Boolean);
  }
  return ["/usr/bin/agentcollab", "/usr/local/bin/agentcollab"];
}

async function openClient() {
  const candidate = installedClientCandidates().find((item) => item && fs.existsSync(item));
  if (!candidate) throw new Error("尚未安装 AgentCollab Client，请先点击“客户端下载”安装");
  const teamLink = `agentcollab://join?hub=${encodeURIComponent(settings.publicUrl)}`;
  if (process.platform === "darwin") {
    const result = spawnSync("/usr/bin/open", ["-a", candidate, "--args", teamLink], { encoding: "utf8" });
    if (result.status !== 0) throw new Error((result.stderr || "无法打开 AgentCollab Client").trim());
  } else {
    const child = spawn(candidate, [teamLink], { detached: true, windowsHide: true, stdio: "ignore" });
    child.unref();
  }
  return { path: candidate };
}

function configureIPC() {
  ipcMain.handle("server:get-state", (event) => { trustedLocalPage(event); return state(); });
  ipcMain.handle("server:configure", async (event, input) => { trustedLocalPage(event); return configure(input); });
  ipcMain.handle("server:restart", async (event) => { trustedLocalPage(event); await stopHub(); await startHub(); return state(); });
  ipcMain.handle("server:open-client", async (event) => { trustedLocalPage(event); return openClient(); });
  ipcMain.handle("server:open-hub", async (event) => { trustedLocalPage(event); return openLocalHub("/"); });
  ipcMain.handle("server:open-admin", async (event) => { trustedLocalPage(event); return openLocalHub("/?admin=1"); });
  ipcMain.handle("server:open-downloads", async (event) => { trustedLocalPage(event); return openLocalHub("/downloads/client"); });
  ipcMain.handle("server:copy-team-url", (event) => {
    trustedLocalPage(event);
    if (!settings) throw new Error("Server 尚未完成设置");
    const copied = settings.publicUrl;
    clipboard.writeText(copied);
    if (settings.previousPublicUrl || settings.addressChangedAt) {
      settings = { ...settings, previousPublicUrl: "", addressChangedAt: "" };
      saveSettings(settings);
      notifyStatus();
    }
    return copied;
  });
  ipcMain.handle("server:open-data", async (event) => { trustedLocalPage(event); await shell.openPath(settings.storageRoot); });
  ipcMain.handle("server:open-logs", async (event) => { trustedLocalPage(event); await shell.openPath(path.dirname(logPath("server"))); });
  ipcMain.handle("server:open-settings", async (event) => { trustedLocalPage(event); await loadOnboarding(); });
  ipcMain.handle("server:quit", (event) => {
    trustedLocalPage(event);
    setTimeout(() => { void requestQuit(); }, 80);
    return { stopping: true };
  });
}

function createWindow() {
  mainWindow = new BrowserWindow({
    title: APP_NAME,
    width: 940,
    height: 720,
    minWidth: 760,
    minHeight: 600,
    show: false,
    backgroundColor: "#f7f9fc",
    autoHideMenuBar: process.platform !== "darwin",
    webPreferences: { preload: path.join(__dirname, "preload.cjs"), contextIsolation: true, nodeIntegration: false, sandbox: true },
  });
  mainWindow.once("ready-to-show", () => {
    if (shouldRevealInitialWindow(process.argv, Boolean(settings))) mainWindow.show();
  });
  mainWindow.on("close", (event) => { if (!quitting) { event.preventDefault(); mainWindow.hide(); } });
  mainWindow.on("closed", () => { mainWindow = undefined; });
}

function showWindow() {
  if (!mainWindow || mainWindow.isDestroyed()) createWindow();
  mainWindow.show();
  mainWindow.focus();
}

function createTrayImage() {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect x="2" y="2" width="28" height="28" rx="8" fill="#172235"/><circle cx="11" cy="11" r="3" fill="#fff"/><circle cx="21" cy="11" r="3" fill="#fff"/><circle cx="11" cy="21" r="3" fill="#fff"/><circle cx="21" cy="21" r="3" fill="#fff"/></svg>`;
  const image = nativeImage.createFromBuffer(Buffer.from(svg)).resize({ width: 18, height: 18 });
  if (process.platform === "darwin") image.setTemplateImage(true);
  return image;
}

function updateTrayMenu() {
  if (!tray) return;
  tray.setToolTip(`${APP_NAME}${settings ? (state().running ? " · 运行中" : " · 已停止") : " · 未设置"}`);
  tray.setContextMenu(Menu.buildFromTemplate([
    { label: "打开 Server 管理", click: () => showWindow() },
    { label: "打开 AgentCollab Client", enabled: Boolean(settings), click: () => void openClient().catch((error) => appendLog(`open Client failed: ${error.message}`)) },
    { label: "网页版（备用）", enabled: Boolean(settings), click: () => void openLocalHub("/").catch((error) => appendLog(`open web fallback failed: ${error.message}`)) },
    { label: "复制团队地址", enabled: Boolean(settings), click: () => { if (settings) clipboard.writeText(settings.publicUrl); } },
    { type: "separator" },
    { label: "退出并停止 Server", click: () => { void requestQuit(); } },
  ]));
}

function createTray() {
  tray = new Tray(createTrayImage());
  tray.on("click", showWindow);
  updateTrayMenu();
}

async function loadOnboarding(reveal = true) {
  await mainWindow.loadFile(path.join(__dirname, "onboarding", "index.html"));
  if (reveal) showWindow();
}

async function loadDashboard(reveal = true) {
  await mainWindow.loadFile(path.join(__dirname, "dashboard", "index.html"));
  if (reveal) showWindow();
}

async function startApplication() {
  settings = loadSettings();
  configureIPC();
  createWindow();
  createTray();
  if (!settings) return loadOnboarding();
  const revealInitialWindow = shouldRevealInitialWindow(process.argv, true);
  writeHubConfig(settings);
  publishBundledPackages(settings);
  if (!isolatedData) app.setLoginItemSettings({ openAtLogin: false });
  try {
    await startHub();
    startNetworkMonitor();
    await loadDashboard(revealInitialWindow);
  } catch (error) {
    appendLog(`startup failed: ${error.stack || error.message}`);
    await loadDashboard(revealInitialWindow);
  }
}

const acquiredLock = app.requestSingleInstanceLock();
if (!acquiredLock) app.exit(0);
else {
  app.on("second-instance", showWindow);
  app.whenReady().then(startApplication).catch((error) => { appendLog(`fatal: ${error.stack || error.message}`); app.quit(); });
}

app.on("activate", showWindow);
app.on("window-all-closed", () => { if (process.platform !== "darwin" && quitting) app.quit(); });
app.on("before-quit", (event) => {
  if (quitting === "stopped") return;
  event.preventDefault();
  void requestQuit();
});

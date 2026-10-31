const { app, BrowserWindow, Menu, Tray, ipcMain, nativeImage, shell, dialog } = require("electron");
const { spawn, spawnSync } = require("node:child_process");
const dgram = require("node:dgram");
const fs = require("node:fs");
const http = require("node:http");
const https = require("node:https");
const os = require("node:os");
const path = require("node:path");
const { standardRunnerConfigPath } = require("./config-paths.cjs");
const { versionNamespace, versionedUserDataName } = require("./major-version.cjs");
const { broadcastAddresses, expandSameHostPorts, hubProxyBypassRules, mergeHubURLs, normalizeHubURL, parseDiscoveryResponse, parseTeamInput } = require("./network.cjs");

const APP_NAME = "AgentCollab";
const APP_VERSION = app.getVersion();
const APP_VERSION_NAMESPACE = versionNamespace(APP_VERSION);
const SETTINGS_VERSION = 6;
const DISCOVERY_PORT = 41729;

let mainWindow;
let tray;
let settings;
let quitting = false;
let runnerProcess;
let runnerRestartTimer;
let showingHubUnavailable = false;
let connectionTimer;
let connectionCheckRunning = false;
let consecutiveHubFailures = 0;
let configuredProxyBypassRules = "";
let pendingTeamLink = process.argv.find((value) => String(value).startsWith("agentcollab://")) || "";
let pendingInviteCode = "";

app.setName(APP_NAME);
const isolatedData = process.env.AGENTCOLLAB_CLIENT_DATA_DIR;
if (isolatedData && !path.isAbsolute(isolatedData)) throw new Error("AGENTCOLLAB_CLIENT_DATA_DIR must be absolute");
app.setPath("userData", isolatedData || path.join(app.getPath("appData"), versionedUserDataName(APP_NAME, APP_VERSION)));
if (isolatedData) process.env.AGENTCOLLAB_INSTALLATION_ID_FILE = path.join(isolatedData, "installation-id");

function userDataPath(...parts) {
  return path.join(app.getPath("userData"), ...parts);
}

function runnerConfigPath() {
  if (isolatedData) return userDataPath("runner.yaml");
  return standardRunnerConfigPath(os.homedir(), APP_VERSION_NAMESPACE);
}

function defaultCredentialsPath() {
  if (isolatedData) return userDataPath("runner", "device.json");
  return path.join(os.homedir(), ".agentcollab", APP_VERSION_NAMESPACE, "runner", "device.json");
}

function runtimeExecutable(name) {
  const suffix = process.platform === "win32" ? ".exe" : "";
  const base = app.isPackaged ? path.join(process.resourcesPath, "runtime") : path.join(__dirname, "runtime");
  return path.join(base, `${name}${suffix}`);
}

function readSimpleYAMLValue(filePath, key) {
  try {
    const content = fs.readFileSync(filePath, "utf8");
    const match = content.match(new RegExp(`^\\s*${key}\\s*:\\s*(.+?)\\s*$`, "m"));
    if (!match) return "";
    const value = match[1].trim();
    if ((value.startsWith("\"") && value.endsWith("\"")) || (value.startsWith("'") && value.endsWith("'"))) {
      return value.slice(1, -1).replaceAll("\\\\", "\\");
    }
    return value;
  } catch {
    return "";
  }
}

function runnerCredentialsPath() {
	return readSimpleYAMLValue(runnerConfigPath(), "credentials_file") || defaultCredentialsPath();
}

function projectBindingsPath() {
  return path.join(path.dirname(runnerCredentialsPath()), "project-bindings.json");
}

function loadProjectBindings() {
  try {
    const parsed = JSON.parse(fs.readFileSync(projectBindingsPath(), "utf8"));
    return parsed?.version === 1 && parsed?.bindings && typeof parsed.bindings === "object"
      ? parsed : { version: 1, bindings: {} };
  } catch {
    return { version: 1, bindings: {} };
  }
}

function saveProjectBindings(value) {
  const target = projectBindingsPath();
  fs.mkdirSync(path.dirname(target), { recursive: true, mode: 0o700 });
  const temporary = `${target}.tmp`;
  fs.writeFileSync(temporary, `${JSON.stringify(value, null, 2)}\n`, { mode: 0o600 });
  fs.renameSync(temporary, target);
}

function publicProjectBindings() {
  const value = loadProjectBindings();
  return Object.values(value.bindings).map((binding) => ({
    stepId: String(binding.stepId || ""),
    workspaceId: String(binding.workspaceId || ""),
    directoryLabel: String(binding.directoryLabel || ""),
    directoryPath: String(binding.directoryPath || ""),
    available: Boolean(binding.directoryPath && fs.existsSync(binding.directoryPath)),
    updatedAt: String(binding.updatedAt || ""),
  }));
}

function runnerIsRegistered() {
	return fs.existsSync(runnerConfigPath()) && fs.existsSync(runnerCredentialsPath());
}

function runnerIdentity() {
  try {
    const credentials = JSON.parse(fs.readFileSync(runnerCredentialsPath(), "utf8"));
    return {
      runnerId: String(credentials.runner_id || ""),
      deviceId: String(credentials.device_id || ""),
      workspaceId: String(credentials.workspace_id || ""),
    };
  } catch {
    return { runnerId: "", deviceId: "", workspaceId: "" };
  }
}

function settingsPath() {
  return userDataPath("desktop-settings.json");
}

function loadSettings() {
	try {
		const loaded = JSON.parse(fs.readFileSync(settingsPath(), "utf8"));
		if ([5, SETTINGS_VERSION].includes(loaded?.version) && loaded?.hubUrl) {
			const normalized = {
				version: SETTINGS_VERSION,
				hubUrl: normalizeHubURL(loaded.hubUrl),
				hubUrls: mergeHubURLs([loaded.hubUrl], loaded.hubUrls),
				hubId: String(loaded.hubId || ""),
				hubName: String(loaded.hubName || ""),
				autoStart: false,
				runnerConfigPath: runnerConfigPath(),
			};
			saveSettings(normalized);
			return normalized;
    }
  } catch {
    // A new major release intentionally starts without reading legacy state.
  }
  return undefined;
}

function saveSettings(next) {
  fs.mkdirSync(path.dirname(settingsPath()), { recursive: true, mode: 0o700 });
  const temporary = `${settingsPath()}.tmp`;
  fs.writeFileSync(temporary, `${JSON.stringify(next, null, 2)}\n`, { mode: 0o600 });
  fs.renameSync(temporary, settingsPath());
}

function logPath(kind) {
  const directory = userDataPath("logs");
  fs.mkdirSync(directory, { recursive: true });
  return path.join(directory, `${kind}.log`);
}

function appendDesktopLog(message) {
  const line = `${new Date().toISOString()} ${message}\n`;
  fs.appendFileSync(logPath("desktop"), line);
}

function processOptions(kind, stdin = "ignore") {
  const output = fs.openSync(logPath(kind), "a");
  return {
    cwd: app.getPath("userData"),
    env: { ...process.env, AGENTCOLLAB_DESKTOP: "1" },
    windowsHide: true,
    stdio: [stdin, output, output],
  };
}

function notifyStatus() {
  if (!mainWindow || mainWindow.isDestroyed()) return;
  mainWindow.webContents.send("desktop:status", desktopState());
}

function desktopState() {
  const identity = runnerIdentity();
  return {
    configured: Boolean(settings),
    hubUrl: settings?.hubUrl,
    hubUrls: settings?.hubUrls ?? [],
    hubId: settings?.hubId ?? "",
    hubName: settings?.hubName ?? "",
    runnerRegistered: runnerIsRegistered(),
    runnerRunning: Boolean(runnerProcess && runnerProcess.exitCode === null),
    hubManaged: false,
    runnerId: identity.runnerId,
    deviceId: identity.deviceId,
    workspaceId: identity.workspaceId,
    platform: process.platform,
    version: app.getVersion(),
  };
}

function spawnManaged(kind, executable, args, onExit) {
  if (!fs.existsSync(executable)) throw new Error(`应用安装不完整：缺少 ${path.basename(executable)}`);
  const child = spawn(executable, args, processOptions(kind));
  appendDesktopLog(`${kind} started pid=${child.pid} executable=${executable}`);
  child.on("error", (error) => appendDesktopLog(`${kind} process error: ${error.stack || error.message}`));
  child.on("exit", (code, signal) => {
    appendDesktopLog(`${kind} stopped code=${code} signal=${signal}`);
    onExit?.(code, signal);
    notifyStatus();
  });
  return child;
}

function requestJSON(target, timeoutMs = 1200) {
  return new Promise((resolve) => {
    const transport = target.protocol === "https:" ? https : http;
    const request = transport.get(target, { timeout: timeoutMs }, (response) => {
      const chunks = [];
      response.on("data", (chunk) => chunks.push(chunk));
      response.on("end", () => {
        if (response.statusCode !== 200) return resolve(undefined);
        try { resolve(JSON.parse(Buffer.concat(chunks).toString("utf8"))); }
        catch { resolve(undefined); }
      });
    });
    request.on("timeout", () => {
      request.destroy();
      resolve(undefined);
    });
    request.on("error", () => resolve(undefined));
  });
}

async function inspectHub(hubURL, timeoutMs = 1200, expectedHubId = "") {
  let normalized;
  try { normalized = normalizeHubURL(hubURL); } catch { return undefined; }
  const info = await requestJSON(new URL("/api/v1/hub/info", normalized), timeoutMs);
  if (!info || info.service !== "agentcollab-hub" || !info.hub_id || !info.name || !info.canonical_url) return undefined;
  if (expectedHubId && info.hub_id !== expectedHubId) return undefined;
  return {
    activeUrl: normalized,
    hubId: String(info.hub_id),
    name: String(info.name),
    version: String(info.version || ""),
    candidateUrls: mergeHubURLs([normalized], info.candidate_urls, [info.canonical_url]),
  };
}

async function healthCheck(hubURL, timeoutMs = 1200) {
  return Boolean(await inspectHub(hubURL, timeoutMs));
}

function discoverLANHubURLs(expectedHubId = "", timeoutMs = 850) {
  return new Promise((resolve) => {
    const found = [];
    const socket = dgram.createSocket({ type: "udp4", reuseAddr: true });
    let finished = false;
    const finish = () => {
      if (finished) return;
      finished = true;
      try { socket.close(); } catch {}
      resolve(mergeHubURLs(found));
    };
    socket.on("message", (message) => {
      const response = parseDiscoveryResponse(message);
      if (!response || (expectedHubId && response.hubId !== expectedHubId)) return;
      found.push(...response.urls);
    });
    socket.on("error", (error) => {
      appendDesktopLog(`LAN discovery unavailable: ${error.message}`);
      finish();
    });
    socket.bind(0, "0.0.0.0", () => {
      try { socket.setBroadcast(true); } catch {}
      const payload = Buffer.from("agentcollab-discover-v1");
      for (const address of broadcastAddresses(os.networkInterfaces())) {
        socket.send(payload, DISCOVERY_PORT, address, () => {});
      }
    });
    setTimeout(finish, timeoutMs);
  });
}

async function inspectCandidateURLs(urls, timeoutMs = 1100, expectedHubId = "") {
  const candidates = expandSameHostPorts(mergeHubURLs(urls));
  const results = await Promise.all(candidates.map((url) => inspectHub(url, timeoutMs, expectedHubId)));
  return results.find(Boolean);
}

async function resolveHub(urls, includeLANDiscovery = true, expectedHubId = "") {
  const direct = await inspectCandidateURLs(urls, 1100, expectedHubId);
  if (direct) return direct;
  if (!includeLANDiscovery) return undefined;
  const discovered = await discoverLANHubURLs(expectedHubId);
  return inspectCandidateURLs(discovered, 1100, expectedHubId);
}

function migrateLegacyServices() {
  if (isolatedData || !app.isPackaged) return;
  if (process.platform === "darwin") {
    const domain = `gui/${process.getuid()}`;
    // The Client owns only the local Runner. A legacy Hub service may still be
    // serving the team until the separate Server application is installed, so
    // upgrading the Client must never stop it.
    for (const label of ["com.agentcollab.runner"]) {
      spawnSync("/bin/launchctl", ["bootout", `${domain}/${label}`], { stdio: "ignore" });
      spawnSync("/bin/launchctl", ["disable", `${domain}/${label}`], { stdio: "ignore" });
      appendDesktopLog(`disabled legacy launch service ${label}`);
    }
    return;
  }
  if (process.platform === "win32") {
    // A matching executable name does not establish ownership. Other profiles
    // or teams may have live tasks; never kill all Runners during migration.
    // The Client manages only the process it starts through runnerProcess.
    const startupShortcut = path.join(
      process.env.APPDATA || "",
      "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "AgentCollab Client.lnk",
    );
    try {
      if (fs.existsSync(startupShortcut)) fs.unlinkSync(startupShortcut);
    } catch (error) {
      appendDesktopLog(`legacy startup shortcut cleanup failed: ${error.message}`);
    }
  }
}

async function waitForHub(hubURL, attempts = 40) {
  for (let index = 0; index < attempts; index += 1) {
    if (await healthCheck(hubURL)) return true;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  return false;
}

function startRunner() {
  if (!runnerIsRegistered() || runnerProcess?.exitCode === null) return false;
  clearTimeout(runnerRestartTimer);
	runnerProcess = spawnManaged("runner", runtimeExecutable("agentcollab-runner"), ["serve", "--config", runnerConfigPath()], (code) => {
    runnerProcess = undefined;
    if (!quitting && code !== 0 && runnerIsRegistered()) {
      runnerRestartTimer = setTimeout(() => {
        try {
          startRunner();
        } catch (error) {
          appendDesktopLog(`runner restart failed: ${error.stack || error.message}`);
        }
      }, 3000);
    }
  });
  notifyStatus();
  return true;
}

async function stopChild(child) {
  if (!child || child.exitCode !== null) return;
  await new Promise((resolve) => {
    const timer = setTimeout(() => {
      try { child.kill("SIGKILL"); } catch {}
      resolve();
    }, 2500);
    child.once("exit", () => {
      clearTimeout(timer);
      resolve();
    });
    try { child.kill("SIGTERM"); } catch {
      clearTimeout(timer);
      resolve();
    }
  });
}

function updateRunnerHubURL(hubUrl) {
  const target = runnerConfigPath();
  if (!fs.existsSync(target)) return false;
  const content = fs.readFileSync(target, "utf8");
  const line = `hub_url: ${JSON.stringify(hubUrl)}`;
  const next = /^\s*hub_url\s*:.*$/m.test(content)
    ? content.replace(/^\s*hub_url\s*:.*$/m, line)
    : `${line}\n${content}`;
  if (next === content) return false;
  const temporary = `${target}.tmp`;
  fs.writeFileSync(temporary, next, { mode: 0o600 });
  fs.renameSync(temporary, target);
  return true;
}

async function adoptHub(info, restartRunner = true) {
  const previousHubUrl = settings?.hubUrl || "";
  const hubUrls = mergeHubURLs([info.activeUrl], info.candidateUrls, settings?.hubUrls);
  const nextSettings = {
    version: SETTINGS_VERSION,
    hubUrl: info.activeUrl,
    hubUrls,
    hubId: info.hubId,
    hubName: info.name,
    autoStart: false,
    runnerConfigPath: runnerConfigPath(),
  };
  const settingsChanged = JSON.stringify(settings || {}) !== JSON.stringify(nextSettings);
  settings = nextSettings;
  if (settingsChanged) saveSettings(settings);
  const changed = previousHubUrl && previousHubUrl !== info.activeUrl;
  let configuredRunnerHub = "";
  try { configuredRunnerHub = normalizeHubURL(readSimpleYAMLValue(runnerConfigPath(), "hub_url")); } catch {}
  const runnerChanged = configuredRunnerHub !== info.activeUrl && updateRunnerHubURL(info.activeUrl);
  if (changed || runnerChanged) {
    appendDesktopLog(`team endpoint recovered previous=${previousHubUrl} current=${info.activeUrl}`);
    if (runnerChanged && restartRunner && runnerIsRegistered()) {
      await stopChild(runnerProcess);
      runnerProcess = undefined;
      startRunner();
    }
  }
  notifyStatus();
  return changed;
}

async function configureHubNetworkSession(urls) {
  if (!mainWindow || mainWindow.isDestroyed()) return;
  const rules = hubProxyBypassRules(urls);
  if (rules === configuredProxyBypassRules) return;
  const networkSession = mainWindow.webContents.session;
  // The Client session is dedicated to the verified Hub. External links are
  // always handed to the system browser, so explicit direct mode is both safer
  // and more reliable than inheriting a corporate proxy that may intercept LAN
  // addresses. Node-side Hub verification already uses the same direct route.
  await networkSession.setProxy({ mode: "direct" });
  await networkSession.closeAllConnections();
  configuredProxyBypassRules = rules;
  appendDesktopLog(`Hub proxy bypass updated rules=${rules}`);
}

async function loadURLWithTimeout(url, timeoutMs = 12_000) {
  let timer;
  try {
    await Promise.race([
      mainWindow.loadURL(url),
      new Promise((_, reject) => {
        timer = setTimeout(() => {
          try { mainWindow.webContents.stop(); } catch {}
          reject(new Error(`页面加载超过 ${Math.round(timeoutMs / 1000)} 秒`));
        }, timeoutMs);
      }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}

async function reconnectRunner() {
  clearTimeout(runnerRestartTimer);
  await stopChild(runnerProcess);
  runnerProcess = undefined;
  if (!runnerIsRegistered()) throw new Error("这台电脑尚未完成首次连接");
  startRunner();
  return desktopState();
}

function runRunnerConnect({ hubUrl, inviteCode }) {
  return new Promise((resolve, reject) => {
    const normalizedHubURL = normalizeHubURL(hubUrl);
    const code = String(inviteCode ?? "").trim();
    if (code.length < 6 || code.length > 512) {
      reject(new Error("设备接入码无效，请重新生成"));
      return;
    }
    const executable = runtimeExecutable("agentcollab-runner");
    if (!fs.existsSync(executable)) {
      reject(new Error("应用安装不完整：缺少本机 Agent 服务"));
      return;
    }
    const output = [];
    const child = spawn(executable, [
      "connect",
		"--config", runnerConfigPath(),
      "--hub-url", normalizedHubURL,
      "--device-name", os.hostname(),
      "--invite-code-stdin",
    ], {
      cwd: app.getPath("userData"),
      env: { ...process.env, AGENTCOLLAB_DESKTOP: "1" },
      windowsHide: true,
      stdio: ["pipe", "pipe", "pipe"],
    });
    child.stdout.on("data", (chunk) => output.push(chunk));
    child.stderr.on("data", (chunk) => output.push(chunk));
    child.stdin.end(`${code}\n`);
    const timer = setTimeout(() => {
      child.kill("SIGKILL");
      reject(new Error("连接超时，请确认当前网络能够访问协作中心"));
    }, 60_000);
    child.on("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
    child.on("exit", async (exitCode) => {
      clearTimeout(timer);
      const message = Buffer.concat(output).toString("utf8").trim();
      appendDesktopLog(`runner connect exit=${exitCode} ${message}`);
      if (exitCode !== 0) {
        reject(new Error(message || "无法连接这台电脑"));
        return;
      }
      try {
        await stopChild(runnerProcess);
        runnerProcess = undefined;
        startRunner();
        resolve(desktopState());
      } catch (error) {
        reject(error);
      }
    });
  });
}

function trustedHubOrigin() {
  try {
    return settings ? new URL(settings.hubUrl).origin : "";
  } catch {
    return "";
  }
}

function assertTrustedSender(event, allowOnboarding = false) {
  const frameURL = event.senderFrame?.url ?? "";
  if (allowOnboarding && frameURL.startsWith("file:")) return;
  let origin = "";
  try { origin = new URL(frameURL).origin; } catch {}
  if (!origin || origin !== trustedHubOrigin()) throw new Error("拒绝来自非协作中心页面的本机操作");
}

function configureIPC() {
  ipcMain.handle("desktop:get-state", (event) => {
    assertTrustedSender(event, true);
    return desktopState();
  });

  ipcMain.handle("desktop:configure", async (event, input) => {
    assertTrustedSender(event, true);
    const intent = parseTeamInput(input?.hubUrl);
    // Resolve the address to a verified AgentCollab Hub and learn all current
    // LAN aliases before persisting it. This prevents blank starts and lets the
    // Client recover when the Server changes IP or its automatic port.
    const info = await resolveHub([intent.hubUrl], true, "");
    if (!info) throw new Error("没有找到可用的协作中心，请确认 Server 正在运行，并检查地址和当前网络");
    await adoptHub(info, false);
    if (!isolatedData) app.setLoginItemSettings({ openAtLogin: false });
    startRunner();
    await loadHub(intent.inviteCode);
    return desktopState();
  });

  ipcMain.handle("desktop:open-team-link", (event, inviteLink) => {
    assertTrustedSender(event);
    const rawLink = String(inviteLink || "").trim();
    // Validate synchronously so the current page can show a useful error. Start
    // the navigation after the IPC reply; otherwise reloading the renderer can
    // destroy the reply channel before the form receives success.
    const intent = parseTeamInput(rawLink);
    if (!intent.inviteCode) throw new Error("这不是完整的 AgentCollab 邀请链接");
    setImmediate(() => void openTeamLink(rawLink));
    return true;
  });

  ipcMain.handle("desktop:connect-current", async (event, input) => {
    assertTrustedSender(event);
    const target = normalizeHubURL(input?.hubUrl);
    if (new URL(target).origin !== trustedHubOrigin()) throw new Error("设备只能连接当前打开的协作中心");
    return runRunnerConnect({ hubUrl: target, inviteCode: input?.inviteCode });
  });

  ipcMain.handle("desktop:reconnect", async (event) => {
    assertTrustedSender(event);
    return reconnectRunner();
  });

  ipcMain.handle("desktop:open-settings", async (event) => {
    assertTrustedSender(event);
    await loadOnboarding();
    return true;
  });

  ipcMain.handle("desktop:list-project-bindings", (event) => {
    assertTrustedSender(event);
    return publicProjectBindings();
  });

  ipcMain.handle("desktop:bind-project-step", async (event, input) => {
    assertTrustedSender(event);
    const stepId = String(input?.stepId || "").trim();
    const workspaceId = String(input?.workspaceId || "").trim();
    if (!stepId || !workspaceId || !runnerIsRegistered()) throw new Error("请先连接这台电脑，并确认项目步骤有效");
    const selection = await dialog.showOpenDialog(mainWindow, {
      title: "选择这个步骤在本机持续维护的目录",
      buttonLabel: "绑定此目录",
      properties: ["openDirectory", "createDirectory"],
    });
    if (selection.canceled || !selection.filePaths[0]) return undefined;
    const directoryPath = path.resolve(selection.filePaths[0]);
    const value = loadProjectBindings();
    const binding = {
      stepId,
      workspaceId,
      directoryPath,
      directoryLabel: path.basename(directoryPath) || directoryPath,
      updatedAt: new Date().toISOString(),
    };
    value.bindings[stepId] = binding;
    saveProjectBindings(value);
    return { ...binding, available: true };
  });

  ipcMain.handle("desktop:unbind-project-step", (event, input) => {
    assertTrustedSender(event);
    const stepId = String(input?.stepId || "").trim();
    if (!stepId) throw new Error("项目步骤无效");
    const value = loadProjectBindings();
    delete value.bindings[stepId];
    saveProjectBindings(value);
    return true;
  });

  ipcMain.handle("desktop:open-project-step", async (event, input) => {
    assertTrustedSender(event);
    const stepId = String(input?.stepId || "").trim();
    const binding = loadProjectBindings().bindings[stepId];
    if (!binding?.directoryPath) throw new Error("这个步骤尚未绑定本机目录");
    const error = await shell.openPath(binding.directoryPath);
    if (error) throw new Error(error);
    return true;
  });
}

function createTrayImage() {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect x="2" y="2" width="28" height="28" rx="8" fill="#3568ea"/><circle cx="11" cy="11" r="3" fill="white"/><circle cx="21" cy="11" r="3" fill="white"/><circle cx="11" cy="21" r="3" fill="white"/><circle cx="21" cy="21" r="3" fill="white"/></svg>`;
  const image = nativeImage.createFromBuffer(Buffer.from(svg)).resize({ width: 18, height: 18 });
  if (process.platform === "darwin") image.setTemplateImage(true);
  return image;
}

function createTray() {
  tray = new Tray(createTrayImage());
  tray.setToolTip(APP_NAME);
  tray.on("click", () => showWindow());
  updateTrayMenu();
}

function updateTrayMenu() {
  if (!tray) return;
  tray.setContextMenu(Menu.buildFromTemplate([
    { label: "打开 AgentCollab", click: () => showWindow() },
    { label: runnerIsRegistered() ? "重新连接本机 Agent" : "本机 Agent 尚未连接", enabled: runnerIsRegistered(), click: () => void reconnectRunner() },
    { type: "separator" },
    { label: "团队连接", click: () => void loadOnboarding() },
    { label: "打开运行日志", click: () => shell.openPath(path.dirname(logPath("desktop"))) },
    { type: "separator" },
    { label: "退出", click: () => { quitting = true; app.quit(); } },
  ]));
}

function showWindow() {
  if (!mainWindow || mainWindow.isDestroyed()) {
    createWindow();
    void (settings ? loadHub() : loadOnboarding());
    return;
  }
  mainWindow.show();
  mainWindow.focus();
}

function createWindow() {
  mainWindow = new BrowserWindow({
    title: APP_NAME,
    width: 1480,
    height: 940,
    minWidth: 980,
    minHeight: 680,
    show: false,
    backgroundColor: "#f7f9fc",
    autoHideMenuBar: process.platform !== "darwin",
    webPreferences: {
      preload: path.join(__dirname, "preload.cjs"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });
  mainWindow.once("ready-to-show", () => {
    if (!process.argv.includes("--hidden")) mainWindow.show();
  });
  mainWindow.on("close", (event) => {
    if (quitting) return;
    event.preventDefault();
    mainWindow.hide();
  });
  mainWindow.on("closed", () => {
    mainWindow = undefined;
  });
  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith("http://") || url.startsWith("https://")) void shell.openExternal(url);
    return { action: "deny" };
  });
  mainWindow.webContents.on("will-navigate", (event, url) => {
    if (url.startsWith("file:")) return;
    try {
      if (new URL(url).origin === trustedHubOrigin()) return;
    } catch {}
    event.preventDefault();
  });
  mainWindow.webContents.on("did-fail-load", (_event, code, description, validatedURL, isMainFrame) => {
    if (!isMainFrame || code === -3) return;
    appendDesktopLog(`page load failed code=${code} url=${validatedURL} description=${description}`);
  });
}

async function loadOnboarding(errorMessage = "", preferredHub = "") {
  if (!mainWindow) return;
  await mainWindow.loadFile(path.join(__dirname, "onboarding", "index.html"), {
    query: settings ? { configured: "1", hub: preferredHub || settings.hubUrl, error: errorMessage } : ((errorMessage || preferredHub) ? { error: errorMessage, hub: preferredHub } : undefined),
  });
  showWindow();
}

async function showHubUnavailable(message) {
  if (showingHubUnavailable || !mainWindow || mainWindow.isDestroyed()) return;
  showingHubUnavailable = true;
  try {
    await loadOnboarding(message);
  } finally {
    showingHubUnavailable = false;
  }
}

async function loadHub(inviteCode = pendingInviteCode) {
  if (!settings || !mainWindow) return loadOnboarding();
  const info = await resolveHub([settings.hubUrl, ...(settings.hubUrls || [])], true, settings.hubId || "");
  if (!info) {
    appendDesktopLog(`Hub unavailable candidates=${JSON.stringify(settings.hubUrls || [settings.hubUrl])}`);
    return showHubUnavailable("无法连接协作中心。请确认 Server 正在运行，或检查团队地址与当前网络。");
  }
  await adoptHub(info);
  try {
    await configureHubNetworkSession([settings.hubUrl, ...(settings.hubUrls || [])]);
    const target = new URL(`${settings.hubUrl}/`);
    if (inviteCode) target.searchParams.set("join", inviteCode);
    await loadURLWithTimeout(target.toString());
    pendingInviteCode = "";
    updateTrayMenu();
  } catch (error) {
    appendDesktopLog(`Hub page load failed: ${error.stack || error.message}`);
    await showHubUnavailable("协作中心页面未能加载。请确认 Server 正在运行后重试。");
  }
}

async function openTeamLink(rawLink) {
  let intent;
  try { intent = parseTeamInput(rawLink); }
  catch (error) {
    await loadOnboarding("邀请链接无效，请让管理员重新发送。", "");
    return;
  }
  const info = await resolveHub([intent.hubUrl], true, "");
  if (!info) {
    pendingInviteCode = intent.inviteCode;
    await loadOnboarding("暂时找不到邀请中的团队。Client 会继续自动寻找，也可以稍后重试。", intent.hubUrl);
    return;
  }
  pendingInviteCode = intent.inviteCode;
  await adoptHub(info);
  startRunner();
  await loadHub(intent.inviteCode);
}

function startConnectionMonitor() {
  clearInterval(connectionTimer);
  connectionTimer = setInterval(() => {
    if (!settings || quitting || connectionCheckRunning) return;
    connectionCheckRunning = true;
    void (async () => {
      const current = await inspectHub(settings.hubUrl, 1800, settings.hubId || "");
      if (current) {
        consecutiveHubFailures = 0;
        await adoptHub(current, false);
        return;
      }
      consecutiveHubFailures += 1;
      if (consecutiveHubFailures < 2) return;
      const recovered = await resolveHub([settings.hubUrl, ...(settings.hubUrls || [])], true, settings.hubId || "");
      if (!recovered) {
        if (consecutiveHubFailures >= 3 && mainWindow && !mainWindow.isDestroyed()) {
          await showHubUnavailable("团队暂时离线，Client 会继续自动寻找 Server；也可以展开手动连接检查地址。");
        }
        return;
      }
      consecutiveHubFailures = 0;
      const changed = await adoptHub(recovered);
      const currentPage = mainWindow?.webContents.getURL() || "";
      if (changed || currentPage.startsWith("file:")) await loadHub();
    })().catch((error) => appendDesktopLog(`connection monitor failed: ${error.stack || error.message}`)).finally(() => {
      connectionCheckRunning = false;
    });
  }, 7000);
}

async function startApplication() {
  settings = loadSettings();
  migrateLegacyServices();
  if (!isolatedData && app.isPackaged) app.setAsDefaultProtocolClient("agentcollab");
  configureIPC();
  createWindow();
  createTray();
  startConnectionMonitor();
  if (pendingTeamLink) {
    const link = pendingTeamLink;
    pendingTeamLink = "";
    await openTeamLink(link);
    return;
  }
  if (settings) {
    if (!isolatedData) app.setLoginItemSettings({ openAtLogin: false });
    try {
      startRunner();
      await loadHub();
    } catch (error) {
      appendDesktopLog(`startup failed: ${error.stack || error.message}`);
      const currentPage = mainWindow?.webContents.getURL() || "";
      if (!currentPage.startsWith("file:")) await loadOnboarding("Client 暂时无法打开团队页面，请稍后重试。", settings?.hubUrl || "");
    }
  } else {
    const localHub = await inspectCandidateURLs(["http://127.0.0.1:8080"]);
    if (localHub) {
      await adoptHub(localHub, false);
      startRunner();
      await loadHub();
    } else {
      await loadOnboarding();
    }
  }
}

const acquiredLock = app.requestSingleInstanceLock();
if (!acquiredLock) {
  app.quit();
} else {
  app.on("second-instance", (_event, argv) => {
    const link = argv.find((value) => String(value).startsWith("agentcollab://"));
    if (link) void openTeamLink(link);
    else showWindow();
  });
  app.whenReady().then(startApplication).catch((error) => {
    appendDesktopLog(`fatal startup error: ${error.stack || error.message}`);
    app.quit();
  });
}

app.on("open-url", (event, url) => {
  event.preventDefault();
  if (!app.isReady() || !mainWindow) pendingTeamLink = url;
  else void openTeamLink(url);
});

app.on("activate", () => showWindow());
app.on("window-all-closed", () => {
  if (process.platform !== "darwin" && quitting) app.quit();
});
app.on("before-quit", async (event) => {
  if (quitting === "stopped") return;
  if (quitting !== true) quitting = true;
  event.preventDefault();
  clearTimeout(runnerRestartTimer);
  clearInterval(connectionTimer);
  await stopChild(runnerProcess);
  quitting = "stopped";
  app.quit();
});

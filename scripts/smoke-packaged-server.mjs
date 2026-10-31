// Verify an actual packaged Server executable using its real renderer and IPC.
// Usage: node scripts/smoke-packaged-server.mjs /path/to/packaged/executable
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import assert from 'node:assert/strict';

const executable = process.argv[2];
if (!executable || !fs.existsSync(executable)) throw new Error('Provide an existing packaged Server executable');
const expectedVersion = JSON.parse(fs.readFileSync(new URL('../server-desktop/package.json', import.meta.url))).version;
const root = fs.mkdtempSync(path.join(os.tmpdir(), 'agentcollab-packaged-'));
const data = path.join(root, 'data');
async function freePort() {
  const server = net.createServer();
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}
const port = await freePort();
let debugPort = await freePort();
while (debugPort === port) debugPort = await freePort();
const child = spawn(executable, [`--remote-debugging-port=${debugPort}`, '--remote-debugging-address=127.0.0.1'], {
  env: { ...process.env, AGENTCOLLAB_SERVER_DATA_DIR: data, AGENTCOLLAB_SERVER_PORT: String(port) },
  stdio: ['ignore', 'ignore', 'pipe'],
});
child.stderr.resume();
let socket;
const pending = new Map();
let sequence = 0;
const exited = new Promise((resolve, reject) => { child.once('exit', resolve); child.once('error', reject); });
const deadline = Date.now() + 60000;
async function until(check) {
  while (Date.now() < deadline) {
    if (child.exitCode !== null) throw new Error('Packaged Server exited before acceptance');
    const result = await check();
    if (result) return result;
    await new Promise(resolve => setTimeout(resolve, 200));
  }
  throw new Error('Packaged Server acceptance timed out');
}
function command(method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = ++sequence;
    const timeout = setTimeout(() => { pending.delete(id); reject(new Error(`CDP timeout: ${method}`)); }, 10000);
    pending.set(id, { resolve: value => { clearTimeout(timeout); resolve(value); }, reject: error => { clearTimeout(timeout); reject(error); } });
    socket.send(JSON.stringify({ id, method, params }));
  });
}
async function evaluate(expression) {
  const response = await command('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
  if (response.exceptionDetails) throw new Error('Packaged renderer evaluation failed');
  return response.result.value;
}
try {
  const target = await until(async () => {
    try {
      const response = await fetch(`http://127.0.0.1:${debugPort}/json/list`, { signal: AbortSignal.timeout(1000) });
      return (await response.json()).find(item => item.type === 'page' && item.url.endsWith('/onboarding/index.html'));
    } catch { return false; }
  });
  socket = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject; });
  socket.onmessage = event => {
    const message = JSON.parse(event.data);
    const request = pending.get(message.id);
    if (!request) return;
    pending.delete(message.id);
    if (message.error) request.reject(new Error(message.error.message)); else request.resolve(message.result);
  };
  const initial = await evaluate('window.agentCollabServer.getState()');
  assert.equal(initial.configured, false);
  assert.equal(initial.version, expectedVersion, 'must test application version, not Electron harness version');
  await evaluate(`(() => {
    const set = (id, value) => { document.getElementById(id).value = value; };
    set('language', 'en'); document.getElementById('language').dispatchEvent(new Event('change'));
    set('name', 'Community Demo'); set('addressMode', 'manual'); document.getElementById('addressMode').dispatchEvent(new Event('change'));
    set('url', 'http://127.0.0.1:${port}'); set('display', 'Demo Owner'); set('password', ${JSON.stringify(randomUUID())});
    document.getElementById('create').click();
    return true;
  })()`);
  await until(async () => {
    try { return await evaluate('location.pathname.endsWith("/dashboard/index.html") && document.body.innerText.includes("Server is running")'); }
    catch { return false; }
  });
  const state = await evaluate('window.agentCollabServer.getState()');
  assert.match(await evaluate('document.querySelector(".help").textContent'), /Members can use a browser; install the Client only to connect local agents/);
  assert.equal(state.version, expectedVersion);
  assert.equal(state.port, port);
  assert.ok(state.storageRoot.startsWith(data + path.sep));
  const health = await fetch(`http://127.0.0.1:${port}/healthz`).then(response => response.json());
  assert.equal(health.status, 'ok');
  await evaluate('window.agentCollabServer.restart()');
  await until(async () => {
    try { return (await fetch(`http://127.0.0.1:${port}/healthz`, { signal: AbortSignal.timeout(1000) }).then(response => response.json())).status === 'ok'; }
    catch { return false; }
  });
  const restarted = await evaluate('window.agentCollabServer.getState()');
  assert.equal(restarted.hubId, state.hubId, 'restart must preserve team identity');
  assert.equal(restarted.storageRoot, state.storageRoot, 'restart must preserve team storage');
  assert.equal(restarted.port, port);
  const screenshot = path.join(root, 'server-en.png');
  const capture = await command('Page.captureScreenshot', { format: 'png' });
  fs.writeFileSync(screenshot, Buffer.from(capture.data, 'base64'));
  await evaluate(`(() => { const language = document.getElementById('language'); language.value = 'zh-CN'; language.dispatchEvent(new Event('change')); return true; })()`);
  await until(() => evaluate('document.getElementById("statusText").textContent === "Server 正常运行"'));
  const translatedState = await evaluate('window.agentCollabServer.getState()');
  assert.match(await evaluate('document.querySelector(".help").textContent'), /成员可以直接用浏览器参与讨论；仅在接入本机 Agent 时需要安装 Client/);
  assert.equal(translatedState.hubId, state.hubId, 'language switch must preserve team identity');
  const chineseScreenshot = path.join(root, 'server-zh-CN.png');
  fs.writeFileSync(chineseScreenshot, Buffer.from((await command('Page.captureScreenshot', { format: 'png' })).data, 'base64'));
  // Do not wait for an IPC reply after the application intentionally exits.
  await evaluate('setTimeout(() => window.agentCollabServer.quit(), 50); true');
  const exitCode = await Promise.race([exited, new Promise((_, reject) => { const timer = setTimeout(() => reject(new Error('Packaged Server did not exit')), 10000); timer.unref(); })]);
  assert.equal(exitCode, 0);
  await assert.rejects(fetch(`http://127.0.0.1:${port}/healthz`, { signal: AbortSignal.timeout(2000) }));
  fs.rmSync(data, { recursive: true, force: true });
  console.log(JSON.stringify({ result: 'PASS', version: expectedVersion, flow: 'packaged application -> first-run form -> healthy Hub -> restart preserves team -> language switch -> clean exit', screenshot, chineseScreenshot }));
} finally {
  socket?.close();
  if (child.exitCode === null) child.kill('SIGTERM');
}

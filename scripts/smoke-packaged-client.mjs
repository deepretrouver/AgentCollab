// Real packaged Client acceptance. All accounts and application state are temporary.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { spawn, spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import assert from 'node:assert/strict';

const [executable, hub] = process.argv.slice(2);
assert.ok(executable && hub && fs.existsSync(executable) && fs.existsSync(hub), 'Usage: node scripts/smoke-packaged-client.mjs CLIENT_EXECUTABLE HUB_BINARY');
const root = fs.mkdtempSync(path.join(os.tmpdir(), 'agentcollab-packaged-client-'));
const profile = path.join(root, 'client');
const config = path.join(root, 'hub.yaml');
const password = randomUUID();
fs.mkdirSync(path.join(root, 'data'));
fs.writeFileSync(config, `storage_root: ${JSON.stringify(path.join(root, 'data'))}\nlisten_address: 127.0.0.1:0\nrequire_smb: false\n`);
let server, client, socket;
let sequence = 0;
const pending = new Map();
async function waitFor(check, label, timeout = 30000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    if (await check()) return;
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error(`Timed out: ${label}`);
}
async function freePort() {
  const listener = net.createServer();
  await new Promise((resolve, reject) => { listener.once('error', reject); listener.listen(0, '127.0.0.1', resolve); });
  const port = listener.address().port;
  await new Promise(resolve => listener.close(resolve));
  return port;
}
function command(method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = ++sequence;
    const timer = setTimeout(() => { pending.delete(id); reject(new Error(`CDP timed out: ${method}`)); }, 15000);
    pending.set(id, { resolve: value => { clearTimeout(timer); resolve(value); }, reject: error => { clearTimeout(timer); reject(error); } });
    socket.send(JSON.stringify({ id, method, params }));
  });
}
async function evaluate(expression) {
  const result = await command('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
  if (result.exceptionDetails) throw new Error('Packaged Client renderer evaluation failed');
  return result.result.value;
}
async function launch(link) {
  const port = await freePort();
  client = spawn(executable, [`--remote-debugging-port=${port}`, '--remote-debugging-address=127.0.0.1', ...(link ? [link] : [])], {
    env: { ...process.env, AGENTCOLLAB_CLIENT_DATA_DIR: profile }, stdio: ['ignore', 'ignore', 'pipe'],
  });
  client.stderr.resume();
  let target;
  await waitFor(async () => {
    if (client.exitCode !== null) throw new Error('Packaged Client exited during startup');
    try { target = (await (await fetch(`http://127.0.0.1:${port}/json/list`, { signal: AbortSignal.timeout(1000) })).json()).find(item => item.type === 'page' && item.url.startsWith('http://127.0.0.1:')); return Boolean(target); }
    catch { return false; }
  }, 'team window');
  socket = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject; });
  socket.onmessage = event => {
    const message = JSON.parse(event.data);
    const request = pending.get(message.id);
    if (!request) return;
    pending.delete(message.id);
    if (message.error) request.reject(new Error(message.error.message)); else request.resolve(message.result);
  };
}
async function closeClient() {
  // Chromium's browser close requests application shutdown, rather than just hiding the window.
  await command('Browser.close').catch(() => {});
  await waitFor(() => client.exitCode !== null, 'Client exit', 15000);
  assert.equal(client.exitCode, 0);
  socket?.close();
}
try {
  assert.equal(spawnSync(hub, ['admin', 'create', '--config', config, '--username', 'demo-owner', '--display-name', 'Demo Owner'], { env: { ...process.env, AGENTCOLLAB_ADMIN_PASSWORD: password }, stdio: 'ignore' }).status, 0);
  let output = '';
  server = spawn(hub, ['serve', '--config', config], { stdio: ['ignore', 'pipe', 'pipe'] });
  server.stdout.on('data', chunk => { output += chunk; });
  server.stderr.resume();
  await waitFor(() => /hub_listening=(\S+)/.test(output), 'temporary Hub');
  const base = 'http://' + output.match(/hub_listening=(\S+)/)[1];
  let cookie = '';
  async function api(route, body) {
    const response = await fetch(base + route, { method: 'POST', headers: { 'Content-Type': 'application/json', Cookie: cookie }, body: JSON.stringify(body), signal: AbortSignal.timeout(10000) });
    assert.ok(response.ok, `fixture API status ${response.status}`);
    if (response.headers.get('set-cookie')) cookie = response.headers.get('set-cookie').split(';')[0];
    return response.json();
  }
  await api('/api/v1/auth/login', { username: 'demo-owner', password });
  const workspace = await api('/api/v1/workspaces', { name: 'Packaged Client demo' });
  const room = await api(`/api/v1/workspaces/${workspace.id}/rooms`, { name: 'Demo room' });
  const invite = await api(`/api/v1/workspaces/${workspace.id}/invites`, { role: 'member', room_id: room.id, expires_in_seconds: 300 });
  await api('/api/v1/auth/register', { username: 'demo-member', display_name: 'Demo Member', password });
  await launch('agentcollab://join?' + new URLSearchParams({ hub: base, invite: invite.code }));
  await waitFor(() => evaluate('Boolean(document.querySelector("input[name=password]"))'), 'login form');
  await evaluate(`(() => {
    const language = document.querySelector('select[aria-label="Language / 语言"]'); language.value = 'en'; language.dispatchEvent(new Event('change', {bubbles:true}));
    return true;
  })()`);
  await waitFor(() => evaluate('document.documentElement.lang === "en"'), 'English');
  await evaluate(`(() => { [...document.querySelectorAll('button[role=tab]')].find(button => button.textContent.trim() === 'Sign in').click(); return true; })()`);
  await evaluate(`(() => {
    for (const [name, value] of Object.entries({username:'demo-member', password:${JSON.stringify(password)}})) {
      const input = document.querySelector('input[name='+name+']'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(input,value); input.dispatchEvent(new Event('input',{bubbles:true}));
    }
    return true;
  })()`);
  await evaluate('document.querySelector("button.login-submit").click(); true');
  await waitFor(() => evaluate('Boolean(document.querySelector(".workspace-sidebar"))'), 'authenticated team');
  await waitFor(() => evaluate('Boolean(document.querySelector(".desktop-account-warning button"))'), 'device connection guidance');
  await evaluate('document.querySelector(".desktop-account-warning button").click(); true');
  await waitFor(() => evaluate('Boolean(document.querySelector(".agent-connect-action:not(:disabled)"))'), 'connect button');
  await evaluate('document.querySelector(".agent-connect-action").click(); true');
  await waitFor(() => evaluate('window.agentCollabDesktop.getState().then(s => s.runnerRegistered && s.runnerRunning)'), 'packaged Runner online', 45000);
  const state = await evaluate('window.agentCollabDesktop.getState()');
  assert.equal(state.version, JSON.parse(fs.readFileSync(new URL('../desktop/package.json', import.meta.url))).version);
  assert.ok(state.deviceId && state.runnerId && state.hubId, 'paired identities must be present');
  await waitFor(() => evaluate('Boolean(document.querySelector(".agent-add-submit")) && !document.querySelector(".needs-connection")'), 'discovery UI after connection', 45000);
  assert.equal(await evaluate('Boolean(document.querySelector(".agent-dialog [role=alert]"))'), false);
  await evaluate('new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))');
  fs.writeFileSync(path.join(root, 'connected.png'), Buffer.from((await command('Page.captureScreenshot', { format: 'png' })).data, 'base64'));
  await closeClient();
  await launch();
  await waitFor(() => evaluate('Boolean(document.querySelector(".workspace-sidebar"))'), 'persisted account');
  await waitFor(() => evaluate('window.agentCollabDesktop.getState().then(s => s.runnerRegistered && s.runnerRunning)'), 'automatic reconnect', 45000);
  const restored = await evaluate('window.agentCollabDesktop.getState()');
  assert.equal(restored.hubUrl, state.hubUrl);
  assert.equal(restored.deviceId, state.deviceId);
  assert.equal(restored.runnerId, state.runnerId);
  assert.equal(restored.hubId, state.hubId);
  assert.equal(await evaluate('document.documentElement.lang'), 'en');
  await closeClient();
  const stopped = new Promise(resolve => server.once('exit', resolve));
  server.kill('SIGTERM');
  await stopped;
  // Keep only screenshots, never temporary account/device credentials.
  fs.rmSync(profile, { recursive: true, force: true });
  fs.rmSync(path.join(root, 'data'), { recursive: true, force: true });
  fs.rmSync(config);
  console.log(JSON.stringify({ result: 'PASS', flow: 'packaged Client -> invitation -> login -> connect -> clean exit -> reopen without invitation or pairing -> account and connection restored', evidence: root }));
} finally {
  socket?.close();
  if (client && client.exitCode === null) client.kill('SIGTERM');
  if (server && server.exitCode === null) server.kill('SIGTERM');
}

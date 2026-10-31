// Fresh-process HTTP smoke test. Never uses an existing team database.
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import assert from 'node:assert/strict';

const binary = resolve(process.argv[2] || 'bin/agentcollab-hub');
const peerIndex = process.argv.indexOf('--pair-hermes');
const hermesPeer = peerIndex >= 0 ? process.argv[peerIndex + 1] : undefined;
if (peerIndex >= 0) assert.ok(hermesPeer && !hermesPeer.startsWith('--') && process.argv[5] === 'opencode', '--pair-hermes requires primary OpenCode and a Hermes executable path');
if (process.argv.includes('--project')) {
  assert.ok(process.argv[3] && process.argv[4] && ['codex', 'opencode'].includes(process.argv[5]), '--project requires Hub, Runner, native CLI path and adapter type');
}
const root = await mkdtemp(join(tmpdir(), 'agentcollab-smoke-'));
const config = join(root, 'hub.yaml');
const password = randomUUID();
let child;
async function runProcess(command, args, options = {}) {
  return new Promise((resolveProcess, reject) => {
    const proc = spawn(command, args, { env: options.env, stdio: ['pipe', 'pipe', 'pipe'] });
    let stderr = '';
    proc.stdout.resume();
    proc.stderr.on('data', chunk => { stderr += chunk; });
    const timer = setTimeout(() => proc.kill('SIGKILL'), options.timeout || 180000);
    proc.once('error', error => { clearTimeout(timer); reject(error); });
    proc.once('close', status => { clearTimeout(timer); resolveProcess({ status, stderr }); });
    proc.stdin.end(options.input || '');
  });
}
try {
  await mkdir(join(root, 'data'));
  await writeFile(config, 'storage_root: ./data\nlisten_address: 127.0.0.1:0\nrequire_smb: false\n');
  const admin = spawnSync(binary, ['admin', 'create', '--config', config, '--username', 'demo-owner', '--display-name', 'Demo Owner'], { env: { ...process.env, AGENTCOLLAB_ADMIN_PASSWORD: password }, encoding: 'utf8' });
  assert.equal(admin.status, 0, admin.stderr);
  async function startServer() {
    child = spawn(binary, ['serve', '--config', config], { stdio: ['ignore', 'pipe', 'pipe'] });
    child.stderr.resume();
    return await new Promise((resolveAddress, reject) => {
    const timer = setTimeout(() => reject(new Error('Server startup timed out')), 15000);
    let output = '';
    child.stdout.on('data', chunk => {
      output += chunk;
      const match = output.match(/hub_listening=(\S+)/);
      if (match) { clearTimeout(timer); resolveAddress(match[1]); }
    });
    child.once('exit', code => { clearTimeout(timer); reject(new Error(`Server exited: ${code}`)); });
    });
  }
  let base = `http://${await startServer()}`;
  const cookies = new Map();
  async function api(path, body, identity = 'owner', expected = 200) {
    const response = await fetch(base + path, { method: body ? 'POST' : 'GET', headers: { 'Content-Type': 'application/json', Cookie: cookies.get(identity) || '' }, body: body ? JSON.stringify(body) : undefined, signal: AbortSignal.timeout(10000) });
    const text = await response.text();
    assert.equal(response.status, expected, `${path}: ${text}`);
    const cookie = response.headers.get('set-cookie');
    if (cookie) cookies.set(identity, cookie.split(';')[0]);
    return text ? JSON.parse(text) : null;
  }
  assert.equal((await api('/healthz')).status, 'ok');
  const originalIdentity = (await api('/api/v1/hub/info')).hub_id;
  assert.ok(originalIdentity, 'source Server must advertise a Client-compatible identity');
  await api('/api/v1/auth/login', { username: 'demo-owner', password });
  const workspace = await api('/api/v1/workspaces', { name: 'Community demo' }, 'owner', 201);
  const room = await api(`/api/v1/workspaces/${workspace.id}/rooms`, { name: 'Demo discussion' }, 'owner', 201);
  await api('/api/v1/auth/register', { username: 'demo-member', display_name: 'Demo Member', password }, 'member', 201);
  await api('/api/v1/admin/users', null, 'member', 403);
  const invite = await api(`/api/v1/workspaces/${workspace.id}/invites`, { role: 'member', room_id: room.id, expires_in_seconds: 3600 }, 'owner', 201);
  await api(`/api/v1/invites/${invite.code}/accept`, {}, 'member');
  await api('/api/v1/auth/login', { username: 'demo-member', password }, 'member');
  const rooms = await api(`/api/v1/workspaces/${workspace.id}/rooms`, null, 'member');
  assert.ok(rooms.rooms.some(item => item.id === room.id));
  const fileContent = 'Public demo reference: review before delivery.\n';
  const form = new FormData();
  form.set('file', new Blob([fileContent], { type: 'text/plain' }), 'reference.txt');
  const uploaded = await fetch(`${base}/api/v1/rooms/${room.id}/knowledge`, { method: 'POST', headers: { Cookie: cookies.get('owner') }, body: form });
  assert.equal(uploaded.status, 201, await uploaded.clone().text());
  const asset = await uploaded.json();
  const filePath = `/api/v1/rooms/${room.id}/knowledge/${asset.id}`;
  const download = await fetch(base + filePath, { headers: { Cookie: cookies.get('member') } });
  assert.equal(download.status, 200);
  assert.equal(await download.text(), fileContent);
  await api('/api/v1/auth/register', { username: 'demo-outsider', display_name: 'Outsider', password }, 'outsider', 201);
  const denied = await fetch(base + filePath, { headers: { Cookie: cookies.get('outsider') } });
  assert.ok([403, 404].includes(denied.status), 'outsider must not read room files');
  const stopped = new Promise(resolveExit => child.once('exit', resolveExit));
  child.kill('SIGTERM');
  await stopped;
  base = `http://${await startServer()}`;
  assert.equal((await api('/api/v1/hub/info')).hub_id, originalIdentity, 'Server identity must survive restart');
  await api('/api/v1/auth/login', { username: 'demo-member', password }, 'member');
  const restored = await api(`/api/v1/workspaces/${workspace.id}/rooms`, null, 'member');
  assert.ok(restored.rooms.some(item => item.id === room.id));
  const persisted = await fetch(base + filePath, { headers: { Cookie: cookies.get('member') } });
  assert.equal(persisted.status, 200);
  assert.equal(await persisted.text(), fileContent);
  console.log('PASS: reference upload, member download, outsider denial, Server restart and persisted room/file');
  // Optional real CLI acceptance: node script hub runner cli-path [codex|opencode|hermes]
  if (process.argv[3] && process.argv[4]) {
    const runner = resolve(process.argv[3]);
    const agentType = process.argv[5] || 'codex';
    assert.ok(['codex', 'opencode', 'hermes'].includes(agentType), 'unsupported smoke adapter');
    const catalog = JSON.parse(await readFile(new URL('../internal/agentcatalog/default.json', import.meta.url), 'utf8'));
    const adapter = catalog.adapters.find(item => item.type === agentType);
    assert.ok(adapter, 'native adapter must exist');
    const runnerConfig = join(root, 'runner.json');
    const credentialsFile = join(root, 'device.json');
    const runnerEnv = { ...process.env, AGENTCOLLAB_INSTALLATION_ID_FILE: join(root, 'installation-id') };
    await writeFile(runnerConfig, JSON.stringify({
      hub_url: base, local_dir: join(root, 'runner'), credentials_file: credentialsFile,
      max_concurrent_tasks: 1, resources: [], agents: [{
        id: 'smoke-native', display_name: `Demo ${agentType}`, type: agentType, command: resolve(process.argv[4]),
        // Exercise the shipped adapter's setup, but never its unrestricted permissions.
        args: agentType === 'codex' ? adapter.args.flatMap(arg => arg === '--dangerously-bypass-approvals-and-sandbox' ? ['--sandbox', 'read-only'] : [arg])
          // Hermes rejects an empty toolset. Clarify cannot read/write project files;
          // the confirmation prompt still explicitly requests no tool calls.
          : agentType === 'hermes' ? ['--ignore-rules', '--toolsets', 'clarify', ...adapter.args.filter(arg => arg !== '--yolo')] : adapter.args,
        prompt_stdin: agentType === 'codex', output_mode: adapter.output_mode, timeout_seconds: 120,
      }, ...(hermesPeer ? [{
        id: 'smoke-peer', display_name: 'Demo Hermes', type: 'hermes', command: resolve(hermesPeer),
        args: ['--ignore-rules', '--toolsets', 'clarify', ...catalog.adapters.find(item => item.type === 'hermes').args.filter(arg => arg !== '--yolo')],
        prompt_stdin: false, output_mode: 'text', timeout_seconds: 120,
      }] : [])],
    }));
    const enrolled = await runProcess(runner, ['enroll', '--config', runnerConfig, '--workspace', workspace.id, '--username', 'demo-owner', '--password-stdin', '--device-name', 'Demo Device'], { input: password, env: runnerEnv, timeout: 60000 });
    assert.equal(enrolled.status, 0, enrolled.stderr);
    const credentials = JSON.parse(await readFile(credentialsFile, 'utf8'));
    const agentID = credentials.agents['smoke-native'];
    assert.ok(agentID, 'configured native agent must be discovered');
    await api(`/api/v1/workspaces/${workspace.id}/agents/import`, { agent_id: agentID }, 'owner', 204);
    await api(`/api/v1/rooms/${room.id}/agents`, { agent_id: agentID }, 'owner', 204);
    const marker = `COMMUNITY_OK_${randomUUID().replaceAll('-', '')}`;
    const { task } = await api(`/api/v1/rooms/${room.id}/tasks`, { target_agent_id: agentID, title: 'Native CLI acceptance', prompt: `Do not call tools or read files. Reply only with: ${marker}` }, 'owner', 201);
    const executed = await runProcess(runner, ['serve', '--config', runnerConfig, '--once'], { env: runnerEnv, timeout: 180000 });
    assert.equal(executed.status, 0, executed.stderr);
    const messages = await api(`/api/v1/rooms/${room.id}/messages`);
    const tasks = await api(`/api/v1/rooms/${room.id}/tasks`);
    const completed = tasks.tasks.find(item => item.id === task.id);
    assert.ok(completed, 'submitted task must remain queryable');
    if (completed.state !== 'delivered') {
      const related = messages.messages.filter(message => message.metadata?.task_id === task.id);
      // Report categories only: native error bodies can contain local paths or provider data.
      const body = related.map(message => message.body || '').join('\n');
      const category = /timeout|timed out|deadline|超时/i.test(body) ? 'timeout'
        : /unknown.*toolsets|valid toolsets|unrecognized arguments|invalid choice/i.test(body) ? 'arguments'
        : /authentication|unauthorized|login|登录|认证/i.test(body) ? 'authentication'
        : /configuration|not found|配置|找不到/i.test(body) ? 'configuration'
        : 'unclassified';
      console.error(`Native acceptance failed: adapter=${agentType}, state=${completed.state}, category=${category}, related_messages=${related.length}`);
    }
    assert.equal(completed.state, 'delivered', 'native task must be delivered, not failed or merely dispatched');
    assert.equal(completed.target_agent_id, agentID, 'delivery must belong to the requested Agent');
    // The stored body also contains the Server's delivery title and execution receipt.
    assert.ok(messages.messages.some(message => message.sender_kind === 'agent' && message.sender_id === agentID && message.kind === 'delivery' && message.metadata?.task_id === task.id && message.metadata?.task_state === 'delivered' && message.body.split('\n').some(line => line.trim() === marker)), 'expected native confirmation linked to the requested Agent and task');
    console.log(`PASS: real ${agentType} discovery, registration, room membership, task dispatch and Agent message delivery`);
    if (hermesPeer) {
      const peerID = credentials.agents['smoke-peer'];
      assert.ok(peerID && peerID !== agentID, 'peer must have a distinct registered identity');
      await api(`/api/v1/workspaces/${workspace.id}/agents/import`, { agent_id: peerID }, 'owner', 204);
      await api(`/api/v1/rooms/${room.id}/agents`, { agent_id: peerID }, 'owner', 204);
      const teamMarker = `TEAM_OK_${randomUUID().replaceAll('-', '')}`;
      const rootMessage = await api(`/api/v1/rooms/${room.id}/messages`, {
        body: `Do not call tools or read files. Both agents: reply only ${teamMarker}`,
        mention_agent_ids: [agentID, peerID],
      }, 'owner', 201);
      const rootTasks = (await api(`/api/v1/rooms/${room.id}/tasks`)).tasks.filter(item => item.source_message_id === rootMessage.id);
      assert.equal(rootTasks.length, 2, 'one group mention must create exactly two tasks');
      assert.deepEqual(new Set(rootTasks.map(item => item.target_agent_id)), new Set([agentID, peerID]));
      for (let index = 0; index < rootTasks.length; index++) {
        const result = await runProcess(runner, ['serve', '--config', runnerConfig, '--once'], { env: runnerEnv, timeout: 180000 });
        assert.equal(result.status, 0, 'paired Runner execution failed');
      }
      const pairedTasks = (await api(`/api/v1/rooms/${room.id}/tasks`)).tasks;
      const pairedMessages = (await api(`/api/v1/rooms/${room.id}/messages`)).messages;
      for (const submitted of rootTasks) {
        assert.equal(pairedTasks.find(item => item.id === submitted.id)?.state, 'delivered', `paired task failed for ${submitted.target_agent_id}`);
        assert.ok(pairedMessages.some(item => item.sender_id === submitted.target_agent_id && item.metadata?.task_id === submitted.id && item.root_message_id === rootMessage.id && item.body.split('\n').some(line => line.trim() === teamMarker)), 'each native reply must link to its own task and the original topic');
      }
      const followup = await api(`/api/v1/rooms/${room.id}/messages`, {
        body: 'Do not call tools or read files. From the earlier replies in this topic, repeat only the exact token beginning TEAM_OK_. Do not invent a token.',
        mention_agent_ids: [peerID], reply_to_message_id: rootMessage.id, root_message_id: rootMessage.id,
      }, 'owner', 201);
      const followupTasks = (await api(`/api/v1/rooms/${room.id}/tasks`)).tasks.filter(item => item.source_message_id === followup.id);
      assert.equal(followupTasks.length, 1, 'explicit topic mention must invoke only the selected peer');
      assert.equal(followupTasks[0].target_agent_id, peerID);
      assert.ok(followupTasks[0].prompt.includes(teamMarker), 'Server must include prior topic content in the followup prompt');
      const result = await runProcess(runner, ['serve', '--config', runnerConfig, '--once'], { env: runnerEnv, timeout: 180000 });
      assert.equal(result.status, 0, 'followup Runner execution failed');
      const finalTasks = (await api(`/api/v1/rooms/${room.id}/tasks`)).tasks;
      assert.equal(finalTasks.find(item => item.id === followupTasks[0].id)?.state, 'delivered');
      const followupReplies = (await api(`/api/v1/rooms/${room.id}/messages`)).messages.filter(item => item.sender_id === peerID && item.metadata?.task_id === followupTasks[0].id);
      const contextPassed = followupReplies.some(item => item.root_message_id === rootMessage.id && item.body.split('\n').some(line => line.trim() === teamMarker));
      if (!contextPassed) console.error(JSON.stringify({ stage: 'peer-context', priorTokenInPrompt: true, replies: followupReplies.map(item => ({ rootMatches: item.root_message_id === rootMessage.id, containsToken: item.body.includes(teamMarker), kind: item.kind })) }));
      assert.ok(contextPassed, 'peer must retrieve the original token from topic context');
      console.log('PASS: real OpenCode/Hermes group mention, distinct task-linked topic replies and context-aware targeted followup');
    }
    if (process.argv.includes('--project')) {
      async function uploadDirectory(path, content) {
        const form = new FormData();
        form.set('source_name', 'demo-module');
        form.append('files', new Blob([content], { type: 'text/plain' }), 'value.txt');
        form.append('paths', 'src/value.txt');
        const response = await fetch(base + path, { method: 'POST', headers: { Cookie: cookies.get('owner') }, body: form, signal: AbortSignal.timeout(15000) });
        assert.equal(response.status, 200, await response.clone().text());
        return response.json();
      }
      const step = await api(`/api/v1/workspaces/${workspace.id}/project-steps`, { title: 'Demo module', weight: 1 }, 'owner', 201);
      await uploadDirectory(`/api/v1/project-steps/${step.id}/workspace`, 'before\n');
      const message = await api(`/api/v1/rooms/${room.id}/messages`, { body: 'Change src/value.txt from before to after. This is a plain-text fixture; no build or dependencies are required. Inspect only the supplied project files.', plan_step_id: step.id }, 'owner', 201);
      async function itemState() {
        const response = await api(`/api/v1/rooms/${room.id}/work-items`);
        const item = response.work_items.find(item => item.root_message_id === message.id);
        assert.ok(item, 'project conversation must produce a traceable work item');
        return item;
      }
      const item = await itemState();
      await uploadDirectory(`/api/v1/work-items/${item.id}/submit-workspace`, 'after\n');
      const review = await api(`/api/v1/work-items/${item.id}/review`);
      assert.equal(review.mergeable, true);
      assert.equal(review.file_count, 1);
      assert.equal(review.changed_files[0].path, 'src/value.txt');
      // A submission must not change main before review and explicit merge.
      const repository = join(root, 'data', 'workspaces', workspace.id, 'git', 'project.git');
      function mainContent() {
        const git = spawnSync('git', ['--git-dir', repository, 'show', `main:steps/${step.id}/src/value.txt`], { encoding: 'utf8' });
        assert.equal(git.status, 0, git.stderr);
        return git.stdout;
      }
      assert.equal(mainContent(), 'before\n');
      await api(`/api/v1/work-items/${item.id}/ai-review`, { review_agent_id: agentID, verification_agent_id: agentID }, 'owner', 201);
      const reviewed = await runProcess(runner, ['serve', '--config', runnerConfig, '--once'], { env: runnerEnv });
      assert.equal(reviewed.status, 0, reviewed.stderr);
      assert.equal((await itemState()).ai_review_status, 'passed', 'real Agent review must approve the supplied change');
      assert.equal(mainContent(), 'before\n');
      await api(`/api/v1/work-items/${item.id}/merge`, {}, 'member', 403);
      await api(`/api/v1/work-items/${item.id}/merge`, {}, 'owner');
      assert.equal(mainContent(), 'after\n');
      const verified = await runProcess(runner, ['serve', '--config', runnerConfig, '--once'], { env: runnerEnv });
      assert.equal(verified.status, 0, verified.stderr);
      const finished = await itemState();
      assert.equal(finished.verification_status, 'passed', 'real Agent must verify the merged main snapshot');
      assert.equal(finished.status, 'done');
      const projectTasks = (await api(`/api/v1/rooms/${room.id}/tasks`)).tasks.filter(task => task.work_item_id === item.id);
      assert.equal(projectTasks.length, 2, 'review and verification must be distinct real tasks');
      const projectMessages = (await api(`/api/v1/rooms/${room.id}/messages`)).messages;
      for (const projectTask of projectTasks) {
        assert.equal(projectTask.state, 'delivered');
        assert.equal(projectTask.target_agent_id, agentID);
        assert.ok(projectMessages.some(message => message.sender_id === agentID && message.kind === 'delivery' && message.metadata?.task_id === projectTask.id), 'each project task must have its own Agent delivery');
      }
      const exited = new Promise(resolveExit => child.once('exit', resolveExit));
      child.kill('SIGTERM');
      await exited;
      base = `http://${await startServer()}`;
      await api('/api/v1/auth/login', { username: 'demo-owner', password });
      assert.equal((await itemState()).status, 'done');
      assert.equal((await itemState()).verification_status, 'passed');
      assert.equal(mainContent(), 'after\n');
      console.log(`PASS: real ${agentType} project upload, isolated branch, review, authorized merge and post-merge verification`);
      if (process.env.AGENTCOLLAB_DEMO_ELECTRON) {
        const output = await mkdtemp(join(tmpdir(), 'agentcollab-demo-captures-'));
        const captured = await runProcess(process.env.AGENTCOLLAB_DEMO_ELECTRON, [new URL('./capture-community-demo.cjs', import.meta.url).pathname], {
          timeout: 60000,
          env: { ...process.env, AGENTCOLLAB_DEMO_URL: base, AGENTCOLLAB_DEMO_COOKIE: cookies.get('owner'), AGENTCOLLAB_DEMO_OUTPUT: output },
        });
        assert.equal(captured.status, 0, 'real UI demo capture failed; do not publish unreviewed images');
        console.log(`PASS: real UI captures for manual privacy review: ${output}`);
      }
    }
  }
  console.log('PASS: fresh Server, admin, workspace, room, second user, admin denial, invitation, independent re-login');
} finally {
  if (child && child.exitCode === null) {
    const exited = new Promise(resolveExit => child.once('exit', resolveExit));
    child.kill('SIGTERM');
    await exited;
  }
  await rm(root, { recursive: true, force: true });
}

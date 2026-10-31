// Electron acceptance harness: real isolated Hub, Client navigation and pairing.
const { app } = require('electron');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawn, spawnSync } = require('node:child_process');
const { randomUUID } = require('node:crypto');
const root = fs.mkdtempSync(path.join(os.tmpdir(), 'agentcollab-client-smoke-'));
const repo = path.resolve(__dirname, '..');
const hub = path.join(repo, 'server-desktop/runtime/agentcollab-hub');
const config = path.join(root, 'hub.yaml');
const password = randomUUID();
const registerMember = process.env.AGENTCOLLAB_SMOKE_AUTH === 'register';
const checkProject = process.env.AGENTCOLLAB_SMOKE_PROJECT === '1';
// Optional real-host discovery check. Not enabled in CI hosts without native CLIs.
const checkAgentDrawer = process.env.AGENTCOLLAB_SMOKE_AGENT_DRAWER === '1';
let hubProcess;
let passed = false;
let base;
let workspace;
let invite;
let checking = false;
process.env.AGENTCOLLAB_CLIENT_DATA_DIR = path.join(root, 'client');
// Set the isolated profile before asynchronous fixture startup lets Electron
// initialize its network service; production main sets this synchronously.
app.setPath('userData', process.env.AGENTCOLLAB_CLIENT_DATA_DIR);
fs.mkdirSync(path.join(root, 'data'));
fs.writeFileSync(config, `storage_root: ${JSON.stringify(path.join(root, 'data'))}\nlisten_address: 127.0.0.1:0\nrequire_smb: false\n`);
const timer = setTimeout(() => fail(new Error('Client acceptance timed out')), 90000);
function fail(error) { console.error('FAIL:', error.message, 'evidence directory:', root); process.exitCode = 1; app.quit(); }
app.on('will-quit', () => {
  clearTimeout(timer);
  if (hubProcess && hubProcess.exitCode === null) hubProcess.kill('SIGTERM');
});
app.on('quit', () => {
  // Electron's default app.quit() exit code does not honor process.exitCode.
  if (!passed) process.exit(1);
});
app.on('browser-window-created', (_event, window) => {
  window.webContents.on('did-finish-load', async () => {
    console.log('Client page:', new URL(window.webContents.getURL()).pathname);
    if (!base || !window.webContents.getURL().startsWith(base) || checking) return;
    checking = true;
    try {
      const target = new URL(window.webContents.getURL());
      if (target.searchParams.get('join') !== invite.code) throw new Error('invitation lost during Client navigation');
      await window.webContents.executeJavaScript(`(async () => {
        const waitFor = async (predicate, label) => {
          const deadline = Date.now() + 15000;
          while (Date.now() < deadline) {
            if (predicate()) return;
            await new Promise(resolve => setTimeout(resolve, 50));
          }
          throw new Error('UI timed out: ' + label);
        };
        await waitFor(() => document.querySelector('select[aria-label="Language / 语言"]'), 'login page');
        const language = document.querySelector('select[aria-label="Language / 语言"]');
        language.value = 'en';
        language.dispatchEvent(new Event('change', {bubbles:true}));
        await waitFor(() => document.documentElement.lang === 'en', 'English locale');
        const registerMember = ${JSON.stringify(registerMember)};
        if (!registerMember) {
          const loginTab = [...document.querySelectorAll('button[role="tab"]')].find(button => button.textContent.trim() === 'Sign in');
          if (!loginTab) throw new Error('Sign-in tab missing');
          loginTab.click();
        }
        await waitFor(() => document.querySelector('input[name="password"]')?.autocomplete === (registerMember ? 'new-password' : 'current-password'), 'authentication form mode');
        const setValue = (selector, value) => {
          const input = document.querySelector(selector);
          if (!input) throw new Error('Missing form field: ' + selector);
          Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, value);
          input.dispatchEvent(new Event('input', {bubbles:true}));
        };
        if (registerMember) setValue('input[autocomplete="name"]', 'Demo Member');
        setValue('input[name="username"]', 'demo-member');
        setValue('input[name="password"]', ${JSON.stringify(password)});
        await new Promise(resolve => setTimeout(resolve, 0));
        document.querySelector('button.login-submit').click();
        await waitFor(() => document.querySelector('.workspace-sidebar'), 'authenticated workspace');
        await waitFor(() => document.querySelector('.workspace-sidebar')?.textContent.includes('Client acceptance'), 'fixture workspace');
        await waitFor(() => [...document.querySelectorAll('.room-select-button')].some(button => button.textContent.trim() === 'Demo'), 'invited room membership');
        [...document.querySelectorAll('.room-select-button')].find(button => button.textContent.trim() === 'Demo').click();
        await waitFor(() => document.querySelector('textarea#chat-message'), 'room composer');
        await waitFor(() => !document.querySelector('.sidebar-skeleton'), 'room list settled');
        await waitFor(() => !document.querySelector('textarea#chat-message').disabled && !document.querySelector('.message-skeletons'), 'room ready to send');
        if (!${JSON.stringify(checkProject)}) {
          await waitFor(() => document.querySelector('.room-onboarding h2')?.textContent === 'Add agents to your workspace', 'English first-room guidance');
          const onboarding = document.querySelector('.room-onboarding');
          if (!onboarding.textContent.includes('people can chat without installing it.')) throw new Error('Human-only onboarding requirement is unclear');
          if (/[\u4e00-\u9fff]/.test(onboarding.textContent)) throw new Error('First-room guidance contains untranslated Chinese');
        } else {
          // Project fixtures already contain a topic, so empty-room guidance is
          // correctly absent. Assert the real seeded content instead.
          await waitFor(() => document.querySelector('.message-list')?.textContent.includes('Review the demo reference'), 'seeded project topic');
        }
        const memberToggle = document.querySelector('button[aria-label="Collapse room members"]');
        if (!memberToggle) throw new Error('English member collapse control missing');
        memberToggle.click();
        await waitFor(() => document.querySelector('button[aria-label="Expand room members"]')?.getAttribute('aria-expanded') === 'false', 'collapsed room members');
        document.querySelector('button[aria-label="Expand room members"]').click();
        await waitFor(() => document.querySelector('button[aria-label="Collapse room members"]')?.getAttribute('aria-expanded') === 'true', 'expanded room members');
        const message = document.querySelector('textarea#chat-message');
        Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(message, 'Hello team — member UI acceptance.');
        message.dispatchEvent(new Event('input', {bubbles:true}));
        await waitFor(() => !document.querySelector('button.send-button[type="submit"]')?.disabled, 'send enabled');
        document.querySelector('button.send-button[type="submit"]').click();
        await waitFor(() => document.querySelector('.message-list')?.textContent.includes('Hello team — member UI acceptance.'), 'member message delivered');
        await waitFor(() => document.querySelector('textarea#chat-message')?.value === '', 'sent composer cleared');
        if (document.querySelector('input[name="password"]')) throw new Error('Login form still visible');
        if (!document.querySelector('[aria-label="Create room"]')) throw new Error('English workspace actions missing');
        await waitFor(() => document.querySelector('.desktop-account-warning')?.textContent.includes('This computer is not connected to your account yet'), 'English connection guidance');
        const expectedNotice = registerMember ? 'Account created and room joined' : 'You joined the invited room';
        if (!document.querySelector('.join-notice')?.textContent.includes(expectedNotice)) throw new Error('Invitation notice is not in English');
        await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
        const shellBounds = document.querySelector('.app-shell').getBoundingClientRect();
        for (const notice of document.querySelectorAll('.join-notice, .desktop-account-warning')) {
          if (notice.getBoundingClientRect().bottom > shellBounds.top + 1) throw new Error('Account notice overlaps main navigation');
        }
        const roomHeader = document.querySelector('.room-header');
        const headerBounds = roomHeader.getBoundingClientRect();
        if (!roomHeader.contains(document.elementFromPoint(headerBounds.left + 20, headerBounds.top + 20))) throw new Error('Room header is obscured');
      })()`);
      const screenshot = path.join(root, 'authenticated-member.png');
      await window.webContents.executeJavaScript('new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))');
      fs.writeFileSync(screenshot, (await window.webContents.capturePage()).toPNG());
      const result = await window.webContents.executeJavaScript(`(async () => {
        const waitFor = async (predicate, label) => {
          const deadline = Date.now() + 30000;
          while (Date.now() < deadline) {
            if (await predicate()) return;
            await new Promise(resolve => setTimeout(resolve, 100));
          }
          throw new Error('Connection UI timed out: ' + label);
        };
        const previousTop = document.querySelector('.app-shell').getBoundingClientRect().top;
        document.querySelector('.join-notice button').click();
        await waitFor(() => !document.querySelector('.join-notice'), 'invitation notice dismissed');
        await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
        if (document.querySelector('.app-shell').getBoundingClientRect().top >= previousTop) throw new Error('Dismissed notice leaves unused space');
        const resolve = document.querySelector('.desktop-account-warning button');
        if (!resolve) throw new Error('Connection guidance button missing');
        resolve.click();
        await waitFor(() => document.querySelector('.agent-connect-action') && !document.querySelector('.agent-connect-action').disabled, 'connect button');
        document.querySelector('.agent-connect-action').click();
        await waitFor(async () => {
          const state = await window.agentCollabDesktop.getState();
          return state.runnerRegistered && state.runnerRunning;
        }, 'registered and running Client');
        await waitFor(() => document.querySelector('.agent-dialog') && !document.querySelector('.needs-connection'), 'connection dialog advances');
        await waitFor(() => document.querySelector('.agent-add-submit'), 'local discovery completes');
        if (document.querySelector('.agent-dialog [role="alert"]')) throw new Error('Local discovery reported an error');
        if (${JSON.stringify(checkAgentDrawer)}) {
          const choice = [...document.querySelectorAll('.agent-choice')].find(row => /opencode/i.test(row.textContent) && !row.querySelector('input').disabled);
          if (!choice) throw new Error('OpenCode must be installed for the optional real-agent drawer check');
          const input = choice.querySelector('input');
          if (!input.checked) input.click();
          await waitFor(() => !document.querySelector('.agent-add-submit')?.disabled, 'agent registration enabled');
          document.querySelector('.agent-add-submit').click();
          await waitFor(() => !document.querySelector('.agent-dialog'), 'agent registered');
          await waitFor(() => document.querySelector('.library-agent-actions button:not(:disabled)'), 'registered agent available');
          document.querySelector('.library-agent-actions button:not(:disabled)').click();
          await waitFor(() => document.querySelector('.agent-detail-trigger'), 'agent joined room');
          document.querySelector('.agent-detail-trigger').click();
          await waitFor(() => document.querySelector('.agent-detail-drawer'), 'agent settings drawer');
          const drawer = document.querySelector('.agent-detail-drawer');
          await Promise.all(drawer.getAnimations().map(animation => animation.finished));
          const options = [...drawer.querySelectorAll('select option')].map(option => option.textContent);
          if (JSON.stringify(options) !== JSON.stringify(['Only when @mentioned', 'Coordinator / automatic routing', 'Reply to all human messages', 'Muted'])) throw new Error('Agent response options are not English');
          if (!drawer.textContent.includes('shared with others.') || !drawer.textContent.includes('Manage skill permissions')) throw new Error('Owner skill permissions missing');
        } else {
          const close = document.querySelector('.agent-dialog header button');
          if (!close) throw new Error('Agent dialog close button missing');
          close.click();
        }
        await waitFor(() => !document.querySelector('.desktop-account-warning'), 'ownership warning clears');
        return window.agentCollabDesktop.getState();
      })()`);
      if (checkAgentDrawer) {
        await window.webContents.executeJavaScript(`new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))`);
        fs.writeFileSync(path.join(root, 'agent-settings-en.png'), (await window.webContents.capturePage()).toPNG());
        await window.webContents.executeJavaScript(`(async () => {
          document.querySelector('.agent-detail-drawer button[aria-label="Close agent room settings"]').click();
          await new Promise(resolve => setTimeout(resolve, 0));
          if (document.querySelector('.agent-detail-drawer')) throw new Error('Agent drawer did not close');
        })()`);
      }
      const credentials = path.join(root, 'client', 'runner', 'device.json');
      if (!fs.existsSync(credentials)) throw new Error('device credentials escaped isolated Client directory');
      const saved = JSON.parse(fs.readFileSync(credentials, 'utf8'));
      if (!saved.runner_id || !saved.device_id) throw new Error('pairing credentials incomplete');
      const settings = JSON.parse(fs.readFileSync(path.join(root, 'client', 'desktop-settings.json'), 'utf8'));
      if (settings.hubUrl !== base || settings.autoStart !== false) throw new Error('wrong team or startup policy');
      await new Promise((resolve, reject) => {
        const timeout = setTimeout(() => reject(new Error('Client reload timed out')), 15000);
        window.webContents.once('did-finish-load', () => { clearTimeout(timeout); resolve(); });
        window.webContents.reload();
      });
      await window.webContents.executeJavaScript(`(async () => {
        const waitFor = async (predicate, label) => {
          const deadline = Date.now() + 15000;
          while (Date.now() < deadline) {
            if (predicate()) return;
            await new Promise(resolve => setTimeout(resolve, 50));
          }
          throw new Error('Reload UI timed out: ' + label);
        };
        await waitFor(() => [...document.querySelectorAll('.room-select-button')].some(button => button.textContent.trim() === 'Demo'), 'saved member session');
        [...document.querySelectorAll('.room-select-button')].find(button => button.textContent.trim() === 'Demo').click();
        await waitFor(() => document.querySelector('.message-list')?.textContent.includes('Hello team — member UI acceptance.'), 'persisted message');
        if (${JSON.stringify(checkAgentDrawer)}) await waitFor(() => [...document.querySelectorAll('.agent-detail-trigger')].some(button => /opencode/i.test(button.getAttribute('aria-label'))), 'persisted OpenCode room membership');
        if (document.documentElement.lang !== 'en') throw new Error('Locale lost after reload');
        if (document.querySelector('input[name="password"]')) throw new Error('Reload unexpectedly requires login');
      })()`);
      const afterReload = JSON.parse(fs.readFileSync(credentials, 'utf8'));
      if (afterReload.runner_id !== saved.runner_id || afterReload.device_id !== saved.device_id) throw new Error('Device identity changed after reload');
      if (checkProject) {
        window.setSize(1440, 1000);
        await window.webContents.executeJavaScript(`(async () => {
          const deadline = Date.now() + 15000;
          const board = [...document.querySelectorAll('.room-surface-tabs button')].find(button => /协作台|Collaboration/.test(button.textContent));
          if (!board) throw new Error('Collaboration tab missing');
          board.click();
          while (!document.querySelector('.project-office .office-step-room')) {
            if (Date.now() > deadline) throw new Error('Project office did not load');
            await new Promise(resolve => setTimeout(resolve, 100));
          }
          const office = document.querySelector('.project-office');
          if (!office.textContent.includes('Project office') || !office.textContent.includes('Reference review')) throw new Error('English project fixture missing');
          if (/[\\u4e00-\\u9fff]/.test(office.textContent)) throw new Error('Untranslated project office content');
          if (office.scrollWidth > office.clientWidth + 2) throw new Error('Project office overflows horizontally');
          const tools = document.querySelector('.room-inspector');
          for (const label of ['Room tools', 'Default room step', 'My agent settings', 'Advanced · Connection checks']) {
            if (!tools?.textContent.includes(label)) throw new Error('English room tool missing: ' + label);
          }
          if (!tools.querySelector('select[aria-label="Default project step for this room"]')?.disabled) throw new Error('Ordinary member unexpectedly allowed to change project default');
          const stepBox = tools.querySelector('.room-project-link').getBoundingClientRect();
          const stepSelect = tools.querySelector('.room-project-link select').getBoundingClientRect();
          if (stepSelect.right > stepBox.right || stepSelect.left < stepBox.left) throw new Error('Default step selector exceeds sidebar width');
        })()`);
        fs.writeFileSync(path.join(root, 'project-office-en.png'), (await window.webContents.capturePage()).toPNG());
        await window.webContents.executeJavaScript(`(async () => {
          const button = [...document.querySelectorAll('.office-task-file')].find(button => button.textContent.includes('Review the demo reference'));
          if (!button) throw new Error('Fixture work item missing from step');
          button.click();
          const deadline = Date.now() + 10000;
          while (!document.querySelector('.office-detail-drawer')) {
            if (Date.now() > deadline) throw new Error('Work item detail did not open');
            await new Promise(resolve => setTimeout(resolve, 50));
          }
          const drawer = document.querySelector('.office-detail-drawer');
          if (!drawer.textContent.includes('Business') || /[\\u4e00-\\u9fff]/.test(drawer.textContent)) throw new Error('English details incomplete');
          await Promise.all(drawer.getAnimations().map(animation => animation.finished));
          await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
          if (Number(getComputedStyle(drawer).opacity) !== 1) throw new Error('Detail drawer remained transparent');
          const rect = drawer.getBoundingClientRect();
          if (!drawer.contains(document.elementFromPoint(rect.x + rect.width / 2, rect.y + 40))) throw new Error('Detail drawer is obscured by another panel');
        })()`);
        fs.writeFileSync(path.join(root, 'project-detail-en.png'), (await window.webContents.capturePage()).toPNG());
        console.log(JSON.stringify({project:'PASS', screenshots:['project-office-en.png','project-detail-en.png'].map(name=>path.join(root,name))}));
      }
      passed = true;
      console.log(JSON.stringify({result:'PASS',electron:process.versions.electron,authMode:registerMember ? 'register' : 'login',agentDrawer:checkAgentDrawer ? 'PASS' : 'not requested',flow:'invitation -> member English authentication form -> room message -> GUI connect button -> reload retains session, message, language and device identity',configured:Boolean(result),screenshot}));
      app.quit();
    } catch (error) { fail(error); }
  });
});
(async () => {
  const admin = spawnSync(hub, ['admin','create','--config',config,'--username','demo-owner','--display-name','Demo Owner'], {env:{...process.env,AGENTCOLLAB_ADMIN_PASSWORD:password},encoding:'utf8'});
  if (admin.status !== 0) throw new Error('temporary administrator creation failed');
  hubProcess = spawn(hub, ['serve','--config',config], {stdio:['ignore','pipe','pipe']});
  hubProcess.stderr.resume();
  base = await new Promise((resolve, reject) => {
    let output='';
    hubProcess.stdout.on('data', chunk => { output+=chunk; const match=output.match(/hub_listening=(\S+)/);if(match)resolve('http://'+match[1]); });
    hubProcess.once('error',reject);
    hubProcess.once('exit',()=>reject(new Error('test Hub exited before startup')));
  });
  let cookie='';
  async function api(url,data){const r=await fetch(base+url,{method:'POST',headers:{'Content-Type':'application/json',Cookie:cookie},body:JSON.stringify(data)});if(!r.ok)throw new Error('fixture API '+r.status);if(r.headers.get('set-cookie'))cookie=r.headers.get('set-cookie').split(';')[0];return r.json();}
  await api('/api/v1/auth/login',{username:'demo-owner',password});
  workspace=await api('/api/v1/workspaces',{name:'Client acceptance'});
  const room=await api('/api/v1/workspaces/'+workspace.id+'/rooms',{name:'Demo'});
  if (checkProject) {
    const step = await api('/api/v1/workspaces/'+workspace.id+'/project-steps',{title:'Reference review',weight:60});
    await api('/api/v1/workspaces/'+workspace.id+'/project-steps',{title:'Publish findings',weight:40});
    await api('/api/v1/rooms/'+room.id+'/messages',{body:'Review the demo reference',plan_step_id:step.id});
  }
  invite=await api('/api/v1/workspaces/'+workspace.id+'/invites',{room_id:room.id,role:'member',expires_in_seconds:300});
  if (!registerMember) await api('/api/v1/auth/register',{username:'demo-member',display_name:'Demo Member',password});
  process.argv.push('agentcollab://join?'+new URLSearchParams({hub:base,invite:invite.code}));
  app.setAppPath(path.join(repo,'desktop'));
  require(path.join(repo,'desktop/main.cjs'));
})().catch(fail);

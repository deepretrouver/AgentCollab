// Run with an Electron executable, not Node. Creates only temporary test data.
const { app } = require('electron');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const net = require('node:net');
const { randomUUID } = require('node:crypto');
const root = fs.mkdtempSync(path.join(os.tmpdir(), 'agentcollab-desktop-smoke-'));
const server = path.resolve(__dirname, '../server-desktop');
process.env.AGENTCOLLAB_SERVER_DATA_DIR = root;
// Set the profile before asynchronous port selection can initialize Electron.
app.setPath('userData', root);
app.setAppPath(server);
let selectedPort;
let configured = false;
let completed = false;
// The production shutdown path uses app.exit(), bypassing will-quit and the
// Node exitCode. Preserve its cleanup while making failed assertions fail CI.
const exitApplication = app.exit.bind(app);
app.exit = code => {
  clearTimeout(timer);
  if (completed) {
    fs.rmSync(root, { recursive: true, force: true });
    console.log('PASS: shutdown completed; temporary data removed');
  }
  exitApplication(completed ? code : 1);
};
const timer = setTimeout(() => {
  console.error('FAIL: desktop startup/configuration timeout; evidence directory:', root);
  process.exitCode = 1;
  app.quit();
}, 60000);
app.on('browser-window-created', (_event, window) => {
  window.webContents.on('did-finish-load', async () => {
    try {
      const url = window.webContents.getURL();
      if (url.endsWith('/onboarding/index.html') && !configured) {
        configured = true;
        const password = randomUUID();
        await window.webContents.executeJavaScript(`(async () => {
          const state = await window.agentCollabServer.getState();
          const set = (id, value) => { document.getElementById(id).value = value; };
          set('language', 'en'); document.getElementById('language').dispatchEvent(new Event('change'));
          set('name', 'Community Demo'); set('addressMode', 'manual');
          document.getElementById('addressMode').dispatchEvent(new Event('change'));
          set('url', 'http://127.0.0.1:' + state.port);
          set('display', 'Demo Owner'); set('password', ${JSON.stringify(password)});
          document.getElementById('create').click();
        })()`);
      } else if (url.endsWith('/dashboard/index.html') && !completed) {
        const state = await window.webContents.executeJavaScript('window.agentCollabServer.getState()');
        if (!state.running) throw new Error('dashboard loaded but Server not healthy');
        if (state.port !== selectedPort) throw new Error('Server did not use the isolated test port');
        if (state.hubName !== 'Community Demo' || !state.storageRoot.startsWith(root + path.sep)) throw new Error('test isolation failed');
        if (process.env.AGENTCOLLAB_SMOKE_FORCE_FAILURE === '1') throw new Error('intentional acceptance failure');
        clearTimeout(timer);
        const screenshot = path.join(os.tmpdir(), `agentcollab-server-demo-${Date.now()}.png`);
        fs.writeFileSync(screenshot, (await window.webContents.capturePage()).toPNG());
        completed = true;
        console.log(JSON.stringify({ result: 'PASS', electron: process.versions.electron, port: selectedPort, flow: 'onboarding form -> IPC -> Hub startup -> healthy dashboard', screenshot }));
        await window.webContents.executeJavaScript('window.agentCollabServer.quit()');
      }
    } catch (error) {
      clearTimeout(timer);
      console.error('FAIL:', error.message, 'evidence directory:', root);
      process.exitCode = 1;
      app.quit();
    }
  });
});
// Release immediately before starting the Hub. If another process races for
// this port, the exact-port assertion above fails rather than accepting it.
const reservation = net.createServer();
reservation.once('error', error => {
  console.error('FAIL: temporary port allocation:', error.message);
  app.exit(1);
});
reservation.listen(0, '127.0.0.1', () => {
  selectedPort = reservation.address().port;
  reservation.close(() => {
    process.env.AGENTCOLLAB_SERVER_PORT = String(selectedPort);
    require(path.join(server, 'main.cjs'));
  });
});

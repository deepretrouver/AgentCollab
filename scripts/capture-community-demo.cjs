// Optional real-UI capture for smoke-community. Credentials stay in process env,
// never in argv, screenshots metadata, or a public evidence file.
const { app, BrowserWindow, session } = require('electron');
const { mkdtempSync, writeFileSync } = require('node:fs');
const { tmpdir } = require('node:os');
const { join } = require('node:path');
const profile = mkdtempSync(join(tmpdir(), 'agentcollab-demo-browser-'));
app.setPath('userData', profile);
const base = process.env.AGENTCOLLAB_DEMO_URL;
const cookie = process.env.AGENTCOLLAB_DEMO_COOKIE;
const output = process.env.AGENTCOLLAB_DEMO_OUTPUT;
delete process.env.AGENTCOLLAB_DEMO_COOKIE;
const timer = setTimeout(() => app.exit(1), 45000);
app.whenReady().then(async () => {
  if (!base || !cookie || !output || new URL(base).hostname !== '127.0.0.1') throw new Error('Isolated demo inputs required');
  const split = cookie.indexOf('=');
  await session.defaultSession.cookies.set({ url: base, name: cookie.slice(0, split), value: cookie.slice(split + 1), httpOnly: true });
  const win = new BrowserWindow({ width: 1440, height: 1000, show: false, webPreferences: { nodeIntegration: false, contextIsolation: true } });
  await win.loadURL(base);
  await win.webContents.executeJavaScript("localStorage.setItem('agentcollab.locale','en')");
  await win.loadURL(base);
  await win.webContents.executeJavaScript(`(async () => {
    const wait = async (fn) => { const until=Date.now()+20000; while(!fn()){if(Date.now()>until)throw new Error('Demo UI timed out'); await new Promise(r=>setTimeout(r,100));} };
    await wait(()=>document.querySelector('.workspace-sidebar'));
    const language=document.querySelector('select[aria-label="Language / 语言"]');
    if(language){language.value='en';language.dispatchEvent(new Event('change',{bubbles:true}));}
    await wait(()=>[...document.querySelectorAll('.room-select-button')].some(b=>b.textContent.trim()==='Demo discussion'));
    [...document.querySelectorAll('.room-select-button')].find(b=>b.textContent.trim()==='Demo discussion').click();
    await wait(()=>document.querySelectorAll('.topic-thread-disclosure').length>0);
    await wait(()=>document.querySelector('.message-list')?.textContent.includes('COMMUNITY_OK_'));
    await wait(()=>!document.querySelector('.message-skeletons'));
    for(const detail of document.querySelectorAll('.topic-thread-disclosure'))if(!detail.open)detail.querySelector('summary').click();
    await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));
    document.querySelector('.message-list').scrollTop=0;
  })()`);
  writeFileSync(join(output, 'native-delivery-en.png'), (await win.webContents.capturePage()).toPNG());
  await win.webContents.executeJavaScript(`(async()=>{
    const tab=[...document.querySelectorAll('.room-surface-tabs button')].find(b=>/Collaboration|协作台/.test(b.textContent));
    if(!tab)throw new Error('Collaboration tab missing');tab.click();
    const until=Date.now()+15000;while(!document.querySelector('.office-control-rail')){if(Date.now()>until)throw new Error('Project office missing');await new Promise(r=>setTimeout(r,100));}
    const activity=[...document.querySelectorAll('.office-control-rail nav button')].find(b=>/Recent activity|最近流转/.test(b.textContent));
    if(!activity)throw new Error('Recent activity missing');activity.click();
    while(!document.querySelector('.office-activity-list button')){if(Date.now()>until)throw new Error('Completed project item missing');await new Promise(r=>setTimeout(r,100));}
    document.querySelector('.office-activity-list button').click();
    while(!document.querySelector('.office-detail-drawer')){if(Date.now()>until)throw new Error('Project detail missing');await new Promise(r=>setTimeout(r,100));}
    const tab2=[...document.querySelectorAll('.office-detail-drawer button')].find(b=>/Git and acceptance|Git 与验收/.test(b.textContent));
    if(tab2)tab2.click();
    await Promise.all(document.querySelector('.office-detail-drawer').getAnimations().map(a=>a.finished));
    await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));
  })()`);
  writeFileSync(join(output, 'native-project-en.png'), (await win.webContents.capturePage()).toPNG());
  // Explicitly clear authentication before exiting this temporary profile.
  await session.defaultSession.clearStorageData();
  clearTimeout(timer);
  app.exit(0);
}).catch(async () => {
  if (app.isReady()) await session.defaultSession.clearStorageData();
  console.error('Demo capture failed; do not publish unreviewed captures.');
  clearTimeout(timer);
  app.exit(1);
});

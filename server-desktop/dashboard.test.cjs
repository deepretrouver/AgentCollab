const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('../web/node_modules/jsdom');

test('dashboard retains English, translates status and copy feedback without changing team data', async () => {
  const folder = path.join(__dirname, 'dashboard');
  const html = fs.readFileSync(path.join(folder, 'index.html'), 'utf8').replace('<script src="i18n.js"></script>', `<script>${fs.readFileSync(path.join(folder, 'i18n.js'), 'utf8')}</script>`);
  let notify;
  const state = { version: '1.3.3', hubName: '演示 Team', publicUrl: 'http://team.example', localUrl: 'http://localhost', addressMode: 'manual', storageRoot: '/tmp/demo', port: 18081, running: true };
  const dom = new JSDOM(html, { url: 'http://localhost/', runScripts: 'dangerously', beforeParse(window) {
    window.localStorage.setItem('agentcollab.locale', 'en');
    window.agentCollabServer = { getState: async () => state, onStatus: callback => { notify = callback; }, copyTeamURL: async () => state.publicUrl };
  } });
  try {
    await new Promise(resolve => setImmediate(resolve));
    const field = id => dom.window.document.getElementById(id);
    assert.equal(field('statusText').textContent, 'Server is running');
    assert.equal(field('name').textContent, state.hubName);
    assert.equal(field('copyUrl').textContent, 'Copy team address');
    assert.match(dom.window.document.querySelector('.help').textContent, /Members can use a browser; install the Client only to connect local agents/);
    field('copyUrl').click();
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(field('feedback').textContent, 'Team address copied: http://team.example');
    notify({ ...state, running: false });
    assert.equal(field('statusText').textContent, 'Server is recovering');
    field('language').value = 'zh-CN';
    field('language').dispatchEvent(new dom.window.Event('change'));
    assert.equal(field('statusText').textContent, 'Server 正在自动恢复');
    assert.equal(field('copyUrl').textContent, '复制团队入口');
    assert.equal(field('url').textContent, state.publicUrl);
    assert.match(dom.window.document.querySelector('.help').textContent, /成员可以直接用浏览器参与讨论；仅在接入本机 Agent 时需要安装 Client/);
    assert.doesNotMatch(dom.window.document.querySelector('.help').textContent, /其他成员只安装 Client/);
  } finally { dom.window.close(); }
});

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('../web/node_modules/jsdom');

test('Server onboarding switches language without losing configuration', async () => {
  let submitted;
  const dom = new JSDOM(fs.readFileSync(path.join(__dirname, 'onboarding/index.html'), 'utf8'), {
    url: 'http://localhost/', runScripts: 'dangerously',
    beforeParse(window) {
      window.agentCollabServer = {
        getState: async () => ({ hubName: 'Demo', port: 18080, publicUrl: 'http://127.0.0.1:18080', storageRoot: '/tmp/demo' }),
        configure: async value => { submitted = value; },
      };
    },
  });
  try {
    await new Promise(resolve => setImmediate(resolve));
    const { document, Event } = dom.window;
    const field = id => document.getElementById(id);
    field('name').value = 'My team';
    field('password').value = 'temporary-test-password';
    field('addressMode').value = 'manual';
    field('url').value = 'https://team.example';
    field('language').value = 'en';
    field('language').dispatchEvent(new Event('change'));
    assert.equal(document.documentElement.lang, 'en');
    assert.equal(document.querySelector('h1').textContent, 'Create your team Server');
    assert.match(field('note').textContent, /Fixed mode/);
    assert.equal(field('name').value, 'My team');
    assert.equal(field('password').value, 'temporary-test-password');
    assert.equal(field('url').value, 'https://team.example');
    assert.equal(dom.window.localStorage.getItem('agentcollab.locale'), 'en');
    field('create').click();
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(submitted.hubName, 'My team');
    assert.equal(submitted.publicUrl, 'https://team.example');
    assert.equal(submitted.port, 18080);
    field('language').value = 'zh-CN';
    field('language').dispatchEvent(new Event('change'));
    assert.equal(document.querySelector('h1').textContent, '创建团队协作中心');
  } finally { dom.window.close(); }
});

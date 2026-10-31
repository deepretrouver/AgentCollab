const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('../web/node_modules/jsdom');

for (const configured of [false, true]) {
  test(`Client language switch preserves invitation (${configured ? 'reconnect' : 'first run'})`, async () => {
    let submitted;
    const invitation = 'http://team.example:8080/?join=demo';
    const query = new URLSearchParams({ hub: invitation });
    if (configured) query.set('configured', '1');
    const dom = new JSDOM(fs.readFileSync(path.join(__dirname, 'onboarding/index.html'), 'utf8'), {
      url: `http://localhost/?${query}`, runScripts: 'dangerously',
      beforeParse(window) {
        window.localStorage.setItem('agentcollab.locale', 'en');
        window.agentCollabDesktop = { configure: async value => { submitted = value; throw new Error(''); } };
      },
    });
    try {
      const { document, Event } = dom.window;
      const field = id => document.getElementById(id);
      assert.equal(document.documentElement.lang, 'en');
      assert.equal(field('title').textContent, configured ? 'Reconnect to your team' : 'Join your team');
      assert.equal(field('hub').value, invitation);
      field('continue').click();
      await new Promise(resolve => setImmediate(resolve));
      assert.deepEqual(submitted.hubUrl, invitation);
      assert.equal(field('continue').disabled, false);
      assert.equal(field('error').textContent, 'Unable to complete setup');
      field('language').value = 'zh-CN';
      field('language').dispatchEvent(new Event('change'));
      assert.equal(field('hub').value, invitation);
      assert.equal(field('title').textContent, configured ? '重新连接团队' : '加入团队');
      assert.equal(dom.window.localStorage.getItem('agentcollab.locale'), 'zh-CN');
    } finally { dom.window.close(); }
  });
}

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '..');

test('Client migration never terminates every Runner by executable name', () => {
  const source = fs.readFileSync(path.join(root, 'desktop/main.cjs'), 'utf8');
  assert.doesNotMatch(source, /spawnSync\("taskkill\.exe",\s*\["\/IM"/);
});

test('isolated Client profiles do not migrate real host services or claim the protocol', () => {
  const source = fs.readFileSync(path.join(root, 'desktop/main.cjs'), 'utf8');
  assert.match(source, /function migrateLegacyServices\(\)\s*\{\s*if \(isolatedData \|\| !app\.isPackaged\) return;/);
  assert.match(source, /if \(!isolatedData && app\.isPackaged\) app\.setAsDefaultProtocolClient\("agentcollab"\)/);
});

for (const directory of ['desktop', 'server-desktop']) {
  test(`${directory} guards every login-item change in isolated profiles`, () => {
    const source = fs.readFileSync(path.join(root, directory, 'main.cjs'), 'utf8');
    const calls = source.split('\n').filter(line => line.includes('app.setLoginItemSettings('));
    assert.ok(calls.length > 0);
    for (const line of calls) {
      assert.match(line, /if \(!isolatedData\) app\.setLoginItemSettings\(\{ openAtLogin: false \}\);/);
    }
  });
}

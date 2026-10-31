const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { copyNotices } = require('./package-notices.cjs');

test('copies exact license bytes and fails when a required notice is absent', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'agentcollab-notices-'));
  try {
    const source = path.join(root, 'LICENSE');
    fs.writeFileSync(source, 'Upstream copyright\nLicense text\n');
    const target = path.join(root, 'out');
    copyNotices({ 'LICENSE.txt': source }, target);
    assert.deepEqual(fs.readFileSync(path.join(target, 'LICENSE.txt')), fs.readFileSync(source));
    assert.throws(() => copyNotices({ absent: path.join(root, 'missing') }, target));
    fs.writeFileSync(source, '');
    assert.throws(() => copyNotices({ empty: source }, target), /Missing license notice/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

for (const platform of ['macos', 'linux']) {
  test(`${platform} compatibility installer preserves registration and starts manually`, () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'agentcollab-installer-'));
    try {
      fs.copyFileSync(path.join(__dirname, `../packaging/runner/install-${platform}.sh`), path.join(root, 'install.sh'));
      fs.writeFileSync(path.join(root, 'agentcollab-runner'), '#!/bin/sh\nprintf "args=%s\\n" "$*"\n', { mode: 0o755 });
      fs.mkdirSync(path.join(root, '.agentcollab'));
      const config = path.join(root, '.agentcollab/runner.yaml');
      fs.writeFileSync(config, 'existing-registration');
      const result = spawnSync('sh', [path.join(root, 'install.sh')], { env: { ...process.env, HOME: root }, encoding: 'utf8' });
      assert.equal(result.status, 0, result.stderr);
      assert.match(result.stdout, /args=\n/);
      assert.equal(fs.readFileSync(config, 'utf8'), 'existing-registration');
      assert.equal(fs.existsSync(path.join(root, 'Library')), false);
    } finally { fs.rmSync(root, { recursive: true, force: true }); }
  });
}

test('Windows compatibility script has no startup or protocol registration', () => {
  const text = fs.readFileSync(path.join(__dirname, '../packaging/runner/install-windows.ps1'), 'utf8');
  assert.doesNotMatch(text, /GetFolderPath|CreateShortcut|HKCU:|schtasks|Start-Process/i);
  assert.match(text, /& \$runner @args/);
});

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const { spawnSync } = require('node:child_process');
const root = path.resolve(__dirname, '..');
const read = file => fs.readFileSync(path.join(root, file), 'utf8');
const hasPowerShell = spawnSync('pwsh', ['-NoProfile', '-Command', 'exit 0'], { timeout: 15000 }).status === 0;

for (const [job, next] of [['mac', 'windows'], ['windows', 'server']]) {
  test(`${job} downloadable Client requires packaged acceptance before archive/upload`, () => {
    const workflow = read('.github/workflows/desktop-release.yml');
    const section = workflow.split(`\n  ${job}:\n`)[1]?.split(`\n  ${next}:\n`)[0];
    assert.ok(section);
    const acceptance = section.indexOf('node scripts/smoke-packaged-client.mjs');
    assert.ok(acceptance >= 0);
    assert.ok(acceptance < section.indexOf('Archive downloadable Client'));
    assert.ok(acceptance < section.indexOf('actions/upload-artifact'));
    assert.doesNotMatch(section, /continue-on-error:\s*true/);
  });
}

test('Windows Server has a distinct installer and bundles all Client downloads', () => {
  const server = JSON.parse(read('server-desktop/package.json'));
  const client = JSON.parse(read('desktop/package.json'));
  assert.notEqual(server.build.appId, client.build.appId);
  assert.notEqual(server.build.productName, client.build.productName);
  assert.match(server.scripts['dist:win'], /--win nsis/);
  assert.deepEqual(server.build.win.target, [{ target: 'nsis', arch: ['x64'] }]);
  assert.equal(server.build.nsis.oneClick, false);
  assert.equal(server.build.nsis.perMachine, false);
  assert.ok(fs.existsSync(path.join(root, 'server-desktop', server.build.win.icon)));
  assert.ok(server.build.extraResources.some(entry => entry.to === 'client-packages'));
});

test('Windows Hub build requires real SQLite and verifies writes before packaging', () => {
  const script = read('scripts/build-server-windows.ps1');
  assert.match(script, /Get-Command gcc/);
  assert.match(script, /\$env:CGO_ENABLED = "1"/);
  assert.match(script, /agentcollab-hub\.exe \.\/cmd\/hub/);
  assert.match(script, /node scripts\/smoke-community\.mjs server-desktop\/runtime\/agentcollab-hub\.exe/);
  assert.match(script, /finally\s*\{\s*\$env:CGO_ENABLED = \$PreviousCGO/);
  assert.match(script, /\$env:CC = \$PreviousCC/);
  assert.doesNotMatch(script, /Startup|Register-ScheduledTask|New-Service/);
});

for (const failure of ['', 'npm', 'metadata', 'go', 'node', 'compiler']) {
  test(`PowerShell build control flow: ${failure || 'success'}`, { skip: !hasPowerShell }, () => {
    const fixture = fs.mkdtempSync(path.join(os.tmpdir(), 'agentcollab-windows-build-'));
    try {
      fs.mkdirSync(path.join(fixture, 'scripts'));
      fs.mkdirSync(path.join(fixture, 'web/dist'), { recursive: true });
      fs.writeFileSync(path.join(fixture, 'web/dist/index.html'), 'public test fixture');
      fs.copyFileSync(path.join(root, 'scripts/build-server-windows.ps1'), path.join(fixture, 'scripts/build.ps1'));
      // Fake external commands only. Real PowerShell directory/copy/finally logic executes
      // exclusively inside this temporary repository; no compiler or package manager runs.
      const script = `
$ErrorActionPreference = 'Stop'
$global:Calls = [System.Collections.Generic.List[string]]::new()
function global:Get-Command { param($Name, $ErrorAction) if ($Name -eq 'gcc' -and $env:FAIL_STAGE -ne 'compiler') { return 'fixture gcc' } }
function global:npm { $global:Calls.Add('npm'); $global:LASTEXITCODE = [int]($env:FAIL_STAGE -eq 'npm') }
function global:go {
  if ($env:CGO_ENABLED -ne '1' -or $env:CC -ne 'gcc') { throw 'SQLite compiler configuration missing' }
  if (($args -join ' ') -notmatch '-ldflags -X fixture.Version=1.3.3') { throw 'Native build identity missing' }
  $global:Calls.Add('go'); $global:LASTEXITCODE = [int]($env:FAIL_STAGE -eq 'go')
}
function global:node {
  if ($args[0] -eq 'scripts/native-build-info.mjs') { $global:LASTEXITCODE = [int]($env:FAIL_STAGE -eq 'metadata'); return '-X fixture.Version=1.3.3' }
  $global:Calls.Add('node'); $global:LASTEXITCODE = [int]($env:FAIL_STAGE -eq 'node')
}
$before = (Get-Location).Path
$env:CGO_ENABLED = 'original-cgo'
$env:CC = 'original-cc'
$failed = $false
try { & ./scripts/build.ps1 } catch { $failed = $true }
if ($failed -ne [bool]$env:FAIL_STAGE) { throw 'Unexpected build success/failure' }
if ($env:CGO_ENABLED -ne 'original-cgo' -or $env:CC -ne 'original-cc') { throw 'Compiler environment leaked' }
if ((Get-Location).Path -ne $before) { throw 'Working directory leaked' }
@{ calls = @($global:Calls.ToArray()); failed = $failed } | ConvertTo-Json -Compress
`;
      const result = spawnSync('pwsh', ['-NoProfile', '-Command', script], {
        cwd: fixture, env: { ...process.env, FAIL_STAGE: failure }, encoding: 'utf8', timeout: 30000,
      });
      assert.equal(result.status, 0, result.stderr || result.error?.message);
      const outcome = JSON.parse(result.stdout.trim());
      const expected = failure === 'compiler' ? [] : ['npm', 'metadata'].includes(failure) ? ['npm'] : failure === 'go' ? ['npm', 'go'] : ['npm', 'go', 'node'];
      assert.deepEqual(outcome.calls, expected);
      assert.equal(outcome.failed, Boolean(failure));
      if (!['compiler', 'npm'].includes(failure)) {
        assert.equal(fs.readFileSync(path.join(fixture, 'internal/httpapi/webdist/index.html'), 'utf8'), 'public test fixture');
      }
    } finally {
      fs.rmSync(fixture, { recursive: true, force: true });
    }
  });
}

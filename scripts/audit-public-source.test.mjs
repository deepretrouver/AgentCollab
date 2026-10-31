import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, writeFile, symlink, rm, mkdir, readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync, spawnSync } from 'node:child_process';

const script = fileURLToPath(new URL('./audit-public-source.mjs', import.meta.url));
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'agentcollab-source-audit-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  execFileSync('git', ['init', '--quiet', root]);
  return root;
}
function audit(root) {
  const result = spawnSync(process.execPath, [script], { cwd: root, encoding: 'utf8' });
  return { status: result.status, report: JSON.parse(result.stdout), output: result.stdout };
}

test('accepts source and reports fake credentials without revealing values', async t => {
  const root = await fixture(t);
  await writeFile(join(root, 'README.md'), '# Example\n');
  assert.equal(audit(root).status, 0);
  const token = 'gh' + 'p_' + 'x'.repeat(36);
  await writeFile(join(root, 'example.txt'), token);
  const result = audit(root);
  assert.equal(result.status, 1);
  assert.ok(result.report.findings.some(f => f.reason === 'github-token'));
  assert.ok(!result.output.includes(token));
});

test('rejects broken links instead of silently ignoring them', async t => {
  const root = await fixture(t);
  await symlink(join(root, '..', 'missing-private-file'), join(root, 'external-link'));
  const result = audit(root);
  assert.equal(result.status, 1);
  assert.deepEqual(result.report.findings, [{ path: 'external-link', reason: 'symlink-review' }]);
});

test('does not scan the contents behind an external symlink', async t => {
  const root = await fixture(t);
  const outside = await mkdtemp(join(tmpdir(), 'agentcollab-private-fixture-'));
  t.after(() => rm(outside, { recursive: true, force: true }));
  const token = 'gh' + 'p_' + 'y'.repeat(36);
  await writeFile(join(outside, 'private.txt'), token);
  await symlink(join(outside, 'private.txt'), join(root, 'external-link'));
  const result = audit(root);
  assert.equal(result.status, 1);
  assert.deepEqual(result.report.findings, [{ path: 'external-link', reason: 'symlink-review' }]);
  assert.ok(!result.output.includes(token));
  assert.ok(!result.output.includes(outside));
});

test('requires archive review even when binary data contains NUL bytes', async t => {
  const root = await fixture(t);
  await writeFile(join(root, 'old-backup.zip'), Buffer.from([80, 75, 0, 1]));
  const result = audit(root);
  assert.equal(result.status, 1);
  assert.ok(result.report.findings.some(f => f.reason === 'archive-or-installer-review'));
});

test('does not silently accept unknown binary or UTF-16 files', async t => {
  const root = await fixture(t);
  await writeFile(join(root, 'payload'), Buffer.from([0, 1, 2, 3]));
  const token = 'gh' + 'p_' + 'z'.repeat(36);
  await writeFile(join(root, 'settings.txt'), Buffer.from(token, 'utf16le'));
  const result = audit(root);
  assert.equal(result.status, 1);
  assert.deepEqual(result.report.findings, [
    { path: 'payload', reason: 'binary-file-review' },
    { path: 'settings.txt', reason: 'binary-file-review' },
  ]);
  assert.ok(!result.output.includes(token));
});

test('reviewed image approval is bound to both exact path and bytes', async t => {
  const root = await fixture(t);
  const bytes = await readFile(new URL('../desktop/assets/icon.png', import.meta.url));
  await mkdir(join(root, 'desktop/assets'), { recursive: true });
  await writeFile(join(root, 'desktop/assets/icon.png'), bytes);
  const accepted = audit(root);
  assert.equal(accepted.status, 0);
  assert.equal(accepted.report.reviewedBinaryFiles.length, 1);
  await writeFile(join(root, 'renamed.png'), bytes);
  assert.deepEqual(audit(root).report.findings, [{ path: 'renamed.png', reason: 'binary-file-review' }]);
  await rm(join(root, 'renamed.png'));
  await writeFile(join(root, 'desktop/assets/icon.png'), Buffer.concat([bytes, Buffer.from('extra')]));
  assert.deepEqual(audit(root).report.findings, [{ path: 'desktop/assets/icon.png', reason: 'binary-file-review' }]);
});

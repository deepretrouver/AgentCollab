// Read-only audit of candidate public files. Reports locations, never values.
import { execFileSync } from 'node:child_process';
import { readFile, lstat } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import reviewedAssets from './reviewed-public-assets.json' with { type: 'json' };
const paths = execFileSync('git', ['ls-files', '--cached', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const checks = [
  ['private-key', /-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/],
  ['github-token', /\b(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,})/],
  ['provider-key', /\bsk-[A-Za-z0-9_-]{24,}/],
  ['personal-path', /\/(?:Users\/yukeguo|Volumes\/30\.40\.)/],
  ['private-network', /\b30\.40\.\d{1,3}\.\d{1,3}\b/],
];
const findings = [];
const reviewedBinaryFiles = [];
for (const path of [...new Set(paths)].sort()) {
  let info;
  try { info = await lstat(path); } catch (error) {
    // Tracked deletions are absent from the candidate tree; other inspection
    // failures must not silently turn a partial scan into a clean result.
    if (error.code !== 'ENOENT') findings.push({ path, reason: 'unreadable-file' });
    continue;
  }
  if (info.isSymbolicLink()) { findings.push({ path, reason: 'symlink-review' }); continue; }
  if (!info.isFile()) continue;
  if (/\.(?:zip|tar|tgz|gz|bz2|xz|7z|rar|dmg|exe|msi)$/i.test(path)) {
    findings.push({ path, reason: 'archive-or-installer-review' });
    continue;
  }
  if (/(?:^|\/)(?:\.env(?:\..+)?|device\.json|auth\.json|hub\.yaml|runner\.yaml)$|\.(?:db|sqlite|sqlite3|log|pem|key)$/.test(path)) findings.push({ path, reason: 'runtime-or-secret-file' });
  if (info.size > 5 * 1024 * 1024) { findings.push({ path, reason: 'large-file-review' }); continue; }
  const bytes = await readFile(path);
  // Binary assets and UTF-16 text cannot be cleared by this UTF-8 scanner.
  // Require explicit review instead of treating an uninspected file as safe.
  if (bytes.includes(0)) {
    const digest = createHash('sha256').update(bytes).digest('hex');
    if (Object.hasOwn(reviewedAssets, path) && reviewedAssets[path] === digest) reviewedBinaryFiles.push({ path, sha256: digest });
    else findings.push({ path, reason: 'binary-file-review' });
    continue;
  }
  const lines = bytes.toString('utf8').split('\n');
  for (let index = 0; index < lines.length; index++) {
    for (const [reason, pattern] of checks) {
      if (pattern.test(lines[index])) findings.push({ path, line: index + 1, reason });
    }
  }
}
console.log(JSON.stringify({ candidateFiles: paths.length, reviewedBinaryFiles, findings }, null, 2));
if (findings.length) process.exitCode = 1;

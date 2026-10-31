// Run the real Client acceptance harness and fail on profile/cache startup errors.
// Usage: node scripts/check-client-desktop.cjs /absolute/path/to/Electron
const { spawnSync } = require('node:child_process');
const path = require('node:path');
const executable = process.argv[2];
if (!executable) {
  console.error('Usage: node scripts/check-client-desktop.cjs /path/to/Electron');
  process.exit(2);
}
const result = spawnSync(executable, [path.join(__dirname, 'smoke-client-desktop.cjs')], {
  cwd: path.resolve(__dirname, '..'), encoding: 'utf8', timeout: 100000,
});
if (result.stdout) process.stdout.write(result.stdout);
if (result.stderr) process.stderr.write(result.stderr);
if (result.error) console.error(result.error.message);
const cacheFailure = /Unable to create cache|Failed to create directory|wrong file structure on disk/.test(result.stderr || '');
const reportedSuccess = (result.stdout || '').split('\n').some(line => {
  try { return JSON.parse(line).result === 'PASS'; } catch { return false; }
});
if (result.status !== 0 || cacheFailure || !reportedSuccess) {
  console.error('FAIL: Client acceptance, profile initialization, or success evidence missing');
  process.exit(1);
}
console.log('PASS: Client acceptance with clean cache initialization');

import { readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';

export function nativeBuildFlags(version, commit, builtAt) {
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) throw new Error('Invalid native build version');
  if (!/^[0-9a-f]{7,40}$/.test(commit)) throw new Error('A verified Git commit is required');
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/.test(builtAt)) throw new Error('Invalid UTC build time');
  const prefix = 'agentcollab.local/agentcollab/internal/buildinfo';
  return `-X ${prefix}.Version=${version} -X ${prefix}.Commit=${commit} -X ${prefix}.BuiltAt=${builtAt}`;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = fileURLToPath(new URL('../', import.meta.url));
  const client = JSON.parse(readFileSync(new URL('../desktop/package.json', import.meta.url)));
  const server = JSON.parse(readFileSync(new URL('../server-desktop/package.json', import.meta.url)));
  if (client.version !== server.version) throw new Error('Client and Server versions differ');
  const commit = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim();
  console.log(nativeBuildFlags(client.version, commit, new Date().toISOString().replace(/\.\d{3}Z$/, 'Z')));
}

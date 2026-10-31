import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
export function collect(packages) {
  const entries = new Map();
  for (const [scope, lock] of Object.entries(packages)) {
    if (lock.lockfileVersion !== 3 || !lock.packages) throw new Error(`Unsupported lockfile: ${scope}`);
    for (const [location, item] of Object.entries(lock.packages)) {
      if (!location) continue;
      const name = item.name || location.split('node_modules/').at(-1);
      if (!name || !item.version || typeof item.license !== 'string' || !item.license.trim()) {
        throw new Error(`Missing dependency metadata: ${scope}/${location}`);
      }
      const key = `${name}@${item.version}`;
      const existing = entries.get(key);
      if (existing && existing.license !== item.license) throw new Error(`Conflicting license metadata: ${key}`);
      const record = existing || { name, version: item.version, license: item.license, scopes: new Set() };
      record.scopes.add(scope);
      entries.set(key, record);
    }
  }
  return [...entries.values()].sort((a, b) => `${a.name}@${a.version}`.localeCompare(`${b.name}@${b.version}`, 'en'));
}

export function render(entries) {
  const escape = (value) => String(value).replaceAll('|', '\\|').replaceAll('\n', ' ');
  return [
    '# JavaScript dependency inventory / JavaScript 依赖清单', '',
    'Generated from the web, Client and Server npm lockfiles, including build and test dependencies.',
    '由 Web、Client 和 Server 的 npm 锁文件生成，包含构建和测试依赖。', '',
    'License identifiers below are upstream package metadata, not legal verification or full license notices.',
    '以下为上游声明的许可标识，并非法律审查或完整版权声明。',
    'This inventory does not cover Go modules, bundled Chromium components, external Agent CLIs or model services.',
    '本清单不覆盖 Go 模块、内嵌 Chromium 组件、外部 Agent CLI 或模型服务。', '',
    'Regenerate: `node scripts/dependency-inventory.mjs > docs/DEPENDENCIES.md`.',
    'Verify: `node scripts/dependency-inventory.mjs --check`.', '',
    '| Package | Version | Declared license | Scope |',
    '| --- | --- | --- | --- |',
    ...entries.map((entry) => `| ${escape(entry.name)} | ${escape(entry.version)} | ${escape(entry.license)} | ${[...entry.scopes].sort().join(', ')} |`), '',
  ].join('\n');
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const locks = Object.fromEntries(['web', 'desktop', 'server-desktop'].map((scope) => [scope, JSON.parse(fs.readFileSync(path.join(root, scope, 'package-lock.json'), 'utf8'))]));
  const output = render(collect(locks));
  if (process.argv.includes('--check')) {
    const target = path.join(root, 'docs/DEPENDENCIES.md');
    if (!fs.existsSync(target) || fs.readFileSync(target, 'utf8') !== output) {
      console.error('Dependency inventory is missing or stale; regenerate docs/DEPENDENCIES.md.');
      process.exitCode = 1;
    } else console.log('Dependency inventory matches all three npm lockfiles.');
  } else process.stdout.write(output);
}

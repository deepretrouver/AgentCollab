// Generate notices from exact go.mod requirements, not personal runtime data.
import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
const root = fileURLToPath(new URL('../', import.meta.url));
const go = (...args) => execFileSync('go', args, { cwd: root, encoding: 'utf8' }).trim();
const mod = JSON.parse(go('mod', 'edit', '-json'));
const moduleCache = go('env', 'GOMODCACHE');
// Go's module cache encodes each uppercase letter as ! followed by lowercase.
const escapeModule = value => value.replace(/[A-Z]/g, letter => `!${letter.toLowerCase()}`);
if (mod.Replace?.length) throw new Error('Review replaced modules before generating public notices');
const parts = [
  '# Go third-party notices / Go 第三方许可声明',
  '',
  'Generated from exact go.mod requirements, including indirect dependencies. Original license and notice text follows; no local cache paths are included.',
  '按 go.mod 声明版本生成（含间接依赖）。以下保留上游许可原文，不包含本地缓存路径。',
  '',
  'This is not a legal opinion or an exhaustive audit of embedded code. Electron/Chromium and npm dependencies are documented separately.',
  '本清单不是法律意见，也不是嵌入代码的穷尽审计；Electron/Chromium 与 npm 依赖另列。',
];
for (const requirement of [...mod.Require].sort((a, b) => a.Path.localeCompare(b.Path, 'en'))) {
  const directory = join(moduleCache, `${escapeModule(requirement.Path)}@${escapeModule(requirement.Version)}`);
  const names = readdirSync(directory).filter(name => /^(LICENSE|COPYING|NOTICE)(\..*)?$/i.test(name)).sort();
  if (!names.some(name => /^(LICENSE|COPYING)/i.test(name))) throw new Error(`License missing: ${requirement.Path}`);
  parts.push('', `## ${requirement.Path} ${requirement.Version}`, '');
  for (const name of names) {
    const text = readFileSync(join(directory, name), 'utf8');
    if (!text.trim()) throw new Error(`Empty notice: ${requirement.Path}/${name}`);
    parts.push(`### ${name}`, '', '````text', text.trimEnd(), '````', '');
  }
}
const goRoot = go('env', 'GOROOT');
// Homebrew keeps LICENSE beside libexec rather than inside GOROOT.
const runtimeLicense = [join(goRoot, 'LICENSE'), ...(basename(goRoot) === 'libexec' ? [join(dirname(goRoot), 'LICENSE')] : [])].find(existsSync);
if (!runtimeLicense) throw new Error('Go runtime LICENSE is missing');
const goLicense = readFileSync(runtimeLicense, 'utf8');
parts.push('', '## Go runtime / Go 运行时', '', '````text', goLicense.trimEnd(), '````', '');
const output = parts.join('\n');
const destination = resolve(root, 'docs/GO-NOTICES.md');
if (process.argv.includes('--write')) writeFileSync(destination, output);
else if (process.argv.includes('--check')) {
  if (readFileSync(destination, 'utf8') !== output) throw new Error('Go notices are stale; run node scripts/go-notices.mjs --write');
  console.log('Go notices match declared modules and runtime license.');
} else process.stdout.write(output);

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
export function collectNotices(repository) {
  const sections = [];
  for (const scope of ['web', 'desktop', 'server-desktop']) {
    const lock = JSON.parse(fs.readFileSync(path.join(repository, scope, 'package-lock.json'), 'utf8'));
    if (lock.lockfileVersion !== 3) throw new Error(`Unsupported lockfile: ${scope}`);
    for (const [location, item] of Object.entries(lock.packages)) {
      if (!location || item.dev) continue;
      if (!location.startsWith('node_modules/') || location.split('/').includes('..') || item.link) throw new Error('Unsupported dependency location');
      const directory = path.join(repository, scope, location);
      const installed = JSON.parse(fs.readFileSync(path.join(directory, 'package.json'), 'utf8'));
      if (installed.version !== item.version) throw new Error(`Installed version differs from lockfile: ${scope}/${location}`);
      const names = fs.readdirSync(directory).filter(name => /^(licen[cs]e|copying|notice)(\.|$)/i.test(name)).sort();
      if (!names.some(name => /^(licen[cs]e|copying)(\.|$)/i.test(name))) throw new Error(`Missing runtime license: ${scope}/${location}`);
      const texts = names.map(name => {
        const file = path.join(directory, name);
        if (!fs.lstatSync(file).isFile()) throw new Error(`Invalid notice file: ${scope}/${location}/${name}`);
        const text = fs.readFileSync(file, 'utf8').trim();
        if (!text) throw new Error(`Empty notice file: ${scope}/${location}/${name}`);
        // Indented blocks preserve upstream text without interpreting Markdown.
        return `### ${name}\n\n${text.split('\n').map(line => line ? `    ${line}` : '').join('\n')}`;
      });
      sections.push(`## ${installed.name}@${item.version} (${scope})\n\n${texts.join('\n\n')}`);
    }
  }
  return '# JavaScript runtime notices / JavaScript 运行时版权声明\n\n'
    + 'Generated from installed production dependencies matching the three npm lockfiles.\n'
    + '根据与锁文件版本一致的已安装生产依赖生成，保留许可证及根目录 NOTICE 原文。\n\n'
    + 'Build-only dependencies are listed separately in DEPENDENCIES.md. Electron/Chromium and Go notices are distributed separately.\n'
    + '构建依赖另见 DEPENDENCIES.md；Electron/Chromium 和 Go 声明分别提供。\n\n'
    + sections.join('\n\n') + '\n';
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const output = collectNotices(root);
  const target = path.join(root, 'docs/NPM-RUNTIME-NOTICES.md');
  if (process.argv.includes('--check')) {
    if (!fs.existsSync(target) || fs.readFileSync(target, 'utf8') !== output) throw new Error('Runtime notices missing or stale');
    console.log('Runtime notices match installed production dependencies and lockfiles.');
  } else if (process.argv.includes('--write')) fs.writeFileSync(target, output);
  else process.stdout.write(output);
}

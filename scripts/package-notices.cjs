const fs = require('node:fs');
const path = require('node:path');

function copyNotices(sources, destination) {
  // Validate everything before copying; do not ship silently incomplete notices.
  for (const source of Object.values(sources)) {
    if (!fs.statSync(source).isFile() || fs.statSync(source).size === 0) throw new Error(`Missing license notice: ${source}`);
  }
  fs.mkdirSync(destination, { recursive: true });
  for (const [name, source] of Object.entries(sources)) fs.copyFileSync(source, path.join(destination, name));
}

function packageNotices(context, resourcesDirectory) {
  const repo = path.resolve(__dirname, '..');
  const project = context.packager.projectDir;
  const customDist = context.packager.config.electronDist;
  const dist = customDist ? path.resolve(project, customDist) : path.join(path.dirname(require.resolve('electron/package.json', { paths: [project] })), 'dist');
  copyNotices({
    'LICENSE-AgentCollab.txt': path.join(repo, 'LICENSE'),
    'LICENSE-Electron.txt': path.join(dist, 'LICENSE'),
    'LICENSES.chromium.html': path.join(dist, 'LICENSES.chromium.html'),
    'DEPENDENCIES.md': path.join(repo, 'docs', 'DEPENDENCIES.md'),
    'GO-NOTICES.md': path.join(repo, 'docs', 'GO-NOTICES.md'),
    'NPM-RUNTIME-NOTICES.md': path.join(repo, 'docs', 'NPM-RUNTIME-NOTICES.md'),
  }, path.join(resourcesDirectory, 'notices'));
}

module.exports = { copyNotices, packageNotices };

import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { collectNotices } from './npm-runtime-notices.mjs';

test('runtime notices preserve attribution and reject stale or incomplete installed dependencies', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'agentcollab-notices-'));
  try {
    for (const scope of ['web', 'desktop', 'server-desktop']) {
      fs.mkdirSync(path.join(root, scope));
      fs.writeFileSync(path.join(root, scope, 'package-lock.json'), JSON.stringify({lockfileVersion:3,packages:scope === 'web' ? {'node_modules/demo':{version:'1.0.0'},'node_modules/build-only':{version:'1',dev:true}} : {}}));
    }
    const dir = path.join(root, 'web/node_modules/demo');
    fs.mkdirSync(dir, {recursive:true});
    fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({name:'demo',version:'1.0.0'}));
    assert.throws(() => collectNotices(root), /Missing runtime license/);
    fs.writeFileSync(path.join(dir, 'LICENSE'), 'Copyright Example\nPermission text');
    fs.writeFileSync(path.join(dir, 'NOTICE'), 'Additional attribution');
    const output = collectNotices(root);
    assert.ok(output.includes('demo@1.0.0'));
    assert.ok(output.includes('Copyright Example\n    Permission text'));
    assert.ok(output.includes('Additional attribution'));
    assert.ok(!output.includes(root));
    assert.ok(!output.includes('build-only'));
    fs.writeFileSync(path.join(dir, 'NOTICE'), '');
    assert.throws(() => collectNotices(root), /Empty notice file/);
    fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({name:'demo',version:'2.0.0'}));
    assert.throws(() => collectNotices(root), /Installed version differs/);
  } finally { fs.rmSync(root, {recursive:true,force:true}); }
});

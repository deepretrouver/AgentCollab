import { test } from 'node:test';
import assert from 'node:assert/strict';
import { collect, render } from './dependency-inventory.mjs';

const lock = (license = 'MIT') => ({ lockfileVersion: 3, packages: { '': { name: 'app' }, 'node_modules/example': { version: '1.0.0', license } } });
test('merges shared dependencies and preserves every delivery scope', () => {
  const entries = collect({ web: lock(), desktop: lock(), 'server-desktop': lock() });
  assert.equal(entries.length, 1);
  assert.deepEqual([...entries[0].scopes], ['web', 'desktop', 'server-desktop']);
  assert.match(render(entries), /desktop, server-desktop, web/);
});
test('fails closed when license metadata is absent or inconsistent', () => {
  assert.throws(() => collect({ web: lock('') }), /Missing dependency metadata/);
  assert.throws(() => collect({ web: lock(), desktop: lock('Apache-2.0') }), /Conflicting license metadata/);
});
test('includes nested versions separately', () => {
  const input = lock();
  input.packages['node_modules/parent/node_modules/example'] = { version: '2.0.0', license: 'MIT' };
  assert.equal(collect({ web: input }).length, 2);
});

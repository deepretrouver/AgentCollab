const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const root = path.resolve(__dirname, '..');

// Inspect Make's expanded recipe without touching runtime or installed apps.
for (const target of ['server-bundle-local', 'server-bundle-packages']) {
  test(`${target} does not opportunistically include an old Windows Client`, () => {
    const recipe = execFileSync('make', ['-n', target], { cwd: root, encoding: 'utf8' });
    assert.match(recipe, /ditto .*AgentCollab\.app/);
    assert.doesNotMatch(recipe, /if test -f .*win-x64\.zip/);
    assert.doesNotMatch(recipe, /cp .*Client-windows/);
  });
}

for (const target of ['desktop-install', 'server-install']) {
  test(`${target} uses the reviewed dependency lock`, () => {
    const recipe = execFileSync('make', ['-n', target], { cwd: root, encoding: 'utf8' });
    assert.match(recipe, /npm --prefix \S+ ci\b/);
    assert.doesNotMatch(recipe, /npm --prefix \S+ install\b/);
  });
}

test('Intel Mac archives Electron Builder mac output rather than nonexistent mac-x64', () => {
  const recipe = execFileSync('make', ['-n', 'server-bundle-local', 'UNAME_M=x86_64'], { cwd: root, encoding: 'utf8' });
  assert.match(recipe, /ditto .*desktop\/release\/mac\/AgentCollab\.app/);
  assert.doesNotMatch(recipe, /desktop\/release\/mac-x64\//);
});

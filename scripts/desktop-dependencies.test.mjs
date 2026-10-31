import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

for (const scope of ['desktop', 'server-desktop']) {
  test(`${scope}: modern minimatch is patched without replacing legacy callable APIs`, () => {
    const manifest = JSON.parse(fs.readFileSync(new URL(`../${scope}/package.json`, import.meta.url)));
    const lock = JSON.parse(fs.readFileSync(new URL(`../${scope}/package-lock.json`, import.meta.url)));
    assert.equal(manifest.overrides['minimatch@>=9'], '11.0.2');
    const versions = Object.entries(lock.packages)
      .filter(([name]) => name.endsWith('/minimatch'))
      .map(([, entry]) => entry.version);
    assert.ok(versions.includes('11.0.2'));
    assert.ok(versions.includes('3.1.5'), 'preserve legacy function-style consumers');
    assert.ok(versions.includes('5.1.9'), 'preserve filelist function-style API');
    assert.ok(versions.every(version => ['3.1.5', '5.1.9', '11.0.2'].includes(version)),
      `Review newly resolved minimatch versions: ${versions.join(', ')}`);
  });
}

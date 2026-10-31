import test from 'node:test';
import assert from 'node:assert/strict';
import { nativeBuildFlags } from './native-build-info.mjs';

test('native build flags stamp the same version, commit and UTC timestamp', () => {
  const flags = nativeBuildFlags('1.3.3', 'abcdef1234567', '2026-10-31T01:00:00Z');
  assert.match(flags, /\.Version=1\.3\.3 /);
  assert.match(flags, /\.Commit=abcdef1234567 /);
  assert.match(flags, /\.BuiltAt=2026-10-31T01:00:00Z$/);
});
test('missing or injectable metadata cannot silently become a release identity', () => {
  for (const args of [['dev', 'abcdef123', '2026-10-31T01:00:00Z'], ['1.3.3', 'unknown', '2026-10-31T01:00:00Z'], ['1.3.3', 'abcdef123', 'unknown'], ['1.3.3 -X injected', 'abcdef123', '2026-10-31T01:00:00Z']]) {
    assert.throws(() => nativeBuildFlags(...args));
  }
});

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const root = path.resolve(__dirname, '..');

test('Obsidian plugin package has a valid desktop manifest and entrypoint', () => {
  const manifest = JSON.parse(fs.readFileSync(path.join(root, 'manifest.json'), 'utf8'));
  assert.equal(manifest.id, 'timekeeper-memory-galaxy');
  assert.equal(manifest.isDesktopOnly, true);
  assert.match(manifest.version, /^\d+\.\d+\.\d+$/);
  assert.ok(fs.statSync(path.join(root, 'main.js')).isFile());
});

test('plugin entrypoint keeps provider state private and exposes settings/reconnect controls', () => {
  const main = fs.readFileSync(path.join(root, 'main.js'), 'utf8');
  assert.match(main, /capability\.token/);
  assert.match(main, /endpoint\.json/);
  assert.match(main, /galaxyFolder/);
  assert.match(main, /Reconnect/);
  assert.match(main, /Memory Galaxy status/);
});

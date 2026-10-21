import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const html = await readFile(new URL('./index.html', import.meta.url), 'utf8');
const script = await readFile(new URL('./file-editor.js', import.meta.url), 'utf8');

test('File History shell exposes project explorer, branch timeline, icon legend, and revision preview', () => {
  for (const marker of [
    'id="file-history-project"',
    'id="file-history-root-path"',
    'id="file-history-set-root"',
    'id="file-history-scan"',
    'id="file-history-filter"',
    'id="file-history-files"',
    'id="file-history-change-kind"',
    'id="file-history-legend"',
    'id="file-history-timeline"',
    'id="file-history-preview"',
    'id="file-history-diff"',
    'id="file-history-restore"'
  ]) assert.ok(html.includes(marker), `dashboard HTML missing ${marker}`);

  for (const marker of [
    "'milestone'", "'bugfix'", "'code_change'", "'reversion'", "'feature'", "'release'", "'feature_eol'", "'file_added'",
    "version.restores_version_id", "change_kind: changeKindSelect.value", "restores_version_id"
  ]) assert.ok(script.includes(marker), `file editor missing branch/type behavior ${marker}`);
});

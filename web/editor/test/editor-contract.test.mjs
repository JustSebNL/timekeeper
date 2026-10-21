import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const source = await readFile(new URL('../codemirror-entry.js', import.meta.url), 'utf8');

test('editor bundle declares common language packages and lint support', () => {
  for (const marker of ['@codemirror/lint', '@codemirror/lang-python', '@codemirror/lang-go', '@codemirror/lang-rust', '@codemirror/lang-sql', '@codemirror/lang-markdown', '@codemirror/lang-yaml']) {
    assert.match(source, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')));
  }
  assert.match(source, /javascript\(\{ typescript: true/);
  assert.match(source, /linter\(/);
});

test('MergeView is exposed as a read-only comparison editor', () => {
  assert.match(source, /@codemirror\/merge/);
  assert.match(source, /new MergeView\(/);
  assert.match(source, /export function mountMerge\(/);
  assert.match(source, /EditorView\.editable\.of\(false\)/);
  assert.match(source, /EditorState\.readOnly\.of\(true\)/);
});

test('CodeMirror forwards document changes to the file-history callback', () => {
  assert.match(source, /if \(update\.docChanged\) options\.onChange\(update\.state\.doc\.toString\(\)\)/);
});

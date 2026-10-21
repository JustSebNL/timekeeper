// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const { editorReference, formatEditorReference, taskDescription } = require('../lib/editor');

test('editorReference records the workspace-relative file and selected lines', () => {
  const vscode = {
    workspace: {
      asRelativePath(uri) {
        assert.equal(uri.fsPath, 'D:\\workspace\\src\\app.js');
        return 'src/app.js';
      },
    },
  };
  const reference = editorReference(vscode, {
    document: { uri: { fsPath: 'D:\\workspace\\src\\app.js' } },
    selection: { start: { line: 9 }, end: { line: 11 } },
  });

  assert.deepEqual(reference, { path: 'src/app.js', startLine: 10, endLine: 12 });
  assert.equal(formatEditorReference(reference), 'src/app.js:L10-L12');
});

test('editorReference formats a caret or single-line selection', () => {
  const vscode = { workspace: { asRelativePath: () => 'src/app.js' } };
  const reference = editorReference(vscode, {
    document: { uri: { fsPath: 'D:\\workspace\\src\\app.js' } },
    selection: { start: { line: 4 }, end: { line: 4 } },
  });

  assert.equal(formatEditorReference(reference), 'src/app.js:L5');
  assert.equal(taskDescription(reference), 'VS Code editor selection\n\nSource: `src/app.js:L5`');
});

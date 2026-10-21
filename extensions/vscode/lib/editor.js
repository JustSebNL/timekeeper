// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

function editorReference(vscode, editor) {
  if (!editor?.document?.uri) {
    return null;
  }
  const selection = editor.selection;
  const startLine = Number.isInteger(selection?.start?.line) ? selection.start.line + 1 : 1;
  const endLine = Number.isInteger(selection?.end?.line) ? selection.end.line + 1 : startLine;
  return {
    path: vscode.workspace.asRelativePath(editor.document.uri, false),
    startLine,
    endLine,
  };
}

function formatEditorReference(reference) {
  if (!reference?.path) {
    return '';
  }
  return reference.startLine === reference.endLine
    ? `${reference.path}:L${reference.startLine}`
    : `${reference.path}:L${reference.startLine}-L${reference.endLine}`;
}

function taskDescription(reference) {
  return `VS Code editor selection\n\nSource: \`${formatEditorReference(reference)}\``;
}

module.exports = { editorReference, formatEditorReference, taskDescription };

'use strict';

const assert = require('node:assert/strict');
const { test } = require('node:test');
const { createHistoryCommands } = require('../lib/history-commands');

const file = { file_id: 'F-1', project_id: 'P-1', current_path: 'src/main.go', current_version_id: 'FV-2', file_type: 'text' };
const versions = [
  { version_id: 'FV-1', file_id: 'F-1', version_number: 1, change_kind: 'code_change' },
  { version_id: 'FV-2', file_id: 'F-1', version_number: 2, change_kind: 'code_change' },
];

function setup({ confirmation = 'Restore as new branch', summary = 'checkpoint summary', projectID = 'P-1' } = {}) {
  const calls = { messages: [], errors: [], opened: [], shown: [], refreshes: 0 };
  const client = {
    projectFiles: async id => { calls.projectID = id; return { items: [file] }; },
    fileVersions: async id => { calls.fileID = id; return { items: versions }; },
    fileVersion: async (fileID, versionID) => {
      calls.readVersion = [fileID, versionID];
      return { version: versions.find(version => version.version_id === versionID), content: 'saved contents' };
    },
    restoreFileVersion: async (...args) => { calls.restore = args; return { version: { version_id: 'FV-3' } }; },
    captureFileSnapshot: async (...args) => { calls.snapshot = args; return { version: { version_id: 'FV-4' } }; },
  };
  const vscode = {
    window: {
      showQuickPick: async (items, options) => {
        if (options.placeHolder.startsWith('Choose a tracked file')) return items[0];
        if (options.placeHolder.startsWith('Open a saved')) return items.find(item => item.version.version_id === 'FV-1');
        if (options.placeHolder.startsWith('Restore a saved')) return items.find(item => item.version.version_id === 'FV-1');
        if (options.placeHolder.startsWith('Choose a history boundary')) return items.find(item => item.phase === 'code_end');
        throw new Error(`Unexpected picker: ${options.placeHolder}`);
      },
      showInputBox: async () => summary,
      showWarningMessage: async (...args) => { calls.confirmation = args; return confirmation; },
      showInformationMessage: (...args) => calls.messages.push(args),
      showErrorMessage: (...args) => calls.errors.push(args),
      showTextDocument: async (document, options) => calls.shown.push([document, options]),
    },
    workspace: {
      openTextDocument: async uri => ({ uri }),
    },
  };
  const historyDocuments = { open: (...args) => { calls.opened.push(args); return 'timekeeper-history://file/F-1/FV-1/main.go'; } };
  const commands = createHistoryCommands({
    vscode,
    getClient: () => client,
    getSelectedProjectID: () => projectID,
    historyDocuments,
    refreshTree: () => { calls.refreshes++; },
    reportError: error => calls.errors.push([error.message]),
  });
  return { calls, client, commands, vscode };
}

test('browsing opens the selected Core version in a read-only virtual document', async () => {
  const { calls, commands } = setup();
  await commands.browseFileHistory();
  assert.equal(calls.projectID, 'P-1');
  assert.deepEqual(calls.readVersion, ['F-1', 'FV-1']);
  assert.equal(calls.opened[0][2], 'saved contents');
  assert.equal(calls.shown[0][1].preview, true);
  assert.equal(calls.errors.length, 0);
});

test('restore requires modal confirmation and sends the expected current version', async () => {
  const { calls, commands } = setup();
  await commands.restoreFileHistoryVersion();
  assert.deepEqual(calls.restore, ['F-1', 'FV-1', 'FV-2']);
  assert.equal(calls.confirmation[1].modal, true);
  assert.equal(calls.refreshes, 1);
  assert.equal(calls.errors.length, 0);
});

test('cancelling restore sends no mutation and capture sends only an explicit checkpoint', async () => {
  const cancelled = setup({ confirmation: null });
  await cancelled.commands.restoreFileHistoryVersion();
  assert.equal(cancelled.calls.restore, undefined);
  assert.equal(cancelled.calls.refreshes, 0);

  const captured = setup({ summary: '  Checkpoint from the IDE  ' });
  await captured.commands.captureFileHistorySnapshot();
  assert.deepEqual(captured.calls.snapshot, ['F-1', {
    phase: 'code_end',
    change_summary: 'Checkpoint from the IDE',
    change_kind: 'code_change',
  }]);
  assert.equal(captured.calls.refreshes, 1);
  assert.equal(captured.calls.errors.length, 0);
});

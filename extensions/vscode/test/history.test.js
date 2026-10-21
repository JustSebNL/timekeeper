// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { createHistoryDocumentProvider, historyFileChoices, historyVersionChoices } = require('../lib/history');

test('history file choices are deterministic and retain exact Core file IDs', () => {
  const choices = historyFileChoices([
    { file_id: 'F-2', current_path: 'z/main.go', file_type: 'go', current_version_id: 'FV-2' },
    { file_id: 'F-1', current_path: 'a/main.go', file_type: 'go', current_version_id: 'FV-1' },
  ]);
  assert.deepEqual(choices.map(item => [item.label, item.fileID]), [
    ['a/main.go', 'F-1'],
    ['z/main.go', 'F-2'],
  ]);
});

test('history version choices identify the current revision and reversion branch target', () => {
  const file = { file_id: 'F-1', current_version_id: 'FV-3' };
  const versions = [
    { version_id: 'FV-1', version_number: 1, change_kind: 'file_added', change_summary: 'Initial' },
    { version_id: 'FV-2', version_number: 2, change_kind: 'feature', change_summary: 'Feature' },
    { version_id: 'FV-3', version_number: 3, change_kind: 'reversion', restores_version_id: 'FV-1', change_summary: 'Restore V1' },
  ];
  const choices = historyVersionChoices(file, versions);
  assert.deepEqual(choices.map(item => item.label), [
    'V3 · current · reversion',
    'V2 · feature',
    'V1 · file_added',
  ]);
  assert.match(choices[0].description, /restores V1/);
  assert.equal(choices[0].version.version_id, 'FV-3');
});

test('history document provider exposes saved text through a read-only URI scheme', () => {
  class Emitter {
    constructor() { this.listeners = []; }
    event = listener => { this.listeners.push(listener); return { dispose() {} }; };
    fire(value) { for (const listener of this.listeners) listener(value); }
    dispose() { this.listeners = []; }
  }
  const vscode = { EventEmitter: Emitter, Uri: { parse: value => ({ toString: () => value }) } };
  const provider = createHistoryDocumentProvider(vscode);
  const uri = provider.open({ file_id: 'F-1', current_path: 'src/main.go' }, { version_id: 'FV-4', version_number: 4 }, 'package main\n');
  assert.match(uri.toString(), /^timekeeper-history:\/\//);
  assert.equal(provider.provideTextDocumentContent(uri), 'package main\n');
  provider.dispose();
});

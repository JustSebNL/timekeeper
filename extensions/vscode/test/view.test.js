// Copyright (c) 2026 Seb. All rights reserved.

'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const { buildTreeItem, TimeKeeperTreeProvider } = require('../extension');

class FakeTreeItem {
  constructor(label, collapsibleState) {
    this.label = label;
    this.collapsibleState = collapsibleState;
  }
}

test('buildTreeItem gives Sprint nodes clear state without click-to-mutate behavior', () => {
  const vscode = {
    TreeItem: FakeTreeItem,
    TreeItemCollapsibleState: { None: 0, Collapsed: 1, Expanded: 2 },
  };
  const item = buildTreeItem(vscode, {
    kind: 'sprint', id: 'SP-10000', parentID: 'T-10000', label: 'Implement extension', status: 'Active',
  });

  assert.equal(item.label, 'Implement extension');
  assert.equal(item.description, 'Active · SP-10000');
  assert.equal(item.contextValue, 'timekeeper.sprint.active');
  assert.equal(item.command, undefined);
});

test('buildTreeItem shows derived Project progress without inventing state', () => {
  const vscode = {
    TreeItem: FakeTreeItem,
    TreeItemCollapsibleState: { None: 0, Collapsed: 1, Expanded: 2 },
  };
  const item = buildTreeItem(vscode, {
    kind: 'project', id: 'P-10000', parentID: null, label: 'Time Keeper', status: 'Open', progress: 68.75,
  });

  assert.equal(item.description, 'Open · 68.8% · P-10000');
  assert.equal(item.contextValue, 'timekeeper.project');
  assert.equal(item.collapsibleState, 1);
});

test('TimeKeeperTreeProvider defers client creation until VS Code requests children', async () => {
  const vscode = {
    EventEmitter: class {
      constructor() { this.event = () => {}; }
      fire() {}
      dispose() {}
    },
    TreeItem: FakeTreeItem,
    TreeItemCollapsibleState: { None: 0, Collapsed: 1, Expanded: 2 },
  };
  let factoryCalls = 0;
  const provider = new TimeKeeperTreeProvider(vscode, () => {
    factoryCalls += 1;
    return { projects: async () => ({ items: [] }) };
  }, () => undefined);

  assert.equal(factoryCalls, 0);
  assert.deepEqual(await provider.getChildren(), []);
  assert.equal(factoryCalls, 1);
});

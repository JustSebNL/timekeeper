// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const { repositoryStatus, formatRepositoryStatus } = require('../lib/git');

test('repositoryStatus summarizes the VS Code Git repository state without remote access', () => {
  const status = repositoryStatus({
    state: {
      HEAD: { name: 'main', upstream: { name: 'origin/main' }, ahead: 2, behind: 1 },
      workingTreeChanges: [{ uri: 'file:///repo/a.js' }],
      indexChanges: [{ uri: 'file:///repo/b.js' }],
      mergeChanges: [],
      untrackedChanges: [{ uri: 'file:///repo/c.js' }],
    },
  });

  assert.deepEqual(status, {
    branch: 'main',
    upstream: 'origin/main',
    ahead: 2,
    behind: 1,
    workingTree: 1,
    index: 1,
    merge: 0,
    untracked: 1,
  });
  assert.equal(formatRepositoryStatus(status), 'branch main · tracking origin/main · 2 ahead, 1 behind · 3 changes (1 unstaged, 1 staged, 0 merge, 1 untracked)');
});

test('repositoryStatus handles clean repositories and unavailable Git state', () => {
  assert.equal(repositoryStatus(undefined), null);
  assert.equal(repositoryStatus({}), null);
  assert.equal(formatRepositoryStatus(repositoryStatus({ state: { HEAD: { name: 'main' } } })), 'branch main · clean working tree');
});

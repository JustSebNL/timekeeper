// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

function changeCount(value) {
  return Array.isArray(value) ? value.length : 0;
}

function repositoryStatus(repository) {
  const state = repository?.state;
  if (!state) {
    return null;
  }
  const head = state.HEAD || {};
  return {
    branch: head.name || 'detached HEAD',
    upstream: head.upstream?.name || '',
    ahead: Number.isInteger(head.ahead) ? head.ahead : 0,
    behind: Number.isInteger(head.behind) ? head.behind : 0,
    workingTree: changeCount(state.workingTreeChanges),
    index: changeCount(state.indexChanges),
    merge: changeCount(state.mergeChanges),
    untracked: changeCount(state.untrackedChanges),
  };
}

function formatRepositoryStatus(status) {
  if (!status) {
    return '';
  }
  const parts = [`branch ${status.branch}`];
  if (status.upstream) {
    parts.push(`tracking ${status.upstream}`);
  }
  if (status.ahead || status.behind) {
    parts.push(`${status.ahead} ahead, ${status.behind} behind`);
  }
  const changed = status.workingTree + status.index + status.merge + status.untracked;
  if (changed === 0) {
    parts.push('clean working tree');
  } else {
    parts.push(`${changed} change${changed === 1 ? '' : 's'} (${status.workingTree} unstaged, ${status.index} staged, ${status.merge} merge, ${status.untracked} untracked)`);
  }
  return parts.join(' · ');
}

module.exports = { repositoryStatus, formatRepositoryStatus };

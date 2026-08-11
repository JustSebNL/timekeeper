// Copyright (c) 2026 Seb. All rights reserved.

'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const { executionNodes } = require('../lib/tree');

test('executionNodes preserves nested Categories and direct Sprint ownership', () => {
  const nodes = executionNodes({
    project: { project_id: 'P-10000', name: 'Workspace', calculated_completion_pct: 25 },
    categories: [{
      category: { category_id: 'C-10001', name: 'Platform', status: 'Open' },
      categories: [{
        category: { category_id: 'C-10002', name: 'Storage', status: 'Open' },
        categories: [],
        tasks: [{ task: { task_id: 'T-10003', name: 'Backup', status: 'Open' }, sprints: [], subtasks: [] }],
      }],
      tasks: [{
        task: { task_id: 'T-10004', name: 'Editor', status: 'Open' },
        sprints: [{ sprint_id: 'SP-10005', name: 'Implement', status: 'Active' }],
        subtasks: [{ subtask: { subtask_id: 'ST-10006', name: 'Smoke', status: 'Open' }, sprints: [{ sprint_id: 'SP-10007', name: 'Verify', status: 'Open' }] }],
      }],
    }],
  });

  assert.deepEqual(nodes.map(node => [node.kind, node.id, node.parentID]), [
    ['project', 'P-10000', null],
    ['category', 'C-10001', 'P-10000'],
    ['category', 'C-10002', 'C-10001'],
    ['task', 'T-10003', 'C-10002'],
    ['task', 'T-10004', 'C-10001'],
    ['sprint', 'SP-10005', 'T-10004'],
    ['subtask', 'ST-10006', 'T-10004'],
    ['sprint', 'SP-10007', 'ST-10006'],
  ]);
  assert.equal(nodes.find(node => node.id === 'SP-10005').status, 'Active');
});

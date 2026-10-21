// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const { summarizeProject, renderProjectContext } = require('../lib/context');

test('summarizeProject reports authority fields and execution counts', () => {
  const summary = summarizeProject({
    project: {
      project_id: 'P-10000',
      project_name: 'Workspace',
      project_goal: 'Ship safely',
      project_description: 'Local project',
      status: 'Open',
      calculated_completion_pct: 42.5,
    },
    categories: [{
      category: { category_id: 'C-10001', name: 'Platform', status: 'Open' },
      categories: [],
      tasks: [{
        task: { task_id: 'T-10002', name: 'Editor', status: 'Open' },
        sprints: [
          { sprint_id: 'SP-10003', name: 'Build', status: 'Active' },
          { sprint_id: 'SP-10004', name: 'Verify', status: 'Completed' },
        ],
        subtasks: [],
      }],
    }],
  });

  assert.deepEqual(summary, {
    projectID: 'P-10000',
    name: 'Workspace',
    status: 'Open',
    goal: 'Ship safely',
    description: 'Local project',
    progress: 42.5,
    categories: 1,
    tasks: 1,
    subtasks: 0,
    sprints: 2,
    sprintStatuses: { Active: 1, Completed: 1 },
  });
});

test('renderProjectContext escapes project data and lists instruction paths only', () => {
  const html = renderProjectContext({
    projectID: 'P-10000',
    name: 'Workspace <script>alert(1)</script>',
    status: 'Open',
    goal: 'Keep "quotes" safe',
    description: 'Local project',
    progress: null,
    categories: 0,
    tasks: 0,
    subtasks: 0,
    sprints: 0,
    sprintStatuses: {},
  }, ['AGENTS.md', 'harness/skills/timeboxing/SKILL.md']);

  assert.match(html, /Content-Security-Policy/);
  assert.match(html, /Workspace &lt;script&gt;alert\(1\)&lt;\/script&gt;/);
  assert.match(html, /Keep &quot;quotes&quot; safe/);
  assert.match(html, /harness\/skills\/timeboxing\/SKILL\.md/);
  assert.doesNotMatch(html, /<script>alert/);
});

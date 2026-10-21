// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const { TimeKeeperClient, normalizeServerURL } = require('../lib/client');

test('normalizeServerURL accepts only numeric loopback HTTP origins', () => {
  assert.equal(normalizeServerURL('http://127.0.0.1:1618'), 'http://127.0.0.1:1618');
  assert.equal(normalizeServerURL('http://[::1]:1618/'), 'http://[::1]:1618');
  for (const invalid of [
    'http://localhost:1618',
    'http://192.168.1.10:1618',
    'https://127.0.0.1:1618',
    'http://127.0.0.1:1618/api',
    'http://127.0.0.1:1618?token=bad',
  ]) {
    assert.throws(() => normalizeServerURL(invalid), /numeric loopback HTTP origin/);
  }
});

test('TimeKeeperClient reads the project hierarchy through the public API', async () => {
  const requests = [];
  const client = new TimeKeeperClient('http://127.0.0.1:1618', async url => {
    requests.push(url);
    return {
      ok: true,
      status: 200,
      async json() {
        return { project: { project_id: 'P-10000', name: 'Workspace' }, categories: [] };
      },
    };
  });

  const tree = await client.projectTree('P-10000');
  assert.equal(tree.project.project_id, 'P-10000');
  assert.deepEqual(requests, ['http://127.0.0.1:1618/api/v1/projects/P-10000/execution-tree']);
});

test('TimeKeeperClient creates a Task through the public project API', async () => {
  const request = [];
  const client = new TimeKeeperClient('http://127.0.0.1:1618', async (url, options) => {
    request.push({ url, options });
    return { ok: true, status: 201, async json() { return { task_id: 'T-10000', name: 'Review selection' }; } };
  });

  const task = await client.createTask('P-10000', {
    category_id: 'C-10000',
    name: 'Review selection',
    description: 'VS Code editor selection\n\nSource: `src/app.js:L5`',
    goal: 'Review the selected code at src/app.js:L5.',
    priority: 'Normal',
    estimated_duration_seconds: 1800,
  });

  assert.equal(task.task_id, 'T-10000');
  assert.equal(request[0].url, 'http://127.0.0.1:1618/api/v1/projects/P-10000/tasks');
  assert.equal(request[0].options.method, 'POST');
  assert.deepEqual(JSON.parse(request[0].options.body), {
    category_id: 'C-10000',
    name: 'Review selection',
    description: 'VS Code editor selection\n\nSource: `src/app.js:L5`',
    goal: 'Review the selected code at src/app.js:L5.',
    priority: 'Normal',
    estimated_duration_seconds: 1800,
  });
});

test('TimeKeeperClient reads tracked files and immutable version contents through Core', async () => {
  const requests = [];
  const client = new TimeKeeperClient('http://127.0.0.1:1618', async (url, options) => {
    requests.push({ url, options });
    const payload = url.endsWith('/file-root')
      ? { project_id: 'P-7', root_path: 'D:/workspace' }
      : url.endsWith('/versions/FV-1')
        ? { version: { version_id: 'FV-1', file_id: 'F-1', version_number: 1 }, content: 'package main' }
        : url.endsWith('/versions')
          ? { items: [{ version_id: 'FV-1', file_id: 'F-1', version_number: 1 }] }
          : { items: [{ file_id: 'F-1', project_id: 'P-7', current_path: 'src/main.go' }] };
    return { ok: true, status: 200, async json() { return payload; } };
  });

  const root = await client.projectFileRoot('P-7');
  const files = await client.projectFiles('P-7');
  const versions = await client.fileVersions('F-1');
  const saved = await client.fileVersion('F-1', 'FV-1');
  assert.equal(root.root_path, 'D:/workspace');
  assert.equal(files.items[0].file_id, 'F-1');
  assert.equal(versions.items[0].version_id, 'FV-1');
  assert.equal(saved.content, 'package main');
  assert.deepEqual(requests.map(request => request.url), [
    'http://127.0.0.1:1618/api/v1/projects/P-7/file-root',
    'http://127.0.0.1:1618/api/v1/projects/P-7/files',
    'http://127.0.0.1:1618/api/v1/files/F-1/versions',
    'http://127.0.0.1:1618/api/v1/files/F-1/versions/FV-1',
  ]);
});

test('TimeKeeperClient compares and restores file versions with explicit optimistic concurrency', async () => {
  const requests = [];
  const client = new TimeKeeperClient('http://127.0.0.1:1618', async (url, options) => {
    requests.push({ url, options });
    return {
      ok: true,
      status: 200,
      async json() {
        return url.includes('/compare?')
          ? { from: { version_id: 'FV-1' }, to: { version_id: 'FV-2' }, hunks: [] }
          : { file: { file_id: 'F-1', current_version_id: 'FV-3' }, version: { version_id: 'FV-3', restores_version_id: 'FV-1' } };
      },
    };
  });

  const comparison = await client.compareFileVersions('F-1', 'FV-1', 'FV-2');
  const restored = await client.restoreFileVersion('F-1', 'FV-1', 'FV-2');
  assert.deepEqual(comparison.from, { version_id: 'FV-1' });
  assert.equal(restored.version.version_id, 'FV-3');
  assert.deepEqual(JSON.parse(requests[1].options.body), { version_id: 'FV-1', expected_current_version_id: 'FV-2' });
  assert.equal(requests[1].options.method, 'POST');
});

test('TimeKeeperClient captures a file history boundary only through the explicit snapshot route', async () => {
  const request = [];
  const client = new TimeKeeperClient('http://127.0.0.1:1618', async (url, options) => {
    request.push({ url, options });
    return { ok: true, status: 201, async json() { return { version: { version_id: 'FV-5', file_id: 'F-1', version_number: 5 } }; } };
  });
  const input = { phase: 'code_end', change_summary: 'Saved the VS Code edit', change_kind: 'code_change' };

  const result = await client.captureFileSnapshot('F-1', input);
  assert.equal(result.version.version_id, 'FV-5');
  assert.equal(request[0].url, 'http://127.0.0.1:1618/api/v1/files/F-1/snapshots');
  assert.equal(request[0].options.method, 'POST');
  assert.deepEqual(JSON.parse(request[0].options.body), input);
  await assert.rejects(client.captureFileSnapshot('F-1', { ...input, phase: 'background' }), /code_start or code_end/);
});

test('TimeKeeperClient sends an explicit Sprint lifecycle action as JSON', async () => {
  const request = [];
  const client = new TimeKeeperClient('http://127.0.0.1:1618', async (url, options) => {
    request.push({ url, options });
    return { ok: true, status: 200, async json() { return { sprint_id: 'SP-10000', status: 'Active' }; } };
  });

  const sprint = await client.sprintAction('SP-10000', 'start');
  assert.equal(sprint.status, 'Active');
  assert.equal(request[0].url, 'http://127.0.0.1:1618/api/v1/sprints/SP-10000/start');
  assert.equal(request[0].options.method, 'POST');
  assert.equal(request[0].options.headers['Content-Type'], 'application/json');
});

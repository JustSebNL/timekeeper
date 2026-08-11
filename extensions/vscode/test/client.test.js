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

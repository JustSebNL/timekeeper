const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { GalaxyRepository } = require('../src/repository');
const { GalaxyApiServer, PROTOCOL_VERSION } = require('../src/api/server');

test('repository creates an idempotent bounded Galaxy item', async () => {
  const vault = await fs.mkdtemp(path.join(os.tmpdir(), 'galaxy-vault-'));
  const repository = new GalaxyRepository(vault);
  const candidate = { title: 'SQLite context', content: 'Keep durable context compact.', type: 'pattern', visibility: 'galaxy', origin: { project_id: 'P-1' } };
  const first = await repository.create(candidate, 'trail:TR-1:promotion:1');
  const retry = await repository.create(candidate, 'trail:TR-1:promotion:1');
  assert.equal(first.duplicate, false);
  assert.equal(retry.duplicate, true);
  assert.equal(retry.item.id, first.item.id);
  assert.equal((await repository.get(first.item.id)).content, candidate.content);
  assert.equal((await repository.search('durable context', 8))[0].id, first.item.id);
});

test('repository rejects unknown frontmatter and hashes manual edits', async () => {
  const vault = await fs.mkdtemp(path.join(os.tmpdir(), 'galaxy-schema-'));
  const repository = new GalaxyRepository(vault);
  const first = await repository.create({ title: 'Manual item', content: 'Original body.', type: 'knowledge', visibility: 'galaxy' }, 'promotion-schema');
  const file = (await repository.files()).find(candidate => candidate.includes(first.item.id));
  let markdown = await fs.readFile(file, 'utf8');
  markdown = markdown.replace('Original body.', 'Edited body.');
  await fs.writeFile(file, markdown);
  const edited = await repository.get(first.item.id);
  assert.equal(edited.content, 'Edited body.');
  assert.notEqual(edited.content_hash, first.item.content_hash);
  const invalid = markdown.replace('\n---\n', '\nunknown_field: should-fail\n---\n');
  await fs.writeFile(file, invalid);
  await assert.rejects(() => repository.get(first.item.id), /not found/);
});

test('API requires capability for non-health routes and writes a descriptor', async () => {
  const vault = await fs.mkdtemp(path.join(os.tmpdir(), 'galaxy-api-'));
  const state = await fs.mkdtemp(path.join(os.tmpdir(), 'galaxy-state-'));
  const server = new GalaxyApiServer(new GalaxyRepository(vault), {
    descriptorPath: path.join(state, 'endpoint.json'),
    tokenPath: path.join(state, 'capability.token'),
    port: 0
  });
  const descriptor = await server.start();
  try {
    assert.equal(descriptor.protocol_version, PROTOCOL_VERSION);
    assert.equal(descriptor.provider, 'obsidian');
    const health = await fetch(descriptor.base_url + '/health');
    assert.equal(health.status, 200);
    assert.equal((await health.json()).status, 'ok');

    const unauthorized = await fetch(descriptor.base_url + '/api/v1/galaxy/status');
    assert.equal(unauthorized.status, 401);

    const token = (await fs.readFile(path.join(state, 'capability.token'), 'utf8')).trim();
    const status = await fetch(descriptor.base_url + '/api/v1/galaxy/status', { headers: { Authorization: `Bearer ${token}` } });
    assert.equal(status.status, 200);
    assert.equal((await status.json()).protocol_version, PROTOCOL_VERSION);

    const create = await fetch(descriptor.base_url + '/api/v1/galaxy/items', {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: 'A decision', content: 'A small item.', type: 'decision', visibility: 'private', promotion_key: 'promotion-1' })
    });
    assert.equal(create.status, 201);
    const item = await create.json();
    const duplicate = await fetch(descriptor.base_url + '/api/v1/galaxy/items', {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: 'A decision', content: 'A small item.', type: 'decision', visibility: 'private', promotion_key: 'promotion-1' })
    });
    assert.equal(duplicate.status, 200);
    assert.equal((await duplicate.json()).id, item.id);
  } finally {
    await server.stop();
  }
});

test('API rejects cross-origin and unknown-field requests', async () => {
  const vault = await fs.mkdtemp(path.join(os.tmpdir(), 'galaxy-api-origin-'));
  const state = await fs.mkdtemp(path.join(os.tmpdir(), 'galaxy-state-origin-'));
  const server = new GalaxyApiServer(new GalaxyRepository(vault), { descriptorPath: path.join(state, 'endpoint.json'), tokenPath: path.join(state, 'token'), port: 0 });
  const descriptor = await server.start();
  try {
    const token = (await fs.readFile(path.join(state, 'token'), 'utf8')).trim();
    const response = await fetch(descriptor.base_url + '/api/v1/galaxy/search', {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, Origin: 'https://evil.invalid', 'Content-Type': 'application/json' },
      body: JSON.stringify({ query: 'x', limit: 1 })
    });
    assert.equal(response.status, 403);
    const unknown = await fetch(descriptor.base_url + '/api/v1/galaxy/search', {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ query: 'x', unexpected: true })
    });
    assert.equal(unknown.status, 400);
  } finally {
    await server.stop();
  }
});

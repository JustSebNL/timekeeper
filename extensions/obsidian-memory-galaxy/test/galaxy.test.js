const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { GalaxyRepository, parseMarkdown } = require('../src/repository');
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

test('repository serializes concurrent retries by promotion key', async () => {
  const vault = await fs.mkdtemp(path.join(os.tmpdir(), 'galaxy-concurrent-'));
  const repository = new GalaxyRepository(vault);
  const candidate = { title: 'Concurrent promotion', content: 'One durable note.', type: 'knowledge', visibility: 'galaxy' };
  const results = await Promise.all(Array.from({ length: 12 }, () => repository.create(candidate, 'promotion-concurrent')));
  assert.equal(new Set(results.map(result => result.item.id)).size, 1);
  assert.equal(results.filter(result => !result.duplicate).length, 1);
  assert.equal((await repository.files()).length, 1);
});

test('repository quarantines duplicate Galaxy IDs and keeps other retrieval available', async () => {
  const vault = await fs.mkdtemp(path.join(os.tmpdir(), 'galaxy-duplicate-'));
  const repository = new GalaxyRepository(vault);
  const duplicate = await repository.create({ title: 'Duplicate note', content: 'Primary copy.', type: 'knowledge', visibility: 'galaxy' }, 'promotion-duplicate');
  const unrelated = await repository.create({ title: 'Unrelated note', content: 'Separate searchable item.', type: 'knowledge', visibility: 'galaxy' }, 'promotion-unrelated');
  const sourcePath = (await repository.files()).find(filePath => filePath.includes(duplicate.item.id));
  const source = await fs.readFile(sourcePath, 'utf8');
  await fs.writeFile(path.join(repository.root, 'Knowledge', 'conflicting-copy.md'), source.replace('Primary copy.', 'Conflicting copy.'));

  const isDuplicateError = error => error.code === 'duplicate_id' && error.status === 409;
  await assert.rejects(() => repository.get(duplicate.item.id), isDuplicateError);
  const quarantine = path.join(repository.root, 'Quarantine');
  const quarantinedFiles = await fs.readdir(quarantine);
  assert.equal(quarantinedFiles.length, 2);
  const quarantinedContent = await Promise.all(quarantinedFiles.map(file => fs.readFile(path.join(quarantine, file), 'utf8')));
  assert.ok(quarantinedContent.some(content => content.includes('Primary copy.')));
  assert.ok(quarantinedContent.some(content => content.includes('Conflicting copy.')));
  assert.equal((await repository.get(unrelated.item.id)).id, unrelated.item.id);
  assert.equal((await repository.search('separate searchable', 8))[0].id, unrelated.item.id);
  await assert.rejects(() => repository.create({ title: 'Duplicate note', content: 'Primary copy.', type: 'knowledge', visibility: 'galaxy' }, 'promotion-duplicate'), isDuplicateError);
});

test('repository preserves topic and tag frontmatter through edits and search', async () => {
  const vault = await fs.mkdtemp(path.join(os.tmpdir(), 'galaxy-metadata-'));
  const repository = new GalaxyRepository(vault);
  const created = await repository.create({
    title: 'Markdown semantics',
    content: 'Memory Galaxy notes stay searchable.',
    type: 'knowledge',
    visibility: 'galaxy',
    topics: ['security', 'retrieval'],
    tags: ['obsidian', 'markdown'],
    origin: { project_id: 'P-1' }
  }, 'promotion-metadata');
  assert.deepEqual(created.item.topics, ['retrieval', 'security']);
  assert.deepEqual(created.item.tags, ['markdown', 'obsidian']);
  assert.equal(created.item.schema_version, 1);
  assert.equal(created.item.sync_status, 'created');
  assert.equal(created.item.sync_revision, 1);
  assert.match(created.item.created_at, /^\d{4}-\d{2}-\d{2}T.*Z$/);
  assert.match(created.item.updated_at, /^\d{4}-\d{2}-\d{2}T.*Z$/);
  const topicHits = await repository.search('security', 8);
  assert.equal(topicHits[0].id, created.item.id);
  assert.deepEqual(topicHits[0].topics, ['retrieval', 'security']);
  assert.deepEqual(topicHits[0].tags, ['markdown', 'obsidian']);
  assert.equal((await repository.search('obsidian', 8))[0].id, created.item.id);

  const file = (await repository.files()).find(candidate => candidate.includes(created.item.id));
  const markdown = await fs.readFile(file, 'utf8');
  const inlineTopics = markdown.replace('topics:\n  - "retrieval"\n  - "security"', 'topics: [security, retrieval]');
  await fs.writeFile(file, inlineTopics);
  const reordered = await repository.get(created.item.id);
  assert.equal(reordered.schema_version, 1);
  assert.equal(reordered.sync_status, 'created');
  assert.equal(reordered.sync_revision, 1);
  assert.deepEqual(reordered.topics, ['retrieval', 'security']);
  assert.equal(reordered.content_hash, created.item.content_hash);

  const edited = inlineTopics.replace('  - "markdown"', '  - "memory"');
  await fs.writeFile(file, edited);
  const updated = await repository.get(created.item.id);
  assert.deepEqual(updated.tags, ['memory', 'obsidian']);
  assert.notEqual(updated.content_hash, created.item.content_hash);
  assert.equal((await repository.search('memory', 8))[0].id, created.item.id);
});

test('repository rejects unknown frontmatter and hashes manual edits', async () => {
  const vault = await fs.mkdtemp(path.join(os.tmpdir(), 'galaxy-schema-'));
  const repository = new GalaxyRepository(vault);
  const first = await repository.create({ title: 'Manual item', content: 'Original body.', type: 'knowledge', visibility: 'galaxy' }, 'promotion-schema');
  const file = (await repository.files()).find(candidate => candidate.includes(first.item.id));
  let markdown = await fs.readFile(file, 'utf8');
  const legacy = markdown.replace('topics: []\n', '').replace('tags: []\n', '');
  assert.deepEqual(parseMarkdown(legacy).topics, []);
  assert.deepEqual(parseMarkdown(legacy).tags, []);
  assert.throws(() => parseMarkdown(markdown.replace(/created_at: .*\n/, 'created_at: yesterday\n')), /UTC RFC 3339/);
  assert.throws(() => parseMarkdown(markdown.replace('sync_revision: 1', 'sync_revision: 0')), /positive integer/);
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

    const createRequest = () => fetch(descriptor.base_url + '/api/v1/galaxy/items', {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: 'A decision', content: 'A small item.', type: 'decision', visibility: 'private', promotion_key: 'promotion-1', topics: ['api'], tags: ['approval'] })
    });
    const creates = await Promise.all([createRequest(), createRequest()]);
    assert.deepEqual(creates.map(response => response.status).sort(), [200, 201]);
    const items = await Promise.all(creates.map(response => response.json()));
    assert.equal(items[0].id, items[1].id);
    const item = items[0];
    assert.deepEqual(item.topics, ['api']);
    assert.deepEqual(item.tags, ['approval']);

    const search = await fetch(descriptor.base_url + '/api/v1/galaxy/search', {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ query: 'approval', limit: 8 })
    });
    assert.equal(search.status, 200);
    const searchPayload = await search.json();
    assert.equal(searchPayload.results[0].id, item.id);
    assert.deepEqual(searchPayload.results[0].topics, ['api']);
    assert.deepEqual(searchPayload.results[0].tags, ['approval']);

    const duplicateItem = await server.repository.create({ title: 'Duplicate API item', content: 'Original API copy.', type: 'reference', visibility: 'private' }, 'promotion-api-duplicate');
    const sourceFile = (await server.repository.files()).find(filePath => filePath.includes(duplicateItem.item.id));
    const duplicateFile = path.join(server.repository.root, 'Knowledge', 'duplicate-api-copy.md');
    await fs.writeFile(duplicateFile, (await fs.readFile(sourceFile, 'utf8')).replace('Original API copy.', 'Conflicting API copy.'));
    const conflict = await fetch(descriptor.base_url + '/api/v1/galaxy/items/' + duplicateItem.item.id, { headers: { Authorization: `Bearer ${token}` } });
    assert.equal(conflict.status, 409);
    assert.equal((await conflict.json()).error.code, 'duplicate_id');
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

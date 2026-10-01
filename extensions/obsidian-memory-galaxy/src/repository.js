// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

const crypto = require('node:crypto');
const fs = require('node:fs/promises');
const path = require('node:path');

const ITEM_TYPES = new Set([
  'knowledge', 'decision', 'pattern', 'failure', 'insight', 'procedure',
  'reference', 'rag-source', 'skill', 'harness', 'research'
]);
const VISIBILITIES = new Set(['private', 'galaxy']);
const FRONTMATTER_FIELDS = new Set(['galaxy_id', 'schema_version', 'title', 'type', 'created_at', 'updated_at', 'review_status', 'sync_status', 'sync_revision', 'content_hash', 'topics', 'tags', 'visibility', 'promotion_key', 'origin']);
const ORIGIN_FIELDS = new Set(['system', 'project_id', 'task_id', 'trail_id']);
const MAX_FILE_BYTES = 128 * 1024;
const MAX_SCAN_FILES = 1000;
const MAX_METADATA_VALUES = 64;
const MAX_METADATA_VALUE_LENGTH = 128;

class GalaxyError extends Error {
  constructor(code, message, retryable = false, status = 400) {
    super(message);
    this.name = 'GalaxyError';
    this.code = code;
    this.retryable = retryable;
    this.status = status;
  }
}

function bounded(value, max, name, required = false) {
  if (typeof value !== 'string') throw new GalaxyError('invalid_item', `${name} must be text`);
  const text = value.trim();
  if (required && !text) throw new GalaxyError('invalid_item', `${name} is required`);
  if (text.length > max) throw new GalaxyError('invalid_item', `${name} is too long`);
  return text;
}

function normalizeStringList(value, name, code = 'invalid_item') {
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value)) throw new GalaxyError(code, `${name} must be a list`);
  if (value.length > MAX_METADATA_VALUES) throw new GalaxyError(code, `${name} has too many values`);
  const values = value.map(item => {
    if (typeof item !== 'string') throw new GalaxyError(code, `${name} values must be text`);
    const text = item.trim();
    if (!text) throw new GalaxyError(code, `${name} values must not be empty`);
    if (text.length > MAX_METADATA_VALUE_LENGTH) throw new GalaxyError(code, `${name} value is too long`);
    return text;
  });
  return [...new Set(values)].sort((left, right) => left < right ? -1 : left > right ? 1 : 0);
}

function normalizeOrigin(value, code = 'invalid_item') {
  const origin = value === undefined || value === null ? {} : value;
  if (typeof origin !== 'object' || Array.isArray(origin)) throw new GalaxyError(code, 'origin must be an object');
  for (const key of Object.keys(origin)) if (!ORIGIN_FIELDS.has(key)) throw new GalaxyError(code, 'unknown origin field');
  const text = (key, fallback = '') => {
    const raw = origin[key] === undefined || origin[key] === null ? fallback : origin[key];
    if (typeof raw !== 'string') throw new GalaxyError(code, `origin.${key} must be text`);
    const value = raw.trim();
    if (value.length > MAX_METADATA_VALUE_LENGTH) throw new GalaxyError(code, `origin.${key} is too long`);
    return value;
  };
  return {
    system: text('system', 'timekeeper') || 'timekeeper',
    project_id: text('project_id'),
    task_id: text('task_id'),
    trail_id: text('trail_id')
  };
}

function normalizeTimestamp(value, name) {
  if (typeof value !== 'string') throw new GalaxyError('invalid_markdown', `${name} must be a UTC RFC 3339 timestamp`);
  const text = value.trim();
  const match = text.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.\d{1,9})?(?:Z|\+00:00)$/);
  const date = new Date(text);
  if (!match || Number.isNaN(date.getTime()) || date.toISOString().slice(0, 19) !== match[1]) {
    throw new GalaxyError('invalid_markdown', `${name} must be a UTC RFC 3339 timestamp`);
  }
  return date.toISOString();
}

function normalizeSyncRevision(value) {
  const text = String(value).trim();
  if (!/^[1-9]\d*$/.test(text)) throw new GalaxyError('invalid_markdown', 'sync_revision must be a positive integer');
  const revision = Number(text);
  if (!Number.isSafeInteger(revision)) throw new GalaxyError('invalid_markdown', 'sync_revision must be a positive integer');
  return revision;
}

function validateCandidate(candidate, idempotencyKey) {
  if (!candidate || typeof candidate !== 'object') throw new GalaxyError('invalid_item', 'item must be an object');
  const title = bounded(candidate.title, 240, 'title', true);
  const content = bounded(candidate.content, 10000, 'content', true);
  const type = bounded(candidate.type, 32, 'type', true);
  const visibility = bounded(candidate.visibility, 16, 'visibility', true);
  const promotionKey = bounded(idempotencyKey || candidate.idempotency_key, 512, 'idempotency_key', true);
  if (!ITEM_TYPES.has(type)) throw new GalaxyError('invalid_item', `unsupported item type ${type}`);
  if (!VISIBILITIES.has(visibility)) throw new GalaxyError('invalid_item', `unsupported visibility ${visibility}`);
  const topics = normalizeStringList(candidate.topics, 'topics');
  const tags = normalizeStringList(candidate.tags, 'tags');
  const origin = normalizeOrigin(candidate.origin);
  return { title, content, type, visibility, promotionKey, topics, tags, origin };
}

function safeFileName(title, id) {
  const stem = title.normalize('NFKC').replace(/[^a-zA-Z0-9 _-]/g, '').trim().replace(/\s+/g, '-').slice(0, 80) || 'untitled';
  return `${stem}--${id}.md`;
}

function galaxyID() {
  const timestamp = Date.now().toString(36).toUpperCase();
  return `MG-${timestamp}-${crypto.randomBytes(8).toString('hex').toUpperCase()}`;
}

function contentHash(value, legacyContent) {
  const item = typeof value === 'string' ? { title: value, content: legacyContent } : value;
  const canonical = {
    schema_version: 1,
    title: item.title,
    type: item.type || '',
    created_at: item.created_at || '',
    updated_at: item.updated_at || '',
    review_status: item.review_status || 'inbox',
    visibility: item.visibility || 'private',
    topics: normalizeStringList(item.topics, 'topics'),
    tags: normalizeStringList(item.tags, 'tags'),
    origin: normalizeOrigin(item.origin),
    content: item.content
  };
  return `sha256:${crypto.createHash('sha256').update(JSON.stringify(canonical), 'utf8').digest('hex')}`;
}

function yamlQuote(value) {
  return JSON.stringify(String(value));
}

function markdownList(name, values) {
  if (values.length === 0) return [`${name}: []`];
  return [`${name}:`, ...values.map(value => `  - ${yamlQuote(value)}`)];
}

function markdownFor(item, promotionKey) {
  const origin = normalizeOrigin(item.origin);
  const lines = [
    '---',
    `galaxy_id: ${yamlQuote(item.id)}`,
    `schema_version: ${item.schema_version || 1}`,
    `title: ${yamlQuote(item.title)}`,
    `type: ${item.type}`,
    `created_at: ${item.created_at}`,
    `updated_at: ${item.updated_at}`,
    `review_status: ${item.review_status}`,
    `sync_status: ${item.sync_status || 'created'}`,
    `sync_revision: ${item.sync_revision || 1}`,
    `content_hash: ${item.content_hash}`,
    `visibility: ${item.visibility}`,
    `promotion_key: ${yamlQuote(promotionKey)}`,
    'origin:',
    `  system: ${yamlQuote(origin.system)}`,
    `  project_id: ${yamlQuote(origin.project_id)}`,
    `  task_id: ${yamlQuote(origin.task_id)}`,
    `  trail_id: ${yamlQuote(origin.trail_id)}`,
    ...markdownList('topics', item.topics || []),
    ...markdownList('tags', item.tags || []),
    '---',
    '',
    `# ${item.title}`,
    '',
    item.content,
    ''
  ];
  return lines.join('\n');
}

function parseScalar(value) {
  const text = value.trim();
  if (text.startsWith('"') && text.endsWith('"')) {
    try { return JSON.parse(text); } catch { throw new GalaxyError('invalid_markdown', 'quoted frontmatter value is invalid'); }
  }
  if (text.startsWith("'") && text.endsWith("'")) return text.slice(1, -1).replace(/''/g, "'");
  if (text.startsWith('"') || text.endsWith('"') || text.startsWith("'") || text.endsWith("'")) {
    throw new GalaxyError('invalid_markdown', 'quoted frontmatter value is not closed');
  }
  return text;
}

function parseInlineList(value, name) {
  const text = value.trim();
  if (text === '[]') return [];
  if (!text.startsWith('[') || !text.endsWith(']')) throw new GalaxyError('invalid_markdown', `${name} must be a string list`);
  const body = text.slice(1, -1).trim();
  if (!body) return [];
  const items = [];
  let start = 0;
  let quote = '';
  let escaped = false;
  for (let i = 0; i < body.length; i++) {
    const character = body[i];
    if (quote === '"') {
      if (escaped) { escaped = false; continue; }
      if (character === '\\') { escaped = true; continue; }
      if (character === '"') quote = '';
      continue;
    }
    if (quote === "'") {
      if (character === "'" && body[i + 1] === "'") { i++; continue; }
      if (character === "'") quote = '';
      continue;
    }
    if (character === '"' || character === "'") { quote = character; continue; }
    if (character === ',') {
      items.push(parseScalar(body.slice(start, i)));
      start = i + 1;
    }
  }
  if (quote) throw new GalaxyError('invalid_markdown', `${name} contains an unterminated quoted value`);
  items.push(parseScalar(body.slice(start)));
  return normalizeStringList(items, name, 'invalid_markdown');
}

function parseMarkdown(text) {
  const source = text.replace(/\r\n?/g, '\n');
  if (!source.startsWith('---\n')) throw new GalaxyError('invalid_markdown', 'frontmatter is required');
  const end = source.indexOf('\n---\n', 4);
  if (end < 0) throw new GalaxyError('invalid_markdown', 'frontmatter is not closed');
  const raw = source.slice(4, end).split('\n');
  const meta = {};
  let section = '';
  let listField = '';
  for (const line of raw) {
    if (!line.trim()) continue;
    const listItem = line.match(/^\s+-\s*(.*)$/);
    if (listItem) {
      if (!listField) throw new GalaxyError('invalid_markdown', 'list item is outside a supported frontmatter list');
      meta[listField].push(parseScalar(listItem[1]));
      continue;
    }
    listField = '';
    if (/^origin:\s*$/.test(line)) { section = 'origin'; meta.origin = {}; continue; }
    const nested = line.match(/^\s+([a-z_]+):\s*(.*)$/);
    if (nested && section === 'origin') {
      if (!ORIGIN_FIELDS.has(nested[1])) throw new GalaxyError('invalid_markdown', 'unknown origin field');
      meta.origin[nested[1]] = parseScalar(nested[2]);
      continue;
    }
    const pair = line.match(/^([a-z_]+):\s*(.*)$/);
    if (!pair || !FRONTMATTER_FIELDS.has(pair[1])) throw new GalaxyError('invalid_markdown', 'unknown frontmatter field');
    if (pair[1] === 'topics' || pair[1] === 'tags') {
      const value = pair[2].trim();
      if (!value) {
        meta[pair[1]] = [];
        listField = pair[1];
        section = pair[1];
      } else {
        meta[pair[1]] = parseInlineList(value, pair[1]);
        section = '';
      }
      continue;
    }
    section = '';
    meta[pair[1]] = parseScalar(pair[2]);
  }
  const body = source.slice(end + 5).replace(/^\n/, '');
  const schemaVersion = Number(meta.schema_version);
  if (!Number.isInteger(schemaVersion) || schemaVersion !== 1) throw new GalaxyError('invalid_markdown', 'frontmatter schema version is invalid');
  const title = bounded(meta.title, 240, 'title', true);
  const content = bounded(body.startsWith(`# ${title}\n\n`) ? body.slice(title.length + 4).trim() : body.trim(), 10000, 'content', true);
  const id = bounded(meta.galaxy_id, 128, 'galaxy_id', true);
  const type = bounded(meta.type, 32, 'type', true);
  const visibility = bounded(meta.visibility, 16, 'visibility', true);
  const reviewStatus = bounded(meta.review_status, 16, 'review_status', true);
  const syncStatus = bounded(meta.sync_status, 16, 'sync_status', true);
  const syncRevision = normalizeSyncRevision(meta.sync_revision);
  const storedHash = bounded(meta.content_hash, 71, 'content_hash', true);
  if (!ITEM_TYPES.has(type) || !VISIBILITIES.has(visibility)) throw new GalaxyError('invalid_markdown', 'frontmatter schema is invalid');
  if (!['inbox', 'draft', 'verified', 'archived', 'rejected'].includes(reviewStatus)) throw new GalaxyError('invalid_markdown', 'review status is invalid');
  if (!['created', 'updated', 'conflict'].includes(syncStatus)) throw new GalaxyError('invalid_markdown', 'sync status is invalid');
  if (!/^sha256:[0-9a-f]{64}$/.test(storedHash)) throw new GalaxyError('invalid_markdown', 'content_hash is invalid');
  if (!Object.prototype.hasOwnProperty.call(meta, 'origin')) throw new GalaxyError('invalid_markdown', 'origin is required');
  const createdAt = normalizeTimestamp(meta.created_at, 'created_at');
  const updatedAt = normalizeTimestamp(meta.updated_at, 'updated_at');
  const topics = normalizeStringList(meta.topics, 'topics', 'invalid_markdown');
  const tags = normalizeStringList(meta.tags, 'tags', 'invalid_markdown');
  const origin = normalizeOrigin(meta.origin, 'invalid_markdown');
  const item = {
    id, schema_version: schemaVersion, title, type, visibility, content, topics, tags,
    review_status: reviewStatus, sync_status: syncStatus, sync_revision: syncRevision,
    created_at: createdAt, updated_at: updatedAt, origin,
    promotion_key: meta.promotion_key || ''
  };
  item.content_hash = contentHash(item);
  return item;
}

async function atomicWrite(filePath, content) {
  const temporary = `${filePath}.tmp-${process.pid}-${crypto.randomBytes(4).toString('hex')}`;
  const handle = await fs.open(temporary, 'wx', 0o600);
  try {
    await handle.writeFile(content, 'utf8');
    await handle.sync();
  } finally {
    await handle.close();
  }
  await fs.rename(temporary, filePath);
}

class GalaxyRepository {
  constructor(vaultPath, folder = 'Memory Galaxy') {
    if (!path.isAbsolute(vaultPath)) throw new GalaxyError('invalid_vault', 'vault path must be absolute');
    this.root = path.join(vaultPath, folder);
    this.promotionLocks = new Map();
    this.scanQueue = Promise.resolve();
  }

  async ensure() {
    await fs.mkdir(this.root, { recursive: true, mode: 0o700 });
    for (const name of ['Inbox', 'Knowledge', 'Decisions', 'Patterns', 'Failures', 'Procedures', 'Research', 'References', 'RAG', 'Skills', 'Harnesses', 'Archive']) {
      await fs.mkdir(path.join(this.root, name), { recursive: true, mode: 0o700 });
    }
  }

  async withPromotionLock(key, operation) {
    const previous = this.promotionLocks.get(key) || Promise.resolve();
    let release;
    const gate = new Promise(resolve => { release = resolve; });
    const queued = previous.then(() => gate);
    this.promotionLocks.set(key, queued);
    await previous;
    try {
      return await operation();
    } finally {
      release();
      if (this.promotionLocks.get(key) === queued) this.promotionLocks.delete(key);
    }
  }

  async files() {
    const results = [];
    const visit = async (directory) => {
      if (results.length >= MAX_SCAN_FILES) return;
      let entries;
      try { entries = await fs.readdir(directory, { withFileTypes: true }); } catch { return; }
      for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
        if (entry.name.startsWith('.')) continue;
        const full = path.join(directory, entry.name);
        if (entry.isDirectory()) {
          if (directory === this.root && entry.name === 'Quarantine') continue;
          await visit(full);
        } else if (entry.isFile() && entry.name.endsWith('.md')) results.push(full);
        if (results.length >= MAX_SCAN_FILES) return;
      }
    };
    await visit(this.root);
    return results;
  }

  async readItem(filePath) {
    const data = await fs.readFile(filePath, 'utf8');
    if (Buffer.byteLength(data, 'utf8') > MAX_FILE_BYTES) throw new GalaxyError('invalid_markdown', 'Galaxy item is too large');
    return { item: parseMarkdown(data), filePath };
  }

  async withScanLock(operation) {
    const previous = this.scanQueue;
    let release;
    const gate = new Promise(resolve => { release = resolve; });
    this.scanQueue = previous.then(() => gate);
    await previous;
    try {
      return await operation();
    } finally {
      release();
    }
  }

  async quarantineDuplicateRecords(records) {
    const quarantineRoot = path.join(this.root, 'Quarantine');
    await fs.mkdir(quarantineRoot, { recursive: true, mode: 0o700 });
    for (const record of records) {
      const target = path.join(quarantineRoot, `${Date.now()}-${crypto.randomBytes(8).toString('hex')}-${path.basename(record.filePath)}`);
      try {
        await fs.rename(record.filePath, target);
      } catch {
        throw new GalaxyError('quarantine_failed', 'Duplicate Galaxy files could not be quarantined; manual review is required', true, 500);
      }
    }
  }

  async readRecords() {
    return this.withScanLock(async () => {
      const quarantineRoot = path.join(this.root, 'Quarantine');
      let quarantinePaths = [];
      try {
        const entries = await fs.readdir(quarantineRoot, { withFileTypes: true });
        if (entries.length > MAX_SCAN_FILES) throw new GalaxyError('repository_limit', 'Quarantine contains too many entries', false, 503);
        quarantinePaths = entries.filter(entry => entry.isFile() && entry.name.endsWith('.md')).map(entry => path.join(quarantineRoot, entry.name));
      } catch (error) {
        if (error instanceof GalaxyError || error.code !== 'ENOENT') throw error;
      }
      const records = [];
      const paths = [
        ...(await this.files()).map(filePath => ({ filePath, quarantined: false })),
        ...quarantinePaths.map(filePath => ({ filePath, quarantined: true }))
      ];
      for (const { filePath, quarantined } of paths) {
        try {
          records.push({ ...(await this.readItem(filePath)), quarantined });
        } catch (error) {
          if (error instanceof GalaxyError || error.code === 'ENOENT') continue;
          throw error;
        }
      }
      const groups = new Map();
      for (const record of records) {
        const group = groups.get(record.item.id) || [];
        group.push(record);
        groups.set(record.item.id, group);
      }
      const duplicates = new Map([...groups].filter(([, group]) => group.length > 1));
      const pending = [...duplicates.values()].flat().filter(record => !record.quarantined);
      if (pending.length) await this.quarantineDuplicateRecords(pending);
      return {
        records: records.filter(record => !record.quarantined && !duplicates.has(record.item.id)),
        duplicates
      };
    });
  }

  async findByID(id) {
    const { records, duplicates } = await this.readRecords();
    if (duplicates.has(id)) throw new GalaxyError('duplicate_id', `Galaxy ID ${id} has duplicate Markdown files in Quarantine`, false, 409);
    const record = records.find(candidate => candidate.item.id === id);
    if (record) return record;
    throw new GalaxyError('not_found', 'Galaxy item was not found', false, 404);
  }

  async findByPromotionKey(key) {
    const { records, duplicates } = await this.readRecords();
    for (const [id, group] of duplicates) {
      if (group.some(record => record.item.promotion_key === key)) {
        throw new GalaxyError('duplicate_id', `Galaxy ID ${id} has duplicate Markdown files in Quarantine`, false, 409);
      }
    }
    return records.find(record => record.item.promotion_key === key) || null;
  }

  async create(candidate, idempotencyKey) {
    const valid = validateCandidate(candidate, idempotencyKey);
    return this.withPromotionLock(valid.promotionKey, async () => {
      await this.ensure();
      const existing = await this.findByPromotionKey(valid.promotionKey);
      if (existing) return { item: existing.item, duplicate: true };
      const now = new Date().toISOString();
      const item = {
        id: galaxyID(), schema_version: 1, title: valid.title, content: valid.content, type: valid.type,
        visibility: valid.visibility, topics: valid.topics, tags: valid.tags,
        review_status: 'inbox', sync_status: 'created', sync_revision: 1,
        created_at: now, updated_at: now, origin: valid.origin
      };
      item.content_hash = contentHash(item);
      const bucket = valid.visibility === 'private' ? 'Inbox' : 'Inbox';
      const filePath = path.join(this.root, bucket, safeFileName(item.title, item.id));
      await atomicWrite(filePath, markdownFor(item, valid.promotionKey));
      return { item, duplicate: false };
    });
  }

  async get(id) {
    return (await this.findByID(bounded(id, 128, 'id', true))).item;
  }

  async search(query, limit = 8) {
    const text = bounded(query, 512, 'query', true).toLowerCase();
    const tokens = text.split(/\s+/).filter(Boolean);
    const hits = [];
    const { records } = await this.readRecords();
    for (const record of records) {
      const item = record.item;
      const haystack = `${item.title} ${item.content} ${item.type} ${(item.topics || []).join(' ')} ${(item.tags || []).join(' ')}`.toLowerCase();
      if (!tokens.every(token => haystack.includes(token))) continue;
      const score = tokens.reduce((total, token) => total + (item.title.toLowerCase().includes(token) ? 4 : 0) + (item.content.toLowerCase().includes(token) ? 1 : 0), 0);
      hits.push({ id: item.id, title: item.title, type: item.type, summary: item.content.slice(0, 280), review_status: item.review_status, visibility: item.visibility, topics: item.topics, tags: item.tags, score });
    }
    return hits.sort((a, b) => b.score - a.score || a.id.localeCompare(b.id)).slice(0, Math.min(Math.max(Number(limit) || 8, 1), 50));
  }
}

module.exports = { GalaxyError, GalaxyRepository, parseMarkdown, validateCandidate, contentHash };

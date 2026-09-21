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

function validateCandidate(candidate, idempotencyKey) {
  if (!candidate || typeof candidate !== 'object') throw new GalaxyError('invalid_item', 'item must be an object');
  const title = bounded(candidate.title, 240, 'title', true);
  const content = bounded(candidate.content, 10000, 'content', true);
  const type = bounded(candidate.type, 32, 'type', true);
  const visibility = bounded(candidate.visibility, 16, 'visibility', true);
  const promotionKey = bounded(idempotencyKey || candidate.idempotency_key, 512, 'idempotency_key', true);
  if (!ITEM_TYPES.has(type)) throw new GalaxyError('invalid_item', `unsupported item type ${type}`);
  if (!VISIBILITIES.has(visibility)) throw new GalaxyError('invalid_item', `unsupported visibility ${visibility}`);
  return { title, content, type, visibility, promotionKey, origin: candidate.origin || {} };
}

function safeFileName(title, id) {
  const stem = title.normalize('NFKC').replace(/[^a-zA-Z0-9 _-]/g, '').trim().replace(/\s+/g, '-').slice(0, 80) || 'untitled';
  return `${stem}--${id}.md`;
}

function galaxyID() {
  const timestamp = Date.now().toString(36).toUpperCase();
  return `MG-${timestamp}-${crypto.randomBytes(8).toString('hex').toUpperCase()}`;
}

function contentHash(title, content) {
  return `sha256:${crypto.createHash('sha256').update(`${title}\n${content}`, 'utf8').digest('hex')}`;
}

function yamlQuote(value) {
  return JSON.stringify(String(value));
}

function markdownFor(item, promotionKey) {
  const origin = item.origin || {};
  const lines = [
    '---',
    `galaxy_id: ${yamlQuote(item.id)}`,
    'schema_version: 1',
    `title: ${yamlQuote(item.title)}`,
    `type: ${item.type}`,
    `created_at: ${item.created_at}`,
    `updated_at: ${item.updated_at}`,
    `review_status: ${item.review_status}`,
    'sync_status: created',
    'sync_revision: 1',
    `content_hash: ${item.content_hash}`,
    `visibility: ${item.visibility}`,
    `promotion_key: ${yamlQuote(promotionKey)}`,
    'origin:',
    `  system: ${yamlQuote(origin.system || 'timekeeper')}`,
    `  project_id: ${yamlQuote(origin.project_id || '')}`,
    `  task_id: ${yamlQuote(origin.task_id || '')}`,
    `  trail_id: ${yamlQuote(origin.trail_id || '')}`,
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
  if ((text.startsWith('"') && text.endsWith('"')) || (text.startsWith("'") && text.endsWith("'"))) {
    try { return JSON.parse(text); } catch { return text.slice(1, -1); }
  }
  return text;
}

function parseMarkdown(text) {
  if (!text.startsWith('---\n')) throw new GalaxyError('invalid_markdown', 'frontmatter is required');
  const end = text.indexOf('\n---\n', 4);
  if (end < 0) throw new GalaxyError('invalid_markdown', 'frontmatter is not closed');
  const raw = text.slice(4, end).split('\n');
  const meta = {};
  let section = '';
  for (const line of raw) {
    if (!line.trim()) continue;
    if (/^origin:\s*$/.test(line)) { section = 'origin'; meta.origin = {}; continue; }
    const nested = line.match(/^\s+([a-z_]+):\s*(.*)$/);
    if (nested && section === 'origin') {
      if (!ORIGIN_FIELDS.has(nested[1])) throw new GalaxyError('invalid_markdown', 'unknown origin field');
      meta.origin[nested[1]] = parseScalar(nested[2]);
      continue;
    }
    const pair = line.match(/^([a-z_]+):\s*(.*)$/);
    if (!pair || !FRONTMATTER_FIELDS.has(pair[1])) throw new GalaxyError('invalid_markdown', 'unknown frontmatter field');
    section = '';
    meta[pair[1]] = parseScalar(pair[2]);
  }
  const body = text.slice(end + 5).replace(/^\n/, '');
  const title = bounded(meta.title, 240, 'title', true);
  const content = bounded(body.startsWith(`# ${title}\n\n`) ? body.slice(title.length + 4).trim() : body.trim(), 10000, 'content', true);
  const id = bounded(meta.galaxy_id, 128, 'galaxy_id', true);
  const type = bounded(meta.type, 32, 'type', true);
  const visibility = bounded(meta.visibility || 'private', 16, 'visibility', true);
  if (String(meta.schema_version || '') !== '1' || !ITEM_TYPES.has(type) || !VISIBILITIES.has(visibility)) throw new GalaxyError('invalid_markdown', 'frontmatter schema is invalid');
  if (!['inbox', 'draft', 'verified', 'archived', 'rejected'].includes(meta.review_status || 'inbox')) throw new GalaxyError('invalid_markdown', 'review status is invalid');
  if (!['created', 'updated', 'conflict'].includes(meta.sync_status || 'created')) throw new GalaxyError('invalid_markdown', 'sync status is invalid');
  return {
    id, title, type, visibility, content,
    review_status: meta.review_status || 'inbox',
    content_hash: contentHash(title, content),
    created_at: meta.created_at || '',
    updated_at: meta.updated_at || '',
    origin: meta.origin || {},
    promotion_key: meta.promotion_key || ''
  };
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
  }

  async ensure() {
    await fs.mkdir(this.root, { recursive: true, mode: 0o700 });
    for (const name of ['Inbox', 'Knowledge', 'Decisions', 'Patterns', 'Failures', 'Procedures', 'Research', 'References', 'RAG', 'Skills', 'Harnesses', 'Archive']) {
      await fs.mkdir(path.join(this.root, name), { recursive: true, mode: 0o700 });
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
        if (entry.isDirectory()) await visit(full);
        else if (entry.isFile() && entry.name.endsWith('.md')) results.push(full);
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

  async findByID(id) {
    for (const filePath of await this.files()) {
      try {
        const record = await this.readItem(filePath);
        if (record.item.id === id) return record;
      } catch (error) {
        if (error instanceof GalaxyError) continue;
      }
    }
    throw new GalaxyError('not_found', 'Galaxy item was not found', false, 404);
  }

  async findByPromotionKey(key) {
    for (const filePath of await this.files()) {
      try {
        const record = await this.readItem(filePath);
        if (record.item.promotion_key === key) return record;
      } catch (error) {
        if (error instanceof GalaxyError) continue;
      }
    }
    return null;
  }

  async create(candidate, idempotencyKey) {
    const valid = validateCandidate(candidate, idempotencyKey);
    await this.ensure();
    const existing = await this.findByPromotionKey(valid.promotionKey);
    if (existing) return { item: existing.item, duplicate: true };
    const now = new Date().toISOString();
    const item = {
      id: galaxyID(), title: valid.title, content: valid.content, type: valid.type,
      visibility: valid.visibility, review_status: 'inbox', created_at: now,
      updated_at: now, content_hash: contentHash(valid.title, valid.content), origin: valid.origin
    };
    const bucket = valid.visibility === 'private' ? 'Inbox' : 'Inbox';
    const filePath = path.join(this.root, bucket, safeFileName(item.title, item.id));
    await atomicWrite(filePath, markdownFor(item, valid.promotionKey));
    return { item, duplicate: false };
  }

  async get(id) {
    return (await this.findByID(bounded(id, 128, 'id', true))).item;
  }

  async search(query, limit = 8) {
    const text = bounded(query, 512, 'query', true).toLowerCase();
    const tokens = text.split(/\s+/).filter(Boolean);
    const hits = [];
    for (const filePath of await this.files()) {
      try {
        const item = (await this.readItem(filePath)).item;
        const haystack = `${item.title} ${item.content} ${item.type} ${(item.topics || []).join(' ')} ${(item.tags || []).join(' ')}`.toLowerCase();
        if (!tokens.every(token => haystack.includes(token))) continue;
        const score = tokens.reduce((total, token) => total + (item.title.toLowerCase().includes(token) ? 4 : 0) + (item.content.toLowerCase().includes(token) ? 1 : 0), 0);
        hits.push({ id: item.id, title: item.title, type: item.type, summary: item.content.slice(0, 280), review_status: item.review_status, visibility: item.visibility, score });
      } catch (error) {
        if (error instanceof GalaxyError) continue;
      }
    }
    return hits.sort((a, b) => b.score - a.score || a.id.localeCompare(b.id)).slice(0, Math.min(Math.max(Number(limit) || 8, 1), 50));
  }
}

module.exports = { GalaxyError, GalaxyRepository, parseMarkdown, validateCandidate, contentHash };

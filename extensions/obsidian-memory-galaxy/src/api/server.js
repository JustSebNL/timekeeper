// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

const crypto = require('node:crypto');
const fs = require('node:fs/promises');
const http = require('node:http');
const path = require('node:path');
const { GalaxyError } = require('../repository');

const PROTOCOL_VERSION = 'timekeeper-knowledge/v1';
const MAX_BODY_BYTES = 16 * 1024;

class GalaxyApiServer {
  constructor(repository, options = {}) {
    this.repository = repository;
    this.host = '127.0.0.1';
    this.port = Number(options.port || 0);
    this.descriptorPath = options.descriptorPath;
    this.tokenPath = options.tokenPath;
    this.server = null;
    this.token = null;
  }

  async start() {
    if (!this.descriptorPath || !this.tokenPath) throw new GalaxyError('invalid_configuration', 'descriptor and capability paths are required');
    this.descriptorPath = path.resolve(this.descriptorPath);
    this.tokenPath = path.resolve(this.tokenPath);
    await fs.mkdir(path.dirname(this.descriptorPath), { recursive: true, mode: 0o700 });
    await fs.mkdir(path.dirname(this.tokenPath), { recursive: true, mode: 0o700 });
    try {
      this.token = (await fs.readFile(this.tokenPath, 'utf8')).trim();
    } catch {
      this.token = crypto.randomBytes(32).toString('base64url');
      await fs.writeFile(this.tokenPath, `${this.token}\n`, { mode: 0o600, flag: 'wx' }).catch(async error => {
        if (error.code !== 'EEXIST') throw error;
        this.token = (await fs.readFile(this.tokenPath, 'utf8')).trim();
      });
    }
    if (!this.token || this.token.length > 512) throw new GalaxyError('invalid_capability', 'capability file is invalid');
    await this.repository.ensure();
    this.server = http.createServer((request, response) => this.handle(request, response));
    await new Promise((resolve, reject) => {
      const onError = error => { this.server.off('listening', resolve); reject(error); };
      this.server.once('error', onError);
      this.server.listen(this.port, this.host, () => {
        this.server.off('error', onError);
        resolve();
      });
    });
    const address = this.server.address();
    const descriptor = {
      base_url: `http://${this.host}:${address.port}`,
      protocol_version: PROTOCOL_VERSION,
      provider: 'obsidian',
      vault_id: crypto.createHash('sha256').update(this.repository.root).digest('hex').slice(0, 24),
      token_file: path.relative(path.dirname(this.descriptorPath), this.tokenPath) || path.basename(this.tokenPath)
    };
    await fs.writeFile(this.descriptorPath, `${JSON.stringify(descriptor, null, 2)}\n`, { mode: 0o600 });
    return descriptor;
  }

  async stop() {
    if (!this.server) return;
    await new Promise((resolve, reject) => this.server.close(error => error ? reject(error) : resolve()));
    this.server = null;
  }

  async handle(request, response) {
    try {
      const url = new URL(request.url, 'http://127.0.0.1');
      if (url.pathname === '/health' && request.method === 'GET') {
        return this.json(response, 200, { status: 'ok', protocol_version: PROTOCOL_VERSION, available: true });
      }
      if (request.headers.origin) throw new GalaxyError('origin_rejected', 'cross-origin requests are not accepted', false, 403);
      if (request.headers.authorization !== `Bearer ${this.token}`) throw new GalaxyError('invalid_capability', 'capability rejected', false, 401);
      if (url.pathname === '/api/v1/galaxy/status' && request.method === 'GET') {
        return this.json(response, 200, { available: true, provider: 'obsidian', protocol_version: PROTOCOL_VERSION, capabilities: ['search', 'get', 'create'], message: 'Memory Galaxy available' });
      }
      if (url.pathname === '/api/v1/galaxy/search' && request.method === 'POST') {
        const body = await this.body(request);
        const query = this.strictObject(body, ['query', 'limit']);
        const results = await this.repository.search(query.query, query.limit);
        return this.json(response, 200, { results });
      }
      if (url.pathname === '/api/v1/galaxy/items' && request.method === 'POST') {
        const body = await this.body(request);
        const candidate = this.strictObject(body, ['title', 'content', 'type', 'visibility', 'promotion_key', 'origin', 'topics', 'tags']);
        const result = await this.repository.create(candidate, candidate.promotion_key);
        return this.json(response, result.duplicate ? 200 : 201, { ...result.item, duplicate: result.duplicate });
      }
      const match = url.pathname.match(/^\/api\/v1\/galaxy\/items\/([^/]+)$/);
      if (match && request.method === 'GET') {
        return this.json(response, 200, await this.repository.get(decodeURIComponent(match[1])));
      }
      throw new GalaxyError('not_found', 'Galaxy route was not found', false, 404);
    } catch (error) {
      const galaxyError = error instanceof GalaxyError ? error : new GalaxyError('provider_error', 'Memory Galaxy request failed', true, 502);
      this.json(response, galaxyError.status || 400, { error: { code: galaxyError.code, message: galaxyError.message, retryable: galaxyError.retryable } });
    }
  }

  async body(request) {
    const chunks = [];
    let size = 0;
    for await (const chunk of request) {
      size += chunk.length;
      if (size > MAX_BODY_BYTES) throw new GalaxyError('request_too_large', 'request body is too large', false, 413);
      chunks.push(chunk);
    }
    try { return JSON.parse(Buffer.concat(chunks).toString('utf8')); } catch { throw new GalaxyError('invalid_json', 'request body must be valid JSON'); }
  }

  strictObject(value, fields) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new GalaxyError('invalid_request', 'request must be an object');
    for (const key of Object.keys(value)) if (!fields.includes(key)) throw new GalaxyError('invalid_request', `unknown request field ${key}`);
    return value;
  }

  json(response, status, payload) {
    const body = JSON.stringify(payload);
    response.writeHead(status, { 'content-type': 'application/json; charset=utf-8', 'content-length': Buffer.byteLength(body) });
    response.end(body);
  }
}

module.exports = { GalaxyApiServer, MAX_BODY_BYTES, PROTOCOL_VERSION };

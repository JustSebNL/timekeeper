// Copyright (c) 2026 Seb. All rights reserved.

'use strict';

function normalizeServerURL(value) {
  let parsed;
  try {
    parsed = new URL(String(value).trim());
  } catch {
    throw new Error('Time Keeper server URL must be a numeric loopback HTTP origin.');
  }
  const numericLoopback = parsed.hostname === '127.0.0.1' || parsed.hostname === '[::1]';
  if (
    parsed.protocol !== 'http:' ||
    !numericLoopback ||
    parsed.username ||
    parsed.password ||
    (parsed.pathname !== '/' && parsed.pathname !== '') ||
    parsed.search ||
    parsed.hash
  ) {
    throw new Error('Time Keeper server URL must be a numeric loopback HTTP origin.');
  }
  return parsed.origin;
}

class TimeKeeperClient {
  constructor(serverURL, fetchImplementation = globalThis.fetch) {
    if (typeof fetchImplementation !== 'function') {
      throw new Error('This VS Code runtime does not provide fetch.');
    }
    this.serverURL = normalizeServerURL(serverURL);
    this.fetch = fetchImplementation;
  }

  async health() {
    return this.request('/health');
  }

  async projects() {
    return this.request('/api/v1/projects');
  }

  async projectTree(projectID) {
    return this.request(`/api/v1/projects/${encodeURIComponent(projectID)}/execution-tree`);
  }

  async sprintAction(sprintID, action) {
    if (!['start', 'hold', 'resume', 'complete'].includes(action)) {
      throw new Error('Unsupported Sprint action.');
    }
    return this.request(`/api/v1/sprints/${encodeURIComponent(sprintID)}/${action}`, { method: 'POST' });
  }

  async request(path, options = {}) {
    const response = await this.fetch(this.serverURL + path, {
      ...options,
      headers: { Accept: 'application/json', 'Content-Type': 'application/json', ...(options.headers || {}) },
    });
    if (!response.ok) {
      let message = `Time Keeper API returned HTTP ${response.status}.`;
      try {
        const payload = await response.json();
        message = payload?.error?.message || payload?.message || message;
      } catch {
        // The stable HTTP status is still useful when a peer returns no JSON.
      }
      throw new Error(message);
    }
    return response.json();
  }
}

module.exports = { TimeKeeperClient, normalizeServerURL };

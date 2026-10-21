import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { runInNewContext } from 'node:vm';

const source = await readFile(new URL('./file-editor.js', import.meta.url), 'utf8');
const pause = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));
const clone = value => JSON.parse(JSON.stringify(value));

class Element {
  constructor() {
    this.handlers = new Map();
    this.children = [];
    this.attributes = new Map();
    this.value = '';
    this.textContent = '';
    this.className = '';
    this.disabled = false;
    this.hidden = false;
  }

  addEventListener(name, handler) {
    this.handlers.set(name, handler);
  }

  setAttribute(name, value) {
    this.attributes.set(name, String(value));
  }

  removeEventListener(name) {
    this.handlers.delete(name);
  }

  replaceChildren() {
    this.children = [];
  }

  appendChild(child) {
    this.children.push(child);
  }

  append(...children) {
    this.children.push(...children);
  }

  get selectedOptions() {
    return this.children.filter(child => child.value === this.value);
  }

  focus() {}
}

test('CodeMirror saves paused edits and manual checkpoints as restorable history versions', async () => {
  const elements = new Map();
  const element = selector => {
    if (!elements.has(selector)) elements.set(selector, new Element());
    return elements.get(selector);
  };
  const file = { file_id: 'F-10001', project_id: 'P-10001', current_path: 'main.go', file_type: 'go', current_version_id: 'FV-10001' };
  const history = [{ version_id: 'FV-10001', version_number: 1, parent_version_id: '', restores_version_id: '', change_kind: 'code_change', change_summary: 'Initial', content_hash: 'hash-1', size_bytes: 12 }];
  const contents = new Map([['FV-10001', 'package one\n']]);
  let editorValue = '';
  let editorOptions;
  let projectRoot = '';

  function createVersion(content, summary, changeKind = 'code_change', restoresVersionID = '') {
    const previous = history[history.length - 1];
    const number = previous.version_number + 1;
    const version = {
      version_id: `FV-1000${number}`,
      file_id: file.file_id,
      version_number: number,
      parent_version_id: previous.version_id,
      restores_version_id: restoresVersionID,
      change_kind: changeKind,
      change_summary: summary,
      content_hash: `hash-${number}`,
      size_bytes: content.length,
      created_at: new Date(2026, 0, number).toISOString()
    };
    history.push(version);
    contents.set(version.version_id, content);
    file.current_version_id = version.version_id;
    return version;
  }

  async function fetchMock(url, options = {}) {
    const pathname = new URL(url, 'http://timekeeper.local').pathname;
    const parts = pathname.split('/');
    const method = options.method || 'GET';
    if (method === 'GET' && pathname === '/api/v1/projects') {
      return { ok: true, status: 200, json: async () => ({ items: [{ project_id: file.project_id, project_name: 'History test project' }] }) };
    }
    if (method === 'GET' && pathname === `/api/v1/projects/${file.project_id}/file-root`) {
      return { ok: true, status: 200, json: async () => ({ project_id: file.project_id, root_path: projectRoot }) };
    }
    if (method === 'PUT' && pathname === `/api/v1/projects/${file.project_id}/file-root`) {
      projectRoot = JSON.parse(options.body).root_path;
      return { ok: true, status: 200, json: async () => ({ project_id: file.project_id, root_path: projectRoot }) };
    }
    if (method === 'POST' && pathname === `/api/v1/projects/${file.project_id}/files/scan`) {
      return { ok: true, status: 200, json: async () => ({ files_discovered: 1, files_added: 0, versions_added: 0, files_skipped: 0 }) };
    }
    if (method === 'GET' && pathname === `/api/v1/projects/${file.project_id}/files`) {
      return { ok: true, status: 200, json: async () => ({ items: [{ file_id: file.file_id, project_id: file.project_id, current_path: file.current_path, file_type: 'go', tracking_policy: 'full' }] }) };
    }
    if (parts[3] !== 'files' || parts[4] !== file.file_id) throw new Error(`Unexpected request ${method} ${pathname}`);
    if (method === 'GET' && parts.length === 5) return { ok: true, status: 200, json: async () => clone(file) };
    if (method === 'GET' && parts[5] === 'versions' && parts.length === 6) return { ok: true, status: 200, json: async () => clone({ items: history }) };
    if (method === 'GET' && parts[5] === 'versions' && parts.length === 7) {
      const version = history.find(item => item.version_id === parts[6]);
      if (!version) return { ok: false, status: 404, json: async () => ({ error: { message: 'version not found' } }) };
      return { ok: true, status: 200, json: async () => ({ version: clone(version), content: contents.get(version.version_id) }) };
    }
    if (method === 'POST' && parts[5] === 'versions') {
      const body = JSON.parse(options.body);
      if (body.expected_current_version_id !== file.current_version_id) return { ok: false, status: 409, json: async () => ({ error: { message: 'file version conflict' } }) };
      if (body.content === contents.get(file.current_version_id)) {
        const current = history[history.length - 1];
        return { ok: true, status: 201, json: async () => ({ file: clone(file), version: clone(current) }) };
      }
      const version = createVersion(body.content, 'Editor edit step', body.change_kind || 'code_change');
      return { ok: true, status: 201, json: async () => ({ file: clone(file), version: clone(version) }) };
    }
    if (method === 'POST' && parts[5] === 'restore') {
      const body = JSON.parse(options.body);
      if (body.expected_current_version_id !== file.current_version_id) return { ok: false, status: 409, json: async () => ({ error: { message: 'file version conflict' } }) };
      const target = history.find(item => item.version_id === body.version_id);
      if (!target) return { ok: false, status: 404, json: async () => ({ error: { message: 'version not found' } }) };
      const version = createVersion(contents.get(target.version_id), `Restored ${target.version_id}`, 'reversion', target.version_id);
      return { ok: true, status: 200, json: async () => ({ file: clone(file), version: clone(version) }) };
    }
    if (method === 'GET' && parts[5] === 'compare') {
      const from = history.find(item => item.version_id === new URL(url, 'http://timekeeper.local').searchParams.get('from'));
      const to = history.find(item => item.version_id === new URL(url, 'http://timekeeper.local').searchParams.get('to'));
      return { ok: true, status: 200, json: async () => ({ from: clone(from), to: clone(to), hunks: [{ from_start: 1, from_count: 1, to_start: 1, to_count: 1, removed: ['package one'], added: ['package four'] }] }) };
    }
    throw new Error(`Unexpected request ${method} ${pathname}`);
  }

  const window = {
    confirm: () => true,
    addEventListener() {},
    TimeKeeperCodeMirror: {
      mount(_host, options) {
        editorOptions = options;
        editorValue = options.value;
        return {
          getValue: () => editorValue,
          setValue: value => { editorValue = String(value); },
          focus() {},
          destroy() {}
        };
      }
    }
  };
  const document = {
    querySelector: element,
    createElement: () => new Element()
  };
  runInNewContext(source, { document, window, fetch: fetchMock, setTimeout, clearTimeout, encodeURIComponent, Promise, String, Error });

  const waitForProjectFiles = async () => {
    const deadline = Date.now() + 1000;
    while (element('#file-history-files').children.length === 0 && Date.now() < deadline) await pause(5);
    assert.equal(element('#file-history-project').value, file.project_id);
    assert.equal(element('#file-history-files').children.length, 1);
    assert.match(element('#file-history-files').children[0].children[0].textContent, /main\.go/);
  };
  await waitForProjectFiles();
  assert.match(element('#file-history-root-status').textContent, /Set a project source folder/);
  element('#file-history-root-path').value = '/workspace/project';
  await element('#file-history-set-root').handlers.get('click')();
  assert.equal(projectRoot, '/workspace/project');
  assert.equal(element('#file-history-scan').disabled, false);
  await element('#file-history-scan').handlers.get('click')();
  assert.match(element('#file-history-root-status').textContent, /Scanned 1 text files/);

  element('#file-history-filter').value = 'does-not-exist';
  element('#file-history-filter').handlers.get('input')();
  assert.match(element('#file-history-files').textContent, /No files match/);
  element('#file-history-filter').value = '';
  element('#file-history-filter').handlers.get('input')();
  await element('#file-history-files').children[0].handlers.get('click')();
  assert.equal(element('#file-history-id').value, file.file_id);
  assert.equal(editorValue, 'package one\n');
  assert.equal(element('#file-history-timeline').children.length, 1);
  assert.match(element('#file-history-timeline').children[0].children[1].children[0].children[0].children[1].textContent, /V1/);
  assert.equal(element('#file-history-legend').children.length, 16);
  assert.equal(element('#file-history-change-kind').children.length, 16);

  const waitForVersions = async count => {
    const deadline = Date.now() + 2500;
    while (history.length < count && Date.now() < deadline) await pause(10);
    assert.equal(history.length, count, `expected ${count} versions, got ${history.length}`);
  };

  editorValue = 'package two\n';
  editorOptions.onChange(editorValue);
  await waitForVersions(2);
  editorValue = 'package three\n';
  editorOptions.onChange(editorValue);
  await waitForVersions(3);

  assert.equal(history[1].parent_version_id, history[0].version_id);
  assert.equal(history[2].parent_version_id, history[1].version_id);
  assert.equal(contents.get(history[1].version_id), 'package two\n');
  assert.equal(contents.get(history[2].version_id), 'package three\n');

  element('#file-history-change-kind').value = 'bugfix';
  editorValue = 'package four\n';
  editorOptions.onChange(editorValue);
  await element('#file-history-save').handlers.get('click')();
  assert.equal(history.length, 4);
  assert.equal(history[3].parent_version_id, history[2].version_id);
  assert.equal(history[3].change_kind, 'bugfix');
  assert.equal(contents.get(history[3].version_id), 'package four\n');
  element('#file-history-compare-from').value = history[0].version_id;
  element('#file-history-compare-to').value = history[3].version_id;
  await element('#file-history-compare').handlers.get('click')();
  assert.match(element('#file-history-diff').textContent, /-package one/);
  assert.match(element('#file-history-diff').textContent, /\+package four/);
  const latestTimelineItem = element('#file-history-timeline').children[0].children[1].children[3].children[0];
  latestTimelineItem.handlers.get('click')();
  await pause(0);
  assert.match(element('#file-history-preview').textContent, /package four/);

  element('#file-history-version').value = history[1].version_id;
  await element('#file-history-restore').handlers.get('click')();
  assert.equal(editorValue, 'package two\n');
  assert.equal(history.length, 5);
  assert.equal(element('#file-history-timeline').children.length, 2);
  assert.match(element('#file-history-timeline').children[1].children[0].children[0].textContent, /Branch 1 from V2/);
  assert.equal(history[4].parent_version_id, history[3].version_id);
  assert.equal(history[4].restores_version_id, history[1].version_id);
  assert.equal(history[4].change_kind, 'reversion');
  assert.equal(contents.get(history[2].version_id), 'package three\n');
  assert.equal(contents.get(history[3].version_id), 'package four\n');
  assert.equal(contents.get(history[4].version_id), 'package two\n');
});

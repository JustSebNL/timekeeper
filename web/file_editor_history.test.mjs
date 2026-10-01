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
    this.value = '';
    this.textContent = '';
    this.className = '';
    this.disabled = false;
    this.hidden = false;
  }

  addEventListener(name, handler) {
    this.handlers.set(name, handler);
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
  const history = [{ version_id: 'FV-10001', version_number: 1, parent_version_id: '', change_summary: 'Initial', content_hash: 'hash-1' }];
  const contents = new Map([['FV-10001', 'package one\n']]);
  let editorValue = '';
  let editorOptions;

  function createVersion(content, summary) {
    const previous = history[history.length - 1];
    const number = previous.version_number + 1;
    const version = {
      version_id: `FV-1000${number}`,
      file_id: file.file_id,
      version_number: number,
      parent_version_id: previous.version_id,
      change_summary: summary,
      content_hash: `hash-${number}`
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
      const version = createVersion(body.content, 'CodeMirror edit step');
      return { ok: true, status: 201, json: async () => ({ file: clone(file), version: clone(version) }) };
    }
    if (method === 'POST' && parts[5] === 'restore') {
      const body = JSON.parse(options.body);
      if (body.expected_current_version_id !== file.current_version_id) return { ok: false, status: 409, json: async () => ({ error: { message: 'file version conflict' } }) };
      const target = history.find(item => item.version_id === body.version_id);
      if (!target) return { ok: false, status: 404, json: async () => ({ error: { message: 'version not found' } }) };
      const version = createVersion(contents.get(target.version_id), `Restored ${target.version_id}`);
      return { ok: true, status: 200, json: async () => ({ file: clone(file), version: clone(version) }) };
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

  element('#file-history-id').value = file.file_id;
  await element('#file-history-load-form').handlers.get('submit')({ preventDefault() {} });
  assert.equal(editorValue, 'package one\n');

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

  editorValue = 'package four\n';
  editorOptions.onChange(editorValue);
  await element('#file-history-save').handlers.get('click')();
  assert.equal(history.length, 4);
  assert.equal(history[3].parent_version_id, history[2].version_id);
  assert.equal(contents.get(history[3].version_id), 'package four\n');

  element('#file-history-version').value = history[1].version_id;
  await element('#file-history-restore').handlers.get('click')();
  assert.equal(editorValue, 'package two\n');
  assert.equal(history.length, 5);
  assert.equal(history[4].parent_version_id, history[3].version_id);
  assert.equal(contents.get(history[2].version_id), 'package three\n');
  assert.equal(contents.get(history[3].version_id), 'package four\n');
  assert.equal(contents.get(history[4].version_id), 'package two\n');
});

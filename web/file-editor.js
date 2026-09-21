(() => {
  'use strict';

  const $ = selector => document.querySelector(selector);
  const loadForm = $('#file-history-load-form');
  const fileIdInput = $('#file-history-id');
  const meta = $('#file-history-meta');
  const status = $('#file-history-status');
  const host = $('#file-editor-host');
  const fallback = $('#file-editor-fallback');
  const saveButton = $('#file-history-save');
  const restoreButton = $('#file-history-restore');
  const versionSelect = $('#file-history-version');
  const fromSelect = $('#file-history-compare-from');
  const toSelect = $('#file-history-compare-to');
  const compareButton = $('#file-history-compare');
  const diff = $('#file-history-diff');
  const feedback = $('#file-history-feedback');
  let fileId = '';
  let file = null;
  let versions = [];
  let editor = null;

  function api(path, options) {
    return fetch('/api/v1' + path, Object.assign({ headers: { 'Content-Type': 'application/json' } }, options || {})).then(async response => {
      const body = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(body.error?.message || `Request failed (${response.status})`);
      return body;
    });
  }

  function setFeedback(message, error = false) {
    feedback.textContent = message || '';
    feedback.className = error ? 'inline-feedback error' : 'inline-feedback';
  }

  function languageFor(path) {
    const extension = String(path || '').split('.').pop().toLowerCase();
    return extension === 'js' || extension === 'mjs' || extension === 'ts' ? 'javascript' : extension;
  }

  function mountEditor(content) {
    if (editor) editor.destroy();
    host.replaceChildren();
    fallback.hidden = true;
    if (window.TimeKeeperCodeMirror && typeof window.TimeKeeperCodeMirror.mount === 'function') {
      editor = window.TimeKeeperCodeMirror.mount(host, { value: content, language: languageFor(file.current_path) });
      return;
    }
    fallback.hidden = false;
    fallback.value = content;
    editor = {
      getValue: () => fallback.value,
      setValue: value => { fallback.value = value; },
      focus: () => fallback.focus(),
      destroy: () => {}
    };
  }

  function setVersionOptions() {
    const selects = [versionSelect, fromSelect, toSelect];
    for (const select of selects) {
      select.replaceChildren();
      for (const version of versions) {
        const option = document.createElement('option');
        option.value = version.version_id;
        option.textContent = `V${version.version_number} · ${version.version_id} · ${version.change_summary || version.content_hash.slice(0, 18)}`;
        select.appendChild(option);
      }
      select.disabled = versions.length === 0;
    }
    if (versions.length) {
      versionSelect.value = versions[versions.length - 1].version_id;
      fromSelect.value = versions.length > 1 ? versions[versions.length - 2].version_id : versions[0].version_id;
      toSelect.value = versions[versions.length - 1].version_id;
    }
  }

  async function loadFile() {
    fileId = fileIdInput.value.trim();
    if (!fileId) return;
    setFeedback('Loading file history…');
    try {
      file = await api(`/files/${encodeURIComponent(fileId)}`);
      const versionResponse = await api(`/files/${encodeURIComponent(fileId)}/versions`);
      versions = versionResponse.items || [];
      const current = versions.find(version => version.version_id === file.current_version_id) || versions[versions.length - 1];
      if (!current) throw new Error('No versions exist for this file.');
      const content = await api(`/files/${encodeURIComponent(fileId)}/versions/${encodeURIComponent(current.version_id)}`);
      mountEditor(content.content || '');
      meta.className = 'file-history-meta';
      meta.textContent = `${file.file_id} · ${file.current_path} · current ${current.version_id} · ${versions.length} version${versions.length === 1 ? '' : 's'}`;
      status.textContent = 'History connected';
      saveButton.disabled = false;
      restoreButton.disabled = false;
      compareButton.disabled = versions.length < 2;
      setVersionOptions();
      setFeedback('Loaded. Saves create a new immutable version.');
    } catch (error) {
      status.textContent = 'History unavailable';
      setFeedback(error.message, true);
    }
  }

  async function saveFile() {
    if (!file || !editor) return;
    saveButton.disabled = true;
    try {
      const result = await api(`/files/${encodeURIComponent(fileId)}/versions`, {
        method: 'POST',
        body: JSON.stringify({ content: editor.getValue(), expected_current_version_id: file.current_version_id })
      });
      file = result.file;
      await loadFile();
      setFeedback(`Saved ${result.version.version_id} without rewriting history.`);
    } catch (error) {
      setFeedback(error.message, true);
      saveButton.disabled = false;
    }
  }

  async function restoreFile() {
    if (!file || !versionSelect.value) return;
    const selected = versionSelect.selectedOptions[0]?.textContent || versionSelect.value;
    if (!window.confirm(`Restore ${selected} as a new version? Previous versions remain preserved.`)) return;
    restoreButton.disabled = true;
    try {
      await api(`/files/${encodeURIComponent(fileId)}/restore`, {
        method: 'POST',
        body: JSON.stringify({ version_id: versionSelect.value, expected_current_version_id: file.current_version_id })
      });
      await loadFile();
      setFeedback('Restored as a new version.');
    } catch (error) {
      setFeedback(error.message, true);
      restoreButton.disabled = false;
    }
  }

  async function compareFiles() {
    if (!fromSelect.value || !toSelect.value) return;
    try {
      const result = await api(`/files/${encodeURIComponent(fileId)}/compare?from=${encodeURIComponent(fromSelect.value)}&to=${encodeURIComponent(toSelect.value)}`);
      diff.hidden = false;
      diff.textContent = `FROM ${result.from.version_id} → TO ${result.to.version_id}\n+${result.added_lines} lines / -${result.removed_lines} lines\n\n--- FROM ---\n${result.from_content}\n\n+++ TO +++\n${result.to_content}`;
    } catch (error) {
      setFeedback(error.message, true);
    }
  }

  loadForm?.addEventListener('submit', event => { event.preventDefault(); loadFile(); });
  saveButton?.addEventListener('click', saveFile);
  restoreButton?.addEventListener('click', restoreFile);
  compareButton?.addEventListener('click', compareFiles);
})();

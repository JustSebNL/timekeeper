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
  let savedContent = '';
  let autoSaveTimer = null;
  let queuedSaves = 0;
  let saveQueue = Promise.resolve();
  const autoSaveDelay = 600;

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
      editor = window.TimeKeeperCodeMirror.mount(host, { value: content, language: languageFor(file.current_path), onChange: handleEditorChange });
      return;
    }
    fallback.hidden = false;
    fallback.value = content;
    const onInput = () => handleEditorChange(fallback.value);
    fallback.addEventListener('input', onInput);
    editor = {
      getValue: () => fallback.value,
      setValue: value => { fallback.value = value; },
      focus: () => fallback.focus(),
      destroy: () => fallback.removeEventListener('input', onInput)
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

  function updateSaveButton() {
    saveButton.disabled = !file || !editor || (editor.getValue() === savedContent && queuedSaves === 0);
  }

  function updateFileMeta() {
    if (!file) return;
    meta.className = 'file-history-meta';
    meta.textContent = `${file.file_id} · ${file.current_path} · current ${file.current_version_id} · ${versions.length} version${versions.length === 1 ? '' : 's'}`;
  }

  function scheduleAutoSave(content) {
    if (autoSaveTimer !== null) clearTimeout(autoSaveTimer);
    autoSaveTimer = setTimeout(() => {
      autoSaveTimer = null;
      queueHistoryStep(content).catch(() => {});
    }, autoSaveDelay);
  }

  function handleEditorChange(content) {
    if (!file || !editor) return;
    const snapshot = String(content);
    if (snapshot === savedContent && queuedSaves === 0) {
      if (autoSaveTimer !== null) clearTimeout(autoSaveTimer);
      autoSaveTimer = null;
      updateSaveButton();
      setFeedback('All edits are saved.');
      return;
    }
    updateSaveButton();
    setFeedback('Saving this edit as a history step after a short pause…');
    scheduleAutoSave(snapshot);
  }

  function finishHistoryStep(success) {
    queuedSaves = Math.max(0, queuedSaves - 1);
    updateSaveButton();
    if (success && editor && file && editor.getValue() !== savedContent && autoSaveTimer === null && queuedSaves === 0) {
      scheduleAutoSave(editor.getValue());
    }
  }

  function queueHistoryStep(content) {
    const snapshot = String(content);
    queuedSaves += 1;
    updateSaveButton();
    const operation = saveQueue.then(async () => {
      if (!file || snapshot === savedContent) return null;
      const targetFileId = fileId;
      const result = await api(`/files/${encodeURIComponent(targetFileId)}/versions`, {
        method: 'POST',
        body: JSON.stringify({ content: snapshot, expected_current_version_id: file.current_version_id })
      });
      if (fileId !== targetFileId) throw new Error('File selection changed while saving; reload history before continuing.');
      file = result.file;
      savedContent = snapshot;
      const existing = versions.findIndex(version => version.version_id === result.version.version_id);
      if (existing < 0) versions.push(result.version);
      else versions[existing] = result.version;
      versions.sort((left, right) => left.version_number - right.version_number);
      setVersionOptions();
      updateFileMeta();
      setFeedback(`Saved V${result.version.version_number}. Every saved step remains available in history.`);
      return result;
    });
    saveQueue = operation.then(
      () => finishHistoryStep(true),
      error => {
        setFeedback(error.message, true);
        finishHistoryStep(false);
      }
    );
    return operation;
  }

  async function loadFile() {
    const requestedFileId = fileIdInput.value.trim();
    if (!requestedFileId) return;
    if (file && editor) {
      if (autoSaveTimer !== null) clearTimeout(autoSaveTimer);
      autoSaveTimer = null;
      const saved = await saveFile();
      if (!saved && !window.confirm('The latest edit could not be saved. Load the selected file and discard the current editor text?')) return;
      await saveQueue;
    }
    fileId = requestedFileId;
    setFeedback('Loading file history…');
    try {
      file = await api(`/files/${encodeURIComponent(fileId)}`);
      const versionResponse = await api(`/files/${encodeURIComponent(fileId)}/versions`);
      versions = versionResponse.items || [];
      const current = versions.find(version => version.version_id === file.current_version_id) || versions[versions.length - 1];
      if (!current) throw new Error('No versions exist for this file.');
      const content = await api(`/files/${encodeURIComponent(fileId)}/versions/${encodeURIComponent(current.version_id)}`);
      savedContent = content.content || '';
      mountEditor(savedContent);
      updateFileMeta();
      status.textContent = 'History connected · Auto-save on';
      restoreButton.disabled = false;
      compareButton.disabled = versions.length < 2;
      setVersionOptions();
      updateSaveButton();
      setFeedback('Loaded. Each edit is saved as a restorable history step after a short pause.');
    } catch (error) {
      status.textContent = 'History unavailable';
      setFeedback(error.message, true);
    }
  }

  async function saveFile() {
    if (!file || !editor) return false;
    if (autoSaveTimer !== null) clearTimeout(autoSaveTimer);
    autoSaveTimer = null;
    const content = editor.getValue();
    if (content === savedContent && queuedSaves === 0) {
      updateSaveButton();
      return true;
    }
    try {
      await queueHistoryStep(content);
      await saveQueue;
      return editor.getValue() === savedContent;
    } catch {
      return false;
    }
  }

  async function restoreFile() {
    if (!file || !versionSelect.value) return;
    const versionId = versionSelect.value;
    const selected = versionSelect.selectedOptions[0]?.textContent || versionId;
    if (!window.confirm(`Restore ${selected} as a new version? Previous steps remain preserved.`)) return;
    if (!(await saveFile())) return;
    restoreButton.disabled = true;
    try {
      await api(`/files/${encodeURIComponent(fileId)}/restore`, {
        method: 'POST',
        body: JSON.stringify({ version_id: versionId, expected_current_version_id: file.current_version_id })
      });
      await loadFile();
      setFeedback(`Restored ${versionId} as a new version; all earlier steps remain in history.`);
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

  loadForm?.addEventListener('submit', event => { event.preventDefault(); return loadFile(); });
  saveButton?.addEventListener('click', saveFile);
  restoreButton?.addEventListener('click', restoreFile);
  compareButton?.addEventListener('click', compareFiles);
  window.addEventListener('beforeunload', event => {
    if (editor && (editor.getValue() !== savedContent || queuedSaves > 0 || autoSaveTimer !== null)) {
      event.preventDefault();
      event.returnValue = '';
    }
  });
})();

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
  const projectSelect = $('#file-history-project');
  const projectRootInput = $('#file-history-root-path');
  const projectRootStatus = $('#file-history-root-status');
  const setProjectRootButton = $('#file-history-set-root');
  const scanProjectButton = $('#file-history-scan');
  const fileFilter = $('#file-history-filter');
  const fileList = $('#file-history-files');
  const timeline = $('#file-history-timeline');
  const versionPreview = $('#file-history-preview');
  const changeKindSelect = $('#file-history-change-kind');
  const changeKindLegend = $('#file-history-legend');
  let files = [];
  let projects = [];
  let selectedVersionID = '';
  let fileListRequest = 0;
  let previewRequest = 0;
  let fileId = '';
  let file = null;
  let versions = [];
  let editor = null;
  let mergeView = null;
  let savedContent = '';
  let autoSaveTimer = null;
  let queuedSaves = 0;
  let saveQueue = Promise.resolve();
  const autoSaveDelay = 600;
  const CHANGE_KINDS = [
    { id: 'milestone', label: 'Milestone', icon: '🏁' },
    { id: 'bugfix', label: 'Bugfix', icon: '🪲' },
    { id: 'code_change', label: 'Regular code change', icon: '{}' },
    { id: 'reversion', label: 'Code reversion / branch', icon: '↶' },
    { id: 'feature', label: 'Feature implementation', icon: '✨' },
    { id: 'release', label: 'App release', icon: '🚀' },
    { id: 'feature_eol', label: 'Feature end-of-life', icon: '⌛' },
    { id: 'file_added', label: 'File added', icon: '＋' },
    { id: 'refactor', label: 'Refactor', icon: '♻' },
    { id: 'test', label: 'Test change', icon: '🧪' },
    { id: 'documentation', label: 'Documentation', icon: '📚' },
    { id: 'security', label: 'Security', icon: '🛡' },
    { id: 'performance', label: 'Performance', icon: '⚡' },
    { id: 'dependency', label: 'Dependency update', icon: '📦' },
    { id: 'configuration', label: 'Configuration', icon: '⚙' },
    { id: 'other', label: 'Other', icon: '•' }
  ];
  const CHANGE_KIND_BY_ID = new Map(CHANGE_KINDS.map(item => [item.id, item]));

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

  function renderChangeKindOptions() {
    changeKindSelect.replaceChildren();
    for (const kind of CHANGE_KINDS) {
      const option = document.createElement('option');
      option.value = kind.id;
      option.textContent = `${kind.icon} ${kind.label}`;
      changeKindSelect.appendChild(option);
    }
    changeKindSelect.value = 'code_change';
  }

  function renderChangeKindLegend() {
    changeKindLegend.replaceChildren();
    for (const kind of CHANGE_KINDS) {
      const item = document.createElement('span');
      item.className = 'file-history-legend-item';
      item.setAttribute('title', kind.label);
      const icon = document.createElement('strong');
      icon.textContent = kind.icon;
      const label = document.createElement('span');
      label.textContent = kind.label;
      item.append(icon, label);
      changeKindLegend.appendChild(item);
    }
  }

  async function loadProjects() {
    projectSelect.disabled = true;
    try {
      const response = await api('/projects');
      projects = (response.items || []).slice().sort((a, b) => String(a.project_name || a.project_id).localeCompare(String(b.project_name || b.project_id)));
      projectSelect.replaceChildren();
      const placeholder = document.createElement('option');
      placeholder.value = '';
      placeholder.textContent = projects.length ? 'Choose a project…' : 'No projects available';
      projectSelect.appendChild(placeholder);
      for (const project of projects) {
        const option = document.createElement('option');
        option.value = project.project_id;
        option.textContent = `${project.project_name || project.project_id} · ${project.project_id}`;
        projectSelect.appendChild(option);
      }
      projectSelect.disabled = projects.length === 0;
      if (projects.length) {
        projectSelect.value = projects[0].project_id;
        await Promise.all([loadProjectRoot(projects[0].project_id), loadProjectFiles(projects[0].project_id)]);
      } else {
        fileList.className = 'file-explorer-list empty';
        fileList.textContent = 'Create a project to browse its tracked files.';
      }
    } catch (error) {
      projectSelect.replaceChildren();
      const option = document.createElement('option');
      option.value = '';
      option.textContent = 'Projects unavailable';
      projectSelect.appendChild(option);
      fileList.className = 'file-explorer-list empty error';
      fileList.textContent = `Could not load projects: ${error.message}`;
      projectSelect.disabled = true;
    }
  }

  async function loadProjectFiles(projectID) {
    if (!projectID) return;
    const requestID = ++fileListRequest;
    fileList.className = 'file-explorer-list empty';
    fileList.textContent = 'Loading tracked files…';
    try {
      const response = await api(`/projects/${encodeURIComponent(projectID)}/files`);
      if (requestID !== fileListRequest) return;
      files = (response.items || []).slice().sort((a, b) => a.current_path.localeCompare(b.current_path));
      renderFiles();
      status.textContent = 'History connected · ' + files.length + ' tracked file' + (files.length === 1 ? '' : 's');
    } catch (error) {
      if (requestID !== fileListRequest) return;
      files = [];
      fileList.className = 'file-explorer-list empty error';
      fileList.textContent = error.message.includes('not configured') || error.message.includes('History')
        ? 'File history is not configured on this server.'
        : `Could not load project files: ${error.message}`;
      status.textContent = 'History unavailable';
    }
  }

  async function loadProjectRoot(projectID) {
    projectRootInput.value = '';
    scanProjectButton.disabled = true;
    if (!projectID) return;
    try {
      const root = await api(`/projects/${encodeURIComponent(projectID)}/file-root`);
      projectRootInput.value = root.root_path || '';
      projectRootStatus.textContent = root.root_path
        ? `Bound to ${root.root_path}`
        : 'Set a project source folder before the first inventory scan.';
      scanProjectButton.disabled = !root.root_path;
    } catch (error) {
      projectRootStatus.textContent = `Project root unavailable: ${error.message}`;
    }
  }

  async function setProjectRoot() {
    const projectID = projectSelect.value;
    const rootPath = projectRootInput.value.trim();
    if (!projectID || !rootPath) return;
    setProjectRootButton.disabled = true;
    projectRootStatus.textContent = 'Validating source folder…';
    try {
      const root = await api(`/projects/${encodeURIComponent(projectID)}/file-root`, {
        method: 'PUT', body: JSON.stringify({ root_path: rootPath })
      });
      projectRootInput.value = root.root_path;
      projectRootStatus.textContent = `Bound to ${root.root_path}`;
      scanProjectButton.disabled = false;
    } catch (error) {
      projectRootStatus.textContent = error.message;
      setFeedback(error.message, true);
    } finally {
      setProjectRootButton.disabled = false;
    }
  }

  async function scanProject() {
    const projectID = projectSelect.value;
    if (!projectID || !projectRootInput.value.trim()) return;
    if (!window.confirm('Scan this folder and add supported text files plus changed contents to this Project’s version history?')) return;
    scanProjectButton.disabled = true;
    projectRootStatus.textContent = 'Scanning project files…';
    try {
      const result = await api(`/projects/${encodeURIComponent(projectID)}/files/scan`, { method: 'POST', body: '{}' });
      projectRootStatus.textContent = `Scanned ${result.files_discovered} text files · ${result.files_added} added · ${result.versions_added} new history steps · ${result.files_skipped} skipped`;
      await loadProjectFiles(projectID);
      if (file && file.project_id === projectID) setFeedback('Project scan complete. Reopen the selected file to load the newest captured version.');
    } catch (error) {
      projectRootStatus.textContent = error.message;
      setFeedback(error.message, true);
    } finally {
      scanProjectButton.disabled = false;
    }
  }

  function renderFiles() {
    const query = fileFilter.value.trim().toLowerCase();
    const visible = files.filter(item => `${item.current_path} ${item.file_id} ${item.file_type}`.toLowerCase().includes(query));
    fileList.replaceChildren();
    fileList.className = visible.length ? 'file-explorer-list' : 'file-explorer-list empty';
    if (!visible.length) {
      fileList.textContent = files.length
        ? 'No files match this filter.'
        : projectRootInput.value ? 'No files indexed yet. Click Scan and index to discover supported text files.' : 'No tracked files for this project yet.';
      return;
    }
    for (const item of visible) {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'file-explorer-item' + (item.file_id === fileId ? ' active' : '');
      button.setAttribute('role', 'option');
      button.setAttribute('aria-selected', String(item.file_id === fileId));
      const path = document.createElement('strong');
      path.textContent = item.current_path;
      const detail = document.createElement('small');
      detail.textContent = `${item.file_id} · ${item.file_type || 'text'} · ${item.tracking_policy || 'tracked'}`;
      button.append(path, detail);
      button.addEventListener('click', async () => {
        fileIdInput.value = item.file_id;
        await loadFile();
      });
      fileList.appendChild(button);
    }
  }

  async function previewVersion(version) {
    if (!file) return;
    const requestID = ++previewRequest;
    versionPreview.hidden = false;
    versionPreview.textContent = `Loading V${version.version_number}…`;
    try {
      const result = await api(`/files/${encodeURIComponent(fileId)}/versions/${encodeURIComponent(version.version_id)}`);
      if (requestID !== previewRequest) return;
      const when = version.created_at ? new Date(version.created_at).toLocaleString() : 'time unknown';
      const kind = CHANGE_KIND_BY_ID.get(version.change_kind) || CHANGE_KIND_BY_ID.get('other');
      const branch = version.restores_version_id ? ` · reverts from ${version.restores_version_id}` : '';
      versionPreview.textContent = `${kind.icon} ${kind.label} · V${version.version_number} · ${when}${branch}\n${version.change_summary || ''}\n${version.content_hash || ''}\n\n${result.content || ''}`;
    } catch (error) {
      if (requestID !== previewRequest) return;
      versionPreview.textContent = `Could not load this revision: ${error.message}`;
    }
  }

  function renderHistoryTimeline() {
    timeline.replaceChildren();
    const ordered = versions.slice().sort((a, b) => a.version_number - b.version_number);
    const lanes = [{ label: 'Main line', branch: false, sourceVersion: null, versions: [] }];
    const laneByVersion = new Map();
    let currentLane = lanes[0];

    for (const version of ordered) {
      if (version.change_kind === 'reversion' && version.restores_version_id) {
        const sourceVersion = versions.find(item => item.version_id === version.restores_version_id);
        const sourceLane = laneByVersion.get(version.restores_version_id);
        currentLane = {
          label: `Branch ${lanes.length} from V${sourceVersion?.version_number || '?'}`,
          branch: true,
          sourceVersion,
          sourceLane,
          versions: []
        };
        lanes.push(currentLane);
      }
      currentLane.versions.push(version);
      laneByVersion.set(version.version_id, currentLane);
    }

    timeline.setAttribute('data-version-count', String(versions.length));
    for (const lane of lanes) {
      const laneElement = document.createElement('section');
      laneElement.className = lane.branch ? 'file-history-lane branch' : 'file-history-lane';
      laneElement.setAttribute('role', 'listitem');
      const heading = document.createElement('header');
      heading.className = 'file-history-lane-heading';
      const title = document.createElement('strong');
      title.textContent = lane.branch ? `↶ ${lane.label}` : `│ ${lane.label}`;
      const source = document.createElement('small');
      source.textContent = lane.branch
        ? `Restored from ${lane.sourceVersion?.version_id || 'unknown'} · ${lane.sourceLane?.label || 'history'}`
        : `${lane.versions.length} revision${lane.versions.length === 1 ? '' : 's'}`;
      heading.append(title, source);
      const events = document.createElement('div');
      events.className = 'file-history-lane-events';
      for (const version of lane.versions) {
        const meta = CHANGE_KIND_BY_ID.get(version.change_kind) || CHANGE_KIND_BY_ID.get('other');
        const event = document.createElement('article');
        event.className = `file-history-timeline-item change-${version.change_kind || 'code_change'}` + (version.version_id === selectedVersionID ? ' active' : '');
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'file-history-version' + (version.version_id === selectedVersionID ? ' active' : '');
        button.setAttribute('aria-pressed', String(version.version_id === selectedVersionID));
        const icon = document.createElement('span');
        icon.className = 'file-history-version-icon';
        icon.setAttribute('aria-label', meta.label);
        icon.textContent = meta.icon;
        const label = document.createElement('strong');
        label.textContent = `V${version.version_number}` + (version.version_id === file?.current_version_id ? ' · current' : '');
        const time = document.createElement('time');
        time.dateTime = version.created_at || '';
        time.textContent = version.created_at ? new Date(version.created_at).toLocaleString() : 'time unknown';
        const summary = document.createElement('small');
        const provenance = [version.change_summary, version.agent_id, version.session_id, version.task_id, version.sprint_id].filter(Boolean).join(' · ');
        const branchNote = version.restores_version_id ? `↶ from V${versions.find(item => item.version_id === version.restores_version_id)?.version_number || '?'}` : '';
        summary.textContent = `${meta.label}${branchNote ? ' ' + branchNote : ''} · ${provenance || 'Snapshot'} · ${version.size_bytes || 0} bytes · ${version.content_hash || ''}`;
        button.append(icon, label, time, summary);
        button.addEventListener('click', () => {
          selectedVersionID = version.version_id;
          versionSelect.value = version.version_id;
          fromSelect.value = version.version_id;
          toSelect.value = file?.current_version_id || versions[versions.length - 1]?.version_id || version.version_id;
          renderHistoryTimeline();
          void previewVersion(version);
          setFeedback(`Selected V${version.version_number} · ${meta.label}. Inspect, compare, or restore this revision as a new branch.`);
        });
        event.appendChild(button);
        events.appendChild(event);
      }
      laneElement.append(heading, events);
      timeline.appendChild(laneElement);
    }
  }

  function languageFor(path) {
    const extension = String(path || '').split('.').pop().toLowerCase();
    return extension === 'js' || extension === 'mjs' || extension === 'ts' ? 'javascript' : extension;
  }

  function destroyMergeView() {
    if (mergeView && typeof mergeView.destroy === 'function') mergeView.destroy();
    mergeView = null;
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
        body: JSON.stringify({ content: snapshot, expected_current_version_id: file.current_version_id, change_kind: changeKindSelect.value || 'code_change' })
      });
      if (fileId !== targetFileId) throw new Error('File selection changed while saving; reload history before continuing.');
      file = result.file;
      savedContent = snapshot;
      const existing = versions.findIndex(version => version.version_id === result.version.version_id);
      if (existing < 0) versions.push(result.version);
      else versions[existing] = result.version;
      versions.sort((left, right) => left.version_number - right.version_number);
      selectedVersionID = result.version.version_id;
      setVersionOptions();
      renderHistoryTimeline();
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
    timeline.replaceChildren();
    destroyMergeView();
    diff.replaceChildren();
    previewRequest += 1;
    versionPreview.hidden = true;
    diff.hidden = true;
    setFeedback('Loading file history…');
    try {
      file = await api(`/files/${encodeURIComponent(fileId)}`);
      if (file.project_id && projectSelect.value !== file.project_id) {
        projectSelect.value = file.project_id;
        await Promise.all([loadProjectRoot(file.project_id), loadProjectFiles(file.project_id)]);
      }
      const versionResponse = await api(`/files/${encodeURIComponent(fileId)}/versions`);
      versions = versionResponse.items || [];
      const current = versions.find(version => version.version_id === file.current_version_id) || versions[versions.length - 1];
      if (!current) throw new Error('No versions exist for this file.');
      selectedVersionID = current.version_id;
      const content = await api(`/files/${encodeURIComponent(fileId)}/versions/${encodeURIComponent(current.version_id)}`);
      savedContent = content.content || '';
      mountEditor(savedContent);
      updateFileMeta();
      status.textContent = 'History connected · Auto-save on';
      restoreButton.disabled = false;
      compareButton.disabled = versions.length < 2;
      setVersionOptions();
      renderHistoryTimeline();
      renderFiles();
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
      destroyMergeView();
      diff.replaceChildren();
      const header = document.createElement('div');
      header.className = 'file-history-diff-header';
      header.textContent = `FROM V${result.from.version_number} (${result.from.version_id}) → TO V${result.to.version_number} (${result.to.version_id})`;
      const viewHost = document.createElement('div');
      viewHost.className = 'file-history-merge-view';
      viewHost.setAttribute('role', 'group');
      viewHost.setAttribute('aria-label', 'Read-only side-by-side version comparison');
      diff.append(header, viewHost);
      diff.hidden = false;
      if (window.TimeKeeperCodeMirror && typeof window.TimeKeeperCodeMirror.mountMerge === 'function') {
        mergeView = window.TimeKeeperCodeMirror.mountMerge(viewHost, {
          original: result.from_content || '',
          modified: result.to_content || ''
        });
      } else {
        const output = [];
        const hunks = result.hunks || [];
        if (!hunks.length) output.push('No textual changes.');
        for (const hunk of hunks) {
          output.push(`@@ -${hunk.from_start},${hunk.from_count} +${hunk.to_start},${hunk.to_count} @@`);
          for (const line of hunk.removed || []) output.push(`-${line}`);
          for (const line of hunk.added || []) output.push(`+${line}`);
        }
        const fallbackDiff = document.createElement('pre');
        fallbackDiff.className = 'file-history-diff-fallback';
        fallbackDiff.textContent = output.join('\n');
        viewHost.appendChild(fallbackDiff);
      }
    } catch (error) {
      setFeedback(error.message, true);
    }
  }

  loadForm?.addEventListener('submit', event => { event.preventDefault(); return loadFile(); });
  projectSelect?.addEventListener('change', () => {
    void loadProjectRoot(projectSelect.value);
    void loadProjectFiles(projectSelect.value);
  });
  setProjectRootButton?.addEventListener('click', setProjectRoot);
  scanProjectButton?.addEventListener('click', scanProject);
  fileFilter?.addEventListener('input', renderFiles);
  saveButton?.addEventListener('click', saveFile);
  restoreButton?.addEventListener('click', restoreFile);
  compareButton?.addEventListener('click', compareFiles);
  window.addEventListener('beforeunload', event => {
    if (editor && (editor.getValue() !== savedContent || queuedSaves > 0 || autoSaveTimer !== null)) {
      event.preventDefault();
      event.returnValue = '';
    }
  });
  renderChangeKindOptions();
  renderChangeKindLegend();
  void loadProjects();
})();

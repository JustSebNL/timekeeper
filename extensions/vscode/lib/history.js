// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

function historyFileChoices(files) {
  return (Array.isArray(files) ? files : [])
    .filter(file => file && file.file_id)
    .slice()
    .sort((left, right) => String(left.current_path || left.file_id).localeCompare(String(right.current_path || right.file_id)))
    .map(file => ({
      label: file.current_path || file.file_id,
      description: file.file_id,
      detail: `${file.file_type || 'text'} · current ${file.current_version_id || 'unversioned'}`,
      fileID: file.file_id,
      file,
    }));
}

function historyVersionChoices(file, versions) {
  const byID = new Map((Array.isArray(versions) ? versions : []).map(version => [version.version_id, version]));
  return [...byID.values()]
    .sort((left, right) => right.version_number - left.version_number || String(right.version_id).localeCompare(String(left.version_id)))
    .map(version => {
      const current = version.version_id === file?.current_version_id;
      const source = version.restores_version_id ? byID.get(version.restores_version_id) : undefined;
      const branch = source ? `restores V${source.version_number}` : version.restores_version_id ? `restores ${version.restores_version_id}` : '';
      const created = version.created_at || 'time unknown';
      return {
        label: `V${version.version_number}${current ? ' · current' : ''} · ${version.change_kind || 'change'}`,
        description: [created, branch].filter(Boolean).join(' · '),
        detail: version.change_summary || version.content_hash || 'Saved Core history version',
        version,
      };
    });
}

function createHistoryDocumentProvider(vscode) {
  const contents = new Map();
  const emitter = new vscode.EventEmitter();
  return {
    onDidChange: emitter.event,
    provideTextDocumentContent(uri) {
      return contents.get(uri.toString()) || '';
    },
    open(file, version, content) {
      const path = String(file.current_path || file.file_id || 'file').split(/[\\/]/).pop();
      const uri = vscode.Uri.parse(`timekeeper-history://file/${encodeURIComponent(file.file_id)}/${encodeURIComponent(version.version_id)}/${encodeURIComponent(path)}`);
      contents.set(uri.toString(), String(content ?? ''));
      emitter.fire(uri);
      return uri;
    },
    dispose() {
      contents.clear();
      emitter.dispose();
    },
  };
}

module.exports = { createHistoryDocumentProvider, historyFileChoices, historyVersionChoices };

// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

const { executionNodes } = require('./tree');

function escapeHTML(value) {
  return String(value ?? '').replace(/[&<>"']/g, character => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }[character]));
}

function summarizeProject(tree) {
  const nodes = executionNodes(tree);
  const project = tree?.project || {};
  const sprintStatuses = {};
  for (const node of nodes.filter(node => node.kind === 'sprint')) {
    const status = node.status || 'Unknown';
    sprintStatuses[status] = (sprintStatuses[status] || 0) + 1;
  }
  return {
    projectID: project.project_id || '',
    name: project.name || project.project_name || project.project_id || 'Selected Project',
    status: project.status || 'Open',
    goal: project.goal || project.project_goal || '',
    description: project.description || project.project_description || '',
    progress: typeof project.calculated_completion_pct === 'number' ? project.calculated_completion_pct : null,
    categories: nodes.filter(node => node.kind === 'category').length,
    tasks: nodes.filter(node => node.kind === 'task').length,
    subtasks: nodes.filter(node => node.kind === 'subtask').length,
    sprints: nodes.filter(node => node.kind === 'sprint').length,
    sprintStatuses,
  };
}

function renderProjectContext(summary, harnessPaths) {
  const progress = summary.progress === null ? 'not reported' : `${summary.progress.toFixed(1)}%`;
  const statuses = Object.entries(summary.sprintStatuses).sort(([left], [right]) => left.localeCompare(right));
  const statusMarkup = statuses.length === 0
    ? '<p>No Sprints are recorded in the current execution tree.</p>'
    : `<ul>${statuses.map(([status, count]) => `<li>${escapeHTML(status)}: ${count}</li>`).join('')}</ul>`;
  const fileMarkup = harnessPaths.length === 0
    ? '<p>No AGENTS.md, harness, .devin, or .agents Skill files were found in this workspace.</p>'
    : `<ul>${harnessPaths.map(path => `<li><code>${escapeHTML(path)}</code></li>`).join('')}</ul>`;

  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">
<style>
body { font-family: var(--vscode-font-family); padding: 1.25rem; }
dl { display: grid; grid-template-columns: max-content 1fr; gap: 0.35rem 1rem; }
dt { font-weight: 600; }
dd { margin: 0; }
section { margin-block-end: 1.5rem; }
code { font-family: var(--vscode-editor-font-family); }
.note { opacity: 0.8; }
</style>
<title>Time Keeper Project Context</title>
</head>
<body>
<h1>Time Keeper Project Context</h1>
<section>
<h2>Project authority</h2>
<dl>
<dt>Project</dt><dd>${escapeHTML(summary.name)}</dd>
<dt>ID</dt><dd><code>${escapeHTML(summary.projectID)}</code></dd>
<dt>Status</dt><dd>${escapeHTML(summary.status)}</dd>
<dt>Progress</dt><dd>${escapeHTML(progress)}</dd>
</dl>
<h3>Goal</h3>
<p>${escapeHTML(summary.goal || 'No project goal is recorded.')}</p>
<h3>Description</h3>
<p>${escapeHTML(summary.description || 'No project description is recorded.')}</p>
</section>
<section>
<h2>Execution tree</h2>
<ul>
<li>Categories: ${summary.categories}</li>
<li>Tasks: ${summary.tasks}</li>
<li>Subtasks: ${summary.subtasks}</li>
<li>Sprints: ${summary.sprints}</li>
</ul>
${statusMarkup}
</section>
<section>
<h2>Workspace harness and Skill files</h2>
${fileMarkup}
</section>
<p class="note">This read-only panel opens only after the Time Keeper: Show Project Context command. It lists workspace instruction paths but does not read or upload their contents.</p>
</body>
</html>`;
}

module.exports = { escapeHTML, summarizeProject, renderProjectContext };

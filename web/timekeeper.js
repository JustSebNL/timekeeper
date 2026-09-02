// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

(() => {
  'use strict';

  const api = '/api/v1/projects';
  const projects = document.querySelector('#projects');
  const pulseTarget = document.querySelector('#pulse');
  const guardianTarget = document.querySelector('#guardian');
  const activityTarget = document.querySelector('#activity');
  const message = document.querySelector('#message');
  const connection = document.querySelector('#connection');
  const sidebarAPIStatus = document.querySelector('#sidebar-api-status');
  const projectFilter = document.querySelector('#project-filter');
  const refreshDashboard = document.querySelector('#refresh-dashboard');
  const statProjects = document.querySelector('#stat-projects');
  const statOpen = document.querySelector('#stat-open');
  const statActive = document.querySelector('#stat-active');
  const statAttention = document.querySelector('#stat-attention');
  const navProjectCount = document.querySelector('#nav-project-count');
  const navAttentionCount = document.querySelector('#nav-attention-count');
  const form = document.querySelector('#project-form');
  const submit = document.querySelector('#submit');
  const agentID = localStorage.getItem('timekeeper_agent_id') || crypto.randomUUID();
  localStorage.setItem('timekeeper_agent_id', agentID);

  async function request(url, options = {}) {
    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        'X-Agent-ID': agentID,
        'X-Agent-Name': 'Time Keeper UI',
        'X-Agent-Type': 'human',
        ...(options.headers || {})
      }
    });
    const data = await response.json().catch(() => null);
    if (!response.ok) throw new Error(data?.error?.message || 'Request failed (' + response.status + ')');
    return data;
  }

  function label(item) {
    const progress = Number.isFinite(item.calculated_completion_pct) ? item.calculated_completion_pct : item.progress_pct;
    const suffix = Number.isFinite(progress) ? ' · ' + progress + '% complete' : '';
    return item.item_address + ' · ' + item.name + suffix;
  }

  function durationToSeconds(value) {
    const match = /^\s*(?:(\d+)h)?(?:(\d+)m)?\s*$/.exec(value);
    if (!match || (!match[1] && !match[2])) return 0;
    return ((Number(match[1] || 0) * 60) + Number(match[2] || 0)) * 60;
  }

  function createInlineForm(options) {
    const f = document.createElement('form');
    f.className = 'inline-form';
    const name = document.createElement('input');
    name.required = true;
    name.maxLength = 200;
    name.placeholder = options.namePlaceholder;
    name.setAttribute('aria-label', options.namePlaceholder);
    f.append(name);
    let estimate;
    if (options.estimate) {
      estimate = document.createElement('input');
      estimate.required = true;
      estimate.className = 'estimate';
      estimate.placeholder = '30m';
      estimate.setAttribute('aria-label', 'Estimate');
      f.append(estimate);
    }
    let buffer;
    if (options.buffer) {
      buffer = document.createElement('input');
      buffer.type = 'number';
      buffer.min = '0';
      buffer.max = '100';
      buffer.step = '1';
      buffer.value = '0';
      buffer.className = 'buffer';
      buffer.setAttribute('aria-label', 'Buffer percent');
      buffer.title = 'Contingency added to this Sprint estimate';
      f.append(buffer);
    }
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = options.submitLabel;
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    f.append(submitBtn, feedback);
    f.addEventListener('submit', async event => {
      event.preventDefault();
      const itemName = name.value.trim();
      const seconds = estimate ? durationToSeconds(estimate.value) : 0;
      const bufferPct = buffer ? Number(buffer.value) : 0;
      if (estimate && seconds <= 0) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = 'Estimate must be positive, such as 30m, 2h, or 1h30m.';
        return;
      }
      if (buffer && (!Number.isInteger(bufferPct) || bufferPct < 0 || bufferPct > 100)) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = 'Buffer percent must be a whole number from 0 to 100.';
        return;
      }
      submitBtn.disabled = true;
      feedback.className = 'inline-feedback';
      feedback.textContent = 'Saving…';
      try {
        await request(options.endpoint, { method: 'POST', body: JSON.stringify(options.payload(itemName, seconds, bufferPct)) });
        await options.refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    return f;
  }

  function toggleIcon(icon, collapsed) {
    icon.textContent = collapsed ? '▶' : '▼';
    icon.classList.toggle('collapsed', collapsed);
  }

  function createCollapsible(targetBody, headerEl, icon) {
    const update = () => {
      const collapsed = targetBody.classList.contains('collapsed');
      toggleIcon(icon, collapsed);
    };
    headerEl.addEventListener('click', () => {
      targetBody.classList.toggle('collapsed');
      update();
    });
    update();
  }

  function sprintActionButton(sprint, action, refresh) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'secondary';
    button.textContent = action === 'hold' ? 'Place On Hold' : action;
    button.addEventListener('click', async () => {
      let reason = '';
      if (action === 'hold' || action === 'cancel') {
        const verb = action === 'hold' ? 'On Hold' : 'cancelled';
        const supplied = window.prompt('Why is this Sprint ' + verb + '? This can be any blocker: dependency, decision, user input, or another constraint.', action === 'hold' ? (sprint.hold_reason || '') : '');
        if (supplied === null) return;
        reason = supplied.trim();
        if (!reason) {
          message.className = 'error';
          message.textContent = action === 'hold' ? 'Placing a Sprint On Hold requires a reason.' : 'Cancelling a Sprint requires a reason.';
          return;
        }
      }
      button.disabled = true;
      try {
        await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/' + action, { method: 'POST', body: JSON.stringify({reason}) });
        await refresh();
      } catch (error) {
        message.className = 'error';
        message.textContent = error.message;
      } finally {
        button.disabled = false;
      }
    });
    return button;
  }

  function sprintHoldReasonButton(sprint, refresh) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'secondary';
    button.textContent = 'Update hold reason';
    button.addEventListener('click', async () => {
      const supplied = window.prompt('Why is this Sprint still On Hold?', sprint.hold_reason || '');
      if (supplied === null) return;
      const reason = supplied.trim();
      if (!reason) { message.className = 'error'; message.textContent = 'An On Hold Sprint needs a reason.'; return; }
      button.disabled = true;
      try {
        await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/hold-reason', { method: 'POST', body: JSON.stringify({reason}) });
        await refresh();
      } catch (error) {
        message.className = 'error';
        message.textContent = error.message;
      } finally {
        button.disabled = false;
      }
    });
    return button;
  }

  function sprintExtensionPanel(sprint, refresh) {
    const panel = document.createElement('div');
    panel.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const summary = document.createElement('span');
    summary.textContent = 'Extend Sprint';
    header.append(icon, summary);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    const duration = document.createElement('input');
    duration.required = true;
    duration.placeholder = 'Additional minutes';
    duration.type = 'number';
    duration.min = '1';
    duration.step = '1';
    duration.setAttribute('aria-label', 'Additional minutes');
    const reason = document.createElement('textarea');
    reason.required = true;
    reason.maxLength = 10000;
    reason.placeholder = 'Extension reason';
    reason.setAttribute('aria-label', 'Extension reason');
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = 'Record extension';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    const history = document.createElement('ul');
    history.className = 'notes-list';
    async function loadHistory() {
      const response = await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/extensions');
      history.replaceChildren();
      for (const extension of response.items || []) {
        const item = document.createElement('li');
        item.className = 'note-item';
        item.textContent = '+' + extension.duration_seconds + 's · ' + extension.reason;
        history.append(item);
      }
    }
    const form = document.createElement('form');
    form.append(duration, reason, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      submitBtn.disabled = true;
      try {
        await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/extensions', { method: 'POST', body: JSON.stringify({duration_seconds: Number(duration.value) * 60, reason: reason.value.trim()}) });
        await loadHistory();
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    body.append(form, history);
    panel.append(header, body);
    createCollapsible(body, header, icon);
    header.addEventListener('click', () => {
      if (!body.classList.contains('collapsed')) {
        loadHistory().catch(error => {
          feedback.className = 'inline-feedback error';
          feedback.textContent = error.message;
        });
      }
    });
    return panel;
  }

  function sprintRetrievalAttemptPanel(sprint, refresh) {
    const panel = document.createElement('div');
    panel.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const summary = document.createElement('span');
    summary.textContent = 'Retrieval attempts';
    header.append(icon, summary);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    const reason = document.createElement('textarea');
    reason.required = true;
    reason.maxLength = 10000;
    reason.placeholder = 'Why this retrieval attempt did not produce material progress';
    reason.setAttribute('aria-label', 'Retrieval attempt reason');
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = 'Record attempt';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    const history = document.createElement('ul');
    history.className = 'notes-list';
    async function loadHistory() {
      const response = await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/retrieval-attempts');
      history.replaceChildren();
      for (const attempt of response.items || []) {
        const item = document.createElement('li');
        item.className = 'note-item';
        item.textContent = 'Attempt ' + attempt.attempt_number + '/4 · ' + attempt.reason + (attempt.timed_out ? ' · TimedOut' : '');
        history.append(item);
      }
    }
    const form = document.createElement('form');
    form.append(reason, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      submitBtn.disabled = true;
      try {
        const result = await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/retrieval-attempts', { method: 'POST', body: JSON.stringify({reason: reason.value.trim()}) });
        reason.value = '';
        await loadHistory();
        if (result.timed_out) feedback.textContent = 'Fourth attempt recorded. This Sprint is now TimedOut.';
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    body.append(form, history);
    panel.append(header, body);
    createCollapsible(body, header, icon);
    header.addEventListener('click', () => {
      if (!body.classList.contains('collapsed')) {
        loadHistory().catch(error => {
          feedback.className = 'inline-feedback error';
          feedback.textContent = error.message;
        });
      }
    });
    return panel;
  }

  function sprintTimeEntryPanel(sprint) {
    const panel = document.createElement('div');
    panel.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const summary = document.createElement('span');
    summary.textContent = 'Recorded intervals';
    header.append(icon, summary);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    const history = document.createElement('ul');
    history.className = 'notes-list';
    async function loadHistory() {
      const response = await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/time-entries');
      history.replaceChildren();
      for (const entry of response.items || []) {
        const item = document.createElement('li');
        item.className = 'note-item';
        item.textContent = entry.entry_type + ' · ' + entry.duration_seconds + 's' + (entry.reason ? ' · ' + entry.reason : '');
        history.append(item);
      }
    }
    body.append(feedback, history);
    panel.append(header, body);
    createCollapsible(body, header, icon);
    header.addEventListener('click', () => {
      if (!body.classList.contains('collapsed')) {
        loadHistory().catch(error => {
          feedback.className = 'inline-feedback error';
          feedback.textContent = error.message;
        });
      }
    });
    return panel;
  }

  function renderSprintItem(sprint, prefix, refresh) {
    const item = document.createElement('div');
    item.className = 'sprint-item';
    const name = document.createElement('span');
    name.className = 'sprint-name';
    name.textContent = prefix + ' ' + label(sprint) + ' · ' + sprint.status;
    if (sprint.status === 'On Hold' && sprint.hold_reason) {
      name.textContent += ' · On Hold: ' + sprint.hold_reason;
    }
    const status = document.createElement('span');
    status.className = 'sprint-status status-' + sprint.status.replace(' ', '\\ ');
    status.textContent = sprint.status;
    const actions = document.createElement('div');
    actions.className = 'sprint-actions';
    const actionsMap = { Open: ['start', 'hold', 'cancel'], Active: ['hold', 'complete', 'cancel'], 'On Hold': ['resume', 'cancel'] };
    for (const action of actionsMap[sprint.status] || []) {
      actions.append(sprintActionButton(sprint, action, refresh));
    }
    if (sprint.status === 'On Hold') {
      actions.append(sprintHoldReasonButton(sprint, refresh));
    }
    item.append(name, status, actions);
    return item;
  }

  function renderSprintList(entries, prefix, refresh) {
    const list = document.createElement('div');
    list.className = 'sprint-list';
    for (const sprint of entries) {
      list.append(renderSprintItem(sprint, prefix, refresh));
    }
    return list;
  }

  function renderProjectNotes(projectID, notes, refresh) {
    const section = document.createElement('div');
    section.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const summary = document.createElement('span');
    summary.textContent = 'Notes (' + notes.length + ')';
    header.append(icon, summary);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    const form = document.createElement('form');
    form.className = 'inline-form';
    const content = document.createElement('textarea');
    content.required = true;
    content.maxLength = 10000;
    content.placeholder = 'Record an observation, decision, or handoff…';
    content.setAttribute('aria-label', 'Project note');
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = 'Add note';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    form.append(content, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      const value = content.value.trim();
      if (!value) return;
      submitBtn.disabled = true;
      try {
        await request(api + '/' + encodeURIComponent(projectID) + '/notes', { method: 'POST', body: JSON.stringify({ content: value }) });
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    body.append(form);
    if (notes.length) {
      const list = document.createElement('div');
      list.className = 'notes-list';
      for (const note of notes) {
        const item = document.createElement('li');
        item.className = 'note-item';
        item.textContent = note.content + (note.actor_id ? ' · ' + note.actor_id : '');
        list.append(item);
      }
      body.append(list);
    }
    section.append(header, body);
    createCollapsible(body, header, icon);
    return section;
  }

  function renderProjectMetadata(project, refresh) {
    const section = document.createElement('div');
    section.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const summary = document.createElement('span');
    summary.textContent = 'Project context';
    header.append(icon, summary);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    const form = document.createElement('form');
    const goal = document.createElement('input');
    goal.value = project.project_goal || '';
    goal.maxLength = 1000;
    goal.placeholder = 'Project goal';
    goal.setAttribute('aria-label', 'Project goal');
    const description = document.createElement('textarea');
    description.value = project.project_description || '';
    description.maxLength = 10000;
    description.placeholder = 'Project description';
    description.setAttribute('aria-label', 'Project description');
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = 'Update context';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    form.append(goal, description, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      submitBtn.disabled = true;
      try {
        await request(api + '/' + encodeURIComponent(project.project_id) + '/metadata', { method: 'POST', body: JSON.stringify({ goal: goal.value, description: description.value }) });
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    body.append(form);
    section.append(header, body);
    createCollapsible(body, header, icon);
    return section;
  }

  function renderProjectStatus(project, refresh) {
    const form = document.createElement('form');
    form.className = 'inline-form';
    const label = document.createElement('label');
    label.textContent = 'Project status';
    const select = document.createElement('select');
    for (const status of ['Open', 'On Hold', 'Completed', 'Cancelled']) {
      const option = document.createElement('option');
      option.value = status;
      option.textContent = status;
      option.selected = project.status === status;
      select.append(option);
    }
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = 'Update status';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    form.append(label, select, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      submitBtn.disabled = true;
      try {
        await request(api + '/' + encodeURIComponent(project.project_id) + '/status', { method: 'POST', body: JSON.stringify({ status: select.value }) });
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    return form;
  }

  function renderTaskMetadata(task, refresh) {
    const section = document.createElement('div');
    section.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const summary = document.createElement('span');
    summary.textContent = 'Task context';
    header.append(icon, summary);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    const form = document.createElement('form');
    const goal = document.createElement('input');
    goal.value = task.goal || '';
    goal.maxLength = 1000;
    goal.placeholder = 'Task goal';
    goal.setAttribute('aria-label', 'Task goal');
    const description = document.createElement('textarea');
    description.value = task.description || '';
    description.maxLength = 10000;
    description.placeholder = 'Task description';
    description.setAttribute('aria-label', 'Task description');
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = 'Update context';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    form.append(goal, description, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      submitBtn.disabled = true;
      try {
        await request('/api/v1/tasks/' + encodeURIComponent(task.task_id) + '/metadata', { method: 'POST', body: JSON.stringify({ goal: goal.value, description: description.value }) });
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    body.append(form);
    section.append(header, body);
    createCollapsible(body, header, icon);
    return section;
  }

  function renderCategoryMetadata(category, refresh) {
    const section = document.createElement('div');
    section.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const summary = document.createElement('span');
    summary.textContent = 'Category context';
    header.append(icon, summary);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    const form = document.createElement('form');
    const goal = document.createElement('input');
    goal.value = category.goal || '';
    goal.maxLength = 1000;
    goal.placeholder = 'Category goal';
    goal.setAttribute('aria-label', 'Category goal');
    const description = document.createElement('textarea');
    description.value = category.description || '';
    description.maxLength = 10000;
    description.placeholder = 'Category description';
    description.setAttribute('aria-label', 'Category description');
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = 'Update context';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    form.append(goal, description, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      submitBtn.disabled = true;
      try {
        await request('/api/v1/categories/' + encodeURIComponent(category.category_id) + '/metadata', { method: 'POST', body: JSON.stringify({ goal: goal.value, description: description.value }) });
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    body.append(form);
    section.append(header, body);
    createCollapsible(body, header, icon);
    return section;
  }

  function renderTaskStatus(task, refresh) {
    const form = document.createElement('form');
    form.className = 'inline-form';
    const label = document.createElement('label');
    label.textContent = 'Task status';
    const select = document.createElement('select');
    for (const status of ['Open', 'On Hold', 'Completed', 'Cancelled']) {
      const option = document.createElement('option');
      option.value = status;
      option.textContent = status;
      option.selected = task.status === status;
      select.append(option);
    }
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = 'Update status';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    form.append(label, select, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      submitBtn.disabled = true;
      try {
        await request('/api/v1/tasks/' + encodeURIComponent(task.task_id) + '/status', { method: 'POST', body: JSON.stringify({ status: select.value }) });
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    return form;
  }

  function renderSubtaskStatus(subtask, refresh) {
    const form = document.createElement('form');
    form.className = 'inline-form';
    const label = document.createElement('label');
    label.textContent = 'Subtask status';
    const select = document.createElement('select');
    for (const status of ['Open', 'On Hold', 'Completed', 'Cancelled']) {
      const option = document.createElement('option');
      option.value = status;
      option.textContent = status;
      option.selected = subtask.status === status;
      select.append(option);
    }
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = 'Update status';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    form.append(label, select, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      submitBtn.disabled = true;
      try {
        await request('/api/v1/subtasks/' + encodeURIComponent(subtask.subtask_id) + '/status', { method: 'POST', body: JSON.stringify({ status: select.value }) });
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    return form;
  }

  function renderCategoryStatus(category, refresh) {
    const form = document.createElement('form');
    form.className = 'inline-form';
    const label = document.createElement('label');
    label.textContent = 'Category status';
    const select = document.createElement('select');
    for (const status of ['Open', 'On Hold', 'Completed', 'Cancelled']) {
      const option = document.createElement('option');
      option.value = status;
      option.textContent = status;
      option.selected = category.status === status;
      select.append(option);
    }
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = 'Update status';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    form.append(label, select, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      submitBtn.disabled = true;
      try {
        await request('/api/v1/categories/' + encodeURIComponent(category.category_id) + '/status', { method: 'POST', body: JSON.stringify({ status: select.value }) });
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    return form;
  }

  function renderExecutionTree(tree, summary, attention, notes, events, pipelines, drafts, refresh) {
    const root = document.createElement('div');
    root.className = 'tree';

    // Project header section
    const headerSection = document.createElement('div');
    headerSection.className = 'tree-section';
    headerSection.append(renderProjectStatus(tree.project, refresh));
    headerSection.append(renderProjectMetadata(tree.project, refresh));
    headerSection.append(renderOperationalSummary(summary));
    headerSection.append(renderProjectAttention(attention));
    headerSection.append(renderPlanningDrafts(tree.project.project_id, pipelines, drafts, refresh));
    headerSection.append(renderProjectEvents(events));
    headerSection.append(renderProjectNotes(tree.project.project_id, notes, refresh));
    root.append(headerSection);

    // Add category form
    root.append(createInlineForm({
      namePlaceholder: 'New category',
      submitLabel: 'Add category',
      estimate: false,
      endpoint: api + '/' + encodeURIComponent(tree.project.project_id) + '/categories',
      payload: name => ({ name }),
      refresh
    }));

    // Category nodes
    function renderCategoryNode(categoryNode) {
      const node = document.createElement('div');
      node.className = 'cat-node';

      const header = document.createElement('div');
      header.className = 'cat-node-header';
      const icon = document.createElement('span');
      icon.className = 'icon';
      const title = document.createElement('span');
      title.textContent = 'Category · ' + label(categoryNode.category);
      header.append(icon, title);

      const body = document.createElement('div');
      body.className = 'cat-node-body';

      body.append(renderCategoryStatus(categoryNode.category, refresh));
      body.append(renderCategoryMetadata(categoryNode.category, refresh));
      body.append(createInlineForm({
        namePlaceholder: 'New child category',
        submitLabel: 'Add child category',
        estimate: false,
        endpoint: api + '/' + encodeURIComponent(tree.project.project_id) + '/categories',
        payload: name => ({ name, parent_category_id: categoryNode.category.category_id }),
        refresh
      }));
      body.append(createInlineForm({
        namePlaceholder: 'New task',
        submitLabel: 'Add task',
        estimate: true,
        endpoint: api + '/' + encodeURIComponent(tree.project.project_id) + '/tasks',
        payload: (name, seconds) => ({ category_id: categoryNode.category.category_id, name, estimated_duration_seconds: seconds }),
        refresh
      }));

      // Tasks
      for (const taskNode of categoryNode.tasks || []) {
        body.append(renderTaskNode(taskNode, refresh));
      }

      // Child categories
      for (const child of categoryNode.categories || []) {
        body.append(renderCategoryNode(child));
      }

      node.append(header, body);
      createCollapsible(body, header, icon);
      return node;
    }

    function renderTaskNode(taskNode, refresh) {
      const node = document.createElement('div');
      node.className = 'task-node';

      const header = document.createElement('div');
      header.className = 'task-node-header';
      const icon = document.createElement('span');
      icon.className = 'icon';
      const title = document.createElement('span');
      title.textContent = 'Task · ' + label(taskNode.task);
      header.append(icon, title);

      const body = document.createElement('div');
      body.className = 'task-node-body';

      body.append(renderTaskStatus(taskNode.task, refresh));
      body.append(renderTaskMetadata(taskNode.task, refresh));
      body.append(createInlineForm({
        namePlaceholder: 'New direct Sprint',
        submitLabel: 'Add Sprint',
        estimate: true,
        buffer: true,
        endpoint: '/api/v1/tasks/' + encodeURIComponent(taskNode.task.task_id) + '/sprints',
        payload: (name, seconds, bufferPct) => ({ name, estimated_duration_seconds: seconds, buffer_pct: bufferPct }),
        refresh
      }));
      body.append(createInlineForm({
        namePlaceholder: 'New subtask',
        submitLabel: 'Add subtask',
        estimate: true,
        endpoint: '/api/v1/tasks/' + encodeURIComponent(taskNode.task.task_id) + '/subtasks',
        payload: (name, seconds) => ({ name, estimated_duration_seconds: seconds }),
        refresh
      }));

      // Direct sprints
      if (taskNode.sprints && taskNode.sprints.length) {
        body.append(renderSprintList(taskNode.sprints || [], 'Direct Sprint ·', refresh));
      }

      // Subtasks
      for (const subtaskNode of taskNode.subtasks || []) {
        body.append(renderSubtaskNode(subtaskNode, refresh));
      }

      node.append(header, body);
      createCollapsible(body, header, icon);
      return node;
    }

    function renderSubtaskNode(subtaskNode, refresh) {
      const node = document.createElement('div');
      node.className = 'subtask-node';

      const header = document.createElement('div');
      header.className = 'subtask-node-header';
      const icon = document.createElement('span');
      icon.className = 'icon';
      const title = document.createElement('span');
      title.textContent = 'Subtask · ' + label(subtaskNode.subtask);
      header.append(icon, title);

      const body = document.createElement('div');
      body.className = 'subtask-node-body';

      body.append(renderSubtaskStatus(subtaskNode.subtask, refresh));
      body.append(createInlineForm({
        namePlaceholder: 'New Sprint',
        submitLabel: 'Add Sprint',
        estimate: true,
        buffer: true,
        endpoint: '/api/v1/subtasks/' + encodeURIComponent(subtaskNode.subtask.subtask_id) + '/sprints',
        payload: (name, seconds, bufferPct) => ({ name, estimated_duration_seconds: seconds, buffer_pct: bufferPct }),
        refresh
      }));

      body.append(renderSprintList(subtaskNode.sprints || [], 'Sprint ·', refresh));

      node.append(header, body);
      createCollapsible(body, header, icon);
      return node;
    }

    for (const categoryNode of tree.categories || []) {
      root.append(renderCategoryNode(categoryNode));
    }

    return root;
  }

  async function loadExecutionTree(projectID, target, button) {
    button.disabled = true;
    target.textContent = 'Loading execution tree…';
    try {
      const [tree, summary, attention, noteResponse, eventResponse, pipelineResponse, draftResponse] = await Promise.all([
        request(api + '/' + encodeURIComponent(projectID) + '/execution-tree'),
        request(api + '/' + encodeURIComponent(projectID) + '/operational-summary'),
        request(api + '/' + encodeURIComponent(projectID) + '/attention'),
        request(api + '/' + encodeURIComponent(projectID) + '/notes'),
        request(api + '/' + encodeURIComponent(projectID) + '/events'),
        request('/api/v1/llm-pipelines'),
        request(api + '/' + encodeURIComponent(projectID) + '/planning-drafts')
      ]);
      target.replaceChildren(renderExecutionTree(tree, summary, attention.items || [], noteResponse.items || [], eventResponse.items || [], pipelineResponse.items || [], draftResponse.items || [], async () => { await loadExecutionTree(projectID, target, button); await loadPulse(); }));
    } catch (error) {
      target.textContent = error.message;
    } finally {
      button.disabled = false;
    }
  }

  function filterProjectCards() {
    const query = (projectFilter?.value || '').trim().toLowerCase();
    for (const card of projects.querySelectorAll('.project-card')) {
      card.hidden = Boolean(query) && !card.textContent.toLowerCase().includes(query);
    }
  }

  function render(items) {
    statProjects.textContent = items.length;
    statOpen.textContent = items.filter(item => item.status === 'Open' || item.status === 'Active').length;
    statActive.textContent = '—';
    navProjectCount.textContent = items.length;
    projects.replaceChildren();
    if (!items.length) {
      projects.textContent = 'No projects yet. Create the first one.';
      projects.className = 'empty';
      return;
    }
    projects.className = '';

    for (const item of items) {
      const card = document.createElement('div');
      card.className = 'project-card';

      const header = document.createElement('div');
      header.className = 'project-card-header';

      const info = document.createElement('div');
      info.className = 'project-card-info';
      const name = document.createElement('p');
      name.className = 'project-card-name';
      name.textContent = item.project_name;
      const meta = document.createElement('div');
      meta.className = 'project-card-meta';
      meta.textContent = item.project_id + ' · ' + item.item_address;
      info.append(name, meta);

      const status = document.createElement('span');
      status.className = 'project-card-status status-' + item.status.replace(' ', '\\ ');
      status.textContent = item.status;

      header.append(info, status);

      // Progress bar
      const progressBar = document.createElement('div');
      progressBar.className = 'project-card-progress';
      const progressFill = document.createElement('div');
      progressFill.className = 'project-card-progress-bar';
      progressFill.style.width = (Number(item.calculated_completion_pct || 0)).toFixed(1) + '%';
      progressBar.append(progressFill);

      // Actions
      const actions = document.createElement('div');
      actions.className = 'project-card-actions';
      const inspectBtn = document.createElement('button');
      inspectBtn.type = 'button';
      inspectBtn.className = 'secondary';
      inspectBtn.textContent = 'Inspect hierarchy';
      actions.append(inspectBtn);

      card.append(header, progressBar, actions);

      // Tree container
      const tree = document.createElement('div');
      tree.className = 'tree';
      tree.style.display = 'none';
      card.append(tree);

      inspectBtn.addEventListener('click', () => {
        const isHidden = tree.style.display === 'none';
        tree.style.display = isHidden ? 'flex' : 'none';
        inspectBtn.textContent = isHidden ? 'Hide hierarchy' : 'Inspect hierarchy';
        if (isHidden) {
          loadExecutionTree(item.project_id, tree, inspectBtn);
        }
      });

      projects.append(card);
    }
    filterProjectCards();
  }

  function renderPulse(snapshot) {
    const attention = Array.isArray(snapshot.attention) ? snapshot.attention : [];
    if (Number.isFinite(snapshot.active_sprints)) statActive.textContent = snapshot.active_sprints;
    statAttention.textContent = attention.length;
    navAttentionCount.textContent = attention.length;
    pulseTarget.replaceChildren();
    if (!attention.length) {
      pulseTarget.className = 'empty';
      pulseTarget.textContent = 'Clear. No active Sprint has exceeded its declared plan.';
      return;
    }
    pulseTarget.className = 'pulse-attention';
    const list = document.createElement('ul');
    for (const item of attention) {
      const row = document.createElement('li');
      row.textContent = item.name + ' · ' + item.sprint_id + ' · ' + item.overdue_duration_seconds + 's overdue (' + item.active_duration_seconds + 's active / ' + item.planned_duration_seconds + 's planned)';
      list.append(row);
    }
    const next = document.createElement('p');
    next.className = 'status';
    next.textContent = 'Suggested next local check: ' + snapshot.recommended_next_pulse_seconds + 's.';
    pulseTarget.append(list, next);
  }

  async function loadGuardianStatus() {
    try {
      const status = await request('/api/v1/guardian/status');
      guardianTarget.replaceChildren();
      if (!status.pulse_guardian_enabled) {
        const note = document.createElement('div');
        note.className = 'empty';
        note.textContent = 'Disabled. Start with TIMEKEEPER_PULSE_GUARDIAN_INTERVAL=5m to evaluate registered local recovery leases.';
        guardianTarget.append(note);
        return;
      }
      const enabled = document.createElement('div');
      enabled.className = 'status';
      enabled.textContent = 'Enabled · checks registered progress leases every ' + status.pulse_guardian_interval_seconds + 's.';
      guardianTarget.append(enabled);

      const policy = document.createElement('div');
      policy.className = 'recovery-policy';
      policy.textContent = 'Recovery policy: ' + (status.recovery_policy || 'unspecified');
      guardianTarget.append(policy);

      const callbacks = status.registered_callbacks || [];
      const cbLabel = document.createElement('div');
      cbLabel.className = 'recovery-callbacks';
      if (callbacks.length === 0) {
        cbLabel.className = 'empty warning';
        cbLabel.textContent = 'No local recovery callback registered. Detection runs, but no recovery action will be taken.';
      } else {
        cbLabel.textContent = 'Registered recovery callbacks:';
        const list = document.createElement('ul');
        for (const cb of callbacks) {
          const item = document.createElement('li');
          item.textContent = cb.agent_id + ' → ' + cb.guardian_url;
          list.append(item);
        }
        cbLabel.append(list);
      }
      guardianTarget.append(cbLabel);
    } catch (error) {
      guardianTarget.className = 'empty error';
      guardianTarget.textContent = error.message;
    }
  }

  async function loadPulse() {
    try {
      renderPulse(await request('/api/v1/pulse'));
    } catch (error) {
      pulseTarget.className = 'empty error';
      pulseTarget.textContent = error.message;
    }
  }

  function formatActivityTime(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return 'just now';
    return date.toLocaleString([], {month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'});
  }

  function renderRecentActivity(items) {
    activityTarget.replaceChildren();
    if (!items.length) {
      activityTarget.className = 'activity-list empty';
      activityTarget.textContent = 'No recorded activity yet.';
      return;
    }
    activityTarget.className = 'activity-list';
    for (const entry of items.slice(0, 8)) {
      const row = document.createElement('div');
      row.className = 'activity-item';
      const dot = document.createElement('span');
      dot.className = 'activity-dot';
      dot.setAttribute('aria-hidden', 'true');
      const copy = document.createElement('div');
      copy.className = 'activity-copy';
      const project = document.createElement('div');
      project.className = 'activity-project';
      project.textContent = entry.projectName;
      const messageText = document.createElement('div');
      messageText.className = 'activity-message';
      messageText.textContent = entry.event.message || entry.event.event_type || 'Project activity recorded';
      const eventType = document.createElement('span');
      eventType.className = 'activity-type';
      eventType.textContent = (entry.event.event_type || 'project event').split('_').join(' ');
      const timestamp = document.createElement('time');
      timestamp.className = 'activity-time';
      timestamp.dateTime = entry.event.created_at || '';
      timestamp.textContent = formatActivityTime(entry.event.created_at);
      copy.append(project, eventType, messageText);
      row.append(dot, copy, timestamp);
      activityTarget.append(row);
    }
  }

  async function loadRecentActivity(items) {
    const responses = await Promise.all(items.map(item => request(api + '/' + encodeURIComponent(item.project_id) + '/events').catch(() => ({items: []}))));
    const activity = [];
    for (let index = 0; index < responses.length; index += 1) {
      for (const event of responses[index].items || []) {
        activity.push({event, projectName: items[index].project_name || items[index].project_id});
      }
    }
    activity.sort((left, right) => new Date(right.event.created_at).getTime() - new Date(left.event.created_at).getTime());
    renderRecentActivity(activity);
  }

  async function load() {
    void loadPulse();
    void loadGuardianStatus();
    try {
      const data = await request(api);
      const items = data.items || [];
      render(items);
      const [summaries] = await Promise.all([
        Promise.all(items.map(item => request(api + '/' + encodeURIComponent(item.project_id) + '/operational-summary').catch(() => null))),
        loadRecentActivity(items)
      ]);
      statActive.textContent = summaries.reduce((total, summary) => total + (summary?.active_sprints || 0), 0);
      connection.textContent = 'Local API connected';
      sidebarAPIStatus.textContent = 'Online';
      sidebarAPIStatus.style.color = 'var(--green)';
    } catch (error) {
      projects.textContent = error.message;
      projects.className = 'empty';
      activityTarget.className = 'activity-list empty error';
      activityTarget.textContent = 'Activity unavailable.';
      connection.textContent = 'Local API unavailable';
      sidebarAPIStatus.textContent = 'Offline';
      sidebarAPIStatus.style.color = 'var(--red)';
    }
  }

  form.addEventListener('submit', async event => {
    event.preventDefault();
    submit.disabled = true;
    message.className = '';
    message.textContent = 'Creating…';
    const fields = new FormData(form);
    try {
      await request(api, { method: 'POST', body: JSON.stringify({ name: fields.get('name'), goal: fields.get('goal'), description: fields.get('description') }) });
      form.reset();
      message.textContent = 'Project created.';
      await load();
    } catch (error) {
      message.className = 'error';
      message.textContent = error.message;
    } finally {
      submit.disabled = false;
    }
  });

  projectFilter?.addEventListener('input', filterProjectCards);
  document.querySelectorAll('.nav-item[href^="#"]').forEach(item => {
    item.addEventListener('click', () => {
      document.querySelectorAll('.nav-item.active').forEach(active => active.classList.remove('active'));
      item.classList.add('active');
    });
  });
  document.addEventListener('keydown', event => {
    if (event.key === '/' && !['INPUT', 'TEXTAREA', 'SELECT'].includes(document.activeElement?.tagName)) {
      event.preventDefault();
      projectFilter?.focus();
    }
  });
  refreshDashboard?.addEventListener('click', () => {
    refreshDashboard.disabled = true;
    Promise.resolve(load()).finally(() => { refreshDashboard.disabled = false; });
  });
  load();

  function renderProjectAttention(items) {
    const section = document.createElement('div');
    section.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const title = document.createElement('span');
    title.textContent = 'Attention beyond Pulse';
    header.append(icon, title);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    if (!items.length) {
      const clear = document.createElement('div');
      clear.className = 'empty';
      clear.textContent = 'Clear. No held, TimedOut, or stranded Open Sprint needs a decision.';
      body.append(clear);
    } else {
      const list = document.createElement('ul');
      for (const item of items) {
        const row = document.createElement('li');
        row.textContent = item.kind + ' · ' + item.name + ' · ' + item.sprint_id + (item.hold_reason ? ' · ' + item.hold_reason : '') + ' · ' + item.detail;
        list.append(row);
      }
      body.append(list);
    }
    section.append(header, body);
    createCollapsible(body, header, icon);
    return section;
  }

  function renderOperationalSummary(summary) {
    const section = document.createElement('div');
    section.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const title = document.createElement('span');
    title.textContent = 'Operational snapshot';
    header.append(icon, title);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    const detail = document.createElement('div');
    detail.textContent = summary.total_sprints + ' Sprint(s) · ' + summary.active_sprints + ' active · ' + summary.held_sprints + ' on hold · ' + summary.timed_out_sprints + ' TimedOut · ' + summary.cancelled_sprints + ' cancelled · ' + summary.estimated_duration_seconds + 's estimate + ' + summary.buffer_duration_seconds + 's buffer + ' + summary.extension_duration_seconds + 's extensions = ' + summary.planned_duration_seconds + 's planned · ' + summary.recorded_work_seconds + 's recorded work · ' + summary.recorded_hold_seconds + 's recorded hold';
    body.append(detail);
    section.append(header, body);
    createCollapsible(body, header, icon);
    return section;
  }

  function renderProjectEvents(events) {
    const section = document.createElement('div');
    section.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const summary = document.createElement('span');
    summary.textContent = 'Activity (' + events.length + ')';
    header.append(icon, summary);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    const list = document.createElement('div');
    list.className = 'events-list';
    for (const event of events) {
      const item = document.createElement('div');
      item.className = 'event-item';
      item.textContent = event.message;
      list.append(item);
    }
    body.append(list);
    section.append(header, body);
    createCollapsible(body, header, icon);
    return section;
  }

  function renderLocalPipelineForm(refresh, headingText, collapsed) {
    const container = document.createElement(collapsed ? 'div' : 'div');
    container.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const heading = document.createElement('span');
    heading.textContent = headingText;
    header.append(icon, heading);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    const form = document.createElement('form');
    const name = document.createElement('input');
    name.required = true;
    name.placeholder = 'Planner name';
    const provider = document.createElement('select');
    for (const value of ['ollama', 'openai-compatible']) {
      const option = document.createElement('option');
      option.value = value;
      option.textContent = value;
      provider.append(option);
    }
    const baseURL = document.createElement('input');
    baseURL.required = true;
    baseURL.placeholder = 'http://127.0.0.1:11434';
    const model = document.createElement('input');
    model.required = true;
    model.placeholder = 'Model name';
    const systemPrompt = document.createElement('textarea');
    systemPrompt.maxLength = 10000;
    systemPrompt.placeholder = 'Optional planning instructions';
    systemPrompt.setAttribute('aria-label', 'Optional planning instructions');
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.textContent = 'Save local planner';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    form.append(name, provider, baseURL, model, systemPrompt, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      submitBtn.disabled = true;
      try {
        await request('/api/v1/llm-pipelines', { method: 'POST', body: JSON.stringify({name: name.value.trim(), provider: provider.value, base_url: baseURL.value.trim(), model: model.value.trim(), system_prompt: systemPrompt.value.trim()}) });
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    body.append(form);
    container.append(header, body);
    createCollapsible(body, header, icon);
    return container;
  }

  function renderPlanningDrafts(projectID, pipelines, drafts, refresh) {
    const section = document.createElement('div');
    section.className = 'collapsible';
    const header = document.createElement('div');
    header.className = 'collapsible-header';
    const icon = document.createElement('span');
    icon.className = 'icon';
    const summary = document.createElement('span');
    summary.textContent = 'Local planning drafts (' + drafts.length + ')';
    header.append(icon, summary);
    const body = document.createElement('div');
    body.className = 'collapsible-body collapsed';
    if (!pipelines.length) {
      const msg = document.createElement('p');
      msg.textContent = 'No local planning pipeline is configured.';
      body.append(msg);
      body.append(renderLocalPipelineForm(refresh, 'Configure local planner', false));
    } else {
      const select = document.createElement('select');
      for (const pipeline of pipelines) {
        const option = document.createElement('option');
        option.value = String(pipeline.pipeline_id);
        option.textContent = pipeline.name + ' · ' + pipeline.provider + ' · ' + pipeline.model;
        select.append(option);
      }
      const generate = document.createElement('button');
      generate.type = 'button';
      generate.textContent = 'Generate plan';
      generate.addEventListener('click', async () => {
        generate.disabled = true;
        try {
          await request(api + '/' + encodeURIComponent(projectID) + '/planning-drafts/generate', { method: 'POST', body: JSON.stringify({pipeline_id: Number(select.value)}) });
          await refresh();
        } finally {
          generate.disabled = false;
        }
      });
      body.append(select, generate);
      body.append(renderLocalPipelineForm(refresh, 'Add another local planner', true));
      for (const draft of drafts) {
        const item = document.createElement('div');
        item.className = 'collapsible';
        const itemHeader = document.createElement('div');
        itemHeader.className = 'collapsible-header';
        const itemIcon = document.createElement('span');
        itemIcon.className = 'icon';
        const itemSummary = document.createElement('span');
        itemSummary.textContent = 'Draft ' + draft.draft_id + ' · ' + draft.status + ' · ' + draft.summary;
        itemHeader.append(itemIcon, itemSummary);
        const itemBody = document.createElement('div');
        itemBody.className = 'collapsible-body collapsed';
        const raw = document.createElement('pre');
        raw.textContent = draft.raw_json;
        itemBody.append(raw);
        if (draft.status === 'Review') {
          const apply = document.createElement('button');
          apply.type = 'button';
          apply.textContent = 'Apply approved draft';
          apply.addEventListener('click', async () => {
            if (!window.confirm('Apply this reviewed planning draft?')) return;
            await request(api + '/' + encodeURIComponent(projectID) + '/planning-drafts/' + encodeURIComponent(draft.draft_id) + '/apply', { method: 'POST', body: '{}' });
            await refresh();
          });
          itemBody.append(apply);
        }
        item.append(itemHeader, itemBody);
        createCollapsible(itemBody, itemHeader, itemIcon);
        body.append(item);
      }
    }
    section.append(header, body);
    createCollapsible(body, header, icon);
    return section;
  }

})();

// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

(() => {
  'use strict';

  const api = '/api/v1/projects';
  const projects = document.querySelector('#projects');
  const deadlineWatch = document.querySelector('#deadline-watch');
  const pulseTarget = document.querySelector('#pulse');
  const guardianTarget = document.querySelector('#guardian');
  const usageCoverage = document.querySelector('#usage-coverage');
  const usageTokens = document.querySelector('#usage-tokens');
  const usageSessions = document.querySelector('#usage-sessions');
  const usageInput = document.querySelector('#usage-input');
  const usageOutput = document.querySelector('#usage-output');
  const usageCost = document.querySelector('#usage-cost');
  const usageBreakdownBar = document.querySelector('#usage-breakdown-bar');
  const usageLegend = document.querySelector('#usage-legend');
  const usageAgents = document.querySelector('#usage-agents');
  const usageModels = document.querySelector('#usage-models');
  const usageProjects = document.querySelector('#usage-projects');
  const usageTimeline = document.querySelector('#usage-timeline');
  const usageSessionList = document.querySelector('#usage-session-list');
  const focusTarget = document.querySelector('#focus');
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
  const adminNav = document.querySelector('#admin-nav');
  const adminCard = document.querySelector('#admin-card');
  const adminIdentity = document.querySelector('#admin-identity');
  const adminUsers = document.querySelector('#admin-users');
  const adminGroups = document.querySelector('#admin-groups');
  const adminUserCount = document.querySelector('#admin-user-count');
  const adminGroupCount = document.querySelector('#admin-group-count');
  const adminGroupForm = document.querySelector('#admin-group-form');
  const adminPermissions = document.querySelector('#admin-permissions');
  const adminAudit = document.querySelector('#admin-audit');
  const adminFeedback = document.querySelector('#admin-feedback');
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
        'X-TimeKeeper-Client': 'timekeeper-web-ui',
        'X-TimeKeeper-Surface': 'web_ui',
        ...(options.headers || {})
      }
    });
    const data = await response.json().catch(() => null);
    if (!response.ok) throw new Error(data?.error?.message || 'Request failed (' + response.status + ')');
    return data;
  }

  async function requestOptional(url, label) {
    try {
      return await request(url);
    } catch (error) {
      console.error(label, error);
      return null;
    }
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
        console.error('Dashboard inline action failed', error);
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
        console.error('Sprint action failed', error);
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
        console.error('Sprint hold-reason update failed', error);
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
      try {
        const response = await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/extensions');
        history.replaceChildren();
        for (const extension of response.items || []) {
          const item = document.createElement('li');
          item.className = 'note-item';
          item.textContent = '+' + extension.duration_seconds + 's · ' + extension.reason;
          history.append(item);
        }
      } catch (error) {
        console.error('Sprint extension history request failed', error);
        throw error;
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
        console.error('Dashboard inline action failed', error);
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
          console.error('Sprint history loading failed', error);
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
      try {
        const response = await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/retrieval-attempts');
        history.replaceChildren();
        for (const attempt of response.items || []) {
          const item = document.createElement('li');
          item.className = 'note-item';
          item.textContent = 'Attempt ' + attempt.attempt_number + '/4 · ' + attempt.reason + (attempt.timed_out ? ' · TimedOut' : '');
          history.append(item);
        }
      } catch (error) {
        console.error('Sprint retrieval history request failed', error);
        throw error;
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
        console.error('Dashboard inline action failed', error);
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
          console.error('Sprint history loading failed', error);
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
      try {
        const response = await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/time-entries');
        history.replaceChildren();
        for (const entry of response.items || []) {
          const item = document.createElement('li');
          item.className = 'note-item';
          item.textContent = entry.entry_type + ' · ' + entry.duration_seconds + 's' + (entry.reason ? ' · ' + entry.reason : '');
          history.append(item);
        }
      } catch (error) {
        console.error('Sprint time-entry history request failed', error);
        throw error;
      }
    }
    body.append(feedback, history);
    panel.append(header, body);
    createCollapsible(body, header, icon);
    header.addEventListener('click', () => {
      if (!body.classList.contains('collapsed')) {
        loadHistory().catch(error => {
          console.error('Sprint extension history loading failed', error);
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
        console.error('Project note creation failed', error);
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
        console.error('Project metadata update failed', error);
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
    const reason = document.createElement('input');
    reason.type = 'text';
    reason.placeholder = 'Closeout evidence / cancellation reason';
    reason.setAttribute('aria-label', 'Project closeout evidence or cancellation reason');
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    form.append(label, select, reason, submitBtn, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      submitBtn.disabled = true;
      try {
        await request(api + '/' + encodeURIComponent(project.project_id) + '/status', { method: 'POST', body: JSON.stringify({ status: select.value, reason: reason.value.trim() }) });
        await refresh();
      } catch (error) {
        console.error('Project status update failed', error);
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    return form;
  }

  function renderProjectSchedule(project, refresh) {
    const section = document.createElement('section');
    section.className = 'project-lifecycle';
    const heading = document.createElement('h4');
    heading.textContent = 'Project lifecycle & schedule';
    const state = document.createElement('p');
    state.className = 'status project-deadline-state';
    state.textContent = 'Deadline · ' + (project.deadline_state || 'unscheduled') + (project.target_end_date ? ' · ' + project.target_end_date : '');

    const scheduleForm = document.createElement('form');
    scheduleForm.className = 'inline-form project-schedule-form';
    const plannedStart = document.createElement('input');
    plannedStart.type = 'date';
    plannedStart.value = project.planned_start_date || '';
    plannedStart.setAttribute('aria-label', 'Planned Project start date');
    const targetEnd = document.createElement('input');
    targetEnd.type = 'date';
    targetEnd.value = project.target_end_date || '';
    targetEnd.setAttribute('aria-label', 'Target Project end date');
    const timezone = document.createElement('input');
    timezone.type = 'text';
    timezone.value = project.schedule_timezone || 'UTC';
    timezone.placeholder = 'IANA timezone';
    timezone.setAttribute('aria-label', 'Project schedule timezone');
    const warningDays = document.createElement('input');
    warningDays.type = 'number';
    warningDays.min = '0';
    warningDays.max = '365';
    warningDays.value = String(project.deadline_warning_days ?? 7);
    warningDays.setAttribute('aria-label', 'Deadline warning days');
    const saveSchedule = document.createElement('button');
    saveSchedule.type = 'submit';
    saveSchedule.textContent = 'Save schedule';
    const scheduleFeedback = document.createElement('span');
    scheduleFeedback.className = 'inline-feedback';
    scheduleForm.append(plannedStart, targetEnd, timezone, warningDays, saveSchedule, scheduleFeedback);
    scheduleForm.addEventListener('submit', async event => {
      event.preventDefault();
      saveSchedule.disabled = true;
      try {
        await request(api + '/' + encodeURIComponent(project.project_id) + '/schedule', {
          method: 'PATCH',
          body: JSON.stringify({ planned_start_date: plannedStart.value, target_end_date: targetEnd.value, schedule_timezone: timezone.value.trim(), deadline_warning_days: Number(warningDays.value) })
        });
        await refresh();
      } catch (error) {
        scheduleFeedback.className = 'inline-feedback error';
        scheduleFeedback.textContent = error.message;
      } finally {
        saveSchedule.disabled = false;
      }
    });

    const phaseForm = document.createElement('form');
    phaseForm.className = 'inline-form project-phase-form';
    const phase = document.createElement('select');
    for (const value of ['intake', 'planning', 'execution', 'validation', 'closeout']) {
      const option = document.createElement('option');
      option.value = value;
      option.textContent = value;
      option.selected = project.lifecycle_phase === value;
      phase.append(option);
    }
    phase.setAttribute('aria-label', 'Project lifecycle phase');
    const phaseReason = document.createElement('input');
    phaseReason.type = 'text';
    phaseReason.placeholder = 'Reason (required when replanning/backtracking)';
    phaseReason.setAttribute('aria-label', 'Project phase transition reason');
    const changePhase = document.createElement('button');
    changePhase.type = 'submit';
    changePhase.textContent = 'Change phase';
    const phaseFeedback = document.createElement('span');
    phaseFeedback.className = 'inline-feedback';
    phaseForm.append(phase, phaseReason, changePhase, phaseFeedback);
    phaseForm.addEventListener('submit', async event => {
      event.preventDefault();
      changePhase.disabled = true;
      try {
        await request(api + '/' + encodeURIComponent(project.project_id) + '/phase', {
          method: 'POST', body: JSON.stringify({ phase: phase.value, reason: phaseReason.value.trim() })
        });
        await refresh();
      } catch (error) {
        phaseFeedback.className = 'inline-feedback error';
        phaseFeedback.textContent = error.message;
      } finally {
        changePhase.disabled = false;
      }
    });
    const actuals = document.createElement('p');
    actuals.className = 'project-lifecycle-actuals';
    actuals.textContent = 'Actual start: ' + (project.actual_started_at || 'not started') + ' · Completed: ' + (project.completed_at || 'not completed') + ' · Cancelled: ' + (project.cancelled_at || 'not cancelled');
    section.append(heading, state, scheduleForm, phaseForm, actuals);
    return section;
  }

  function renderProjectMilestones(project, milestones, refresh) {
    const section = document.createElement('section');
    section.className = 'project-milestones';
    const heading = document.createElement('h4');
    heading.textContent = 'Milestones';
    const list = document.createElement('div');
    list.className = 'milestone-list';
    for (const milestone of milestones || []) {
      const row = document.createElement('div');
      row.className = 'milestone-row';
      const title = document.createElement('strong');
      title.textContent = milestone.name;
      const due = document.createElement('input');
      due.type = 'date';
      due.value = milestone.target_date;
      due.setAttribute('aria-label', 'Target date for ' + milestone.name);
      const dueState = document.createElement('span');
      dueState.className = 'status milestone-deadline-state';
      dueState.textContent = (milestone.status || 'Planned') + ' · ' + (milestone.deadline_state || 'unscheduled');
      const linkedTask = document.createElement('span');
      linkedTask.textContent = milestone.linked_task_id ? 'Task ' + milestone.linked_task_id : 'No linked Task';
      const reason = document.createElement('input');
      reason.type = 'text';
      reason.placeholder = 'Reason for cancellation/reopening';
      reason.setAttribute('aria-label', 'Milestone transition reason');
      const status = document.createElement('select');
      for (const value of ['Planned', 'In Progress', 'Completed', 'Cancelled']) {
        const option = document.createElement('option');
        option.value = value;
        option.textContent = value;
        option.selected = milestone.status === value;
        status.append(option);
      }
      status.setAttribute('aria-label', 'Status for ' + milestone.name);
      const save = document.createElement('button');
      save.type = 'button';
      save.textContent = 'Save milestone';
      const feedback = document.createElement('span');
      feedback.className = 'inline-feedback';
      save.addEventListener('click', async () => {
        save.disabled = true;
        try {
          if (due.value !== milestone.target_date) {
            await request(api + '/' + encodeURIComponent(project.project_id) + '/milestones/' + encodeURIComponent(milestone.milestone_id), { method: 'PATCH', body: JSON.stringify({ target_date: due.value }) });
          }
          if (status.value !== milestone.status) {
            await request(api + '/' + encodeURIComponent(project.project_id) + '/milestones/' + encodeURIComponent(milestone.milestone_id) + '/status', { method: 'POST', body: JSON.stringify({ status: status.value, reason: reason.value.trim() }) });
          }
          await refresh();
        } catch (error) {
          feedback.className = 'inline-feedback error';
          feedback.textContent = error.message;
        } finally {
          save.disabled = false;
        }
      });
      row.append(title, due, dueState, linkedTask, status, reason, save, feedback);
      list.append(row);
    }
    if (!milestones || !milestones.length) {
      const empty = document.createElement('p');
      empty.className = 'empty';
      empty.textContent = 'No milestones yet. Add a dated deliverable checkpoint.';
      list.append(empty);
    }
    const form = document.createElement('form');
    form.className = 'inline-form milestone-create-form';
    const name = document.createElement('input');
    name.required = true;
    name.placeholder = 'Milestone name';
    name.setAttribute('aria-label', 'Milestone name');
    const targetDate = document.createElement('input');
    targetDate.type = 'date';
    targetDate.required = true;
    targetDate.setAttribute('aria-label', 'Milestone target date');
    const description = document.createElement('input');
    description.placeholder = 'Description / acceptance note';
    description.setAttribute('aria-label', 'Milestone description');
    const linkedTask = document.createElement('input');
    linkedTask.placeholder = 'Optional Task ID';
    linkedTask.setAttribute('aria-label', 'Optional linked Task ID');
    const add = document.createElement('button');
    add.type = 'submit';
    add.textContent = 'Add milestone';
    const feedback = document.createElement('span');
    feedback.className = 'inline-feedback';
    form.append(name, targetDate, description, linkedTask, add, feedback);
    form.addEventListener('submit', async event => {
      event.preventDefault();
      add.disabled = true;
      try {
        await request(api + '/' + encodeURIComponent(project.project_id) + '/milestones', { method: 'POST', body: JSON.stringify({ name: name.value.trim(), target_date: targetDate.value, description: description.value.trim(), linked_task_id: linkedTask.value.trim() }) });
        await refresh();
      } catch (error) {
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        add.disabled = false;
      }
    });
    section.append(heading, list, form);
    return section;
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
        console.error('Task metadata update failed', error);
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
        console.error('Category metadata update failed', error);
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
        console.error('Task status update failed', error);
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
        console.error('Subtask status update failed', error);
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
        console.error('Category status update failed', error);
        feedback.className = 'inline-feedback error';
        feedback.textContent = error.message;
      } finally {
        submitBtn.disabled = false;
      }
    });
    return form;
  }

  function renderExecutionTree(tree, milestones, summary, attention, notes, events, pipelines, drafts, refresh) {
    const root = document.createElement('div');
    root.className = 'tree';

    // Project header section
    const headerSection = document.createElement('div');
    headerSection.className = 'tree-section';
    headerSection.append(renderProjectStatus(tree.project, refresh));
    headerSection.append(renderProjectMetadata(tree.project, refresh));
    headerSection.append(renderProjectSchedule(tree.project, refresh));
    headerSection.append(renderProjectMilestones(tree.project, milestones, refresh));
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
      const [tree, summary, attention, noteResponse, eventResponse, pipelineResponse, draftResponse, milestones] = await Promise.all([
        request(api + '/' + encodeURIComponent(projectID) + '/execution-tree'),
        request(api + '/' + encodeURIComponent(projectID) + '/operational-summary'),
        request(api + '/' + encodeURIComponent(projectID) + '/attention'),
        request(api + '/' + encodeURIComponent(projectID) + '/notes'),
        request(api + '/' + encodeURIComponent(projectID) + '/events'),
        request('/api/v1/llm-pipelines'),
        request(api + '/' + encodeURIComponent(projectID) + '/planning-drafts'),
        request(api + '/' + encodeURIComponent(projectID) + '/milestones')
      ]);
      target.replaceChildren(renderExecutionTree(tree, milestones || [], summary, attention.items || [], noteResponse.items || [], eventResponse.items || [], pipelineResponse.items || [], draftResponse.items || [], () => refreshExecutionTreeAndPulse(projectID, target, button)));
    } catch (error) {
      console.error('Execution tree loading failed', error);
      target.textContent = error.message;
    } finally {
      button.disabled = false;
    }
  }

  async function refreshExecutionTreeAndPulse(projectID, target, button) {
    try {
      await loadExecutionTree(projectID, target, button);
      await loadPulse();
    } catch (error) {
      console.error('Execution tree refresh failed', error);
      target.textContent = error.message;
    }
  }

  function renderProjectDeadlineWatch(watch, target = deadlineWatch, projectList = projects) {
    if (!target) return;
    target.replaceChildren();
    if (!watch || !Array.isArray(watch.items)) {
      target.className = 'deadline-watch empty warning';
      target.textContent = 'Deadline watch unavailable.';
      return;
    }
    const items = watch.items;
    if (!items.length) {
      target.className = 'deadline-watch empty';
      target.textContent = 'No Projects or milestones due soon.';
      return;
    }
    target.className = 'deadline-watch';
    const heading = document.createElement('div');
    heading.className = 'deadline-watch-heading';
    heading.textContent = 'Deadline watch · ' + items.length + ' due soon or overdue';
    const list = document.createElement('ul');
    list.className = 'deadline-watch-list';
    for (const item of items) {
      const row = document.createElement('li');
      row.className = 'deadline-watch-item ' + (item.deadline_state === 'overdue' ? 'overdue' : 'due-soon');
      const label = document.createElement('strong');
      label.textContent = item.name;
      const state = document.createElement('span');
      state.className = 'deadline-watch-state';
      state.textContent = item.deadline_state === 'overdue' ? 'OVERDUE' : 'DUE SOON';
      const details = document.createElement('span');
      details.className = 'deadline-watch-details';
      const days = Number(item.days_to_deadline);
      const relative = days < 0 ? Math.abs(days) + 'd overdue' : days === 0 ? 'due today' : 'in ' + days + 'd';
      details.textContent = item.project_name + ' · ' + item.entity_type + ' · ' + item.target_date + ' · ' + relative;
      const inspect = document.createElement('button');
      inspect.type = 'button';
      inspect.className = 'deadline-watch-open';
      inspect.textContent = 'Inspect';
      inspect.setAttribute('aria-label', 'Inspect ' + item.project_name + ' for ' + item.name);
      inspect.addEventListener('click', () => {
        const card = Array.from(projectList?.querySelectorAll('.project-card') || []).find(candidate => candidate.dataset.projectId === item.project_id);
        if (!card) return;
        card.hidden = false;
        const tree = card.querySelector('.tree');
        const inspectButton = card.querySelector('.project-card-actions button');
        if (tree?.style.display === 'none') inspectButton?.click();
        card.scrollIntoView({ behavior: 'smooth', block: 'center' });
      });
      row.append(label, state, details, inspect);
      list.append(row);
    }
    target.append(heading, list);
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
      card.dataset.projectId = item.project_id;

      const header = document.createElement('div');
      header.className = 'project-card-header';

      const info = document.createElement('div');
      info.className = 'project-card-info';
      const name = document.createElement('p');
      name.className = 'project-card-name';
      name.textContent = item.project_name;
      const meta = document.createElement('div');
      meta.className = 'project-card-meta';
      meta.textContent = item.project_id + ' · ' + item.item_address + (item.target_end_date ? ' · Due ' + item.target_end_date + ' (' + (item.deadline_state || 'scheduled') + ')' : ' · ' + (item.deadline_state || 'unscheduled'));
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

  // Plugin Store: the registry is optional and deliberately disabled unless
  // -plugin-store-root is set. A 503 means "not configured", which is a
  // different fact from "configured but empty" (200 with zero plugins), so the
  // two states are never collapsed into one "0 plugins" message.
  async function loadPluginStore() {
    const storeStatus = document.querySelector('#store-status');
    const navStoreCount = document.querySelector('#nav-store-count');
    const storeTarget = document.querySelector('#store');
    if (!storeTarget) return;
    try {
      const response = await fetch('/api/v1/plugins');
      const data = await response.json().catch(() => null);
      if (response.status === 503) {
        if (storeStatus) storeStatus.textContent = 'Not configured';
        if (navStoreCount) navStoreCount.textContent = '';
        storeTarget.className = 'empty warning';
        storeTarget.textContent = 'The plugin store is not configured. Start the server with -plugin-store-root <dir> (or TIMEKEEPER_PLUGIN_STORE_ROOT) to expose locally installed plugins.';
        return;
      }
      if (!response.ok) throw new Error(data?.error?.message || 'Request failed (' + response.status + ')');

      const plugins = Array.isArray(data?.plugins) ? data.plugins : [];
      if (navStoreCount) navStoreCount.textContent = plugins.length ? String(plugins.length) : '';
      if (storeStatus) storeStatus.textContent = plugins.length + (plugins.length === 1 ? ' plugin' : ' plugins');
      storeTarget.className = 'store-list';
      storeTarget.replaceChildren();
      if (!plugins.length) {
        storeTarget.className = 'empty';
        storeTarget.textContent = 'No plugins installed. Each plugin needs its own directory with a valid plugin.json.';
        return;
      }
      const list = document.createElement('div');
      list.className = 'store-items';
      for (const plugin of plugins) {
        const item = document.createElement('div');
        item.className = 'store-item';
        const heading = document.createElement('div');
        heading.className = 'store-item-heading';
        const name = document.createElement('strong');
        name.textContent = plugin.name || plugin.id;
        const version = document.createElement('span');
        version.className = 'badge-soft';
        version.textContent = plugin.version || '—';
        heading.append(name, version);
        item.append(heading);
        const meta = document.createElement('div');
        meta.className = 'store-item-meta';
        meta.textContent = [plugin.id, plugin.runtime, plugin.protocol].filter(Boolean).join(' · ');
        item.append(meta);
        if (plugin.description) {
          const description = document.createElement('div');
          description.className = 'store-item-description';
          description.textContent = plugin.description;
          item.append(description);
        }
        const capabilities = Array.isArray(plugin.capabilities) ? plugin.capabilities : [];
        if (capabilities.length) {
          const tags = document.createElement('div');
          tags.className = 'store-item-tags';
          for (const capability of capabilities.slice(0, 8)) {
            const tag = document.createElement('span');
            tag.className = 'badge-soft';
            tag.textContent = capability;
            tags.append(tag);
          }
          item.append(tags);
        }
        if (plugin.artifact) {
          const link = document.createElement('a');
          link.className = 'store-item-artifact';
          link.href = '/api/v1/plugins/' + encodeURIComponent(plugin.id) + '/artifact';
          link.textContent = 'Download ' + plugin.artifact;
          item.append(link);
        }
        list.append(item);
      }
      storeTarget.append(list);
    } catch (error) {
      if (storeStatus) storeStatus.textContent = 'Unavailable';
      if (navStoreCount) navStoreCount.textContent = '';
      storeTarget.className = 'empty error';
      storeTarget.textContent = error.message;
    }
  }

  async function loadPulse() {
    try {
      renderPulse(await request('/api/v1/pulse'));
    } catch (error) {
      console.error('Pulse loading failed', error);
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
      if (entry.event.actor && entry.event.actor.fase_id) {
        const actorLink = document.createElement('a');
        actorLink.href = '/api/v1/agent-profiles/' + encodeURIComponent(entry.event.actor.fase_id) + '/view';
        actorLink.textContent = ' · ' + entry.event.actor.fase_id;
        actorLink.className = 'activity-agent';
        messageText.append(actorLink);
      }
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
    try {
      const responses = await Promise.all(items.map(item => requestOptional(api + '/' + encodeURIComponent(item.project_id) + '/events', 'Project activity history unavailable')));
      const activity = [];
      for (let index = 0; index < responses.length; index += 1) {
        for (const event of responses[index]?.items || []) {
          activity.push({event, projectName: items[index].project_name || items[index].project_id});
        }
      }
      activity.sort((left, right) => new Date(right.event.created_at).getTime() - new Date(left.event.created_at).getTime());
      renderRecentActivity(activity);
    } catch (error) {
      console.error('Recent activity loading failed', error);
      renderRecentActivity([]);
    }
  }

  function formatSeconds(value) {
    const seconds = Math.max(0, Number(value) || 0);
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (hours) return hours + 'h ' + minutes + 'm';
    return minutes + 'm';
  }

  // formatRelative returns a short human-readable relative-time string
  // for an ISO timestamp, e.g. "just now", "12m ago", "3h ago", "5d ago".
  // Returns null on bad input so callers can render a graceful fallback.
  function formatRelative(iso) {
    if (!iso) return null;
    const t = Date.parse(iso);
    if (Number.isNaN(t)) return null;
    const deltaMs = Date.now() - t;
    if (deltaMs < 0) return 'just now';
    const sec = Math.floor(deltaMs / 1000);
    if (sec < 60) return 'just now';
    const min = Math.floor(sec / 60);
    if (min < 60) return min + 'm ago';
    const hr = Math.floor(min / 60);
    if (hr < 24) return hr + 'h ago';
    const day = Math.floor(hr / 24);
    if (day < 30) return day + 'd ago';
    const mo = Math.floor(day / 30);
    if (mo < 12) return mo + 'mo ago';
    return Math.floor(day / 365) + 'y ago';
  }

  // compactEventType maps a raw event_type ("sprint_completed") to a
  // short label ("completed") suitable for the focus row timeline.
  // Anything not in the known set falls back to the raw name.
  function compactEventType(eventType) {
    const map = {
      sprint_started: 'started',
      sprint_completed: 'completed',
      sprint_held: 'on hold',
      sprint_resumed: 'resumed',
      sprint_cancelled: 'cancelled',
      sprint_timed_out: 'timed out',
      project_created: 'created',
      project_metadata_updated: 'updated',
      project_status_changed: 'status',
      note_recorded: 'note',
    };
    const value = map[eventType];
    if (value) return value;
    return eventType ? eventType.replace(/_/g, ' ').replace(/^./, c => c.toUpperCase()) : 'event';
  }

  function renderFocus(items, summaries, lastEvents) {
    focusTarget.replaceChildren();
    if (!items.length) {
      focusTarget.className = 'focus-list empty';
      focusTarget.textContent = 'No projects to focus on yet.';
      return;
    }
    focusTarget.className = 'focus-list';
    for (let index = 0; index < items.length; index += 1) {
      const project = items[index];
      const summary = summaries[index];
      if (!summary) continue;
      const lastEvent = lastEvents && lastEvents[index] && lastEvents[index].items && lastEvents[index].items[0];
      const row = document.createElement('div');
      row.className = 'focus-row';
      const projectInfo = document.createElement('div');
      projectInfo.className = 'focus-project';
      const name = document.createElement('strong');
      name.textContent = project.project_name;
      const status = document.createElement('small');
      status.textContent = project.status + ' · ' + summary.total_sprints + ' sprint(s)';
      projectInfo.append(name, status);
      // Last-activity subline. Real data from /events, newest-first.
      // Graceful fallback: no events => omit; bad event_type => show
      // the compact label without a relative time.
      if (lastEvent) {
        const timeline = document.createElement('small');
        timeline.className = 'focus-timeline';
        const relative = formatRelative(lastEvent.created_at);
        const label = compactEventType(lastEvent.event_type);
        timeline.textContent = 'Last: ' + label + (relative ? ' · ' + relative : '');
        timeline.title = lastEvent.event_type + ' on ' + (lastEvent.created_at || '');
        projectInfo.append(timeline);
      }
      const progress = document.createElement('div');
      progress.className = 'focus-progress';
      const track = document.createElement('div');
      track.className = 'focus-progress-track';
      const fill = document.createElement('div');
      fill.className = 'focus-progress-fill';
      fill.style.width = Math.max(0, Math.min(100, Number(project.calculated_completion_pct || 0))) + '%';
      track.append(fill);
      const progressLabel = document.createElement('span');
      progressLabel.className = 'focus-progress-label';
      progressLabel.textContent = Number(project.calculated_completion_pct || 0).toFixed(1) + '% complete';
      progress.append(track, progressLabel);
      row.append(projectInfo, progress);
      const metrics = [
        ['Active', summary.active_sprints],
        ['On hold', summary.held_sprints],
        ['Planned', formatSeconds(summary.planned_duration_seconds)],
        ['Recorded', formatSeconds(summary.recorded_work_seconds)]
      ];
      for (const [labelText, value] of metrics) {
        const metric = document.createElement('div');
        metric.className = 'focus-metric';
        const label = document.createElement('span');
        label.textContent = labelText;
        const number = document.createElement('strong');
        number.textContent = value;
        metric.append(label, number);
        row.append(metric);
      }
      focusTarget.append(row);
    }
    if (!focusTarget.children.length) {
      focusTarget.className = 'focus-list empty';
      focusTarget.textContent = 'Project focus data unavailable.';
    }
  }

  function compactTokenCount(value) {
    if (value >= 1000000000) return (value / 1000000000).toFixed(1) + 'B';
    if (value >= 1000000) return (value / 1000000).toFixed(1) + 'M';
    if (value >= 1000) return (value / 1000).toFixed(1) + 'K';
    return String(value);
  }

  function usageCostLabel(cost) {
    if (!cost || cost.status !== 'priced' || !/^[A-Z]{3}$/.test(cost.currency || '')) return 'Unpriced';
    try {
      return new Intl.NumberFormat(undefined, {style: 'currency', currency: cost.currency}).format((cost.amount_micros || 0) / 1000000);
    } catch (_) {
      return cost.currency + ' ' + ((cost.amount_micros || 0) / 1000000).toFixed(2);
    }
  }

  function aggregateUsageCost(entries) {
    let priced = false;
    let currency = '';
    let amountMicros = 0;
    let reason = 'no recorded usage sessions';
    for (const entry of entries) {
      const cost = entry.summary?.totals?.cost;
      if (!cost) continue;
      if (cost.status !== 'priced') {
        return {status: 'unpriced', reason: cost.reason || 'one or more usage sessions have no trusted price'};
      }
      if (currency && currency !== cost.currency) {
        return {status: 'unpriced', reason: 'usage sessions do not share one trusted pricing currency'};
      }
      priced = true;
      currency = cost.currency;
      amountMicros += cost.amount_micros || 0;
      reason = '';
    }
    return priced ? {status: 'priced', currency, amount_micros: amountMicros} : {status: 'unpriced', reason};
  }

  function renderUsageOwners(target, values, emptyText) {
    target.replaceChildren();
    if (!values.length) {
      target.className = 'usage-owner-list empty';
      target.textContent = emptyText;
      return;
    }
    target.className = 'usage-owner-list';
    const max = values[0][1] || 1;
    for (const [name, value] of values.slice(0, 6)) {
      const row = document.createElement('div');
      row.className = 'usage-owner-row';
      const main = document.createElement('div');
      main.className = 'usage-owner-main';
      const label = document.createElement('div');
      label.className = 'usage-owner-name';
      label.textContent = name;
      const track = document.createElement('div');
      track.className = 'usage-owner-track';
      const fill = document.createElement('div');
      fill.className = 'usage-owner-fill';
      fill.style.width = (value / max * 100) + '%';
      track.append(fill);
      main.append(label, track);
      const amount = document.createElement('span');
      amount.className = 'usage-owner-value';
      amount.textContent = compactTokenCount(value);
      row.append(main, amount);
      target.append(row);
    }
  }

  function renderUsage(entries) {
    const sessions = entries.flatMap(entry => (entry.summary?.sessions || []).map(session => ({session, projectName: entry.projectName})));
    const totals = sessions.reduce((sum, entry) => {
      sum.input += entry.session.input_tokens || 0;
      sum.output += entry.session.output_tokens || 0;
      sum.cacheCreation += entry.session.cache_creation_tokens || 0;
      sum.cacheRead += entry.session.cache_read_tokens || 0;
      return sum;
    }, {input: 0, output: 0, cacheCreation: 0, cacheRead: 0});
    const totalTokens = totals.input + totals.output + totals.cacheCreation + totals.cacheRead;
    const cost = aggregateUsageCost(entries);
    usageTokens.textContent = compactTokenCount(totalTokens);
    usageSessions.textContent = sessions.length;
    usageInput.textContent = compactTokenCount(totals.input);
    usageOutput.textContent = compactTokenCount(totals.output);
    usageCost.textContent = usageCostLabel(cost);
    usageCost.title = cost.reason || (cost.status === 'priced' ? 'Trusted server-side estimate' : 'No trusted price available');
    usageCoverage.textContent = sessions.length ? sessions.length + ' recorded' : 'Awaiting data';
    usageBreakdownBar.replaceChildren();
    usageLegend.replaceChildren();
    const types = [
      ['Input', totals.input, 'var(--purple)'],
      ['Output', totals.output, 'var(--cyan)'],
      ['Cache write', totals.cacheCreation, 'var(--green)'],
      ['Cache read', totals.cacheRead, 'var(--amber)']
    ];
    for (const [labelText, value, color] of types) {
      const segment = document.createElement('span');
      segment.className = 'usage-breakdown-segment';
      segment.style.width = (totalTokens ? (value / totalTokens * 100) : 0) + '%';
      segment.style.background = color;
      usageBreakdownBar.append(segment);
      const legend = document.createElement('span');
      legend.className = 'usage-legend-item';
      const dot = document.createElement('i');
      dot.className = 'usage-legend-dot';
      dot.style.background = color;
      const text = document.createElement('span');
      text.textContent = labelText;
      const number = document.createElement('strong');
      number.textContent = compactTokenCount(value);
      legend.append(dot, text, number);
      usageLegend.append(legend);
    }
    usageProjects.replaceChildren();
    usageTimeline.replaceChildren();
    usageSessionList.replaceChildren();
    if (!sessions.length) {
      usageProjects.className = 'usage-projects empty';
      usageProjects.textContent = 'No agent usage recorded yet. The panel will populate when a connected agent sends its first snapshot.';
      usageTimeline.className = 'usage-days empty';
      usageTimeline.textContent = 'No daily usage recorded yet.';
      renderUsageOwners(usageAgents, [], 'No agents recorded yet.');
      renderUsageOwners(usageModels, [], 'No models recorded yet.');
      usageSessionList.className = 'usage-session-list empty';
      usageSessionList.textContent = 'No sessions recorded yet.';
      return;
    }
    usageProjects.className = 'usage-projects';
    const daily = new Map();
    for (const entry of entries) {
      for (const day of entry.summary?.days || []) {
        const current = daily.get(day.date) || {date: day.date, tokens: 0};
        current.tokens += (day.input_tokens || 0) + (day.output_tokens || 0) + (day.cache_creation_tokens || 0) + (day.cache_read_tokens || 0);
        daily.set(day.date, current);
      }
    }
    usageTimeline.className = 'usage-days';
    const dayRows = [...daily.values()].sort((left, right) => left.date.localeCompare(right.date)).slice(-7);
    if (!dayRows.length) {
      usageTimeline.className = 'usage-days empty';
      usageTimeline.textContent = 'No daily usage recorded yet.';
    } else {
      const maxDay = Math.max(...dayRows.map(day => day.tokens), 1);
      for (const day of dayRows) {
        const column = document.createElement('div');
        column.className = 'usage-day';
        const value = document.createElement('span');
        value.className = 'usage-day-value';
        value.textContent = compactTokenCount(day.tokens);
        const bar = document.createElement('span');
        bar.className = 'usage-day-bar';
        bar.style.height = Math.max(3, day.tokens / maxDay * 46) + 'px';
        bar.title = day.date + ': ' + day.tokens.toLocaleString() + ' tokens';
        const label = document.createElement('span');
        label.className = 'usage-day-label';
        label.textContent = day.date.slice(5);
        column.append(value, bar, label);
        usageTimeline.append(column);
      }
    }
    const agentTotals = new Map();
    const modelTotals = new Map();
    for (const entry of sessions) {
      const value = (entry.session.input_tokens || 0) + (entry.session.output_tokens || 0) + (entry.session.cache_creation_tokens || 0) + (entry.session.cache_read_tokens || 0);
      const agent = entry.session.agent_id || 'Unknown agent';
      const model = entry.session.model || 'Unknown model';
      agentTotals.set(agent, (agentTotals.get(agent) || 0) + value);
      modelTotals.set(model, (modelTotals.get(model) || 0) + value);
    }
    renderUsageOwners(usageAgents, [...agentTotals.entries()].sort((left, right) => right[1] - left[1]), 'No agents recorded yet.');
    renderUsageOwners(usageModels, [...modelTotals.entries()].sort((left, right) => right[1] - left[1]), 'No models recorded yet.');
    usageSessionList.className = 'usage-session-list';
    for (const entry of sessions.slice().sort((left, right) => {
      const leftTotal = (left.session.input_tokens || 0) + (left.session.output_tokens || 0) + (left.session.cache_creation_tokens || 0) + (left.session.cache_read_tokens || 0);
      const rightTotal = (right.session.input_tokens || 0) + (right.session.output_tokens || 0) + (right.session.cache_creation_tokens || 0) + (right.session.cache_read_tokens || 0);
      return rightTotal - leftTotal;
    }).slice(0, 6)) {
      const row = document.createElement('div');
      row.className = 'usage-session-row';
      const copy = document.createElement('div');
      copy.className = 'usage-session-main';
      const title = document.createElement('div');
      title.className = 'usage-session-title';
      title.textContent = entry.session.title || entry.session.session_id;
      const meta = document.createElement('div');
      meta.className = 'usage-session-meta';
      meta.textContent = entry.projectName + ' · ' + (entry.session.agent_id || 'unknown agent') + ' · ' + (entry.session.model || 'unknown model');
      const tokens = document.createElement('span');
      tokens.className = 'usage-session-tokens';
      tokens.textContent = compactTokenCount((entry.session.input_tokens || 0) + (entry.session.output_tokens || 0) + (entry.session.cache_creation_tokens || 0) + (entry.session.cache_read_tokens || 0));
      copy.append(title, meta);
      row.append(copy, tokens);
      usageSessionList.append(row);
    }
    const projectTotals = new Map();
    for (const entry of sessions) {
      const value = (entry.session.input_tokens || 0) + (entry.session.output_tokens || 0) + (entry.session.cache_creation_tokens || 0) + (entry.session.cache_read_tokens || 0);
      projectTotals.set(entry.projectName, (projectTotals.get(entry.projectName) || 0) + value);
    }
    const rows = [...projectTotals.entries()].sort((left, right) => right[1] - left[1]).slice(0, 6);
    const max = rows[0]?.[1] || 1;
    for (const [projectName, value] of rows) {
      const card = document.createElement('div');
      card.className = 'usage-project';
      const heading = document.createElement('div');
      heading.className = 'usage-project-heading';
      const name = document.createElement('strong');
      name.textContent = projectName;
      const amount = document.createElement('span');
      amount.className = 'usage-project-value';
      amount.textContent = compactTokenCount(value);
      heading.append(name, amount);
      const track = document.createElement('div');
      track.className = 'usage-project-track';
      const fill = document.createElement('div');
      fill.className = 'usage-project-fill';
      fill.style.width = (value / max * 100) + '%';
      track.append(fill);
      card.append(heading, track);
      usageProjects.append(card);
    }
  }

  async function load() {
    void loadPulse();
    void loadGuardianStatus();
    void loadPluginStore();
    try {
      const data = await request(api);
      const items = data.items || [];
      render(items);
      const [summaries, usageSummaries, events, deadlineResponse] = await Promise.all([
        Promise.all(items.map(item => requestOptional(api + '/' + encodeURIComponent(item.project_id) + '/operational-summary', 'Project operational summary unavailable'))),
        Promise.all(items.map(item => requestOptional(api + '/' + encodeURIComponent(item.project_id) + '/usage-summary', 'Project usage summary unavailable'))),
        // Pull the most recent event per project for the focus row
        // timeline. The events endpoint returns newest-first; we only
        // need the first item, so we keep the response shape small.
        // A null item is tolerated (caught, returned as null) so a
        // project with no events does not fail the whole fetch.
        Promise.all(items.map(item => requestOptional(api + '/' + encodeURIComponent(item.project_id) + '/events?limit=1', 'Project activity unavailable'))),
        requestOptional('/api/v1/project-deadlines', 'Project deadline watch unavailable'),
        loadRecentActivity(items)
      ]);
      renderProjectDeadlineWatch(deadlineResponse, deadlineWatch);
      statActive.textContent = summaries.reduce((total, summary) => total + (summary?.active_sprints || 0), 0);
      renderFocus(items, summaries, events);
      renderUsage(usageSummaries.map((summary, index) => ({summary, projectName: items[index].project_name || items[index].project_id})));
      connection.textContent = 'Local API connected';
      sidebarAPIStatus.textContent = 'Online';
      sidebarAPIStatus.style.color = 'var(--green)';
    } catch (error) {
      console.error('Dashboard load failed', error);
      projects.textContent = error.message;
      projects.className = 'empty';
      renderProjectDeadlineWatch(null, deadlineWatch);
      activityTarget.className = 'activity-list empty error';
      activityTarget.textContent = 'Activity unavailable.';
      focusTarget.className = 'focus-list empty error';
      focusTarget.textContent = 'Project focus unavailable.';
      usageCoverage.textContent = 'Unavailable';
      usageProjects.className = 'usage-projects empty error';
      usageProjects.textContent = 'Token usage unavailable.';
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
      await request(api, { method: 'POST', body: JSON.stringify({ name: fields.get('name'), goal: fields.get('goal'), description: fields.get('description'), planned_start_date: fields.get('planned_start_date') || '', target_end_date: fields.get('target_end_date') || '' }) });
      form.reset();
      message.textContent = 'Project created.';
      await load();
    } catch (error) {
      console.error('Project creation failed', error);
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
      if (event.actor && event.actor.fase_id) {
        const actorLink = document.createElement('a');
        actorLink.href = '/api/v1/agent-profiles/' + encodeURIComponent(event.actor.fase_id) + '/view';
        actorLink.textContent = ' · ' + event.actor.fase_id;
        item.append(actorLink);
      }
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
        console.error('LLM pipeline creation failed', error);
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
        } catch (error) {
          console.error('Planning draft generation failed', error);
          message.className = 'error';
          message.textContent = 'Could not generate planning draft: ' + error.message;
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
          try {
            await request(api + '/' + encodeURIComponent(projectID) + '/planning-drafts/' + encodeURIComponent(draft.draft_id) + '/apply', { method: 'POST', body: '{}' });
            await refresh();
          } catch (error) {
            console.error('Planning draft application failed', error);
            message.className = 'error';
            message.textContent = 'Could not apply planning draft: ' + error.message;
          }
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

  // ─── Trail of Thought Board ─────────────────────────────────────────────
  const MESSAGE_MAX = 1000;
  const messagesList = document.querySelector('#messages-list');
  const messagesProjectLabel = document.querySelector('#messages-project-label');
  const messagesProjectSelect = document.querySelector('#messages-project');
  const messagesSearch = document.querySelector('#messages-search');
  const messagesKind = document.querySelector('#messages-kind');
  const messageForm = document.querySelector('#message-compose-form');
  const messageTitle = document.querySelector('#message-compose-title');
  const messageCancel = document.querySelector('#message-compose-cancel');
  const messageBody = document.querySelector('#message-compose-body');
  const messageComposeKind = document.querySelector('#message-compose-kind');
  const messageTags = document.querySelector('#message-compose-tags');
  const messageLink = document.querySelector('#message-compose-link');
  const messageCount = document.querySelector('#message-compose-count');
  const messageSubmit = document.querySelector('#message-compose-submit');
  const messageContext = document.querySelector('#message-compose-context');
  const messageFeedback = document.querySelector('#message-compose-feedback');
  let messagesProjectID = null;
  let messageItems = [];
  let messagesDebounce = 0;
  let messagesLoadToken = 0;
  let editingMessageID = null;
  let replyParentID = null;

  function messagePath(projectID, messageID, suffix) {
    return api + '/' + encodeURIComponent(projectID) + '/messages/' + encodeURIComponent(messageID) + (suffix ? '/' + suffix : '');
  }

  function messageIDValue(message) {
    return Number(message && message.message_id) || 0;
  }

  function resetMessageComposer() {
    editingMessageID = null;
    replyParentID = null;
    if (messageTitle) messageTitle.textContent = 'Post to the board';
    if (messageSubmit) messageSubmit.textContent = 'Post memory';
    if (messageCancel) messageCancel.hidden = true;
    if (messageContext) { messageContext.hidden = true; messageContext.textContent = ''; }
    if (messageForm) messageForm.reset();
    if (messageComposeKind) messageComposeKind.value = 'note';
    if (messageFeedback) { messageFeedback.textContent = ''; messageFeedback.className = 'inline-feedback'; }
    updateMessageCount();
  }

  function beginReply(message) {
    const id = messageIDValue(message);
    if (!id || !messageForm) return;
    editingMessageID = null;
    replyParentID = id;
    messageTitle.textContent = 'Reply to ' + (message.ref || 'M-' + id);
    messageSubmit.textContent = 'Post reply';
    messageCancel.hidden = false;
    messageContext.hidden = false;
    messageContext.textContent = 'Thread context: ' + (message.body || '').slice(0, 180);
    messageBody.value = '';
    messageComposeKind.value = 'answer';
    messageBody.focus();
  }

  function beginEdit(message) {
    const id = messageIDValue(message);
    if (!id || !messageForm) return;
    editingMessageID = id;
    replyParentID = null;
    messageTitle.textContent = 'Edit ' + (message.ref || 'M-' + id);
    messageSubmit.textContent = 'Save revision';
    messageCancel.hidden = false;
    messageContext.hidden = false;
    messageContext.textContent = 'Editing creates an immutable revision; the original remains in history.';
    messageBody.value = message.body || '';
    messageComposeKind.value = message.kind || 'note';
    messageTags.value = message.tags || '';
    messageLink.value = message.link || '';
    messageBody.focus();
    updateMessageCount();
  }

  function updateMessageCount() {
    if (!messageCount || !messageBody) return;
    const length = Array.from(messageBody.value || '').length;
    messageCount.textContent = length + ' / ' + MESSAGE_MAX;
    messageCount.classList.toggle('over', length > MESSAGE_MAX);
  }

  function renderMessageBody(parent, message) {
    const body = document.createElement('div');
    body.className = 'message-body';
    if (!message.snippet) {
      body.textContent = message.body || '';
      parent.appendChild(body);
      return;
    }
    const snippet = String(message.snippet);
    const marker = /<mark>([\s\S]*?)<\/mark>/g;
    let cursor = 0;
    let match;
    while ((match = marker.exec(snippet)) !== null) {
      if (match.index > cursor) body.appendChild(document.createTextNode(snippet.slice(cursor, match.index)));
      const highlight = document.createElement('mark');
      highlight.textContent = match[1];
      body.appendChild(highlight);
      cursor = marker.lastIndex;
    }
    if (cursor < snippet.length) body.appendChild(document.createTextNode(snippet.slice(cursor)));
    parent.appendChild(body);
  }

  function renderMessageCard(message, depth, children, parent) {
    const id = messageIDValue(message);
    const item = document.createElement('article');
    const safeKind = /^[a-z]+$/.test(String(message.kind || 'note')) ? message.kind : 'note';
    item.className = 'message-item kind-' + safeKind;
    item.style.marginLeft = Math.min(depth, 8) * 14 + 'px';
    item.dataset.messageId = id;

    const meta = document.createElement('div');
    meta.className = 'message-meta';
    const kindTag = document.createElement('span');
    kindTag.className = 'message-kind-tag';
    kindTag.textContent = message.kind || 'note';
    const author = document.createElement('span');
    author.textContent = (message.author || 'anon') + ' · ' + (message.ref || 'M-' + id);
    const revision = document.createElement('span');
    revision.textContent = 'r' + (message.revision || 1);
    const timestamp = document.createElement('time');
    timestamp.dateTime = message.updated_at || message.created_at || '';
    timestamp.textContent = (message.updated_at || message.created_at || '').replace('T', ' ').replace('Z', '');
    meta.append(kindTag, author, revision, timestamp);
    item.appendChild(meta);
    renderMessageBody(item, message);

    if (message.link) {
      const link = document.createElement('a');
      link.className = 'message-link';
      link.href = message.link;
      link.target = '_blank';
      link.rel = 'noopener noreferrer';
      link.textContent = message.link;
      item.appendChild(link);
    }
    if (message.tags) {
      const tags = document.createElement('div');
      tags.className = 'message-tags';
      tags.textContent = 'tags: ' + message.tags;
      item.appendChild(tags);
    }

    const actions = document.createElement('div');
    actions.className = 'message-actions';
    const reply = document.createElement('button');
    reply.type = 'button';
    reply.textContent = 'Reply';
    reply.addEventListener('click', () => beginReply(message));
    const edit = document.createElement('button');
    edit.type = 'button';
    edit.textContent = 'Edit';
    edit.addEventListener('click', () => beginEdit(message));
    const historyButton = document.createElement('button');
    historyButton.type = 'button';
    historyButton.textContent = 'Revisions';
    const history = document.createElement('div');
    history.className = 'message-revisions';
    history.hidden = true;
    historyButton.addEventListener('click', async () => {
      history.hidden = !history.hidden;
      if (history.hidden || history.dataset.loaded === 'true') return;
      history.textContent = 'Loading revisions…';
      try {
        const data = await request(messagePath(messagesProjectID, id, 'revisions'));
        history.replaceChildren();
        for (const revisionItem of data.items || []) {
          const row = document.createElement('div');
          row.className = 'message-revision';
          const label = document.createElement('strong');
          label.textContent = 'r' + revisionItem.revision + ' · ' + (revisionItem.edited_by || 'created');
          const text = document.createElement('div');
          text.className = 'message-revision-body';
          text.textContent = revisionItem.body || '';
          row.append(label, text);
          history.appendChild(row);
        }
        if (!history.children.length) history.textContent = 'No revision history.';
        history.dataset.loaded = 'true';
      } catch (error) {
        console.error('Trail of Thought revision loading failed', error);
        history.textContent = 'Could not load revisions: ' + error.message;
      }
    });
    const pinButton = document.createElement('button');
    pinButton.type = 'button';
    pinButton.className = 'message-pin';
    const updatePinLabel = () => { pinButton.textContent = message.pinned ? 'Unpin' : 'Pin'; };
    updatePinLabel();
    pinButton.addEventListener('click', async () => {
      pinButton.disabled = true;
      try {
        const updated = await request(messagePath(messagesProjectID, id, 'pin'), { method: 'POST', body: JSON.stringify({ pinned: !message.pinned }) });
        message.pinned = Boolean(updated.pinned);
        updatePinLabel();
        await loadMessages();
      } catch (error) {
        console.error('Trail of Thought pin mutation failed', error);
        pinButton.textContent = 'Pin failed';
      } finally {
        pinButton.disabled = false;
      }
    });
    const promote = document.createElement('button');
    promote.type = 'button';
    promote.className = 'message-promote';
    promote.textContent = 'Promote to Galaxy';
    promote.addEventListener('click', async () => {
      if (!window.confirm('Promote this Trail entry to the cross-project Memory Galaxy? Confirm that it contains no credentials, private prompts, cookies, or unbounded logs.')) return;
      promote.disabled = true;
      promote.textContent = 'Promoting…';
      try {
        const result = await request(messagePath(messagesProjectID, id, 'promote'), {
          method: 'POST',
          body: JSON.stringify({ redaction_status: 'verified', type: 'knowledge', visibility: 'galaxy' })
        });
        promote.textContent = result && result.item && result.item.id ? '✓ ' + result.item.id : '✓ Galaxy';
        promote.title = 'Promoted to the Memory Galaxy';
      } catch (error) {
        console.error('Trail of Thought promotion failed', error);
        promote.textContent = 'Promote failed';
        promote.disabled = false;
      }
    });

    const contextButton = document.createElement('button');
    contextButton.type = 'button';
    contextButton.textContent = 'Context';

    const context = document.createElement('div');
    context.className = 'message-context';
    context.hidden = true;
    contextButton.addEventListener('click', async () => {
      context.hidden = !context.hidden;
      if (context.hidden || context.dataset.loaded === 'true') return;
      context.textContent = 'Hydrating related context…';
      try {
        const data = await request(messagePath(messagesProjectID, id, 'context') + '?thread_limit=12&related_limit=8&revision_limit=4');
        context.replaceChildren();
        const heading = document.createElement('strong');
        heading.textContent = 'Context · ' + (data.related_mode || 'fts') + ' · ' + (data.related_query || 'no related query');
        context.appendChild(heading);
        const stats = document.createElement('div');
        stats.className = 'message-context-stats';
        stats.textContent = 'thread ' + (data.thread || []).length + (data.thread_truncated ? '+' : '') + ' · related ' + (data.related || []).length + (data.related_truncated ? '+' : '') + ' · revisions ' + (data.revisions || []).length + (data.revisions_truncated ? '+' : '');
        context.appendChild(stats);
        const related = data.related || [];
        if (related.length) {
          const relatedHeading = document.createElement('div');
          relatedHeading.className = 'message-context-label';
          relatedHeading.textContent = 'Related memory';
          context.appendChild(relatedHeading);
          for (const hit of related) {
            const row = document.createElement('div');
            row.className = 'message-context-row';
            const ref = document.createElement('strong');
            ref.textContent = hit.ref || 'M-' + messageIDValue(hit);
            const score = document.createElement('span');
            score.textContent = Number.isFinite(hit.semantic_score) && hit.semantic_score > 0 ? ' · ' + hit.semantic_score.toFixed(3) : '';
            const text = document.createElement('span');
            text.textContent = ' · ' + (hit.body || '');
            row.append(ref, score, text);
            context.appendChild(row);
          }
        } else {
          const empty = document.createElement('div');
          empty.className = 'message-context-empty';
          empty.textContent = 'No related memory found.';
          context.appendChild(empty);
        }
        if (data.thread_truncated || data.related_truncated || data.revisions_truncated) {
          const truncated = document.createElement('div');
          truncated.className = 'message-context-truncated';
          truncated.textContent = 'Bundle bounded; use the CLI/API with deliberate limits for more.';
          context.appendChild(truncated);
        }
        context.dataset.loaded = 'true';
      } catch (error) {
        console.error('Trail of Thought context hydration failed', error);
        context.textContent = 'Could not hydrate context: ' + error.message;
      }
    });
    actions.append(reply, edit, historyButton, contextButton, pinButton, promote);
    item.append(actions, history, context);
    parent.appendChild(item);
    for (const child of children.get(id) || []) renderMessageCard(child, depth + 1, children, parent);
  }

  function renderMessages(items) {
    messagesList.replaceChildren();
    const children = new Map();
    const roots = [];
    const known = new Set(items.map(messageIDValue));
    for (const messageItem of items) {
      const parentID = Number(messageItem.parent_message_id) || 0;
      if (parentID && known.has(parentID)) {
        if (!children.has(parentID)) children.set(parentID, []);
        children.get(parentID).push(messageItem);
      } else {
        roots.push(messageItem);
      }
    }
    const chronological = (left, right) => String(left.created_at || '').localeCompare(String(right.created_at || '')) || messageIDValue(left) - messageIDValue(right);
    roots.sort((left, right) => String(right.updated_at || right.created_at || '').localeCompare(String(left.updated_at || left.created_at || '')) || messageIDValue(right) - messageIDValue(left));
    children.forEach(list => list.sort(chronological));
    for (const root of roots) renderMessageCard(root, 0, children, messagesList);
  }

  async function loadMessageProjects() {
    if (!messagesList) return;
    try {
      const data = await request(api);
      const items = data.items || [];
      if (messagesProjectSelect) {
        messagesProjectSelect.replaceChildren();
        for (const project of items) {
          const option = document.createElement('option');
          option.value = project.project_id;
          option.textContent = project.item_address + ' · ' + project.project_name;
          messagesProjectSelect.appendChild(option);
        }
      }
      if (!items.length) {
        messagesProjectID = null;
        messagesProjectLabel.textContent = 'No projects yet';
        messagesList.className = 'messages-list empty';
        messagesList.textContent = 'Create a project to start a Trail of Thought board.';
        return;
      }
      const selected = items.find(item => item.project_id === messagesProjectID) || items[0];
      messagesProjectID = selected.project_id;
      if (messagesProjectSelect) messagesProjectSelect.value = messagesProjectID;
      messagesProjectLabel.textContent = selected.item_address + ' · ' + selected.project_name;
      await loadMessages();
    } catch (error) {
      console.error('Trail of Thought project loading failed', error);
      messagesList.className = 'messages-list empty';
      messagesList.textContent = 'Could not load projects: ' + error.message;
    }
  }

  async function loadMessages() {
    if (!messagesList || !messagesProjectID) return;
    const token = ++messagesLoadToken;
    const q = (messagesSearch && messagesSearch.value || '').trim();
    const kind = (messagesKind && messagesKind.value || '').trim();
    messagesList.className = 'messages-list';
    messagesList.textContent = 'Loading…';
    try {
      let items = [];
      if (q) {
        const params = new URLSearchParams({ q: q, limit: '50' });
        const data = await request(messagePath(messagesProjectID, 'search') + '?' + params.toString());
        items = (data && data.items) || [];
      } else {
        const params = new URLSearchParams({ limit: '100' });
        if (kind) params.set('kind', kind);
        const data = await request(api + '/' + encodeURIComponent(messagesProjectID) + '/messages?' + params.toString());
        items = (data && data.items) || [];
      }
      if (token !== messagesLoadToken) return;
      if (q && kind) items = items.filter(item => item.kind === kind);
      messageItems = items;
      if (!items.length) {
        messagesList.className = 'messages-list empty';
        messagesList.textContent = q ? 'No matches for "' + q + '".' : (kind ? 'No ' + kind + ' messages yet.' : 'No messages yet — post the first durable thought.');
        return;
      }
      renderMessages(items);
    } catch (error) {
      console.error('Trail of Thought message loading failed', error);
      if (token !== messagesLoadToken) return;
      messagesList.className = 'messages-list empty';
      messagesList.textContent = 'Could not load messages: ' + error.message;
    }
  }

  function canAdmin(principal, permission) {
    return (principal.permissions || []).includes('*') || (principal.permissions || []).includes(permission);
  }

  function renderAdminUsers(users, principal) {
    adminUsers.replaceChildren();
    adminUserCount.textContent = String(users.length);
    if (!users.length) {
      adminUsers.className = 'admin-list empty';
      adminUsers.textContent = 'No accounts have been discovered yet.';
      return;
    }
    adminUsers.className = 'admin-list';
    for (const user of users) {
      const row = document.createElement('div');
      row.className = 'admin-user-row';
      const copy = document.createElement('div');
      copy.className = 'admin-identity-copy';
      const name = document.createElement('strong');
      name.textContent = user.display_name + ' · ' + user.kind;
      const detail = document.createElement('small');
      detail.textContent = user.external_id + ' · ' + ((user.groups || []).join(', ') || 'no groups');
      copy.append(name, detail);
      const actions = document.createElement('div');
      const status = document.createElement('span');
      status.className = 'admin-status' + (user.enabled ? '' : ' disabled');
      status.textContent = user.enabled ? 'Enabled' : 'Disabled';
      actions.appendChild(status);
      if (canAdmin(principal, 'admin.users.manage')) {
        const toggle = document.createElement('button');
        toggle.type = 'button';
        toggle.className = 'secondary';
        toggle.textContent = user.enabled ? 'Disable' : 'Enable';
        toggle.addEventListener('click', async () => {
          toggle.disabled = true;
          try {
            await request('/api/v1/admin/users/' + encodeURIComponent(user.user_id), { method: 'PATCH', body: JSON.stringify({ enabled: !user.enabled }) });
            await loadAdministrator();
          } catch (error) {
            adminFeedback.className = 'inline-feedback error';
            adminFeedback.textContent = error.message;
          } finally {
            toggle.disabled = false;
          }
        });
        actions.appendChild(toggle);
      }
      row.append(copy, actions);
      adminUsers.appendChild(row);
    }
  }

  function renderAdminGroups(groups, users, principal) {
    adminGroups.replaceChildren();
    adminGroupCount.textContent = String(groups.length);
    if (!groups.length) {
      adminGroups.className = 'admin-list empty';
      adminGroups.textContent = 'No security groups exist.';
      return;
    }
    adminGroups.className = 'admin-list';
    for (const group of groups) {
      const row = document.createElement('div');
      row.className = 'admin-group-row';
      const copy = document.createElement('div');
      copy.className = 'admin-identity-copy';
      const name = document.createElement('strong');
      name.textContent = group.name + (group.built_in ? ' · built-in' : '');
      const detail = document.createElement('small');
      detail.textContent = group.slug + ' · ' + group.member_count + ' member(s) · ' + ((group.permissions || []).join(', ') || 'no permissions');
      copy.append(name, detail);
      const actions = document.createElement('div');
      if (canAdmin(principal, 'admin.groups.manage')) {
        const members = document.createElement('select');
        members.multiple = true;
        members.title = 'Group members';
        members.setAttribute('aria-label', 'Members of ' + group.name);
        for (const user of users) {
          const option = document.createElement('option');
          option.value = user.user_id;
          option.textContent = user.display_name;
          option.selected = (user.groups || []).includes(group.slug);
          members.appendChild(option);
        }
        const save = document.createElement('button');
        save.type = 'button';
        save.className = 'secondary';
        save.textContent = 'Save members';
        save.addEventListener('click', async () => {
          save.disabled = true;
          try {
            await request('/api/v1/admin/groups/' + encodeURIComponent(group.group_id) + '/members', { method: 'PUT', body: JSON.stringify({ user_ids: Array.from(members.selectedOptions).map(option => option.value) }) });
            await loadAdministrator();
          } catch (error) {
            adminFeedback.className = 'inline-feedback error';
            adminFeedback.textContent = error.message;
          } finally {
            save.disabled = false;
          }
        });
        actions.append(members, save);
      }
      row.append(copy, actions);
      adminGroups.appendChild(row);
    }
  }

  function renderAdminPermissions(permissions) {
    if (!adminPermissions) return;
    adminPermissions.replaceChildren();
    for (const permission of permissions) {
      const label = document.createElement('label');
      const checkbox = document.createElement('input');
      checkbox.type = 'checkbox';
      checkbox.value = permission.resource + '.' + permission.action;
      label.append(checkbox, document.createTextNode(' ' + checkbox.value));
      adminPermissions.appendChild(label);
    }
  }

  function renderAdminAudit(events) {
    if (!adminAudit) return;
    adminAudit.replaceChildren();
    if (!events.length) {
      adminAudit.className = 'admin-list empty';
      adminAudit.textContent = 'No administrator events yet.';
      return;
    }
    adminAudit.className = 'admin-list';
    for (const event of events) {
      const row = document.createElement('div');
      row.className = 'admin-audit-row';
      const action = document.createElement('strong');
      action.textContent = event.action + ' · ' + event.target_type + (event.target_id ? '/' + event.target_id : '');
      const detail = document.createElement('small');
      detail.textContent = event.actor_user_id + ' · ' + (event.surface || 'unknown surface') + ' · ' + new Date(event.created_at).toLocaleString();
      row.append(action, detail);
      adminAudit.appendChild(row);
    }
  }

  async function loadAdministrator() {
    if (!adminCard) return;
    try {
      const principal = await request('/api/v1/admin/me');
      const visible = canAdmin(principal, 'admin.users.read') || canAdmin(principal, 'admin.groups.read');
      if (!visible) {
        adminNav.hidden = true;
        adminCard.hidden = true;
        return;
      }
      adminNav.hidden = false;
      adminCard.hidden = false;
      adminIdentity.textContent = principal.user.display_name + ' · ' + ((principal.user.groups || []).join(', ') || 'no group');
      const [usersPayload, groupsPayload, permissionsPayload, auditPayload] = await Promise.all([
        canAdmin(principal, 'admin.users.read') ? request('/api/v1/admin/users') : Promise.resolve({ users: [] }),
        canAdmin(principal, 'admin.groups.read') ? request('/api/v1/admin/groups') : Promise.resolve({ groups: [] }),
        canAdmin(principal, 'admin.groups.read') ? request('/api/v1/admin/permissions') : Promise.resolve({ permissions: [] }),
        canAdmin(principal, 'admin.audit.read') ? request('/api/v1/admin/audit?limit=20') : Promise.resolve({ events: [] })
      ]);
      renderAdminUsers(usersPayload.users || [], principal);
      renderAdminGroups(groupsPayload.groups || [], usersPayload.users || [], principal);
      renderAdminPermissions(permissionsPayload.permissions || []);
      renderAdminAudit(auditPayload.events || []);
      adminGroupForm.hidden = !canAdmin(principal, 'admin.groups.manage');
      adminFeedback.className = 'inline-feedback';
      adminFeedback.textContent = principal.is_bootstrap ? 'Bootstrap administrator account created for this local identity.' : 'Access is enforced by the API.';
    } catch (error) {
      console.error('Administrator panel failed', error);
      adminNav.hidden = true;
      adminCard.hidden = true;
    }
  }

  if (adminGroupForm) adminGroupForm.addEventListener('submit', async event => {
    event.preventDefault();
    const selected = Array.from(adminPermissions.querySelectorAll('input:checked')).map(input => input.value);
    const slug = document.querySelector('#admin-group-slug').value.trim();
    const name = document.querySelector('#admin-group-name').value.trim();
    const description = document.querySelector('#admin-group-description').value.trim();
    try {
      await request('/api/v1/admin/groups', { method: 'POST', body: JSON.stringify({ slug, name, description, permissions: selected }) });
      adminGroupForm.reset();
      await loadAdministrator();
    } catch (error) {
      adminFeedback.className = 'inline-feedback error';
      adminFeedback.textContent = error.message;
    }
  });

  loadAdministrator();

  if (messageBody) messageBody.addEventListener('input', updateMessageCount);
  if (messageCancel) messageCancel.addEventListener('click', resetMessageComposer);
  if (messagesProjectSelect) messagesProjectSelect.addEventListener('change', () => {
    messagesProjectID = messagesProjectSelect.value;
    const option = messagesProjectSelect.selectedOptions[0];
    messagesProjectLabel.textContent = option ? option.textContent : 'Project board';
    resetMessageComposer();
    loadMessages();
  });
  if (messageForm) messageForm.addEventListener('submit', async event => {
    event.preventDefault();
    if (!messagesProjectID) return;
    const body = messageBody.value.trim();
    if (!body || Array.from(body).length > MESSAGE_MAX) {
      messageFeedback.className = 'inline-feedback error';
      messageFeedback.textContent = 'Memory posts must contain 1–' + MESSAGE_MAX + ' characters.';
      return;
    }
    messageSubmit.disabled = true;
    messageFeedback.className = 'inline-feedback';
    messageFeedback.textContent = editingMessageID ? 'Saving revision…' : 'Posting…';
    const payload = {
      kind: messageComposeKind.value,
      author: agentID,
      body: body,
      tags: messageTags.value.trim(),
      link: messageLink.value.trim()
    };
    if (replyParentID) payload.parent_message_id = replyParentID;
    try {
      if (editingMessageID) {
        await request(messagePath(messagesProjectID, editingMessageID), { method: 'PATCH', body: JSON.stringify(payload) });
      } else {
        await request(api + '/' + encodeURIComponent(messagesProjectID) + '/messages', { method: 'POST', body: JSON.stringify(payload) });
      }
      resetMessageComposer();
      await loadMessages();
    } catch (error) {
      console.error('Trail of Thought message mutation failed', error);
      messageFeedback.className = 'inline-feedback error';
      messageFeedback.textContent = error.message;
    } finally {
      messageSubmit.disabled = false;
    }
  });
  if (messagesSearch) messagesSearch.addEventListener('input', () => {
    clearTimeout(messagesDebounce);
    messagesDebounce = window.setTimeout(loadMessages, 220);
  });
  if (messagesKind) messagesKind.addEventListener('change', loadMessages);
  loadMessageProjects();

})();

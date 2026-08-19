// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

(() => {
      'use strict';
      const api = '/api/v1/projects';
      const projects = document.querySelector('#projects');
      const pulseTarget = document.querySelector('#pulse');
      const guardianTarget = document.querySelector('#guardian');
      const message = document.querySelector('#message');
      const connection = document.querySelector('#connection');
      const form = document.querySelector('#project-form');
      const submit = document.querySelector('#submit');
      const agentID = localStorage.getItem('timekeeper_agent_id') || crypto.randomUUID();
      localStorage.setItem('timekeeper_agent_id', agentID);

      async function request(url, options = {}) {
        const response = await fetch(url, { ...options, headers: {
          'Content-Type': 'application/json',
          'X-Agent-ID': agentID,
          'X-Agent-Name': 'Time Keeper UI',
          'X-Agent-Type': 'human',
          ...(options.headers || {})
        }});
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
        const form = document.createElement('form');
        form.className = 'inline-form';
        const name = document.createElement('input');
        name.required = true;
        name.maxLength = 200;
        name.placeholder = options.namePlaceholder;
        name.setAttribute('aria-label', options.namePlaceholder);
        form.append(name);
        let estimate;
        if (options.estimate) {
          estimate = document.createElement('input');
          estimate.required = true;
          estimate.className = 'estimate';
          estimate.placeholder = '30m';
          estimate.setAttribute('aria-label', 'Estimate');
          form.append(estimate);
        }
        let buffer;
        if (options.buffer) {
          buffer = document.createElement('input');
          buffer.type = 'number';
          buffer.min = '0';
          buffer.max = '100';
          buffer.step = '1';
          buffer.value = '0';
          buffer.setAttribute('aria-label', 'Buffer percent');
          buffer.title = 'Contingency added to this Sprint estimate';
          form.append(buffer);
        }
        const submit = document.createElement('button');
        submit.type = 'submit';
        submit.textContent = options.submitLabel;
        const feedback = document.createElement('span');
        feedback.className = 'inline-feedback';
        form.append(submit, feedback);
        form.addEventListener('submit', async event => {
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
          submit.disabled = true;
          feedback.className = 'inline-feedback';
          feedback.textContent = 'Saving…';
          try {
            await request(options.endpoint, { method: 'POST', body: JSON.stringify(options.payload(itemName, seconds, bufferPct)) });
            await options.refresh();
          } catch (error) {
            feedback.className = 'inline-feedback error';
            feedback.textContent = error.message;
          } finally {
            submit.disabled = false;
          }
        });
        return form;
      }

      function sprintActionButton(sprint, action, refresh) {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'sprint-action';
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
        button.className = 'sprint-action';
        button.textContent = 'Update hold reason';
        button.addEventListener('click', async () => {
          const supplied = window.prompt('Why is this Sprint still On Hold?', sprint.hold_reason || '');
          if (supplied === null) return;
          const reason = supplied.trim();
          if (!reason) { message.className = 'error'; message.textContent = 'An On Hold Sprint needs a reason.'; return; }
          button.disabled = true;
          try { await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/hold-reason', { method: 'POST', body: JSON.stringify({reason}) }); await refresh(); }
          catch (error) { message.className = 'error'; message.textContent = error.message; }
          finally { button.disabled = false; }
        });
        return button;
      }

      function sprintExtensionPanel(sprint, refresh) {
        const panel = document.createElement('details'); const summary = document.createElement('summary'); summary.textContent = 'Extend Sprint'; const form = document.createElement('form');
        const duration = document.createElement('input'); duration.required = true; duration.placeholder = 'Additional minutes'; duration.type = 'number'; duration.min = '1'; duration.step = '1'; duration.setAttribute('aria-label', 'Additional minutes');
        const reason = document.createElement('textarea'); reason.required = true; reason.maxLength = 10000; reason.placeholder = 'Extension reason'; reason.setAttribute('aria-label', 'Extension reason'); const submit = document.createElement('button'); submit.type = 'submit'; submit.textContent = 'Record extension'; const feedback = document.createElement('span'); feedback.className = 'inline-feedback'; const history = document.createElement('ul');
        async function loadHistory() { const response = await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/extensions'); history.replaceChildren(); for (const extension of response.items || []) { const item = document.createElement('li'); item.textContent = '+' + extension.duration_seconds + 's · ' + extension.reason; history.append(item); } }
        form.append(duration, reason, submit, feedback); form.addEventListener('submit', async event => { event.preventDefault(); submit.disabled = true; try { await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/extensions', { method: 'POST', body: JSON.stringify({duration_seconds: Number(duration.value) * 60, reason: reason.value.trim()}) }); await loadHistory(); await refresh(); } catch (error) { feedback.className = 'inline-feedback error'; feedback.textContent = error.message; } finally { submit.disabled = false; } }); panel.addEventListener('toggle', () => { if (panel.open) { loadHistory().catch(error => { feedback.className = 'inline-feedback error'; feedback.textContent = error.message; }); } }); panel.append(summary, form, history); return panel;
      }

      function sprintRetrievalAttemptPanel(sprint, refresh) {
        const panel = document.createElement('details');
        const summary = document.createElement('summary'); summary.textContent = 'Retrieval attempts';
        const form = document.createElement('form');
        const reason = document.createElement('textarea'); reason.required = true; reason.maxLength = 10000; reason.placeholder = 'Why this retrieval attempt did not produce material progress'; reason.setAttribute('aria-label', 'Retrieval attempt reason');
        const submit = document.createElement('button'); submit.type = 'submit'; submit.textContent = 'Record attempt';
        const feedback = document.createElement('span'); feedback.className = 'inline-feedback';
        const history = document.createElement('ul');
        async function loadHistory() {
          const response = await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/retrieval-attempts');
          history.replaceChildren();
          for (const attempt of response.items || []) { const item = document.createElement('li'); item.textContent = 'Attempt ' + attempt.attempt_number + '/4 · ' + attempt.reason + (attempt.timed_out ? ' · TimedOut' : ''); history.append(item); }
        }
        form.append(reason, submit, feedback);
        form.addEventListener('submit', async event => {
          event.preventDefault(); submit.disabled = true;
          try { const result = await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/retrieval-attempts', { method: 'POST', body: JSON.stringify({reason: reason.value.trim()}) }); reason.value = ''; await loadHistory(); if (result.timed_out) feedback.textContent = 'Fourth attempt recorded. This Sprint is now TimedOut.'; await refresh(); }
          catch (error) { feedback.className = 'inline-feedback error'; feedback.textContent = error.message; }
          finally { submit.disabled = false; }
        });
        panel.addEventListener('toggle', () => { if (panel.open) loadHistory().catch(error => { feedback.className = 'inline-feedback error'; feedback.textContent = error.message; }); });
        panel.append(summary, form, history); return panel;
      }

      function sprintTimeEntryPanel(sprint) {
        const panel = document.createElement('details');
        const summary = document.createElement('summary');
        summary.textContent = 'Recorded intervals';
        const feedback = document.createElement('span');
        feedback.className = 'inline-feedback';
        const history = document.createElement('ul');
        async function loadHistory() {
          const response = await request('/api/v1/sprints/' + encodeURIComponent(sprint.sprint_id) + '/time-entries');
          history.replaceChildren();
          for (const entry of response.items || []) {
            const item = document.createElement('li');
            item.textContent = entry.entry_type + ' · ' + entry.duration_seconds + 's' + (entry.reason ? ' · ' + entry.reason : '');
            history.append(item);
          }
        }
        panel.addEventListener('toggle', () => {
          if (panel.open) {
            loadHistory().catch(error => {
              feedback.className = 'inline-feedback error';
              feedback.textContent = error.message;
            });
          }
        });
        panel.append(summary, feedback, history);
        return panel;
      }

      function appendSprintList(parent, entries, prefix, refresh) {
        if (!entries.length) return;
        const actions = { Open: ['start', 'hold', 'cancel'], Active: ['hold', 'complete', 'cancel'], 'On Hold': ['resume', 'cancel'] };
        const items = document.createElement('ul');
        for (const sprint of entries) {
          const item = document.createElement('li');
          const text = document.createElement('span');
          const holdDetail = sprint.status === 'On Hold' && sprint.hold_reason ? ' · On Hold: ' + sprint.hold_reason : '';
          text.textContent = prefix + ' ' + label(sprint) + ' · ' + sprint.status + holdDetail;
          item.append(text);
          for (const action of actions[sprint.status] || []) item.append(sprintActionButton(sprint, action, refresh));
          if (sprint.status === 'On Hold') item.append(sprintHoldReasonButton(sprint, refresh));
          item.append(sprintExtensionPanel(sprint, refresh));
          if (sprint.status !== 'Completed' && sprint.status !== 'TimedOut') item.append(sprintRetrievalAttemptPanel(sprint, refresh));
          item.append(sprintTimeEntryPanel(sprint));
          items.append(item);
        }
        parent.append(items);
      }

      function renderProjectNotes(projectID, notes, refresh) {
        const section = document.createElement('details');
        section.className = 'notes';
        const summary = document.createElement('summary');
        summary.textContent = 'Notes (' + notes.length + ')';
        const form = document.createElement('form');
        const content = document.createElement('textarea');
        content.required = true;
        content.maxLength = 10000;
        content.placeholder = 'Record an observation, decision, or handoff…';
        content.setAttribute('aria-label', 'Project note');
        const submit = document.createElement('button');
        submit.type = 'submit';
        submit.textContent = 'Add note';
        const feedback = document.createElement('div');
        feedback.className = 'inline-feedback';
        form.append(content, submit, feedback);
        form.addEventListener('submit', async event => {
          event.preventDefault();
          const value = content.value.trim();
          if (!value) return;
          submit.disabled = true;
          try {
            await request(api + '/' + encodeURIComponent(projectID) + '/notes', { method: 'POST', body: JSON.stringify({ content: value }) });
            await refresh();
          } catch (error) {
            feedback.className = 'inline-feedback error';
            feedback.textContent = error.message;
          } finally {
            submit.disabled = false;
          }
        });
        section.append(summary, form);
        if (notes.length) {
          const list = document.createElement('ul');
          for (const note of notes) {
            const item = document.createElement('li');
            item.textContent = note.content + (note.actor_id ? ' · ' + note.actor_id : '');
            list.append(item);
          }
          section.append(list);
        }
        return section;
      }

      function renderProjectMetadata(project, refresh) {
        const section = document.createElement('details'); const summary = document.createElement('summary'); summary.textContent = 'Project context';
        const form = document.createElement('form'); const goal = document.createElement('input'); goal.value = project.project_goal || ''; goal.maxLength = 1000; goal.placeholder = 'Project goal'; goal.setAttribute('aria-label', 'Project goal');
        const description = document.createElement('textarea'); description.value = project.project_description || ''; description.maxLength = 10000; description.placeholder = 'Project description'; description.setAttribute('aria-label', 'Project description');
        const submit = document.createElement('button'); submit.type = 'submit'; submit.textContent = 'Update context'; const feedback = document.createElement('span'); feedback.className = 'inline-feedback';
        form.append(goal, description, submit, feedback); form.addEventListener('submit', async event => { event.preventDefault(); submit.disabled = true; try { await request(api + '/' + encodeURIComponent(project.project_id) + '/metadata', { method: 'POST', body: JSON.stringify({ goal: goal.value, description: description.value }) }); await refresh(); } catch (error) { feedback.className = 'inline-feedback error'; feedback.textContent = error.message; } finally { submit.disabled = false; } });
        section.append(summary, form); return section;
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
        const submit = document.createElement('button');
        submit.type = 'submit';
        submit.textContent = 'Update status';
        const feedback = document.createElement('span');
        feedback.className = 'inline-feedback';
        form.append(label, select, submit, feedback);
        form.addEventListener('submit', async event => {
          event.preventDefault();
          submit.disabled = true;
          try {
            await request(api + '/' + encodeURIComponent(project.project_id) + '/status', { method: 'POST', body: JSON.stringify({ status: select.value }) });
            await refresh();
          } catch (error) {
            feedback.className = 'inline-feedback error';
            feedback.textContent = error.message;
          } finally { submit.disabled = false; }
        });
        return form;
      }

      function renderTaskMetadata(task, refresh) {
        const section = document.createElement('details'); const summary = document.createElement('summary'); summary.textContent = 'Task context';
        const form = document.createElement('form'); const goal = document.createElement('input'); goal.value = task.goal || ''; goal.maxLength = 1000; goal.placeholder = 'Task goal'; goal.setAttribute('aria-label', 'Task goal');
        const description = document.createElement('textarea'); description.value = task.description || ''; description.maxLength = 10000; description.placeholder = 'Task description'; description.setAttribute('aria-label', 'Task description');
        const submit = document.createElement('button'); submit.type = 'submit'; submit.textContent = 'Update context'; const feedback = document.createElement('span'); feedback.className = 'inline-feedback';
        form.append(goal, description, submit, feedback); form.addEventListener('submit', async event => { event.preventDefault(); submit.disabled = true; try { await request('/api/v1/tasks/' + encodeURIComponent(task.task_id) + '/metadata', { method: 'POST', body: JSON.stringify({ goal: goal.value, description: description.value }) }); await refresh(); } catch (error) { feedback.className = 'inline-feedback error'; feedback.textContent = error.message; } finally { submit.disabled = false; } });
        section.append(summary, form); return section;
      }

      function renderCategoryMetadata(category, refresh) {
        const section = document.createElement('details'); const summary = document.createElement('summary'); summary.textContent = 'Category context';
        const form = document.createElement('form'); const goal = document.createElement('input'); goal.value = category.goal || ''; goal.maxLength = 1000; goal.placeholder = 'Category goal'; goal.setAttribute('aria-label', 'Category goal');
        const description = document.createElement('textarea'); description.value = category.description || ''; description.maxLength = 10000; description.placeholder = 'Category description'; description.setAttribute('aria-label', 'Category description');
        const submit = document.createElement('button'); submit.type = 'submit'; submit.textContent = 'Update context'; const feedback = document.createElement('span'); feedback.className = 'inline-feedback';
        form.append(goal, description, submit, feedback); form.addEventListener('submit', async event => { event.preventDefault(); submit.disabled = true; try { await request('/api/v1/categories/' + encodeURIComponent(category.category_id) + '/metadata', { method: 'POST', body: JSON.stringify({ goal: goal.value, description: description.value }) }); await refresh(); } catch (error) { feedback.className = 'inline-feedback error'; feedback.textContent = error.message; } finally { submit.disabled = false; } });
        section.append(summary, form); return section;
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
        const submit = document.createElement('button');
        submit.type = 'submit';
        submit.textContent = 'Update status';
        const feedback = document.createElement('span');
        feedback.className = 'inline-feedback';
        form.append(label, select, submit, feedback);
        form.addEventListener('submit', async event => {
          event.preventDefault();
          submit.disabled = true;
          try {
            await request('/api/v1/tasks/' + encodeURIComponent(task.task_id) + '/status', { method: 'POST', body: JSON.stringify({ status: select.value }) });
            await refresh();
          } catch (error) { feedback.className = 'inline-feedback error'; feedback.textContent = error.message; }
          finally { submit.disabled = false; }
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
        const submit = document.createElement('button');
        submit.type = 'submit';
        submit.textContent = 'Update status';
        const feedback = document.createElement('span');
        feedback.className = 'inline-feedback';
        form.append(label, select, submit, feedback);
        form.addEventListener('submit', async event => {
          event.preventDefault();
          submit.disabled = true;
          try {
            await request('/api/v1/subtasks/' + encodeURIComponent(subtask.subtask_id) + '/status', { method: 'POST', body: JSON.stringify({ status: select.value }) });
            await refresh();
          } catch (error) { feedback.className = 'inline-feedback error'; feedback.textContent = error.message; }
          finally { submit.disabled = false; }
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
        const submit = document.createElement('button');
        submit.type = 'submit';
        submit.textContent = 'Update status';
        const feedback = document.createElement('span');
        feedback.className = 'inline-feedback';
        form.append(label, select, submit, feedback);
        form.addEventListener('submit', async event => {
          event.preventDefault();
          submit.disabled = true;
          try {
            await request('/api/v1/categories/' + encodeURIComponent(category.category_id) + '/status', { method: 'POST', body: JSON.stringify({ status: select.value }) });
            await refresh();
          } catch (error) { feedback.className = 'inline-feedback error'; feedback.textContent = error.message; }
          finally { submit.disabled = false; }
        });
        return form;
      }

      function renderExecutionTree(tree, summary, attention, notes, events, pipelines, drafts, refresh) {
        const root = document.createElement('div');
        root.className = 'tree';
        root.append(renderProjectStatus(tree.project, refresh), renderProjectMetadata(tree.project, refresh), renderOperationalSummary(summary), renderProjectAttention(attention), renderPlanningDrafts(tree.project.project_id, pipelines, drafts, refresh), renderProjectEvents(events), renderProjectNotes(tree.project.project_id, notes, refresh));
        root.append(createInlineForm({
          namePlaceholder: 'New category', submitLabel: 'Add category', estimate: false,
          endpoint: api + '/' + encodeURIComponent(tree.project.project_id) + '/categories',
          payload: name => ({ name }), refresh
        }));
        function renderCategoryNode(categoryNode) {
          const category = document.createElement('details');
          const categorySummary = document.createElement('summary');
          categorySummary.textContent = 'Category · ' + label(categoryNode.category);
          category.append(categorySummary);
          category.append(renderCategoryStatus(categoryNode.category, refresh));
          category.append(renderCategoryMetadata(categoryNode.category, refresh));
          category.append(createInlineForm({
            namePlaceholder: 'New child category', submitLabel: 'Add child category', estimate: false,
            endpoint: api + '/' + encodeURIComponent(tree.project.project_id) + '/categories',
            payload: name => ({ name, parent_category_id: categoryNode.category.category_id }), refresh
          }));
          category.append(createInlineForm({
            namePlaceholder: 'New task', submitLabel: 'Add task', estimate: true,
            endpoint: api + '/' + encodeURIComponent(tree.project.project_id) + '/tasks',
            payload: (name, seconds) => ({ category_id: categoryNode.category.category_id, name, estimated_duration_seconds: seconds }), refresh
          }));
          for (const taskNode of categoryNode.tasks || []) {
            const task = document.createElement('details');
            const taskSummary = document.createElement('summary');
            taskSummary.textContent = 'Task · ' + label(taskNode.task);
            task.append(taskSummary);
            task.append(renderTaskStatus(taskNode.task, refresh));
            task.append(renderTaskMetadata(taskNode.task, refresh));
            task.append(createInlineForm({
              namePlaceholder: 'New direct Sprint', submitLabel: 'Add Sprint', estimate: true, buffer: true,
              endpoint: '/api/v1/tasks/' + encodeURIComponent(taskNode.task.task_id) + '/sprints',
              payload: (name, seconds, bufferPct) => ({ name, estimated_duration_seconds: seconds, buffer_pct: bufferPct }), refresh
            }));
            task.append(createInlineForm({
              namePlaceholder: 'New subtask', submitLabel: 'Add subtask', estimate: true,
              endpoint: '/api/v1/tasks/' + encodeURIComponent(taskNode.task.task_id) + '/subtasks',
              payload: (name, seconds) => ({ name, estimated_duration_seconds: seconds }), refresh
            }));
            appendSprintList(task, taskNode.sprints || [], 'Direct Sprint ·', refresh);
            for (const subtaskNode of taskNode.subtasks || []) {
              const subtask = document.createElement('details');
              const subtaskSummary = document.createElement('summary');
              subtaskSummary.textContent = 'Subtask · ' + label(subtaskNode.subtask);
              subtask.append(subtaskSummary);
              subtask.append(renderSubtaskStatus(subtaskNode.subtask, refresh));
              subtask.append(createInlineForm({
                namePlaceholder: 'New Sprint', submitLabel: 'Add Sprint', estimate: true, buffer: true,
                endpoint: '/api/v1/subtasks/' + encodeURIComponent(subtaskNode.subtask.subtask_id) + '/sprints',
                payload: (name, seconds, bufferPct) => ({ name, estimated_duration_seconds: seconds, buffer_pct: bufferPct }), refresh
              }));
              appendSprintList(subtask, subtaskNode.sprints || [], 'Sprint ·', refresh);
              task.append(subtask);
            }
            category.append(task);
          }
          for (const child of categoryNode.categories || []) {
            category.append(renderCategoryNode(child));
          }
          return category;
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

      function render(items) {
        projects.replaceChildren();
        if (!items.length) { projects.textContent = 'No projects yet. Create the first one.'; projects.className = 'empty'; return; }
        projects.className = '';
        const table = document.createElement('table');
        const head = document.createElement('thead');
        const header = document.createElement('tr');
        for (const label of ['Project', 'Status', 'Progress', 'Execution']) { const cell = document.createElement('th'); cell.textContent = label; header.append(cell); }
        head.append(header); table.append(head);
        const body = document.createElement('tbody');
        for (const item of items) {
          const row = document.createElement('tr');
          const project = document.createElement('td');
          const name = document.createElement('strong'); name.textContent = item.project_name;
          const id = document.createElement('div'); id.className = 'status'; id.textContent = item.project_id + ' · ' + item.item_address;
          project.append(name, id);
          const status = document.createElement('td'); status.textContent = item.status;
          const progress = document.createElement('td'); progress.textContent = Number(item.calculated_completion_pct || 0).toFixed(1) + '%';
          const execution = document.createElement('td');
          const button = document.createElement('button'); button.type = 'button'; button.className = 'tree-button'; button.textContent = 'Inspect hierarchy';
          const tree = document.createElement('div'); tree.className = 'tree';
          button.addEventListener('click', () => loadExecutionTree(item.project_id, tree, button));
          execution.append(button, tree);
          row.append(project, status, progress, execution); body.append(row);
        }
        table.append(body); projects.append(table);
      }

      function renderPulse(snapshot) {
        const attention = Array.isArray(snapshot.attention) ? snapshot.attention : [];
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

      async function load() {
        void loadPulse();
        void loadGuardianStatus();
        try { const data = await request(api); render(data.items || []); connection.textContent = 'Local SQLite API connected'; }
        catch (error) { projects.textContent = error.message; projects.className = 'empty'; connection.textContent = 'Local API unavailable'; }
      }

      form.addEventListener('submit', async event => {
        event.preventDefault(); submit.disabled = true; message.className = ''; message.textContent = 'Creating…';
        const fields = new FormData(form);
        try {
          await request(api, { method: 'POST', body: JSON.stringify({ name: fields.get('name'), goal: fields.get('goal'), description: fields.get('description') }) });
          form.reset(); message.textContent = 'Project created.'; await load();
        } catch (error) { message.className = 'error'; message.textContent = error.message; }
        finally { submit.disabled = false; }
      });
      load();
      function renderProjectAttention(items) {
        const section = document.createElement('section');
        const title = document.createElement('strong'); title.textContent = 'Attention beyond Pulse';
        section.append(title);
        if (!items.length) {
          const clear = document.createElement('div'); clear.className = 'empty'; clear.textContent = 'Clear. No held, TimedOut, or stranded Open Sprint needs a decision.';
          section.append(clear); return section;
        }
        const list = document.createElement('ul');
        for (const item of items) {
          const row = document.createElement('li');
          row.textContent = item.kind + ' · ' + item.name + ' · ' + item.sprint_id + (item.hold_reason ? ' · ' + item.hold_reason : '') + ' · ' + item.detail;
          list.append(row);
        }
        section.append(list); return section;
      }

      function renderOperationalSummary(summary) {
        const section = document.createElement('section');
        const title = document.createElement('strong'); title.textContent = 'Operational snapshot';
        const detail = document.createElement('div');
        detail.textContent = summary.total_sprints + ' Sprint(s) · ' + summary.active_sprints + ' active · ' + summary.held_sprints + ' on hold · ' + summary.timed_out_sprints + ' TimedOut · ' + summary.cancelled_sprints + ' cancelled · ' + summary.estimated_duration_seconds + 's estimate + ' + summary.buffer_duration_seconds + 's buffer + ' + summary.extension_duration_seconds + 's extensions = ' + summary.planned_duration_seconds + 's planned · ' + summary.recorded_work_seconds + 's recorded work · ' + summary.recorded_hold_seconds + 's recorded hold';
        section.append(title, detail); return section;
      }
      function renderProjectEvents(events) {
        const section = document.createElement('details'); const summary = document.createElement('summary'); summary.textContent = 'Activity (' + events.length + ')'; section.append(summary);
        const list = document.createElement('ul'); for (const event of events) { const item = document.createElement('li'); item.textContent = event.message; list.append(item); } section.append(list); return section;
      }

      function renderLocalPipelineForm(refresh, headingText, collapsed) {
        const container=document.createElement(collapsed ? 'details' : 'div'); const heading=document.createElement(collapsed ? 'summary' : 'strong'); heading.textContent=headingText;
        const form=document.createElement('form'); const name=document.createElement('input'); name.required=true; name.placeholder='Planner name'; const provider=document.createElement('select'); for (const value of ['ollama','openai-compatible']) { const option=document.createElement('option'); option.value=value; option.textContent=value; provider.append(option); }
        const baseURL=document.createElement('input'); baseURL.required=true; baseURL.placeholder='http://127.0.0.1:11434'; const model=document.createElement('input'); model.required=true; model.placeholder='Model name'; const systemPrompt=document.createElement('textarea'); systemPrompt.maxLength=10000; systemPrompt.placeholder='Optional planning instructions'; systemPrompt.setAttribute('aria-label','Optional planning instructions'); const submit=document.createElement('button'); submit.type='submit'; submit.textContent='Save local planner'; const feedback=document.createElement('span'); feedback.className='inline-feedback';
        form.append(name,provider,baseURL,model,systemPrompt,submit,feedback); form.addEventListener('submit',async event=>{ event.preventDefault(); submit.disabled=true; try { await request('/api/v1/llm-pipelines',{method:'POST',body:JSON.stringify({name:name.value.trim(),provider:provider.value,base_url:baseURL.value.trim(),model:model.value.trim(),system_prompt:systemPrompt.value.trim()})}); await refresh(); } catch(error) { feedback.className='inline-feedback error'; feedback.textContent=error.message; } finally { submit.disabled=false; } }); container.append(heading,form); return container;
      }

      function renderPlanningDrafts(projectID, pipelines, drafts, refresh) {
        const section=document.createElement('details'); const summary=document.createElement('summary'); summary.textContent='Local planning drafts ('+drafts.length+')'; section.append(summary);
        if (!pipelines.length) {
          const message=document.createElement('p'); message.textContent='No local planning pipeline is configured.'; section.append(message);
          section.append(renderLocalPipelineForm(refresh,'Configure local planner',false)); return section;
        }
        const select=document.createElement('select'); for (const pipeline of pipelines) { const option=document.createElement('option'); option.value=String(pipeline.pipeline_id); option.textContent=pipeline.name+' · '+pipeline.provider+' · '+pipeline.model; select.append(option); }
        const generate=document.createElement('button'); generate.type='button'; generate.textContent='Generate plan'; generate.addEventListener('click',async()=>{ generate.disabled=true; try { await request(api+'/'+encodeURIComponent(projectID)+'/planning-drafts/generate',{method:'POST',body:JSON.stringify({pipeline_id:Number(select.value)})}); await refresh(); } finally { generate.disabled=false; } }); section.append(select,generate);
        section.append(renderLocalPipelineForm(refresh,'Add another local planner',true));
        for (const draft of drafts) { const item=document.createElement('details'); const itemSummary=document.createElement('summary'); itemSummary.textContent='Draft '+draft.draft_id+' · '+draft.status+' · '+draft.summary; const raw=document.createElement('pre'); raw.textContent=draft.raw_json; item.append(itemSummary,raw); if(draft.status==='Review'){ const apply=document.createElement('button'); apply.type='button'; apply.textContent='Apply approved draft'; apply.addEventListener('click',async()=>{if(!window.confirm('Apply this reviewed planning draft?'))return; await request(api+'/'+encodeURIComponent(projectID)+'/planning-drafts/'+encodeURIComponent(draft.draft_id)+'/apply',{method:'POST',body:'{}'});await refresh();});item.append(apply);}section.append(item); } return section;
      }

    })();

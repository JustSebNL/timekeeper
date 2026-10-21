<script lang="ts">
  import { tick } from 'svelte'
  import type { SprintAction, TaskDetail, TaskStatus } from './api'
  import {
    sprintActionLabel,
    sprintActionRequiresReason,
    sprintActionsForStatus,
    retainAvailableSprintAction,
    taskStatusRequiresConfirmation,
  } from './taskActions'

  interface Props {
    detail: TaskDetail | null
    loading: boolean
    error: string
    busy: boolean
    onBack: () => void
    onRetry: () => void
    onTaskStatusChange: (status: TaskStatus) => Promise<void>
    onSprintAction: (sprintID: string, action: SprintAction, reason: string) => Promise<void>
  }

  let {
    detail,
    loading,
    error,
    busy,
    onBack,
    onRetry,
    onTaskStatusChange,
    onSprintAction,
  }: Props = $props()

  const taskStatuses: TaskStatus[] = ['Open', 'On Hold', 'Completed', 'Cancelled']
  let selectedSprintID = $state('')
  let nextTaskStatus = $state<TaskStatus | ''>('')
  let confirmTaskCancel = $state(false)
  let pendingSprintAction = $state<SprintAction | null>(null)
  let reason = $state('')
  let actionError = $state('')
  let actionPending = $state(false)
  let taskHeading = $state<HTMLHeadingElement | null>(null)
  let lastFocusedTaskID = ''

  const selectedSprint = $derived(
    detail?.sprints.find((sprint) => sprint.sprint_id === selectedSprintID) ?? detail?.sprints[0] ?? null,
  )
  const availableActions = $derived(selectedSprint ? sprintActionsForStatus(selectedSprint.status) : [])

  $effect(() => {
    const taskID = detail?.task.task_id
    if (!taskID || taskID === lastFocusedTaskID) return
    lastFocusedTaskID = taskID
    void tick().then(() => taskHeading?.focus())
  })

  $effect(() => {
    const retainedAction = retainAvailableSprintAction(pendingSprintAction, availableActions)
    if (retainedAction !== pendingSprintAction) {
      pendingSprintAction = retainedAction
      reason = ''
    }
  })

  function duration(seconds = 0): string {
    const minutes = Math.max(0, Math.floor(seconds / 60))
    const hours = Math.floor(minutes / 60)
    const remainder = minutes % 60
    if (hours && remainder) return `${hours}h ${remainder}m`
    if (hours) return `${hours}h`
    return `${minutes}m`
  }

  function dateTime(value?: string | null): string {
    if (!value) return 'Not recorded'
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
  }

  async function changeTaskStatus() {
    if (!detail || !nextTaskStatus || busy || actionPending) return
    actionError = ''
    actionPending = true
    try {
      await onTaskStatusChange(nextTaskStatus)
      nextTaskStatus = ''
      confirmTaskCancel = false
    } catch (cause) {
      actionError = cause instanceof Error ? cause.message : 'Core could not update this Task.'
    } finally {
      actionPending = false
    }
  }

  function requestSprintAction(action: SprintAction) {
    actionError = ''
    if (sprintActionRequiresReason(action)) {
      pendingSprintAction = action
      reason = ''
      return
    }
    if (selectedSprint) void runSprintAction(selectedSprint.sprint_id, action, '')
  }

  async function runSprintAction(sprintID: string, action: SprintAction, actionReason: string) {
    if (busy || actionPending) return
    actionError = ''
    actionPending = true
    try {
      await onSprintAction(sprintID, action, actionReason)
      pendingSprintAction = null
      reason = ''
    } catch (cause) {
      actionError = cause instanceof Error ? cause.message : 'Core could not update this Sprint.'
    } finally {
      actionPending = false
    }
  }

  function submitReason(event: SubmitEvent) {
    event.preventDefault()
    if (!selectedSprint || !pendingSprintAction || !reason.trim()) return
    if (!availableActions.includes(pendingSprintAction)) {
      pendingSprintAction = null
      reason = ''
      return
    }
    void runSprintAction(selectedSprint.sprint_id, pendingSprintAction, reason.trim())
  }

  function actionButtonLabel(action: SprintAction): string {
    if (action === 'start') return 'Start Sprint'
    if (action === 'hold') return 'Put Sprint on hold'
    if (action === 'resume') return 'Resume Sprint'
    if (action === 'complete') return 'Complete Sprint'
    return 'Cancel Sprint'
  }
</script>

<section class="task-workspace" aria-labelledby="task-workspace-title">
  <div class="workspace-toolbar">
    <button class="text-button" type="button" onclick={onBack}>← Back to Project hierarchy</button>
    <span class="read-only-tag"><span aria-hidden="true">◉</span> Core-backed workspace</span>
  </div>

  {#if error}
    <div class="state-panel error-panel" role="alert">
      <span class="state-symbol" aria-hidden="true">!</span>
      <div><strong>Couldn't refresh Task workspace</strong><p>{error}</p><small>Current details are retained until Core can be reached again.</small></div>
      <button class="text-button" type="button" onclick={onRetry}>Retry Task workspace</button>
    </div>
  {/if}

  {#if loading && !detail}
    <div class="state-panel" aria-live="polite"><span class="loading-dot"></span> Loading Task detail from Core.</div>
  {:else if !detail}
    <div class="state-panel"><span class="state-symbol" aria-hidden="true">?</span><div><strong>No Task selected</strong><p>Choose a Task in the Project hierarchy to inspect its details.</p></div></div>
  {:else}
    <header class="task-workspace-heading">
      <div>
        <p class="eyebrow">TASK WORKSPACE · {detail.task.task_id}{#if detail.task.item_address} · {detail.task.item_address}{/if}</p>
        <h2 id="task-workspace-title" tabindex="-1" bind:this={taskHeading}>{detail.task.name}</h2>
        {#if detail.task.goal}<p class="subtitle">{detail.task.goal}</p>{/if}
      </div>
      <span class="project-status"><i></i>{detail.task.status}</span>
    </header>

    {#if detail.task.description}<p class="task-description">{detail.task.description}</p>{/if}

    <div class="task-summary-grid" aria-label="Task summary">
      <article><span>PRIORITY</span><strong>{detail.task.priority || 'Not set'}</strong></article>
      <article><span>ESTIMATE</span><strong>{duration(detail.task.estimated_duration_seconds)}</strong></article>
      <article><span>CALCULATED COMPLETION</span><strong>{Math.round(detail.task.calculated_completion_pct || 0)}%</strong></article>
    </div>

    <section class="task-control-panel" aria-labelledby="task-status-heading">
      <div><p class="eyebrow">CONTROLLED CORE ACTION</p><h3 id="task-status-heading">Task status</h3></div>
      <div class="task-status-form">
        <label for="task-status">Set Task status</label>
        <select
          id="task-status"
          value={nextTaskStatus}
          disabled={busy || actionPending}
          onchange={(event) => {
            nextTaskStatus = event.currentTarget.value as TaskStatus | ''
            confirmTaskCancel = false
          }}
        >
          <option value="">Choose a status</option>
          {#each taskStatuses as status (status)}<option value={status}>{status}</option>{/each}
        </select>
        {#if nextTaskStatus === 'Cancelled'}
          <label class="confirm-check"><input type="checkbox" bind:checked={confirmTaskCancel} disabled={busy || actionPending} /> Confirm Task cancellation</label>
        {/if}
        <button
          class="primary-button"
          type="button"
          disabled={busy || actionPending || !nextTaskStatus || (nextTaskStatus === 'Completed' && !detail.children.can_complete) || (taskStatusRequiresConfirmation(nextTaskStatus) && !confirmTaskCancel)}
          onclick={changeTaskStatus}
        >Update Task status</button>
      </div>
      {#if !detail.children.can_complete}
        <p class="completion-blocker" role="status">
          Open Subtasks must be Completed or Cancelled, and open Sprints must be Completed, Cancelled, or TimedOut before this Task can complete ({detail.children.open_subtasks} Subtasks; {detail.children.open_sprints} Sprints).
        </p>
      {/if}
    </section>

    {#if actionError}<p class="workspace-action-error" role="alert">{actionError}</p>{/if}

    <div class="task-children-grid">
      <section class="task-children-panel" aria-labelledby="subtasks-heading">
        <div class="task-panel-heading"><div><p class="eyebrow">DIRECT CHILDREN</p><h3 id="subtasks-heading">Subtasks <span class="count">{detail.subtasks.length}</span></h3></div></div>
        {#if detail.subtasks.length === 0}
          <p class="empty-inline">No direct Subtasks.</p>
        {:else}
          <ul class="task-child-list">
            {#each detail.subtasks as subtask (subtask.subtask_id)}
              <li><span><strong>{subtask.name}</strong><small>{subtask.subtask_id}{#if subtask.item_address} · {subtask.item_address}{/if}</small></span><span class="hierarchy-status">{subtask.status}</span></li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="task-children-panel sprint-panel" aria-labelledby="sprints-heading">
        <div class="task-panel-heading"><div><p class="eyebrow">DIRECT EXECUTION CUTS</p><h3 id="sprints-heading">Sprints <span class="count">{detail.sprints.length}</span></h3></div></div>
        {#if detail.sprints.length === 0}
          <p class="empty-inline">No direct Sprints.</p>
        {:else}
          <label class="sprint-select-label" for="selected-sprint">Selected Sprint</label>
          <select
            id="selected-sprint"
            aria-label="Choose Sprint"
            value={selectedSprint?.sprint_id ?? ''}
            onchange={(event) => {
              selectedSprintID = event.currentTarget.value
              pendingSprintAction = null
              actionError = ''
            }}
            disabled={busy || actionPending}
          >
            {#each detail.sprints as sprint (sprint.sprint_id)}<option value={sprint.sprint_id}>{sprint.name} · {sprint.status}</option>{/each}
          </select>
          {#if selectedSprint}
            <article class="sprint-detail-card">
              <div class="sprint-detail-title"><div><small>{selectedSprint.sprint_id}{#if selectedSprint.item_address} · {selectedSprint.item_address}{/if}</small><h4>{selectedSprint.name}</h4></div><span class="project-status">{selectedSprint.status}</span></div>
              {#if selectedSprint.goal}<p>{selectedSprint.goal}</p>{/if}
              {#if selectedSprint.description}<p>{selectedSprint.description}</p>{/if}
              <dl class="sprint-metrics">
                <div><dt>Estimate</dt><dd>{duration(selectedSprint.estimated_duration_seconds)}</dd></div>
                <div><dt>Recorded active time</dt><dd>{duration(selectedSprint.active_duration_seconds)}</dd></div>
                <div><dt>Recorded hold time</dt><dd>{duration(selectedSprint.hold_duration_seconds)}</dd></div>
                <div><dt>Started</dt><dd>{dateTime(selectedSprint.started_at)}</dd></div>
                <div><dt>Ended</dt><dd>{dateTime(selectedSprint.ended_at)}</dd></div>
              </dl>
              {#if selectedSprint.hold_reason}<p class="hold-reason"><strong>Hold reason:</strong> {selectedSprint.hold_reason}</p>{/if}
            </article>

            {#if availableActions.length}
              <div class="sprint-actions" aria-label="Available Sprint actions">
                {#each availableActions as action (action)}
                  <button class={`sprint-action-button ${action === 'start' || action === 'resume' ? 'primary-button' : 'secondary-button'}`} type="button" disabled={busy || actionPending} onclick={() => requestSprintAction(action)}>
                    {actionButtonLabel(action)}
                  </button>
                {/each}
              </div>
            {/if}

            {#if pendingSprintAction}
              <form class="sprint-reason-form" onsubmit={submitReason}>
                <label for="sprint-action-reason">Reason to {sprintActionLabel(pendingSprintAction).toLowerCase()} Sprint</label>
                <textarea id="sprint-action-reason" bind:value={reason} required rows="3" disabled={busy || actionPending}></textarea>
                <div class="form-actions">
                  <button class="secondary-button" type="button" disabled={busy || actionPending} onclick={() => (pendingSprintAction = null)}>Keep current status</button>
                  <button class="primary-button" type="submit" disabled={busy || actionPending || !reason.trim()}>{sprintActionLabel(pendingSprintAction)} Sprint</button>
                </div>
              </form>
            {/if}
          {/if}
        {/if}
      </section>
    </div>
  {/if}
</section>

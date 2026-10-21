<script lang="ts">
  import type { ProjectExecutionTree, ProjectHandoffSnapshot } from './api'
  import { flattenExecutionTree } from './executionTree'

  interface Props {
    tree: ProjectExecutionTree | null
    loading: boolean
    error: string
    handoff?: ProjectHandoffSnapshot | null
    handoffLoading?: boolean
    handoffError?: string
    onBack: () => void
    onRetry: () => void
    onSelectTask: (taskID: string) => void
  }

  let {
    tree,
    loading,
    error,
    handoff = null,
    handoffLoading = false,
    handoffError = '',
    onBack,
    onRetry,
    onSelectTask,
  }: Props = $props()
  const rows = $derived.by(() => tree ? flattenExecutionTree(tree) : [])

  function formatTimestamp(value: string): string {
    const parsed = new Date(value)
    return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
  }

  function deadlineSummary(state?: string, days?: number | null): string {
    if (state === 'overdue') {
      const overdueDays = typeof days === 'number' ? Math.abs(days) : null
      if (overdueDays === null) return 'Overdue'
      return `Overdue by ${overdueDays} ${overdueDays === 1 ? 'day' : 'days'}`
    }
    if (state === 'due_soon') {
      if (days === 0) return 'Due today'
      if (typeof days === 'number' && days > 0) return `Due in ${days} ${days === 1 ? 'day' : 'days'}`
      return 'Due soon'
    }
    if (state === 'on_track') {
      if (typeof days === 'number') return `${days} ${days === 1 ? 'day' : 'days'} remaining`
      return 'On track'
    }
    if (state === 'completed') return 'Completed'
    if (state === 'cancelled') return 'Cancelled'
    if (state === 'unscheduled' || !state) return 'No target date set'
    return state.replace(/_/g, ' ')
  }
</script>

<section class="project-workspace" aria-labelledby="project-workspace-title">
  <div class="workspace-toolbar">
    <button class="text-button" type="button" onclick={onBack}>← Back to projects</button>
    <span class="read-only-tag"><span aria-hidden="true">◉</span> Read only</span>
  </div>

  {#if error}
    <div class="state-panel error-panel" role="alert">
      <span class="state-symbol" aria-hidden="true">!</span>
      <div><strong>Couldn’t load Project hierarchy</strong><p>{error}</p><small>The existing Core execution-tree endpoint is the source of truth.</small></div>
      <button class="text-button" type="button" onclick={onRetry}>Retry</button>
    </div>
  {:else if loading}
    <div class="state-panel" aria-live="polite"><span class="loading-dot"></span> Loading Project hierarchy from Core…</div>
  {:else if !tree}
    <div class="state-panel"><span class="state-symbol" aria-hidden="true">⌂</span><div><strong>No Project selected</strong><p>Choose a Project to inspect its hierarchy.</p></div></div>
  {:else}
    <header class="workspace-project-heading">
      <div>
        <p class="eyebrow">PROJECT WORKSPACE · {tree.project.project_id}</p>
        <h2 id="project-workspace-title">{tree.project.project_name}</h2>
        {#if tree.project.project_goal}<p class="subtitle">{tree.project.project_goal}</p>{/if}
      </div>
      <span class="project-status"><i></i>{tree.project.status}</span>
    </header>

    <section class="project-schedule-panel" aria-labelledby="project-schedule-title">
      <div class="schedule-heading">
        <div><p class="eyebrow">CORE PROJECT PLAN</p><h3 id="project-schedule-title">Project schedule</h3></div>
        <span class="deadline-pill" data-state={tree.project.deadline_state ?? 'unscheduled'}>
          {deadlineSummary(tree.project.deadline_state, tree.project.days_to_deadline)}
        </span>
      </div>
      <dl class="schedule-grid">
        <div><dt>Planned start</dt><dd>{tree.project.planned_start_date || 'Not scheduled'}</dd></div>
        <div><dt>Target end</dt><dd>{tree.project.target_end_date || 'No target date set'}</dd></div>
        {#if tree.project.actual_started_at}
          <div><dt>Actual start</dt><dd><time datetime={tree.project.actual_started_at}>{formatTimestamp(tree.project.actual_started_at)}</time></dd></div>
        {/if}
        {#if tree.project.completed_at}
          <div><dt>Completed at</dt><dd><time datetime={tree.project.completed_at}>{formatTimestamp(tree.project.completed_at)}</time></dd></div>
        {/if}
        {#if tree.project.cancelled_at}
          <div><dt>Cancelled at</dt><dd><time datetime={tree.project.cancelled_at}>{formatTimestamp(tree.project.cancelled_at)}</time></dd></div>
        {/if}
        <div><dt>Lifecycle phase</dt><dd>{tree.project.lifecycle_phase || 'Not set'}</dd></div>
        <div><dt>Schedule timezone</dt><dd>{tree.project.schedule_timezone || 'UTC'}</dd></div>
        <div><dt>Due-soon warning</dt><dd>{#if typeof tree.project.deadline_warning_days === 'number'}{tree.project.deadline_warning_days} {tree.project.deadline_warning_days === 1 ? 'day' : 'days'} before target{:else}Not available from Core{/if}</dd></div>
      </dl>
    </section>

    {#if rows.length === 0}
      <div class="state-panel"><span class="state-symbol" aria-hidden="true">▤</span><div><strong>No categories yet</strong><p>This Project has no executable hierarchy yet.</p></div></div>
    {:else}
      <section class="hierarchy-panel" aria-label="Project execution hierarchy">
        <div class="hierarchy-heading"><span>ITEM</span><span>STATUS</span><span>ADDRESS</span></div>
        <ol class="hierarchy-tree" aria-label="Categories, Tasks, Subtasks and Sprints">
          {#each rows as row (row.id)}
            <li class="hierarchy-item" data-node-id={row.id}>
              {#if row.kind === 'task'}
                <button
                  class={`hierarchy-row kind-${row.kind} hierarchy-action`}
                  data-task-id={row.id}
                  type="button"
                  aria-label={`Open Task ${row.name} workspace`}
                  style={`--hierarchy-depth:${row.depth}`}
                  onclick={() => onSelectTask(row.id)}
                >
                  <span class="hierarchy-name"><span class="hierarchy-kind">{row.kind}</span><strong>{row.name}</strong></span>
                  <span class="hierarchy-status">{row.status}{#if typeof row.completionPercent === 'number'} · {Math.round(row.completionPercent)}%{/if}</span>
                  <code>{row.itemAddress}</code>
                </button>
              {:else}
                <div class={`hierarchy-row kind-${row.kind}`} style={`--hierarchy-depth:${row.depth}`}>
                  <span class="hierarchy-name"><span class="hierarchy-kind">{row.kind}</span><strong>{row.name}</strong></span>
                  <span class="hierarchy-status">{row.status}{#if typeof row.completionPercent === 'number'} · {Math.round(row.completionPercent)}%{/if}</span>
                  <code>{row.itemAddress}</code>
                </div>
              {/if}
            </li>
          {/each}
        </ol>
      </section>
    {/if}

    <section class="project-context-grid" aria-label="Project activity, notes, and Trail of Thought">
      <section class="context-panel" aria-labelledby="project-activity-title">
        <div class="context-panel-heading">
          <div><p class="eyebrow">PERSISTED CORE EVENTS</p><h3 id="project-activity-title">Recent activity</h3></div>
          <span class="read-only-tag">At last refresh</span>
        </div>
        {#if handoffLoading}
          <div class="context-empty" role="status">Loading recent activity from Core…</div>
        {:else if handoffError}
          <div class="context-empty" role="status"><strong>Recent activity unavailable</strong><p>{handoffError}</p><button class="text-button" type="button" onclick={onRetry}>Retry</button></div>
        {:else if !handoff}
          <div class="context-empty" role="status">Project activity snapshot unavailable.</div>
        {:else if (handoff.recent_events?.length ?? 0) === 0}
          <div class="context-empty">No recent system events.</div>
        {:else}
          <ol class="activity-list" aria-label="Recent system events">
            {#each handoff.recent_events ?? [] as event (event.event_id)}
              <li class="activity-entry">
                <div class="activity-meta"><time datetime={event.created_at}>{formatTimestamp(event.created_at)}</time><span>{event.event_type}</span></div>
                <p>{event.message}</p>
                {#if event.entity_type.toLowerCase() === 'task' && event.entity_id}
                  <button class="activity-task-link" type="button" aria-label={`Open Task ${event.entity_id} workspace`} onclick={() => onSelectTask(event.entity_id)}>Open Task</button>
                {/if}
              </li>
            {/each}
          </ol>
        {/if}
      </section>

      <section class="context-panel" aria-labelledby="project-notes-title">
        <div class="context-panel-heading">
          <div><p class="eyebrow">ATTRIBUTED PROJECT NOTES</p><h3 id="project-notes-title">Recent project notes</h3></div>
          <span class="read-only-tag">Separate from system events</span>
        </div>
        {#if handoffLoading}
          <div class="context-empty" role="status">Loading project notes from Core…</div>
        {:else if handoffError}
          <div class="context-empty" role="status">Project notes are unavailable.</div>
        {:else if !handoff}
          <div class="context-empty" role="status">Project notes snapshot unavailable.</div>
        {:else if (handoff.recent_notes?.length ?? 0) === 0}
          <div class="context-empty">No recent project notes.</div>
        {:else}
          <ol class="activity-list" aria-label="Recent project notes">
            {#each handoff.recent_notes ?? [] as note (note.note_id)}
              <li class="activity-entry note-entry">
                <time datetime={note.created_at}>{formatTimestamp(note.created_at)}</time>
                <p>{note.content}</p>
              </li>
            {/each}
          </ol>
        {/if}
      </section>

      <section class="context-panel" aria-labelledby="project-messages-title">
        <div class="context-panel-heading">
          <div><p class="eyebrow">PROJECT TRAIL OF THOUGHT</p><h3 id="project-messages-title">Trail of Thought</h3></div>
          <span class="read-only-tag">Core messages</span>
        </div>
        {#if handoffLoading}
          <div class="context-empty" role="status">Loading Trail of Thought from Core…</div>
        {:else if handoffError}
          <div class="context-empty" role="status">Trail of Thought is unavailable.</div>
        {:else if !handoff}
          <div class="context-empty" role="status">Trail of Thought snapshot unavailable.</div>
        {:else if (handoff.recent_messages?.length ?? 0) === 0}
          <div class="context-empty">No recent Trail of Thought messages.</div>
        {:else}
          <ol class="activity-list" aria-label="Recent Trail of Thought messages">
            {#each handoff.recent_messages ?? [] as message (message.message_id)}
              <li class="activity-entry message-entry" data-message-ref={message.ref}>
                <div class="activity-meta">
                  <time datetime={message.created_at}>{formatTimestamp(message.created_at)}</time>
                  <span>{message.kind}</span>
                  <span>{message.author || 'Unknown author'}</span>
                  <code>{message.ref}</code>
                  {#if message.pinned}<span>Pinned</span>{/if}
                </div>
                <p>{message.body}</p>
                {#if message.tags}<small class="message-detail">Tags: {message.tags}</small>{/if}
                {#if message.link}<small class="message-detail">Link: <code>{message.link}</code></small>{/if}
              </li>
            {/each}
          </ol>
        {/if}
      </section>
    </section>
  {/if}
</section>

<style>
  .project-schedule-panel {
    margin-top: 14px;
    padding: 14px 16px;
    border: 1px solid var(--line, #e9edf2);
    border-radius: 10px;
    background: #fff;
    box-shadow: 0 2px 6px #1b28380a;
  }

  .schedule-heading { display: flex; align-items: center; justify-content: space-between; gap: 14px; margin-bottom: 12px; }
  .schedule-heading .eyebrow { margin-bottom: 5px; }
  .schedule-heading h3 { margin: 0; color: #344052; font-size: 13px; }
  .schedule-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); gap: 12px; margin: 0; }
  .schedule-grid div { min-width: 0; }
  .schedule-grid dt { color: #929cac; font-size: 9px; font-weight: 700; letter-spacing: .04em; text-transform: uppercase; }
  .schedule-grid dd { margin: 3px 0 0; color: #536073; font-size: 11px; overflow-wrap: anywhere; }
  .deadline-pill { flex: 0 0 auto; padding: 4px 9px; border: 1px solid #e2e7ee; border-radius: 999px; background: #f7f9fc; color: #536073; font-size: 10px; font-weight: 700; }
  .deadline-pill[data-state="overdue"] { border-color: #f1c6c3; background: #fff4f3; color: #a33b33; }
  .deadline-pill[data-state="due_soon"] { border-color: #f0d7a5; background: #fff9ea; color: #886000; }
  .deadline-pill[data-state="completed"] { border-color: #bfe3c8; background: #f0fbf2; color: #28743a; }

  .project-context-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 320px), 1fr));
    gap: 12px;
    margin-top: 16px;
  }

  .context-panel {
    min-width: 0;
    padding: 15px;
    border: 1px solid var(--line, #e9edf2);
    border-radius: 10px;
    background: #fff;
    box-shadow: 0 2px 6px #1b28380a;
  }

  .context-panel-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
  }

  .context-panel-heading .eyebrow { margin-bottom: 5px; }
  .context-panel-heading h3 { margin: 0; color: #344052; font-size: 13px; }

  .activity-list {
    display: grid;
    gap: 0;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .activity-entry {
    padding: 11px 0;
    border-top: 1px solid #f0f2f5;
  }

  .activity-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; color: #929cac; font-size: 9px; }
  .activity-meta span { color: #7357dc; font-weight: 650; }
  .activity-entry p { margin: 5px 0 0; color: #536073; font-size: 10px; line-height: 1.5; white-space: pre-wrap; overflow-wrap: anywhere; }
  .activity-task-link { margin-top: 6px; padding: 0; border: 0; color: #6e58cc; background: transparent; font-size: 9px; }
  .activity-task-link:focus-visible { outline: 2px solid #8c78e7; outline-offset: 2px; }
  .note-entry time { color: #929cac; font-size: 9px; }
  .message-detail { display: block; margin-top: 4px; color: #7f8a9a; font-size: 9px; overflow-wrap: anywhere; }
  .context-empty { padding: 12px; border-radius: 7px; color: #7f8a9a; background: #fafbfd; font-size: 10px; }
  .context-empty strong { color: #536073; }
  .context-empty p { margin: 5px 0; }
  .context-empty .text-button { margin-left: 0; padding: 0; }
</style>

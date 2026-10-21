<script lang="ts">
  import type { ProjectSummary, RegistrationQueueItem, RegistrationQueuePage, RegistrationQueueStatus } from './api'

  interface QueueAPI {
    getRegistrationQueue(projectID: string, status?: string, limit?: number, offset?: number): Promise<RegistrationQueuePage>
    getRegistrationQueueItem(projectID: string, queueID: string): Promise<RegistrationQueueItem>
  }

  interface Props {
    projects: ProjectSummary[]
    initialProjectID: string | null
    api: QueueAPI
  }

  let { projects, initialProjectID, api }: Props = $props()
  let projectID = $state('')
  let statusFilter = $state('')
  let items = $state<RegistrationQueueItem[]>([])
  let selected = $state<RegistrationQueueItem | null>(null)
  let hasMore = $state(false)
  let nextOffset = $state<number | null>(null)
  let loading = $state(false)
  let loadingMore = $state(false)
  let detailLoading = $state(false)
  let error = $state('')
  let detailError = $state('')
  let requestGeneration = 0

  $effect(() => {
    if (initialProjectID && initialProjectID !== projectID) projectID = initialProjectID
    else if (!projectID && projects.length > 0) projectID = projects[0].project_id
  })

  $effect(() => {
    const activeProject = projectID
    const activeStatus = statusFilter
    if (activeProject) void loadQueue(activeProject, activeStatus)
  })

  async function loadQueue(activeProject = projectID, activeStatus = statusFilter) {
    const generation = ++requestGeneration
    if (!activeProject) {
      items = []
      selected = null
      hasMore = false
      nextOffset = null
      loading = false
      loadingMore = false
      return
    }
    loading = true
    loadingMore = false
    error = ''
    detailError = ''
    selected = null
    items = []
    hasMore = false
    nextOffset = null
    try {
      const page = await api.getRegistrationQueue(activeProject, activeStatus, 50, 0)
      if (generation !== requestGeneration) return
      items = page.items
      hasMore = page.has_more
      nextOffset = page.next_offset
    } catch (cause) {
      if (generation !== requestGeneration) return
      error = cause instanceof Error ? cause.message : 'Could not load registration offers.'
    } finally {
      if (generation === requestGeneration) loading = false
    }
  }

  async function loadMore() {
    const activeProject = projectID
    const activeStatus = statusFilter
    const offset = nextOffset
    const generation = requestGeneration
    if (!activeProject || offset === null || !hasMore || loadingMore) return
    loadingMore = true
    error = ''
    try {
      const page = await api.getRegistrationQueue(activeProject, activeStatus, 50, offset)
      if (generation !== requestGeneration || projectID !== activeProject || statusFilter !== activeStatus) return
      const knownIDs = new Set(items.map((item) => item.queue_id))
      items = [...items, ...page.items.filter((item) => !knownIDs.has(item.queue_id))]
      hasMore = page.has_more
      nextOffset = page.next_offset
    } catch (cause) {
      if (generation !== requestGeneration) return
      error = cause instanceof Error ? cause.message : 'Could not load older registration offers.'
    } finally {
      if (generation === requestGeneration) loadingMore = false
    }
  }

  async function loadDetail(queueID: string, activeProject = projectID, parentGeneration = requestGeneration) {
    detailLoading = true
    detailError = ''
    try {
      const detail = await api.getRegistrationQueueItem(activeProject, queueID)
      if (parentGeneration !== requestGeneration || projectID !== activeProject) return
      selected = detail
    } catch (cause) {
      if (parentGeneration !== requestGeneration || projectID !== activeProject) return
      detailError = cause instanceof Error ? cause.message : 'Could not load this registration offer.'
    } finally {
      if (parentGeneration === requestGeneration && projectID === activeProject) detailLoading = false
    }
  }

  async function selectItem(item: RegistrationQueueItem) {
    selected = item
    await loadDetail(item.queue_id)
  }

  function formatCounts(item: RegistrationQueueItem): string {
    const counts = item.counts
    return `${counts.tasks} Tasks · ${counts.subtasks} Subtasks · ${counts.sprints} Sprints`
  }

  function formatTimestamp(value: string): string {
    const time = new Date(value)
    return Number.isNaN(time.getTime()) ? value : time.toLocaleString()
  }

  function offeredEntries(item: RegistrationQueueItem): string[] {
    const entries: string[] = []
    for (const task of item.payload?.tasks ?? []) {
      entries.push(`Task · ${task.name}`)
      for (const sprint of task.sprints ?? []) entries.push(`Sprint · ${sprint.name}`)
      for (const subtask of task.subtasks ?? []) {
        entries.push(`Subtask · ${subtask.name}`)
        for (const sprint of subtask.sprints ?? []) entries.push(`Sprint · ${sprint.name}`)
      }
    }
    for (const extension of item.payload?.extensions ?? []) {
      const owner = extension.task_id ?? extension.subtask_id ?? 'existing owner'
      for (const subtask of extension.subtasks ?? []) {
        entries.push(`Subtask for ${owner} · ${subtask.name}`)
        for (const sprint of subtask.sprints ?? []) entries.push(`Sprint · ${sprint.name}`)
      }
      for (const sprint of extension.sprints ?? []) entries.push(`Sprint for ${owner} · ${sprint.name}`)
    }
    return entries
  }

  function statusLabel(status: RegistrationQueueStatus): string {
    return status.toLocaleLowerCase()
  }
</script>

<section class="registration-queue" aria-labelledby="registration-queue-title">
  <header class="queue-heading">
    <div>
      <h2 id="registration-queue-title">API registration queue</h2>
      <p>Every accepted offer is persisted here first. Core registers queued work FIFO; registered Tasks, Subtasks, and Sprints then appear in the normal Project workspace.</p>
    </div>
    <button type="button" class="queue-refresh" onclick={() => loadQueue()} disabled={loading || loadingMore || !projectID}>
      {loading ? 'Refreshing…' : 'Refresh queue'}
    </button>
  </header>

  <div class="queue-controls">
    <label>
      Project
      <select aria-label="Registration queue Project" bind:value={projectID}>
        <option value="">Choose a Project</option>
        {#each projects as project (project.project_id)}
          <option value={project.project_id}>{project.project_name} ({project.project_id})</option>
        {/each}
      </select>
    </label>
    <label>
      Status
      <select aria-label="Filter registration queue status" bind:value={statusFilter}>
        <option value="">All statuses</option>
        <option value="Queued">Queued</option>
        <option value="Registered">Registered</option>
        <option value="Rejected">Rejected</option>
      </select>
    </label>
  </div>

  {#if error}<p class="queue-error" role="alert">{error}</p>{/if}
  {#if !projectID}
    <div class="queue-empty"><strong>No Project selected</strong><span>Select a Project to inspect its registration offers.</span></div>
  {:else if loading && items.length === 0}
    <p class="queue-muted" role="status">Loading registration queue…</p>
  {:else if !loading && items.length === 0 && !error}
    <div class="queue-empty"><strong>Queue is empty</strong><span>API offers will be retained here until Core registers or rejects them.</span></div>
  {:else}
    <div class="queue-layout">
      <section class="queue-list" aria-label="Registration offers">
        <div class="queue-list-heading"><h3>Offers</h3><span>{items.length}{hasMore ? '+' : ''} shown</span></div>
        <ol>
          {#each items as item (item.queue_id)}
            <li>
              <button type="button" class:active={selected?.queue_id === item.queue_id} onclick={() => selectItem(item)}>
                <span class="queue-row-top"><strong>{item.queue_id}</strong><span class={`queue-status status-${statusLabel(item.status)}`}>{statusLabel(item.status)}</span></span>
                <span class="queue-row-meta">{item.kind.replaceAll('_', ' ')} · {formatCounts(item)}</span>
                <span class="queue-row-time">{formatTimestamp(item.created_at)}</span>
              </button>
            </li>
          {/each}
        </ol>
        {#if hasMore}
          <button type="button" class="queue-load-more" onclick={loadMore} disabled={loadingMore}>
            {loadingMore ? 'Loading older offers…' : 'Load older offers'}
          </button>
        {/if}
      </section>

      <section class="queue-detail" aria-label="Selected registration offer">
        {#if !selected}
          <div class="queue-empty"><strong>Select an offer</strong><span>Queue entries remain visible after registration so submission and outcome are auditable.</span></div>
        {:else}
          <header class="queue-detail-heading">
            <div><p class="eyebrow">{selected.kind.replaceAll('_', ' ')}</p><h3>{selected.queue_id}</h3></div>
            <span class={`queue-status status-${statusLabel(selected.status)}`}>{statusLabel(selected.status)}</span>
          </header>
          {#if detailLoading}<p class="queue-muted" role="status">Loading offer details…</p>{/if}
          {#if detailError}<p class="queue-error" role="alert">{detailError}</p>{/if}
          {#if selected.last_error}<p class="queue-error" role="alert">{selected.last_error}</p>{/if}
          <p class="queue-detail-counts">{formatCounts(selected)}</p>
          <p class="queue-detail-time">Submitted {formatTimestamp(selected.created_at)}{selected.processed_at ? ` · Processed ${formatTimestamp(selected.processed_at)}` : ''}</p>
          {#if offeredEntries(selected).length > 0}
            <h4>Offered jobs</h4>
            <ul class="queue-task-names">{#each offeredEntries(selected) as entry}<li>{entry}</li>{/each}</ul>
          {/if}
          {#if selected.result}
            <h4>Registered Core records</h4>
            <ul class="queue-task-names">
              {#each selected.result.tasks ?? [] as task}<li>{task.task_id} · {task.name}</li>{/each}
              {#each selected.result.subtasks ?? [] as subtask}<li>{subtask.subtask_id} · {subtask.name}</li>{/each}
              {#each selected.result.sprints ?? [] as sprint}<li>{sprint.sprint_id} · {sprint.name}</li>{/each}
            </ul>
          {/if}
          {#if selected.validation && (selected.validation.issues?.length ?? 0) > 0}
            <h4>Registration findings</h4>
            <ul class="queue-issues">
              {#each selected.validation.issues ?? [] as issue}
                <li><strong>{issue.path}.{issue.field}</strong><span>{issue.message}</span><small>{issue.suggestion}</small></li>
              {/each}
            </ul>
          {/if}
          {#if selected.status === 'Queued'}
            <p class="queue-muted" role="status">This offer is durable and waiting for the Core registration worker.</p>
          {:else if selected.status === 'Registered'}
            <p class="queue-success" role="status">Registration completed. The created work is available in the Project workspace and runnable Sprint queue.</p>
          {/if}
        {/if}
      </section>
    </div>
  {/if}
</section>

<style>
  .registration-queue { display: grid; gap: 1rem; }
  .queue-heading, .queue-detail-heading, .queue-list-heading, .queue-row-top { display: flex; align-items: center; justify-content: space-between; gap: 1rem; }
  .queue-heading h2, .queue-list-heading h3, .queue-detail-heading h3 { margin: 0; }
  .queue-heading p, .queue-muted, .queue-detail-time, .queue-row-meta, .queue-row-time { color: var(--text-muted, #64748b); }
  .queue-controls { display: flex; flex-wrap: wrap; gap: 1rem; }
  .queue-controls label { display: grid; gap: .35rem; min-width: 12rem; color: var(--text-muted, #64748b); font-size: .8rem; }
  .queue-controls select { border: 1px solid var(--border, #d7dce5); border-radius: .6rem; padding: .6rem; color: var(--text, #182230); background: var(--surface, #fff); }
  .queue-layout { display: grid; grid-template-columns: minmax(18rem, .8fr) minmax(20rem, 1.2fr); gap: 1rem; }
  .queue-list, .queue-detail { min-width: 0; border: 1px solid var(--border, #e2e8f0); border-radius: .9rem; background: var(--surface, #fff); padding: 1rem; }
  .queue-list ol, .queue-task-names, .queue-issues { list-style: none; padding: 0; margin: .8rem 0 0; }
  .queue-list li + li { margin-top: .45rem; }
  .queue-list button { width: 100%; display: grid; gap: .25rem; padding: .7rem; border: 1px solid transparent; border-radius: .65rem; text-align: left; background: transparent; color: inherit; cursor: pointer; }
  .queue-list button:hover, .queue-list button.active { border-color: var(--accent, #6558d3); background: var(--surface-subtle, #f7f8fc); }
  .queue-row-meta, .queue-row-time { font-size: .78rem; }
  .queue-status { border-radius: 999px; padding: .2rem .55rem; font-size: .72rem; font-weight: 700; text-transform: uppercase; }
  .status-queued { color: #805b00; background: #fff3c4; }
  .status-registered { color: #166534; background: #dcfce7; }
  .status-rejected { color: #991b1b; background: #fee2e2; }
  .queue-empty { display: grid; gap: .35rem; padding: 1.4rem; border: 1px dashed var(--border, #cbd5e1); border-radius: .75rem; color: var(--text-muted, #64748b); }
  .queue-empty strong { color: var(--text, #182230); }
  .queue-refresh, .queue-load-more { border: 1px solid var(--border, #d7dce5); border-radius: .6rem; padding: .6rem .8rem; background: var(--surface, #fff); color: var(--text, #182230); cursor: pointer; }
  .queue-load-more { width: 100%; margin-top: .8rem; }
  .queue-error { color: #991b1b; }
  .queue-success { color: #166534; }
  .queue-issues li { display: grid; gap: .25rem; padding: .6rem 0; border-bottom: 1px solid var(--border, #e2e8f0); }
  .queue-issues small { color: var(--text-muted, #64748b); }
  @media (max-width: 900px) { .queue-layout { grid-template-columns: 1fr; } }
</style>

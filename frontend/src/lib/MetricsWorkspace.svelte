<script lang="ts">
  import type { ProjectOperationalSummary, ProjectSummary, ProjectUsageSummary, UsageCost } from './api'

  interface MetricsAPI {
    getProjectOperationalSummary: (projectID: string) => Promise<ProjectOperationalSummary>
    getProjectUsageSummary: (projectID: string) => Promise<ProjectUsageSummary>
  }

  interface Props {
    projects: ProjectSummary[]
    initialProjectID: string | null
    api: MetricsAPI
  }

  let { projects, initialProjectID, api }: Props = $props()
  let selectedProjectID = $state('')
  let usageSummary = $state<ProjectUsageSummary | null>(null)
  let operationalSummary = $state<ProjectOperationalSummary | null>(null)
  let usageLoading = $state(false)
  let operationalLoading = $state(false)
  let usageError = $state('')
  let operationalError = $state('')
  let requestID = 0
  const selectedProjectName = $derived(projects.find((project) => project.project_id === selectedProjectID)?.project_name ?? selectedProjectID)

  $effect(() => {
    const availableProjects = projects
    const preferredProjectID = initialProjectID && availableProjects.some((project) => project.project_id === initialProjectID)
      ? initialProjectID
      : availableProjects[0]?.project_id ?? ''
    if (availableProjects.length === 0) return
    if (selectedProjectID && availableProjects.some((project) => project.project_id === selectedProjectID)) return
    if (!preferredProjectID) return
    selectedProjectID = preferredProjectID
    void loadMetrics(preferredProjectID)
  })

  async function loadMetrics(projectID: string) {
    const currentRequestID = ++requestID
    if (!projectID) return
    usageSummary = null
    operationalSummary = null
    usageError = ''
    operationalError = ''
    usageLoading = true
    operationalLoading = true

    const [usageResult, operationalResult] = await Promise.allSettled([
      api.getProjectUsageSummary(projectID),
      api.getProjectOperationalSummary(projectID),
    ])
    if (currentRequestID !== requestID || projectID !== selectedProjectID) return

    if (usageResult.status === 'fulfilled') usageSummary = usageResult.value
    else usageError = usageResult.reason instanceof Error ? usageResult.reason.message : 'Agent token usage is unavailable from Core.'
    if (operationalResult.status === 'fulfilled') operationalSummary = operationalResult.value
    else operationalError = operationalResult.reason instanceof Error ? operationalResult.reason.message : 'Operational metrics are unavailable from Core.'
    usageLoading = false
    operationalLoading = false
  }

  function chooseProject(event: Event) {
    const nextProjectID = (event.currentTarget as HTMLSelectElement).value
    selectedProjectID = nextProjectID
    void loadMetrics(nextProjectID)
  }

  function formatNumber(value: number): string {
    return new Intl.NumberFormat('en-US').format(value)
  }

  function formatDuration(seconds: number): string {
    const totalMinutes = Math.floor(Math.max(0, seconds) / 60)
    const hours = Math.floor(totalMinutes / 60)
    const minutes = totalMinutes % 60
    return hours > 0 ? `${formatNumber(hours)}h ${minutes}m` : `${minutes}m`
  }

  function formatCost(cost?: UsageCost): string {
    if (!cost) return 'Cost unavailable'
    if (cost.status === 'unpriced') return 'Unpriced'
    if (cost.status === 'priced' && typeof cost.amount_micros === 'number') {
      if (!cost.currency) return `${formatNumber(cost.amount_micros)} micro-units`
      try {
        return new Intl.NumberFormat(undefined, { style: 'currency', currency: cost.currency, maximumFractionDigits: 6 })
          .format(cost.amount_micros / 1_000_000)
      } catch {
        return `${formatNumber(cost.amount_micros)} micro-units (${cost.currency})`
      }
    }
    return cost.status || 'Cost unavailable'
  }

  function formatTimestamp(value?: string): string {
    if (!value) return '—'
    const parsed = new Date(value)
    return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
  }
</script>

<section class="metrics-workspace" aria-labelledby="project-metrics-title">
  <header class="metrics-heading">
    <div>
      <p class="eyebrow">CORE-OWNED PROJECT METRICS</p>
      <h2 id="project-metrics-title">Project metrics</h2>
      <p class="metrics-subtitle">Operational time and agent token usage reported by TimeKeeper Core.</p>
    </div>
    <div class="metrics-controls">
      <label for="metrics-project">Project
        <select id="metrics-project" aria-label="Metrics project" value={selectedProjectID} onchange={chooseProject}>
          <option value="" disabled>Choose a Project</option>
          {#each projects as project (project.project_id)}
            <option value={project.project_id}>{project.project_name} · {project.project_id}</option>
          {/each}
        </select>
      </label>
      <button class="text-button" type="button" aria-label="Refresh metrics" disabled={!selectedProjectID || usageLoading || operationalLoading} onclick={() => void loadMetrics(selectedProjectID)}>Refresh metrics</button>
    </div>
  </header>

  {#if projects.length === 0}
    <div class="state-panel" role="status">No Projects are available in Core.</div>
  {:else if !selectedProjectID}
    <div class="state-panel" role="status">Choose a Project to inspect its metrics.</div>
  {:else}
    <div class="metrics-context" aria-live="polite">Showing Core metrics for <strong>{selectedProjectName}</strong> <code>{selectedProjectID}</code></div>

    <section class="metrics-panel" aria-labelledby="operational-metrics-title">
      <div class="metrics-panel-heading"><div><p class="eyebrow">EXECUTION SNAPSHOT</p><h3 id="operational-metrics-title">Project operations</h3></div><span class="read-only-tag">Read only · Core</span></div>
      {#if operationalLoading && !operationalSummary}
        <div class="metrics-state" role="status">Loading operational metrics from Core…</div>
      {:else if operationalError}
        <div class="metrics-state error" role="alert"><strong>Operational metrics unavailable</strong><p>{operationalError}</p></div>
      {:else if operationalSummary}
        <div class="metrics-kpis" aria-label="Sprint counts">
          <article><span>Total Sprints</span><strong>{formatNumber(operationalSummary.total_sprints)}</strong></article>
          <article><span>Open</span><strong>{formatNumber(operationalSummary.open_sprints)}</strong></article>
          <article><span>Active</span><strong>{formatNumber(operationalSummary.active_sprints)}</strong></article>
          <article><span>On hold</span><strong>{formatNumber(operationalSummary.held_sprints)}</strong></article>
          <article><span>Completed</span><strong>{formatNumber(operationalSummary.completed_sprints)}</strong></article>
          <article><span>Timed out / cancelled</span><strong>{formatNumber(operationalSummary.timed_out_sprints + operationalSummary.cancelled_sprints)}</strong></article>
        </div>
        <div class="metrics-time-grid" aria-label="Recorded and planned time">
          <article><span>Estimated</span><strong>{formatDuration(operationalSummary.estimated_duration_seconds)}</strong></article>
          <article><span>Buffer</span><strong>{formatDuration(operationalSummary.buffer_duration_seconds)}</strong></article>
          <article><span>Extensions</span><strong>{formatDuration(operationalSummary.extension_duration_seconds)}</strong></article>
          <article><span>Planned</span><strong>{formatDuration(operationalSummary.planned_duration_seconds)}</strong></article>
          <article><span>Recorded work</span><strong>{formatDuration(operationalSummary.recorded_work_seconds)}</strong></article>
          <article><span>Recorded hold</span><strong>{formatDuration(operationalSummary.recorded_hold_seconds)}</strong></article>
        </div>
        <p class="metrics-equation">Estimated + buffer + extensions = planned time. Recorded work and hold are persisted intervals, not a live timer.</p>
      {/if}
    </section>

    <section class="metrics-panel" aria-labelledby="usage-metrics-title">
      <div class="metrics-panel-heading"><div><p class="eyebrow">AGENT USAGE SNAPSHOT</p><h3 id="usage-metrics-title">Agent token usage</h3></div><span class="read-only-tag">Read only · Core</span></div>
      {#if usageLoading && !usageSummary}
        <div class="metrics-state" role="status">Loading usage metrics from Core…</div>
      {:else if usageError}
        <div class="metrics-state error" role="alert"><strong>Agent token usage unavailable</strong><p>{usageError}</p></div>
      {:else if usageSummary}
        <div class="metrics-kpis usage-kpis" aria-label="Agent token totals">
          <article><span>Input tokens</span><strong>{formatNumber(usageSummary.totals.input_tokens)}</strong></article>
          <article><span>Output tokens</span><strong>{formatNumber(usageSummary.totals.output_tokens)}</strong></article>
          <article><span>Cache creation</span><strong>{formatNumber(usageSummary.totals.cache_creation_tokens)}</strong></article>
          <article><span>Cache read</span><strong>{formatNumber(usageSummary.totals.cache_read_tokens)}</strong></article>
          <article><span>Messages</span><strong>{formatNumber(usageSummary.totals.messages)}</strong></article>
          <article><span>Sessions</span><strong>{formatNumber(usageSummary.totals.session_count)}</strong></article>
          <article><span>Cost status</span><strong>{formatCost(usageSummary.totals.cost)}</strong>{#if usageSummary.totals.cost.reason}<small>{usageSummary.totals.cost.reason}</small>{/if}</article>
        </div>

        <div class="metrics-tables">
          <section aria-labelledby="usage-sessions-title">
            <h4 id="usage-sessions-title">Sessions</h4>
            {#if (usageSummary.sessions?.length ?? 0) === 0}
              <p class="metrics-state">No agent usage sessions have been recorded.</p>
            {:else}
              <div class="metrics-table-scroll"><table aria-label="Agent usage sessions"><thead><tr><th>Agent / model</th><th>Source / coverage</th><th>Input</th><th>Output</th><th>Messages</th><th>Last activity</th></tr></thead><tbody>
                {#each usageSummary.sessions ?? [] as session (session.session_id)}
                  <tr><th scope="row"><strong>{session.agent_id}</strong><small>{session.model || 'Model not reported'}</small></th><td>{session.source}<small>{session.coverage}</small></td><td>{formatNumber(session.input_tokens)}</td><td>{formatNumber(session.output_tokens)}</td><td>{formatNumber(session.messages)}</td><td><time datetime={session.last_activity_at}>{formatTimestamp(session.last_activity_at)}</time></td></tr>
                {/each}
              </tbody></table></div>
            {/if}
          </section>
          <section aria-labelledby="usage-days-title">
            <h4 id="usage-days-title">Daily usage</h4>
            {#if (usageSummary.days?.length ?? 0) === 0}
              <p class="metrics-state">No daily usage deltas have been recorded.</p>
            {:else}
              <div class="metrics-table-scroll"><table aria-label="Daily token usage"><thead><tr><th>Date</th><th>Input</th><th>Output</th><th>Messages</th><th>Sessions</th></tr></thead><tbody>
                {#each usageSummary.days ?? [] as day (day.date)}
                  <tr><th scope="row">{day.date}</th><td>{formatNumber(day.input_tokens)}</td><td>{formatNumber(day.output_tokens)}</td><td>{formatNumber(day.messages)}</td><td>{formatNumber(day.session_count)}</td></tr>
                {/each}
              </tbody></table></div>
            {/if}
          </section>
        </div>
        <p class="metrics-equation">Cost is priced only by Core from its trusted local catalogue. Unpriced means no trusted price is configured; it is not zero.</p>
      {/if}
    </section>
  {/if}
</section>

<style>
  .metrics-workspace { display: grid; gap: 14px; }
  .metrics-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
  .metrics-heading h2 { margin: 0; color: #253246; font-size: 18px; }
  .metrics-heading .eyebrow { margin: 0 0 5px; }
  .metrics-subtitle { margin: 5px 0 0; color: #7a8798; font-size: 11px; }
  .metrics-controls { display: flex; align-items: end; gap: 10px; }
  .metrics-controls label { display: grid; gap: 5px; color: #7c8797; font-size: 9px; font-weight: 650; }
  .metrics-controls select { min-width: 210px; max-width: 320px; padding: 8px 10px; border: 1px solid var(--line, #e9edf2); border-radius: 7px; background: #fff; color: #344052; font: inherit; }
  .metrics-context { color: #7a8798; font-size: 10px; }
  .metrics-context strong { color: #344052; }
  .metrics-context code { margin-left: 5px; color: #718097; }
  .metrics-panel { display: grid; gap: 12px; padding: 14px; border: 1px solid var(--line, #e9edf2); border-radius: 10px; background: #fff; }
  .metrics-panel-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .metrics-panel-heading .eyebrow { margin: 0 0 4px; }
  .metrics-panel-heading h3 { margin: 0; color: #344052; font-size: 14px; }
  .metrics-kpis, .metrics-time-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(115px, 1fr)); gap: 8px; }
  .metrics-kpis article, .metrics-time-grid article { display: grid; align-content: start; gap: 5px; min-height: 50px; padding: 10px; border: 1px solid #eef1f4; border-radius: 8px; background: #fafbfd; }
  .metrics-kpis article span, .metrics-time-grid article span { color: #8994a3; font-size: 8px; font-weight: 700; letter-spacing: .45px; text-transform: uppercase; }
  .metrics-kpis article strong, .metrics-time-grid article strong { color: #344052; font-size: 13px; font-weight: 650; }
  .metrics-kpis article small { color: #858f9e; font-size: 9px; line-height: 1.4; overflow-wrap: anywhere; }
  .metrics-time-grid { padding-top: 1px; }
  .metrics-equation { margin: 0; color: #7f8a9a; font-size: 9px; line-height: 1.5; }
  .metrics-state { margin: 0; padding: 12px; border-radius: 7px; color: #7f8a9a; background: #fafbfd; font-size: 10px; }
  .metrics-state.error { border: 1px solid #f1d9d9; color: #a64242; background: #fff8f8; }
  .metrics-state.error p { margin: 4px 0 0; }
  .metrics-tables { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 12px; }
  .metrics-tables h4 { margin: 0 0 7px; color: #344052; font-size: 10px; }
  .metrics-table-scroll { overflow-x: auto; border: 1px solid #eef1f4; border-radius: 8px; }
  .metrics-table-scroll table { width: 100%; border-collapse: collapse; font-size: 9px; text-align: left; }
  .metrics-table-scroll th, .metrics-table-scroll td { padding: 8px; border-bottom: 1px solid #eef1f4; color: #5d687a; vertical-align: top; white-space: nowrap; }
  .metrics-table-scroll thead th { color: #8994a3; font-size: 8px; text-transform: uppercase; }
  .metrics-table-scroll tbody tr:last-child th, .metrics-table-scroll tbody tr:last-child td { border-bottom: 0; }
  .metrics-table-scroll tbody th { color: #344052; font-weight: 600; }
  .metrics-table-scroll small { display: block; margin-top: 3px; color: #929cac; font-size: 8px; font-weight: 400; }
  .metrics-table-scroll time { white-space: normal; }
  @media (max-width: 780px) {
    .metrics-heading { flex-direction: column; }
    .metrics-controls { width: 100%; align-items: end; justify-content: space-between; }
    .metrics-controls label { min-width: 0; flex: 1; }
    .metrics-controls select { width: 100%; min-width: 0; }
  }
</style>

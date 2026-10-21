<script lang="ts">
  import type { AgentWorkState } from './api'

  interface Props {
    states: AgentWorkState[] | null
    loading: boolean
    error: string
    onRefresh: () => void | Promise<void>
  }

  let { states, loading, error, onRefresh }: Props = $props()

  const stateSummary = $derived.by(() => {
    const counts = new Map<string, number>()
    for (const agent of states ?? []) counts.set(agent.state, (counts.get(agent.state) ?? 0) + 1)
    return [...counts.entries()].sort(([left], [right]) => left.localeCompare(right))
  })

  function formatTimestamp(value?: string | null): string {
    if (!value) return '—'
    const parsed = new Date(value)
    return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
  }

  function safeStateClass(value: string): string {
    return value.toLowerCase().replace(/[^a-z0-9_-]/g, '-')
  }
</script>

<section class="agent-pulse-workspace" aria-labelledby="agent-pulse-title">
  <header class="pulse-heading">
    <div>
      <p class="eyebrow">CORE-REPORTED AGENT ACTIVITY</p>
      <h2 id="agent-pulse-title">Current work states</h2>
      <p class="pulse-subtitle">A read-only snapshot of the work states currently held by TimeKeeper Core.</p>
    </div>
    <div class="pulse-heading-actions">
      <span class="read-only-tag">At last refresh</span>
      <button class="text-button" type="button" aria-label="Refresh agent pulse" onclick={() => void onRefresh()}>Refresh</button>
    </div>
  </header>

  {#if loading && states === null}
    <div class="state-panel" role="status">Loading agent state from Core…</div>
  {:else if error}
    <div class="state-panel error-panel" role="alert">
      <strong>Agent Pulse unavailable</strong>
      <p>{error}</p>
    </div>
  {:else if states === null}
    <div class="state-panel" role="status">Agent state has not been loaded yet.</div>
  {:else if states.length === 0}
    <div class="state-panel" role="status">No agents have reported work state.</div>
  {:else}
    <section class="pulse-summary" aria-label="Agent state summary">
      <strong>{states.length} {states.length === 1 ? 'agent' : 'agents'}</strong>
      {#each stateSummary as [state, count] (state)}
        <span class={`pulse-count state-${safeStateClass(state)}`}><b>{count}</b> {state}</span>
      {/each}
    </section>

    <div class="pulse-table-scroll">
      <table class="pulse-table" aria-label="Agent pulse states">
        <thead>
          <tr>
            <th scope="col">Agent</th>
            <th scope="col">Core state</th>
            <th scope="col">Project</th>
            <th scope="col">Active Sprint</th>
            <th scope="col">Last contact</th>
            <th scope="col">Last progress</th>
            <th scope="col">Context</th>
          </tr>
        </thead>
        <tbody>
          {#each states as agent (agent.agent_id)}
            <tr data-agent-id={agent.agent_id}>
              <th scope="row">
                <code>{agent.agent_id}</code>
                <small>{agent.runtime || 'Runtime not reported'}</small>
              </th>
              <td>
                <span class={`pulse-state state-${safeStateClass(agent.state)}`}>{agent.state}</span>
                <small>reported: {agent.reported_state || 'unknown'}</small>
              </td>
              <td>{#if agent.project_id}<code>{agent.project_id}</code>{:else}—{/if}</td>
              <td>{#if agent.active_sprint_id}<code>{agent.active_sprint_id}</code>{:else}—{/if}</td>
              <td><time datetime={agent.last_contact_at}>{formatTimestamp(agent.last_contact_at)}</time></td>
              <td><time datetime={agent.last_progress_at}>{formatTimestamp(agent.last_progress_at)}</time></td>
              <td>
                <span>{agent.reason || 'No reason reported'}</span>
                <small>{agent.work_expected ? 'Work expected' : 'No work expected'} · {agent.lease_duration_seconds ?? 0}s lease</small>
                {#if agent.wait_until}<small>Wait until {formatTimestamp(agent.wait_until)}</small>{/if}
                {#if agent.deadline}<small>Deadline {formatTimestamp(agent.deadline)}</small>{/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>

<style>
  .agent-pulse-workspace { display: grid; gap: 14px; }
  .pulse-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
  .pulse-heading h2 { margin: 0; color: #253246; font-size: 18px; }
  .pulse-heading .eyebrow { margin: 0 0 5px; }
  .pulse-subtitle { margin: 5px 0 0; color: #7a8798; font-size: 11px; }
  .pulse-heading-actions { display: flex; align-items: center; gap: 12px; }
  .pulse-summary { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 10px 12px; border: 1px solid var(--line, #e9edf2); border-radius: 9px; background: #fff; color: #647084; font-size: 10px; }
  .pulse-summary > strong { margin-right: 3px; color: #344052; font-size: 11px; }
  .pulse-count { display: inline-flex; align-items: center; gap: 4px; padding: 4px 8px; border-radius: 999px; background: #f3f5f8; text-transform: capitalize; }
  .pulse-count b { color: #334155; }
  .pulse-table-scroll { overflow-x: auto; border: 1px solid var(--line, #e9edf2); border-radius: 9px; background: #fff; }
  .pulse-table { width: 100%; border-collapse: collapse; font-size: 10px; text-align: left; }
  .pulse-table th, .pulse-table td { padding: 10px 11px; border-bottom: 1px solid #eef1f4; vertical-align: top; }
  .pulse-table thead th { color: #8792a1; font-size: 9px; font-weight: 650; text-transform: uppercase; white-space: nowrap; }
  .pulse-table tbody tr:last-child th, .pulse-table tbody tr:last-child td { border-bottom: 0; }
  .pulse-table tbody th { min-width: 140px; color: #344052; font-weight: 600; }
  .pulse-table td { color: #566276; min-width: 105px; }
  .pulse-table td:last-child { min-width: 190px; white-space: pre-wrap; overflow-wrap: anywhere; }
  .pulse-table code { color: #58657b; font-size: 9px; }
  .pulse-table small { display: block; margin-top: 4px; color: #929cac; font-size: 9px; font-weight: 400; }
  .pulse-state { display: inline-block; padding: 3px 7px; border-radius: 999px; color: #536073; background: #f3f5f8; text-transform: capitalize; }
  .state-working { color: #176c4a; background: #e8f7ef; }
  .state-blocked { color: #8a4d0d; background: #fff2dc; }
  .state-finished { color: #5b4c9b; background: #f0ecff; }
  .state-idle, .state-stopped { color: #687386; background: #eef1f4; }
  .state-available { color: #2563a7; background: #e9f3ff; }
  @media (max-width: 780px) {
    .pulse-heading { flex-direction: column; }
    .pulse-heading-actions { align-self: stretch; justify-content: space-between; }
  }
</style>

<script lang="ts">
  import type { ProjectMilestone } from './api'

  interface Props {
    milestones: ProjectMilestone[] | null
    loading: boolean
    error: string
    onRetry?: () => void
    onOpenTask?: (taskID: string) => void
  }

  let { milestones, loading, error, onRetry, onOpenTask }: Props = $props()

  function deadlineLabel(milestone: ProjectMilestone): string {
    const state = milestone.deadline_state
    const days = milestone.days_to_deadline
    if (state === 'overdue') {
      if (typeof days !== 'number') return 'Overdue'
      const elapsed = Math.abs(days)
      return `Overdue by ${elapsed} ${elapsed === 1 ? 'day' : 'days'}`
    }
    if (state === 'due_soon') {
      if (days === 0) return 'Due today'
      if (typeof days === 'number' && days > 0) return `Due in ${days} ${days === 1 ? 'day' : 'days'}`
      return 'Due soon'
    }
    if (state === 'on_track') return 'On track'
    if (state === 'completed') return 'Completed'
    if (state === 'cancelled') return 'Cancelled'
    if (state === 'unscheduled' || !state) return 'Deadline status unavailable'
    return state.replace(/_/g, ' ')
  }
</script>

<section class="milestone-island" aria-labelledby="milestone-timeline-title">
  <header>
    <div>
      <p class="eyebrow">PROJECT CHECKPOINTS</p>
      <h2 id="milestone-timeline-title">Milestone timeline</h2>
    </div>
    <span class="read-only-tag">Core-owned facts</span>
  </header>

  {#if loading}
    <div class="state-panel" role="status">Loading milestone data from Core…</div>
  {:else if error}
    <div class="state-panel error-panel" role="alert">
      <strong>Milestones unavailable</strong>
      <p>{error}</p>
      {#if onRetry}<button type="button" class="text-button" onclick={onRetry}>Retry milestones</button>{/if}
    </div>
  {:else if milestones === null}
    <div class="state-panel" role="status">
      <strong>Milestone data is not connected yet.</strong>
      <p>This view is ready to receive a Core-backed milestone list.</p>
    </div>
  {:else if milestones.length === 0}
    <div class="state-panel" role="status">No milestones recorded yet.</div>
  {:else}
    <ol class="milestone-list" aria-label="Project milestones">
      {#each milestones as milestone (milestone.milestone_id)}
        <li class="milestone-card" data-deadline-state={milestone.deadline_state ?? 'unknown'}>
          <div class="milestone-meta">
            <time datetime={milestone.target_date}>{milestone.target_date}</time>
            <span class="status-pill" data-status={milestone.status}>{milestone.status}</span>
            <span class="deadline-pill" data-state={milestone.deadline_state ?? 'unknown'}>{deadlineLabel(milestone)}</span>
          </div>
          <h3>{milestone.name}</h3>
          {#if milestone.description}<p class="milestone-description">{milestone.description}</p>{/if}
          {#if milestone.linked_task_id}
            {#if onOpenTask}
              <button class="task-link" type="button" aria-label={`Open Task ${milestone.linked_task_id}`} onclick={() => onOpenTask?.(milestone.linked_task_id!)}>
                Open Task {milestone.linked_task_id}
              </button>
            {:else}
              <span class="task-reference">Linked Task {milestone.linked_task_id}</span>
            {/if}
          {/if}
        </li>
      {/each}
    </ol>
  {/if}
</section>

<style>
  .milestone-island { display: grid; gap: 12px; min-width: 0; }
  header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .eyebrow { margin: 0 0 4px; color: #818c9b; font-size: 9px; font-weight: 700; letter-spacing: .06em; }
  h2 { margin: 0; color: #344052; font-size: 15px; }
  .read-only-tag { flex: 0 0 auto; padding: 4px 8px; border: 1px solid #e6eaf0; border-radius: 999px; color: #778395; font-size: 9px; }
  .state-panel { padding: 14px; border: 1px solid #e9edf2; border-radius: 8px; background: #fff; color: #536073; font-size: 11px; }
  .state-panel p { margin: 5px 0; color: #7f8a9a; }
  .error-panel { border-color: #f1cfcc; background: #fff8f7; }
  .milestone-list { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
  .milestone-card { min-width: 0; padding: 13px 14px; border: 1px solid #e9edf2; border-left: 3px solid #8b7be8; border-radius: 8px; background: #fff; }
  .milestone-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 7px; color: #7f8a9a; font-size: 10px; }
  .status-pill, .deadline-pill { padding: 3px 7px; border-radius: 999px; background: #f4f6f9; color: #586579; font-size: 9px; font-weight: 700; }
  .status-pill[data-status="Completed"], .deadline-pill[data-state="completed"] { background: #eff9f0; color: #28743a; }
  .deadline-pill[data-state="overdue"] { background: #fff0ef; color: #a33b33; }
  .deadline-pill[data-state="due_soon"] { background: #fff8e8; color: #886000; }
  .milestone-card h3 { margin: 8px 0 0; color: #344052; font-size: 12px; }
  .milestone-description { margin: 5px 0 0; color: #6f7b8b; font-size: 10px; line-height: 1.5; white-space: pre-wrap; overflow-wrap: anywhere; }
  .task-link { margin-top: 8px; padding: 0; border: 0; background: transparent; color: #6e58cc; font-size: 10px; cursor: pointer; }
  .task-link:focus-visible, .text-button:focus-visible { outline: 2px solid #8c78e7; outline-offset: 2px; }
  .task-reference { display: inline-block; margin-top: 8px; color: #7f8a9a; font-size: 10px; }
  .text-button { margin: 4px 0 0; padding: 0; border: 0; background: transparent; color: #6e58cc; font-size: 10px; cursor: pointer; }
</style>

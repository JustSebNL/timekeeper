import { render } from 'svelte/server'
import { describe, expect, it, vi } from 'vitest'
import ProjectWorkspace from './ProjectWorkspace.svelte'

const tree = {
  project: { project_id: 'P-7', project_name: 'Example', status: 'Open', project_goal: 'Ship safely' },
  categories: [{
    category: { category_id: 'C-1', name: 'Delivery', status: 'Open', item_address: '1' },
    categories: [{ category: { category_id: 'C-2', name: 'Quality', status: 'Open', item_address: '1.2' }, categories: [], tasks: [] }],
    tasks: [{
      task: { task_id: 'T-3', name: 'Build', status: 'Active', item_address: '1.3', calculated_completion_pct: 50 },
      sprints: [{ sprint_id: 'SP-4', item_address: '1.3.4', name: 'Implementation', status: 'Active' }],
      subtasks: [{
        subtask: { subtask_id: 'ST-5', name: 'Verify', status: 'Open', item_address: '1.3.5', calculated_completion_pct: 0 },
        sprints: [{ sprint_id: 'SP-6', item_address: '1.3.5.6', name: 'Acceptance', status: 'Open' }],
      }],
    }],
  }],
}

describe('ProjectWorkspace', () => {
  it('renders project metadata and the nested executable hierarchy read-only', () => {
    const { body } = render(ProjectWorkspace, {
      props: { tree, loading: false, error: '', onBack: vi.fn(), onRetry: vi.fn(), onSelectTask: vi.fn() },
    })

    expect(body).toContain('Example')
    expect(body).toContain('Ship safely')
    expect(body).toContain('aria-label="Categories, Tasks, Subtasks and Sprints"')
    expect(body).toContain('Delivery')
    expect(body).toContain('Build')
    expect(body).toContain('Implementation')
    expect(body).toContain('Verify')
    expect(body).toContain('Acceptance')
    expect(body).toContain('Back to projects')
    expect(body).toContain('aria-label="Open Task Build workspace"')
  })

  it('renders Trail of Thought messages separately from events and notes', () => {
    const { body } = render(ProjectWorkspace, {
      props: {
        tree,
        loading: false,
        error: '',
        handoffLoading: false,
        handoffError: '',
        handoff: {
          project: { project_id: 'P-7', project_name: 'Example', status: 'Open' },
          recent_events: [],
          recent_notes: [],
          recent_messages: [{
            message_id: 12,
            ref: 'M-12',
            project_id: 'P-7',
            kind: 'decision',
            author: 'Seb',
            body: 'Keep Core as the single authority.',
            link: 'docs/two-box-architecture.md',
            tags: 'architecture,core',
            revision: 2,
            pinned: true,
            created_at: '2026-10-18T15:00:00Z',
            updated_at: '2026-10-18T15:10:00Z',
          }],
        },
        onBack: vi.fn(),
        onRetry: vi.fn(),
        onSelectTask: vi.fn(),
      },
    })

    expect(body).toContain('Trail of Thought')
    expect(body).toContain('M-12')
    expect(body).toContain('decision')
    expect(body).toContain('Seb')
    expect(body).toContain('Keep Core as the single authority.')
    expect(body).toContain('architecture,core')
    expect(body).toContain('Pinned')
  })

  it('renders Core events and project notes as separate read-only activity surfaces', () => {
    const { body } = render(ProjectWorkspace, {
      props: {
        tree,
        loading: false,
        error: '',
        handoffLoading: false,
        handoffError: '',
        handoff: {
          project: { project_id: 'P-7', project_name: 'Example', status: 'Open' },
          recent_events: [{ event_id: 3, entity_type: 'task', entity_id: 'T-3', event_type: 'task_started', message: 'Task started', created_at: '2026-10-18T10:00:00Z' }],
          recent_notes: [{ note_id: 7, project_id: 'P-7', content: 'Capture the decision', created_at: '2026-10-18T09:00:00Z' }],
        },
        onBack: vi.fn(),
        onRetry: vi.fn(),
        onSelectTask: vi.fn(),
      },
    })

    expect(body).toContain('Recent activity')
    expect(body).toContain('Task started')
    expect(body).toContain('aria-label="Open Task T-3 workspace"')
    expect(body).toContain('Recent project notes')
    expect(body).toContain('Capture the decision')
  })

  it('renders the Core-provided Project schedule and deadline state without recalculating it', () => {
    const scheduledTree = {
      ...tree,
      project: {
        ...tree.project,
        planned_start_date: '2026-10-01',
        target_end_date: '2026-10-20',
        lifecycle_phase: 'execution',
        schedule_timezone: 'Europe/Amsterdam',
        deadline_warning_days: 7,
        deadline_state: 'due_soon',
        days_to_deadline: 1,
      },
    }
    const { body } = render(ProjectWorkspace, {
      props: { tree: scheduledTree, loading: false, error: '', onBack: vi.fn(), onRetry: vi.fn(), onSelectTask: vi.fn() },
    })

    expect(body).toContain('Project schedule')
    expect(body).toContain('Planned start')
    expect(body).toContain('2026-10-01')
    expect(body).toContain('Target end')
    expect(body).toContain('2026-10-20')
    expect(body).toContain('execution')
    expect(body).toContain('Europe/Amsterdam')
    expect(body).toContain('Due in 1 day')
    expect(body).toContain('Due-soon warning')
    expect(body).toContain('7 days')
  })

  it('shows an honest unscheduled state when Core has no target date', () => {
    const { body } = render(ProjectWorkspace, {
      props: { tree, loading: false, error: '', onBack: vi.fn(), onRetry: vi.fn(), onSelectTask: vi.fn() },
    })

    expect(body).toContain('Project schedule')
    expect(body).toContain('No target date set')
    expect(body).not.toContain('Actual start')
    expect(body).not.toContain('Completed at')
  })

  it('shows observed lifecycle timestamps from Core without inferring them', () => {
    const closedTree = {
      ...tree,
      project: {
        ...tree.project,
        status: 'Completed',
        lifecycle_phase: 'closeout',
        actual_started_at: '2026-10-02T11:00:00Z',
        completed_at: '2026-10-20T12:00:00Z',
        deadline_state: 'completed',
      },
    }
    const { body } = render(ProjectWorkspace, {
      props: { tree: closedTree, loading: false, error: '', onBack: vi.fn(), onRetry: vi.fn(), onSelectTask: vi.fn() },
    })

    expect(body).toContain('Actual start')
    expect(body).toContain('Completed at')
    expect(body).toContain('datetime="2026-10-02T11:00:00Z"')
    expect(body).toContain('datetime="2026-10-20T12:00:00Z"')
  })
})

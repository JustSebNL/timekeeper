// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ProjectOperationalSummary, ProjectSummary, ProjectUsageSummary } from './api'
import MetricsWorkspace from './MetricsWorkspace.svelte'

afterEach(cleanup)

const projects: ProjectSummary[] = [{ project_id: 'P-1', project_name: 'Project one', status: 'Open' }]
const operational: ProjectOperationalSummary = {
  project_id: 'P-1', total_sprints: 4, open_sprints: 1, active_sprints: 1, held_sprints: 0,
  completed_sprints: 2, timed_out_sprints: 0, cancelled_sprints: 0,
  estimated_duration_seconds: 7200, buffer_duration_seconds: 600, extension_duration_seconds: 300,
  planned_duration_seconds: 8100, recorded_work_seconds: 3600, recorded_hold_seconds: 0,
}
const usage: ProjectUsageSummary = {
  project_id: 'P-1',
  totals: { input_tokens: 1200, output_tokens: 350, cache_creation_tokens: 80, cache_read_tokens: 40, messages: 5, session_count: 1, cost: { status: 'unpriced', reason: 'no trusted local price is configured' } },
  sessions: [{ session_id: 'session-1', project_id: 'P-1', agent_id: 'xatia', model: 'nemotron-3-ultra', source: 'manual', coverage: 'explicit_snapshot', turn_seq: 4, input_tokens: 1200, output_tokens: 350, cache_creation_tokens: 80, cache_read_tokens: 40, messages: 5, cost: { status: 'unpriced', reason: 'no trusted local price is configured' }, last_activity_at: '2026-10-18T18:00:00Z', created_at: '2026-10-18T17:00:00Z', updated_at: '2026-10-18T18:00:00Z' }],
  days: [{ date: '2026-10-18', input_tokens: 1200, output_tokens: 350, cache_creation_tokens: 80, cache_read_tokens: 40, messages: 5, session_count: 1 }],
}

describe('MetricsWorkspace', () => {
  it('shows a clear empty state without calling Core when no Projects exist', () => {
    const api = { getProjectOperationalSummary: vi.fn(), getProjectUsageSummary: vi.fn() }
    render(MetricsWorkspace, { props: { projects: [], initialProjectID: null, api } })

    expect(screen.getByRole('status').textContent).toContain('No Projects are available in Core.')
    expect(api.getProjectOperationalSummary).not.toHaveBeenCalled()
    expect(api.getProjectUsageSummary).not.toHaveBeenCalled()
  })

  it('keeps operational data visible when usage fails and refreshes the failed panel', async () => {
    const api = {
      getProjectOperationalSummary: vi.fn().mockResolvedValue(operational),
      getProjectUsageSummary: vi.fn().mockRejectedValueOnce(new Error('usage endpoint unavailable')).mockResolvedValue(usage),
    }
    render(MetricsWorkspace, { props: { projects, initialProjectID: 'P-1', api } })

    expect(await screen.findByText('Total Sprints')).not.toBeNull()
    expect(screen.getByText('usage endpoint unavailable')).not.toBeNull()
    expect(screen.getByText('2h 15m')).not.toBeNull()

    await fireEvent.click(screen.getByRole('button', { name: 'Refresh metrics' }))
    expect(await screen.findByText('Unpriced')).not.toBeNull()
    expect(screen.getByText('nemotron-3-ultra')).not.toBeNull()
    expect(screen.getByText('2026-10-18')).not.toBeNull()
    expect(api.getProjectUsageSummary).toHaveBeenCalledTimes(2)
    await waitFor(() => expect(api.getProjectOperationalSummary).toHaveBeenCalledTimes(2))
  })

  it('loads the summaries for the newly selected Project', async () => {
    const secondProject: ProjectSummary = { project_id: 'P-2', project_name: 'Project two', status: 'Open' }
    const secondUsage = { ...usage, project_id: 'P-2', totals: { ...usage.totals, input_tokens: 2222 } }
    const api = {
      getProjectOperationalSummary: vi.fn().mockImplementation((projectID: string) => Promise.resolve({ ...operational, project_id: projectID })),
      getProjectUsageSummary: vi.fn().mockImplementation((projectID: string) => Promise.resolve(projectID === 'P-2' ? secondUsage : usage)),
    }
    render(MetricsWorkspace, { props: { projects: [...projects, secondProject], initialProjectID: 'P-1', api } })
    await screen.findByText('Agent token usage')

    await fireEvent.change(screen.getByLabelText('Metrics project'), { target: { value: 'P-2' } })
    expect(await screen.findByText('2,222')).not.toBeNull()
    expect(api.getProjectUsageSummary).toHaveBeenLastCalledWith('P-2')
    await waitFor(() => expect(api.getProjectOperationalSummary).toHaveBeenLastCalledWith('P-2'))
  })
})

// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AgentWorkState } from './api'
import AgentPulseWorkspace from './AgentPulseWorkspace.svelte'

afterEach(cleanup)

const states: AgentWorkState[] = [
  {
    agent_id: 'agent-working',
    state: 'working',
    reported_state: 'working',
    runtime: 'codex',
    project_id: 'P-10053',
    active_sprint_id: 'SP-11644',
    reason: 'Implementing the pulse workspace',
    work_expected: true,
    last_contact_at: '2026-10-18T17:00:00Z',
    last_progress_at: '2026-10-18T16:59:00Z',
    state_changed_at: '2026-10-18T16:50:00Z',
    deadline: '2026-10-18T17:15:00Z',
    wait_until: null,
    lease_duration_seconds: 900,
  },
  {
    agent_id: 'agent-blocked',
    state: 'blocked',
    reported_state: 'working',
    runtime: 'hermes',
    project_id: 'P-10053',
    active_sprint_id: 'SP-11643',
    reason: 'Waiting for the Core response',
    work_expected: true,
    last_contact_at: '2026-10-18T16:58:00Z',
    last_progress_at: '2026-10-18T16:40:00Z',
    state_changed_at: '2026-10-18T16:45:00Z',
    deadline: null,
    wait_until: null,
    lease_duration_seconds: 900,
  },
]

describe('AgentPulseWorkspace', () => {
  it('presents Core-computed state and current work context without reclassifying it', async () => {
    const onRefresh = vi.fn()
    render(AgentPulseWorkspace, { props: { states, loading: false, error: '', onRefresh } })

    expect(screen.getByRole('heading', { name: 'Current work states' })).not.toBeNull()
    expect(screen.getByLabelText('Agent pulse states')).not.toBeNull()
    expect(screen.getByText('agent-working')).not.toBeNull()
    expect(screen.getAllByText('working')).toHaveLength(2)
    expect(screen.getAllByText('blocked')).toHaveLength(2)
    expect(screen.getByText('Waiting for the Core response')).not.toBeNull()
    expect(screen.getAllByText('P-10053')).toHaveLength(2)
    expect(screen.getByText('SP-11644')).not.toBeNull()
    expect(screen.getByText('2 agents')).not.toBeNull()

    await fireEvent.click(screen.getByRole('button', { name: 'Refresh agent pulse' }))
    expect(onRefresh).toHaveBeenCalledOnce()
  })

  it('distinguishes an empty Core snapshot from an unavailable snapshot', () => {
    const { rerender } = render(AgentPulseWorkspace, { props: { states: [], loading: false, error: '', onRefresh: vi.fn() } })
    expect(screen.getByText('No agents have reported work state.')).not.toBeNull()

    rerender({ states: null, loading: false, error: '503 Service Unavailable', onRefresh: vi.fn() })
    expect(screen.getByRole('alert').textContent).toContain('503 Service Unavailable')
  })
})

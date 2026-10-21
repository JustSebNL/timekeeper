// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  getHealth: vi.fn().mockResolvedValue({ status: 'ok' }),
  getProjects: vi.fn().mockResolvedValue([]),
  getAgentWorkStates: vi.fn().mockResolvedValue([]),
  getExecutionTree: vi.fn(),
  getTaskDetail: vi.fn(),
  updateTaskStatus: vi.fn(),
  transitionSprint: vi.fn(),
}))
const persistence = vi.hoisted(() => ({
  readSelectedProjectID: vi.fn(() => null),
  readWorkspaceSection: vi.fn(() => 'overview'),
  saveSelectedProjectID: vi.fn(),
  saveWorkspaceSection: vi.fn(),
}))

vi.mock('./lib/api', () => ({ timekeeperApi: api }))
vi.mock('./lib/workspacePersistence', () => persistence)

import App from './App.svelte'

afterEach(cleanup)

describe('Svelte workspace shell', () => {
  it('composes active Overview and Projects navigation with upcoming workspaces', async () => {
    render(App)

    expect(screen.getByRole('navigation', { name: 'Workspace sections' })).not.toBeNull()
    expect(screen.getByRole('button', { name: 'Overview' }).getAttribute('aria-current')).toBe('page')
    expect(screen.getByRole('button', { name: 'Projects' }).getAttribute('data-workspace-section')).toBe('projects')
    expect(screen.getByLabelText('Search TimeKeeper')).not.toBeNull()
    expect(screen.getByLabelText('Workspace summary')).not.toBeNull()
    expect(screen.getByLabelText('Main navigation').querySelector('[data-workspace-disabled="tasks"]')).not.toBeNull()
    expect(screen.getByRole('button', { name: 'Memory' }).getAttribute('data-workspace-section')).toBe('memory')
  })

  it('shows the Core-reported count of currently working agents on Overview', async () => {
    api.getAgentWorkStates.mockClear()
    api.getAgentWorkStates.mockResolvedValueOnce([
      { agent_id: 'agent-working', state: 'working' },
      { agent_id: 'agent-waiting', state: 'waiting' },
    ])
    render(App)

    await screen.findByText('AT LAST REFRESH')
    await waitFor(() => expect(api.getAgentWorkStates).toHaveBeenCalledOnce())
    const card = screen.getByText('Agents working').closest('article')
    expect(card?.textContent).toContain('1')
    expect(card?.textContent).toContain('AT LAST REFRESH')
  })
})

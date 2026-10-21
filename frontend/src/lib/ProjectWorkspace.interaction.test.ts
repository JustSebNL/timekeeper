// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ProjectWorkspace from './ProjectWorkspace.svelte'

const tree = {
  project: { project_id: 'P-7', project_name: 'Example', status: 'Open' },
  categories: [],
}

afterEach(cleanup)

describe('ProjectWorkspace activity links', () => {
  it('opens a Task referenced by a Core event through the existing Task workspace callback', async () => {
    const onSelectTask = vi.fn()
    render(ProjectWorkspace, {
      props: {
        tree,
        loading: false,
        error: '',
        handoffLoading: false,
        handoffError: '',
        handoff: {
          project: tree.project,
          recent_events: [{
            event_id: 3,
            entity_type: 'task',
            entity_id: 'T-3',
            event_type: 'task_started',
            message: 'Task started',
            created_at: '2026-10-18T10:00:00Z',
          }],
          recent_notes: [],
        },
        onBack: vi.fn(),
        onRetry: vi.fn(),
        onSelectTask,
      },
    })

    await fireEvent.click(screen.getByRole('button', { name: 'Open Task T-3 workspace' }))
    expect(onSelectTask).toHaveBeenCalledWith('T-3')
  })
})

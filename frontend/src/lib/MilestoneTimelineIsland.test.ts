// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import MilestoneTimelineIsland from './MilestoneTimelineIsland.svelte'

afterEach(cleanup)

describe('MilestoneTimelineIsland', () => {
  it('makes the disconnected state explicit without inventing an empty Core result', () => {
    render(MilestoneTimelineIsland, {
      props: { milestones: null, loading: false, error: '', onRetry: vi.fn(), onOpenTask: vi.fn() },
    })

    expect(screen.getByRole('status').textContent).toContain('Milestone data is not connected yet.')
  })

  it('shows an explicit loading state', () => {
    render(MilestoneTimelineIsland, {
      props: { milestones: null, loading: true, error: '' },
    })

    expect(screen.getByRole('status').textContent).toContain('Loading milestone data from Core')
  })

  it('shows Core errors with an optional retry callback', async () => {
    const onRetry = vi.fn()
    render(MilestoneTimelineIsland, {
      props: { milestones: null, loading: false, error: 'Core unavailable', onRetry },
    })

    expect(screen.getByRole('alert').textContent).toContain('Core unavailable')
    await fireEvent.click(screen.getByRole('button', { name: 'Retry milestones' }))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('distinguishes an empty Core result from disconnected data', () => {
    render(MilestoneTimelineIsland, {
      props: { milestones: [], loading: false, error: '' },
    })

    expect(screen.getByRole('status').textContent).toContain('No milestones recorded yet.')
  })

  it('renders Core milestone facts and delegates linked Task navigation', async () => {
    const onOpenTask = vi.fn()
    render(MilestoneTimelineIsland, {
      props: {
        milestones: [{
          milestone_id: 'MS-789',
          project_id: 'P-123',
          name: 'Beta gate',
          description: 'Ready for acceptance testing',
          target_date: '2026-10-22',
          status: 'Planned',
          linked_task_id: 'T-123',
          created_at: '2026-10-01T10:00:00Z',
          updated_at: '2026-10-01T10:00:00Z',
          deadline_state: 'due_soon',
          days_to_deadline: 2,
        }],
        loading: false,
        error: '',
        onOpenTask,
      },
    })

    expect(screen.getByRole('heading', { name: 'Milestone timeline' })).toBeTruthy()
    expect(screen.getByText('Beta gate')).toBeTruthy()
    expect(screen.getByText('Ready for acceptance testing')).toBeTruthy()
    expect(screen.getByText('2026-10-22')).toBeTruthy()
    expect(screen.getByText('Planned')).toBeTruthy()
    expect(screen.getByText('Due in 2 days')).toBeTruthy()
    await fireEvent.click(screen.getByRole('button', { name: 'Open Task T-123' }))
    expect(onOpenTask).toHaveBeenCalledWith('T-123')
  })
})

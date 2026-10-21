import { describe, expect, it, vi } from 'vitest'
import { runAndRefreshTaskWorkspace, sprintActionsForStatus, sprintActionRequiresReason, taskStatusRequiresConfirmation, retainAvailableSprintAction } from './taskActions'

describe('Sprint action availability', () => {
  it.each([
    ['Open', ['start', 'hold', 'cancel']],
    ['Active', ['hold', 'complete', 'cancel']],
    ['On Hold', ['resume', 'cancel']],
    ['Completed', []],
    ['Cancelled', []],
    ['TimedOut', []],
    ['unknown', []],
  ])('offers only Core actions for %s', (status, expected) => {
    expect(sprintActionsForStatus(status)).toEqual(expected)
  })
})

describe('explicit confirmation requirements', () => {
  it('requires a reason for hold and cancel, but not other Sprint actions', () => {
    expect(sprintActionRequiresReason('hold')).toBe(true)
    expect(sprintActionRequiresReason('cancel')).toBe(true)
    expect(sprintActionRequiresReason('start')).toBe(false)
    expect(sprintActionRequiresReason('resume')).toBe(false)
    expect(sprintActionRequiresReason('complete')).toBe(false)
  })

  it('requires confirmation before cancelling a Task', () => {
    expect(taskStatusRequiresConfirmation('Cancelled')).toBe(true)
    expect(taskStatusRequiresConfirmation('Completed')).toBe(false)
  })

  it('clears a pending reason action when refreshed status no longer permits it', () => {
    expect(retainAvailableSprintAction('hold', [])).toBeNull()
    expect(retainAvailableSprintAction('cancel', ['resume'])).toBeNull()
    expect(retainAvailableSprintAction('hold', ['hold', 'complete'])).toBe('hold')
    expect(retainAvailableSprintAction(null, ['hold'])).toBeNull()
  })
})

describe('authoritative refresh after Task workspace actions', () => {
  it('refreshes after a successful Core action before resolving', async () => {
    const action = vi.fn().mockResolvedValue(undefined)
    const refresh = vi.fn().mockResolvedValue(true)

    await expect(runAndRefreshTaskWorkspace(action, refresh, 'refresh failed')).resolves.toBeUndefined()
    expect(action).toHaveBeenCalledOnce()
    expect(refresh).toHaveBeenCalledOnce()
  })

  it('refreshes after a Core conflict and retains the original error', async () => {
    const conflict = new Error('Core request failed (409 Conflict).')
    const action = vi.fn().mockRejectedValue(conflict)
    const refresh = vi.fn().mockResolvedValue(true)

    await expect(runAndRefreshTaskWorkspace(action, refresh, 'refresh failed')).rejects.toBe(conflict)
    expect(refresh).toHaveBeenCalledOnce()
  })

  it('does not treat a failed post-write refresh as confirmed current state', async () => {
    const action = vi.fn().mockResolvedValue(undefined)
    const refresh = vi.fn().mockResolvedValue(false)

    await expect(runAndRefreshTaskWorkspace(action, refresh, 'Core accepted the action, but refresh failed.')).rejects.toThrow('Core accepted the action')
  })
})

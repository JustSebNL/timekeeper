// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import TaskWorkspace from './TaskWorkspace.svelte'
import type { SprintAction, TaskDetail, TaskStatus } from './api'

afterEach(cleanup)

function taskDetail(sprintStatus = 'Active'): TaskDetail {
  return {
    task: { task_id: 'T-1', project_id: 'P-1', name: 'Task one', status: 'Open' },
    subtasks: [],
    sprints: [{ sprint_id: 'SP-1', name: 'Sprint one', status: sprintStatus }],
    children: { subtask_ids: [], sprint_ids: ['SP-1'], open_subtasks: 0, open_sprints: sprintStatus === 'Completed' ? 0 : 1, can_complete: sprintStatus === 'Completed' },
  } as TaskDetail
}

interface WorkspaceProps {
  detail: TaskDetail | null
  loading: boolean
  error: string
  busy: boolean
  onBack: () => void
  onRetry: () => void
  onTaskStatusChange: (status: TaskStatus) => Promise<void>
  onSprintAction: (sprintID: string, action: SprintAction, reason: string) => Promise<void>
}

function mountWorkspace(overrides: Partial<WorkspaceProps> = {}) {
  return render(TaskWorkspace, {
    props: {
      detail: taskDetail(),
      loading: false,
      error: '',
      busy: false,
      onBack: vi.fn(),
      onRetry: vi.fn(),
      onTaskStatusChange: vi.fn().mockResolvedValue(undefined),
      onSprintAction: vi.fn().mockResolvedValue(undefined),
      ...overrides,
    },
  })
}

describe('TaskWorkspace lifecycle controls', () => {
  it('requires explicit confirmation before cancelling a Task', async () => {
    const onTaskStatusChange = vi.fn().mockResolvedValue(undefined)
    mountWorkspace({ onTaskStatusChange })

    await fireEvent.change(screen.getByLabelText('Set Task status'), { target: { value: 'Cancelled' } })
    expect((screen.getByRole('button', { name: 'Update Task status' }) as HTMLButtonElement).disabled).toBe(true)

    await fireEvent.click(screen.getByLabelText('Confirm Task cancellation'))
    await fireEvent.click(screen.getByRole('button', { name: 'Update Task status' }))
    expect(onTaskStatusChange).toHaveBeenCalledWith('Cancelled')
  })

  it('moves focus to the Task heading when the detail view opens', async () => {
    mountWorkspace()
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Task one' }))
  })

  it('requires a nonblank reason before putting a Sprint on hold', async () => {
    const onSprintAction = vi.fn().mockResolvedValue(undefined)
    mountWorkspace({ onSprintAction })

    await fireEvent.click(screen.getByRole('button', { name: 'Put Sprint on hold' }))
    const submit = screen.getByRole('button', { name: 'Hold Sprint' })
    expect((submit as HTMLButtonElement).disabled).toBe(true)

    await fireEvent.input(screen.getByLabelText('Reason to hold Sprint'), { target: { value: 'waiting for review' } })
    await fireEvent.click(submit)
    expect(onSprintAction).toHaveBeenCalledWith('SP-1', 'hold', 'waiting for review')
  })

  it('removes a pending reason form when refreshed Sprint status makes its action invalid', async () => {
    const view = mountWorkspace()

    await fireEvent.click(screen.getByRole('button', { name: 'Put Sprint on hold' }))
    expect(screen.getByLabelText('Reason to hold Sprint')).not.toBeNull()

    await view.rerender({ detail: taskDetail('Completed'), loading: false, error: '', busy: false })
    expect(screen.queryByLabelText('Reason to hold Sprint')).toBeNull()
  })

  it('does not submit a Sprint action twice while the first request is pending', async () => {
    let resolveAction: (() => void) | undefined
    const onSprintAction = vi.fn(() => new Promise<void>((resolve) => { resolveAction = resolve }))
    mountWorkspace({ onSprintAction })

    await fireEvent.click(screen.getByRole('button', { name: 'Put Sprint on hold' }))
    await fireEvent.input(screen.getByLabelText('Reason to hold Sprint'), { target: { value: 'waiting for review' } })
    const submit = screen.getByRole('button', { name: 'Hold Sprint' })
    const first = fireEvent.click(submit)
    const second = fireEvent.click(submit)
    await Promise.all([first, second])

    expect(onSprintAction).toHaveBeenCalledOnce()
    resolveAction?.()
  })
})

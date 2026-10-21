import { describe, expect, it, vi } from 'vitest'
import { loadTaskWorkspaceSnapshot, TaskWorkspaceRequestGuard } from './taskWorkspace'
import type { ProjectExecutionTree, TaskDetail } from './api'

describe('Task workspace authoritative reads', () => {
  it('loads Task detail and its owning Project hierarchy together', async () => {
    const detail = { task: { task_id: 'T-1', project_id: 'P-2', name: 'Task', status: 'Open' } } as TaskDetail
    const tree = { project: { project_id: 'P-2', project_name: 'Project', status: 'Open' }, categories: [] } as ProjectExecutionTree
    const api = {
      getTaskDetail: vi.fn().mockResolvedValue(detail),
      getExecutionTree: vi.fn().mockResolvedValue(tree),
    }

    await expect(loadTaskWorkspaceSnapshot(api, 'T-1', 'P-2')).resolves.toEqual({ detail, tree })
    expect(api.getTaskDetail).toHaveBeenCalledWith('T-1')
    expect(api.getExecutionTree).toHaveBeenCalledWith('P-2')
  })

  it('rejects mismatched Project context and does not return a partial refresh', async () => {
    const api = {
      getTaskDetail: vi.fn().mockResolvedValue({ task: { task_id: 'T-1', project_id: 'P-other' } }),
      getExecutionTree: vi.fn().mockResolvedValue({ project: { project_id: 'P-2' }, categories: [] }),
    }

    await expect(loadTaskWorkspaceSnapshot(api, 'T-1', 'P-2')).rejects.toThrow('does not belong to the selected Project')
    expect(api.getTaskDetail).toHaveBeenCalledOnce()
    expect(api.getExecutionTree).toHaveBeenCalledOnce()
  })

  it('rejects instead of returning either half when one authoritative read fails', async () => {
    const api = {
      getTaskDetail: vi.fn().mockResolvedValue({ task: { task_id: 'T-1', project_id: 'P-2' } }),
      getExecutionTree: vi.fn().mockRejectedValue(new Error('Core hierarchy unavailable')),
    }

    await expect(loadTaskWorkspaceSnapshot(api, 'T-1', 'P-2')).rejects.toThrow('Core hierarchy unavailable')
  })
})

describe('Task workspace request guard', () => {
  it('does not allow an older Task load to commit after a newer selection', () => {
    const guard = new TaskWorkspaceRequestGuard()
    const taskA = guard.begin('T-A', 'P-1')
    const taskB = guard.begin('T-B', 'P-1')

    expect(taskA.isCurrent('T-A', 'P-1')).toBe(false)
    expect(taskB.isCurrent('T-B', 'P-1')).toBe(true)
    expect(taskB.isCurrent('T-A', 'P-1')).toBe(false)
  })

  it('invalidates an in-flight Task load when leaving the workspace', () => {
    const guard = new TaskWorkspaceRequestGuard()
    const request = guard.begin('T-A', 'P-1')

    guard.invalidate()

    expect(request.isCurrent('T-A', 'P-1')).toBe(false)
  })
})

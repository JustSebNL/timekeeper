import { describe, expect, it, vi } from 'vitest'
import { createTimekeeperApi } from './api'

describe('TimeKeeper API client', () => {
  it('reads health and unwraps the project collection envelope', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ status: 'ok' }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        items: [{ project_id: 'P-1', project_name: 'Example', status: 'Open' }],
      }), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getHealth()).resolves.toEqual({ status: 'ok' })
    await expect(api.getProjects()).resolves.toEqual([
      { project_id: 'P-1', project_name: 'Example', status: 'Open' },
    ])
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual(['/health', '/api/v1/projects'])
  })

  it('tests the selected browser endpoint with a read-only compatibility request', async () => {
    const connectionInfo = {
      service: 'TimeKeeper',
      instance_id: 'tk_instance_0123456789abcdef0123456789abcdef',
      version: '0.0.1-alpha',
      api_version: 'v1',
      capabilities: ['projects.list', 'tasks.lifecycle'],
      friendly_urls: ['http://timekeeper.local/'],
    }
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify(connectionInfo), { status: 200 }))
    const api = createTimekeeperApi('http://timekeeper.local', fetcher)

    await expect(api.testConnection()).resolves.toEqual({
      client_surface: 'browser',
      selected_endpoint: 'http://timekeeper.local',
      used_fallback: false,
      connection_info: connectionInfo,
    })
    expect(fetcher).toHaveBeenCalledWith('http://timekeeper.local/api/v1/connection-info', { headers: { Accept: 'application/json' } })
  })

  it('uses the Wails read-only TestConnection binding when available', async () => {
    const diagnostic = {
      client_surface: 'desktop',
      selected_endpoint: 'http://127.0.0.1:1618',
      used_fallback: true,
      connection_info: {
        service: 'TimeKeeper',
        instance_id: 'tk_instance_0123456789abcdef0123456789abcdef',
        version: '0.0.1-alpha',
        api_version: 'v1',
        capabilities: ['projects.list', 'tasks.lifecycle'],
        friendly_urls: ['http://timekeeper.local/'],
      },
    }
    const desktop = {
      Health: vi.fn().mockResolvedValue({ status: 'ok' }),
      Projects: vi.fn().mockResolvedValue([]),
      TestConnection: vi.fn().mockResolvedValue(diagnostic),
    }
    const fetcher = vi.fn()
    const api = createTimekeeperApi('', fetcher, desktop)

    await expect(api.testConnection()).resolves.toEqual(diagnostic)
    expect(desktop.TestConnection).toHaveBeenCalledOnce()
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('reports incompatible API metadata instead of calling it a successful connection', async () => {
    const connectionInfo = {
      service: 'TimeKeeper',
      instance_id: 'tk_instance_0123456789abcdef0123456789abcdef',
      version: '0.0.1-alpha',
      api_version: 'v2',
      capabilities: ['projects.list', 'tasks.lifecycle'],
      friendly_urls: [],
    }
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify(connectionInfo), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.testConnection()).rejects.toThrow('API version "v2" is incompatible; expected v1.')
  })

  it('reports missing required capabilities as an incompatible connection', async () => {
    const connectionInfo = {
      service: 'TimeKeeper',
      instance_id: 'tk_instance_0123456789abcdef0123456789abcdef',
      version: '0.0.1-alpha',
      api_version: 'v1',
      capabilities: ['projects.list'],
      friendly_urls: [],
    }
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify(connectionInfo), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.testConnection()).rejects.toThrow('Required Core capability "tasks.lifecycle" is unavailable.')
  })


  it('reads the recursive Project execution tree over the Core HTTP contract', async () => {
    const tree = {
      project: { project_id: 'P-7', project_name: 'Example', status: 'Open' },
      categories: [{ category: { category_id: 'C-2', name: 'Delivery', status: 'Open' }, categories: [], tasks: [] }],
    }
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify(tree), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getExecutionTree('P-7')).resolves.toEqual(tree)
    expect(fetcher).toHaveBeenCalledWith('/api/v1/projects/P-7/execution-tree', { headers: { Accept: 'application/json' } })
  })

  it('reads Task detail and sends lifecycle mutations through existing Core routes', async () => {
    const detail = {
      task: { task_id: 'T-8', name: 'Task', status: 'Open' },
      subtasks: [],
      sprints: [{ sprint_id: 'SP-9', name: 'Sprint', status: 'Open' }],
      children: { subtask_ids: [], sprint_ids: ['SP-9'], open_subtasks: 0, open_sprints: 1, can_complete: false },
    }
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(detail), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ...detail.task, status: 'On Hold' }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ...detail.sprints[0], status: 'On Hold' }), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getTaskDetail('T-8')).resolves.toEqual(detail)
    await expect(api.updateTaskStatus('T-8', 'On Hold')).resolves.toMatchObject({ status: 'On Hold' })
    await expect(api.transitionSprint('SP-9', 'hold', 'waiting for review')).resolves.toMatchObject({ status: 'On Hold' })
    expect(fetcher.mock.calls.map(([url, init]) => [url, init?.method, init?.body])).toEqual([
      ['/api/v1/tasks/T-8', undefined, undefined],
      ['/api/v1/tasks/T-8/status', 'POST', JSON.stringify({ status: 'On Hold' })],
      ['/api/v1/sprints/SP-9/hold', 'POST', JSON.stringify({ reason: 'waiting for review' })],
    ])
  })

  it('reads file roots, tracked files, version history, and immutable contents through the Core contract', async () => {
    const root = { project_id: 'P-7', root_path: 'D:/workspace/project' }
    const files = { items: [{ file_id: 'F-1', project_id: 'P-7', current_path: 'src/main.go', file_type: 'go', current_version_id: 'FV-2', tracking_policy: 'project_scan' }] }
    const versions = { items: [{ version_id: 'FV-1', file_id: 'F-1', version_number: 1, change_kind: 'file_added', created_at: '2026-10-12T10:00:00Z', size_bytes: 5 }] }
    const content = { version: versions.items[0], content: 'hello' }
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(root), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(files), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(versions), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(content), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getProjectFileRoot('P-7')).resolves.toEqual(root)
    await expect(api.getProjectFiles('P-7')).resolves.toEqual(files.items)
    await expect(api.getFileVersions('F-1')).resolves.toEqual(versions.items)
    await expect(api.getFileVersion('F-1', 'FV-1')).resolves.toEqual(content)
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      '/api/v1/projects/P-7/file-root',
      '/api/v1/projects/P-7/files',
      '/api/v1/files/F-1/versions',
      '/api/v1/files/F-1/versions/FV-1',
    ])
  })

  it('normalizes a null tracked-file collection to an empty list', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: null }), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)
    await expect(api.getProjectFiles('P-7')).resolves.toEqual([])
  })

  it('sets the Project source root and runs scans only through explicit Core mutations', async () => {
    const root = { project_id: 'P-7', root_path: 'D:/workspace' }
    const scan = { project_id: 'P-7', root_path: 'D:/workspace', files_discovered: 2, files_added: 1, versions_added: 2, files_skipped: 0 }
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(root), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(scan), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.setProjectFileRoot('P-7', 'D:/workspace')).resolves.toEqual(root)
    await expect(api.scanProjectFiles('P-7')).resolves.toEqual(scan)
    expect(fetcher.mock.calls.map(([url, init]) => [url, init?.method, init?.body])).toEqual([
      ['/api/v1/projects/P-7/file-root', 'PUT', JSON.stringify({ root_path: 'D:/workspace' })],
      ['/api/v1/projects/P-7/files/scan', 'POST', '{}'],
    ])
  })

  it('compares and restores a version through explicit, optimistic Core mutations', async () => {
    const comparison = { from: { version_id: 'FV-1' }, to: { version_id: 'FV-2' }, from_content: 'old', to_content: 'new', hunks: [] }
    const restored = { file: { file_id: 'F-1', current_version_id: 'FV-3' }, version: { version_id: 'FV-3', restores_version_id: 'FV-1', change_kind: 'reversion' } }
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(comparison), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(restored), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.compareFileVersions('F-1', 'FV-1', 'FV-2')).resolves.toEqual(comparison)
    await expect(api.restoreFileVersion('F-1', { version_id: 'FV-1', expected_current_version_id: 'FV-2' })).resolves.toEqual(restored)
    expect(fetcher.mock.calls.map(([url, init]) => [url, init?.method, init?.body])).toEqual([
      ['/api/v1/files/F-1/compare?from=FV-1&to=FV-2', undefined, undefined],
      ['/api/v1/files/F-1/restore', 'POST', JSON.stringify({ version_id: 'FV-1', expected_current_version_id: 'FV-2' })],
    ])
  })

  it('uses typed Wails bindings for file history when available', async () => {
    const root = { project_id: 'P-7', root_path: '' }
    const file = { file_id: 'F-1', project_id: 'P-7', current_path: 'main.go', file_type: 'go', tracking_policy: 'project_scan' }
    const version = { version_id: 'FV-1', file_id: 'F-1', version_number: 1, change_kind: 'file_added' }
    const desktop = {
      Health: vi.fn().mockResolvedValue({ status: 'ok' }),
      Projects: vi.fn().mockResolvedValue([]),
      ProjectFileRoot: vi.fn().mockResolvedValue(root),
      ProjectFiles: vi.fn().mockResolvedValue([file]),
      FileVersions: vi.fn().mockResolvedValue([version]),
      ReadFileVersion: vi.fn().mockResolvedValue({ version, content: 'package main' }),
    }
    const fetcher = vi.fn()
    const api = createTimekeeperApi('', fetcher, desktop)

    await expect(api.getProjectFileRoot('P-7')).resolves.toEqual(root)
    await expect(api.getProjectFiles('P-7')).resolves.toEqual([file])
    await expect(api.getFileVersions('F-1')).resolves.toEqual([version])
    await expect(api.getFileVersion('F-1', 'FV-1')).resolves.toEqual({ version, content: 'package main' })
    expect(desktop.ProjectFileRoot).toHaveBeenCalledWith('P-7')
    expect(desktop.ProjectFiles).toHaveBeenCalledWith('P-7')
    expect(desktop.FileVersions).toHaveBeenCalledWith('F-1')
    expect(desktop.ReadFileVersion).toHaveBeenCalledWith('F-1', 'FV-1')
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('uses read-only Wails bindings when the desktop bridge is available', async () => {
    const fetcher = vi.fn()
    const desktop = {
      Health: vi.fn().mockResolvedValue({ status: 'ok' }),
      Projects: vi.fn().mockResolvedValue([
        { project_id: 'P-2', project_name: 'Desktop data', status: 'Open' },
      ]),
    }
    const api = createTimekeeperApi('', fetcher, desktop)

    await expect(api.getHealth()).resolves.toEqual({ status: 'ok' })
    await expect(api.getProjects()).resolves.toEqual([
      { project_id: 'P-2', project_name: 'Desktop data', status: 'Open' },
    ])
    expect(desktop.Health).toHaveBeenCalledOnce()
    expect(desktop.Projects).toHaveBeenCalledOnce()
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('uses Task workspace Wails bindings when available', async () => {
    const detail = {
      task: { task_id: 'T-2', name: 'Desktop', status: 'Open' },
      subtasks: [], sprints: [],
      children: { subtask_ids: [], sprint_ids: [], open_subtasks: 0, open_sprints: 0, can_complete: true },
    }
    const desktop = {
      Health: vi.fn().mockResolvedValue({ status: 'ok' }),
      Projects: vi.fn().mockResolvedValue([]),
      TaskDetail: vi.fn().mockResolvedValue(detail),
      UpdateTaskStatus: vi.fn().mockResolvedValue({ ...detail.task, status: 'On Hold' }),
      TransitionSprint: vi.fn().mockResolvedValue({ sprint_id: 'SP-3', status: 'Active' }),
    }
    const fetcher = vi.fn()
    const api = createTimekeeperApi('', fetcher, desktop)

    await expect(api.getTaskDetail('T-2')).resolves.toEqual(detail)
    await expect(api.updateTaskStatus('T-2', 'On Hold')).resolves.toMatchObject({ status: 'On Hold' })
    await expect(api.transitionSprint('SP-3', 'start', '')).resolves.toMatchObject({ status: 'Active' })
    expect(desktop.TaskDetail).toHaveBeenCalledWith('T-2')
    expect(desktop.UpdateTaskStatus).toHaveBeenCalledWith('T-2', 'On Hold')
    expect(desktop.TransitionSprint).toHaveBeenCalledWith('SP-3', 'start', '')
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('does not expose arbitrary Core response text from Task workspace requests', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response('private secret=do-not-render', { status: 409, statusText: 'Conflict' }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getTaskDetail('T-2')).rejects.toThrow('409 Conflict')
    await expect(api.getTaskDetail('T-2')).rejects.not.toThrow('do-not-render')
    await expect(api.updateTaskStatus('T-2', 'Completed')).rejects.toThrow('409 Conflict')
    await expect(api.updateTaskStatus('T-2', 'Completed')).rejects.not.toThrow('do-not-render')
  })

  it('does not expose arbitrary Core response text from Project hierarchy reads', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response('private secret=do-not-render', { status: 500, statusText: 'Internal Server Error' }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getExecutionTree('P-2')).rejects.toThrow('Core request failed (500 Internal Server Error).')
    await expect(api.getExecutionTree('P-2')).rejects.not.toThrow('do-not-render')
  })

  it('resolves Wails bindings when a request runs if the runtime loads after the API module', async () => {
    const runtime = globalThis as typeof globalThis & {
      go?: { main?: { DesktopApp?: { Health: () => Promise<{ status: string }>; Projects: () => Promise<unknown[]> } } }
    }
    const previous = runtime.go
    delete runtime.go
    const fetcher = vi.fn()
    const api = createTimekeeperApi('', fetcher)
    const desktop = {
      Health: vi.fn().mockResolvedValue({ status: 'ok' }),
      Projects: vi.fn().mockResolvedValue([]),
    }
    runtime.go = { main: { DesktopApp: desktop } }

    try {
      await expect(api.getHealth()).resolves.toEqual({ status: 'ok' })
      await expect(api.getProjects()).resolves.toEqual([])
      expect(desktop.Health).toHaveBeenCalledOnce()
      expect(desktop.Projects).toHaveBeenCalledOnce()
      expect(fetcher).not.toHaveBeenCalled()
    } finally {
      if (previous === undefined) delete runtime.go
      else runtime.go = previous
    }
  })

  it('lists and reads durable registration queue offers through Core', async () => {
    const item = {
      queue_id: 'RQ-abc123',
      project_id: 'P-1',
      kind: 'execution_batch',
      status: 'Queued',
      payload_hash: 'a'.repeat(64),
      counts: { tasks: 1, subtasks: 1, sprints: 1, total_objects: 3 },
      attempts: 0,
      created_at: '2026-10-14T10:00:00Z',
      updated_at: '2026-10-14T10:00:00Z',
    }
    const detail = { ...item, payload: { version: 'timekeeper-execution-batch/v1', tasks: [{ category_id: 'C-1', name: 'Queued task' }] } }
    const page = { items: [item], limit: 1, offset: 0, has_more: true, next_offset: 1 }
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(page), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(detail), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getRegistrationQueue('P-1', 'Queued', 1, 0)).resolves.toEqual(page)
    await expect(api.getRegistrationQueueItem('P-1', 'RQ-abc123')).resolves.toEqual(detail)
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      '/api/v1/projects/P-1/registration-queue?status=Queued&limit=1&offset=0',
      '/api/v1/projects/P-1/registration-queue/RQ-abc123',
    ])
  })

  it('surfaces the Core status and body when a read fails', async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response('Core unavailable', { status: 503, statusText: 'Service Unavailable' }),
    )
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getProjects()).rejects.toThrow('503 Service Unavailable: Core unavailable')
  })

  it('reads normalized agent work states from the Core activity projection', async () => {
    const states = [{ agent_id: 'agent-1', session_id: 'session-1', state: 'working', reported_state: 'working' }]
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: states }), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getAgentWorkStates()).resolves.toEqual(states)
    expect(fetcher).toHaveBeenCalledWith('/api/v1/agents/work-states', { headers: { Accept: 'application/json' } })
  })

  it('uses the Wails Core binding for agent work states when available', async () => {
    const states = [{ agent_id: 'agent-1', session_id: 'session-1', state: 'waiting', reported_state: 'waiting' }]
    const desktop = {
      Health: vi.fn().mockResolvedValue({ status: 'ok' }),
      Projects: vi.fn().mockResolvedValue([]),
      AgentWorkStates: vi.fn().mockResolvedValue(states),
    }
    const fetcher = vi.fn()
    const api = createTimekeeperApi('', fetcher, desktop)

    await expect(api.getAgentWorkStates()).resolves.toEqual(states)
    expect(desktop.AgentWorkStates).toHaveBeenCalledOnce()
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('reads a Project handoff snapshot through the existing Core route', async () => {
    const snapshot = {
      project: { project_id: 'P-7', project_name: 'Example', status: 'Open' },
      recent_events: [{ event_id: 3, entity_type: 'task', entity_id: 'T-3', event_type: 'task_started', message: 'Task started', created_at: '2026-10-18T10:00:00Z' }],
      recent_notes: [{ note_id: 7, project_id: 'P-7', content: 'Capture the decision', created_at: '2026-10-18T09:00:00Z' }],
      recent_messages: [{ message_id: 12, ref: 'M-12', project_id: 'P-7', kind: 'decision', author: 'Seb', body: 'Keep Core as the single authority.', revision: 1, pinned: false, created_at: '2026-10-18T08:00:00Z', updated_at: '2026-10-18T08:00:00Z' }],
    }
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify(snapshot), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getProjectHandoff('P-7')).resolves.toEqual(snapshot)
    expect(fetcher).toHaveBeenCalledWith('/api/v1/projects/P-7/handoff', { headers: { Accept: 'application/json' } })
  })

  it('normalizes Core handoff messages for the Trail of Thought view', async () => {
    const snapshot = { project: { project_id: 'P-7', project_name: 'Example', status: 'Open' }, recent_events: [], recent_notes: [] }
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify(snapshot), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getProjectHandoff('P-7')).resolves.toEqual({ ...snapshot, recent_messages: [] })
  })

  it('uses the Wails Core handoff binding when available', async () => {
    const snapshot = { project: { project_id: 'P-7', project_name: 'Example', status: 'Open' }, recent_events: [], recent_notes: [], recent_messages: [{ message_id: 21, ref: 'M-21', project_id: 'P-7', kind: 'lesson', author: 'agent-1', body: 'Retain the tested fallback.', revision: 1, pinned: true, created_at: '2026-10-18T08:00:00Z', updated_at: '2026-10-18T08:00:00Z' }] }
    const desktop = {
      Health: vi.fn().mockResolvedValue({ status: 'ok' }),
      Projects: vi.fn().mockResolvedValue([]),
      ProjectHandoff: vi.fn().mockResolvedValue(snapshot),
    }
    const fetcher = vi.fn()
    const api = createTimekeeperApi('', fetcher, desktop)

    await expect(api.getProjectHandoff('P-7')).resolves.toEqual(snapshot)
    expect(desktop.ProjectHandoff).toHaveBeenCalledWith('P-7')
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('reads project usage and operational summaries through the existing Core APIs', async () => {
    const usage = {
      project_id: 'P-7',
      totals: { input_tokens: 1200, output_tokens: 350, cache_creation_tokens: 80, cache_read_tokens: 40, messages: 5, session_count: 1, cost: { status: 'unpriced', reason: 'no trusted local price' } },
      sessions: [],
      days: [],
    }
    const operational = {
      project_id: 'P-7', total_sprints: 4, open_sprints: 1, active_sprints: 1, held_sprints: 0,
      completed_sprints: 2, timed_out_sprints: 0, cancelled_sprints: 0,
      estimated_duration_seconds: 7200, buffer_duration_seconds: 600, extension_duration_seconds: 300,
      planned_duration_seconds: 8100, recorded_work_seconds: 3600, recorded_hold_seconds: 0,
    }
    const fetcher = vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(
      String(input).endsWith('/usage-summary') ? JSON.stringify(usage) : JSON.stringify(operational),
      { status: 200 },
    )))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getProjectUsageSummary('P-7')).resolves.toEqual(usage)
    await expect(api.getProjectOperationalSummary('P-7')).resolves.toEqual(operational)
    expect(fetcher.mock.calls.map(([path]) => path)).toEqual([
      '/api/v1/projects/P-7/usage-summary',
      '/api/v1/projects/P-7/operational-summary',
    ])
  })

  it('normalizes absent usage session and day arrays from Core', async () => {
    const payload = { project_id: 'P-7', totals: { input_tokens: 0, output_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, messages: 0, session_count: 0, cost: { status: 'unpriced', reason: 'not configured' } }, sessions: null, days: null }
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getProjectUsageSummary('P-7')).resolves.toEqual({ ...payload, sessions: [], days: [] })
  })

  it('uses Wails Core bindings for both project metrics reads when available', async () => {
    const usage = { project_id: 'P-7', totals: { input_tokens: 1, output_tokens: 2, cache_creation_tokens: 0, cache_read_tokens: 0, messages: 1, session_count: 1, cost: { status: 'unpriced', reason: 'not configured' } }, sessions: [], days: [] }
    const operational = { project_id: 'P-7', total_sprints: 0, open_sprints: 0, active_sprints: 0, held_sprints: 0, completed_sprints: 0, timed_out_sprints: 0, cancelled_sprints: 0, estimated_duration_seconds: 0, buffer_duration_seconds: 0, extension_duration_seconds: 0, planned_duration_seconds: 0, recorded_work_seconds: 0, recorded_hold_seconds: 0 }
    const desktop = {
      Health: vi.fn().mockResolvedValue({ status: 'ok' }),
      Projects: vi.fn().mockResolvedValue([]),
      ProjectUsageSummary: vi.fn().mockResolvedValue(usage),
      ProjectOperationalSummary: vi.fn().mockResolvedValue(operational),
    }
    const fetcher = vi.fn()
    const api = createTimekeeperApi('', fetcher, desktop)

    await expect(api.getProjectUsageSummary('P-7')).resolves.toEqual(usage)
    await expect(api.getProjectOperationalSummary('P-7')).resolves.toEqual(operational)
    expect(desktop.ProjectUsageSummary).toHaveBeenCalledWith('P-7')
    expect(desktop.ProjectOperationalSummary).toHaveBeenCalledWith('P-7')
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('reads Memory Galaxy status and searches through the provider-neutral Core contract', async () => {
    const status = { available: true, provider: 'obsidian', protocol_version: 'timekeeper-knowledge/v1', capabilities: ['knowledge.search'], last_checked_at: '2026-10-18T19:00:00Z' }
    const results = [{ id: 'MG-1', title: 'Durable context', type: 'pattern', summary: 'Store context across sessions.', topics: ['memory'], tags: ['core'], review_status: 'verified', visibility: 'private' }]
    const fetcher = vi.fn((input: RequestInfo | URL, init?: RequestInit) => Promise.resolve(
      String(input).endsWith('/knowledge/status')
        ? new Response(JSON.stringify(status), { status: 200 })
        : new Response(JSON.stringify({ results }), { status: 200 }),
    ))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getKnowledgeStatus()).resolves.toEqual(status)
    await expect(api.searchKnowledge('  durable context  ', 12)).resolves.toEqual(results)
    expect(fetcher.mock.calls[0][0]).toBe('/api/v1/knowledge/status')
    expect(fetcher.mock.calls[1][0]).toBe('/api/v1/knowledge/search')
    expect(fetcher.mock.calls[1][1]?.method).toBe('POST')
    expect(JSON.parse(String(fetcher.mock.calls[1][1]?.body))).toEqual({ query: 'durable context', limit: 12 })
  })

  it('uses Wails Core knowledge bindings when available', async () => {
    const status = { available: true, provider: 'obsidian', protocol_version: 'timekeeper-knowledge/v1', capabilities: ['knowledge.search'], last_checked_at: '2026-10-18T19:00:00Z' }
    const results = [{ id: 'MG-2', title: 'Test pattern', type: 'pattern', review_status: 'verified', visibility: 'galaxy' }]
    const desktop = {
      Health: vi.fn().mockResolvedValue({ status: 'ok' }),
      Projects: vi.fn().mockResolvedValue([]),
      KnowledgeStatus: vi.fn().mockResolvedValue(status),
      SearchKnowledge: vi.fn().mockResolvedValue(results),
    }
    const fetcher = vi.fn()
    const api = createTimekeeperApi('', fetcher, desktop)

    await expect(api.getKnowledgeStatus()).resolves.toEqual(status)
    await expect(api.searchKnowledge('pattern', 8)).resolves.toEqual(results)
    expect(desktop.KnowledgeStatus).toHaveBeenCalledOnce()
    expect(desktop.SearchKnowledge).toHaveBeenCalledWith('pattern', 8)
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('reads the read-only plugin catalog from Core and normalizes its entries', async () => {
    const catalog = {
      schema_version: 'timekeeper-plugin-index/v1',
      generated_at: '2026-10-18T20:00:00Z',
      plugins: [{ id: 'extension.example', version: '1.2.0', name: 'Example extension', description: 'Read-only sample manifest.', runtime: 'wasi', entrypoint: 'plugin.wasm', protocol: 'timekeeper-plugin/v1', capabilities: ['knowledge.search'], requires: ['files.read'] }],
    }
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify(catalog), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.getPluginCatalog()).resolves.toEqual(catalog)
    expect(fetcher).toHaveBeenCalledWith('/api/v1/plugins', { headers: { Accept: 'application/json' } })
  })

  it('uses the Wails Core plugin catalog binding when available', async () => {
    const catalog = { schema_version: 'timekeeper-plugin-index/v1', generated_at: '2026-10-18T20:00:00Z', plugins: [] }
    const desktop = {
      Health: vi.fn().mockResolvedValue({ status: 'ok' }),
      Projects: vi.fn().mockResolvedValue([]),
      PluginCatalog: vi.fn().mockResolvedValue(catalog),
    }
    const fetcher = vi.fn()
    const api = createTimekeeperApi('', fetcher, desktop)

    await expect(api.getPluginCatalog()).resolves.toEqual(catalog)
    expect(desktop.PluginCatalog).toHaveBeenCalledOnce()
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('searches the Core global-search contract with scopes and bounded limit', async () => {
    const response = {
      query: 'queue',
      scopes: ['messages', 'projects', 'tasks'],
      items: [{ type: 'task', project_id: 'P-1', project_name: 'Example', object_id: 'T-1', ref: 'T-1', title: 'Queue task', snippet: 'queue worker', score: 200, created_at: '2026-10-18T10:00:00Z', updated_at: '2026-10-18T10:00:00Z' }],
      total: 1,
      limit: 20,
      has_more: false,
    }
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify(response), { status: 200 }))
    const api = createTimekeeperApi('', fetcher)

    await expect(api.searchGlobal(' queue ', 20, ['messages', 'projects', 'tasks'])).resolves.toEqual(response)
    expect(fetcher).toHaveBeenCalledWith('/api/v1/search?q=queue&limit=20&scope=messages&scope=projects&scope=tasks', { headers: { Accept: 'application/json' } })
  })

  it('uses the Wails Core global-search binding when available', async () => {
    const response = { query: 'queue', scopes: ['projects'], items: [], total: 0, limit: 20, has_more: false }
    const desktop = {
      Health: vi.fn().mockResolvedValue({ status: 'ok' }),
      Projects: vi.fn().mockResolvedValue([]),
      GlobalSearch: vi.fn().mockResolvedValue(response),
    }
    const fetcher = vi.fn()
    const api = createTimekeeperApi('', fetcher, desktop)

    await expect(api.searchGlobal('queue', 20, ['projects'])).resolves.toEqual(response)
    expect(desktop.GlobalSearch).toHaveBeenCalledWith('queue', 20, ['projects'])
    expect(fetcher).not.toHaveBeenCalled()
  })
})

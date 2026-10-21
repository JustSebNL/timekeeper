// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ProjectExecutionTree, ProjectSummary, TaskDetail, TaskStatus } from './lib/api'

const api = vi.hoisted(() => ({
  getHealth: vi.fn(),
  getProjects: vi.fn(),
  getAgentWorkStates: vi.fn(),
  getProjectHandoff: vi.fn(),
  getProjectUsageSummary: vi.fn(),
  getProjectOperationalSummary: vi.fn(),
  getKnowledgeStatus: vi.fn(),
  searchKnowledge: vi.fn(),
  getPluginCatalog: vi.fn(),
  searchGlobal: vi.fn(),
  testConnection: vi.fn(),
  getExecutionTree: vi.fn(),
  getTaskDetail: vi.fn(),
  updateTaskStatus: vi.fn(),
  transitionSprint: vi.fn(),
  getProjectFileRoot: vi.fn(),
  getProjectFiles: vi.fn(),
  setProjectFileRoot: vi.fn(),
  scanProjectFiles: vi.fn(),
  getFileVersions: vi.fn(),
  getFileVersion: vi.fn(),
  compareFileVersions: vi.fn(),
  restoreFileVersion: vi.fn(),
  hasNativeDirectoryPicker: vi.fn(),
  chooseProjectSourceDirectory: vi.fn(),
}))

const persistence = vi.hoisted(() => ({
  readSelectedProjectID: vi.fn(() => 'P-1'),
  readWorkspaceSection: vi.fn(() => 'projects'),
  saveSelectedProjectID: vi.fn(),
  saveWorkspaceSection: vi.fn(),
}))

vi.mock('./lib/api', () => ({ timekeeperApi: api }))
vi.mock('./lib/workspacePersistence', () => persistence)

import App from './App.svelte'

afterEach(cleanup)

const project: ProjectSummary = { project_id: 'P-1', project_name: 'Project one', status: 'Open' }
const tree: ProjectExecutionTree = {
  project,
  categories: [{
    category: { category_id: 'C-1', name: 'Delivery', status: 'Open', item_address: 'P-1/C-1' },
    categories: [],
    tasks: [
      { task: { task_id: 'T-1', name: 'Task one', status: 'Open', item_address: 'P-1/C-1/T-1' }, sprints: [], subtasks: [] },
      { task: { task_id: 'T-2', name: 'Task two', status: 'Open', item_address: 'P-1/C-1/T-2' }, sprints: [], subtasks: [] },
    ],
  }],
}

let currentStatus: TaskStatus = 'Open'
const taskDetail: TaskDetail = {
  task: { task_id: 'T-1', project_id: 'P-1', name: 'Task one', status: 'Open' },
  subtasks: [],
  sprints: [],
  children: { subtask_ids: [], sprint_ids: [], open_subtasks: 0, open_sprints: 0, can_complete: true },
}

function detailFor(taskID: string, taskName: string): TaskDetail {
  return { ...taskDetail, task: { ...taskDetail.task, task_id: taskID, name: taskName } }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => { resolve = resolvePromise })
  return { promise, resolve }
}

beforeEach(() => {
  vi.clearAllMocks()
  currentStatus = 'Open'
  api.getHealth.mockResolvedValue({ status: 'ok' })
  api.getProjects.mockResolvedValue([project])
  api.getAgentWorkStates.mockResolvedValue([])
  api.getProjectHandoff.mockResolvedValue({ project, recent_events: [], recent_notes: [] })
  api.getProjectUsageSummary.mockResolvedValue({ project_id: 'P-1', totals: { input_tokens: 0, output_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, messages: 0, session_count: 0, cost: { status: 'unpriced', reason: 'not configured' } }, sessions: [], days: [] })
  api.getProjectOperationalSummary.mockResolvedValue({ project_id: 'P-1', total_sprints: 0, open_sprints: 0, active_sprints: 0, held_sprints: 0, completed_sprints: 0, timed_out_sprints: 0, cancelled_sprints: 0, estimated_duration_seconds: 0, buffer_duration_seconds: 0, extension_duration_seconds: 0, planned_duration_seconds: 0, recorded_work_seconds: 0, recorded_hold_seconds: 0 })
  api.getKnowledgeStatus.mockRejectedValue(new Error('Core GET /api/v1/knowledge/status returned HTTP 503'))
  api.searchKnowledge.mockResolvedValue([])
  api.getPluginCatalog.mockRejectedValue(new Error('Core request failed (503 Service Unavailable).'))
  api.searchGlobal.mockResolvedValue({ query: '', scopes: [], items: [], total: 0, limit: 20, has_more: false })
  api.testConnection.mockResolvedValue({
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
  })
  api.getExecutionTree.mockResolvedValue(tree)
  api.getTaskDetail.mockImplementation(async () => ({ ...taskDetail, task: { ...taskDetail.task, status: currentStatus } }))
  api.updateTaskStatus.mockImplementation(async (_taskID: string, status: TaskStatus) => { currentStatus = status })
  api.transitionSprint.mockResolvedValue(undefined)
  api.getProjectFileRoot.mockResolvedValue({ project_id: 'P-1', root_path: '' })
  api.getProjectFiles.mockResolvedValue([])
  api.setProjectFileRoot.mockResolvedValue({ project_id: 'P-1', root_path: 'D:/workspace' })
  api.scanProjectFiles.mockResolvedValue({ project_id: 'P-1', root_path: 'D:/workspace', files_discovered: 0, files_added: 0, versions_added: 0, files_skipped: 0 })
  api.getFileVersions.mockResolvedValue([])
  api.getFileVersion.mockResolvedValue({ version: { version_id: '', file_id: '', version_number: 0, change_kind: 'other' }, content: '' })
  api.compareFileVersions.mockResolvedValue({ from: { version_id: '', file_id: '', version_number: 0, change_kind: 'other' }, to: { version_id: '', file_id: '', version_number: 0, change_kind: 'other' }, from_content: '', to_content: '', hunks: [] })
  api.restoreFileVersion.mockResolvedValue({ file: { file_id: '', project_id: '', current_path: '', file_type: '', tracking_policy: '' }, version: { version_id: '', file_id: '', version_number: 0, change_kind: 'other' } })
  api.hasNativeDirectoryPicker.mockReturnValue(false)
  api.chooseProjectSourceDirectory.mockResolvedValue('')
})

describe('File history workspace application wiring', () => {
  it('opens the Svelte Files workspace against Core without scanning implicitly', async () => {
    render(App)
    await fireEvent.click(await screen.findByRole('button', { name: 'Files & History' }))

    expect(await screen.findByRole('heading', { name: 'Files & History', level: 1 })).not.toBeNull()
    await waitFor(() => expect(api.getProjectFileRoot).toHaveBeenCalledWith('P-1'))
    expect(api.getProjectFiles).toHaveBeenCalledWith('P-1')
    expect(api.scanProjectFiles).not.toHaveBeenCalled()
    expect(screen.getByText(/Source folder is not configured/)).not.toBeNull()
  })
})

describe('Project activity workspace wiring', () => {
  it('loads recent Core events and notes when a Project is opened', async () => {
    api.getProjectHandoff.mockResolvedValueOnce({
      project,
      recent_events: [{ event_id: 3, entity_type: 'task', entity_id: 'T-1', event_type: 'task_started', message: 'Task started from Core', created_at: '2026-10-18T10:00:00Z' }],
      recent_notes: [{ note_id: 7, project_id: 'P-1', content: 'Decision from the project log', created_at: '2026-10-18T09:00:00Z' }],
      recent_messages: [{ message_id: 12, ref: 'M-12', project_id: 'P-1', kind: 'decision', author: 'Seb', body: 'Keep the Core result authoritative.', revision: 1, pinned: true, created_at: '2026-10-18T08:00:00Z', updated_at: '2026-10-18T08:00:00Z' }],
    })
    render(App)

    expect(await screen.findByText('Task started from Core')).not.toBeNull()
    expect(screen.getByText('Decision from the project log')).not.toBeNull()
    expect(screen.getByText('Keep the Core result authoritative.')).not.toBeNull()
    expect(api.getProjectHandoff).toHaveBeenCalledWith('P-1')
  })
})

describe('Global Search workspace', () => {
  it('debounces Core search and increases the result window when loading more', async () => {
    const firstPage = {
      query: 'queue', scopes: ['messages', 'projects', 'tasks'],
      items: [{ type: 'task', project_id: 'P-1', project_name: 'Project one', object_id: 'T-1', ref: 'T-1', title: 'Queue worker', snippet: 'queue worker snippet', score: 200, created_at: '2026-10-18T10:00:00Z', updated_at: '2026-10-18T10:00:00Z' }],
      total: 21, limit: 20, has_more: true,
    }
    const secondPage = { ...firstPage, items: [...firstPage.items, { ...firstPage.items[0], object_id: 'T-2', ref: 'T-2', title: 'Queue cleanup' }], limit: 40, has_more: false }
    api.searchGlobal.mockResolvedValueOnce(firstPage).mockResolvedValueOnce(secondPage)
    render(App)

    await fireEvent.input(screen.getByLabelText('Search TimeKeeper'), { target: { value: 'queue' } })
    expect(await screen.findByText('queue worker snippet')).not.toBeNull()
    await fireEvent.click(screen.getByRole('button', { name: /Load .* more results/ }))

    await waitFor(() => expect(api.searchGlobal.mock.calls.map(([query, limit]) => [query, limit])).toEqual([['queue', 20], ['queue', 40]]))
    expect(await screen.findByText('Queue cleanup')).not.toBeNull()
  })

  it('does not let an older search response replace newer results', async () => {
    const older = deferred<{ query: string; scopes: string[]; items: Array<Record<string, unknown>>; total: number; limit: number; has_more: boolean }>()
    const oldPage = { query: 'old', scopes: ['projects'], items: [{ type: 'project', project_id: 'P-1', project_name: 'Project one', object_id: 'P-1', ref: 'P-1', title: 'Old result', snippet: 'old result snippet', score: 100, created_at: '2026-10-18T10:00:00Z', updated_at: '2026-10-18T10:00:00Z' }], total: 1, limit: 20, has_more: false }
    const newPage = { query: 'new', scopes: ['projects'], items: [{ ...oldPage.items[0], title: 'New result', snippet: 'new result snippet' }], total: 1, limit: 20, has_more: false }
    api.searchGlobal.mockImplementation((query: string) => query === 'old' ? older.promise : Promise.resolve(newPage))
    render(App)

    const input = screen.getByLabelText('Search TimeKeeper')
    await fireEvent.input(input, { target: { value: 'old' } })
    await waitFor(() => expect(api.searchGlobal.mock.calls.map(([query, limit]) => [query, limit])).toContainEqual(['old', 20]))
    await fireEvent.input(input, { target: { value: 'new' } })
    expect(await screen.findByText('new result snippet')).not.toBeNull()
    older.resolve(oldPage)
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(screen.getByText('new result snippet')).not.toBeNull()
    expect(screen.queryByText('old result snippet')).toBeNull()
  })

  it('opens a Task search result through the existing Project and Task readers', async () => {
    api.searchGlobal.mockResolvedValueOnce({
      query: 'queue', scopes: ['tasks'],
      items: [{ type: 'task', project_id: 'P-1', project_name: 'Project one', object_id: 'T-1', ref: 'T-1', title: 'Queue worker', snippet: 'queue worker snippet', score: 200, created_at: '2026-10-18T10:00:00Z', updated_at: '2026-10-18T10:00:00Z' }],
      total: 1, limit: 20, has_more: false,
    })
    render(App)

    await fireEvent.input(screen.getByLabelText('Search TimeKeeper'), { target: { value: 'queue' } })
    await screen.findByText('queue worker snippet')
    await fireEvent.click(screen.getByRole('button', { name: 'Open Task Queue worker workspace' }))

    expect(await screen.findByRole('heading', { name: 'Task one' })).not.toBeNull()
    expect(api.getExecutionTree).toHaveBeenCalledWith('P-1')
    expect(api.getTaskDetail).toHaveBeenCalledWith('T-1')
  })
})

describe('Agent Pulse workspace', () => {
  it('navigates to Agent Pulse and renders Core-owned agent context', async () => {
    api.getAgentWorkStates.mockResolvedValueOnce([{
      agent_id: 'agent-one',
      state: 'waiting',
      reported_state: 'waiting',
      runtime: 'codex',
      project_id: 'P-1',
      active_sprint_id: 'SP-10',
      reason: 'Waiting for the next Core checkpoint',
      work_expected: true,
      last_contact_at: '2026-10-18T17:00:00Z',
      last_progress_at: '2026-10-18T16:59:00Z',
      state_changed_at: '2026-10-18T16:55:00Z',
      wait_until: '2026-10-18T17:10:00Z',
      lease_duration_seconds: 900,
    }])
    render(App)

    await fireEvent.click(await screen.findByRole('button', { name: 'Agents' }))
    expect(await screen.findByRole('heading', { name: 'Agent Pulse' })).not.toBeNull()
    expect(screen.getByText('agent-one')).not.toBeNull()
    expect(screen.getByText('Waiting for the next Core checkpoint')).not.toBeNull()
    expect(screen.getByText('SP-10')).not.toBeNull()
    expect(screen.getByText('1 agent')).not.toBeNull()
  })

  it('keeps Core online when the optional Agent Pulse read fails', async () => {
    api.getAgentWorkStates.mockRejectedValueOnce(new Error('Agent Pulse endpoint unavailable'))
    render(App)

    await fireEvent.click(await screen.findByRole('button', { name: 'Agents' }))
    expect((await screen.findByRole('alert')).textContent).toContain('Agent Pulse endpoint unavailable')
    expect(screen.getByText('Connected to TimeKeeper Core')).not.toBeNull()
  })
})

describe('Metrics workspace', () => {
  it('loads operational and token usage summaries for the selected Project', async () => {
    api.getProjectOperationalSummary.mockResolvedValueOnce({
      project_id: 'P-1', total_sprints: 4, open_sprints: 1, active_sprints: 1, held_sprints: 0,
      completed_sprints: 2, timed_out_sprints: 0, cancelled_sprints: 0,
      estimated_duration_seconds: 7200, buffer_duration_seconds: 600, extension_duration_seconds: 300,
      planned_duration_seconds: 8100, recorded_work_seconds: 3600, recorded_hold_seconds: 0,
    })
    api.getProjectUsageSummary.mockResolvedValueOnce({
      project_id: 'P-1',
      totals: { input_tokens: 1200, output_tokens: 350, cache_creation_tokens: 80, cache_read_tokens: 40, messages: 5, session_count: 1, cost: { status: 'unpriced', reason: 'no trusted local price is configured' } },
      sessions: [], days: [],
    })
    render(App)

    await fireEvent.click(await screen.findByRole('button', { name: 'Metrics' }))
    expect(await screen.findByRole('heading', { name: 'Metrics', level: 1 })).not.toBeNull()
    expect(await screen.findByText('Agent token usage')).not.toBeNull()
    expect(screen.getByText('Unpriced')).not.toBeNull()
    expect(screen.getByText('no trusted local price is configured')).not.toBeNull()
    expect(screen.getByText('2h 15m')).not.toBeNull()
    expect(screen.getByText('1,200')).not.toBeNull()
    expect(api.getProjectOperationalSummary).toHaveBeenCalledWith('P-1')
    expect(api.getProjectUsageSummary).toHaveBeenCalledWith('P-1')
  })
})

describe('Memory Galaxy workspace', () => {
  it('shows the Core provider-unavailable state and sends no search request', async () => {
    render(App)
    await fireEvent.click(await screen.findByRole('button', { name: 'Memory' }))

    expect(await screen.findByRole('heading', { name: 'Memory Galaxy', level: 1 })).not.toBeNull()
    expect((await screen.findByRole('alert')).textContent).toContain('Memory Galaxy is unavailable')
    expect(api.getKnowledgeStatus).toHaveBeenCalledOnce()
    expect(api.searchKnowledge).not.toHaveBeenCalled()
  })
})

describe('Plugins workspace', () => {
  it('shows the unconfigured plugin catalog without exposing install or activation actions', async () => {
    render(App)
    await fireEvent.click(await screen.findByRole('button', { name: 'Plugins' }))

    expect(await screen.findByRole('heading', { name: 'Plugins', level: 1 })).not.toBeNull()
    expect((await screen.findByRole('alert')).textContent).toContain('Plugin catalog is unavailable')
    expect(api.getPluginCatalog).toHaveBeenCalledOnce()
    expect(screen.queryByRole('button', { name: /install|activate|download/i })).toBeNull()
  })
})

describe('Settings connection diagnostics', () => {
  it('tests the selected endpoint only after an explicit action', async () => {
    render(App)
    await fireEvent.click(await screen.findByRole('button', { name: 'Settings' }))

    expect(await screen.findByRole('heading', { name: 'Settings', level: 1 })).not.toBeNull()
    expect(api.testConnection).not.toHaveBeenCalled()
    await fireEvent.click(screen.getByRole('button', { name: 'Test connection' }))
    await waitFor(() => expect(api.testConnection).toHaveBeenCalledOnce())
    expect(screen.getByRole('status').textContent).toContain('loopback fallback')
    expect(screen.getByText('tk_instance_0123456789abcdef0123456789abcdef')).not.toBeNull()
  })
})

describe('Task workspace application wiring', () => {
  it('routes Task status changes through Core and restores focus to the originating hierarchy row', async () => {
    render(App)

    const taskRow = await screen.findByRole('button', { name: 'Open Task Task one workspace' })
    await fireEvent.click(taskRow)
    expect(await screen.findByRole('heading', { name: 'Task one' })).not.toBeNull()
    expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Task one' }))

    await fireEvent.change(screen.getByLabelText('Set Task status'), { target: { value: 'On Hold' } })
    await fireEvent.click(screen.getByRole('button', { name: 'Update Task status' }))
    await waitFor(() => expect(api.updateTaskStatus).toHaveBeenCalledWith('T-1', 'On Hold'))
    await waitFor(() => expect(screen.getAllByText('On Hold').length).toBeGreaterThan(0))

    await fireEvent.click(screen.getByRole('button', { name: /Back to Project hierarchy/ }))
    await waitFor(() => expect(document.activeElement?.getAttribute('data-task-id')).toBe('T-1'))
  })

  it('ignores an older Task response after returning and opening a different Task', async () => {
    const taskOne = deferred<TaskDetail>()
    const taskTwo = deferred<TaskDetail>()
    api.getTaskDetail.mockImplementation((taskID: string) => taskID === 'T-1' ? taskOne.promise : taskTwo.promise)
    render(App)

    await fireEvent.click(await screen.findByRole('button', { name: 'Open Task Task one workspace' }))
    await waitFor(() => expect(api.getTaskDetail).toHaveBeenCalledWith('T-1'))
    await fireEvent.click(screen.getByRole('button', { name: /Back to Project hierarchy/ }))

    await fireEvent.click(await screen.findByRole('button', { name: 'Open Task Task two workspace' }))
    await waitFor(() => expect(api.getTaskDetail).toHaveBeenCalledWith('T-2'))
    taskTwo.resolve(detailFor('T-2', 'Task two'))
    expect(await screen.findByRole('heading', { name: 'Task two' })).not.toBeNull()

    taskOne.resolve(detailFor('T-1', 'Task one'))
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(screen.getByRole('heading', { name: 'Task two' })).not.toBeNull()
    expect(screen.queryByRole('heading', { name: 'Task one' })).toBeNull()
  })

  it('does not let a settled old Task action invalidate the newly selected Task read', async () => {
    const taskTwo = deferred<TaskDetail>()
    const sprintAction = deferred<void>()
    api.getTaskDetail.mockImplementation((taskID: string) => taskID === 'T-1'
      ? Promise.resolve({ ...detailFor('T-1', 'Task one'), sprints: [{ sprint_id: 'SP-1', name: 'Sprint one', status: 'Open' }] })
      : taskTwo.promise)
    api.transitionSprint.mockImplementation(() => sprintAction.promise)
    render(App)

    await fireEvent.click(await screen.findByRole('button', { name: 'Open Task Task one workspace' }))
    await screen.findByRole('heading', { name: 'Task one' })
    await fireEvent.click(screen.getByRole('button', { name: 'Start Sprint' }))
    await waitFor(() => expect(api.transitionSprint).toHaveBeenCalledWith('SP-1', 'start', ''))
    await fireEvent.click(screen.getByRole('button', { name: /Back to Project hierarchy/ }))

    await fireEvent.click(await screen.findByRole('button', { name: 'Open Task Task two workspace' }))
    await waitFor(() => expect(api.getTaskDetail).toHaveBeenCalledWith('T-2'))
    sprintAction.resolve()
    await sprintAction.promise
    await new Promise((resolve) => setTimeout(resolve, 0))
    taskTwo.resolve(detailFor('T-2', 'Task two'))

    expect(await screen.findByRole('heading', { name: 'Task two' })).not.toBeNull()
    expect(api.getTaskDetail.mock.calls.filter(([taskID]) => taskID === 'T-1')).toHaveLength(1)
  })
})

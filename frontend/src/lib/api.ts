export interface CoreHealth {
  status: string
}

export interface CoreConnectionInfo {
  service: string
  instance_id: string
  version: string
  commit?: string
  build_date?: string
  go_version?: string
  api_version: string
  capabilities: string[]
  friendly_urls: string[]
}

export interface ConnectionDiagnostic {
  client_surface: 'browser' | 'desktop'
  selected_endpoint: string
  used_fallback: boolean
  connection_info: CoreConnectionInfo
}

export interface ProjectSummary {
  project_id: string
  project_name: string
  status: string
  project_goal?: string
  project_description?: string
  calculated_completion_pct?: number
  planned_start_date?: string
  target_end_date?: string
  actual_started_at?: string | null
  completed_at?: string | null
  cancelled_at?: string | null
  lifecycle_phase?: string
  schedule_timezone?: string
  deadline_warning_days?: number
  deadline_state?: string
  days_to_deadline?: number | null
}

export interface AgentWorkState {
  agent_id: string
  state: string
  session_id?: string
  runtime?: string
  project_id?: string
  active_sprint_id?: string
  reported_state?: string
  reason?: string
  work_expected?: boolean
  last_contact_at?: string
  last_progress_at?: string
  state_changed_at?: string
  deadline?: string | null
  wait_until?: string | null
  lease_duration_seconds?: number
}

export type ProjectMilestoneStatus = 'Planned' | 'In Progress' | 'Completed' | 'Cancelled'

export interface ProjectMilestone {
  milestone_id: string
  project_id: string
  name: string
  description?: string
  target_date: string
  status: ProjectMilestoneStatus
  linked_task_id?: string
  completed_at?: string | null
  created_at: string
  updated_at: string
  deadline_state?: string
  days_to_deadline?: number | null
}

export interface ProjectActivityEvent {
  event_id: number
  entity_type: string
  entity_id: string
  event_type: string
  message: string
  created_at: string
}

export interface ProjectActivityNote {
  note_id: number
  project_id: string
  content: string
  created_at: string
}

export type ProjectMessageKind = 'note' | 'decision' | 'observation' | 'link' | 'lesson' | 'question' | 'answer'

export interface ProjectMessage {
  message_id: number
  ref: string
  project_id: string
  parent_message_id?: number
  kind: ProjectMessageKind
  author?: string
  body: string
  link?: string
  tags?: string
  revision: number
  pinned: boolean
  created_at: string
  updated_at: string
}

export interface ProjectHandoffSnapshot {
  project: ProjectSummary
  recent_events?: ProjectActivityEvent[] | null
  recent_notes?: ProjectActivityNote[] | null
  recent_messages?: ProjectMessage[] | null
}

export interface UsageCost {
  status: string
  currency?: string
  amount_micros?: number
  source?: string
  reason?: string
}

export interface ProjectUsageSession {
  session_id: string
  project_id: string
  sprint_id?: string
  agent_id: string
  model?: string
  source: string
  coverage: string
  turn_seq: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  context_used?: number
  context_size?: number
  messages: number
  cost: UsageCost
  last_activity_at: string
  created_at: string
  updated_at: string
}

export interface ProjectUsageDay {
  date: string
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  messages: number
  session_count: number
}

export interface ProjectUsageSummary {
  project_id: string
  totals: {
    input_tokens: number
    output_tokens: number
    cache_creation_tokens: number
    cache_read_tokens: number
    messages: number
    session_count: number
    cost: UsageCost
  }
  sessions?: ProjectUsageSession[] | null
  days?: ProjectUsageDay[] | null
}

export interface ProjectOperationalSummary {
  project_id: string
  total_sprints: number
  open_sprints: number
  active_sprints: number
  held_sprints: number
  completed_sprints: number
  timed_out_sprints: number
  cancelled_sprints: number
  estimated_duration_seconds: number
  buffer_duration_seconds: number
  extension_duration_seconds: number
  planned_duration_seconds: number
  recorded_work_seconds: number
  recorded_hold_seconds: number
}

export interface KnowledgeProviderStatus {
  available: boolean
  provider: string
  protocol_version: string
  vault_id?: string
  capabilities?: string[] | null
  last_checked_at: string
  message?: string
}

export interface KnowledgeResult {
  id: string
  title: string
  type: string
  summary?: string
  topics?: string[] | null
  tags?: string[] | null
  review_status: string
  visibility: string
}

export type GlobalSearchScope = 'projects' | 'tasks' | 'messages'

export interface GlobalSearchResult {
  type: 'project' | 'task' | 'message'
  project_id: string
  project_name: string
  object_id: string
  ref: string
  title: string
  status?: string
  kind?: string
  tags?: string
  snippet: string
  score: number
  rank?: number
  created_at: string
  updated_at: string
}

export interface GlobalSearchResponse {
  query: string
  project_id?: string
  scopes: GlobalSearchScope[]
  items: GlobalSearchResult[]
  total: number
  limit: number
  has_more: boolean
}

export interface ExecutionSprint {
  sprint_id: string
  item_address: string
  name: string
  status: string
}

export interface ExecutionSubtask {
  subtask: { subtask_id: string; name: string; status: string; item_address: string; calculated_completion_pct?: number }
  sprints: ExecutionSprint[]
}

export interface ExecutionTask {
  task: { task_id: string; name: string; status: string; item_address: string; calculated_completion_pct?: number }
  sprints: ExecutionSprint[]
  subtasks: ExecutionSubtask[]
}

export interface ExecutionCategory {
  category: { category_id: string; name: string; status: string; item_address: string; parent_category_id?: string }
  categories: ExecutionCategory[]
  tasks: ExecutionTask[]
}

export interface ProjectExecutionTree {
  project: ProjectSummary
  categories: ExecutionCategory[]
}

export type RegistrationQueueStatus = 'Queued' | 'Registered' | 'Rejected'

export interface RegistrationQueuePage {
  items: RegistrationQueueItem[]
  limit: number
  offset: number
  has_more: boolean
  next_offset: number | null
}

export interface RegistrationQueueValidationIssue {
  path: string
  field: string
  code: string
  message: string
  suggestion: string
}

export interface RegistrationQueueBatchCounts {
  tasks: number
  subtasks: number
  sprints: number
  total_objects: number
}

export interface RegistrationQueueItem {
  queue_id: string
  project_id: string
  kind: 'task' | 'subtask' | 'sprint' | 'task_batch' | 'execution_batch'
  status: RegistrationQueueStatus
  idempotency_key?: string
  payload_hash: string
  counts: RegistrationQueueBatchCounts
  payload?: {
    version: string
    tasks?: Array<{ name: string; category_id: string; subtasks?: Array<{ name: string; sprints?: Array<{ name: string }> }>; sprints?: Array<{ name: string }> }>
    extensions?: Array<{ task_id?: string; subtask_id?: string; subtasks?: Array<{ name: string; sprints?: Array<{ name: string }> }>; sprints?: Array<{ name: string }> }>
  }
  validation?: { valid: boolean; counts: RegistrationQueueBatchCounts; issues: RegistrationQueueValidationIssue[] | null }
  result?: { tasks: TaskRecord[] | null; subtasks: SubtaskRecord[] | null; sprints: SprintRecord[] | null; counts: RegistrationQueueBatchCounts }
  attempts: number
  last_error?: string
  created_at: string
  updated_at: string
  processed_at?: string
}

export type TaskStatus = 'Open' | 'On Hold' | 'Completed' | 'Cancelled'
export type SprintAction = 'start' | 'hold' | 'resume' | 'complete' | 'cancel'

export interface TaskRecord {
  task_id: string
  project_id?: string
  category_id?: string
  item_address?: string
  name: string
  description?: string
  goal?: string
  status: TaskStatus
  priority?: string
  estimated_duration_seconds?: number
  reported_completion_pct?: number
  calculated_completion_pct?: number
  auto_close?: boolean
  created_at?: string
  updated_at?: string
}

export interface SubtaskRecord {
  subtask_id: string
  item_address?: string
  name: string
  description?: string
  goal?: string
  status: string
  priority?: string
  calculated_completion_pct?: number
}

export interface SprintRecord {
  sprint_id: string
  item_address?: string
  name: string
  description?: string
  goal?: string
  status: string
  priority?: string
  estimated_duration_seconds?: number
  buffer_duration_seconds?: number
  active_duration_seconds?: number
  hold_duration_seconds?: number
  hold_reason?: string
  started_at?: string | null
  ended_at?: string | null
}

export interface TaskChildrenSummary {
  subtask_ids: string[]
  sprint_ids: string[]
  open_subtasks: number
  open_sprints: number
  can_complete: boolean
}

export interface TaskDetail {
  task: TaskRecord
  subtasks: SubtaskRecord[]
  sprints: SprintRecord[]
  children: TaskChildrenSummary
}

interface ProjectListResponse {
  items: ProjectSummary[]
}

function validateConnectionInfo(info: CoreConnectionInfo): void {
  if (info.service !== 'TimeKeeper') {
    throw new Error('Connection metadata does not identify a TimeKeeper Core.')
  }
  if (!/^tk_instance_[0-9a-f]{32}$/.test(info.instance_id ?? '')) {
    throw new Error('Connection metadata has a missing or invalid instance ID.')
  }
  const apiVersion = info.api_version || 'unknown'
  if (apiVersion !== 'v1') {
    throw new Error(`API version "${apiVersion}" is incompatible; expected v1.`)
  }
  const capabilities = Array.isArray(info.capabilities) ? info.capabilities : []
  for (const required of ['projects.list', 'tasks.lifecycle']) {
    if (!capabilities.includes(required)) {
      throw new Error(`Required Core capability "${required}" is unavailable.`)
    }
  }
}

function browserEndpoint(baseUrl: string): string {
  const explicitBaseUrl = baseUrl.trim().replace(/\/$/, '')
  if (explicitBaseUrl) return explicitBaseUrl
  return typeof globalThis.location !== 'undefined' ? globalThis.location.origin : 'same-origin'
}

type Fetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

export interface FileHistoryRoot {
  project_id: string
  root_path: string
  updated_at?: string
}

export interface TrackedFile {
  file_id: string
  project_id: string
  current_path: string
  file_type: string
  created_at?: string
  current_version_id?: string
  tracking_policy: string
}

export interface FileVersion {
  version_id: string
  file_id: string
  version_number: number
  content_hash?: string
  parent_version_id?: string
  restores_version_id?: string
  change_kind: string
  snapshot_type?: string
  size_bytes?: number
  created_at?: string
  agent_id?: string
  session_id?: string
  model?: string
  task_id?: string
  subtask_id?: string
  sprint_id?: string
  checkpoint_id?: string
  change_summary?: string
  status?: string
}

export interface FileVersionContent {
  version: FileVersion
  content: string
}

export interface FileCompareHunk {
  from_start: number
  from_count: number
  to_start: number
  to_count: number
  removed: string[]
  added: string[]
}

export interface FileCompareResult {
  from: FileVersion
  to: FileVersion
  from_content: string
  to_content: string
  hunks: FileCompareHunk[]
}

export interface ProjectFileScanResult {
  project_id: string
  root_path: string
  files_discovered: number
  files_added: number
  versions_added: number
  files_skipped: number
}

export interface FileHistorySaveInput {
  content: string
  expected_current_version_id: string
  change_kind?: string
}

export interface FileHistoryRestoreInput {
  version_id: string
  expected_current_version_id: string
}

export interface FileHistoryMutationResult {
  file: TrackedFile
  version: FileVersion
}

export interface PluginCatalogEntry {
  schema_version?: string
  id: string
  version: string
  name: string
  description?: string
  runtime: string
  entrypoint: string
  protocol: string
  capabilities?: string[]
  requires?: string[]
  permissions?: Record<string, boolean>
  artifact?: string
  sha256?: string
  metadata?: Record<string, string>
}

export interface PluginCatalog {
  schema_version: string
  generated_at?: string
  plugins: PluginCatalogEntry[]
}

export interface DesktopTimekeeperBindings {
  Health: () => Promise<CoreHealth>
  Projects: () => Promise<ProjectSummary[]>
  AgentWorkStates?: () => Promise<AgentWorkState[]>
  ProjectHandoff?: (projectID: string) => Promise<ProjectHandoffSnapshot>
  ProjectUsageSummary?: (projectID: string) => Promise<ProjectUsageSummary>
  ProjectOperationalSummary?: (projectID: string) => Promise<ProjectOperationalSummary>
  KnowledgeStatus?: () => Promise<KnowledgeProviderStatus>
  SearchKnowledge?: (query: string, limit: number) => Promise<KnowledgeResult[]>
  PluginCatalog?: () => Promise<PluginCatalog>
  GlobalSearch?: (query: string, limit: number, scopes?: GlobalSearchScope[]) => Promise<GlobalSearchResponse>
  TestConnection?: () => Promise<ConnectionDiagnostic>
  ProjectExecutionTree?: (projectID: string) => Promise<ProjectExecutionTree>
  TaskDetail?: (taskID: string) => Promise<TaskDetail>
  UpdateTaskStatus?: (taskID: string, status: TaskStatus) => Promise<TaskRecord>
  TransitionSprint?: (sprintID: string, action: SprintAction, reason: string) => Promise<SprintRecord>
  ProjectFileRoot?: (projectID: string) => Promise<FileHistoryRoot>
  ProjectFiles?: (projectID: string) => Promise<TrackedFile[]>
  FileVersions?: (fileID: string) => Promise<FileVersion[]>
  ReadFileVersion?: (fileID: string, versionID: string) => Promise<FileVersionContent>
  SetProjectFileRoot?: (projectID: string, rootPath: string) => Promise<FileHistoryRoot>
  ScanProjectFiles?: (projectID: string) => Promise<ProjectFileScanResult>
  CompareFileVersions?: (fileID: string, fromVersionID: string, toVersionID: string) => Promise<FileCompareResult>
  SaveFileVersion?: (fileID: string, input: FileHistorySaveInput) => Promise<FileHistoryMutationResult>
  RestoreFileVersion?: (fileID: string, input: FileHistoryRestoreInput) => Promise<FileHistoryMutationResult>
  OpenDirectoryDialog?: (options: { title: string; defaultDirectory?: string }) => Promise<string>
}

function desktopBindingsFromWindow(): DesktopTimekeeperBindings | undefined {
  const runtime = globalThis as typeof globalThis & {
    go?: { main?: { DesktopApp?: DesktopTimekeeperBindings } }
  }
  return runtime.go?.main?.DesktopApp
}

export function createTimekeeperApi(
  baseUrl = '',
  fetcher: Fetcher = fetch,
  desktop?: DesktopTimekeeperBindings,
) {
  const getDesktopBindings = () => desktop ?? desktopBindingsFromWindow()

  const request = async <T>(path: string): Promise<T> => {
    const response = await fetcher(`${baseUrl.replace(/\/$/, '')}${path}`, {
      headers: { Accept: 'application/json' },
    })

    if (!response.ok) {
      const detail = (await response.text()).trim()
      const reason = detail || response.statusText || 'request failed'
      throw new Error(`${response.status} ${response.statusText}: ${reason}`)
    }

    return response.json() as Promise<T>
  }

  const requestSafe = async <T>(path: string, init?: RequestInit): Promise<T> => {
    const response = await fetcher(`${baseUrl.replace(/\/$/, '')}${path}`, {
      ...init,
      headers: { Accept: 'application/json', ...init?.headers },
    })
    if (!response.ok) {
      throw new Error(`Core request failed (${response.status} ${response.statusText || 'error'}).`)
    }
    return response.json() as Promise<T>
  }

  return {
    getHealth: () => {
      const bindings = getDesktopBindings()
      return bindings ? bindings.Health() : request<CoreHealth>('/health')
    },
    testConnection: async (): Promise<ConnectionDiagnostic> => {
      const bindings = getDesktopBindings()
      if (bindings?.TestConnection) {
        const diagnostic = await bindings.TestConnection()
        validateConnectionInfo(diagnostic.connection_info)
        return diagnostic
      }
      const connectionInfo = await requestSafe<CoreConnectionInfo>('/api/v1/connection-info')
      validateConnectionInfo(connectionInfo)
      return { client_surface: 'browser', selected_endpoint: browserEndpoint(baseUrl), used_fallback: false, connection_info: connectionInfo }
    },
    getProjects: async () => {
      const bindings = getDesktopBindings()
      return bindings ? bindings.Projects() : (await request<ProjectListResponse>('/api/v1/projects')).items
    },
    getAgentWorkStates: async () => {
      const bindings = getDesktopBindings()
      if (bindings?.AgentWorkStates) return bindings.AgentWorkStates()
      const response = await requestSafe<{ items?: AgentWorkState[] | null }>('/api/v1/agents/work-states')
      return response.items ?? []
    },
    getProjectHandoff: async (projectID: string) => {
      const bindings = getDesktopBindings()
      const snapshot = bindings?.ProjectHandoff
        ? await bindings.ProjectHandoff(projectID)
        : await requestSafe<ProjectHandoffSnapshot>(`/api/v1/projects/${encodeURIComponent(projectID)}/handoff`)
      return {
        ...snapshot,
        recent_events: snapshot.recent_events ?? [],
        recent_notes: snapshot.recent_notes ?? [],
        recent_messages: snapshot.recent_messages ?? [],
      }
    },
    getProjectUsageSummary: async (projectID: string): Promise<ProjectUsageSummary> => {
      const bindings = getDesktopBindings()
      const summary = bindings?.ProjectUsageSummary
        ? await bindings.ProjectUsageSummary(projectID)
        : await requestSafe<ProjectUsageSummary>(`/api/v1/projects/${encodeURIComponent(projectID)}/usage-summary`)
      return { ...summary, sessions: summary.sessions ?? [], days: summary.days ?? [] }
    },
    getProjectOperationalSummary: (projectID: string): Promise<ProjectOperationalSummary> => {
      const bindings = getDesktopBindings()
      if (bindings?.ProjectOperationalSummary) return bindings.ProjectOperationalSummary(projectID)
      return requestSafe<ProjectOperationalSummary>(`/api/v1/projects/${encodeURIComponent(projectID)}/operational-summary`)
    },
    getKnowledgeStatus: () => {
      const bindings = getDesktopBindings()
      if (bindings?.KnowledgeStatus) return bindings.KnowledgeStatus()
      return requestSafe<KnowledgeProviderStatus>('/api/v1/knowledge/status')
    },
    searchKnowledge: async (query: string, limit = 8): Promise<KnowledgeResult[]> => {
      const normalizedQuery = query.trim()
      const boundedLimit = Math.min(50, Math.max(1, Math.trunc(limit || 8)))
      const bindings = getDesktopBindings()
      if (bindings?.SearchKnowledge) return bindings.SearchKnowledge(normalizedQuery, boundedLimit)
      const response = await requestSafe<{ results?: KnowledgeResult[] | null }>('/api/v1/knowledge/search', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: normalizedQuery, limit: boundedLimit }),
      })
      return response.results ?? []
    },
    getPluginCatalog: () => {
      const bindings = getDesktopBindings()
      if (bindings?.PluginCatalog) return bindings.PluginCatalog()
      return requestSafe<PluginCatalog>('/api/v1/plugins')
    },
    searchGlobal: async (query: string, limit = 20, scopes?: GlobalSearchScope[]) => {
      const normalizedQuery = query.trim()
      const boundedLimit = Math.min(200, Math.max(1, Math.trunc(limit || 20)))
      const bindings = getDesktopBindings()
      let response: Partial<GlobalSearchResponse>
      if (bindings?.GlobalSearch) {
        response = await bindings.GlobalSearch(normalizedQuery, boundedLimit, scopes)
      } else {
        const params = new URLSearchParams({ q: normalizedQuery, limit: String(boundedLimit) })
        for (const scope of scopes ?? []) params.append('scope', scope)
        response = await requestSafe<Partial<GlobalSearchResponse>>(`/api/v1/search?${params.toString()}`)
      }
      return {
        query: response.query ?? normalizedQuery,
        project_id: response.project_id,
        scopes: response.scopes ?? scopes ?? [],
        items: response.items ?? [],
        total: response.total ?? 0,
        limit: response.limit ?? boundedLimit,
        has_more: response.has_more ?? false,
      }
    },
    getExecutionTree: (projectID: string) => {
      const bindings = getDesktopBindings()
      if (bindings?.ProjectExecutionTree) return bindings.ProjectExecutionTree(projectID)
      return requestSafe<ProjectExecutionTree>(`/api/v1/projects/${encodeURIComponent(projectID)}/execution-tree`)
    },
    getRegistrationQueue: async (projectID: string, status = '', limit = 50, offset = 0): Promise<RegistrationQueuePage> => {
      const query = new URLSearchParams()
      if (status) query.set('status', status)
      query.set('limit', String(limit))
      query.set('offset', String(offset))
      const response = await requestSafe<Partial<RegistrationQueuePage>>(`/api/v1/projects/${encodeURIComponent(projectID)}/registration-queue?${query}`)
      const items = response.items ?? []
      const hasMore = response.has_more ?? items.length === limit
      return {
        items,
        limit: response.limit ?? limit,
        offset: response.offset ?? offset,
        has_more: hasMore,
        next_offset: response.next_offset ?? (hasMore ? offset + items.length : null),
      }
    },
    getRegistrationQueueItem: (projectID: string, queueID: string) =>
      requestSafe<RegistrationQueueItem>(`/api/v1/projects/${encodeURIComponent(projectID)}/registration-queue/${encodeURIComponent(queueID)}`),
    getTaskDetail: (taskID: string) => {
      const bindings = getDesktopBindings()
      if (bindings?.TaskDetail) return bindings.TaskDetail(taskID)
      return requestSafe<TaskDetail>(`/api/v1/tasks/${encodeURIComponent(taskID)}`)
    },
    updateTaskStatus: (taskID: string, status: TaskStatus) => {
      const bindings = getDesktopBindings()
      if (bindings?.UpdateTaskStatus) return bindings.UpdateTaskStatus(taskID, status)
      return requestSafe<TaskRecord>(`/api/v1/tasks/${encodeURIComponent(taskID)}/status`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status }),
      })
    },
    transitionSprint: (sprintID: string, action: SprintAction, reason: string) => {
      const bindings = getDesktopBindings()
      if (bindings?.TransitionSprint) return bindings.TransitionSprint(sprintID, action, reason)
      return requestSafe<SprintRecord>(`/api/v1/sprints/${encodeURIComponent(sprintID)}/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reason }),
      })
    },
    getProjectFileRoot: (projectID: string) => {
      const bindings = getDesktopBindings()
      if (bindings?.ProjectFileRoot) return bindings.ProjectFileRoot(projectID)
      return requestSafe<FileHistoryRoot>(`/api/v1/projects/${encodeURIComponent(projectID)}/file-root`)
    },
    getProjectFiles: async (projectID: string) => {
      const bindings = getDesktopBindings()
      if (bindings?.ProjectFiles) return bindings.ProjectFiles(projectID)
      const response = await requestSafe<{ items?: TrackedFile[] | null }>(`/api/v1/projects/${encodeURIComponent(projectID)}/files`)
      return response.items ?? []
    },
    getFileVersions: async (fileID: string) => {
      const bindings = getDesktopBindings()
      if (bindings?.FileVersions) return bindings.FileVersions(fileID)
      const response = await requestSafe<{ items?: FileVersion[] | null }>(`/api/v1/files/${encodeURIComponent(fileID)}/versions`)
      return response.items ?? []
    },
    getFileVersion: (fileID: string, versionID: string) => {
      const bindings = getDesktopBindings()
      if (bindings?.ReadFileVersion) return bindings.ReadFileVersion(fileID, versionID)
      return requestSafe<FileVersionContent>(`/api/v1/files/${encodeURIComponent(fileID)}/versions/${encodeURIComponent(versionID)}`)
    },
    setProjectFileRoot: (projectID: string, rootPath: string) => {
      const bindings = getDesktopBindings()
      if (bindings?.SetProjectFileRoot) return bindings.SetProjectFileRoot(projectID, rootPath)
      return requestSafe<FileHistoryRoot>(`/api/v1/projects/${encodeURIComponent(projectID)}/file-root`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ root_path: rootPath }),
      })
    },
    scanProjectFiles: (projectID: string) => {
      const bindings = getDesktopBindings()
      if (bindings?.ScanProjectFiles) return bindings.ScanProjectFiles(projectID)
      return requestSafe<ProjectFileScanResult>(`/api/v1/projects/${encodeURIComponent(projectID)}/files/scan`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
      })
    },
    compareFileVersions: (fileID: string, fromVersionID: string, toVersionID: string) => {
      const bindings = getDesktopBindings()
      if (bindings?.CompareFileVersions) return bindings.CompareFileVersions(fileID, fromVersionID, toVersionID)
      const query = new URLSearchParams({ from: fromVersionID, to: toVersionID })
      return requestSafe<FileCompareResult>(`/api/v1/files/${encodeURIComponent(fileID)}/compare?${query}`)
    },
    saveFileVersion: (fileID: string, input: FileHistorySaveInput) => {
      const bindings = getDesktopBindings()
      if (bindings?.SaveFileVersion) return bindings.SaveFileVersion(fileID, input)
      return requestSafe<FileHistoryMutationResult>(`/api/v1/files/${encodeURIComponent(fileID)}/versions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
    },
    restoreFileVersion: (fileID: string, input: FileHistoryRestoreInput) => {
      const bindings = getDesktopBindings()
      if (bindings?.RestoreFileVersion) return bindings.RestoreFileVersion(fileID, input)
      return requestSafe<FileHistoryMutationResult>(`/api/v1/files/${encodeURIComponent(fileID)}/restore`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
    },
    hasNativeDirectoryPicker: () => Boolean(getDesktopBindings()?.OpenDirectoryDialog),
    chooseProjectSourceDirectory: (defaultDirectory = '') => {
      const picker = getDesktopBindings()?.OpenDirectoryDialog
      if (!picker) return Promise.resolve('')
      return picker({ title: 'Choose TimeKeeper Project source folder', defaultDirectory })
    },
  }
}

export const timekeeperApi = createTimekeeperApi(import.meta.env.VITE_TIMEKEEPER_API_BASE_URL ?? '')

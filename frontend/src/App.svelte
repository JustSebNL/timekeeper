<script lang="ts">
import { onMount, tick } from 'svelte'
  import { timekeeperApi, type AgentWorkState, type GlobalSearchResponse, type GlobalSearchResult, type ProjectExecutionTree, type ProjectHandoffSnapshot, type ProjectSummary, type SprintAction, type TaskDetail, type TaskStatus } from './lib/api'
  import Sidebar from './lib/components/Sidebar.svelte'
  import ProjectWorkspace from './lib/ProjectWorkspace.svelte'
  import AgentPulseWorkspace from './lib/AgentPulseWorkspace.svelte'
  import MetricsWorkspace from './lib/MetricsWorkspace.svelte'
  import MemoryWorkspace from './lib/MemoryWorkspace.svelte'
  import HistoryWorkspace from './lib/HistoryWorkspace.svelte'
  import ConnectionWorkspace from './lib/ConnectionWorkspace.svelte'
  import RegistrationQueueWorkspace from './lib/RegistrationQueueWorkspace.svelte'
  import TaskWorkspace from './lib/TaskWorkspace.svelte'
  import PluginWorkspace from './lib/PluginWorkspace.svelte'
  import StatCard from './lib/components/StatCard.svelte'
  import Topbar from './lib/components/Topbar.svelte'
  import { workspaceSectionTitle, type WorkspaceSection } from './lib/navigation'
  import { runAndRefreshTaskWorkspace } from './lib/taskActions'
  import { loadTaskWorkspaceSnapshot, TaskWorkspaceRequestGuard } from './lib/taskWorkspace'
  import { readSelectedProjectID, readWorkspaceSection, saveSelectedProjectID, saveWorkspaceSection } from './lib/workspacePersistence'
  let projects = $state<ProjectSummary[]>([])
  let search = $state('')
  let globalSearchResponse = $state<GlobalSearchResponse | null>(null)
  let globalSearchLoading = $state(false)
  let globalSearchError = $state('')
  let globalSearchRequestID = 0
  let globalSearchDebounce: ReturnType<typeof setTimeout> | null = null
  let activeSection = $state<WorkspaceSection>('overview')
  let selectedProjectID = $state<string | null>(null)
  let executionTree = $state<ProjectExecutionTree | null>(null)
  let projectHandoff = $state<ProjectHandoffSnapshot | null>(null)
  let selectedTaskID = $state<string | null>(null)
  let taskDetail = $state<TaskDetail | null>(null)
  let taskLoading = $state(false)
  let taskError = $state('')
  let taskActionBusy = $state(false)
  const taskWorkspaceRequestGuard = new TaskWorkspaceRequestGuard()
  let workspaceLoading = $state(false)
  let workspaceError = $state('')
  let handoffLoading = $state(false)
  let handoffError = $state('')
  let projectWorkspaceRequestID = 0
  let loading = $state(true)
  let refreshing = $state(false)
  let coreOnline = $state(false)
  let agentWorkStates = $state<AgentWorkState[] | null>(null)
  let agentWorkStatesError = $state('')
  const agentWorkingCount = $derived(agentWorkStates === null ? null : agentWorkStates.filter((agent) => agent.state === 'working').length)
  let error = $state('')

  const visibleProjects = $derived.by(() => {
    const query = search.trim().toLocaleLowerCase()
    if (!query) return projects
    return projects.filter((project) =>
      `${project.project_name} ${project.project_id} ${project.status}`.toLocaleLowerCase().includes(query),
    )
  })

  const sectionTitle = $derived(workspaceSectionTitle(activeSection))

  function updateSearch(value: string) {
    search = value
    if (globalSearchDebounce !== null) clearTimeout(globalSearchDebounce)
    globalSearchDebounce = null
    const query = value.trim()
    const requestID = ++globalSearchRequestID
    globalSearchError = ''
    if (!query) {
      globalSearchResponse = null
      globalSearchLoading = false
      return
    }
    globalSearchResponse = null
    globalSearchLoading = true
    globalSearchDebounce = setTimeout(() => {
      globalSearchDebounce = null
      void runGlobalSearch(query, 20, requestID)
    }, 250)
  }

  async function runGlobalSearch(query: string, limit: number, requestID: number) {
    try {
      const response = await timekeeperApi.searchGlobal(query, limit)
      if (requestID === globalSearchRequestID && search.trim() === query) globalSearchResponse = response
    } catch (cause) {
      if (requestID === globalSearchRequestID && search.trim() === query) {
        globalSearchError = cause instanceof Error ? cause.message : 'Core search is unavailable.'
      }
    } finally {
      if (requestID === globalSearchRequestID && search.trim() === query) globalSearchLoading = false
    }
  }

  function loadMoreGlobalSearch() {
    if (!globalSearchResponse?.has_more || globalSearchLoading || globalSearchResponse.limit >= 200) return
    const query = search.trim()
    const requestID = ++globalSearchRequestID
    const nextLimit = Math.min(globalSearchResponse.limit + 20, 200)
    globalSearchLoading = true
    globalSearchError = ''
    void runGlobalSearch(query, nextLimit, requestID)
  }

  async function openGlobalSearchResult(result: GlobalSearchResult) {
    if (!result.project_id) return
    if (result.type !== 'message') updateSearch('')
    await openProject(result.project_id)
    if (result.type === 'task') await openTask(result.object_id)
  }

  function navigateWorkspace(section: WorkspaceSection) {
    activeSection = section
    saveWorkspaceSection(section)
    if (section === 'overview') {
      backToProjects()
      void refresh()
    }
  }

  function backToProjects() {
    projectWorkspaceRequestID++
    taskWorkspaceRequestGuard.invalidate()
    selectedProjectID = null
    selectedTaskID = null
    taskDetail = null
    taskError = ''
    executionTree = null
    projectHandoff = null
    workspaceError = ''
    handoffError = ''
    workspaceLoading = false
    handoffLoading = false
    saveSelectedProjectID(null)
  }

  async function openProject(projectID: string) {
    taskWorkspaceRequestGuard.invalidate()
    activeSection = 'projects'
    saveWorkspaceSection('projects')
    selectedProjectID = projectID
    selectedTaskID = null
    taskDetail = null
    taskError = ''
    saveSelectedProjectID(projectID)
    await loadProjectWorkspace(projectID)
  }

  async function loadProjectWorkspace(projectID: string) {
    const requestID = ++projectWorkspaceRequestID
    const isCurrent = () => requestID === projectWorkspaceRequestID && selectedProjectID === projectID
    executionTree = null
    projectHandoff = null
    workspaceError = ''
    handoffError = ''
    workspaceLoading = true
    handoffLoading = true

    const treeLoad = Promise.resolve()
      .then(() => timekeeperApi.getExecutionTree(projectID))
      .then((tree) => {
        if (isCurrent()) executionTree = tree
      })
      .catch((cause) => {
        if (isCurrent()) workspaceError = cause instanceof Error ? cause.message : 'Unable to load this Project hierarchy.'
      })
      .finally(() => {
        if (isCurrent()) workspaceLoading = false
      })

    const handoffLoad = Promise.resolve()
      .then(() => timekeeperApi.getProjectHandoff(projectID))
      .then((snapshot) => {
        if (isCurrent()) projectHandoff = snapshot
      })
      .catch((cause) => {
        if (isCurrent()) handoffError = cause instanceof Error ? cause.message : 'Unable to load recent Project activity.'
      })
      .finally(() => {
        if (isCurrent()) handoffLoading = false
      })

    await Promise.all([treeLoad, handoffLoad])
  }

  async function retryProjectWorkspace() {
    if (selectedProjectID) await loadProjectWorkspace(selectedProjectID)
  }

  async function loadTaskWorkspace(taskID: string): Promise<boolean> {
    if (!selectedProjectID || selectedTaskID !== taskID) return false
    const projectID = selectedProjectID
    const request = taskWorkspaceRequestGuard.begin(taskID, projectID)
    taskLoading = true
    taskError = ''
    try {
      const snapshot = await loadTaskWorkspaceSnapshot(timekeeperApi, taskID, projectID)
      if (!request.isCurrent(selectedTaskID, selectedProjectID)) return false
      taskDetail = snapshot.detail
      executionTree = snapshot.tree
      return true
    } catch (cause) {
      if (!request.isCurrent(selectedTaskID, selectedProjectID)) return false
      taskError = cause instanceof Error ? cause.message : 'Unable to load this Task workspace.'
      return false
    } finally {
      if (request.isCurrent(selectedTaskID, selectedProjectID)) taskLoading = false
    }
  }

  async function openTask(taskID: string) {
    selectedTaskID = taskID
    taskDetail = null
    taskError = ''
    await loadTaskWorkspace(taskID)
  }

  async function retryTaskWorkspace() {
    if (selectedTaskID) await loadTaskWorkspace(selectedTaskID)
  }

  function backToTaskHierarchy() {
    const returnTaskID = selectedTaskID
    taskWorkspaceRequestGuard.invalidate()
    selectedTaskID = null
    taskDetail = null
    taskError = ''
    taskLoading = false
    if (returnTaskID) {
      void tick().then(() => {
        const returnButton = Array.from(document.querySelectorAll<HTMLButtonElement>('[data-task-id]'))
          .find((button) => button.dataset.taskId === returnTaskID)
        returnButton?.focus()
      })
    }
  }

  async function updateTaskStatus(status: TaskStatus) {
    if (!selectedTaskID) throw new Error('No Task is selected.')
    taskActionBusy = true
    try {
      const taskID = selectedTaskID
      const projectID = selectedProjectID
      const refreshIfStillSelected = () => selectedTaskID === taskID && selectedProjectID === projectID
        ? loadTaskWorkspace(taskID)
        : Promise.resolve(false)
      await runAndRefreshTaskWorkspace(
        () => timekeeperApi.updateTaskStatus(taskID, status),
        refreshIfStillSelected,
        'Core accepted the status change, but the authoritative refresh failed. The displayed data may be stale; retry the refresh.',
      )
    } finally {
      taskActionBusy = false
    }
  }

  async function transitionSprint(sprintID: string, action: SprintAction, reason: string) {
    if (!selectedTaskID) throw new Error('No Task is selected.')
    taskActionBusy = true
    try {
      const taskID = selectedTaskID
      const projectID = selectedProjectID
      const refreshIfStillSelected = () => selectedTaskID === taskID && selectedProjectID === projectID
        ? loadTaskWorkspace(taskID)
        : Promise.resolve(false)
      await runAndRefreshTaskWorkspace(
        () => timekeeperApi.transitionSprint(sprintID, action, reason),
        refreshIfStillSelected,
        'Core accepted the Sprint action, but the authoritative refresh failed. The displayed data may be stale; retry the refresh.',
      )
    } finally {
      taskActionBusy = false
    }
  }

  async function refresh() {
    const restoreProjectID = selectedProjectID || readSelectedProjectID()
    const sectionBeforeRefresh = activeSection
    refreshing = true
    error = ''
    agentWorkStatesError = ''
    try {
      const activityRequest = timekeeperApi.getAgentWorkStates()
        .then((states) => ({ states, error: '' }))
        .catch((cause) => ({
          states: null,
          error: cause instanceof Error ? cause.message : 'Agent Pulse is unavailable from Core.',
        }))
      const [health, projectList, activity] = await Promise.all([
        timekeeperApi.getHealth(),
        timekeeperApi.getProjects(),
        activityRequest,
      ])
      coreOnline = health.status === 'ok'
      projects = projectList
      agentWorkStates = activity.states
      agentWorkStatesError = activity.error
      if (restoreProjectID && projectList.some((project) => project.project_id === restoreProjectID)) {
        selectedProjectID = restoreProjectID
        if (sectionBeforeRefresh === 'overview') {
          activeSection = 'projects'
          saveWorkspaceSection('projects')
        }
        if (activeSection === 'projects') {
          if (selectedTaskID) await loadTaskWorkspace(selectedTaskID)
          else await loadProjectWorkspace(restoreProjectID)
        }
      } else if (restoreProjectID) {
        backToProjects()
      }
    } catch (cause) {
      coreOnline = false
      agentWorkStates = null
      error = cause instanceof Error ? cause.message : 'Unable to reach the TimeKeeper Core.'
      agentWorkStatesError = error
      if (restoreProjectID) workspaceError = error
    } finally {
      loading = false
      refreshing = false
    }
  }

  onMount(() => {
    activeSection = readWorkspaceSection()
    selectedProjectID = readSelectedProjectID()
    if (selectedProjectID && activeSection === 'overview') activeSection = 'projects'
    void refresh()
  })
</script>

<svelte:head>
  <title>TimeKeeper · {sectionTitle}</title>
  <meta name="description" content="A Core-backed Svelte workspace for TimeKeeper Projects, Tasks, Sprints, and registration offers." />
</svelte:head>

  <div class="app-shell">
    <Sidebar activeSection={activeSection} onNavigate={navigateWorkspace} />

  <main id={activeSection} class="main-content">
    <Topbar
      sectionTitle={sectionTitle}
      {coreOnline}
      {search}
      {refreshing}
      onSearch={updateSearch}
      onRefresh={refresh}
    />

    {#if search.trim()}
      <section class="global-search-panel" aria-label="Global search results">
        {#if globalSearchError}
          <div class="search-panel-error" role="status">Search unavailable: {globalSearchError}</div>
        {/if}
        {#if globalSearchLoading && !globalSearchResponse}
          <div class="search-panel-state" role="status">Searching TimeKeeper Core…</div>
        {/if}
        {#if globalSearchResponse}
          <div class="global-search-heading">
            <strong>Search results</strong>
            <span>Showing {globalSearchResponse.items.length} of {globalSearchResponse.total}</span>
          </div>
          {#if globalSearchResponse.items.length === 0}
            <div class="search-panel-state">No results for “{globalSearchResponse.query}”.</div>
          {:else}
            <ol class="global-search-list" aria-label="Search result items">
              {#each globalSearchResponse.items as result (`${result.type}:${result.project_id}:${result.object_id}`)}
                <li>
                  <button
                    class="global-search-result"
                    type="button"
                    aria-label={result.type === 'message' ? `Open ${result.project_name} Project for message ${result.ref}` : result.type === 'task' ? `Open Task ${result.title} workspace` : `Open ${result.project_name} Project`}
                    onclick={() => void openGlobalSearchResult(result)}
                  >
                    <span class="global-search-kicker">{result.type}{#if result.kind} · {result.kind}{/if} · {result.project_name}</span>
                    <strong>{result.title}</strong>
                    <span class="global-search-snippet">{result.snippet}</span>
                    <small>{result.ref}{#if result.status} · {result.status}{/if}{#if result.tags} · {result.tags}{/if}</small>
                  </button>
                </li>
              {/each}
            </ol>
          {/if}
          {#if globalSearchResponse.has_more}
            {#if globalSearchResponse.limit < 200}
              <button class="global-search-more" type="button" onclick={loadMoreGlobalSearch} disabled={globalSearchLoading}>
                {globalSearchLoading ? 'Loading more…' : `Load ${Math.min(20, 200 - globalSearchResponse.limit)} more results`}
              </button>
            {:else}
              <p class="search-panel-state">Showing the first 200 results. Refine your query to narrow the list.</p>
            {/if}
          {/if}
        {/if}
      </section>
    {/if}

    <section class="page-heading">
      <div>
        <p class="eyebrow">{activeSection === 'overview' ? 'YOUR WORKSPACE AT A GLANCE' : activeSection === 'files' ? 'PROJECT FILES & HISTORY' : activeSection === 'settings' ? 'CONNECTION SETTINGS' : activeSection === 'queue' ? 'API REGISTRATION INBOX' : activeSection === 'agents' ? 'CORE-REPORTED AGENT ACTIVITY' : activeSection === 'metrics' ? 'CORE-OWNED PROJECT METRICS' : activeSection === 'memory' ? 'CROSS-PROJECT KNOWLEDGE' : activeSection === 'plugins' ? 'EXTENSIONS & STORE' : 'PROJECT WORKSPACE'}</p>
        <h1>{activeSection === 'overview' ? 'Good to see you.' : activeSection === 'files' ? 'Files & History' : activeSection === 'settings' ? 'Settings' : activeSection === 'queue' ? 'Registration Queue' : activeSection === 'agents' ? 'Agent Pulse' : activeSection === 'metrics' ? 'Metrics' : activeSection === 'memory' ? 'Memory Galaxy' : activeSection === 'plugins' ? 'Plugins' : 'Projects'}</h1>
        <p class="subtitle">{activeSection === 'overview' ? 'A clear view of the work in your TimeKeeper Core.' : activeSection === 'files' ? 'Inspect versioned source history through the authoritative Core.' : activeSection === 'settings' ? 'Test the active local endpoint and review Core compatibility.' : activeSection === 'queue' ? 'API offers are durably queued, processed FIFO, and then appear as normal Core work.' : activeSection === 'agents' ? 'Inspect agent state and work context as reported by Core.' : activeSection === 'metrics' ? 'Inspect project usage and recorded Sprint operations from Core.' : activeSection === 'memory' ? 'Search cross-project knowledge through the configured Core provider.' : activeSection === 'plugins' ? 'Inspect Core-registered plugins and extensions.' : 'Browse the Projects available from your TimeKeeper Core.'}</p>
      </div>
      {#if activeSection !== 'settings' && activeSection !== 'queue'}
        <button class="refresh-button" onclick={refresh} disabled={refreshing}>
          <span aria-hidden="true">↻</span> {refreshing ? 'Refreshing…' : 'Refresh data'}
        </button>
      {/if}
    </section>

    {#if activeSection === 'overview'}
      <section class="stat-grid" aria-label="Workspace summary">
        <StatCard icon="▦" tone="violet" caption="IN YOUR WORKSPACE" value={loading ? '—' : projects.length} label="Projects" />
        <StatCard icon="◉" tone="green" caption="CONNECTION" value={loading ? 'Checking' : coreOnline ? 'Healthy' : 'Offline'} label="Core service" />
        <StatCard icon="✓" tone="blue" caption="COMING IN A LATER SLICE" value="—" label="Active tasks" muted />
        <StatCard
          icon="⌁"
          tone="amber"
          caption={agentWorkStates === null ? 'ACTIVITY UNAVAILABLE' : 'AT LAST REFRESH'}
          value={agentWorkingCount ?? '—'}
          label="Agents working"
          muted={agentWorkStates === null}
        />
      </section>
    {/if}

    {#if activeSection === 'files'}
      <HistoryWorkspace projects={projects} initialProjectID={selectedProjectID} api={timekeeperApi} />
    {:else if activeSection === 'settings'}
      <ConnectionWorkspace api={timekeeperApi} />
    {:else if activeSection === 'queue'}
      <RegistrationQueueWorkspace projects={projects} initialProjectID={selectedProjectID} api={timekeeperApi} />
    {:else if activeSection === 'agents'}
      <AgentPulseWorkspace
        states={agentWorkStates}
        loading={loading || (refreshing && agentWorkStates === null)}
        error={agentWorkStatesError}
        onRefresh={refresh}
      />
    {:else if activeSection === 'metrics'}
      <MetricsWorkspace projects={projects} initialProjectID={selectedProjectID} api={timekeeperApi} />
    {:else if activeSection === 'memory'}
      <MemoryWorkspace api={timekeeperApi} />
    {:else if activeSection === 'plugins'}
      <PluginWorkspace api={timekeeperApi} />
    {:else if selectedProjectID}
      {#if selectedTaskID}
        <TaskWorkspace
          detail={taskDetail}
          loading={taskLoading}
          error={taskError}
          busy={taskActionBusy}
          onBack={backToTaskHierarchy}
          onRetry={retryTaskWorkspace}
          onTaskStatusChange={updateTaskStatus}
          onSprintAction={transitionSprint}
        />
      {:else}
        <ProjectWorkspace
          tree={executionTree}
          handoff={projectHandoff}
          loading={workspaceLoading}
          error={workspaceError}
          handoffLoading={handoffLoading}
          handoffError={handoffError}
          onBack={backToProjects}
          onRetry={retryProjectWorkspace}
          onSelectTask={openTask}
        />
      {/if}
    {:else}
      <section class="projects-section" aria-labelledby="projects-title">
        <div class="section-heading">
          <div><p class="eyebrow">YOUR TIMEKEEPER DATA</p><h2 id="projects-title">Projects <span class="count">{loading ? '…' : visibleProjects.length}</span></h2></div>
          <span class="read-only-tag"><span>◉</span> Core-backed workspace</span>
        </div>

        {#if error}
          <div class="state-panel error-panel" role="alert">
            <span class="state-symbol">!</span>
            <div><strong>Couldn’t connect to TimeKeeper Core</strong><p>{error}</p><small>Check that the local Core server is running, then try again.</small></div>
            <button class="text-button" onclick={refresh}>Retry</button>
          </div>
        {:else if loading}
          <div class="state-panel" aria-live="polite"><span class="loading-dot"></span> Loading projects from Core…</div>
        {:else if visibleProjects.length === 0}
          <div class="state-panel">
            <span class="state-symbol">⌕</span>
            <div><strong>{search ? 'No matching projects' : 'No projects yet'}</strong><p>{search ? 'Try a different filter.' : 'Projects will appear here when they are available in Core.'}</p></div>
          </div>
        {:else}
          <div class="project-list">
            {#each visibleProjects as project (project.project_id)}
              <article class="project-row">
                <span class="project-mark">{project.project_name.slice(0, 1).toUpperCase()}</span>
                <button class="project-info project-open" type="button" aria-label={`Open ${project.project_name} Project workspace`} onclick={() => openProject(project.project_id)}>
                  <strong>{project.project_name}</strong><span>{project.project_id}{project.project_goal ? ` · ${project.project_goal}` : ''}</span>
                </button>
                <span class="project-status"><i></i>{project.status}</span>
                {#if typeof project.calculated_completion_pct === 'number'}
                  <div class="project-progress" aria-label={`${Math.round(project.calculated_completion_pct)}% complete`}><span style={`width: ${Math.max(0, Math.min(100, project.calculated_completion_pct))}%`}></span></div>
                {/if}
                <span class="row-arrow" aria-hidden="true">→</span>
              </article>
            {/each}
          </div>
        {/if}
      </section>
    {/if}

    <footer class="footer"><span><i class:online={coreOnline}></i> {coreOnline ? 'Connected to TimeKeeper Core' : 'Waiting for Core connection'}</span><span>Core-backed migration workspace</span></footer>
  </main>
</div>

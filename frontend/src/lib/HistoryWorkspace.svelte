<script lang="ts">
  import { tick } from 'svelte'
  import { buildHistoryLanes } from './historyTimeline'
  import type { FileCompareResult, FileHistoryRestoreInput, ProjectSummary, TrackedFile, createTimekeeperApi } from './api'

  interface Props {
    projects: ProjectSummary[]
    initialProjectID: string | null
    api: ReturnType<typeof createTimekeeperApi>
  }

  let { projects, initialProjectID, api }: Props = $props()
  let projectID = $state('')
  let rootPath = $state('')
  let persistedRootPath = $state('')
  let rootStatus = $state('')
  let loadingProject = $state(false)
  let loadingFiles = $state(false)
  let loadingVersion = $state(false)
  let busy = $state(false)
  let error = $state('')
  let scanSummary = $state('')
  let fileFilter = $state('')
  let files = $state<TrackedFile[]>([])
  let fileID = $state('')
  let versions = $state<Awaited<ReturnType<typeof api.getFileVersions>>>([])
  let selectedVersionID = $state('')
  let versionContent = $state('')
  let versionError = $state('')
  let compareFromID = $state('')
  let compareToID = $state('')
  let comparison = $state<FileCompareResult | null>(null)
  let compareError = $state('')
  let projectRequest = 0
  let fileRequest = 0
  let versionRequest = 0

  const visibleFiles = $derived.by(() => {
    const query = fileFilter.trim().toLocaleLowerCase()
    return files.filter(file => !query || `${file.current_path} ${file.file_id} ${file.file_type}`.toLocaleLowerCase().includes(query))
  })
  const selectedFile = $derived(files.find(file => file.file_id === fileID) ?? null)
  const timelineLanes = $derived(buildHistoryLanes(versions))

  $effect(() => {
    if (projectID) return
    const initial = projects.some(project => project.project_id === initialProjectID)
      ? initialProjectID
      : projects[0]?.project_id ?? null
    if (initial) {
      projectID = initial
      void loadProject(initial)
    }
  })

  async function loadProject(targetProjectID: string) {
    const request = ++projectRequest
    fileRequest++
    versionRequest++
    projectID = targetProjectID
    rootPath = ''
    persistedRootPath = ''
    rootStatus = ''
    files = []
    fileID = ''
    versions = []
    selectedVersionID = ''
    versionContent = ''
    comparison = null
    scanSummary = ''
    error = ''
    loadingProject = true
    try {
      const [root, tracked] = await Promise.all([
        api.getProjectFileRoot(targetProjectID),
        api.getProjectFiles(targetProjectID),
      ])
      if (request !== projectRequest) return
      rootPath = root.root_path || ''
      persistedRootPath = rootPath
      files = tracked
      rootStatus = rootPath
        ? `Source folder: ${rootPath}`
        : 'Source folder is not configured. Choose a folder on the Core host before indexing.'
    } catch (cause) {
      if (request !== projectRequest) return
      error = cause instanceof Error ? cause.message : 'Could not load file history.'
      rootPath = ''
      persistedRootPath = ''
      rootStatus = 'File history is unavailable.'
    } finally {
      if (request === projectRequest) loadingProject = false
    }
  }

  async function chooseProject() {
    if (projectID) await loadProject(projectID)
  }

  async function browseSourceFolder() {
    if (!api.hasNativeDirectoryPicker()) {
      rootStatus = 'Use the path field to enter a folder visible to the TimeKeeper Core host.'
      return
    }
    try {
      const selected = await api.chooseProjectSourceDirectory(rootPath)
      if (selected) rootPath = selected
    } catch (cause) {
      rootStatus = cause instanceof Error ? cause.message : 'Could not open the folder picker.'
    }
  }

  async function setSourceFolder() {
    if (!projectID || !rootPath.trim()) return
    busy = true
    error = ''
    scanSummary = ''
    try {
      const saved = await api.setProjectFileRoot(projectID, rootPath.trim())
      rootPath = saved.root_path
      persistedRootPath = saved.root_path
      rootStatus = `Source folder configured: ${saved.root_path}`
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Could not configure this source folder.'
    } finally {
      busy = false
    }
  }

  async function scanProject() {
    if (!projectID || !rootPath.trim() || rootPath.trim() !== persistedRootPath || busy) return
    if (!window.confirm('Index supported text files in this Project source folder? Existing versions remain preserved.')) return
    busy = true
    error = ''
    scanSummary = ''
    try {
      const result = await api.scanProjectFiles(projectID)
      const summary = `Scanned ${result.files_discovered} text files · ${result.files_added} added · ${result.versions_added} new versions · ${result.files_skipped} skipped.`
      await loadProject(projectID)
      scanSummary = summary
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Project scan failed.'
    } finally {
      busy = false
    }
  }

  async function openFile(file: TrackedFile) {
    const request = ++fileRequest
    versionRequest++
    fileID = file.file_id
    versions = []
    selectedVersionID = ''
    versionContent = ''
    versionError = ''
    comparison = null
    compareError = ''
    loadingFiles = true
    try {
      const loadedVersions = await api.getFileVersions(file.file_id)
      if (request !== fileRequest) return
      versions = loadedVersions.slice().sort((a, b) => a.version_number - b.version_number)
      await tick()
      const selected = versions.find(version => version.version_id === file.current_version_id) ?? versions.at(-1)
      if (selected) await selectVersion(selected.version_id)
      compareFromID = versions.length > 1 ? versions[versions.length - 2].version_id : versions[0]?.version_id ?? ''
      compareToID = versions.at(-1)?.version_id ?? ''
    } catch (cause) {
      if (request !== fileRequest) return
      versionError = cause instanceof Error ? cause.message : 'Could not load file history.'
    } finally {
      if (request === fileRequest) loadingFiles = false
    }
  }

  async function selectVersion(targetVersionID: string) {
    if (!fileID || !targetVersionID) return
    const request = ++versionRequest
    selectedVersionID = targetVersionID
    versionError = ''
    loadingVersion = true
    try {
      const result = await api.getFileVersion(fileID, targetVersionID)
      if (request !== versionRequest) return
      versionContent = result.content
    } catch (cause) {
      if (request !== versionRequest) return
      versionError = cause instanceof Error ? cause.message : 'Could not read this saved version.'
    } finally {
      if (request === versionRequest) loadingVersion = false
    }
  }

  async function compareVersions() {
    if (!fileID || !compareFromID || !compareToID || compareFromID === compareToID) return
    compareError = ''
    comparison = null
    try {
      comparison = await api.compareFileVersions(fileID, compareFromID, compareToID)
    } catch (cause) {
      compareError = cause instanceof Error ? cause.message : 'Could not compare these versions.'
    }
  }

  async function restoreVersion() {
    if (!selectedFile || !selectedVersionID || busy) return
    const targetFileID = selectedFile.file_id
    const expectedCurrentVersionID = selectedFile.current_version_id
    if (!expectedCurrentVersionID) return
    const selected = versions.find(version => version.version_id === selectedVersionID)
    if (!selected || selected.version_id === expectedCurrentVersionID) return
    if (!window.confirm(`Restore V${selected.version_number} as a new history branch? Existing versions will remain intact.`)) return
    busy = true
    error = ''
    try {
      await api.restoreFileVersion(targetFileID, {
        version_id: selectedVersionID,
        expected_current_version_id: expectedCurrentVersionID,
      } satisfies FileHistoryRestoreInput)
      await loadProject(projectID)
      const restoredFile = files.find(file => file.file_id === targetFileID)
      if (restoredFile) await openFile(restoredFile)
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Could not restore this saved version.'
    } finally {
      busy = false
    }
  }

  function versionLabel(version: (typeof versions)[number]) {
    const state = version.version_id === selectedFile?.current_version_id ? ' · current' : ''
    const summary = version.change_summary || version.change_kind || 'Snapshot'
    const branch = version.restores_version_id ? ` · reverts ${version.restores_version_id}` : ''
    return `V${version.version_number}${state} · ${summary}${branch}`
  }
</script>

<section class="history-workspace" aria-labelledby="history-title">
  <header class="history-heading">
    <div>
      <p class="eyebrow">CORE-BACKED · PROJECT-SCOPED</p>
      <h2 id="history-title">Files &amp; History</h2>
      <p class="subtitle">Browse immutable source versions and branch-aware restores. Core remains the only history authority.</p>
    </div>
  </header>

  {#if error}<p class="history-error" role="alert">{error}</p>{/if}

  <div class="history-controls">
    <label>
      Project
      <select aria-label="Project for file history" bind:value={projectID} onchange={chooseProject} disabled={loadingProject || projects.length === 0}>
        <option value="">Choose a Project…</option>
        {#each projects as project (project.project_id)}
          <option value={project.project_id}>{project.project_name} · {project.project_id}</option>
        {/each}
      </select>
    </label>
    <label class="history-root-field">
      Project source folder path
      <div class="history-root-input">
        <input aria-label="Project source folder path" bind:value={rootPath} placeholder="Path visible to the Core host" />
        {#if api.hasNativeDirectoryPicker()}
          <button type="button" class="secondary-button" onclick={browseSourceFolder}>Browse…</button>
        {/if}
      </div>
    </label>
    <button type="button" class="secondary-button" onclick={setSourceFolder} disabled={!projectID || !rootPath.trim() || busy}>Set source folder</button>
    <button type="button" class="primary-button" onclick={scanProject} disabled={!projectID || !rootPath.trim() || rootPath.trim() !== persistedRootPath || busy}>Scan and index</button>
  </div>

  <p class="history-root-status" role="status">{loadingProject ? 'Loading file history…' : rootStatus}</p>
  {#if scanSummary}<p class="history-scan-summary" role="status">{scanSummary}</p>{/if}

  {#if projects.length === 0}
    <div class="history-empty"><strong>No Projects available</strong><span>Choose a Project in Core before browsing file history.</span></div>
  {:else if projectID && rootPath.trim() && !loadingProject && files.length === 0}
    <div class="history-empty"><strong>No files indexed yet</strong><span>Scanning is explicit; select “Scan and index” to capture supported text files.</span></div>
  {/if}

  <div class="history-layout">
    <section class="history-file-panel" aria-label="Tracked Project files">
      <div class="history-panel-heading"><h3>Tracked files</h3><span>{files.length}</span></div>
      <label class="history-filter-label">Filter files<input aria-label="Filter tracked files" bind:value={fileFilter} placeholder="Path, ID, or type" /></label>
      {#if loadingProject || loadingFiles}
        <p class="history-muted" role="status">Loading tracked files…</p>
      {:else if !rootPath.trim()}
        <p class="history-muted">Set a source folder before indexing this Project.</p>
      {:else if visibleFiles.length === 0}
        <p class="history-muted">{files.length ? 'No files match this filter.' : 'No tracked files in this Project yet.'}</p>
      {:else}
        <ul class="history-file-list" aria-label="File results">
          {#each visibleFiles as file (file.file_id)}
            <li>
              <button type="button" class:active={file.file_id === fileID} aria-label={`Open ${file.current_path}`} onclick={() => openFile(file)}>
                <strong>{file.current_path}</strong><small>{file.file_type || 'text'} · {file.file_id}</small>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <section class="history-detail-panel" aria-label="File version history">
      {#if !selectedFile}
        <div class="history-empty"><strong>Select a tracked file</strong><span>Saved versions and restore branches will appear here.</span></div>
      {:else}
        <div class="history-panel-heading"><div><p class="eyebrow">{selectedFile.file_id}</p><h3>{selectedFile.current_path}</h3></div><span>{versions.length} versions</span></div>
        {#if loadingFiles}<p class="history-muted" role="status">Loading version history…</p>{/if}
        {#if versionError}<p class="history-error" role="alert">{versionError}</p>{/if}
        {#if versions.length === 0 && !loadingFiles}<p class="history-muted">No saved versions for this tracked file.</p>{/if}

        {#if versions.length}
          <div class="history-timeline" aria-label="Version timeline">
            {#each timelineLanes as lane (lane.id)}
              <section class:branch={lane.branch} class="history-lane" aria-label={lane.label}>
                <header><strong>{lane.label}</strong>{#if lane.branch}<small>Restored from {lane.sourceVersionID}</small>{:else}<small>{lane.versions.length} revisions</small>{/if}</header>
                <ol>
                  {#each lane.versions as version (version.version_id)}
                    <li>
                      <button type="button" class:active={version.version_id === selectedVersionID} aria-pressed={version.version_id === selectedVersionID} onclick={() => selectVersion(version.version_id)}>
                        <strong>{versionLabel(version)}</strong>
                        <small>{version.created_at ? new Date(version.created_at).toLocaleString() : 'Time unknown'} · {version.size_bytes ?? 0} bytes</small>
                      </button>
                    </li>
                  {/each}
                </ol>
              </section>
            {/each}
          </div>

          <div class="history-actions">
            <label>Compare from
              <select aria-label="Compare from version" bind:value={compareFromID}>
                {#each versions as version (version.version_id)}<option value={version.version_id}>V{version.version_number}</option>{/each}
              </select>
            </label>
            <label>Compare to
              <select aria-label="Compare to version" bind:value={compareToID}>
                {#each versions as version (version.version_id)}<option value={version.version_id}>V{version.version_number}</option>{/each}
              </select>
            </label>
            <button type="button" class="secondary-button" onclick={compareVersions} disabled={busy || versions.length < 2 || compareFromID === compareToID}>Compare</button>
            <button type="button" class="secondary-button" onclick={restoreVersion} disabled={busy || !selectedVersionID || selectedVersionID === selectedFile.current_version_id}>Restore selected version</button>
          </div>
          {#if compareError}<p class="history-error" role="alert">{compareError}</p>{/if}
          {#if comparison}
            <section class="history-diff" aria-label="Version comparison">
              <h4>V{comparison.from.version_number} → V{comparison.to.version_number}</h4>
              {#if comparison.hunks.length === 0}<p>No line differences.</p>{:else}
                {#each comparison.hunks as hunk, index (`${index}-${hunk.from_start}-${hunk.to_start}`)}
                  <div class="history-hunk">
                    {#each hunk.removed as line, lineIndex (`r-${index}-${lineIndex}`)}<div class="removed-line">− {line}</div>{/each}
                    {#each hunk.added as line, lineIndex (`a-${index}-${lineIndex}`)}<div class="added-line">+ {line}</div>{/each}
                  </div>
                {/each}
              {/if}
            </section>
          {/if}

          <section class="history-preview" aria-label="Saved version preview">
            <header><h4>Saved version preview</h4><span>{loadingVersion ? 'Loading…' : selectedVersionID}</span></header>
            {#if versionError}<p class="history-error">{versionError}</p>{:else}<pre><code>{versionContent}</code></pre>{/if}
          </section>
        {/if}
      {/if}
    </section>
  </div>
</section>

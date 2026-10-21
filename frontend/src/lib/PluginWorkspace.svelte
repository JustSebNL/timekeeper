<script lang="ts">
  import { onMount } from 'svelte'
  import type { PluginCatalog, PluginCatalogEntry } from './api'

  interface PluginAPI {
    getPluginCatalog: () => Promise<PluginCatalog>
  }

  interface Props { api: PluginAPI }

  let { api }: Props = $props()
  let catalog = $state<PluginCatalog | null>(null)
  let loading = $state(true)
  let error = $state('')
  let query = $state('')
  let selectedRuntime = $state('all')
  let requestID = 0

  const plugins = $derived(catalog?.plugins ?? [])
  const runtimes = $derived.by(() => {
    const set = new Set<string>()
    for (const p of plugins) {
      if (p.runtime) set.add(p.runtime)
    }
    return Array.from(set).sort()
  })

  const visiblePlugins = $derived.by(() => {
    const q = query.trim().toLowerCase()
    return plugins.filter((p) => {
      if (selectedRuntime !== 'all' && p.runtime !== selectedRuntime) return false
      if (!q) return true
      return (
        p.name.toLowerCase().includes(q) ||
        p.id.toLowerCase().includes(q) ||
        (p.description ?? '').toLowerCase().includes(q) ||
        (p.capabilities ?? []).some((c) => c.toLowerCase().includes(q))
      )
    })
  })

  onMount(() => { void refreshCatalog() })

  async function refreshCatalog() {
    const currentID = ++requestID
    loading = true
    error = ''
    try {
      const response = await api.getPluginCatalog()
      if (currentID === requestID) catalog = response
    } catch (cause) {
      if (currentID === requestID) {
        error = cause instanceof Error ? cause.message : 'Core could not load the plugin catalog.'
      }
    } finally {
      if (currentID === requestID) loading = false
    }
  }
</script>

<section class="plugin-workspace" aria-labelledby="plugin-workspace-title">
  <header class="plugin-heading">
    <div>
      <p class="eyebrow">EXTENSIONS &amp; STORE</p>
      <h2 id="plugin-workspace-title">Plugins</h2>
      <p class="plugin-subtitle">Inspect Core-registered plugins and extensions. The plugin catalog is read only.</p>
    </div>
    <div class="plugin-actions">
      <span class="read-only-tag">Core-backed · Read only</span>
      <button class="text-button" type="button" aria-label="Refresh plugin catalog" onclick={() => void refreshCatalog()} disabled={loading}>Refresh catalog</button>
    </div>
  </header>

  {#if loading && !catalog}
    <div class="state-panel" role="status">Loading plugin catalog from Core…</div>
  {:else if error}
    <div class="state-panel error-panel" role="alert">
      <strong>Plugin catalog is unavailable</strong>
      <p>Core could not load the plugin catalog.</p>
      <small>{error}</small>
    </div>
  {:else if catalog && plugins.length === 0}
    <div class="state-panel" role="status">
      <strong>No plugins configured</strong>
      <p>No extensions or plugins are currently registered with this TimeKeeper Core instance.</p>
    </div>
  {:else if catalog}
    <div class="plugin-summary-bar" role="status">
      <span><strong>Plugins</strong> {plugins.length} registered</span>
      {#if catalog.generated_at}<span><strong>Updated</strong> {catalog.generated_at}</span>{/if}
      <span><strong>Schema</strong> {catalog.schema_version}</span>
    </div>

    <div class="plugin-controls">
      <label class="plugin-search-label" for="plugin-filter">Filter plugins
        <input
          id="plugin-filter"
          type="search"
          placeholder="Filter by name, ID, capability…"
          bind:value={query}
        />
      </label>
      {#if runtimes.length > 1}
        <div class="runtime-filters" role="group" aria-label="Filter by runtime">
          <button
            type="button"
            class="filter-pill"
            class:active={selectedRuntime === 'all'}
            onclick={() => { selectedRuntime = 'all' }}
          >
            All
          </button>
          {#each runtimes as rt (rt)}
            <button
              type="button"
              class="filter-pill"
              class:active={selectedRuntime === rt}
              onclick={() => { selectedRuntime = rt }}
            >
              {rt}
            </button>
          {/each}
        </div>
      {/if}
    </div>

    {#if visiblePlugins.length === 0}
      <div class="state-panel" role="status">No plugins match the current filter.</div>
    {:else}
      <ol class="plugin-list" aria-label="Plugin list">
        {#each visiblePlugins as plugin (plugin.id)}
          <li class="plugin-card" data-plugin-id={plugin.id}>
            <div class="plugin-card-header">
              <div class="plugin-card-title">
                <strong>{plugin.name}</strong>
                <span class="plugin-version">v{plugin.version}</span>
                <code>{plugin.id}</code>
              </div>
              <div class="plugin-badges">
                <span class="badge runtime-badge">{plugin.runtime}</span>
                {#if plugin.protocol}<span class="badge protocol-badge">{plugin.protocol}</span>{/if}
              </div>
            </div>

            {#if plugin.description}
              <p class="plugin-desc">{plugin.description}</p>
            {/if}

            <div class="plugin-meta-row">
              {#if plugin.entrypoint}
                <span class="plugin-entrypoint"><strong>Entrypoint:</strong> <code>{plugin.entrypoint}</code></span>
              {/if}
              {#if plugin.capabilities && plugin.capabilities.length > 0}
                <div class="plugin-caps">
                  {#each plugin.capabilities as cap (cap)}
                    <span class="cap-tag">{cap}</span>
                  {/each}
                </div>
              {/if}
            </div>

            {#if plugin.requires && plugin.requires.length > 0}
              <div class="plugin-requires">
                <small>Requires: {plugin.requires.join(', ')}</small>
              </div>
            {/if}
          </li>
        {/each}
      </ol>
      <p class="plugin-readonly-note">Plugins execute in isolated subprocesses outside the Core process; installation and configuration changes must be performed through the TimeKeeper CLI or Store gateway.</p>
    {/if}
  {/if}
</section>

<style>
  .plugin-workspace { display: grid; gap: 14px; }
  .plugin-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
  .plugin-heading h2 { margin: 0; color: #253246; font-size: 18px; }
  .plugin-heading .eyebrow { margin: 0 0 5px; }
  .plugin-subtitle { margin: 5px 0 0; color: #7a8798; font-size: 11px; }
  .plugin-actions { display: flex; align-items: center; gap: 12px; }
  .plugin-summary-bar { display: flex; flex-wrap: wrap; gap: 8px 18px; padding: 10px 12px; border: 1px solid var(--line, #e9edf2); border-radius: 8px; background: #fff; color: #6d798b; font-size: 9px; }
  .plugin-summary-bar strong { color: #344052; }
  .plugin-controls { display: flex; flex-wrap: wrap; align-items: end; gap: 12px; }
  .plugin-search-label { display: grid; flex: 1; min-width: 200px; gap: 5px; color: #687487; font-size: 9px; font-weight: 650; }
  .plugin-search-label input { min-width: 0; padding: 8px 10px; border: 1px solid #e4e8ee; border-radius: 7px; color: #344052; font: inherit; font-size: 11px; }
  .plugin-search-label input:focus { outline: 2px solid #ddd6fe; border-color: #8b5cf6; }
  .runtime-filters { display: flex; gap: 6px; }
  .filter-pill { padding: 4px 10px; border: 1px solid #e4e8ee; border-radius: 999px; background: #fff; color: #687487; font: inherit; font-size: 9px; cursor: pointer; transition: all 120ms ease; }
  .filter-pill:hover { border-color: #cbd5e1; color: #334155; }
  .filter-pill.active { background: #7057c9; border-color: #7057c9; color: #fff; }
  .plugin-list { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
  .plugin-card { display: grid; gap: 8px; padding: 14px 16px; border: 1px solid var(--line, #e9edf2); border-radius: 9px; background: #fff; }
  .plugin-card-header { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .plugin-card-title { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
  .plugin-card-title strong { color: #253246; font-size: 13px; }
  .plugin-version { font-size: 10px; color: #8792a1; font-weight: 600; }
  .plugin-card-title code { color: #8792a1; font-size: 9px; background: #f8fafc; padding: 2px 6px; border-radius: 4px; }
  .plugin-badges { display: flex; gap: 6px; }
  .badge { padding: 2px 8px; border-radius: 999px; font-size: 8px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; }
  .runtime-badge { background: #ede9fe; color: #6d28d9; }
  .protocol-badge { background: #e0f2fe; color: #0369a1; }
  .plugin-desc { margin: 0; color: #596579; font-size: 11px; line-height: 1.5; }
  .plugin-meta-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; font-size: 9px; }
  .plugin-entrypoint { color: #64748b; }
  .plugin-entrypoint code { color: #334155; }
  .plugin-caps { display: flex; flex-wrap: wrap; gap: 5px; }
  .cap-tag { padding: 2px 7px; border-radius: 999px; color: #047857; background: #ecfdf5; font-size: 8px; }
  .plugin-requires { color: #94a3b8; font-size: 9px; }
  .plugin-readonly-note { margin: 0; color: #828d9c; font-size: 9px; }
  @media (max-width: 780px) {
    .plugin-heading { flex-direction: column; }
    .plugin-actions { width: 100%; justify-content: space-between; }
    .plugin-controls { flex-direction: column; align-items: stretch; }
  }
</style>

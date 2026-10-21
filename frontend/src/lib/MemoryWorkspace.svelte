<script lang="ts">
  import { onMount } from 'svelte'
  import type { KnowledgeProviderStatus, KnowledgeResult } from './api'

  interface MemoryAPI {
    getKnowledgeStatus: () => Promise<KnowledgeProviderStatus>
    searchKnowledge: (query: string, limit: number) => Promise<KnowledgeResult[]>
  }

  interface Props { api: MemoryAPI }

  let { api }: Props = $props()
  let status = $state<KnowledgeProviderStatus | null>(null)
  let statusLoading = $state(true)
  let statusError = $state('')
  let query = $state('')
  let searching = $state(false)
  let searchError = $state('')
  let results = $state<KnowledgeResult[] | null>(null)
  let statusRequestID = 0
  let searchRequestID = 0
  const canSearch = $derived(status?.available === true && (status.capabilities ?? []).includes('knowledge.search'))

  onMount(() => { void refreshStatus() })

  async function refreshStatus() {
    const requestID = ++statusRequestID
    searchRequestID++
    searching = false
    status = null
    statusError = ''
    searchError = ''
    results = null
    statusLoading = true
    try {
      const response = await api.getKnowledgeStatus()
      if (requestID === statusRequestID) status = response
    } catch (cause) {
      if (requestID === statusRequestID) {
        statusError = cause instanceof Error ? cause.message : 'Core could not reach the Memory Galaxy provider.'
      }
    } finally {
      if (requestID === statusRequestID) statusLoading = false
    }
  }

  async function submitSearch(event: SubmitEvent) {
    event.preventDefault()
    const normalizedQuery = query.trim()
    if (!canSearch || !normalizedQuery || searching) return
    const requestID = ++searchRequestID
    searchError = ''
    results = null
    searching = true
    try {
      const response = await api.searchKnowledge(normalizedQuery, 8)
      if (requestID === searchRequestID) results = response ?? []
    } catch (cause) {
      if (requestID === searchRequestID) {
        searchError = cause instanceof Error ? cause.message : 'Memory Galaxy search is unavailable.'
      }
    } finally {
      if (requestID === searchRequestID) searching = false
    }
  }
</script>

<section class="memory-workspace" aria-labelledby="memory-workspace-title">
  <header class="memory-heading">
    <div>
      <p class="eyebrow">CROSS-PROJECT KNOWLEDGE</p>
      <h2 id="memory-workspace-title">Memory Galaxy</h2>
      <p class="memory-subtitle">Search provider-managed knowledge through the TimeKeeper Core boundary. Results are read only.</p>
    </div>
    <div class="memory-actions">
      <span class="read-only-tag">Core-backed · Read only</span>
      <button class="text-button" type="button" aria-label="Refresh Memory Galaxy status" onclick={() => void refreshStatus()} disabled={statusLoading}>Refresh status</button>
    </div>
  </header>

  {#if statusLoading && !status}
    <div class="state-panel" role="status">Checking Memory Galaxy through Core…</div>
  {:else if statusError}
    <div class="state-panel error-panel" role="alert">
      <strong>Memory Galaxy is unavailable</strong>
      <p>Core could not reach an available knowledge provider. Search is disabled.</p>
      <small>{statusError}</small>
    </div>
  {:else if status && !status.available}
    <div class="state-panel" role="status">
      <strong>Memory Galaxy provider is unavailable</strong>
      <p>Core reports the provider is not available. Search is disabled until Core reports it as available.</p>
    </div>
  {:else if status && !canSearch}
    <div class="state-panel" role="status">
      <strong>Knowledge search is unavailable</strong>
      <p>The configured provider does not advertise the knowledge.search capability.</p>
    </div>
  {:else if canSearch}
    <div class="memory-provider-summary" role="status">
      <span><strong>Provider</strong> {status?.provider || 'Unknown'}</span>
      <span><strong>Protocol</strong> {status?.protocol_version || 'Unknown'}</span>
      <span><strong>Capabilities</strong> {(status?.capabilities ?? []).join(', ')}</span>
    </div>

    <form class="memory-search" onsubmit={submitSearch}>
      <label for="memory-query">Search Memory Galaxy
        <input id="memory-query" aria-label="Search Memory Galaxy" bind:value={query} maxlength="512" placeholder="Search cross-project knowledge…" />
      </label>
      <button class="primary-button" type="submit" disabled={!query.trim() || searching}>
        {searching ? 'Searching…' : 'Search knowledge'}
      </button>
    </form>

    {#if searchError}
      <div class="state-panel error-panel" role="alert"><strong>Knowledge search failed</strong><p>{searchError}</p></div>
    {:else if searching}
      <div class="state-panel" role="status">Searching Core knowledge…</div>
    {:else if results && results.length === 0}
      <div class="state-panel" role="status">No knowledge results for this query.</div>
    {:else if results}
      <ol class="knowledge-results" aria-label="Memory Galaxy search results">
        {#each results as result (result.id)}
          <li data-knowledge-id={result.id}>
            <div class="knowledge-result-heading">
              <strong>{result.title}</strong>
              <code>{result.id}</code>
            </div>
            {#if result.summary}<p>{result.summary}</p>{/if}
            <div class="knowledge-result-meta">
              <span>{result.type}</span><span>{result.review_status}</span><span>{result.visibility}</span>
            </div>
            {#if result.topics?.length}<small>Topics: {result.topics.join(', ')}</small>{/if}
            {#if result.tags?.length}<small>Tags: {result.tags.join(', ')}</small>{/if}
          </li>
        {/each}
      </ol>
      <p class="memory-readonly-note">Knowledge results are displayed as read-only summaries; promotion and edits are not available here.</p>
    {/if}
  {/if}
</section>

<style>
  .memory-workspace { display: grid; gap: 14px; }
  .memory-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
  .memory-heading h2 { margin: 0; color: #253246; font-size: 18px; }
  .memory-heading .eyebrow { margin: 0 0 5px; }
  .memory-subtitle { margin: 5px 0 0; color: #7a8798; font-size: 11px; }
  .memory-actions { display: flex; align-items: center; gap: 12px; }
  .memory-provider-summary { display: flex; flex-wrap: wrap; gap: 8px 18px; padding: 10px 12px; border: 1px solid var(--line, #e9edf2); border-radius: 8px; background: #fff; color: #6d798b; font-size: 9px; }
  .memory-provider-summary strong { color: #344052; }
  .memory-search { display: flex; align-items: end; gap: 10px; padding: 13px; border: 1px solid var(--line, #e9edf2); border-radius: 9px; background: #fff; }
  .memory-search label { display: grid; flex: 1; gap: 5px; color: #687487; font-size: 9px; font-weight: 650; }
  .memory-search input { min-width: 0; padding: 9px 10px; border: 1px solid #e4e8ee; border-radius: 7px; color: #344052; font: inherit; }
  .memory-search input:focus { outline: 2px solid #ddd6fe; border-color: #8b5cf6; }
  .memory-search button { padding: 9px 13px; border: 0; border-radius: 7px; color: #fff; background: #7057c9; font: inherit; font-size: 10px; cursor: pointer; }
  .memory-search button:disabled { opacity: .55; cursor: not-allowed; }
  .knowledge-results { display: grid; gap: 9px; margin: 0; padding: 0; list-style: none; }
  .knowledge-results li { display: grid; gap: 7px; padding: 12px 14px; border: 1px solid var(--line, #e9edf2); border-radius: 9px; background: #fff; }
  .knowledge-result-heading { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; color: #344052; font-size: 11px; }
  .knowledge-result-heading code { color: #8792a1; font-size: 9px; }
  .knowledge-results p { margin: 0; color: #596579; font-size: 10px; line-height: 1.5; }
  .knowledge-result-meta { display: flex; flex-wrap: wrap; gap: 6px; }
  .knowledge-result-meta span { padding: 3px 7px; border-radius: 999px; color: #657287; background: #f2f4f7; font-size: 8px; }
  .knowledge-results small { color: #8792a1; font-size: 9px; overflow-wrap: anywhere; }
  .memory-readonly-note { margin: 0; color: #828d9c; font-size: 9px; }
  @media (max-width: 780px) {
    .memory-heading { flex-direction: column; }
    .memory-actions { width: 100%; justify-content: space-between; }
    .memory-search { align-items: stretch; flex-direction: column; }
  }
</style>

<script lang="ts">
  import { clearProjectFilterShortcut, focusProjectFilterShortcut } from '../keyboardShortcuts'

  interface Props {
    sectionTitle: string
    coreOnline: boolean
    search: string
    refreshing: boolean
    onSearch: (value: string) => void
    onRefresh: () => void
  }

  let { sectionTitle, coreOnline, search, refreshing, onSearch, onRefresh }: Props = $props()
  let filterInput: HTMLInputElement | undefined

  function updateSearch(event: Event) {
    if (event.currentTarget instanceof HTMLInputElement) onSearch(event.currentTarget.value)
  }

  function handleKeydown(event: KeyboardEvent) {
    if (clearProjectFilterShortcut(event, filterInput ?? null, () => onSearch(''))) return
    focusProjectFilterShortcut(event, filterInput ?? null)
  }

  function clearSearch() {
    onSearch('')
    filterInput?.focus()
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<header class="topbar">
  <div class="breadcrumbs"><span>Workspace</span><span class="crumb-separator">/</span><strong>{sectionTitle}</strong></div>
  <div class="topbar-actions">
    <label class="search-box">
      <span aria-hidden="true">⌕</span>
      <input bind:this={filterInput} aria-label="Search TimeKeeper" value={search} oninput={updateSearch} placeholder="Search projects, tasks, messages…" />
      <kbd>/</kbd>
      <kbd>Esc</kbd>
    </label>
    {#if search}
      <button class="icon-button filter-clear-button" type="button" aria-label="Clear search" title="Clear search" onclick={clearSearch}>
        <span aria-hidden="true">×</span>
      </button>
    {/if}
    <span class:online={coreOnline} class="connection"><i></i>{coreOnline ? 'Core connected' : 'Core offline'}</span>
    <button class="icon-button" aria-label="Refresh data" onclick={onRefresh} disabled={refreshing}>
      <span class:spinning={refreshing} aria-hidden="true">↻</span>
    </button>
  </div>
</header>

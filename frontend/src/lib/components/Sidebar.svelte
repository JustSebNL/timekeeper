<script lang="ts">
  import { resolveWorkspaceSection, workspaceNavigation, type WorkspaceSection } from '../navigation'

  interface Props {
    activeSection: WorkspaceSection
    onNavigate: (section: WorkspaceSection) => void
  }

  let { activeSection, onNavigate }: Props = $props()
</script>

<aside class="sidebar" aria-label="Main navigation">
  <a class="brand" href="#overview" aria-label="TimeKeeper home">
    <span class="brand-mark">T</span>
    <span><strong>timekeeper</strong><small>WORKSPACE</small></span>
  </a>

  <div class="nav-label">WORKSPACE</div>
  <nav aria-label="Workspace sections">
    {#each workspaceNavigation as item (item.id)}
      {#if item.enabled}
        <button
          type="button"
          class="nav-item nav-button"
          class:active={activeSection === item.id}
          aria-current={activeSection === item.id ? 'page' : undefined}
          data-workspace-section={item.id}
          onclick={() => onNavigate(resolveWorkspaceSection(item.id))}
        >
          <span class="nav-icon" aria-hidden="true">{item.icon}</span>{item.label}
        </button>
      {:else}
        <span class="nav-item upcoming" aria-disabled="true" data-workspace-disabled={item.id}>
          <span class="nav-icon" aria-hidden="true">{item.icon}</span>{item.label}
          {#if item.hint}<small>{item.hint}</small>{/if}
        </span>
      {/if}
    {/each}
  </nav>

  <div class="sidebar-bottom">
    <span class="avatar">TK</span>
    <span><strong>TimeKeeper Core</strong><small>Local workspace</small></span>
    <span class="more" aria-hidden="true">···</span>
  </div>
</aside>

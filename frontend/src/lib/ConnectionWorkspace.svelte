<script lang="ts">
  import type { ConnectionDiagnostic } from './api'

  interface Props {
    api: { testConnection: () => Promise<ConnectionDiagnostic> }
  }

  let { api }: Props = $props()
  let testing = $state(false)
  let error = $state('')
  let diagnostic = $state<ConnectionDiagnostic | null>(null)

  async function testConnection() {
    testing = true
    error = ''
    diagnostic = null
    try {
      diagnostic = await api.testConnection()
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Unable to test the selected TimeKeeper endpoint.'
    } finally {
      testing = false
    }
  }
</script>

<section class="connection-workspace" aria-label="Connection settings">
  <article class="connection-profile-card">
    <div>
      <p class="eyebrow">LOCAL PROFILE</p>
      <h2>TimeKeeper Core</h2>
      <p class="connection-copy">Preferred local endpoint: <code>http://timekeeper.local</code></p>
      <p class="connection-copy">Native Wails uses <code>http://127.0.0.1:1618</code> only when the friendly endpoint is unreachable; browser builds use the browser-facing origin and cannot override native proxy or DNS routing.</p>
      <p class="history-muted">This connection test is read-only. It does not start a Core process or change database state.</p>
    </div>
    <button class="primary-button" type="button" onclick={testConnection} disabled={testing}>
      {testing ? 'Testing…' : 'Test connection'}
    </button>
  </article>

  {#if error}
    <p class="history-error" role="alert">{error}</p>
  {/if}

  {#if diagnostic}
    <section class="connection-result" aria-label="Connection diagnostics">
      <p class="connection-result-status" role="status">
        {diagnostic.used_fallback
          ? 'Friendly endpoint unavailable; connected through loopback fallback.'
          : 'Connected to the selected endpoint.'}
      </p>
      {#if diagnostic.client_surface === 'browser'}
        <p class="connection-surface-note">Browser builds report the browser-facing API origin; native proxy and DNS routing are controlled by the browser and host.</p>
      {/if}
      <dl class="connection-details">
        <div><dt>{diagnostic.client_surface === 'desktop' ? 'Selected Core endpoint' : 'Browser API origin'}</dt><dd>{diagnostic.selected_endpoint}</dd></div>
        <div><dt>Service</dt><dd>{diagnostic.connection_info.service}</dd></div>
        <div><dt>Core version</dt><dd>{diagnostic.connection_info.version}</dd></div>
        <div><dt>API version</dt><dd>{diagnostic.connection_info.api_version}</dd></div>
        <div><dt>Instance ID</dt><dd>{diagnostic.connection_info.instance_id}</dd></div>
      </dl>
      <div class="connection-capabilities">
        <h3>Available capabilities</h3>
        {#if diagnostic.connection_info.capabilities?.length}
          <ul>
            {#each diagnostic.connection_info.capabilities as capability (capability)}
              <li><code>{capability}</code></li>
            {/each}
          </ul>
        {:else}
          <p class="history-muted">Core did not report any capabilities.</p>
        {/if}
      </div>
      {#if diagnostic.connection_info.friendly_urls?.length}
        <p class="connection-friendly-urls">Advertised URLs: {diagnostic.connection_info.friendly_urls.join(', ')}</p>
      {/if}
    </section>
  {/if}
</section>

<style>
  .connection-workspace { display: grid; gap: 14px; margin-top: 3px; }
  .connection-profile-card, .connection-result { display: grid; gap: 14px; padding: 16px; border: 1px solid #e8ecf1; border-radius: 9px; background: #fff; box-shadow: 0 2px 6px #1b28380a; }
  .connection-profile-card { grid-template-columns: minmax(0, 1fr) auto; align-items: center; }
  .connection-profile-card h2 { font-size: 16px; }
  .connection-copy { margin: 7px 0 0; color: #778395; font-size: 10px; line-height: 1.55; }
  .connection-copy code, .connection-details dd, .connection-capabilities code { overflow-wrap: anywhere; font-family: Consolas, monospace; }
  .connection-result-status { margin: 0; color: #2b8a66; font-size: 10px; font-weight: 600; }
  .connection-surface-note { margin: -7px 0 0; color: #929cac; font-size: 9px; line-height: 1.45; }
  .connection-details { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; margin: 0; }
  .connection-details div { min-width: 0; padding: 9px; border: 1px solid #edf0f4; border-radius: 6px; background: #fbfcfe; }
  .connection-details dt { margin-bottom: 5px; color: #929cac; font-size: 8px; font-weight: 700; letter-spacing: .6px; text-transform: uppercase; }
  .connection-details dd { margin: 0; color: #455165; font-size: 10px; }
  .connection-capabilities h3 { margin: 0 0 7px; color: #596579; font-size: 10px; }
  .connection-capabilities ul { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; }
  .connection-capabilities li { padding: 5px 7px; border: 1px solid #e8ecf1; border-radius: 12px; color: #596579; background: #fbfcfe; font-size: 9px; }
  .connection-friendly-urls { margin: 0; color: #778395; font-size: 9px; overflow-wrap: anywhere; }
  @media (max-width: 720px) {
    .connection-profile-card { grid-template-columns: 1fr; }
    .connection-profile-card .primary-button { justify-self: start; }
    .connection-details { grid-template-columns: 1fr; }
  }
</style>

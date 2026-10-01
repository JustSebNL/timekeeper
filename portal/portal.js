// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

'use strict';

(function () {
  const BASE_URL = window.TIMEKEEPER_URL || 'http://127.0.0.1:1618';

  // ── Tab Navigation ──────────────────────────────────────────
  const navLinks = document.querySelectorAll('.nav-link[data-tab]');
  const panels = document.querySelectorAll('.tab-panel');

  function switchTab(tabName) {
    navLinks.forEach(l => l.classList.toggle('active', l.dataset.tab === tabName));
    panels.forEach(p => p.classList.toggle('active', p.id === 'panel-' + tabName));
    if (tabName === 'store') loadPluginStore();
  }

  navLinks.forEach(link => {
    link.addEventListener('click', e => {
      e.preventDefault();
      switchTab(link.dataset.tab);
      window.location.hash = link.dataset.tab;
    });
  });

  // Route from hash on load
  const initialTab = window.location.hash.replace('#', '') || 'docs';
  if (['docs', 'store', 'api'].includes(initialTab)) switchTab(initialTab);

  // ── Core Health Check ───────────────────────────────────────
  const statusDot = document.getElementById('coreStatus');
  const statusLabel = document.getElementById('coreStatusLabel');
  const footerVersion = document.getElementById('footerVersion');

  async function checkCoreHealth() {
    try {
      const res = await fetch(BASE_URL + '/api/v1/version', { signal: AbortSignal.timeout(3000) });
      if (!res.ok) throw new Error('non-200');
      const data = await res.json();
      statusDot.className = 'status-dot online';
      statusLabel.textContent = 'Core Online';
      if (data.display) footerVersion.textContent = data.display;
    } catch {
      statusDot.className = 'status-dot offline';
      statusLabel.textContent = 'Core Offline';
    }
  }

  checkCoreHealth();
  setInterval(checkCoreHealth, 30000);

  // ── Plugin Store ────────────────────────────────────────────
  const storeGrid = document.getElementById('storeGrid');
  const storeLoading = document.getElementById('storeLoading');
  const storeEmpty = document.getElementById('storeEmpty');
  let loadedPlugins = [];

  const PLUGIN_ICONS = {
    knowledge: '🧠',
    observer: '📡',
    automation: '⚙️',
    default: '🔌'
  };

  async function loadPluginStore() {
    if (loadedPlugins.length > 0) return; // Already loaded
    storeLoading.style.display = 'block';
    storeEmpty.style.display = 'none';

    try {
      const res = await fetch(BASE_URL + '/api/v1/plugins', { signal: AbortSignal.timeout(5000) });
      if (!res.ok) throw new Error('store unavailable');
      const data = await res.json();
      loadedPlugins = data.plugins || [];
      renderPlugins(loadedPlugins);
    } catch {
      storeLoading.style.display = 'none';
      storeEmpty.style.display = 'block';
    }
  }

  function renderPlugins(plugins) {
    storeLoading.style.display = 'none';
    // Remove previous plugin cards
    storeGrid.querySelectorAll('.plugin-card').forEach(c => c.remove());

    if (plugins.length === 0) {
      storeEmpty.style.display = 'block';
      return;
    }

    storeEmpty.style.display = 'none';
    plugins.forEach(plugin => {
      const card = document.createElement('div');
      card.className = 'plugin-card';
      card.dataset.runtime = plugin.runtime || '';
      card.dataset.id = plugin.id || '';

      const icon = PLUGIN_ICONS[plugin.runtime] || PLUGIN_ICONS.default;
      const caps = (plugin.capabilities || []).map(c =>
        '<span class="plugin-tag">' + escapeHtml(c) + '</span>'
      ).join('');

      card.innerHTML = [
        '<div class="plugin-card-header">',
        '  <div class="plugin-icon">' + icon + '</div>',
        '  <div class="plugin-meta">',
        '    <h3>' + escapeHtml(plugin.name || plugin.id) + '</h3>',
        '    <span class="plugin-version">v' + escapeHtml(plugin.version) + ' · ' + escapeHtml(plugin.runtime) + '</span>',
        '  </div>',
        '</div>',
        '<p class="plugin-description">' + escapeHtml(plugin.description || 'No description provided.') + '</p>',
        '<div class="plugin-tags">' + caps + '</div>'
      ].join('\n');

      storeGrid.appendChild(card);
    });
  }

  // ── Store Search ────────────────────────────────────────────
  const storeSearchInput = document.getElementById('storeSearch');
  if (storeSearchInput) {
    storeSearchInput.addEventListener('input', () => {
      const query = storeSearchInput.value.toLowerCase().trim();
      const filtered = loadedPlugins.filter(p =>
        (p.name || p.id || '').toLowerCase().includes(query) ||
        (p.description || '').toLowerCase().includes(query)
      );
      renderPlugins(filtered);
    });
  }

  // ── Store Filters ───────────────────────────────────────────
  const filterBtns = document.querySelectorAll('.filter-btn[data-filter]');
  filterBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      filterBtns.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      const filter = btn.dataset.filter;
      if (filter === 'all') {
        renderPlugins(loadedPlugins);
      } else {
        renderPlugins(loadedPlugins.filter(p => (p.runtime || '').includes(filter)));
      }
    });
  });

  // ── Documentation Search ────────────────────────────────────
  const docSearch = document.getElementById('docSearch');
  const docLinks = document.querySelectorAll('.doc-link');

  if (docSearch) {
    docSearch.addEventListener('input', () => {
      const query = docSearch.value.toLowerCase().trim();
      docLinks.forEach(link => {
        const text = link.textContent.toLowerCase();
        link.style.display = text.includes(query) || query === '' ? '' : 'none';
      });
    });
  }

  // ── Doc Link Click ──────────────────────────────────────────
  docLinks.forEach(link => {
    link.addEventListener('click', e => {
      e.preventDefault();
      docLinks.forEach(l => l.classList.remove('active'));
      link.classList.add('active');
    });
  });

  // ── API Link Click ──────────────────────────────────────────
  const apiLinks = document.querySelectorAll('.api-link');
  apiLinks.forEach(link => {
    link.addEventListener('click', e => {
      e.preventDefault();
      apiLinks.forEach(l => l.classList.remove('active'));
      link.classList.add('active');
    });
  });

  // ── Utility ─────────────────────────────────────────────────
  function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }
})();

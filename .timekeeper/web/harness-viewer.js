// TimeKeeper Harness Viewer — sidebar menu + textarea
// Builds menu from /api/harness/catalog, loads content via /api/harness/content?id=<id>
(() => {
  const menu = document.getElementById('harness-menu');
  const countEl = document.getElementById('harness-count');
  const titleEl = document.getElementById('harness-title');
  const pathEl = document.getElementById('harness-path');
  const sourceEl = document.getElementById('harness-source');
  const copyBtn = document.getElementById('harness-copy');

  let currentContent = '';
  let items = [];

  async function loadCatalog() {
    try {
      const resp = await fetch('/api/harness/catalog', { headers: { 'Accept': 'application/json' } });
      if (!resp.ok) throw new Error('catalog ' + resp.status);
      items = await resp.json();
      renderMenu();
      countEl.textContent = items.length + ' documents';
      const selectedId = document.body.dataset.selectedId;
      if (selectedId) {
        const selected = items.find(item => (item.id || item.ID) === selectedId);
        if (selected) selectItem(selected);
      }
    } catch (e) {
      countEl.textContent = 'Failed to load catalog';
      console.error(e);
    }
  }

  function renderMenu() {
    menu.innerHTML = '';
    items.forEach((item, idx) => {
      const li = document.createElement('li');
      li.className = 'harness-menu-item';
      li.setAttribute('role', 'option');
      li.innerHTML = '<span class="harness-menu-id">' + escapeHtml(item.id || item.ID) + '</span>' +
        '<span class="harness-menu-desc">' + escapeHtml(item.description || item.Description) + '</span>';
      const itemId = item.id || item.ID;
      li.setAttribute('data-id', itemId);
      li.tabIndex = 0;
      li.addEventListener('click', () => selectItem(item));
      li.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectItem(item); }
      });
      menu.appendChild(li);
    });
  }

  function escapeHtml(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  async function selectItem(item) {
    const id = item.id || item.ID;
    const path = item.path || item.Path;
    // Update active state
    menu.querySelectorAll('.harness-menu-item').forEach(el => el.classList.remove('active'));
    const activeEl = menu.querySelector('[data-id="' + escapeSelector(id) + '"]');
    if (activeEl) activeEl.classList.add('active');

    // Update header
    titleEl.textContent = id;
    pathEl.textContent = path;
    history.replaceState(null, '', '/harness.html?item=' + encodeURIComponent(id));

    // Load content
    sourceEl.value = 'Loading…';
    copyBtn.disabled = true;
    try {
      const resp = await fetch('/api/harness/content?id=' + encodeURIComponent(id), {
        headers: { 'Accept': 'text/plain' }
      });
      if (!resp.ok) throw new Error('content ' + resp.status);
      currentContent = await resp.text();
      sourceEl.value = currentContent;
      copyBtn.disabled = false;
    } catch (e) {
      sourceEl.value = 'Failed to load: ' + e.message;
      console.error(e);
    }
  }

  function escapeSelector(s) {
    return s.replace(/([!"#$%&'()*+,./:;<=>?@[\]^`{|}~])/g, '\\$1');
  }

  copyBtn.addEventListener('click', async () => {
    if (!currentContent) return;
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(currentContent);
      } else {
        // Fallback for non-secure origins (http://timekeeper.local)
        const ta = document.createElement('textarea');
        ta.value = currentContent;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
      }
      copyBtn.textContent = 'Copied!';
      setTimeout(() => copyBtn.textContent = 'Copy file', 1500);
    } catch (e) {
      copyBtn.textContent = 'Failed';
      setTimeout(() => copyBtn.textContent = 'Copy file', 1500);
      console.error(e);
    }
  });

  // Initialize
  loadCatalog();
})();
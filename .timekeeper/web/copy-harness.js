// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

(() => {
  'use strict';

  const textarea = document.querySelector('#harness-source');
  const button = document.querySelector('#harness-copy');
  if (!textarea || !button) return;

  const LABEL_IDLE = 'Copy file';
  const LABEL_COPIED = 'Copied ✓';
  let resetTimer = null;

  function fallbackCopy(value) {
    // document.execCommand is deprecated but remains the only way to set the
    // clipboard from a non-secure origin; timekeeper.local is served over
    // plain http, so navigator.clipboard may be unavailable there.
    textarea.focus();
    textarea.select();
    textarea.setSelectionRange(0, value.length);
    let ok = false;
    try {
      ok = document.execCommand('copy');
    } catch (error) {
      ok = false;
    }
    window.getSelection().removeAllRanges();
    return ok;
  }

  async function copySource() {
    const value = textarea.value;
    let copied = false;
    if (navigator.clipboard && window.isSecureContext) {
      try {
        await navigator.clipboard.writeText(value);
        copied = true;
      } catch (error) {
        copied = fallbackCopy(value);
      }
    } else {
      copied = fallbackCopy(value);
    }
    button.textContent = copied ? LABEL_COPIED : 'Press Ctrl+C';
    button.disabled = false;
    if (resetTimer) clearTimeout(resetTimer);
    resetTimer = window.setTimeout(() => { button.textContent = LABEL_IDLE; }, 1600);
  }

  button.addEventListener('click', () => { void copySource(); });
})();

(function () {
  'use strict';

  function getStoredTheme() {
    return localStorage.getItem('dirio-theme') || 'system';
  }

  function applyTheme(t) {
    document.documentElement.setAttribute('data-theme', t);
    localStorage.setItem('dirio-theme', t);
    updateButtons(t);
  }

  function updateButtons(t) {
    document.querySelectorAll('[data-theme-btn]').forEach(function (btn) {
      btn.setAttribute('aria-pressed', btn.getAttribute('data-theme-btn') === t ? 'true' : 'false');
    });
  }

  document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('[data-theme-btn]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        applyTheme(btn.getAttribute('data-theme-btn'));
      });
    });
    updateButtons(getStoredTheme());
  });
}());
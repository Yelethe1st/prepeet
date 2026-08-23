/* Prepeet · theme.js
   Theme + motion + density preferences. Runs synchronously in <head> to avoid flashes.
   - Pages declare a default via <html data-theme-default="dark|light">.
   - Users' explicit choices persist in localStorage under "prepeet.theme".
   - [data-theme-toggle] buttons (rendered anywhere) switch themes.
*/
(function () {
  var KEY = 'prepeet.theme';
  var MOTION_KEY = 'prepeet.motion';
  var root = document.documentElement;
  var stored = null;
  // ?theme=dark|light lets any screen be linked in a specific theme for review; it persists
  // like an explicit user choice so the rest of the walkthrough stays in that theme.
  try {
    var q = new URLSearchParams(location.search).get('theme');
    if (q === 'light' || q === 'dark') { localStorage.setItem(KEY, q); }
  } catch (e) {}
  try { stored = localStorage.getItem(KEY); } catch (e) {}
  var fallback = root.getAttribute('data-theme-default') || 'light';
  var theme = (stored === 'light' || stored === 'dark') ? stored : fallback;
  root.setAttribute('data-theme', theme);
  try { if (localStorage.getItem(MOTION_KEY) === 'reduced') root.setAttribute('data-motion', 'reduced'); } catch (e) {}

  function applyToggles() {
    var current = root.getAttribute('data-theme');
    document.querySelectorAll('[data-theme-toggle]').forEach(function (btn) {
      var isDark = current === 'dark';
      btn.setAttribute('aria-pressed', isDark ? 'true' : 'false');
      btn.setAttribute('aria-label', isDark ? 'Switch to light theme' : 'Switch to dark theme');
      btn.setAttribute('title', isDark ? 'Switch to light theme' : 'Switch to dark theme');
      var sun = btn.querySelector('[data-icon-sun]'), moon = btn.querySelector('[data-icon-moon]');
      if (sun) sun.hidden = !isDark;
      if (moon) moon.hidden = isDark;
      var lbl = btn.querySelector('[data-theme-label]');
      if (lbl) lbl.textContent = isDark ? 'Light theme' : 'Dark theme';
    });
    document.querySelectorAll('[data-theme-radio]').forEach(function (r) { r.checked = r.value === current; });
  }

  window.PrepeetTheme = {
    get: function () { return root.getAttribute('data-theme'); },
    set: function (t) {
      root.setAttribute('data-theme', t);
      try { localStorage.setItem(KEY, t); } catch (e) {}
      applyToggles();
      document.dispatchEvent(new CustomEvent('prepeet:theme', { detail: { theme: t } }));
    },
    toggle: function () { this.set(this.get() === 'dark' ? 'light' : 'dark'); },
    setMotion: function (m) {
      if (m === 'reduced') root.setAttribute('data-motion', 'reduced'); else root.removeAttribute('data-motion');
      try { localStorage.setItem(MOTION_KEY, m); } catch (e) {}
    },
    refresh: applyToggles
  };

  document.addEventListener('click', function (e) {
    var btn = e.target.closest('[data-theme-toggle]');
    if (btn) { e.preventDefault(); window.PrepeetTheme.toggle(); }
  });
  document.addEventListener('change', function (e) {
    if (e.target.matches('[data-theme-radio]')) window.PrepeetTheme.set(e.target.value);
  });
  document.addEventListener('DOMContentLoaded', applyToggles);
})();

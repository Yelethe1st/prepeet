/* Prepeet · ui.js
   Shared component behaviours. Declarative via data attributes so pages need no custom JS:
   - Tabs:       <div data-tabs> <button role="tab" aria-controls="id"> ... <div role="tabpanel" id="id">
   - Accordion:  <button class="accordion-trigger" aria-expanded aria-controls="id">
   - Dialog:     <button data-dialog-open="id">  <dialog id="id" class="dialog">  [data-dialog-close]
   - Drawer:     <button data-drawer-open="id">  <dialog id="id" class="drawer">
   - Menu:       <button data-menu="id"> <div id="id" class="menu" hidden>
   - Tooltip:    any element with data-tooltip="text"
   - Toast:      Prepeet.toast({title, desc, kind})  or <button data-toast="Saved" data-toast-kind="success">
   - Toggle:     <button data-toggle-target="id"> toggles hidden; data-toggle-class="x" toggles class on target
   - Pressed:    <button data-pressable> toggles aria-pressed
   - Segmented:  <div data-segmented> buttons manage aria-pressed; emits change
   - Copy:       <button data-copy="text">
   - Sim button: <button data-loading-sim="1200"> shows loading then restores
   - Score ring: <div class="ring" data-score="74" data-band="auto">
   - Query:      Prepeet.query.get('mode')  / Prepeet.query.set({mode:'screen'})
*/
(function () {
  var P = window.Prepeet = window.Prepeet || {};

  /* ---------- Query-string helpers ---------- */
  P.query = {
    get: function (k, def) { var v = new URLSearchParams(location.search).get(k); return v === null ? (def === undefined ? null : def) : v; },
    all: function () { var o = {}; new URLSearchParams(location.search).forEach(function (v, k) { o[k] = v; }); return o; },
    set: function (obj, replace) {
      var sp = new URLSearchParams(location.search);
      Object.keys(obj).forEach(function (k) { if (obj[k] === null || obj[k] === undefined || obj[k] === '') sp.delete(k); else sp.set(k, obj[k]); });
      var url = location.pathname + (sp.toString() ? '?' + sp.toString() : '') + location.hash;
      // Some browsers throw SecurityError on history.pushState/replaceState for file:// URLs.
      // URL state is a convenience here, never a correctness requirement — never let it break a render.
      try {
        if (replace) history.replaceState(null, '', url); else history.pushState(null, '', url);
      } catch (e) { /* file:// preview — carry on without updating the address bar */ }
    },
    withParams: function (href, obj) {
      var u = href.split('#'); var parts = u[0].split('?');
      var sp = new URLSearchParams(parts[1] || '');
      Object.keys(obj).forEach(function (k) { if (obj[k] == null) sp.delete(k); else sp.set(k, obj[k]); });
      return parts[0] + (sp.toString() ? '?' + sp.toString() : '') + (u[1] ? '#' + u[1] : '');
    }
  };

  /* ---------- Icons ---------- */
  P.icons = function (scope) { if (window.lucide && window.lucide.createIcons) { try { window.lucide.createIcons({ attrs: { 'aria-hidden': 'true' } }); } catch (e) { window.lucide.createIcons(); } } };

  /* ---------- Live region ---------- */
  function liveRegion() {
    var el = document.getElementById('pp-live');
    if (!el) { el = document.createElement('div'); el.id = 'pp-live'; el.className = 'sr-only'; el.setAttribute('aria-live', 'polite'); el.setAttribute('aria-atomic', 'true'); document.body.appendChild(el); }
    return el;
  }
  P.announce = function (msg) { var el = liveRegion(); el.textContent = ''; setTimeout(function () { el.textContent = msg; }, 30); };

  /* ---------- Toasts ---------- */
  var ICONS = { success: 'check-circle-2', warning: 'alert-triangle', danger: 'x-octagon', info: 'info', default: 'bell' };
  P.toast = function (opts) {
    opts = typeof opts === 'string' ? { title: opts } : (opts || {});
    var region = document.querySelector('.toast-region');
    if (!region) { region = document.createElement('div'); region.className = 'toast-region'; region.setAttribute('role', 'status'); region.setAttribute('aria-live', 'polite'); document.body.appendChild(region); }
    var kind = opts.kind || 'default';
    var t = document.createElement('div');
    t.className = 'toast ' + (kind !== 'default' ? kind : '');
    t.innerHTML = '<i data-lucide="' + (ICONS[kind] || ICONS.default) + '" class="ic"></i><div><div class="toast-title"></div>' + (opts.desc ? '<div class="toast-desc"></div>' : '') + '</div><button class="icon-btn icon-btn-sm" aria-label="Dismiss notification"><i data-lucide="x" class="ic-sm ic"></i></button>';
    t.querySelector('.toast-title').textContent = opts.title || '';
    if (opts.desc) t.querySelector('.toast-desc').textContent = opts.desc;
    region.appendChild(t);
    P.icons();
    var remove = function () { if (t.parentNode) { t.style.opacity = '0'; t.style.transition = 'opacity .2s'; setTimeout(function () { t.remove(); }, 200); } };
    t.querySelector('button').addEventListener('click', remove);
    setTimeout(remove, opts.duration || 5000);
    return t;
  };

  /* ---------- Dialogs / drawers ---------- */
  var lastFocus = null;
  P.openDialog = function (id) {
    var d = typeof id === 'string' ? document.getElementById(id) : id;
    if (!d) return;
    lastFocus = document.activeElement;
    if (typeof d.showModal === 'function') { if (!d.open) d.showModal(); } else { d.setAttribute('open', ''); }
    P.icons();
    var first = d.querySelector('[autofocus], input:not([type=hidden]), button:not([data-dialog-close]):not([data-drawer-close]), [href], select, textarea');
    var closeBtn = d.querySelector('[data-dialog-close], [data-drawer-close]');
    (first || closeBtn || d).focus();
    document.dispatchEvent(new CustomEvent('prepeet:dialog-open', { detail: { id: d.id } }));
  };
  P.closeDialog = function (id) {
    var d = typeof id === 'string' ? document.getElementById(id) : id;
    if (!d) return;
    if (typeof d.close === 'function' && d.open) d.close(); else d.removeAttribute('open');
    if (lastFocus && lastFocus.focus) lastFocus.focus();
  };

  /* ---------- Tabs ---------- */
  function initTabs(root) {
    var tabs = Array.prototype.slice.call(root.querySelectorAll('[role="tab"]'));
    if (!tabs.length) return;
    var param = root.getAttribute('data-tabs-param');
    function activate(tab, focus, silent) {
      tabs.forEach(function (t) {
        var on = t === tab;
        t.setAttribute('aria-selected', on ? 'true' : 'false');
        t.setAttribute('tabindex', on ? '0' : '-1');
        var panel = document.getElementById(t.getAttribute('aria-controls'));
        if (panel) panel.hidden = !on;
      });
      if (focus) tab.focus();
      if (param && !silent) P.query.set((function(){ var o={}; o[param]=tab.getAttribute('data-tab')||tab.getAttribute('aria-controls'); return o;})(), true);
      root.dispatchEvent(new CustomEvent('prepeet:tab', { detail: { id: tab.getAttribute('aria-controls') }, bubbles: true }));
    }
    tabs.forEach(function (t, i) {
      t.addEventListener('click', function (e) { e.preventDefault(); activate(t); });
      t.addEventListener('keydown', function (e) {
        var idx = tabs.indexOf(t), n = null;
        if (e.key === 'ArrowRight' || e.key === 'ArrowDown') n = tabs[(idx + 1) % tabs.length];
        if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') n = tabs[(idx - 1 + tabs.length) % tabs.length];
        if (e.key === 'Home') n = tabs[0]; if (e.key === 'End') n = tabs[tabs.length - 1];
        if (n) { e.preventDefault(); activate(n, true); }
      });
    });
    var initial = null;
    if (param) { var want = P.query.get(param); if (want) initial = tabs.filter(function (t) { return (t.getAttribute('data-tab') || t.getAttribute('aria-controls')) === want; })[0]; }
    if (!initial && location.hash) initial = tabs.filter(function (t) { return '#' + t.getAttribute('aria-controls') === location.hash; })[0];
    if (!initial) initial = tabs.filter(function (t) { return t.getAttribute('aria-selected') === 'true'; })[0] || tabs[0];
    activate(initial, false, true);
  }
  P.activateTab = function (panelId) {
    var tab = document.querySelector('[role="tab"][aria-controls="' + panelId + '"]');
    if (tab) tab.click();
  };

  /* ---------- Accordion ---------- */
  function initAccordionTrigger(btn) {
    var panel = document.getElementById(btn.getAttribute('aria-controls'));
    if (!panel) return;
    var open = btn.getAttribute('aria-expanded') === 'true';
    panel.hidden = !open;
    btn.addEventListener('click', function () {
      var now = btn.getAttribute('aria-expanded') === 'true';
      var group = btn.closest('[data-accordion-single]');
      if (group && !now) {
        group.querySelectorAll('.accordion-trigger[aria-expanded="true"]').forEach(function (o) { o.setAttribute('aria-expanded', 'false'); var p = document.getElementById(o.getAttribute('aria-controls')); if (p) p.hidden = true; });
      }
      btn.setAttribute('aria-expanded', now ? 'false' : 'true');
      panel.hidden = now;
    });
  }

  /* ---------- Tooltips ---------- */
  var tipEl = null, tipTimer = null;
  function showTip(target) {
    var text = target.getAttribute('data-tooltip'); if (!text) return;
    if (!tipEl) { tipEl = document.createElement('div'); tipEl.className = 'tooltip'; tipEl.setAttribute('role', 'tooltip'); tipEl.id = 'pp-tooltip'; document.body.appendChild(tipEl); }
    tipEl.textContent = text;
    target.setAttribute('aria-describedby', 'pp-tooltip');
    var r = target.getBoundingClientRect();
    tipEl.style.left = '0px'; tipEl.style.top = '0px'; tipEl.classList.add('is-visible');
    var tw = tipEl.offsetWidth, th = tipEl.offsetHeight;
    var left = Math.max(8, Math.min(window.innerWidth - tw - 8, r.left + r.width / 2 - tw / 2));
    var top = r.top - th - 8; if (top < 8) top = r.bottom + 8;
    tipEl.style.left = left + 'px'; tipEl.style.top = top + 'px';
  }
  function hideTip() { if (tipEl) tipEl.classList.remove('is-visible'); document.querySelectorAll('[aria-describedby="pp-tooltip"]').forEach(function (t) { t.removeAttribute('aria-describedby'); }); }
  document.addEventListener('mouseover', function (e) { var t = e.target.closest('[data-tooltip]'); if (t) { clearTimeout(tipTimer); tipTimer = setTimeout(function () { showTip(t); }, 250); } });
  document.addEventListener('mouseout', function (e) { var t = e.target.closest('[data-tooltip]'); if (t) { clearTimeout(tipTimer); hideTip(); } });
  document.addEventListener('focusin', function (e) { var t = e.target.closest('[data-tooltip]'); if (t) showTip(t); });
  document.addEventListener('focusout', function (e) { var t = e.target.closest('[data-tooltip]'); if (t) hideTip(); });
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape') hideTip(); });
  window.addEventListener('scroll', hideTip, true);

  /* ---------- Menus ---------- */
  function closeMenus(except) {
    document.querySelectorAll('.menu:not([hidden])').forEach(function (m) { if (m !== except) { m.hidden = true; var b = document.querySelector('[data-menu="' + m.id + '"]'); if (b) b.setAttribute('aria-expanded', 'false'); } });
  }
  document.addEventListener('click', function (e) {
    var btn = e.target.closest('[data-menu]');
    if (btn) {
      e.stopPropagation();
      var m = document.getElementById(btn.getAttribute('data-menu')); if (!m) return;
      var willOpen = m.hidden; closeMenus(m); m.hidden = !willOpen; btn.setAttribute('aria-expanded', willOpen ? 'true' : 'false');
      if (willOpen) { var f = m.querySelector('.menu-item'); if (f) f.focus(); }
      return;
    }
    if (!e.target.closest('.menu')) closeMenus();
  });
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape') { closeMenus(); } });

  /* ---------- Score ring ---------- */
  P.band = function (score) { if (score == null || isNaN(score)) return 'insufficient'; if (score >= 80) return 'strong'; if (score >= 65) return 'solid'; if (score >= 50) return 'developing'; return 'concern'; };
  P.bandLabel = function (b) { return { strong: 'Strong', solid: 'Solid', developing: 'Developing', concern: 'Needs work', insufficient: 'Insufficient evidence' }[b] || b; };
  function initRing(el) {
    var score = el.getAttribute('data-score'); var val = score === '' || score == null ? null : Number(score);
    var band = el.getAttribute('data-band'); if (!band || band === 'auto') band = P.band(val);
    el.classList.add('band-' + band);
    el.style.setProperty('--value', val == null ? 0 : Math.max(0, Math.min(100, val)));
    if (!el.querySelector('svg')) {
      var sub = el.getAttribute('data-sub') || '';
      el.innerHTML = '<svg viewBox="0 0 36 36" aria-hidden="true"><circle class="track" cx="18" cy="18" r="15.915"/><circle class="fill" cx="18" cy="18" r="15.915"/></svg><div class="ring-label"><span class="ring-value">' + (val == null ? '—' : val) + '</span>' + (sub ? '<span class="ring-sub">' + sub + '</span>' : '') + '</div>';
    }
    if (!el.getAttribute('role')) { el.setAttribute('role', 'img'); el.setAttribute('aria-label', (el.getAttribute('data-label') || 'Score') + ': ' + (val == null ? 'insufficient evidence' : val + ' out of 100, ' + P.bandLabel(band))); }
  }

  /* ---------- Waveform helper ---------- */
  P.waveform = function (el, n) { n = n || Number(el.getAttribute('data-bars') || 24); if (el.children.length) return; for (var i = 0; i < n; i++) { var s = document.createElement('span'); s.style.animationDelay = (i * 0.06 % 0.6) + 's'; el.appendChild(s); } };

  /* ---------- Mini wave for audio player ---------- */
  P.miniWave = function (el, n) { n = n || 40; if (el.children.length) return; var seed = 7; for (var i = 0; i < n; i++) { seed = (seed * 9301 + 49297) % 233280; var h = 6 + Math.round((seed / 233280) * 20); var s = document.createElement('span'); s.style.height = h + 'px'; el.appendChild(s); } };

  /* ---------- Copy ---------- */
  function copyText(text) { if (navigator.clipboard) return navigator.clipboard.writeText(text); var ta = document.createElement('textarea'); ta.value = text; document.body.appendChild(ta); ta.select(); try { document.execCommand('copy'); } catch (e) {} ta.remove(); return Promise.resolve(); }

  /* ---------- Global click delegation ---------- */
  document.addEventListener('click', function (e) {
    var t;
    if ((t = e.target.closest('[data-dialog-open]'))) { e.preventDefault(); P.openDialog(t.getAttribute('data-dialog-open')); return; }
    if ((t = e.target.closest('[data-drawer-open]'))) { e.preventDefault(); P.openDialog(t.getAttribute('data-drawer-open')); return; }
    if ((t = e.target.closest('[data-dialog-close], [data-drawer-close]'))) { e.preventDefault(); var d = t.closest('dialog'); if (d) P.closeDialog(d); return; }
    if ((t = e.target.closest('[data-toast]'))) { P.toast({ title: t.getAttribute('data-toast'), desc: t.getAttribute('data-toast-desc') || '', kind: t.getAttribute('data-toast-kind') || 'default' }); }
    if ((t = e.target.closest('[data-toggle-target]'))) {
      var tg = document.getElementById(t.getAttribute('data-toggle-target'));
      if (tg) { var cls = t.getAttribute('data-toggle-class'); if (cls) tg.classList.toggle(cls); else tg.hidden = !tg.hidden; var exp = t.getAttribute('aria-expanded'); if (exp !== null) t.setAttribute('aria-expanded', exp === 'true' ? 'false' : 'true'); }
    }
    if ((t = e.target.closest('[data-pressable]'))) { t.setAttribute('aria-pressed', t.getAttribute('aria-pressed') === 'true' ? 'false' : 'true'); }
    if ((t = e.target.closest('[data-segmented] > button'))) { var seg = t.parentNode; seg.querySelectorAll('button').forEach(function (b) { b.setAttribute('aria-pressed', b === t ? 'true' : 'false'); }); seg.dispatchEvent(new CustomEvent('change', { bubbles: true, detail: { value: t.getAttribute('data-value') } })); var qp = seg.getAttribute('data-segmented-param'); if (qp) { var o = {}; o[qp] = t.getAttribute('data-value'); P.query.set(o, true); } }
    if ((t = e.target.closest('[data-copy]'))) { copyText(t.getAttribute('data-copy')).then(function () { P.toast({ title: 'Copied to clipboard', kind: 'success', duration: 2500 }); }); }
    if ((t = e.target.closest('[data-loading-sim]'))) {
      if (t.classList.contains('is-loading')) return;
      var ms = Number(t.getAttribute('data-loading-sim')) || 1000; t.classList.add('is-loading'); t.setAttribute('aria-busy', 'true');
      setTimeout(function () { t.classList.remove('is-loading'); t.removeAttribute('aria-busy'); var done = t.getAttribute('data-loading-done'); if (done) P.toast({ title: done, kind: t.getAttribute('data-loading-kind') || 'success' }); var href = t.getAttribute('data-loading-href'); if (href) location.href = href; }, ms);
    }
  });

  /* Dialog: click on backdrop closes */
  document.addEventListener('click', function (e) { if (e.target.tagName === 'DIALOG' && e.target.open) { P.closeDialog(e.target); } });
  document.addEventListener('close', function (e) { if (e.target.tagName === 'DIALOG' && lastFocus && lastFocus.focus) lastFocus.focus(); }, true);

  /* ---------- Form validation helper ---------- */
  P.validate = function (form) {
    var ok = true;
    form.querySelectorAll('[required], [pattern], [type=email]').forEach(function (f) {
      var wrap = f.closest('.field'); var err = wrap ? wrap.querySelector('.error-text') : null;
      var valid = f.checkValidity();
      f.setAttribute('aria-invalid', valid ? 'false' : 'true');
      if (err) { err.hidden = valid; }
      if (!valid) ok = false;
    });
    if (!ok) { var first = form.querySelector('[aria-invalid="true"]'); if (first) first.focus(); P.announce('Some fields need attention.'); }
    return ok;
  };

  /* ---------- Init ---------- */
  P.init = function (scope) {
    scope = scope || document;
    scope.querySelectorAll('[data-tabs]').forEach(function (r) { if (!r.__tabs) { r.__tabs = true; initTabs(r); } });
    scope.querySelectorAll('.accordion-trigger[aria-controls]').forEach(function (b) { if (!b.__acc) { b.__acc = true; initAccordionTrigger(b); } });
    scope.querySelectorAll('.ring[data-score]').forEach(function (r) { if (!r.__ring) { r.__ring = true; initRing(r); } });
    scope.querySelectorAll('.waveform').forEach(function (w) { P.waveform(w); });
    scope.querySelectorAll('.wave-mini').forEach(function (w) { P.miniWave(w); });
    scope.querySelectorAll('form[data-validate]').forEach(function (f) { if (!f.__val) { f.__val = true; f.setAttribute('novalidate', ''); f.addEventListener('submit', function (e) { if (!P.validate(f)) { e.preventDefault(); return; } var sim = f.getAttribute('data-validate'); if (sim && sim !== 'true' && sim !== '') { e.preventDefault(); var btn = f.querySelector('[type=submit]'); if (btn) { btn.classList.add('is-loading'); } setTimeout(function () { location.href = sim; }, Number(f.getAttribute('data-validate-delay') || 900)); } }); } });
    P.icons();
  };
  document.addEventListener('DOMContentLoaded', function () { P.init(); });
})();

/* Prepeet · shell.js
   Renders the application chrome (sidebar, topbar, mobile nav) from one
   permission-aware navigation config, so every screen shares one implementation.

   Usage in a page:
     <body class="app" data-shell="candidate" data-nav="dashboard"
           data-title="Dashboard" data-crumbs="Candidate">
       <div data-shell-sidebar></div>
       <main class="app-main">
         <div data-shell-topbar></div>
         <div class="app-content"> ...page... </div>
       </main>

   Roles (prototype demo switcher, persisted in localStorage "prepeet.role"):
     recruiter · tenant_admin · platform_admin · god
   Mode (candidate mock data, "prepeet.mode" or ?mode=): practice · screen
*/
(function () {
  var P = window.Prepeet = window.Prepeet || {};

  /* ------------------------------------------------------------------ role */
  var ROLE_KEY = 'prepeet.role';
  var ROLES = {
    recruiter:      { label: 'Recruiter',          rank: 1, desc: 'Reviews candidates in one tenant' },
    tenant_admin:   { label: 'Tenant admin',       rank: 2, desc: 'Configures the tenant workspace' },
    platform_admin: { label: 'Platform admin',     rank: 3, desc: 'Operates Prepeet across tenants' },
    god:            { label: 'Super administrator',rank: 4, desc: 'Restricted break-glass access' }
  };
  P.roles = ROLES;
  P.getRole = function () {
    var q = new URLSearchParams(location.search).get('role');
    if (q && ROLES[q]) { try { localStorage.setItem(ROLE_KEY, q); } catch (e) {} return q; }
    var r; try { r = localStorage.getItem(ROLE_KEY); } catch (e) {}
    return ROLES[r] ? r : 'tenant_admin';
  };
  P.setRole = function (r) { try { localStorage.setItem(ROLE_KEY, r); } catch (e) {} location.reload(); };
  P.can = function (minRole) { return ROLES[P.getRole()].rank >= ROLES[minRole].rank; };

  /* ------------------------------------------------------------------ mode */
  var MODE_KEY = 'prepeet.mode';
  P.getMode = function () {
    var q = new URLSearchParams(location.search).get('mode');
    if (q === 'practice' || q === 'screen') { try { localStorage.setItem(MODE_KEY, q); } catch (e) {} return q; }
    var m; try { m = localStorage.getItem(MODE_KEY); } catch (e) {}
    return m === 'screen' ? 'screen' : 'practice';
  };
  P.setMode = function (m) {
    try { localStorage.setItem(MODE_KEY, m); } catch (e) {}
    var sp = new URLSearchParams(location.search); sp.set('mode', m);
    location.search = sp.toString();
  };

  /* ------------------------------------------------------------------ logo */
  P.logoSvg = '<svg viewBox="0 0 24 24" fill="none" aria-hidden="true">' +
    '<rect x="3" y="9.5" width="2.6" height="5" rx="1.3" fill="currentColor"/>' +
    '<rect x="8" y="5" width="2.6" height="14" rx="1.3" fill="currentColor"/>' +
    '<rect x="13" y="7.5" width="2.6" height="9" rx="1.3" fill="currentColor"/>' +
    '<rect x="18" y="10.5" width="2.6" height="3" rx="1.3" fill="currentColor"/></svg>';

  P.favicon = function () {
    if (document.querySelector('link[rel="icon"][data-pp]')) return;   // idempotent
    var svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="6" fill="%23177068"/><g fill="white"><rect x="4" y="10" width="2.4" height="4" rx="1.2"/><rect x="8.4" y="6" width="2.4" height="12" rx="1.2"/><rect x="12.8" y="8.2" width="2.4" height="7.6" rx="1.2"/><rect x="17.2" y="10.8" width="2.4" height="2.4" rx="1.2"/></g></svg>';
    var l = document.createElement('link'); l.rel = 'icon'; l.setAttribute('data-pp',''); l.href = 'data:image/svg+xml,' + svg;
    document.head.appendChild(l);
  };

  /* ------------------------------------------------------------- nav config */
  function navConfig(shell) {
    if (shell === 'candidate') {
      /* A screening candidate is inside one invitation, not inside a practice
         account. Practice surfaces are not theirs to see, so they are not
         rendered — the pages guard themselves as well; this is not the lock. */
      if (P.getMode() === 'screen') {
        return [
          { label: 'Your screening', items: [
            { id: 'dashboard', label: 'Interview status', icon: 'file-check', href: 'candidate-dashboard.html' },
            { id: 'invitation', label: 'Invitation & consent', icon: 'mail-check', href: 'invitation-accept.html' }
          ]},
          { label: 'Account', items: [
            { id: 'settings', label: 'Privacy & settings', icon: 'settings', href: 'candidate-settings.html' }
          ]}
        ];
      }
      return [
        { label: 'Prepare', items: [
          { id: 'dashboard', label: 'Dashboard', icon: 'layout-dashboard', href: 'candidate-dashboard.html' },
          { id: 'start', label: 'Start an interview', icon: 'circle-play', href: 'candidate-start-interview.html' }
        ]},
        { label: 'Your practice', items: [
          { id: 'sessions', label: 'Sessions', icon: 'history', href: 'candidate-sessions.html', badge: '12' },
          { id: 'skills', label: 'Skills', icon: 'target', href: 'candidate-skills.html' },
          { id: 'progression', label: 'Progression', icon: 'trending-up', href: 'candidate-progression.html' },
          { id: 'goals', label: 'Goals', icon: 'flag', href: 'candidate-goals.html' }
        ]},
        { label: 'Account', items: [
          { id: 'profile', label: 'Profile & résumé', icon: 'user-round', href: 'candidate-profile.html' },
          { id: 'settings', label: 'Settings', icon: 'settings', href: 'candidate-settings.html' }
        ]}
      ];
    }
    // admin shell — each group/item declares the minimum role required
    return [
      { label: 'Hiring', role: 'recruiter', items: [
        { id: 'admin-dashboard', label: 'Overview', icon: 'layout-dashboard', href: 'admin-dashboard.html' },
        { id: 'campaigns', label: 'Campaigns', icon: 'layers', href: 'admin-campaigns.html' },
        { id: 'recruiter', label: 'Candidates', icon: 'users-round', href: 'admin-recruiter.html', badge: '7' },
        { id: 'compare', label: 'Compare', icon: 'columns-3', href: 'admin-recruiter-compare.html' },
        { id: 'invitations', label: 'Invitations', icon: 'mail-plus', href: 'admin-invitations.html' },
        { id: 'appeals', label: 'Re-review queue', icon: 'gavel', href: 'admin-appeals.html', badge: '2' }
      ]},
      { label: 'Tenant configuration', role: 'tenant_admin', items: [
        { id: 'calibration', label: 'Calibration', icon: 'sliders-horizontal', href: 'admin-recruiter-calibration.html' },
        { id: 'rubrics', label: 'Rubrics', icon: 'list-checks', href: 'admin-rubrics.html' },
        { id: 'members', label: 'Members & roles', icon: 'user-cog', href: 'admin-members.html' },
        { id: 'integrations', label: 'Integrations', icon: 'webhook', href: 'admin-integrations.html' },
        { id: 'tenant-settings', label: 'Tenant settings', icon: 'building-2', href: 'admin-tenant-settings.html' }
      ]},
      { label: 'Platform', role: 'platform_admin', items: [
        { id: 'pf-analytics', label: 'Analytics', icon: 'bar-chart-3', href: 'admin-platform-analytics.html' },
        { id: 'pf-sessions', label: 'Sessions', icon: 'activity', href: 'admin-platform-analytics-sessions.html' },
        { id: 'pf-evaluations', label: 'Evaluation quality', icon: 'scale', href: 'admin-platform-analytics-evaluations.html' },
        { id: 'pf-system', label: 'System health', icon: 'server', href: 'admin-platform-system.html' },
        { id: 'pf-llm', label: 'AI usage & cost', icon: 'cpu', href: 'admin-platform-system-llm-usage.html' },
        { id: 'pf-quotas', label: 'Quotas', icon: 'gauge', href: 'admin-platform-quotas.html' },
        { id: 'grafana', label: 'Grafana', icon: 'external-link', href: 'https://grafana.prepeet.example/d/prepeet-overview', external: true }
      ]},
      { label: 'Restricted', role: 'god', privileged: true, items: [
        { id: 'god', label: 'Super-admin console', icon: 'shield-alert', href: 'admin-god.html', privileged: true },
        { id: 'god-audit', label: 'Audit log', icon: 'scroll-text', href: 'admin-god-audit.html', privileged: true }
      ]}
    ];
  }

  /* --------------------------------------------------------------- rendering */
  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }

  function renderSidebar(host, shell, active) {
    var role = P.getRole();
    var groups = navConfig(shell).filter(function (g) { return !g.role || P.can(g.role); });
    var user = shell === 'candidate' ? P.data.candidateUser : P.data.adminUser;
    var tenant = shell === 'candidate'
      ? (P.getMode() === 'screen' ? 'Screening invitation' : 'Practice workspace')
      : P.data.tenant.name;

    var html = '<aside class="sidebar" id="app-sidebar" aria-label="Primary">' +
      '<a class="sidebar-brand" href="' + (shell === 'candidate' ? 'candidate-dashboard.html' : 'admin-dashboard.html') + '">' +
        '<span class="logo-mark">' + P.logoSvg + '</span>' +
        '<span><span class="wordmark">Prepeet</span><span class="tenant">' + esc(tenant) + '</span></span>' +
        '<button class="icon-btn sidebar-close" type="button" data-sidebar-close aria-label="Close navigation"><i data-lucide="x" class="ic"></i></button>' +
      '</a><nav class="sidebar-nav">';

    groups.forEach(function (g) {
      html += '<div><div class="nav-group-label">' + esc(g.label) + '</div><ul>';
      g.items.forEach(function (it) {
        if (it.role && !P.can(it.role)) return;
        var cur = it.id === active;
        html += '<li><a class="nav-item' + (it.privileged ? ' nav-privileged' : '') + (it.external ? ' ext' : '') + '" href="' + it.href + '"' +
          (cur ? ' aria-current="page"' : '') +
          (it.external ? ' target="_blank" rel="noopener noreferrer"' : '') + '>' +
          '<i data-lucide="' + it.icon + '" class="ic"></i><span>' + esc(it.label) + '</span>' +
          (it.external ? '<span class="sr-only">(opens in a new tab)</span>' : '') +
          (it.badge ? '<span class="nav-badge">' + esc(it.badge) + '</span>' : '') + '</a></li>';
      });
      html += '</ul></div>';
    });

    html += '</nav><div class="sidebar-foot">';
    if (shell === 'candidate') {
      html += '<a class="nav-item" href="candidate-settings.html#help"><i data-lucide="life-buoy" class="ic"></i><span>Help &amp; accessibility</span></a>';
    }
    html += '<button class="sidebar-user" type="button" data-menu="user-menu" aria-haspopup="true" aria-expanded="false">' +
        '<span class="avatar">' + esc(user.initials) + '</span>' +
        '<div><span class="u-name">' + esc(user.name) + '</span><span class="u-role">' + esc(shell === 'candidate' ? user.role : ROLES[role].label) + '</span></div>' +
        '<i data-lucide="chevrons-up-down" class="ic ic-sm" style="margin-left:auto;color:var(--fg-3)"></i></button>' +
      '<div class="menu" id="user-menu" hidden role="menu" style="left:10px;bottom:64px;width:calc(var(--sidebar-w) - 20px)">' +
        (shell === 'candidate'
          ? (P.getMode() === 'screen' ? '' : '<a class="menu-item" role="menuitem" href="candidate-profile.html"><i data-lucide="user-round" class="ic ic-sm"></i>Profile &amp; résumé</a>') +
            '<a class="menu-item" role="menuitem" href="candidate-settings.html"><i data-lucide="settings" class="ic ic-sm"></i>Settings</a>'
          : '<a class="menu-item" role="menuitem" href="admin-members.html"><i data-lucide="user-cog" class="ic ic-sm"></i>Your access</a>') +
        '<button class="menu-item" role="menuitem" type="button" data-theme-toggle><i data-lucide="moon" class="ic ic-sm" data-icon-moon></i><i data-lucide="sun" class="ic ic-sm" data-icon-sun hidden></i><span data-theme-label>Dark theme</span></button>' +
        '<div class="menu-sep"></div>' +
        '<a class="menu-item danger" role="menuitem" href="login.html"><i data-lucide="log-out" class="ic ic-sm"></i>Sign out</a>' +
      '</div></div></aside><div class="sidebar-scrim" data-sidebar-close aria-hidden="true"></div>';

    host.outerHTML = html;
  }

  function renderTopbar(host, shell, opts) {
    var crumbs = (opts.crumbs || '').split('|').filter(Boolean);
    var title = opts.title || document.title.split('·')[0].trim();
    var role = P.getRole();

    var html = '<header class="topbar">' +
      '<button class="icon-btn menu-toggle" type="button" data-sidebar-open aria-label="Open navigation" aria-controls="app-sidebar" aria-expanded="false"><i data-lucide="menu" class="ic"></i></button>' +
      '<nav class="crumbs" aria-label="Breadcrumb"><ol style="display:flex;align-items:center;gap:6px;min-width:0">';
    crumbs.forEach(function (c) {
      var parts = c.split('>');
      html += '<li>' + (parts[1] ? '<a href="' + esc(parts[1].trim()) + '">' + esc(parts[0].trim()) + '</a>' : esc(parts[0].trim())) + '</li><li class="sep" aria-hidden="true">/</li>';
    });
    html += '<li class="current" aria-current="page">' + esc(title) + '</li></ol></nav>';

    html += '<div class="topbar-actions">';

    if (shell === 'candidate') {
      var mode = P.getMode();
      html += '<div class="mode-switch"><span class="lbl">Mock data</span>' +
        '<div class="segmented" role="group" aria-label="Preview mock data as practice or screen mode">' +
        '<button type="button" data-mode-set="practice" aria-pressed="' + (mode === 'practice') + '">Practice</button>' +
        '<button type="button" data-mode-set="screen" aria-pressed="' + (mode === 'screen') + '">Screen</button>' +
        '</div></div>';
    } else {
      html += '<div class="mode-switch"><span class="lbl">Viewing as</span>' +
        '<select class="select input-sm" data-role-set aria-label="Preview the console as a different role">';
      Object.keys(ROLES).forEach(function (r) { html += '<option value="' + r + '"' + (r === role ? ' selected' : '') + '>' + ROLES[r].label + '</option>'; });
      html += '</select></div>';
    }

    if (opts.search !== 'off') {
      html += '<div class="input-wrap topbar-search"><i data-lucide="search" class="ic ic-sm"></i>' +
        '<label class="sr-only" for="topbar-search">Search</label>' +
        '<input class="input input-sm" id="topbar-search" type="search" placeholder="' + esc(shell === 'candidate' ? 'Search your sessions…' : 'Search candidates, sessions, tenants…') + '"></div>';
    }

    html += '<button class="icon-btn" type="button" data-theme-toggle aria-pressed="false">' +
        '<i data-lucide="moon" class="ic" data-icon-moon></i><i data-lucide="sun" class="ic" data-icon-sun hidden></i></button>' +
      '<button class="icon-btn" type="button" data-menu="notif-menu" aria-haspopup="true" aria-expanded="false" aria-label="Notifications, 2 unread" style="position:relative">' +
        '<i data-lucide="bell" class="ic"></i><span style="position:absolute;top:6px;right:7px;width:7px;height:7px;border-radius:50%;background:var(--danger);border:1.5px solid var(--surface)"></span></button>' +
      '<div class="menu" id="notif-menu" hidden role="menu" style="right:16px;top:52px;width:320px">' +
        '<div class="menu-label">Notifications</div>' + notifications(shell) +
      '</div></div></header>';

    host.outerHTML = html;
  }

  function notifications(shell) {
    /* A screening candidate is only ever told about their own invitation. */
    if (shell === 'candidate' && P.getMode() === 'screen') {
      return [
        { i: 'file-check', t: 'Your ICU screening was submitted to Northwind Health', s: '22 August', h: 'candidate-session-complete.html?mode=screen' },
        { i: 'mail', t: 'Northwind Health System invited you to a screening', s: '2 days ago', h: 'invitation-accept.html' }
      ].map(function (n) {
        return '<a class="menu-item" role="menuitem" href="' + n.h + '" style="align-items:flex-start">' +
          '<i data-lucide="' + n.i + '" class="ic ic-sm" style="margin-top:2px;color:var(--fg-3)"></i>' +
          '<span><span style="display:block">' + esc(n.t) + '</span><span style="display:block;font-size:var(--text-2xs);color:var(--fg-3);margin-top:2px">' + esc(n.s) + '</span></span></a>';
      }).join('') + '<div class="menu-sep"></div><a class="menu-item" role="menuitem" href="candidate-settings.html#notifications"><i data-lucide="settings" class="ic ic-sm"></i>Notification settings</a>';
    }
    var items = shell === 'candidate' ? [
      { i: 'sparkles', t: 'Your Systems Design review is ready', s: '12 minutes ago', h: 'candidate-session-review.html' },
      { i: 'flag', t: 'Goal “3 sessions this week” is 2 of 3 done', s: 'Yesterday', h: 'candidate-goals.html' },
      { i: 'mail', t: 'Northwind Health invited you to a screening', s: '2 days ago', h: 'invitation-accept.html' }
    ] : [
      { i: 'user-check', t: 'Amara Osei completed an ICU screening', s: '8 minutes ago', h: 'admin-recruiter-detail.html' },
      { i: 'gavel', t: 'Re-review requested by Tomás Herrera', s: '1 hour ago', h: 'admin-appeals.html' },
      { i: 'alert-triangle', t: 'Evaluation queue depth above threshold', s: '3 hours ago', h: 'admin-platform-system.html' }
    ];
    return items.map(function (n) {
      return '<a class="menu-item" role="menuitem" href="' + n.h + '" style="align-items:flex-start">' +
        '<i data-lucide="' + n.i + '" class="ic ic-sm" style="margin-top:2px;color:var(--fg-3)"></i>' +
        '<span><span style="display:block">' + esc(n.t) + '</span><span style="display:block;font-size:var(--text-2xs);color:var(--fg-3);margin-top:2px">' + esc(n.s) + '</span></span></a>';
    }).join('') + '<div class="menu-sep"></div><a class="menu-item" role="menuitem" href="' + (shell === 'candidate' ? 'candidate-settings.html#notifications' : 'admin-tenant-settings.html#notifications') + '"><i data-lucide="settings" class="ic ic-sm"></i>Notification settings</a>';
  }

  function renderBottomNav(shell, active) {
    if (shell !== 'candidate') return;
    var items = P.getMode() === 'screen' ? [
      { id: 'dashboard', label: 'Status', icon: 'file-check', href: 'candidate-dashboard.html' },
      { id: 'invitation', label: 'Invitation', icon: 'mail-check', href: 'invitation-accept.html' },
      { id: 'settings', label: 'You', icon: 'user-round', href: 'candidate-settings.html' }
    ] : [
      { id: 'dashboard', label: 'Home', icon: 'home', href: 'candidate-dashboard.html' },
      { id: 'sessions', label: 'Sessions', icon: 'history', href: 'candidate-sessions.html' },
      { id: 'start', label: 'Practise', icon: 'circle-play', href: 'candidate-start-interview.html' },
      { id: 'progression', label: 'Progress', icon: 'trending-up', href: 'candidate-progression.html' },
      { id: 'profile', label: 'You', icon: 'user-round', href: 'candidate-profile.html' }
    ];
    var nav = document.createElement('nav');
    nav.className = 'bottom-nav';
    nav.setAttribute('aria-label', 'Primary, mobile');
    nav.innerHTML = '<ul>' + items.map(function (i) {
      return '<li><a href="' + i.href + '"' + (i.id === active ? ' aria-current="page"' : '') + '><i data-lucide="' + i.icon + '" class="ic"></i>' + esc(i.label) + '</a></li>';
    }).join('') + '</ul>';
    document.body.appendChild(nav);
  }

  /* -------------------------------------------------------------- behaviour */
  function wire() {
    var app = document.querySelector('.app');
    document.addEventListener('click', function (e) {
      if (e.target.closest('[data-sidebar-open]')) { app.classList.add('sidebar-open'); var c = document.querySelector('[data-sidebar-close]'); if (c) c.focus(); var b = e.target.closest('[data-sidebar-open]'); b.setAttribute('aria-expanded', 'true'); }
      else if (e.target.closest('[data-sidebar-close]')) { app.classList.remove('sidebar-open'); var ob = document.querySelector('[data-sidebar-open]'); if (ob) { ob.setAttribute('aria-expanded', 'false'); ob.focus(); } }
      var mb = e.target.closest('[data-mode-set]');
      if (mb) { P.setMode(mb.getAttribute('data-mode-set')); }
      var cb = e.target.closest('[data-sidebar-collapse]');
      if (cb) { app.classList.toggle('sidebar-collapsed'); try { localStorage.setItem('prepeet.sidebar', app.classList.contains('sidebar-collapsed') ? 'collapsed' : 'open'); } catch (err) {} }
    });
    document.addEventListener('change', function (e) { if (e.target.matches('[data-role-set]')) P.setRole(e.target.value); });
    document.addEventListener('keydown', function (e) { if (e.key === 'Escape' && app.classList.contains('sidebar-open')) { app.classList.remove('sidebar-open'); } });
    try { if (localStorage.getItem('prepeet.sidebar') === 'collapsed') app.classList.add('sidebar-collapsed'); } catch (e) {}
  }

  /* -------------------------------------------------------------------- init */
  P.mountShell = function () {
    var body = document.body;
    P.favicon();                       // every page gets the favicon, shell or not
    var shell = body.getAttribute('data-shell');
    if (!shell) { if (P.init) P.init(); if (window.PrepeetTheme) window.PrepeetTheme.refresh(); return; }
    var active = body.getAttribute('data-nav') || '';
    var sb = document.querySelector('[data-shell-sidebar]');
    if (sb) renderSidebar(sb, shell, active);
    var tb = document.querySelector('[data-shell-topbar]');
    if (tb) renderTopbar(tb, shell, { title: body.getAttribute('data-title'), crumbs: body.getAttribute('data-crumbs'), search: body.getAttribute('data-search') });
    renderBottomNav(shell, active);
    if (!document.querySelector('.skip-link')) {
      var sk = document.createElement('a'); sk.className = 'skip-link'; sk.href = '#main-content'; sk.textContent = 'Skip to main content';
      body.insertBefore(sk, body.firstChild);
    }
    wire();
    if (window.PrepeetTheme) window.PrepeetTheme.refresh();
    if (P.init) P.init();
  };

  document.addEventListener('DOMContentLoaded', function () { P.mountShell(); });
})();

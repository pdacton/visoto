// static/js/sidebar.js
// Left nav sidebar: remembered collapsed state, tab persistence and active-link
// highlighting. The slide-in/out itself is slide-sidebar.js; on mobile this
// sidebar is Tabler's push-down menu, not an overlay.

document.addEventListener('DOMContentLoaded', function () {
  const sidebar = document.querySelector('.navbar-vertical');
  const sidebarToggle = document.getElementById('sidebar-toggle');

  if (!sidebar || !document.querySelector('.page-wrapper') || !sidebarToggle) return;

  // The desktop collapsed state is remembered across page loads, so closing the
  // sidebar survives a navigation instead of reopening on the next page. Only the
  // collapsed state is stored: open is the default, so it is the absence of a key.
  // layout/base.html reads the same key before the first paint to avoid a flash;
  // tabler_overrides.css documents that handoff.
  const COLLAPSED_KEY = 'visoto-sidebar-collapsed';
  const PRELOAD_CLASS = 'vs-sidebar-preload-collapsed';
  const TAB_KEY = 'visoto-sidebar-tab';

  // Wrapped because localStorage throws outright in some privacy modes, where the
  // sidebar should still work — just without remembering.
  function storageGet(key) {
    try { return localStorage.getItem(key); } catch (e) { return null; }
  }

  function storageSet(key, value) {
    try {
      if (value === null) localStorage.removeItem(key);
      else localStorage.setItem(key, value);
    } catch (e) { /* private mode: this visit just will not be remembered */ }
  }

  VisotoSlideSidebar({
    el: sidebar,
    side: 'left',
    menu: document.getElementById('sidebar-menu'),
    toggles: [sidebarToggle],
    // The same custom property the pre-paint rule in tabler_overrides.css uses,
    // so the two offsets cannot drift apart. Not measured: below the breakpoint
    // the sidebar spans the viewport, and that width would stick on widening.
    width: function () {
      return parseInt(getComputedStyle(document.documentElement)
        .getPropertyValue('--vs-sidebar-width'), 10) || 240;
    },
    readOpen: function () { return storageGet(COLLAPSED_KEY) !== '1'; },
    writeOpen: function (open) { storageSet(COLLAPSED_KEY, open ? null : '1'); },
  });

  // Hand over from the pre-paint head start in layout/base.html: the inline
  // margins just set by VisotoSlideSidebar are the real state from here on, and
  // leaving the root class in place would keep the sidebar shut through a later
  // reopen, since that selector knows nothing about the click.
  document.documentElement.classList.remove(PRELOAD_CLASS);

  // Persist active sidebar tab across page loads
  const savedTab = storageGet(TAB_KEY);
  if (savedTab) {
    const tabLink = document.querySelector('#sidebar-tabs .nav-link[href="' + savedTab + '"]');
    if (tabLink) tabLink.click();
  }
  document.querySelectorAll('#sidebar-tabs .nav-link').forEach(function (el) {
    el.addEventListener('shown.bs.tab', function () {
      storageSet(TAB_KEY, el.getAttribute('href'));
    });
  });

  // Highlight the sidebar link matching the current URL and reopen its parent
  // submenu (Tabler convention: `active` on the item, `show` on the dropdown).
  // Resource links are compared by their IRI (canonical /resource?iri= form;
  // the legacy /resource/<iri> path form is still understood), other links by
  // decoded pathname.
  function decodedPath(path) {
    try { return decodeURIComponent(path); } catch (e) { return path; }
  }
  function resourceIri(u) {
    const q = u.searchParams.get('iri');
    if (q) return q;
    const m = u.pathname.match(/^\/resource\/(.+)/);
    return m ? decodedPath(m[1]) : null;
  }
  const currentUrl = new URL(window.location.href);
  const currentIri = resourceIri(currentUrl);
  const currentPath = decodedPath(currentUrl.pathname);
  document.querySelectorAll('#sidebar-panel-nav a[href]').forEach(function (link) {
    const href = link.getAttribute('href');
    if (!href || href.startsWith('#')) return;
    const linkUrl = new URL(href, window.location.origin);
    const linkIri = resourceIri(linkUrl);
    const matches = currentIri
      ? linkIri === currentIri
      : (!linkIri && decodedPath(linkUrl.pathname) === currentPath);
    if (!matches) return;
    link.classList.add('active');
    const dropdown = link.closest('.nav-item.dropdown');
    if (dropdown) {
      const toggle = dropdown.querySelector('.dropdown-toggle');
      const menu = dropdown.querySelector('.dropdown-menu');
      if (toggle && menu) {
        toggle.classList.add('show');
        toggle.setAttribute('aria-expanded', 'true');
        menu.classList.add('show');
      }
    }
  });
});

// static/js/right-sidebar.js
// Right assistant sidebar: remembered open state and width, close button and
// the drag-to-resize handle. The slide-in/out itself is slide-sidebar.js; on
// mobile this sidebar slides in as an overlay. Default state: CLOSED (unlike
// the left sidebar, which is open).

document.addEventListener('DOMContentLoaded', function () {
  const rightSidebar = document.querySelector('.right-sidebar');
  const pageWrapper = document.querySelector('.page-wrapper');
  const rightSidebarToggle = document.getElementById('right-sidebar-toggle');
  const resizeHandle = document.querySelector('.right-sidebar-resize-handle');

  if (!rightSidebar || !pageWrapper || !rightSidebarToggle) return;

  // Width constraints
  const MIN_WIDTH = 200;
  const MAX_WIDTH = 600;
  const DEFAULT_WIDTH = 250;

  // Safe localStorage wrapper
  function getLocalStorage(key, defaultValue = null) {
    try {
      return localStorage.getItem(key);
    } catch (e) {
      console.warn('localStorage access failed:', e);
      return defaultValue;
    }
  }

  function setLocalStorage(key, value) {
    try {
      localStorage.setItem(key, value);
    } catch (e) {
      console.warn('localStorage write failed:', e);
    }
  }

  // Restore saved width from localStorage or use default
  let sidebarWidth = parseInt(getLocalStorage('rightSidebarWidth')) || DEFAULT_WIDTH;
  sidebarWidth = Math.max(MIN_WIDTH, Math.min(MAX_WIDTH, sidebarWidth));

  // The topbar button, and the settings-menu item that stands in for it on
  // narrow screens where the button itself is hidden.
  const sidebar = VisotoSlideSidebar({
    el: rightSidebar,
    side: 'right',
    menu: document.getElementById('right-sidebar-menu'),
    toggles: [rightSidebarToggle, document.getElementById('right-sidebar-toggle-menu')],
    width: function () { return sidebarWidth; },
    sizeEl: true,
    // chat.js reads the same key to decide whether to summarise on load.
    readOpen: function () { return getLocalStorage('rightSidebarOpen') === 'true'; },
    writeOpen: function (open) { setLocalStorage('rightSidebarOpen', String(open)); },
    overlay: { className: 'right-sidebar-overlay', backdropClass: 'right-sidebar-backdrop' },
  });

  // Close button handler (inside sidebar)
  const rightSidebarClose = document.getElementById('right-sidebar-close');
  if (rightSidebarClose) {
    rightSidebarClose.addEventListener('click', function (e) {
      e.preventDefault();
      sidebar.close();
    });
  }

  // Resize functionality (desktop only)
  if (resizeHandle) {
    let isResizing = false;
    let startX = 0;
    let startWidth = 0;

    resizeHandle.addEventListener('mousedown', function(e) {
      // Only allow resize when sidebar is open and on desktop
      if (sidebar.isMobile() || !sidebar.isOpen()) return;

      isResizing = true;
      startX = e.clientX;
      startWidth = rightSidebar.offsetWidth;

      // Disable transitions during resize for smooth dragging
      sidebar.setTransitions(false);

      // Change cursor and prevent text selection
      document.body.style.cursor = 'ew-resize';
      document.body.style.userSelect = 'none';
      document.body.classList.add('resizing');

      e.preventDefault();
    });

    document.addEventListener('mousemove', function(e) {
      if (!isResizing) return;

      // Calculate new width (invert delta for right-side)
      const deltaX = startX - e.clientX;
      sidebarWidth = Math.max(MIN_WIDTH, Math.min(MAX_WIDTH, startWidth + deltaX));
      sidebar.relayout();

      e.preventDefault();
    });

    document.addEventListener('mouseup', function() {
      if (!isResizing) return;

      isResizing = false;

      // Re-enable transitions
      sidebar.setTransitions(true);

      // Restore cursor and text selection
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      document.body.classList.remove('resizing');

      // Save width to localStorage
      setLocalStorage('rightSidebarWidth', sidebarWidth);
    });
  }
});

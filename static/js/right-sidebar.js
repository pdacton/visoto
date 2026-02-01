// static/js/right-sidebar.js
// Right sidebar toggle logic with responsive behavior and resize functionality
// Desktop: slides sidebar in/out from right and squeezes content
// Mobile: shows sidebar as overlay with menu auto-expanded
// Default state: CLOSED (unlike left sidebar which is open)

document.addEventListener('DOMContentLoaded', function () {
  const rightSidebar = document.querySelector('.right-sidebar');
  const pageWrapper = document.querySelector('.page-wrapper');
  const rightSidebarToggle = document.getElementById('right-sidebar-toggle');
  const rightSidebarMenu = document.getElementById('right-sidebar-menu');
  const resizeHandle = document.querySelector('.right-sidebar-resize-handle');

  if (!rightSidebar || !pageWrapper || !rightSidebarToggle) return;

  // Add transition classes for smooth sliding
  rightSidebar.style.transition = 'margin-right 0.3s cubic-bezier(.4,0,.2,1), transform 0.3s cubic-bezier(.4,0,.2,1)';
  pageWrapper.style.transition = pageWrapper.style.transition || 'margin-left 0.3s cubic-bezier(.4,0,.2,1), margin-right 0.3s cubic-bezier(.4,0,.2,1)';

  // Width constraints
  const MIN_WIDTH = 200;
  const MAX_WIDTH = 600;
  const DEFAULT_WIDTH = 250;

  // Restore saved width from localStorage or use default
  let sidebarWidth = parseInt(localStorage.getItem('rightSidebarWidth')) || DEFAULT_WIDTH;
  sidebarWidth = Math.max(MIN_WIDTH, Math.min(MAX_WIDTH, sidebarWidth));

  // Restore saved open/closed state from localStorage (default: CLOSED)
  let rightSidebarOpen = localStorage.getItem('rightSidebarOpen') === 'true';
  const MOBILE_BREAKPOINT = 992;

  function isMobile() {
    return window.innerWidth < MOBILE_BREAKPOINT;
  }

  function openRightSidebarDesktop() {
    rightSidebar.style.marginRight = '0';
    rightSidebar.style.width = sidebarWidth + 'px';
    rightSidebar.style.transform = 'none';
    pageWrapper.style.marginRight = sidebarWidth + 'px';
    rightSidebar.classList.remove('right-sidebar-overlay');
    rightSidebarOpen = true;
    localStorage.setItem('rightSidebarOpen', 'true');
  }

  function closeRightSidebarDesktop() {
    rightSidebar.style.marginRight = '-' + sidebarWidth + 'px';
    rightSidebar.style.width = sidebarWidth + 'px';
    rightSidebar.style.transform = 'none';
    pageWrapper.style.marginRight = '0';
    rightSidebar.classList.remove('right-sidebar-overlay');
    rightSidebarOpen = false;
    localStorage.setItem('rightSidebarOpen', 'false');
  }

  function openRightSidebarMobile() {
    rightSidebar.classList.add('right-sidebar-overlay', 'show');
    rightSidebar.style.marginRight = '';
    rightSidebar.style.transform = '';
    rightSidebar.style.width = '';
    pageWrapper.style.marginRight = '';
    // Auto-expand menu on mobile
    if (rightSidebarMenu) {
      rightSidebarMenu.classList.add('show');
    }
    // Add backdrop
    addBackdrop();
    rightSidebarOpen = true;
    localStorage.setItem('rightSidebarOpen', 'true');
  }

  function closeRightSidebarMobile() {
    rightSidebar.classList.remove('show');
    rightSidebar.style.transform = '';
    rightSidebar.style.marginRight = '';
    if (rightSidebarMenu) {
      rightSidebarMenu.classList.remove('show');
    }
    removeBackdrop();
    rightSidebarOpen = false;
    localStorage.setItem('rightSidebarOpen', 'false');
  }

  function addBackdrop() {
    let backdrop = document.querySelector('.right-sidebar-backdrop');
    if (!backdrop) {
      backdrop = document.createElement('div');
      backdrop.className = 'right-sidebar-backdrop';
      backdrop.addEventListener('click', function() {
        closeRightSidebarMobile();
      });
      document.body.appendChild(backdrop);
    }
    setTimeout(() => backdrop.classList.add('show'), 10);
  }

  function removeBackdrop() {
    const backdrop = document.querySelector('.right-sidebar-backdrop');
    if (backdrop) {
      backdrop.classList.remove('show');
      setTimeout(() => backdrop.remove(), 300);
    }
  }

  // Initialize sidebar state based on screen size and saved state
  function initializeRightSidebar() {
    if (isMobile()) {
      // On mobile, always start closed (overlay mode doesn't persist across pages)
      rightSidebar.style.marginRight = '';
      rightSidebar.style.transform = '';
      rightSidebar.style.width = '';
      pageWrapper.style.marginRight = '';
      rightSidebar.classList.remove('right-sidebar-overlay', 'show');
      if (rightSidebarMenu) {
        rightSidebarMenu.classList.remove('show');
      }
      rightSidebarOpen = false;
    } else {
      // Desktop: restore saved state or default to CLOSED
      if (rightSidebarOpen) {
        openRightSidebarDesktop();
      } else {
        closeRightSidebarDesktop();
      }
    }
  }

  initializeRightSidebar();

  // Toggle handler
  rightSidebarToggle.addEventListener('click', function (e) {
    e.preventDefault();
    if (isMobile()) {
      if (rightSidebarOpen) {
        closeRightSidebarMobile();
      } else {
        openRightSidebarMobile();
      }
    } else {
      if (rightSidebarOpen) {
        closeRightSidebarDesktop();
      } else {
        openRightSidebarDesktop();
      }
    }
  });

  // Handle resize - reinitialize on breakpoint change
  let wasMobile = isMobile();
  window.addEventListener('resize', function() {
    const nowMobile = isMobile();
    if (wasMobile !== nowMobile) {
      // Breakpoint changed
      wasMobile = nowMobile;
      initializeRightSidebar();
    }
  });

  // Resize functionality (desktop only)
  if (resizeHandle) {
    let isResizing = false;
    let startX = 0;
    let startWidth = 0;

    resizeHandle.addEventListener('mousedown', function(e) {
      // Only allow resize when sidebar is open and on desktop
      if (isMobile() || !rightSidebarOpen) return;

      isResizing = true;
      startX = e.clientX;
      startWidth = rightSidebar.offsetWidth;

      // Disable transitions during resize for smooth dragging
      rightSidebar.style.transition = 'none';
      pageWrapper.style.transition = 'none';

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
      const newWidth = Math.max(MIN_WIDTH, Math.min(MAX_WIDTH, startWidth + deltaX));

      // Update width
      sidebarWidth = newWidth;
      rightSidebar.style.width = newWidth + 'px';
      pageWrapper.style.marginRight = newWidth + 'px';

      e.preventDefault();
    });

    document.addEventListener('mouseup', function() {
      if (!isResizing) return;

      isResizing = false;

      // Re-enable transitions
      rightSidebar.style.transition = '';
      pageWrapper.style.transition = '';

      // Restore cursor and text selection
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      document.body.classList.remove('resizing');

      // Save width to localStorage
      localStorage.setItem('rightSidebarWidth', sidebarWidth);
    });
  }
});

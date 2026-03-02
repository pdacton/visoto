// assets/js/sidebar.js
// Unified sidebar toggle logic with responsive behavior
// Desktop: slides sidebar in/out and squeezes content
// Mobile: shows sidebar as overlay with menu auto-expanded

document.addEventListener('DOMContentLoaded', function () {
  const sidebar = document.querySelector('.navbar-vertical');
  const pageWrapper = document.querySelector('.page-wrapper');
  const sidebarToggle = document.getElementById('sidebar-toggle');
  const sidebarMenu = document.getElementById('sidebar-menu');

  if (!sidebar || !pageWrapper || !sidebarToggle) return;

  // Add transition classes for smooth sliding
  sidebar.style.transition = 'margin-left 0.3s cubic-bezier(.4,0,.2,1), transform 0.3s cubic-bezier(.4,0,.2,1)';
  pageWrapper.style.transition = 'margin-left 0.3s cubic-bezier(.4,0,.2,1)';

  // Set initial state
  let sidebarOpen = true;
  const sidebarWidth = sidebar.offsetWidth || 250;
  const MOBILE_BREAKPOINT = 992;

  function isMobile() {
    return window.innerWidth < MOBILE_BREAKPOINT;
  }

  function openSidebarDesktop() {
    sidebar.style.marginLeft = '0';
    sidebar.style.transform = 'none';
    pageWrapper.style.marginLeft = sidebarWidth + 'px';
    sidebar.classList.remove('sidebar-overlay');
    sidebarOpen = true;
  }

  function closeSidebarDesktop() {
    sidebar.style.marginLeft = '-' + sidebarWidth + 'px';
    sidebar.style.transform = 'none';
    pageWrapper.style.marginLeft = '0';
    sidebar.classList.remove('sidebar-overlay');
    sidebarOpen = false;
  }

  function openSidebarMobile() {
    sidebar.classList.add('sidebar-overlay', 'show');
    sidebar.style.marginLeft = '';
    sidebar.style.transform = '';
    pageWrapper.style.marginLeft = '';
    // Auto-expand menu on mobile
    if (sidebarMenu) {
      sidebarMenu.classList.add('show');
    }
    // Add backdrop
    addBackdrop();
    sidebarOpen = true;
  }

  function closeSidebarMobile() {
    sidebar.classList.remove('show');
    sidebar.style.transform = '';
    sidebar.style.marginLeft = '';
    if (sidebarMenu) {
      sidebarMenu.classList.remove('show');
    }
    removeBackdrop();
    sidebarOpen = false;
  }

  function addBackdrop() {
    let backdrop = document.querySelector('.sidebar-backdrop');
    if (!backdrop) {
      backdrop = document.createElement('div');
      backdrop.className = 'sidebar-backdrop';
      backdrop.addEventListener('click', function() {
        closeSidebarMobile();
      });
      document.body.appendChild(backdrop);
    }
    setTimeout(() => backdrop.classList.add('show'), 10);
  }

  function removeBackdrop() {
    const backdrop = document.querySelector('.sidebar-backdrop');
    if (backdrop) {
      backdrop.classList.remove('show');
      setTimeout(() => backdrop.remove(), 300);
    }
  }

  // Initialize sidebar state based on screen size
  function initializeSidebar() {
    if (isMobile()) {
      // Clear any inline styles from desktop mode
      sidebar.style.marginLeft = '';
      sidebar.style.transform = '';
      pageWrapper.style.marginLeft = '';
      sidebar.classList.remove('sidebar-overlay', 'show');
      if (sidebarMenu) {
        sidebarMenu.classList.remove('show');
      }
      sidebarOpen = false;
    } else {
      openSidebarDesktop();
    }
  }

  initializeSidebar();

  // Toggle handler
  sidebarToggle.addEventListener('click', function (e) {
    e.preventDefault();
    if (isMobile()) {
      if (sidebarOpen) {
        closeSidebarMobile();
      } else {
        openSidebarMobile();
      }
    } else {
      if (sidebarOpen) {
        closeSidebarDesktop();
      } else {
        openSidebarDesktop();
      }
    }
  });

  // Persist active sidebar tab across page loads
  const savedTab = localStorage.getItem('visoto-sidebar-tab');
  if (savedTab) {
    const tabLink = document.querySelector('#sidebar-tabs .nav-link[href="' + savedTab + '"]');
    if (tabLink) tabLink.click();
  }
  document.querySelectorAll('#sidebar-tabs .nav-link').forEach(function (el) {
    el.addEventListener('shown.bs.tab', function () {
      localStorage.setItem('visoto-sidebar-tab', el.getAttribute('href'));
    });
  });

  // Handle resize - reinitialize on breakpoint change
  let wasMobile = isMobile();
  window.addEventListener('resize', function() {
    const nowMobile = isMobile();
    if (wasMobile !== nowMobile) {
      // Breakpoint changed
      wasMobile = nowMobile;
      initializeSidebar();
    }
  });
});

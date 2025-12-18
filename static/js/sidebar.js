// assets/js/sidebar.js
// Sidebar toggle logic: slides sidebar in/out and squeezes content container

document.addEventListener('DOMContentLoaded', function () {
  const sidebar = document.querySelector('.navbar-vertical');
  const pageWrapper = document.querySelector('.page-wrapper');
  const sidebarToggle = document.getElementById('sidebar-toggle');

  if (!sidebar || !pageWrapper || !sidebarToggle) return;

  // Add transition classes for smooth sliding
  sidebar.style.transition = 'margin-left 0.3s cubic-bezier(.4,0,.2,1)';
  pageWrapper.style.transition = 'margin-left 0.3s cubic-bezier(.4,0,.2,1)';

  // Set initial state
  let sidebarOpen = true;
  const sidebarWidth = sidebar.offsetWidth || 250; // fallback width

  function openSidebar() {
    sidebar.style.marginLeft = '0';
    pageWrapper.style.marginLeft = sidebarWidth + 'px';
    sidebarOpen = true;
  }

  function closeSidebar() {
    sidebar.style.marginLeft = '-' + sidebarWidth + 'px';
    pageWrapper.style.marginLeft = '0';
    sidebarOpen = false;
  }

  // Initialize sidebar state
  openSidebar();

  sidebarToggle.addEventListener('click', function (e) {
    e.preventDefault();
    if (sidebarOpen) {
      closeSidebar();
    } else {
      openSidebar();
    }
  });

  // Responsive: auto-close sidebar on small screens
  function handleResize() {
    if (window.innerWidth < 992) {
      closeSidebar();
    } else {
      openSidebar();
    }
  }
  window.addEventListener('resize', handleResize);
  handleResize();
});

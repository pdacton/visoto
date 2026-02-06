// assets/js/darkmode.js

document.addEventListener('DOMContentLoaded', function () {
  const themeToggle = document.getElementById('theme-toggle');
  const html = document.documentElement;

  // Helper to update icon and tooltip based on theme
  function updateThemeToggle(theme) {
    if (!themeToggle) return;
    const themeIcon = themeToggle.querySelector('#theme-icon');

    // switch the icon
    if (themeIcon) {
      if (theme === 'dark') {
        themeIcon.setAttribute('data-lucide', 'sun');
        themeIcon.setAttribute('aria-label', 'Switch to light mode');
      } else {
        themeIcon.setAttribute('data-lucide', 'moon');
        themeIcon.setAttribute('aria-label', 'Switch to dark mode');
      }
      // Re-initialize lucide icons after changing the data-lucide attribute
      if (window.lucide) {
        window.lucide.createIcons();
      }
    }

    // switch the container of the icon
    if (theme === 'dark') {
      themeToggle.setAttribute('aria-label', 'Enable light mode');
      themeToggle.setAttribute('data-bs-original-title', 'Enable light mode');
    } else {
      themeToggle.setAttribute('aria-label', 'Enable dark mode');
      themeToggle.setAttribute('data-bs-original-title', 'Enable dark mode');
    }
    
    // If using Bootstrap tooltips, update the tooltip if already initialized
    // Note: Tabler exposes Bootstrap under tabler.bootstrap
    if (window.tabler && window.tabler.bootstrap && window.tabler.bootstrap.Tooltip) {
      const tooltip = window.tabler.bootstrap.Tooltip.getInstance(themeToggle);
      if (tooltip) tooltip.setContent({ '.tooltip-inner': themeToggle.getAttribute('data-bs-original-title') });
    }
  }

  // Load theme from localStorage if available
  const savedTheme = localStorage.getItem('bs-theme');
  const currentTheme = html.getAttribute('data-bs-theme');

  if (savedTheme) {
    // Only set attribute if it's different from current value to avoid triggering MutationObserver unnecessarily
    if (savedTheme !== currentTheme) {
      html.setAttribute('data-bs-theme', savedTheme);
    }
    updateThemeToggle(savedTheme);
  } else {
    // Set icon based on current theme (default)
    updateThemeToggle(currentTheme);
  }

  themeToggle?.addEventListener('click', function () {
    const currentTheme = html.getAttribute('data-bs-theme');
    const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
    html.setAttribute('data-bs-theme', newTheme);
    localStorage.setItem('bs-theme', newTheme);
    updateThemeToggle(newTheme);
  });
});

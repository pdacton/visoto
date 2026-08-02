/**
 * Search functionality for visoto
 * Handles topbar search box submission and advanced filter UX
 */

document.addEventListener('DOMContentLoaded', function() {

  // Topbar search form - submit on Enter key
  const topbarForm = document.getElementById('topbar-search-form');
  const topbarInput = document.getElementById('topbar-search-input');

  if (topbarForm && topbarInput) {
    // On search page, populate topbar with current query
    const searchPageInput = document.getElementById('search-query');
    if (searchPageInput && searchPageInput.value) {
      topbarInput.value = searchPageInput.value;
    }

    topbarInput.addEventListener('keypress', function(e) {
      if (e.key === 'Enter') {
        e.preventDefault();
        if (topbarInput.value.trim()) {
          topbarForm.submit();
        }
      }
    });

    // "/" focuses search from anywhere on the page, Escape gives focus back.
    // Ignored while the user is already typing into a field (including
    // contenteditable, e.g. the assistant), or while a modifier is held, so it
    // never swallows a literal slash — SPARQL and IRIs are full of them.
    document.addEventListener('keydown', function(e) {
      if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) return;
      const el = document.activeElement;
      if (el && (el.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName))) return;
      e.preventDefault();
      topbarInput.focus();
      topbarInput.select();
    });

    topbarInput.addEventListener('keydown', function(e) {
      if (e.key === 'Escape') topbarInput.blur();
    });
  }

  // Search page - auto-expand advanced filters if any filter is selected
  const searchPage = document.getElementById('search-form');
  if (searchPage) {
    const classFilter = document.getElementById('class-filter');
    const propertyFilter = document.getElementById('property-filter');
    const filterContent = document.getElementById('filter-content');

    if (classFilter && propertyFilter && filterContent) {
      // Auto-expand accordion if filters are set
      if (classFilter.value !== '' || propertyFilter.value !== '') {
        // Use Bootstrap's Collapse API to show the accordion
        const bsCollapse = new bootstrap.Collapse(filterContent, {
          toggle: true
        });
      }
    }
  }

});

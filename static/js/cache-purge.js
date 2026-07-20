document.getElementById('clear-cache-btn')?.addEventListener('click', async (e) => {
  e.preventDefault();
  const btn = e.currentTarget;
  const icon = btn.querySelector('i');
  const originalLabel = btn.textContent.trim();
  const originalIcon = icon.getAttribute('data-lucide');

  const setFeedback = (lucideIcon, label) => {
    icon.setAttribute('data-lucide', lucideIcon);
    btn.lastChild.textContent = label;
    if (window.lucide) lucide.createIcons();
  };

  try {
    const resp = await fetch('/api/cache/purge', { method: 'POST' });
    const data = await resp.json();
    setFeedback(data.success ? 'check' : 'x', data.success ? 'Cache cleared' : 'Failed to clear cache');
  } catch {
    setFeedback('x', 'Failed to clear cache');
  }

  setTimeout(() => setFeedback(originalIcon, originalLabel), 2000);
});

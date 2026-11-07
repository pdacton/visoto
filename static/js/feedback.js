/*
  Topbar feedback button → prefilled GitHub issue.

  The modal (#feedbackModal, topbar.html, marked data-feedback) only composes the
  issue; "Continue on GitHub" opens the configured new-issue URL
  (data-feedback-url, from feedback_url in visoto.config) in a new tab with the
  title and text filled in, and the visitor submits it on GitHub. Nothing is
  posted to Visoto.

  The page context (URL, endpoint, language, browser) is read here at open time,
  never rendered by the server: the topbar is cached and identical for everyone.

  buildIssueURL is pure, so tests/js/feedback.test.js can cover it under node.
*/
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else {
    root.VisotoFeedback = api;
    api.boot();
  }
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  // Free-text cap, matching the textarea's maxlength.
  var MAX_DESCRIPTION = 3000;
  // GitHub rejects a new-issue URL much past 8 KB. The text is sent twice (see
  // buildIssueURL), so the description is shortened until the URL fits.
  var MAX_URL = 7500;

  function issueURL(base, title, description, context) {
    var url = new URL(base);
    url.searchParams.set('title', title);
    // `body` is what GitHub uses when the template= issue form does not exist
    // (yet) on the repo's default branch — or there is no template= at all.
    // When the form does exist, GitHub ignores `body` and fills the form's
    // `description` and `context` fields (.github/ISSUE_TEMPLATE/feedback.yml)
    // instead. Sending both works either way.
    url.searchParams.set('body',
      (description ? description + '\n\n' : '') + '### Context\n\n' + context);
    if (url.searchParams.has('template')) {
      url.searchParams.set('description', description);
      url.searchParams.set('context', context);
    }
    return url.toString();
  }

  /**
   * The new-issue URL for one piece of feedback.
   *   base     feedback_url, e.g. …/issues/new?template=feedback.yml
   *   fb       { kind, title, description, context }
   */
  function buildIssueURL(base, fb) {
    var title = (fb.title || '').trim();
    if (fb.kind) title = '[' + fb.kind + '] ' + title;
    var description = (fb.description || '').trim().slice(0, MAX_DESCRIPTION);
    var context = fb.context || '';
    var href = issueURL(base, title, description, context);
    while (href.length > MAX_URL && description.length > 0) {
      description = description.length < 20
        ? ''
        : description.slice(0, Math.floor(description.length * 0.8)).trimEnd() + '…';
      href = issueURL(base, title, description, context);
    }
    return href;
  }

  /** The page context as a Markdown list, from plain values. */
  function contextText(ctx) {
    var lines = [
      '- Page: ' + ctx.page,
      '- Endpoint: ' + (ctx.endpoint || '(default)'),
      '- Language: ' + (ctx.language === '' ? '(none)' : ctx.language || '(default)'),
      '- Browser: ' + ctx.browser,
    ];
    return lines.join('\n');
  }

  function currentContext() {
    return contextText({
      page: window.location.href,
      endpoint: typeof window.activeEndpointSlug === 'function' ? window.activeEndpointSlug() : '',
      language: typeof window.activeSiteLang === 'function' ? window.activeSiteLang() : undefined,
      browser: navigator.userAgent,
    });
  }

  function wire(modal) {
    var form = modal.querySelector('[data-feedback-form]');
    var titleInput = modal.querySelector('#feedback-title');
    var descInput = modal.querySelector('#feedback-description');
    var contextEl = modal.querySelector('[data-feedback-context]');
    var submit = modal.querySelector('[data-feedback-submit]');
    if (!form || !titleInput || !submit) return;

    var sync = function () { submit.disabled = titleInput.value.trim() === ''; };
    titleInput.addEventListener('input', sync);

    // Bootstrap dispatches these as DOM events, so no handle on its global is
    // needed, and the listener works whatever order the scripts loaded in.
    modal.addEventListener('show.bs.modal', function () {
      if (contextEl) contextEl.textContent = currentContext();
      sync();
    });
    modal.addEventListener('shown.bs.modal', function () { titleInput.focus(); });

    form.addEventListener('submit', function (e) {
      e.preventDefault();
      if (titleInput.value.trim() === '') return;
      var kind = modal.querySelector('input[name="feedback-kind"]:checked');
      var href = buildIssueURL(modal.getAttribute('data-feedback-url'), {
        kind: kind ? kind.value : '',
        title: titleInput.value,
        description: descInput ? descInput.value : '',
        context: currentContext(),
      });
      window.open(href, '_blank', 'noopener');
      form.reset();
      sync();
      var Modal = window.tabler && window.tabler.Modal;
      if (Modal) Modal.getOrCreateInstance(modal).hide();
    });
  }

  function boot() {
    var run = function () {
      var modal = document.querySelector('[data-feedback]');
      if (modal) wire(modal);
    };
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', run);
    else run();
  }

  return { buildIssueURL: buildIssueURL, contextText: contextText, boot: boot, MAX_DESCRIPTION: MAX_DESCRIPTION };
});

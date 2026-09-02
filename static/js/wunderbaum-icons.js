/* eslint-disable */
/*
  Lucide icon map for Wunderbaum, shared by both trees.

  Wunderbaum ships a Bootstrap Icons map by default (iconMap: "bootstrap"), so it
  renders expanders and node icons as <i class="bi bi-chevron-right"> and friends.
  This project loads Lucide, not Bootstrap Icons, so every one of those glyphs
  resolves to nothing: the expanders were invisible and the node icons occupied a
  20px box that pushed each title away from its chevron.

  Rather than restyle Wunderbaum's elements from the outside, this replaces the
  icons at the source. Wunderbaum's renderer treats any iconMap value containing
  "<" as markup and REPLACES the element with it, so handing it real Lucide SVG
  gets the actual icon — same geometry, stroke weight and round caps as every other
  icon on the page — with no mask, no font, and no ::before styling.

  A node icon is suppressed rather than replaced: the tree already shows a label
  (and, on the lazy tree, a resource icon), so a folder glyph beside it is noise.
  Wunderbaum skips the element entirely when a node's icon is the literal false,
  which is what reclaims the horizontal space; "" would still leave the empty box.

  Sizes come from Wunderbaum's own --wb-icon-* variables so the boxes keep the
  layout it expects; only the artwork changes.
*/
(function () {
  'use strict';

  // One <svg> factory: Lucide's icons differ only in their inner geometry.
  //
  // Wunderbaum REPLACES its own element with this markup, and it hit-tests clicks
  // by reading classList.contains("wb-expander") on whatever was clicked. So the
  // markup must reproduce that <i class="wb-expander"> itself — returning a bare
  // <svg> renders correctly but silently kills expand/collapse, because no element
  // carries the class the handler looks for any more.
  //
  // The <i> therefore keeps Wunderbaum's own classes and the SVG goes inside it,
  // with pointer-events: none in CSS so a click always lands on the <i>.
  function icon(inner, wbClass, extraClass) {
    return '<i class="' + wbClass + ' vs-wb-icon' + (extraClass ? ' ' + extraClass : '') + '">' +
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" ' +
      'stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" ' +
      'aria-hidden="true" focusable="false">' + inner + '</svg></i>';
  }

  // Expanders must carry wb-expander (hit-testing); other icons carry wb-icon.
  function expander(inner, extraClass) { return icon(inner, 'wb-expander', extraClass); }
  function svg(inner, extraClass) { return icon(inner, 'wb-icon', extraClass); }

  var CHEVRON_RIGHT = '<path d="m9 18 6-6-6-6"/>';
  var CHEVRON_DOWN = '<path d="m6 9 6 6 6-6"/>';
  var DOT = '<circle cx="12" cy="12" r="1"/>';
  var SPINNER = '<path d="M21 12a9 9 0 1 1-6.219-8.56"/>';
  var ALERT = '<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/>' +
    '<path d="M12 9v4"/><path d="M12 17h.01"/>';

  // Wunderbaum applies these by key; anything omitted falls back to its default,
  // so every key that could render a Bootstrap glyph is listed.
  window.visotoWunderbaumIcons = {
    // Expander states. The collapsed and lazy states share the right chevron: a
    // lazy node looks expandable because it may be, and finding out is the point.
    expanderCollapsed: expander(CHEVRON_RIGHT, 'vs-wb-expander'),
    expanderLazy: expander(CHEVRON_RIGHT, 'vs-wb-expander'),
    expanderExpanded: expander(CHEVRON_DOWN, 'vs-wb-expander'),

    // A leaf: a dot in the chevron's place, so the title stays anchored to its
    // indent level instead of floating. Lucide's dot is a 2-unit circle on a
    // 24-unit canvas, so it is scaled up in CSS to read at the chevron's weight.
    noData: svg(DOT, 'vs-wb-leaf'),

    loading: expander(SPINNER, 'vs-wb-spin'),
    error: expander(ALERT, 'vs-wb-error'),

    // Node icons are NOT suppressed from here: the renderer honours `false` only
    // as a node's own icon property, so each tree sets icon:false per node instead
    // (cmd/visoto/lazy_tree.go and sparql-tree.js). These entries only decide what
    // a node that DOES ask for an icon would get.
    folder: false,
    folderOpen: false,
    folderLazy: false,
    doc: false,
  };
})();

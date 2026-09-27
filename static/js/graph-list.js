/* eslint-disable */
/*
  List view of a Graph Explorer embed (GL-39): the canvas as two tables — nodes
  and edges — in the same card, the screen-reader equivalent of the diagram.
  Selection is shared: a node's checkbox is its selection state on the canvas,
  and a change on either side shows on the other.

  Installed by graph-kit.js on kit.attach(); markup: graphListView in
  templates/partials/graph-toolbar.html. Rows are rebuilt from the model on
  every history change while the view is open.
*/
(function () {
  'use strict';

  if (window.VisotoGraphList) return;

  function cell(text, cls) {
    var td = document.createElement('td');
    if (cls) td.className = cls;
    td.textContent = text;
    return td;
  }
  function resourceCell(label, iri) {
    var td = document.createElement('td');
    var a = document.createElement('a');
    a.href = window.visotoResourceHref ? window.visotoResourceHref(iri) : iri;
    a.textContent = label;
    a.title = iri;
    td.appendChild(a);
    return td;
  }

  function install(kit) {
    var c = kit.commands;
    var id = kit.id;
    var view = document.getElementById(id + '-list');
    var toolbar = document.getElementById(id + '-toolbar');
    if (!view) return;
    var nodesBody = view.querySelector('[data-graph-list-nodes]');
    var edgesBody = view.querySelector('[data-graph-list-edges]');

    function render() {
      if (view.hidden) return;
      var g = c.drawnGraph();
      var selected = {};
      c.selectedIds().forEach(function (s) { selected[s] = true; });
      var degree = {};
      g.links.forEach(function (l) {
        degree[l.sourceId] = (degree[l.sourceId] || 0) + 1;
        degree[l.targetId] = (degree[l.targetId] || 0) + 1;
      });
      var labelOf = {};
      g.elements.forEach(function (e) { labelOf[e.iri] = e.label; });

      nodesBody.replaceChildren();
      g.elements.slice().sort(function (a, b) { return a.label.localeCompare(b.label); }).forEach(function (e) {
        var tr = document.createElement('tr');
        var td = document.createElement('td');
        var box = document.createElement('input');
        box.type = 'checkbox';
        box.className = 'form-check-input m-0';
        box.checked = !!selected[e.id];
        box.setAttribute('aria-label', vsTf('js.graph.selectNode', 'Select {name}', { name: e.label }));
        box.addEventListener('change', function () {
          var ids = c.selectedIds().filter(function (x) { return x !== e.id; });
          if (box.checked) ids.push(e.id);
          c.setSelection(ids);
        });
        td.appendChild(box);
        tr.appendChild(td);
        tr.appendChild(resourceCell(e.label, e.iri));
        tr.appendChild(cell(e.typeLabels.join(', '), 'text-secondary'));
        tr.appendChild(cell(String(degree[e.id] || 0), 'text-end'));
        if (selected[e.id]) tr.classList.add('table-active');
        nodesBody.appendChild(tr);
      });

      edgesBody.replaceChildren();
      g.links.forEach(function (l) {
        var tr = document.createElement('tr');
        tr.appendChild(resourceCell(labelOf[l.source] || l.source, l.source));
        tr.appendChild(cell(l.typeLabel, 'text-secondary'));
        tr.appendChild(resourceCell(labelOf[l.target] || l.target, l.target));
        edgesBody.appendChild(tr);
      });
      view.querySelector('[data-graph-list-nodecount]').textContent = String(g.elements.length);
      view.querySelector('[data-graph-list-edgecount]').textContent = String(g.links.length);
    }

    function toggle(show) {
      view.hidden = show === undefined ? !view.hidden : !show;
      if (toolbar) {
        toolbar.querySelectorAll('[data-graph-action="list-view"]').forEach(function (btn) {
          btn.classList.toggle('active', !view.hidden);
          btn.setAttribute('aria-pressed', String(!view.hidden));
        });
      }
      render();
      if (!view.hidden) {
        var first = view.querySelector('input, a');
        if (first) first.focus();
      }
    }
    kit.toggleList = toggle;

    c.onSelectionChange(render);
    c.history.events.on('historyChanged', render);
    if (toolbar) {
      toolbar.addEventListener('click', function (e) {
        var btn = e.target instanceof Element && e.target.closest('[data-graph-action="list-view"]');
        if (!btn) return;
        e.preventDefault();
        toggle();
      });
    }
  }

  window.VisotoGraphList = { install: install };
})();

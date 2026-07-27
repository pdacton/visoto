/* eslint-disable */
/*
  Behaviour for the "sparqlTree" partial (templates/partials/sparql-tree.html).

  Extracted from an inline <script> in that partial so the code is cacheable,
  lintable and editable as JavaScript. Every value it needs already travelled in
  JSON data islands; the only thing the template interpolated was the instance
  id used to address them, which now arrives as a data attribute.

  Attributes read from the root element:
    data-sparql-tree      marker; presence means "initialize me"
    data-sparql-tree-id   DOM id prefix for this tree's elements and islands
*/
(function () {
  'use strict';

  function initSparqlTree(root) {
    var ID = root.getAttribute('data-sparql-tree-id');
    if (!ID) return;


    // Parse embedded JSON data
    var bindings = JSON.parse(document.getElementById(ID + "-bindings").innerHTML.trim());
    var startExpanded = document.getElementById(ID + "-expanded").innerHTML.trim() === "true";
    var vars = JSON.parse(document.getElementById(ID + "-vars").innerHTML.trim());
    var extraVars = vars.filter(function(v) { return v !== "node" && v !== "parent"; });
    var resourceIRI = document.getElementById(ID + "-resourceIRI").innerHTML.trim();

    // --- arrayToTree: Convert flat SPARQL bindings to Wunderbaum tree structure ---
    // Single pass: create nodes and wire parent-child relationships together.
    // Nodes whose parent hasn't been seen yet are stored in pendingChildren and
    // re-wired at the end (handles bindings where parent rows come after child rows).
    function arrayToTree(bindings, expanded) {
      var nodeMap = {};   // key -> wunderbaum node object
      var roots = [];     // top-level nodes, in encounter order
      var rootKeys = new Set();
      var childKeys = new Set();
      // child nodes whose parent binding hadn't been seen yet
      var pendingChildren = {}; // parentKey -> [childNode, ...]

      bindings.forEach(function(binding) {
        var nodeBinding = binding["node"];
        if (!nodeBinding) return;

        var key = nodeBinding.Value;

        // Extra vars go directly on the source node object (not nested under "data").
        // Wunderbaum exposes non-reserved source properties as e.node.data[varName].
        var node = { title: nodeBinding.DisplayText || key, key: key, _iri: key };
        for (var varName in binding) {
          if (varName === "node" || varName === "parent") continue;
          var b = binding[varName];
          if (b) node[varName] = { value: b.Value, label: b.DisplayText || b.Value, type: b.Type };
        }
        if (expanded) node.expanded = true;
        nodeMap[key] = node;

        // Flush any children that were waiting for this node as parent
        if (pendingChildren[key]) {
          node.children = pendingChildren[key];
          pendingChildren[key].forEach(function(c) { childKeys.add(c.key); });
          delete pendingChildren[key];
        }

        // Wire to parent
        var parentBinding = binding["parent"];
        if (!parentBinding || !parentBinding.Value) {
          rootKeys.add(key);
          roots.push(node);
        } else {
          var parentKey = parentBinding.Value;
          var parent = nodeMap[parentKey];
          if (parent) {
            if (!parent.children) parent.children = [];
            parent.children.push(node);
            childKeys.add(key);
          } else {
            // Parent not yet seen — queue for later
            if (!pendingChildren[parentKey]) pendingChildren[parentKey] = [];
            pendingChildren[parentKey].push(node);
          }
        }
      });

      // Any remaining pending children whose parent was never in the dataset = roots
      for (var parentKey in pendingChildren) {
        if (!nodeMap[parentKey]) {
          pendingChildren[parentKey].forEach(function(node) { roots.push(node); });
        }
      }

      return roots;
    }

    // Build the tree data structure
    var treeData = arrayToTree(bindings, startExpanded);

    // Build column definitions for treegrid mode
    var columns = null;
    if (extraVars.length > 0) {
      columns = [{ id: "*", title: "BEZEICHNUNG", width: "*" }];
      extraVars.forEach(function(varName) {
        columns.push({ id: varName, title: varName.toUpperCase(), width: "150px" });
      });
    }

    // Set tree container height: fit content up to a max, so virtual scrolling
    // kicks in for large datasets while small trees don't leave whitespace.
    var treeElem = document.getElementById(ID + "-tree");
    var rowHeightPx = 44;
    var headerHeightPx = columns ? 44 : 0;
    var maxHeightPx = 600;
    var contentHeightPx = bindings.length * rowHeightPx + headerHeightPx;
    treeElem.style.height = Math.min(contentHeightPx, maxHeightPx) + "px";

    // Initialize Wunderbaum
    var tree = new mar10.Wunderbaum({
      element: treeElem,
      source: treeData,
      columns: columns,
      rowHeightPx: 44, // static row height required by wunderbaum, must be same as --wb-header-height CSS variable in css override file
      filter: {
        autoApply: true,
        mode: "hide"
      },
      init: function(e) {
        // Expand to specific ResourceIRI if provided (after tree is fully initialized)
        if (resourceIRI) {
          console.log("Expanding tree to resource IRI:", resourceIRI);
          var targetNode = tree.findFirst(function(node) {
            return node.key === resourceIRI;
          });
          if (targetNode) {
            targetNode.makeVisible().then(function() {
              targetNode.setActive();
            });
          } else {
            console.log("Node with key '" + resourceIRI + "' not found in tree");
          }
        }
      },
      render: function(e) {
        const titleSpan = e.nodeElem.querySelector(".wb-title");
        if (titleSpan) {
          const displayTitle = e.node.titleWithHighlight || e.node.title;
          titleSpan.innerHTML = "<a href='/resource?iri=" + encodeURIComponent(e.node.key) + "'>" + displayTitle + "</a>";
        }
        if (e.renderColInfosById) {
          for (const col of Object.values(e.renderColInfosById)) {
            if (col.id === "*") continue;
            const colData = e.node.data[col.id];
            col.elem.textContent = colData ? (colData.label || colData.value || "") : "";
          }
        }
      },
    });

    // --- Filter with debounce ---
    var searchInput = document.getElementById("search-" + ID);
    if (searchInput) {
      var filterTimeout;
      searchInput.addEventListener("input", function(e) {
        clearTimeout(filterTimeout);
        filterTimeout = setTimeout(function() {
          var searchValue = e.target.value.trim();
          if (!searchValue) {
            tree.clearFilter();
          } else {
            tree.filterNodes(searchValue);
          }
        }, 300);
      });
    }

    // --- Expand All button ---
    var expandBtn = document.getElementById("expand-" + ID);
    if (expandBtn) {
      expandBtn.addEventListener("click", function() {
        tree.expandAll();
      });
    }

    // --- Collapse All button ---
    var collapseBtn = document.getElementById("collapse-tree-" + ID);
    if (collapseBtn) {
      collapseBtn.addEventListener("click", function() {
        tree.visit(function(node) {
          node.setExpanded(false);
        });
      });
    }

  }

  // Re-entrant on purpose: a duplicate <script src> tag, or this file being
  // re-executed inside an HTMX-swapped fragment, must still pick up elements
  // that were not in the DOM the first time. There is deliberately no
  // module-level "already loaded" latch — only the per-element guard below.
  // Initialize every tree on the page. Guarded per element so a second boot
  // (duplicate script tag, or a fragment swapped in later) cannot double-init.
  function boot() {
    document.querySelectorAll('[data-sparql-tree]').forEach(function (root) {
      if (root.__visotoSparqlTreeInit) return;
      root.__visotoSparqlTreeInit = true;
      initSparqlTree(root);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();

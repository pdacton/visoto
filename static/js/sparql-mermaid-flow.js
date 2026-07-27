/* eslint-disable */
/*
  Behaviour for the "sparqlMermaidFlow" partial
  (templates/partials/sparql-mermaid-flow.html).

  Extracted from an inline <script> in that partial so the code is cacheable,
  lintable and editable as JavaScript. The template now emits only markup and
  JSON data islands; every per-instance value arrives as a data attribute.

  Attributes read from the root element:
    data-mermaid-flow            marker; presence means "initialize me"
    data-mermaid-flow-id         id prefix for this diagram's elements/islands
    data-mermaid-flow-direction  initial direction ("LR", "TD" or "graph")

  Note: because this is no longer Go template output, the Mermaid hexagon shape
  template is written literally as the two-brace LABEL placeholder instead of
  the quoting dance the template engine required.
*/
(function () {
  'use strict';

  // Mermaid is loaded by the page, so rendering waits for window 'load'. Run
  // immediately when that has already happened (this file may be parsed later
  // than the inline block it replaced).
  function onWindowLoad(cb) {
    if (document.readyState === 'complete') { cb(); return; }
    window.addEventListener('load', cb);
  }

  function initMermaidFlow(root) {
    // Initialize global registry for Mermaid diagram renderers
    if (!window.mermaidDiagramRenderers) {
      window.mermaidDiagramRenderers = {};
    }

    // Wrap in IIFE to avoid variable name collisions between multiple diagrams
    var diagramId = root.getAttribute('data-mermaid-flow-id');
    var panZoomInstance = null;

    onWindowLoad(function () {
      // ========================================
      // Variables
      // ========================================
      var bindings = JSON.parse(document.getElementById(diagramId + "-bindings").innerHTML.trim());
      var initialDirection = document.getElementById("direction-" + diagramId).value || root.getAttribute('data-mermaid-flow-direction') || "LR";
      var mapping = null;
      var configElement = document.getElementById(diagramId + "-config");
      var config = configElement ? JSON.parse(configElement.innerHTML.trim()) : getDefaultConfig();
      var extraFieldElement = document.getElementById(diagramId + "-extraField");
      var extraField = extraFieldElement && extraFieldElement.innerHTML.trim() ? extraFieldElement.innerHTML.trim() : null;

      // ========================================
      // Constants
      // ========================================

      // Mermaid shape templates
      var SHAPE_TEMPLATES = {
        'rectangle': '["LABEL"]',
        'rounded': '("LABEL")',
        'stadium': '(["LABEL"])',
        'subroutine': '[["LABEL"]]',
        'cylindrical': '[("LABEL")]',
        'circle': '(("LABEL"))',
        'asymmetric': '>"LABEL"]',
        'rhombus': '{LABEL}',
        'hexagon': '{{LABEL}}',
        'parallelogram': '[/"LABEL"/]',
        'parallelogram_alt': '[\\"LABEL"\\]',
        'trapezoid': '[/"LABEL"\\]',
        'trapezoid_alt': '[\\"LABEL"/]',
        'double_circle': '((("LABEL")))'
      };

      // ========================================
      // Helper Functions
      // ========================================

      // Return default configuration
      function getDefaultConfig() {
        return {
          typeMap: {},
          classDefs: [
            'classDef default fill:#f0f0f0,stroke:#999999,stroke-width:2px'
          ],
          reversePredicates: [],
          defaultShape: 'rounded'
        };
      }

      // Escape special characters in labels for Mermaid syntax
      function escapeLabel(text) {
        if (!text) return '';
        // Replace special characters that could break Mermaid syntax
        // Use HTML entities and escaped characters
        return text
          .replace(/"/g, '&quot;')
          .replace(/'/g, '&apos;')
          .replace(/</g, '&lt;')
          .replace(/>/g, '&gt;')
          .replace(/\n/g, '<br>');
      }

      // Map RDF type URI to CSS class name for node styling
      function getNodeClass(typeUri) {
        if (!typeUri || !config.typeMap) return 'default';

        // Check each type in typeMap for a match
        for (var typeName in config.typeMap) {
          if (typeUri.includes(typeName)) {
            return config.typeMap[typeName].cssClass || 'default';
          }
        }

        return 'default';
      }

      // Get node shape based on RDF type
      function getNodeShape(typeUri, label) {
        var shapeName = config.defaultShape || 'rounded';

        // Look up shape from typeMap if type is recognized
        if (typeUri && config.typeMap) {
          for (var typeName in config.typeMap) {
            if (typeUri.includes(typeName)) {
              shapeName = config.typeMap[typeName].shape || shapeName;
              break;
            }
          }
        }

        // Get shape template and substitute label
        var template = SHAPE_TEMPLATES[shapeName] || SHAPE_TEMPLATES['rounded'];
        return template.replace('LABEL', label);
      }

      // ========================================
      // Data Processing Functions
      // ========================================

      // Get CSS class definitions from config (native Mermaid syntax)
      function buildClassDefs() {
        // Return classDefs array from config, or empty array if not provided
        return config.classDefs || [];
      }

      // Build URI to node ID mapping with type information (simple index: node1, node2, etc.)
      function buildNodeMapping(bindings) {
        var uriToId = {};
        var idToUri = {};
        var idToType = {};
        var idToExtraField = {};
        var index = 1;

        bindings.forEach(function(binding) {
          // Skip bindings without required fields
          if (!binding.from || !binding.to) return;

          if (!uriToId[binding.from.Value]) {
            var id = 'node' + index++;
            uriToId[binding.from.Value] = id;
            idToUri[id] = binding.from.Value;
            // Store fromType if available
            if (binding.fromType && binding.fromType.DisplayText) {
              idToType[id] = binding.fromType.DisplayText;
            } else if (binding.fromType && binding.fromType.Value) {
              idToType[id] = binding.fromType.Value;
            }
          }
          if (!uriToId[binding.to.Value]) {
            var id = 'node' + index++;
            uriToId[binding.to.Value] = id;
            idToUri[id] = binding.to.Value;
            // Store toType if available
            if (binding.toType && binding.toType.DisplayText) {
              idToType[id] = binding.toType.DisplayText;
            } else if (binding.toType && binding.toType.Value) {
              idToType[id] = binding.toType.Value;
            }
          }
        });

        // Second pass: collect extraField values for all nodes
        // Check for both 'from' and 'to' specific fields (fromValidFrom, toValidFrom)
        // or a single shared field (validFrom)
        if (extraField) {
          var fromFieldName = 'from' + extraField.charAt(0).toUpperCase() + extraField.slice(1);
          var toFieldName = 'to' + extraField.charAt(0).toUpperCase() + extraField.slice(1);

          bindings.forEach(function(binding) {
            if (!binding.from || !binding.to) return;

            var fromUri = binding.from.Value;
            var toUri = binding.to.Value;
            var fromId = uriToId[fromUri];
            var toId = uriToId[toUri];

            // Check for from-specific field (e.g., fromValidFrom)
            if (binding[fromFieldName]) {
              var fromValue = binding[fromFieldName].DisplayText || binding[fromFieldName].Value;
              if (fromValue && fromId && !idToExtraField[fromId]) {
                idToExtraField[fromId] = fromValue;
              }
            }

            // Check for to-specific field (e.g., toValidFrom)
            if (binding[toFieldName]) {
              var toValue = binding[toFieldName].DisplayText || binding[toFieldName].Value;
              if (toValue && toId && !idToExtraField[toId]) {
                idToExtraField[toId] = toValue;
              }
            }

            // Fallback: check for single shared field (e.g., validFrom)
            if (binding[extraField]) {
              var extraValue = binding[extraField].DisplayText || binding[extraField].Value;
              if (extraValue) {
                if (fromId && !idToExtraField[fromId]) {
                  idToExtraField[fromId] = extraValue;
                }
                if (toId && !idToExtraField[toId]) {
                  idToExtraField[toId] = extraValue;
                }
              }
            }
          });
        }

        return { uriToId: uriToId, idToUri: idToUri, idToType: idToType, idToExtraField: idToExtraField };
      }

      // Build Mermaid diagram syntax
      function buildMermaidDiagram(bindings, direction, uriToId, idToType, idToExtraField) {
        var header = direction === "graph" ? "graph LR" : "flowchart " + direction;
        var nodeLines = [];
        var edgeLines = [];
        var styleLines = [];
        var nodesDefined = {};

        // Collect all unique nodes and edges
        bindings.forEach(function(binding) {
          // Skip bindings without required fields
          if (!binding.from || !binding.to || !binding.predicate) return;

          var fromId = uriToId[binding.from.Value];
          var toId = uriToId[binding.to.Value];
          var fromLabel = escapeLabel(binding.from.DisplayText || binding.from.Value);
          var toLabel = escapeLabel(binding.to.DisplayText || binding.to.Value);
          var edgeLabel = escapeLabel(binding.predicate.DisplayText || binding.predicate.Value);

          // Build node label with type if available
          var fromNodeLabel = fromLabel;
          if (idToType[fromId]) {
            fromNodeLabel = fromLabel + '<br><small>' + escapeLabel(idToType[fromId]) + '</small>';
          }
          // Add extra field if available
          if (idToExtraField && idToExtraField[fromId]) {
            fromNodeLabel = fromNodeLabel + '<br><small>' + escapeLabel(idToExtraField[fromId]) + '</small>';
          }

          var toNodeLabel = toLabel;
          if (idToType[toId]) {
            toNodeLabel = toLabel + '<br><small>' + escapeLabel(idToType[toId]) + '</small>';
          }
          // Add extra field if available
          if (idToExtraField && idToExtraField[toId]) {
            toNodeLabel = toNodeLabel + '<br><small>' + escapeLabel(idToExtraField[toId]) + '</small>';
          }

          // Collect unique nodes with styling
          if (!nodesDefined[fromId]) {
            nodeLines.push(fromId + getNodeShape(idToType[fromId], fromNodeLabel));

            // Add style if type is recognized
            var fromClass = getNodeClass(idToType[fromId]);
            if (fromClass) {
              styleLines.push(fromId + ':::' + fromClass);
            }

            nodesDefined[fromId] = true;
          }
          if (!nodesDefined[toId]) {
            nodeLines.push(toId + getNodeShape(idToType[toId], toNodeLabel));

            // Add style if type is recognized
            var toClass = getNodeClass(idToType[toId]);
            if (toClass) {
              styleLines.push(toId + ':::' + toClass);
            }

            nodesDefined[toId] = true;
          }

          // Collect edge with direction based on predicate type
          var predicateValue = binding.predicate.Value || '';
          var isPredecessor = config.reversePredicates && config.reversePredicates.some(function(pred) {
            return predicateValue.includes(pred);
          });

          // Debug: Log predicate matching
          if (edgeLines.length === 0) {
            console.log('[Mermaid Debug - ' + diagramId + '] Predicate reversal check:');
            console.log('  reversePredicates config:', config.reversePredicates);
            console.log('  First predicate value:', predicateValue);
            console.log('  First predicate label:', edgeLabel);
            console.log('  isPredecessor:', isPredecessor);
          }

          if (isPredecessor) {
            // Reverse edge direction with dotted arrow for visual distinction
            edgeLines.push(toId + ' -.->|' + edgeLabel + '| ' + fromId);
          } else {
            // Normal edge direction with solid arrow
            edgeLines.push(fromId + ' -->|' + edgeLabel + '| ' + toId);
          }
        });

        // Build CSS class definitions dynamically from config
        var classDefs = buildClassDefs();

        // Build final output: header, class definitions, nodes, styles, edges
        var mermaidSyntax = [header].concat(classDefs, nodeLines, styleLines, edgeLines).join('\n');

        // Debug: Log the final Mermaid syntax
        console.log('[Mermaid Debug - ' + diagramId + '] Final Mermaid Syntax:');
        console.log(mermaidSyntax);
        console.log('[Mermaid Debug - ' + diagramId + '] Components:');
        console.log('  Header:', header);
        console.log('  Class Definitions:', classDefs);
        console.log('  Node Lines:', nodeLines);
        console.log('  Style Lines:', styleLines);
        console.log('  Edge Lines:', edgeLines);

        return mermaidSyntax;
      }

      // ========================================
      // Rendering Functions
      // ========================================

      // Render diagram
      function renderDiagram(direction) {
        var mermaidCode = buildMermaidDiagram(bindings, direction, mapping.uriToId, mapping.idToType, mapping.idToExtraField);
        var container = document.getElementById(diagramId + "-diagram");

        // Add transition class for smoother re-renders
        container.style.transition = 'opacity 0.15s ease-in-out';

        // Fade out old diagram
        var isReRender = container.querySelector('svg') !== null;
        if (isReRender) {
          container.style.opacity = '0';
        }

        // Small delay for fade out, then render
        var renderDelay = isReRender ? 150 : 0;

        return new Promise(function(resolve) {
          setTimeout(function() {
            container.innerHTML = mermaidCode;
            container.classList.add('mermaid');
            container.removeAttribute('data-processed');

            try {
              mermaid.run({ nodes: [container] }).then(function() {
                // Fade in new diagram
                container.style.opacity = '1';

                // Add click handlers and zoom/pan after Mermaid renders
                setTimeout(function() {
                  addClickHandlers(mapping.idToUri);
                  initPanZoom();
                }, 100);
                resolve();
              }).catch(function(err) {
                console.error('[Mermaid Error]', err);
                container.classList.remove('mermaid');
                container.innerHTML = '<div class="alert alert-danger">Mermaid syntax error. Check console for details.</div>';
                container.style.opacity = '1';
                resolve();
              });
            } catch (err) {
              console.error('[Mermaid Error]', err);
              container.classList.remove('mermaid');
              container.innerHTML = '<div class="alert alert-danger">Mermaid syntax error. Check console for details.</div>';
              container.style.opacity = '1';
              resolve();
            }
          }, renderDelay);
        });
      }

      // ========================================
      // Interaction Functions
      // ========================================

      // Make nodes clickable to navigate to resource pages (using event delegation)
      function addClickHandlers(idToUri) {
        var svgElement = document.querySelector("#" + diagramId + "-diagram svg");
        if (!svgElement) return;

        // Track mouse position to detect dragging vs clicking
        var mouseDownX = 0;
        var mouseDownY = 0;
        var isDragging = false;

        svgElement.addEventListener('mousedown', function(e) {
          mouseDownX = e.clientX;
          mouseDownY = e.clientY;
          isDragging = false;
        });

        svgElement.addEventListener('mousemove', function(e) {
          // If mouse moved more than 5px, consider it a drag
          var deltaX = Math.abs(e.clientX - mouseDownX);
          var deltaY = Math.abs(e.clientY - mouseDownY);
          if (deltaX > 5 || deltaY > 5) {
            isDragging = true;
          }
        });

        // Smart cursor: only show pointer when hovering over nodes
        svgElement.addEventListener('mouseover', function(e) {
          var nodeId = findNodeId(e.target);
          svgElement.style.cursor = nodeId ? 'pointer' : 'default';
        });

        svgElement.addEventListener('click', function(e) {
          // Don't navigate if user was dragging
          if (isDragging) {
            return;
          }

          var nodeId = findNodeId(e.target);
          if (nodeId) {
            e.preventDefault();
            e.stopPropagation();

            var url = visotoResourceHref(idToUri[nodeId]);
            if (e.ctrlKey || e.metaKey) {
              // Ctrl/Cmd+Click: open in new tab
              window.open(url, '_blank');
            } else {
              // Normal click: navigate in current window
              window.location.href = url;
            }
          }
        });

        // Helper function to find node ID from clicked element
        // Mermaid generates IDs like "flowchart-node1-123" or similar
        function findNodeId(element) {
          var target = element;
          while (target && target !== svgElement) {
            if (target.id) {
              // Try exact match first (for direct node elements)
              if (idToUri[target.id]) {
                return target.id;
              }

              // Try to extract node ID from Mermaid-generated IDs
              // Mermaid typically uses patterns like "flowchart-nodeX-..." or "nodeX-..."
              for (var nodeId in idToUri) {
                // Use word boundary matching to avoid "node1" matching "node10"
                var pattern = new RegExp('\\b' + nodeId + '\\b');
                if (pattern.test(target.id)) {
                  return nodeId;
                }
              }
            }

            // Check for data attributes or class names that might contain node info
            if (target.className && target.className.baseVal) {
              var classes = target.className.baseVal;
              for (var nodeId in idToUri) {
                var pattern = new RegExp('\\b' + nodeId + '\\b');
                if (pattern.test(classes)) {
                  return nodeId;
                }
              }
            }

            target = target.parentElement;
          }
          return null;
        }
      }

      // Initialize svg-pan-zoom after Mermaid renders
      function initPanZoom() {
        var svgElement = document.querySelector("#" + diagramId + "-diagram svg");
        if (!svgElement) return;

        // Destroy existing instance if re-rendering
        if (panZoomInstance) {
          panZoomInstance.destroy();
          panZoomInstance = null;
        }

        // Initialize svg-pan-zoom
        panZoomInstance = svgPanZoom(svgElement, {
          zoomEnabled: true,
          controlIconsEnabled: false,
          fit: true,
          center: true,
          minZoom: 0.1,
          maxZoom: 10,
          zoomScaleSensitivity: 0.3,
          panEnabled: true,
          dblClickZoomEnabled: false
        });

        // Wire up custom zoom controls
        var zoomInBtn = document.getElementById("zoom-in-" + diagramId);
        var zoomOutBtn = document.getElementById("zoom-out-" + diagramId);
        var zoomResetBtn = document.getElementById("zoom-reset-" + diagramId);

        if (zoomInBtn) {
          zoomInBtn.onclick = function() {
            panZoomInstance.zoomIn();
          };
        }
        if (zoomOutBtn) {
          zoomOutBtn.onclick = function() {
            panZoomInstance.zoomOut();
          };
        }
        if (zoomResetBtn) {
          zoomResetBtn.onclick = function() {
            panZoomInstance.resetZoom();
            panZoomInstance.center();
            panZoomInstance.fit();
          };
        }
      }

      // ========================================
      // Initialization
      // ========================================

      // Build node mapping
      mapping = buildNodeMapping(bindings);

      // Add resize functionality
      var resizeHandle = document.getElementById(diagramId + "-resize-handle");
      var cardBody = document.getElementById(diagramId + "-card-body");
      var isResizing = false;
      var startY = 0;
      var startHeight = 0;

      if (resizeHandle && cardBody) {
        resizeHandle.addEventListener('mousedown', function(e) {
          isResizing = true;
          startY = e.clientY;
          startHeight = cardBody.offsetHeight;
          e.preventDefault();
        });

        document.addEventListener('mousemove', function(e) {
          if (!isResizing) return;
          var deltaY = e.clientY - startY;
          var newHeight = Math.max(200, startHeight + deltaY);
          cardBody.style.height = newHeight + 'px';

          // Resize svg-pan-zoom viewport if initialized (without resetting zoom)
          if (panZoomInstance) {
            panZoomInstance.resize();
          }
        });

        document.addEventListener('mouseup', function() {
          if (isResizing) {
            isResizing = false;
            // Save height preference
            try {
              localStorage.setItem("mermaid-height-" + diagramId, cardBody.style.height);
            } catch (err) {
              // localStorage unavailable
            }
          }
        });

        // Restore saved height preference
        try {
          var savedHeight = localStorage.getItem("mermaid-height-" + diagramId);
          if (savedHeight) {
            cardBody.style.height = savedHeight;
          }
        } catch (err) {
          // localStorage unavailable
        }
      }

      // Initialize diagram (Mermaid should be loaded since we're in window.load event)
      var directionSelect = document.getElementById("direction-" + diagramId);

      // Restore saved direction preference or use initial
      var savedDirection = null;
      try {
        savedDirection = localStorage.getItem("mermaid-direction-" + diagramId);
      } catch (err) {
        // localStorage unavailable
      }

      var direction = savedDirection || initialDirection;
      if (directionSelect && savedDirection) {
        directionSelect.value = savedDirection;
      }

      // Initial render
      renderDiagram(direction);

      // Register render function globally for theme change re-rendering
      window.mermaidDiagramRenderers[diagramId] = {
        render: function() {
          // Get current direction from selector or use saved direction
          var currentDirection = directionSelect ? directionSelect.value : direction;
          return renderDiagram(currentDirection);
        },
        id: diagramId
      };

      // Direction selector with localStorage persistence
      if (directionSelect) {
        directionSelect.addEventListener("change", function(e) {
          var direction = e.target.value;
          renderDiagram(direction);
          try {
            localStorage.setItem("mermaid-direction-" + diagramId, direction);
          } catch (err) {
            // localStorage unavailable
          }
        });
      }
    });
  }

  // Re-entrant on purpose: a duplicate <script src> tag, or this file being
  // re-executed inside an HTMX-swapped fragment, must still pick up elements
  // that were not in the DOM the first time. There is deliberately no
  // module-level "already loaded" latch — only the per-element guard below.
  // Initialize every diagram on the page. Guarded per element so a second boot
  // (duplicate script tag, or a fragment swapped in later) cannot double-init.
  function boot() {
    document.querySelectorAll('[data-mermaid-flow]').forEach(function (root) {
      if (root.__visotoMermaidFlowInit) return;
      root.__visotoMermaidFlowInit = true;
      initMermaidFlow(root);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();

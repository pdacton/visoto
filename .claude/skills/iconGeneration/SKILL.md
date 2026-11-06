---
name: iconGeneration
description: Generate a Visoto resource icon (tinted squircle + Lucide glyph in an accent color, optional series badge) into static/img/resource/. Use when asked to create, generate, or add a resource/class icon.
---

# Icon Generation

Every resource icon follows one recipe: a squircle filled with an accent color at 20% opacity, plus a matching [Lucide](https://lucide.dev/icons/) glyph drawn as strokes in that same color, optionally with a small **series badge** at the bottom right.

Do not hand-write the SVG — run the generator from the repo root (Python 3, standard library only):

```
./scripts/generate-icon.py <lucide-name> <color> <OutputName> [--badge <lucide-name>] [--fit square|circle|portrait|landscape]
```

Examples:

```
./scripts/generate-icon.py bike '#FFC000' Bicycle
# -> static/img/resource/Bicycle.svg
./scripts/generate-icon.py key '#00B0F0' cube/KeyDimension --badge box
# -> static/img/resource/cube/KeyDimension.svg
```

The script fetches glyphs from unpkg (`lucide-static`), so every name must exist on https://lucide.dev/icons/. Pick the glyph whose meaning best matches the class; prefer simple, recognizable glyphs.

If the class belongs to a namespace with a configured prefix, write it into that prefix's folder (`<prefix>/Name` as the output name) so it matches only that namespace; see `static/img/resource/README.md`. The folder must already exist. Search the subfolders too when looking for related icons.

## The grid

Icons are 1024×1024 on the Apple icon grid. The script prints the fit it chose and the glyph's size; check it.

- **Optical sizing.** The main glyph's outline is fitted into a box chosen from its shape, then centered on the canvas, so every icon reads at the same visual size:

  | Fit | Box | Detected when |
  |---|---|---|
  | `square` | 600×600 | blocky: the convex outline fills ≥ 84% of its bounding box (file, calendar, building) |
  | `circle` | 640×640 | round or diagonal (globe, key, ruler, tag, pin) |
  | `portrait` | 560×720 | taller than 1 : 0.72 (thermometer, smartphone) |
  | `landscape` | 720×560 | wider than 0.72 : 1 (car, ticket) |

  Pass `--fit` only when the detected fit looks visibly too large or too small next to its neighbours.
- **Lines** are 64px at any glyph size.
- **Series badge** (`--badge`): a 240px glyph at the bottom right with thinner 36px lines. It marks a family of related classes, e.g. every `cube:` dimension carries a `box` badge. The main glyph is cut away 52px around the badge's own lines, so the two never touch and no opacity stacks up (it works on dark backgrounds too). Reuse the badge already used by the family's existing icons.

## Choosing the color

Before picking a color, check `static/img/resource/` for semantically related existing icons and match their color. The established palette:

| Domain | Color |
|---|---|
| People, agents, organizations, roles | `#C00000` |
| Government, admin areas, files, technical | `#0070C0` |
| Places, political geography | `#00B050` |
| Data cubes, dimensions, observations | `#00B0F0` |
| DCAT catalogs, datasets, distributions | `#0F9ED5` |
| Archival records, activities | `#156082` |
| Biology, taxonomy | `#9BC134` |
| SKOS concepts, labels | `#A02B93` |
| Creative works, publications | `#E97132` |
| Transport, tariffs, zones, stations | `#FFC000` |
| Rail transport, locations, pricing | `#4EA72E` |
| Versioning / lifecycle | `#79B1D9` |
| Provenance qualifications (PROV-O attribution) | `#800020` |

An explicit color outside this table may always be passed when the user asks for one or no domain fits.

## Notes

- Older icons in the folder are 512×512 generated icons (fixed 336px glyph area) or 519×519 Office exports. They render alongside the 1024 ones; leave them as they are unless asked to regenerate them.
- The OWL/RDFS meta icons (Class.svg, Property.svg, …) and the elaborate geo icons (Feature.svg, Geometry.svg) use different approaches — this skill does not cover them.

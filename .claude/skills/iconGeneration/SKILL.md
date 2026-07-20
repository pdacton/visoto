---
name: iconGeneration
description: Generate a Visoto resource icon (tinted squircle + Lucide icon in an accent color) into static/img/resource/. Use when asked to create, generate, or add a resource/class icon.
---

# Icon Generation

Every resource icon is the same mechanical recipe: a squircle background filled with an accent color at 20% opacity, plus a matching [Lucide](https://lucide.dev/icons/) icon drawn as strokes in that same color. Icons are 512×512, with the Lucide artwork in a 408×408 area (52px inset on all sides).

Do not hand-write the SVG — run the generator script from the repo root:

```
./scripts/generate-icon.sh <lucide-name> <color> <OutputName>
```

Example:

```
./scripts/generate-icon.sh bike '#FFC000' Bicycle
# -> static/img/resource/Bicycle.svg
```

The script fetches the Lucide icon from unpkg (`lucide-static`), so the icon name must exist on https://lucide.dev/icons/. Pick the Lucide icon whose meaning best matches the class; prefer simple, recognizable glyphs.

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

An explicit color outside this table may always be passed when the user asks for one or no domain fits.

## Notes

- Older icons in the folder are 519×519 Office exports with slightly different internals; they render identically alongside generated ones. Leave them as they are.
- The OWL/RDFS meta icons (Class.svg, Property.svg, …) and the elaborate geo icons (Feature.svg, Geometry.svg) use different approaches — this skill does not cover them.

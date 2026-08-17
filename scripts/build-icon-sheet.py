#!/usr/bin/env python3
"""Build an Inkscape-friendly contact sheet from static/img/resource/*.svg.

Each icon becomes one top-level <g> named `icon-<Name>`, scaled to a uniform
512x512 cell and laid out on a fixed grid. The group id is what Inkscape's
"Export > Batch export selected objects" uses as the output filename, so
round-tripping an edited icon back to `<Name>.svg` needs no renaming.

Internal ids (clipPath etc.) are namespaced per icon because 113 of the Office
exports all use id="clip0" and would otherwise collide in one document.
"""

import argparse
import pathlib
import re
import sys
import xml.etree.ElementTree as ET

SVG = "http://www.w3.org/2000/svg"
INKSCAPE = "http://www.inkscape.org/namespaces/inkscape"
SODIPODI = "http://sodipodi.sourceforge.net/DTD/sodipodi-0.0.dtd"

ET.register_namespace("", SVG)
ET.register_namespace("inkscape", INKSCAPE)
ET.register_namespace("sodipodi", SODIPODI)

CELL = 512  # nominal icon size; every source icon is normalized to this


def parse_len(value):
    """Strip units off an SVG length attribute."""
    if value is None:
        return None
    m = re.match(r"\s*(-?[\d.]+)", value)
    return float(m.group(1)) if m else None


def source_box(root):
    """Return (min_x, min_y, width, height) for an icon's user-space extent."""
    vb = root.get("viewBox")
    if vb:
        parts = [float(p) for p in re.split(r"[ ,]+", vb.strip()) if p]
        if len(parts) == 4:
            return tuple(parts)
    w = parse_len(root.get("width")) or CELL
    h = parse_len(root.get("height")) or CELL
    return (0.0, 0.0, w, h)


def strip_bounding_clips(elem, width, height):
    """Drop clip-paths that merely restate the icon's own bounds.

    The Office exports clip every icon to a rect the same size as the canvas.
    It changes nothing visually, but Inkscape treats a clipped group as an
    awkward-to-enter object, so editing a single glyph means releasing the clip
    first. A clip is only kept if it actually crops something.
    """
    clips = {}
    for cp in elem.iter(f"{{{SVG}}}clipPath"):
        cp_id = cp.get("id")
        rects = list(cp)
        if cp_id and len(rects) == 1 and rects[0].tag == f"{{{SVG}}}rect":
            r = rects[0]
            rw = parse_len(r.get("width")) or 0
            rh = parse_len(r.get("height")) or 0
            # Treat as non-cropping when it covers the full icon box (1px slack).
            if rw + 1 >= width and rh + 1 >= height:
                clips[cp_id] = True

    removed = 0
    for node in elem.iter():
        ref = node.get("clip-path")
        if not ref:
            continue
        m = re.match(r"url\(#(.+)\)", ref.strip())
        if m and clips.get(m.group(1)):
            del node.attrib["clip-path"]
            removed += 1

    if removed:
        # Delete the now-unreferenced clipPath defs, and any <defs> they emptied.
        for parent in list(elem.iter()):
            for child in list(parent):
                if child.tag == f"{{{SVG}}}clipPath" and clips.get(child.get("id")):
                    parent.remove(child)
        for parent in list(elem.iter()):
            for child in list(parent):
                if child.tag == f"{{{SVG}}}defs" and len(child) == 0:
                    parent.remove(child)
    return removed


def flatten_transparent_groups(parent):
    """Collapse <g> wrappers that carry no attributes of their own.

    Turns `g > g > g > path` from the Office exports into a shallow tree so a
    double-click in Inkscape lands on real geometry instead of a wrapper.
    """
    changed = True
    while changed:
        changed = False
        for node in list(parent.iter()):
            for child in list(node):
                if child.tag != f"{{{SVG}}}g":
                    continue
                # Only inline a group that adds nothing: no transform, style,
                # clip, or id that something else might reference.
                if any(k for k in child.attrib):
                    continue
                index = list(node).index(child)
                for offset, grandchild in enumerate(list(child)):
                    node.insert(index + offset, grandchild)
                node.remove(child)
                changed = True


def absorb_lone_translate(group, base_transform):
    """Fold a single translate-only child <g> into the parent's transform.

    The Office exports offset all artwork by a translate on one wrapper group
    (e.g. `translate(-2044 -120)`). Merging it upward removes the last wrapper
    between the named group and the actual paths.
    """
    children = list(group)
    if len(children) != 1 or children[0].tag != f"{{{SVG}}}g":
        return base_transform
    child = children[0]
    if set(child.attrib) - {"transform"}:
        return base_transform
    inner = child.get("transform", "").strip()
    m = re.fullmatch(r"translate\(\s*(-?[\d.]+)[ ,]+(-?[\d.]+)\s*\)", inner)
    if not m:
        return base_transform
    group.remove(child)
    for grandchild in list(child):
        group.append(grandchild)
    return f"{base_transform} translate({m.group(1)},{m.group(2)})"


def namespace_ids(elem, prefix):
    """Rewrite id="x" -> id="prefix-x" and every url(#x) reference to match."""
    mapping = {}
    for node in elem.iter():
        node_id = node.get("id")
        if node_id:
            new_id = f"{prefix}-{node_id}"
            mapping[node_id] = new_id
            node.set("id", new_id)
    if not mapping:
        return
    pattern = re.compile(r"url\(#([^)]+)\)")

    def sub(match):
        return f"url(#{mapping.get(match.group(1), match.group(1))})"

    for node in elem.iter():
        for key, value in list(node.attrib.items()):
            if "url(#" in value:
                node.set(key, pattern.sub(sub, value))
            elif key in ("href", f"{{http://www.w3.org/1999/xlink}}href") and value.startswith("#"):
                target = value[1:]
                if target in mapping:
                    node.set(key, f"#{mapping[target]}")


def build(icon_dir, out_path, limit, columns, gap, names=None):
    files = sorted(icon_dir.glob("*.svg"), key=lambda p: p.name.lower())
    if names:
        wanted = {n.lower() for n in names}
        files = [f for f in files if f.stem.lower() in wanted or f.name.lower() in wanted]
    if limit:
        files = files[:limit]
    if not files:
        sys.exit("no matching icons found")

    step = CELL + gap
    rows = (len(files) + columns - 1) // columns
    sheet_w = columns * step - gap
    sheet_h = rows * step - gap

    sheet = ET.Element(
        f"{{{SVG}}}svg",
        {
            "width": str(sheet_w),
            "height": str(sheet_h),
            "viewBox": f"0 0 {sheet_w} {sheet_h}",
            f"{{{INKSCAPE}}}version": "1.x",
        },
    )
    ET.SubElement(
        sheet,
        f"{{{SODIPODI}}}namedview",
        {
            "id": "namedview",
            f"{{{INKSCAPE}}}document-units": "px",
            # A grid whose spacing equals one cell+gap so icons snap to cells.
            f"{{{INKSCAPE}}}snap-global": "true",
        },
    )
    grid = ET.SubElement(
        sheet.find(f"{{{SODIPODI}}}namedview"),
        f"{{{INKSCAPE}}}grid",
        {
            "id": "iconGrid",
            "type": "xygrid",
            "units": "px",
            "spacingx": str(step),
            "spacingy": str(step),
            "originx": "0",
            "originy": "0",
            "empspacing": "1",
            "visible": "true",
        },
    )
    del grid  # constructed for its side effect on the tree

    layer = ET.SubElement(
        sheet,
        f"{{{SVG}}}g",
        {
            "id": "layer-icons",
            f"{{{INKSCAPE}}}groupmode": "layer",
            f"{{{INKSCAPE}}}label": "icons",
        },
    )

    placed = []
    for index, path in enumerate(files):
        name = path.name[: -len(".svg")]
        try:
            src = ET.parse(path).getroot()
        except ET.ParseError as exc:
            print(f"skip {path.name}: {exc}", file=sys.stderr)
            continue

        min_x, min_y, width, height = source_box(src)
        if not width or not height:
            print(f"skip {path.name}: zero extent", file=sys.stderr)
            continue

        col, row = index % columns, index // columns
        # Uniform scale so the icon's own box exactly fills a CELL, centered.
        scale = min(CELL / width, CELL / height)
        tx = col * step + (CELL - width * scale) / 2 - min_x * scale
        ty = row * step + (CELL - height * scale) / 2 - min_y * scale

        # `icon-<Name>` is the batch-export filename, so keep it verbatim.
        group = ET.SubElement(
            layer,
            f"{{{SVG}}}g",
            {
                "id": f"icon-{name}",
                f"{{{INKSCAPE}}}label": name,
                "transform": f"translate({tx:g},{ty:g}) scale({scale:g})",
            },
        )
        # Children go straight onto the named group: one group per icon means a
        # single double-click enters it, and batch export sees a flat list.
        for child in list(src):
            group.append(child)
        strip_bounding_clips(group, width, height)
        flatten_transparent_groups(group)
        group.set("transform", absorb_lone_translate(group, group.get("transform", "")))
        flatten_transparent_groups(group)
        # Namespace ids only after stripping, so clips removed above don't
        # leave orphaned prefixed ids behind.
        namespace_ids(group, f"i{index}")
        # namespace_ids also rewrote the group's own id; restore the export name.
        group.set("id", f"icon-{name}")
        placed.append((name, col, row))

    out_path.parent.mkdir(parents=True, exist_ok=True)
    ET.ElementTree(sheet).write(out_path, encoding="utf-8", xml_declaration=True)
    print(f"{out_path}: {len(placed)} icons, {columns}x{rows} grid, {gap}px gap, cell {CELL}px")
    for name, col, row in placed:
        print(f"  r{row + 1}c{col + 1}  icon-{name}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--icon-dir", default="static/img/resource", type=pathlib.Path)
    ap.add_argument("--out", default="static/img/resource/_sheet/icon-sheet.svg", type=pathlib.Path)
    ap.add_argument("--limit", type=int, default=20, help="0 = all icons")
    ap.add_argument("--columns", type=int, default=5)
    ap.add_argument("--gap", type=int, default=64, help="pixel gap between cells")
    ap.add_argument("--name", action="append", dest="names", help="only this icon (repeatable)")
    args = ap.parse_args()
    build(args.icon_dir, args.out, args.limit, args.columns, args.gap, args.names)


if __name__ == "__main__":
    main()

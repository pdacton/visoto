#!/usr/bin/env python3
"""Generate a Visoto resource icon: tinted squircle + Lucide glyph, optionally
with a series badge at the bottom right.

See .claude/skills/iconGeneration/SKILL.md for the palette and conventions.

    ./scripts/generate-icon.py bike '#FFC000' Bicycle
    ./scripts/generate-icon.py key '#00B0F0' cube/KeyDimension --badge box

Canvas is 1024x1024 (Apple icon grid). The main glyph is sized by its shape
(optical sizing): its outline's bounding box is fitted into a keyline box chosen
from the outline, then centered on the canvas.

  square     600x600  blocky: convex hull fills >= 84% of the bounding box
  circle     640x640  round or diagonal
  portrait   560x720  taller than 1:0.72
  landscape  720x560  wider than 0.72:1

Lines are 64px at any glyph size. The badge is a fixed 240px live area at the
bottom right with 36px lines; the main glyph is cut away 52px around the
badge's own lines so the two never touch. Standard library only.
"""

import argparse
import math
import pathlib
import re
import sys
import urllib.error
import urllib.request

S = 1024
# Same squircle as the earlier 512 icons, scaled x2.
SQUIRCLE = ("M0 512C0 102.4 102.4 0 512 0 921.6 0 1024 102.4 1024 512 "
            "1024 921.6 921.6 1024 512 1024 102.4 1024 0 921.6 0 512Z")

FIT_BOX = {  # (max width, max height) of the glyph outline, px
    "square": (600, 600),
    "circle": (640, 640),
    "portrait": (560, 720),
    "landscape": (720, 560),
}
NARROW_ASPECT = 0.72
BLOCKY_FILL = 0.84
MAIN_STROKE_PX = 64

BADGE_SCALE = 12          # Lucide 24 units -> 288px box, 240px live area
BADGE_BOX = 616           # box top-left; live area 640..880
BADGE_STROKE_PX = 36
BADGE_GAP_PX = 52

LUCIDE_URL = "https://unpkg.com/lucide-static/icons/{}.svg"
OUT_DIR = pathlib.Path(__file__).resolve().parent.parent / "static/img/resource"

NUM = r"[-+]?(?:\d*\.\d+|\d+\.?)(?:[eE][-+]?\d+)?"


class Glyph:
    """A Lucide glyph: its inner markup and sampled outline (24-unit space)."""

    def __init__(self, name):
        self.name = name
        try:
            with urllib.request.urlopen(LUCIDE_URL.format(name), timeout=20) as r:
                svg = r.read().decode()
        except urllib.error.URLError as e:
            sys.exit(f"Error: Lucide icon '{name}' not found ({e}). "
                     "Browse available icons at https://lucide.dev/icons/")
        svg = re.sub(r"<!--.*?-->", "", svg, flags=re.S)
        svg = re.sub(r".*?<svg[^>]*>", "", svg, count=1, flags=re.S).replace("</svg>", "")
        self.markup = "\n".join(l.strip() for l in svg.strip().splitlines())
        self.points = Outline(self.markup).points

    def bbox(self):
        xs = [p[0] for p in self.points]
        ys = [p[1] for p in self.points]
        return min(xs), min(ys), max(xs), max(ys)

    def hull_fill(self):
        """Convex-hull area / bbox area: ~1 blocky, ~0.79 circle, less for diagonals."""
        pts = sorted(set(self.points))

        def cross(o, a, b):
            return (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0])

        lo, hi = [], []
        for p in pts:
            while len(lo) >= 2 and cross(lo[-2], lo[-1], p) <= 0:
                lo.pop()
            lo.append(p)
        for p in reversed(pts):
            while len(hi) >= 2 and cross(hi[-2], hi[-1], p) <= 0:
                hi.pop()
            hi.append(p)
        h = lo[:-1] + hi[:-1]
        area = abs(sum(h[i][0] * h[(i + 1) % len(h)][1] - h[(i + 1) % len(h)][0] * h[i][1]
                       for i in range(len(h)))) / 2
        x0, y0, x1, y1 = self.bbox()
        return area / ((x1 - x0) * (y1 - y0))

    def classify(self):
        x0, y0, x1, y1 = self.bbox()
        aspect = (x1 - x0) / (y1 - y0)
        if aspect < NARROW_ASPECT:
            return "portrait"
        if aspect > 1 / NARROW_ASPECT:
            return "landscape"
        return "square" if self.hull_fill() >= BLOCKY_FILL else "circle"

    def placement(self, fit):
        """-> (scale, tx, ty, width, height) fitting the outline into FIT_BOX[fit]."""
        x0, y0, x1, y1 = self.bbox()
        bw, bh = FIT_BOX[fit]
        scale = min(bw / (x1 - x0), bh / (y1 - y0))
        return (scale, S / 2 - scale * (x0 + x1) / 2, S / 2 - scale * (y0 + y1) / 2,
                scale * (x1 - x0), scale * (y1 - y0))


class Outline:
    """Samples points along every shape of a Lucide glyph (no dependencies)."""

    def __init__(self, markup):
        self.points = []
        for tag in re.findall(r"<(?:path|circle|ellipse|rect|line|polyline|polygon)\b[^>]*>", markup):
            kind = re.match(r"<(\w+)", tag).group(1)
            a = dict(re.findall(r'([\w-]+)="([^"]*)"', tag))
            getattr(self, "_" + kind)(a)

    @staticmethod
    def _f(a, k, d=0.0):
        return float(a.get(k, d))

    def _circle(self, a):
        self._ellipse(a, r=self._f(a, "r"))

    def _ellipse(self, a, r=None):
        rx = r if r is not None else self._f(a, "rx")
        ry = r if r is not None else self._f(a, "ry")
        cx, cy = self._f(a, "cx"), self._f(a, "cy")
        self.points += [(cx + rx * math.cos(t * math.pi / 32), cy + ry * math.sin(t * math.pi / 32))
                        for t in range(64)]

    def _rect(self, a):
        x0, y0, w, h = self._f(a, "x"), self._f(a, "y"), self._f(a, "width"), self._f(a, "height")
        r = min(self._f(a, "rx", a.get("ry", 0)), w / 2, h / 2)
        for cx, cy, sx, sy in ((x0 + r, y0 + r, -1, -1), (x0 + w - r, y0 + r, 1, -1),
                               (x0 + r, y0 + h - r, -1, 1), (x0 + w - r, y0 + h - r, 1, 1)):
            self.points += [(cx + sx * r * math.cos(t * math.pi / 16), cy + sy * r * math.sin(t * math.pi / 16))
                            for t in range(9)]

    def _line(self, a):
        self.points += [(self._f(a, "x1"), self._f(a, "y1")), (self._f(a, "x2"), self._f(a, "y2"))]

    def _polyline(self, a):
        nums = [float(v) for v in re.findall(NUM, a.get("points", ""))]
        self.points += list(zip(nums[::2], nums[1::2]))

    _polygon = _polyline

    def _path(self, a, n=16):
        toks = re.findall(rf"[A-Za-z]|{NUM}", a["d"])
        pts = self.points
        i, cmd = 0, None
        x = y = sx = sy = 0.0
        lc = None  # previous control point, for S/T reflection

        def num():
            nonlocal i
            i += 1
            return float(toks[i - 1])

        def flag():  # arc flags may be packed: "a2 2 0 011 1"
            nonlocal i
            t = toks[i]
            if len(t) > 1 and t[0] in "01" and t[1] != ".":
                toks[i] = t[1:]
                return int(t[0])
            i += 1
            return int(float(t))

        while i < len(toks):
            if toks[i].isalpha():
                cmd = toks[i]
                i += 1
                if cmd in "Zz":
                    x, y, lc = sx, sy, None
                    pts.append((x, y))
                    continue
            C = cmd.upper()
            ox, oy = (x, y) if cmd.islower() else (0.0, 0.0)
            if C == "M":
                x, y = ox + num(), oy + num()
                sx, sy, lc = x, y, None
                pts.append((x, y))
                cmd = "l" if cmd.islower() else "L"
            elif C == "L":
                x, y, lc = ox + num(), oy + num(), None
                pts.append((x, y))
            elif C == "H":
                x, lc = ox + num(), None
                pts.append((x, y))
            elif C == "V":
                y, lc = oy + num(), None
                pts.append((x, y))
            elif C in "CS":
                c1 = (ox + num(), oy + num()) if C == "C" else ((2 * x - lc[0], 2 * y - lc[1]) if lc else (x, y))
                c2 = (ox + num(), oy + num())
                e = (ox + num(), oy + num())
                for k in range(1, n + 1):
                    t, u = k / n, 1 - k / n
                    pts.append((u ** 3 * x + 3 * u * u * t * c1[0] + 3 * u * t * t * c2[0] + t ** 3 * e[0],
                                u ** 3 * y + 3 * u * u * t * c1[1] + 3 * u * t * t * c2[1] + t ** 3 * e[1]))
                lc, (x, y) = c2, e
            elif C in "QT":
                c1 = (ox + num(), oy + num()) if C == "Q" else ((2 * x - lc[0], 2 * y - lc[1]) if lc else (x, y))
                e = (ox + num(), oy + num())
                for k in range(1, n + 1):
                    t, u = k / n, 1 - k / n
                    pts.append((u * u * x + 2 * u * t * c1[0] + t * t * e[0],
                                u * u * y + 2 * u * t * c1[1] + t * t * e[1]))
                lc, (x, y) = c1, e
            elif C == "A":
                rx, ry, phi = num(), num(), num()
                fa, fs = flag(), flag()
                ex, ey = ox + num(), oy + num()
                pts += self._arc(x, y, rx, ry, phi, fa, fs, ex, ey)
                x, y, lc = ex, ey, None
            else:
                raise ValueError(f"unsupported path command {cmd!r}")

    @staticmethod
    def _arc(x1, y1, rx, ry, phi, fa, fs, x2, y2, n=24):
        """Endpoint arc -> sample points (SVG 1.1 implementation notes F.6.5)."""
        if rx == 0 or ry == 0:
            return [(x2, y2)]
        rx, ry = abs(rx), abs(ry)
        c, s = math.cos(math.radians(phi)), math.sin(math.radians(phi))
        dx, dy = (x1 - x2) / 2, (y1 - y2) / 2
        xp, yp = c * dx + s * dy, -s * dx + c * dy
        lam = xp ** 2 / rx ** 2 + yp ** 2 / ry ** 2
        if lam > 1:
            rx, ry = rx * math.sqrt(lam), ry * math.sqrt(lam)
        den = rx ** 2 * yp ** 2 + ry ** 2 * xp ** 2
        k = math.sqrt(max(0.0, (rx ** 2 * ry ** 2 - den) / den)) if den else 0.0
        if fa == fs:
            k = -k
        cxp, cyp = k * rx * yp / ry, -k * ry * xp / rx
        cx, cy = c * cxp - s * cyp + (x1 + x2) / 2, s * cxp + c * cyp + (y1 + y2) / 2

        def ang(ux, uy, vx, vy):
            return math.atan2(ux * vy - uy * vx, ux * vx + uy * vy)

        t1 = ang(1, 0, (xp - cxp) / rx, (yp - cyp) / ry)
        dt = ang((xp - cxp) / rx, (yp - cyp) / ry, (-xp - cxp) / rx, (-yp - cyp) / ry)
        if not fs and dt > 0:
            dt -= 2 * math.pi
        elif fs and dt < 0:
            dt += 2 * math.pi
        out = []
        for j in range(1, n + 1):
            t = t1 + dt * j / n
            ex, ey = rx * math.cos(t), ry * math.sin(t)
            out.append((c * ex - s * ey + cx, s * ex + c * ey + cy))
        return out


def render(main, color, badge=None, fit=None):
    fit = fit or main.classify()
    scale, tx, ty, _, _ = main.placement(fit)
    glyph_attrs = ('fill="none" color="{c}" stroke="{c}" stroke-width="{w:.4g}" '
                   'stroke-linecap="round" stroke-linejoin="round"')
    badge_tf = f"translate({BADGE_BOX} {BADGE_BOX}) scale({BADGE_SCALE})"
    out = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {S} {S}">',
           # Keeps the icon's bounds at the full canvas when ungrouped in Office.
           f'<rect width="{S}" height="{S}" fill="none"/>',
           f'<path d="{SQUIRCLE}" fill="{color}" fill-opacity="0.2"/>']
    mask = ""
    if badge:
        gap_w = (BADGE_STROKE_PX + 2 * BADGE_GAP_PX) / BADGE_SCALE
        out.append(f'<defs><mask id="badge-gap" maskUnits="userSpaceOnUse" x="0" y="0" width="{S}" height="{S}">'
                   f'<rect width="{S}" height="{S}" fill="#fff"/>'
                   f'<g transform="{badge_tf}" fill="none" stroke="#000" stroke-width="{gap_w:.4g}" '
                   f'stroke-linecap="round" stroke-linejoin="round">\n{badge.markup}\n</g></mask></defs>')
        mask = ' mask="url(#badge-gap)"'
    out.append(f'<g{mask}><g transform="translate({tx:.2f} {ty:.2f}) scale({scale:.4f})" '
               + glyph_attrs.format(c=color, w=MAIN_STROKE_PX / scale) + f'>\n{main.markup}\n</g></g>')
    if badge:
        out.append(f'<g transform="{badge_tf}" '
                   + glyph_attrs.format(c=color, w=BADGE_STROKE_PX / BADGE_SCALE) + f'>\n{badge.markup}\n</g>')
    out.append("</svg>\n")
    return "\n".join(out), fit


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("lucide", help="Lucide icon name, see https://lucide.dev/icons/")
    ap.add_argument("color", help="accent color, 6-digit hex like '#FFC000'")
    ap.add_argument("output", help="output name under static/img/resource/, e.g. Bicycle or cube/KeyDimension")
    ap.add_argument("--badge", metavar="LUCIDE", help="series badge glyph at the bottom right")
    ap.add_argument("--fit", choices=FIT_BOX, help="override the shape detected for the main glyph")
    args = ap.parse_args()

    if not re.fullmatch(r"#[0-9A-Fa-f]{6}", args.color):
        sys.exit(f"Error: color must be a 6-digit hex value like '#FFC000', got '{args.color}'")
    out_file = OUT_DIR / f"{args.output}.svg"
    if not out_file.parent.is_dir():
        sys.exit(f"Error: folder {out_file.parent} does not exist (namespace folders must match a declared prefix)")

    glyph = Glyph(args.lucide)
    svg, fit = render(glyph, args.color, Glyph(args.badge) if args.badge else None, args.fit)
    out_file.write_text(svg)
    w, h = glyph.placement(fit)[3:]
    how = "override" if args.fit else "detected"
    print(f"Wrote {out_file.relative_to(OUT_DIR.parent.parent.parent)} "
          f"(lucide: {args.lucide}, fit: {fit} [{how}], {w:.0f}x{h:.0f}px"
          + (f", badge: {args.badge}" if args.badge else "") + f", color: {args.color})")


if __name__ == "__main__":
    main()

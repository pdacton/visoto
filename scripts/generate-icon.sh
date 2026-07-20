#!/usr/bin/env bash
# Generate a Visoto resource icon: tinted squircle + Lucide icon in an accent color.
# See .claude/skills/iconGeneration/SKILL.md for the palette and conventions.
#
# Usage: ./scripts/generate-icon.sh <lucide-name> <color> <OutputName>
# Example: ./scripts/generate-icon.sh bike '#FFC000' Bicycle
#   -> static/img/resource/Bicycle.svg
set -euo pipefail

if [ $# -ne 3 ]; then
	echo "Usage: $0 <lucide-name> <color> <OutputName>" >&2
	echo "Example: $0 bike '#FFC000' Bicycle" >&2
	exit 1
fi

lucide_name="$1"
color="$2"
output_name="$3"

if ! [[ "$color" =~ ^#[0-9A-Fa-f]{6}$ ]]; then
	echo "Error: color must be a 6-digit hex value like '#FFC000', got '$color'" >&2
	exit 1
fi

out_dir="$(dirname "$0")/../static/img/resource"
out_file="$out_dir/$output_name.svg"

lucide_url="https://unpkg.com/lucide-static/icons/$lucide_name.svg"
if ! lucide_svg=$(curl -fsSL "$lucide_url"); then
	echo "Error: Lucide icon '$lucide_name' not found ($lucide_url)" >&2
	echo "Browse available icons at https://lucide.dev/icons/" >&2
	exit 1
fi

# Keep only the elements inside the <svg> wrapper (drop license comment and svg tag)
inner=$(printf '%s' "$lucide_svg" | tr '\n' ' ' \
	| sed -e 's/<!--[^>]*-->//g' -e 's/.*<svg[^>]*>//' -e 's|</svg>.*||' \
	| sed -e 's/> *</>\n</g' -e 's/^ *//' -e 's/ *$//')

squircle="M0 256C0 51.2 51.2 0 256 0 460.8 0 512 51.2 512 256 512 460.8 460.8 512 256 512 51.2 512 0 460.8 0 256"

# The invisible 24x24 rect keeps the icon group's bounds intact when the SVG
# is ungrouped in PowerPoint/Office (otherwise bounds collapse to the glyph).
cat > "$out_file" <<EOF
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
<path d="$squircle" fill="$color" fill-opacity="0.2"/>
<g transform="translate(52 52) scale(17)" fill="none" stroke="$color" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
<rect width="24" height="24" fill="none" stroke="none"/>
$inner
</g>
</svg>
EOF

echo "Wrote $out_file (lucide: $lucide_name, color: $color)"

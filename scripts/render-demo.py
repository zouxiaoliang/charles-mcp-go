#!/usr/bin/env python3
"""Render verified demo output; optional dependency: Pillow."""

import json
from pathlib import Path
import textwrap

from PIL import Image, ImageDraw, ImageFont


ROOT = Path(__file__).resolve().parents[1]
ASSETS = ROOT / "docs" / "assets"
STAGES = json.loads((ASSETS / "demo-transcript.json").read_text())


def font(size):
    for path in (
        "/System/Library/Fonts/Menlo.ttc",
        "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
        "C:/Windows/Fonts/consola.ttf",
    ):
        if Path(path).exists():
            return ImageFont.truetype(path, size)
    return ImageFont.load_default(size=size)


frames = []
labels = ["Import recording", "Find failed login", "Decode response", "Replay + compare"]
for index, stage in enumerate(STAGES):
    frame = Image.new("RGB", (1120, 640), "#101921")
    draw = ImageDraw.Draw(frame)
    draw.text((42, 30), "CHARLES MCP   /   VERIFIED TOOL RUN", font=font(17), fill="#62dcc1")
    draw.text((42, 76), "Inspect. Change. Verify.", font=font(42), fill="#f0f5fa")
    draw.text((44, 139), "A failed login, explained and replayed through MCP.", font=font(18), fill="#adbdca")
    for n, label in enumerate(labels):
        y = 227 + n * 65
        active = n == index
        if active:
            draw.rounded_rectangle((30, y - 14, 327, y + 39), radius=9, fill="#203a40")
        color = "#62dcc1" if n <= index else "#71828f"
        draw.text((45, y), f"{n + 1:02d}", font=font(22), fill=color)
        draw.text((94, y + 3), label, font=font(17), fill="#f0f5fa" if active else "#adbdca")
    draw.rounded_rectangle((350, 202, 1080, 528), radius=14, fill="#192630", outline="#35434e", width=1)
    draw.text((380, 228), stage["title"], font=font(22), fill="#f0f5fa")
    draw.text((380, 279), "> " + stage["tool"], font=font(20), fill="#62dcc1")
    y = 336
    for line in stage["lines"]:
        for wrapped in textwrap.wrap(line, width=52):
            draw.text((380, y), wrapped, font=font(19), fill="#e1eaf1")
            y += 29
        y += 10
    draw.text((44, 562), "LOCAL DEMO API  |  SAMPLE DATA  |  REAL MCP CALLS", font=font(17), fill="#62dcc1")
    draw.text((44, 598), "github.com/zouxiaoliang/charles-mcp-go", font=font(16), fill="#adbdca")
    frames.append(frame)

frames[-1].save(ASSETS / "demo.png")
frames[0].save(
    ASSETS / "demo.gif", save_all=True, append_images=frames[1:],
    duration=8000, loop=0, optimize=True,
)
print(f"Rendered {len(frames)} verified stages ({len(frames) * 8}s)")

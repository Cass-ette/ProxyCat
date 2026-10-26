#!/usr/bin/env python3
"""Prepare the selected generated icon for macOS. Requires Pillow, numpy, scipy.

Source: gpt-image-2.5-sunburst through the user's imagegen provider. The prompt
is in prompt.txt. This only removes the exterior white matte and sizes assets.
"""
from pathlib import Path
import numpy as np
from PIL import Image, ImageFilter
from scipy import ndimage

ROOT = Path(__file__).resolve().parents[2]
RESOURCES = ROOT / "app/ProxyCat/ProxyCat/Resources"
ASSETS = ROOT / "app/ProxyCat/ProxyCat/Assets.xcassets/AppIcon.appiconset"
source = Image.open(RESOURCES / "ProxyCatIcon-source.png").convert("RGB")
rgb = np.asarray(source).astype(np.int16)

# The ivory tile is warm; the exterior white matte and gray contact shadow
# are neutral. Fill its internal black mark so only the exterior is keyed.
warm = (rgb[:, :, 0] - rgb[:, :, 2] > 8) & (rgb[:, :, 0] < 254)
labels, _ = ndimage.label(warm)
areas = np.bincount(labels.ravel())
areas[0] = 0
tile = ndimage.binary_fill_holes(labels == areas.argmax())
tile = ndimage.binary_closing(tile, iterations=2)
ys, xs = np.where(tile)
mask = Image.fromarray((tile * 255).astype(np.uint8)).filter(ImageFilter.GaussianBlur(0.6))
coverage = np.asarray(mask).astype(float) / 255

# Unmatte the neutral exterior into a black translucent shadow. This avoids
# a rectangular white canvas or white fringe on a dark Dock or desktop.
shadow = 255 - np.min(rgb, axis=2)
shadow[shadow < 7] = 0
alpha = coverage + (1 - coverage) * shadow / 255
color = np.divide(rgb * coverage[:, :, None], alpha[:, :, None],
                  out=np.zeros_like(rgb, dtype=float), where=alpha[:, :, None] > 0)
rgba = np.dstack((np.clip(color, 0, 255).astype(np.uint8), (alpha * 255).astype(np.uint8)))
cutout = Image.fromarray(rgba)

# Standard macOS icon margin: an approximately 824px tile on a 1024px canvas.
factor = 824 / max(xs.max() - xs.min() + 1, ys.max() - ys.min() + 1)
scaled = cutout.resize((round(source.width * factor), round(source.height * factor)), Image.Resampling.LANCZOS)
master = Image.new("RGBA", (1024, 1024))
offset = (round(512 - (xs.min() + xs.max()) / 2 * factor), round(512 - (ys.min() + ys.max()) / 2 * factor))
master.paste(scaled, offset)
master.save(RESOURCES / "ProxyCatIcon-1024.png")
for points in (16, 32, 128, 256, 512):
    for scale in (1, 2):
        pixels = points * scale
        master.resize((pixels, pixels), Image.Resampling.LANCZOS).save(ASSETS / f"icon_{points}x{points}@{scale}x.png")
print("Prepared transparent macOS AppIcon assets from the selected generated source.")

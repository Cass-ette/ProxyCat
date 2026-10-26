#!/usr/bin/env python3
"""Build the public updater metadata from this release's verified packages."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re


def generate(tag, repository, notes, dist):
    if not re.fullmatch(r"v\d+\.\d+\.\d+", tag):
        raise ValueError("Expected a stable version tag")
    if not re.fullmatch(r"[\w.-]+/[\w.-]+", repository):
        raise ValueError("Expected owner/repository")
    version = tag[1:]
    base = f"https://github.com/{repository}/releases"
    assets = []
    for suffix in (".dmg", "-installer.zip"):
        package = dist / f"ProxyCat-{version}{suffix}"
        sidecar = package.with_name(package.name + ".sha256")
        expected = sidecar.read_text().split()[0]
        if hashlib.sha256(package.read_bytes()).hexdigest() != expected:
            raise ValueError(f"Checksum mismatch: {package.name}")
        for path in (package, sidecar):
            assets.append({"name": path.name,
                           "browser_download_url": f"{base}/download/{tag}/{path.name}",
                           "size": path.stat().st_size})
    return {"tag_name": tag, "body": notes.read_text(encoding="utf-8"),
            "html_url": f"{base}/tag/{tag}",
            "published_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "assets": assets}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("repository")
    parser.add_argument("notes", type=Path)
    parser.add_argument("dist", type=Path)
    args = parser.parse_args()
    manifest = generate(args.tag, args.repository, args.notes, args.dist)
    (args.dist / "update.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

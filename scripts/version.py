#!/usr/bin/env python3
"""Read, check or bump ProxyCat's version in its single source, Info.plist."""

import argparse
from pathlib import Path
import plistlib
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent
INFO = Path("app/ProxyCat/ProxyCat/Info.plist")
PROJECT = Path("app/ProxyCat/ProxyCat.xcodeproj/project.pbxproj")
VERSION_PATTERN = r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"


def read_version(root=ROOT):
    info = plistlib.loads((root / INFO).read_bytes())
    version = info["CFBundleShortVersionString"]
    if not re.fullmatch(VERSION_PATTERN, version):
        raise ValueError("Version must be MAJOR.MINOR.PATCH, without a v prefix or leading zeros")
    build = info.get("CFBundleVersion", "")
    if not re.fullmatch(r"[1-9][0-9]*", build):
        raise ValueError("CFBundleVersion must be a positive, increasing integer")
    return version, int(build)


def check(root=ROOT, tag=None, previous_tags=()):
    version, build = read_version(root)
    project = (root / PROJECT).read_text()
    if re.search(r"\b(?:MARKETING_VERSION|CURRENT_PROJECT_VERSION)\s*=", project):
        raise ValueError("Keep version numbers only in Info.plist; remove Xcode project overrides")
    notes = root / "docs/releases" / f"v{version}.md"
    if not notes.is_file() or not notes.read_text(encoding="utf-8").strip():
        raise ValueError(f"Missing release notes: {notes.relative_to(root)}")
    if tag is not None:
        if tag != "v" + version:
            raise ValueError(f"Release tag {tag} does not match app version v{version}")
        current = tuple(map(int, version.split(".")))
        for previous in previous_tags:
            if previous != tag and re.fullmatch("v" + VERSION_PATTERN, previous):
                if tuple(map(int, previous[1:].split("."))) >= current:
                    raise ValueError(f"Release {tag} must be newer than {previous}")
    return version, build


def bump(level, root=ROOT):
    version, build = read_version(root)
    parts = list(map(int, version.split(".")))
    index = {"major": 0, "minor": 1, "patch": 2}[level]
    parts[index] += 1
    for i in range(index + 1, 3):
        parts[i] = 0
    updated = ".".join(map(str, parts))
    path = root / INFO
    text = path.read_text()
    for key, value in (("CFBundleShortVersionString", updated), ("CFBundleVersion", str(build + 1))):
        text, count = re.subn(r"(<key>" + key + r"</key>\s*<string>)[^<]*(</string>)",
                              lambda m: m[1] + value + m[2], text)
        if count != 1:
            raise ValueError(f"Expected exactly one {key}")
    path.write_text(text)
    return updated, build + 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("show")
    validate = commands.add_parser("check")
    validate.add_argument("--tag")
    change = commands.add_parser("bump")
    change.add_argument("level", choices=("major", "minor", "patch"))
    args = parser.parse_args()
    if args.command == "bump":
        version, build = bump(args.level)
        print(f"{version} (build {build}); add docs/releases/v{version}.md before building")
    elif args.command == "check":
        tags = []
        if args.tag:
            tags = subprocess.check_output(["git", "tag", "--list", "v*"], cwd=ROOT, text=True).splitlines()
        version, build = check(tag=args.tag, previous_tags=tags)
        print(f"Version {version} (build {build}) is consistent")
    else:
        print(read_version()[0])


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        print(f"Version check failed: {error}", file=sys.stderr)
        sys.exit(1)

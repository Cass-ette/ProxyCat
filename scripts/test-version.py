#!/usr/bin/env python3
import importlib.util
from pathlib import Path
import plistlib
import sys
import tempfile
import unittest

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("version", Path(__file__).with_name("version.py"))
version = importlib.util.module_from_spec(spec)
spec.loader.exec_module(version)


class VersionTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        (self.root / version.INFO).parent.mkdir(parents=True)
        (self.root / version.PROJECT).parent.mkdir(parents=True)
        (self.root / version.PROJECT).write_text("{ buildSettings = {}; }")
        self.write_info("0.12.4", "7")
        notes = self.root / "docs/releases/v0.12.4.md"
        notes.parent.mkdir(parents=True)
        notes.write_text("## 修复\n- 更新检查\n", encoding="utf-8")

    def write_info(self, text, build):
        (self.root / version.INFO).write_bytes(plistlib.dumps({
            "CFBundleShortVersionString": text, "CFBundleVersion": build}))

    def test_canonical_version_matches_release_tag(self):
        self.assertEqual(version.check(self.root, "v0.12.4", ["v0.12.3", "v0.12.4"]), ("0.12.4", 7))

    def test_bumps_reset_lower_components_and_increment_build(self):
        for level, expected in (("patch", "0.12.5"), ("minor", "0.13.0"), ("major", "1.0.0")):
            with self.subTest(level=level):
                self.write_info("0.12.4", "7")
                self.assertEqual(version.bump(level, self.root), (expected, 8))
                self.assertEqual(version.read_version(self.root), (expected, 8))

    def test_rejects_mismatched_tag_and_version_rollback(self):
        for tag, tags in (("v0.13.0", []), ("v0.12.4", ["v0.13.0"])):
            with self.subTest(tag=tag, tags=tags), self.assertRaises(ValueError):
                version.check(self.root, tag, tags)

    def test_rejects_duplicate_project_versions(self):
        for setting in ("MARKETING_VERSION", "CURRENT_PROJECT_VERSION"):
            with self.subTest(setting=setting):
                (self.root / version.PROJECT).write_text(setting + " = 0.12.4;")
                with self.assertRaisesRegex(ValueError, "only in Info.plist"):
                    version.check(self.root)

    def test_requires_bundled_release_notes(self):
        (self.root / "docs/releases/v0.12.4.md").unlink()
        with self.assertRaisesRegex(ValueError, "Missing release notes"):
            version.check(self.root)

    def test_rejects_ambiguous_version_formats(self):
        for text in ("v0.12.4", "0.12", "0.12.04", "0.13.0-beta.1"):
            with self.subTest(version=text):
                self.write_info(text, "7")
                with self.assertRaises(ValueError):
                    version.read_version(self.root)


if __name__ == "__main__":
    unittest.main()

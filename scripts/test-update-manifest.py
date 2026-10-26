#!/usr/bin/env python3
import hashlib
import importlib.util
from pathlib import Path
import tempfile
import unittest
import sys

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("manifest", Path(__file__).with_name("generate-update-manifest.py"))
manifest = importlib.util.module_from_spec(spec)
spec.loader.exec_module(manifest)


class ManifestTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.dist = Path(self.temporary.name)
        self.notes = self.dist / "notes.md"
        self.notes.write_text("## 修复\n\n- 更新检查\n", encoding="utf-8")
        for suffix in (".dmg", "-installer.zip"):
            package = self.dist / ("ProxyCat-0.12.4" + suffix)
            package.write_bytes(b"package contents")
            package.with_name(package.name + ".sha256").write_text(
                hashlib.sha256(package.read_bytes()).hexdigest() + "  " + package.name + "\n")

    def generate(self):
        return manifest.generate("v0.12.4", "Cass-ette/ProxyCat", self.notes, self.dist)

    def test_release_contains_reviewable_notes_and_matched_downloads(self):
        release = self.generate()
        self.assertEqual(release["body"], self.notes.read_text())
        self.assertEqual(release["tag_name"], "v0.12.4")
        self.assertEqual(release["html_url"], "https://github.com/Cass-ette/ProxyCat/releases/tag/v0.12.4")
        self.assertEqual(len(release["assets"]), 4)
        for asset in release["assets"]:
            self.assertEqual(asset["size"], (self.dist / asset["name"]).stat().st_size)
            self.assertEqual(asset["browser_download_url"],
                             "https://github.com/Cass-ette/ProxyCat/releases/download/v0.12.4/" + asset["name"])

    def test_corrupt_package_cannot_be_published_in_manifest(self):
        (self.dist / "ProxyCat-0.12.4.dmg").write_bytes(b"corrupted")
        with self.assertRaisesRegex(ValueError, "Checksum mismatch"):
            self.generate()

    def test_missing_sidecar_cannot_be_published_in_manifest(self):
        (self.dist / "ProxyCat-0.12.4.dmg.sha256").unlink()
        with self.assertRaises(FileNotFoundError):
            self.generate()


if __name__ == "__main__":
    unittest.main()

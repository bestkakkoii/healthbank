#!/usr/bin/env python3
# Copyright 2026 bestkakkoii llc.
# SPDX-License-Identifier: MIT
"""Check deployable archive layout without requiring a compiler or real user data."""

from __future__ import annotations

import hashlib
from pathlib import Path, PurePosixPath
import subprocess
import sys
import tarfile
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("release.py")
EPOCH = 1700000000


def package(kind: str, root: Path, destination: Path, *extra: str) -> None:
    subprocess.run(
        [sys.executable, "-W", "error", str(SCRIPT), kind, str(root),
         str(destination), str(EPOCH), *extra],
        check=True,
    )


def write_fixture(root: Path, name: str, content: str) -> Path:
    path = root / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    path.chmod(0o644)
    return path


class ReleaseArchiveTests(unittest.TestCase):
    def test_binary_roots_and_existing_data(self) -> None:
        for arch in ("amd64", "arm64"):
            with self.subTest(arch=arch), tempfile.TemporaryDirectory() as temporary:
                base = Path(temporary)
                root = base / f"healthbank_test_linux_{arch}"
                executable = write_fixture(root, "healthbank", "synthetic executable\n")
                executable.chmod(0o755)
                write_fixture(root, "LICENSE", "synthetic license\n")
                write_fixture(root, "docs/privacy.zh-TW.md", "synthetic notice\n")
                write_fixture(root, "deploy/nginx.healthbank.conf", "synthetic config\n")
                destination = base / "binary.tar.gz"
                package("binary", root, destination)
                installed = base / "installed"
                preserved = write_fixture(installed, "data/settings.sqlite", "existing synthetic data\n")
                identity = write_fixture(installed, "data/identity.key", "existing synthetic key\n")
                with tarfile.open(destination, "r:gz") as archive:
                    self.assertEqual(set(archive.getnames()), {
                        "healthbank", "LICENSE", "docs/privacy.zh-TW.md",
                        "deploy/nginx.healthbank.conf",
                    })
                    for member in archive.getmembers():
                        path = PurePosixPath(member.name)
                        self.assertFalse(path.is_absolute())
                        self.assertNotIn("..", path.parts)
                        self.assertTrue(member.isfile())
                        self.assertEqual((member.uid, member.gid, member.mtime), (0, 0, EPOCH))
                    self.assertEqual(archive.getmember("healthbank").mode, 0o755)
                    archive.extractall(installed, filter="data")
                self.assertEqual((installed / "healthbank").read_text(), "synthetic executable\n")
                self.assertEqual((installed / "healthbank").stat().st_mode & 0o777, 0o755)
                self.assertFalse((installed / root.name).exists())
                self.assertEqual(preserved.read_text(), "existing synthetic data\n")
                self.assertEqual(identity.read_text(), "existing synthetic key\n")

    def test_reproducibility_and_checksums(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            root = base / "input"
            write_fixture(root, "healthbank", "synthetic executable\n").chmod(0o755)
            first, second = base / "healthbank_first.tar.gz", base / "healthbank_second.tar.gz"
            package("binary", root, first)
            package("binary", root, second)
            self.assertEqual(first.read_bytes(), second.read_bytes())
            subprocess.run(
                [sys.executable, "-W", "error", str(SCRIPT), "checksums", str(base), "healthbank_"],
                check=True,
            )
            expected = "".join(
                f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n"
                for path in (first, second)
            )
            self.assertEqual((base / "SHA256SUMS").read_text(), expected)

    def test_source_prefix_and_private_file_exclusions(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            root = base / "source"
            write_fixture(root, "README.md", "synthetic readme\n")
            write_fixture(root, "RELEASE_CANDIDATE", "v1.0.3-rc.7\n")
            write_fixture(root, "docs/example.md", "synthetic documentation\n")
            write_fixture(root, "data/settings.sqlite", "must stay private\n")
            write_fixture(root, "internal/identity.key", "must stay private\n")
            write_fixture(root, "docs/.repair/transfer.json", "temporary work\n")
            destination = base / "source.tar.gz"
            package("source", root, destination, "healthbank_source")
            with tarfile.open(destination, "r:gz") as archive:
                self.assertEqual(archive.getnames(), [
                    "healthbank_source/README.md", "healthbank_source/RELEASE_CANDIDATE",
                    "healthbank_source/docs/example.md",
                ])

    def test_binary_rejects_private_and_temporary_inputs(self) -> None:
        for name in ("data/settings.sqlite", "identity.key", ".env.production",
                     "docs/report.db", "logs/server.log", ".repair/transfer.json"):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temporary:
                base = Path(temporary)
                root = base / "input"
                write_fixture(root, "healthbank", "synthetic executable\n")
                write_fixture(root, name, "synthetic forbidden content\n")
                destination = base / "binary.tar.gz"
                result = subprocess.run(
                    [sys.executable, "-W", "error", str(SCRIPT), "binary", str(root),
                     str(destination), str(EPOCH)],
                    capture_output=True, text=True, check=False,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Private or temporary file", result.stderr)
                self.assertFalse(destination.exists())


if __name__ == "__main__":
    unittest.main(verbosity=2)

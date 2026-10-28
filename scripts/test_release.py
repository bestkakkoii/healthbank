#!/usr/bin/env python3
# Copyright 2026 bestkakkoii llc.
# SPDX-License-Identifier: MIT
"""Check deployable archive layout without requiring a compiler or real user data."""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import sys
import tarfile
import tempfile
import unittest

import release


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
            first, second = (base / f"healthbank_1.2.3_linux_{arch}.tar.gz" for arch in ("amd64", "arm64"))
            package("binary", root, first)
            package("binary", root, second)
            self.assertEqual(first.read_bytes(), second.read_bytes())
            source = base / "healthbank_1.2.3_source.tar.gz"
            source_root = base / "source"
            write_fixture(source_root, "README.md", "synthetic source\n")
            package("source", source_root, source, "healthbank_1.2.3")
            subprocess.run(
                [sys.executable, "-W", "error", str(SCRIPT), "checksums", str(base), "healthbank_1.2.3_"],
                check=True,
            )
            expected = "".join(
                f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n"
                for path in (first, second, source)
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
                self.assertIn("Private or temporary", result.stderr)
                self.assertFalse(destination.exists())

    def test_rejects_symlink_directories_files_and_special_files(self) -> None:
        for kind in ("source", "binary"):
            for special in ("directory", "file", "fifo"):
                with self.subTest(kind=kind, special=special), tempfile.TemporaryDirectory() as temporary:
                    base = Path(temporary)
                    root = base / "input"
                    write_fixture(root, "README.md", "synthetic\n")
                    outside = write_fixture(base, "outside/LICENSE", "synthetic\n")
                    if special == "directory":
                        (root / "licenses").symlink_to(outside.parent, target_is_directory=True)
                    elif special == "file":
                        (root / "LICENSE").symlink_to(outside)
                    else:
                        os.mkfifo(root / "LICENSE")
                    destination = base / "output.tar.gz"
                    result = subprocess.run(
                        [sys.executable, "-W", "error", str(SCRIPT), kind, str(root),
                         str(destination), str(EPOCH), "healthbank_source"],
                        capture_output=True, text=True, check=False, timeout=5,
                    )
                    self.assertNotEqual(result.returncode, 0)
                    self.assertRegex(result.stderr, "symlinks|special files")
                    self.assertFalse(destination.exists())

    def test_checksum_set_is_exact_and_regular(self) -> None:
        for invalid in ("missing", "extra", "symlink"):
            with self.subTest(invalid=invalid), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                for name in ("linux_amd64", "linux_arm64", "source"):
                    write_fixture(root, f"healthbank_1.2.3_{name}.tar.gz", "synthetic archive\n")
                if invalid == "missing":
                    (root / "healthbank_1.2.3_source.tar.gz").unlink()
                elif invalid == "extra":
                    write_fixture(root, "healthbank_1.2.3_stale.tar.gz", "stale\n")
                else:
                    (root / "healthbank_1.2.3_source.tar.gz").unlink()
                    (root / "healthbank_1.2.3_source.tar.gz").symlink_to(root / "healthbank_1.2.3_linux_amd64.tar.gz")
                with self.assertRaises(ValueError):
                    release.checksums(root, "healthbank_1.2.3_")
                self.assertFalse((root / "SHA256SUMS").exists())

    def test_source_prefix_cannot_escape_or_add_nested_roots(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            write_fixture(root, "README.md", "synthetic\n")
            for prefix in ("../outside", "/absolute", "nested/root", "..\\outside", ".."):
                with self.subTest(prefix=prefix), self.assertRaises(ValueError):
                    release.create_archive("source", root, root / "output.tar.gz", EPOCH, prefix)


class ReleaseProvenanceTests(unittest.TestCase):
    def git(self, root: Path, *args: str) -> str:
        environment = os.environ.copy()
        environment.update(GIT_AUTHOR_DATE=f"@{EPOCH} +0000", GIT_COMMITTER_DATE=f"@{EPOCH} +0000")
        return subprocess.check_output(
            ["git", "-C", str(root), "-c", "core.hooksPath=/dev/null",
             "-c", "user.name=Synthetic release fixture", "-c", "user.email=synthetic@example.invalid",
             *args], env=environment, text=True, stderr=subprocess.DEVNULL,
        ).strip()

    def fixture(self, root: Path) -> str:
        write_fixture(root, "README.md", "synthetic committed source\n")
        write_fixture(root, "internal/essential.go", "package synthetic\n")
        self.git(root, "init", "-q")
        self.git(root, "add", ".")
        self.git(root, "commit", "-qm", "synthetic committed fixture")
        return self.git(root, "rev-parse", "HEAD")

    def test_identity_comes_from_clean_committed_source(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            head = self.fixture(root)
            self.assertEqual(release.release_identity(root), (head, str(EPOCH)))
            self.assertEqual(release.release_identity(root, "HEAD", str(EPOCH)), (head, str(EPOCH)))
            self.assertEqual(release.release_identity(root, head[:12]), (head, str(EPOCH)))

    def test_rejects_dirty_staged_untracked_ignored_and_assumed_unchanged_source(self) -> None:
        for change in ("dirty", "staged", "untracked", "ignored", "assume-unchanged", "skip-worktree-deleted"):
            with self.subTest(change=change), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                self.fixture(root)
                if change in {"dirty", "staged", "assume-unchanged"}:
                    if change == "assume-unchanged":
                        self.git(root, "update-index", "--assume-unchanged", "README.md")
                    write_fixture(root, "README.md", "synthetic changed source\n")
                    if change == "staged":
                        self.git(root, "add", "README.md")
                elif change == "skip-worktree-deleted":
                    self.git(root, "update-index", "--skip-worktree", "internal/essential.go")
                    (root / "internal/essential.go").unlink()
                else:
                    write_fixture(root, "docs/uncommitted.md", "synthetic uncommitted source\n")
                    if change == "ignored":
                        (root / ".git/info/exclude").write_text("docs/uncommitted.md\n")
                with self.assertRaisesRegex(ValueError, "clean Git|not committed|differs from HEAD|missing from the working tree"):
                    release.release_identity(root)

    def test_rejects_false_commit_and_timestamp(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            old = self.fixture(root)
            self.git(root, "commit", "--allow-empty", "-qm", "synthetic next commit")
            for commit in (old, "0" * 40, "invented-label"):
                with self.subTest(commit=commit), self.assertRaises(ValueError):
                    release.release_identity(root, commit)
            for epoch in ("1", "tomorrow", "١٧٠٠٠٠٠٠٠٠"):
                with self.subTest(epoch=epoch), self.assertRaisesRegex(ValueError, "HEAD commit timestamp"):
                    release.release_identity(root, source_epoch=epoch)

    def test_official_packaging_requires_its_own_git_root(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            with self.assertRaisesRegex(ValueError, "Git checkout"):
                release.release_identity(root)
            self.fixture(root)
            nested = root / "source-archive"
            nested.mkdir()
            with self.assertRaisesRegex(ValueError, "actual Git checkout root"):
                release.release_identity(nested)

    def test_required_license_files_and_actual_compiler_revision(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            document = {
                "go_runtime_modules": [{"license_files": ["licenses/example/AUTHORS"]}],
                "sources": [{"name": "Go", "version": "devel go1.28", "commit": "a" * 40}],
            }
            write_fixture(root, "dependencies.lock.json", json.dumps(document))
            for name in release.REQUIRED_LICENSE_FILES | {"licenses/example/AUTHORS"}:
                write_fixture(root, name, "synthetic attribution\n")
            release.validate_licenses(root)
            release.validate_go_version(root, "go version devel go1.28-" + "a" * 40 + " linux/amd64")
            for version in ("go version go1.27.3 linux/amd64", "go version devel go1.28-" + "b" * 40 + " linux/amd64"):
                with self.subTest(version=version), self.assertRaisesRegex(ValueError, "compiler does not match"):
                    release.validate_go_version(root, version)
            (root / "licenses/example/AUTHORS").unlink()
            with self.assertRaisesRegex(ValueError, "attribution is missing"):
                release.validate_licenses(root)

    def test_semantic_version_identifiers(self) -> None:
        for value in ("1.2.3", "v1.2.3-rc.1", "v1.2.3+build-01", "v1.2.3-alpha.0+metadata"):
            with self.subTest(value=value):
                self.assertEqual(release.normalize_version(value), "v" + value.removeprefix("v"))
        for value in ("v01.2.3", "v1.2.3-rc.01", "v1.2.3-..", "v1.2.3-", "v1.2.3+", "v1.2.3\n"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                release.normalize_version(value)


if __name__ == "__main__":
    unittest.main(verbosity=2)

#!/usr/bin/env python3
# Copyright 2026 bestkakkoi llc.
# SPDX-License-Identifier: MIT
"""Deterministic local release archives. Python is a build-time dependency only."""

from __future__ import annotations

import gzip
import hashlib
import os
from pathlib import Path
import sys
import tarfile

SOURCE_ROOT_FILES = {
    "go.mod", "go.sum", "LICENSE", "README.md", "README.zh-TW.md",
    "README_AI.md", "AGENTS.md", "package.json", "package-lock.json",
    "CHANGELOG.md", "CONTRIBUTING.md", "SECURITY.md", "NOTICE",
    "THIRD_PARTY_NOTICES.md", "DEPENDENCIES.md", "dependencies.lock.json",
    "Makefile", ".gitignore", ".gitattributes", ".editorconfig",
    ".golangci.yml", ".golangci.yaml", ".goreleaser.yml", ".goreleaser.yaml",
}
SOURCE_ROOT_DIRS = {"cmd", "internal", "web", "scripts", "docs", "deploy", ".github", "licenses"}
EXCLUDED_PARTS = {
    ".git", "node_modules", "dist", "data", "uploads", "upload", "reports",
    "coverage", "playwright-report", "test-results", ".cache", "__pycache__",
    "private", "secrets", ".next", ".vite", ".tools",
}
EXCLUDED_SUFFIXES = {".sqlite", ".sqlite3", ".db", ".pem", ".key", ".p12", ".pfx", ".log", ".pyc"}


def source_allowed(path: Path, root: Path) -> bool:
    relative = path.relative_to(root)
    if len(relative.parts) == 1:
        return path.name in SOURCE_ROOT_FILES
    if relative.parts[0] not in SOURCE_ROOT_DIRS:
        return False
    if any(part in EXCLUDED_PARTS for part in relative.parts):
        return False
    if path.name.startswith(".env") or path.suffix.lower() in EXCLUDED_SUFFIXES:
        return False
    if path.name.endswith(("-wal", "-shm", "~", ".bak")):
        return False
    return True


def add_file(archive: tarfile.TarFile, file: Path, name: str, epoch: int) -> None:
    if file.is_symlink():
        raise ValueError(f"Release inputs must not be symlinks: {file}")
    stat = file.stat()
    info = tarfile.TarInfo(name)
    info.size = stat.st_size
    info.mtime = epoch
    info.uid = info.gid = 0
    info.uname = info.gname = ""
    info.mode = 0o755 if stat.st_mode & 0o111 else 0o644
    with file.open("rb") as stream:
        archive.addfile(info, stream)


def create_archive(kind: str, root: Path, destination: Path, epoch: int, prefix: str) -> None:
    if not root.is_dir():
        raise ValueError(f"Missing release input directory: {root}")
    selected: list[Path] = []
    for current, directories, files in os.walk(root, followlinks=False):
        directory = Path(current)
        if kind == "source":
            if directory == root:
                directories[:] = sorted(name for name in directories if name in SOURCE_ROOT_DIRS)
            else:
                directories[:] = sorted(name for name in directories if name not in EXCLUDED_PARTS)
        for name in sorted(files):
            file = directory / name
            if kind != "source" or source_allowed(file, root):
                selected.append(file)
    if not selected:
        raise ValueError("Refusing an empty release archive")
    selected.sort(key=lambda file: file.relative_to(root).as_posix())
    temporary = destination.with_suffix(destination.suffix + ".tmp")
    try:
        with temporary.open("wb") as output:
            with gzip.GzipFile(filename="", mode="wb", fileobj=output, mtime=epoch) as compressed:
                with tarfile.open(fileobj=compressed, mode="w", format=tarfile.PAX_FORMAT) as archive:
                    for file in selected:
                        add_file(archive, file, f"{prefix}/{file.relative_to(root).as_posix()}", epoch)
        temporary.replace(destination)
    finally:
        temporary.unlink(missing_ok=True)


def checksums(root: Path, prefix: str) -> None:
    files = sorted(root.glob(f"{prefix}*.tar.gz"))
    if not files:
        raise ValueError("No versioned release archives found for checksums")
    lines = []
    for file in files:
        digest = hashlib.sha256()
        with file.open("rb") as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                digest.update(chunk)
        lines.append(f"{digest.hexdigest()}  {file.name}\n")
    (root / "SHA256SUMS").write_text("".join(lines), encoding="utf-8")


def main() -> None:
    command = sys.argv[1]
    if command == "checksums":
        checksums(Path(sys.argv[2]).resolve(), sys.argv[3])
        return
    if command not in {"binary", "source"}:
        raise ValueError("Expected binary, source or checksums")
    root = Path(sys.argv[2]).resolve()
    destination = Path(sys.argv[3]).resolve()
    epoch = int(sys.argv[4])
    prefix = sys.argv[5] if len(sys.argv) > 5 else root.name
    create_archive(command, root, destination, epoch, prefix)


if __name__ == "__main__":
    main()

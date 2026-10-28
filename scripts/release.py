#!/usr/bin/env python3
# Copyright 2026 bestkakkoii llc.
# SPDX-License-Identifier: MIT
"""Deterministic local release archives. Python is a build-time dependency only."""

from __future__ import annotations

import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tarfile

SOURCE_ROOT_FILES = {
    "go.mod", "go.sum", "LICENSE", "README.md", "README.zh-TW.md",
    "README_AI.md", "AGENTS.md", "package.json", "package-lock.json",
    "CHANGELOG.md", "CONTRIBUTING.md", "SECURITY.md", "NOTICE",
    "THIRD_PARTY_NOTICES.md", "DEPENDENCIES.md", "dependencies.lock.json", "RELEASE_CANDIDATE",
    "Makefile", ".gitignore", ".gitattributes", ".editorconfig",
    ".golangci.yml", ".golangci.yaml", ".goreleaser.yml", ".goreleaser.yaml",
}
SOURCE_ROOT_DIRS = {"cmd", "internal", "web", "scripts", "docs", "deploy", ".github", "licenses"}
EXCLUDED_PARTS = {
    ".git", "node_modules", "dist", "data", "uploads", "upload", "reports",
    "coverage", "playwright-report", "test-results", ".cache", "__pycache__",
    "private", "secrets", ".next", ".vite", ".tools", ".repair",
}
EXCLUDED_SUFFIXES = {".sqlite", ".sqlite3", ".db", ".pem", ".key", ".p12", ".pfx", ".log", ".pyc"}
REQUIRED_LICENSE_FILES = {
    "licenses/go/LICENSE", "licenses/go/PATENTS", "licenses/react/LICENSE",
    "licenses/definitelytyped/LICENSE", "licenses/csstype/LICENSE", "licenses/eslint/LICENSE",
}


def normalize_version(value: str) -> str:
    match = re.fullmatch(
        r"v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
        r"(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
        r"(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?", value,
    )
    if not match or (match[4] and any(
        part.isdigit() and len(part) > 1 and part.startswith("0")
        for part in match[4].split(".")
    )):
        raise ValueError("Version must be a semantic version such as v1.0.0 or v1.0.0-rc.1")
    return "v" + value.removeprefix("v")


def git_output(root: Path, *args: str) -> bytes:
    environment = os.environ.copy()
    # Resolve the supplied checkout itself, independent of an inherited Git context.
    for name in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_NAMESPACE",
                 "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES"):
        environment.pop(name, None)
    environment["GIT_NO_REPLACE_OBJECTS"] = "1"
    result = subprocess.run(
        ["git", "-C", str(root), "-c", "core.fsmonitor=false", *args],
        env=environment, capture_output=True, check=False,
    )
    if result.returncode:
        raise ValueError("Official release packaging requires an intact Git checkout with a committed HEAD")
    return result.stdout


def release_identity(root: Path, commit: str = "", source_epoch: str = "") -> tuple[str, str]:
    root = root.resolve()
    top = Path(os.fsdecode(git_output(root, "rev-parse", "--show-toplevel").rstrip(b"\n"))).resolve()
    if top != root:
        raise ValueError("Official release packaging must run from the actual Git checkout root")
    head = git_output(root, "rev-parse", "--verify", "HEAD^{commit}").decode().strip()
    if commit:
        resolved = git_output(root, "rev-parse", "--verify", "--end-of-options", commit + "^{commit}").decode().strip()
        if resolved != head:
            raise ValueError("COMMIT must resolve to the current checkout HEAD")
    epoch = git_output(root, "show", "-s", "--format=%ct", "HEAD").decode().strip()
    if source_epoch and (not source_epoch.isascii() or not source_epoch.isdecimal() or int(source_epoch) != int(epoch)):
        raise ValueError("SOURCE_DATE_EPOCH must equal the HEAD commit timestamp")
    if git_output(root, "status", "--porcelain=v1", "--untracked-files=all"):
        raise ValueError("Official releases require a clean Git index and working tree; commit all intended changes first")

    tree = {}
    for record in git_output(root, "ls-tree", "-r", "-z", "HEAD").split(b"\0"):
        if record:
            metadata, name = record.split(b"\t", 1)
            mode, kind, object_id = metadata.decode().split(" ")
            tree[os.fsdecode(name)] = (mode, kind, object_id)
    algorithm = git_output(root, "rev-parse", "--show-object-format").decode().strip()
    if algorithm not in {"sha1", "sha256"}:
        raise ValueError("Unsupported Git object hash format")
    selected = release_files("source", root)
    expected_paths = {name for name in tree if source_allowed(root / name, root)}
    for name in expected_paths:
        if tree[name][0] not in {"100644", "100755"} or tree[name][1] != "blob":
            raise ValueError(f"Committed release input must be a regular file: {name}")
    missing = expected_paths - {file.relative_to(root).as_posix() for file in selected}
    if missing:
        raise ValueError(f"Release source input missing from the working tree: {sorted(missing)[0]}")
    # Git status can hide ignored extras, assume-unchanged contents and deleted
    # skip-worktree files. Check the complete packaged path set, bytes and modes.
    for file in selected:
        relative = file.relative_to(root).as_posix()
        entry = tree.get(relative)
        if entry is None or entry[1] != "blob":
            raise ValueError(f"Release source input is not committed at HEAD: {relative}")
        metadata = file.stat()
        digest = hashlib.new(algorithm)
        digest.update(f"blob {metadata.st_size}\0".encode("ascii"))
        with file.open("rb") as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                digest.update(chunk)
        mode = "100755" if metadata.st_mode & 0o111 else "100644"
        if digest.hexdigest() != entry[2] or mode != entry[0]:
            raise ValueError(f"Release source input differs from HEAD: {relative}")
    return head, epoch


def validate_licenses(root: Path) -> None:
    document = json.loads((root / "dependencies.lock.json").read_text(encoding="utf-8"))
    required = set(REQUIRED_LICENSE_FILES)
    for module in document["go_runtime_modules"]:
        for name in module["license_files"]:
            relative = Path(name)
            if not relative.parts or relative.is_absolute() or ".." in relative.parts or relative.parts[0] != "licenses":
                raise ValueError("Dependency license path must stay inside licenses/")
            required.add(name)
    for name in sorted(required):
        file = root / name
        if not file.is_file() or file.is_symlink() or file.stat().st_size == 0:
            raise ValueError(f"Required third-party license or attribution is missing: {name}")


def validate_go_version(root: Path, actual: str) -> None:
    document = json.loads((root / "dependencies.lock.json").read_text(encoding="utf-8"))
    source = next(item for item in document["sources"] if item["name"] == "Go")
    expected = "go version " + source["version"] + "-" + source["commit"] + " "
    if not actual.startswith(expected) or len(actual.splitlines()) != 1:
        raise ValueError("Release compiler does not match the Go source revision in dependencies.lock.json; run the pinned bootstrap")


def private_path(relative: Path) -> bool:
    return (
        any(part in EXCLUDED_PARTS for part in relative.parts)
        or relative.name.startswith(".env")
        or relative.suffix.lower() in EXCLUDED_SUFFIXES
        or relative.name.endswith(("-wal", "-shm", "~", ".bak"))
    )


def source_allowed(path: Path, root: Path) -> bool:
    relative = path.relative_to(root)
    if len(relative.parts) == 1:
        return path.name in SOURCE_ROOT_FILES
    if relative.parts[0] not in SOURCE_ROOT_DIRS:
        return False
    return not private_path(relative)


def add_file(archive: tarfile.TarFile, file: Path, name: str, epoch: int) -> None:
    metadata = file.lstat()
    if not stat.S_ISREG(metadata.st_mode):
        raise ValueError(f"Release inputs must be regular files, not symlinks or special files: {file}")
    info = tarfile.TarInfo(name)
    info.size = metadata.st_size
    info.mtime = epoch
    info.uid = info.gid = 0
    info.uname = info.gname = ""
    info.mode = 0o755 if metadata.st_mode & 0o111 else 0o644
    with file.open("rb") as stream:
        archive.addfile(info, stream)


def release_files(kind: str, root: Path) -> list[Path]:
    if not root.is_dir():
        raise ValueError(f"Missing release input directory: {root}")
    selected: list[Path] = []

    def traversal_error(error: OSError) -> None:
        raise error

    for current, directories, files in os.walk(root, followlinks=False, onerror=traversal_error):
        directory = Path(current)
        if kind == "source":
            if directory == root:
                directories[:] = sorted(name for name in directories if name in SOURCE_ROOT_DIRS)
            else:
                directories[:] = sorted(name for name in directories if name not in EXCLUDED_PARTS)
        for name in directories:
            child = directory / name
            if child.is_symlink():
                raise ValueError(f"Release input directories must not be symlinks: {child.relative_to(root)}")
            if kind == "binary" and private_path(child.relative_to(root)):
                raise ValueError(f"Private or temporary directory in binary release input: {child.relative_to(root)}")
        for name in sorted(files):
            file = directory / name
            if kind == "binary" and private_path(file.relative_to(root)):
                raise ValueError(f"Private or temporary file in binary release input: {file.relative_to(root)}")
            if kind != "source" or source_allowed(file, root):
                if not stat.S_ISREG(file.lstat().st_mode):
                    raise ValueError(f"Release inputs must be regular files, not symlinks or special files: {file.relative_to(root)}")
                selected.append(file)
    if not selected:
        raise ValueError("Refusing an empty release archive")
    selected.sort(key=lambda file: file.relative_to(root).as_posix())
    return selected


def create_archive(kind: str, root: Path, destination: Path, epoch: int, prefix: str) -> None:
    if prefix and ("/" in prefix or "\\" in prefix or prefix in {".", ".."}):
        raise ValueError("Source archive prefix must be one safe directory name")
    if kind == "binary" and prefix:
        raise ValueError("Binary archives must extract directly into the destination root")
    selected = release_files(kind, root)
    temporary = destination.with_suffix(destination.suffix + ".tmp")
    try:
        with temporary.open("wb") as output:
            with gzip.GzipFile(filename="", mode="wb", fileobj=output, mtime=epoch) as compressed:
                with tarfile.open(fileobj=compressed, mode="w", format=tarfile.PAX_FORMAT) as archive:
                    for file in selected:
                        relative_name = file.relative_to(root).as_posix()
                        name = f"{prefix}/{relative_name}" if prefix else relative_name
                        add_file(archive, file, name, epoch)
        temporary.replace(destination)
    finally:
        temporary.unlink(missing_ok=True)


def checksums(root: Path, prefix: str) -> None:
    if not re.fullmatch(r"healthbank_[0-9A-Za-z.+_-]+_", prefix):
        raise ValueError("Invalid release archive prefix")
    files = sorted(root.glob(f"{prefix}*.tar.gz"))
    expected = {prefix + name + ".tar.gz" for name in ("linux_amd64", "linux_arm64", "source")}
    if {file.name for file in files} != expected:
        raise ValueError("Checksums require exactly the selected version's amd64, arm64 and source archives")
    lines = []
    for file in files:
        if not stat.S_ISREG(file.lstat().st_mode):
            raise ValueError(f"Release archives must be regular files: {file.name}")
        digest = hashlib.sha256()
        with file.open("rb") as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                digest.update(chunk)
        lines.append(f"{digest.hexdigest()}  {file.name}\n")
    temporary = root / "SHA256SUMS.tmp"
    try:
        temporary.write_text("".join(lines), encoding="utf-8")
        temporary.replace(root / "SHA256SUMS")
    finally:
        temporary.unlink(missing_ok=True)


def main() -> None:
    command = sys.argv[1]
    if command == "version":
        print(normalize_version(sys.argv[2]))
        return
    if command == "preflight":
        root = Path(sys.argv[2]).resolve()
        commit, epoch = release_identity(root, sys.argv[3], sys.argv[4])
        validate_licenses(root)
        print(commit + "\t" + epoch)
        return
    if command == "verify-go":
        validate_go_version(Path(sys.argv[2]).resolve(), sys.argv[3])
        return
    if command == "checksums":
        checksums(Path(sys.argv[2]).resolve(), sys.argv[3])
        return
    if command not in {"binary", "source"}:
        raise ValueError("Expected binary, source or checksums")
    root = Path(sys.argv[2]).resolve()
    destination = Path(sys.argv[3]).resolve()
    epoch = int(sys.argv[4])
    # Deployable binaries extract directly into the selected application directory.
    # Source bundles keep their conventional versioned source directory.
    prefix = "" if command == "binary" else (sys.argv[5] if len(sys.argv) > 5 else root.name)
    create_archive(command, root, destination, epoch, prefix)


if __name__ == "__main__":
    main()

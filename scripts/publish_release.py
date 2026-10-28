#!/usr/bin/env python3
# Copyright 2026 bestkakkoii llc.
# SPDX-License-Identifier: MIT
"""Publish only the verified version's assets at the exact built source commit."""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tarfile
import tempfile
from urllib.parse import quote

from release import git_output, normalize_version


def github_json(*arguments: str) -> object:
    result = subprocess.run(["gh", "api", *arguments], capture_output=True, text=True, check=True)
    return json.loads(result.stdout)


def release_exists(repository: str, version: str) -> bool:
    pages = github_json(f"repos/{repository}/releases?per_page=100", "--paginate", "--slurp")
    if not isinstance(pages, list) or any(not isinstance(page, list) for page in pages):
        raise ValueError("Unexpected GitHub release-list response; refusing to publish")
    tags = []
    for page in pages:
        for item in page:
            if not isinstance(item, dict) or not isinstance(item.get("tag_name"), str):
                raise ValueError("Unexpected GitHub release entry; refusing to publish")
            tags.append(item["tag_name"])
    return version in tags


def tag_commit(repository: str, version: str) -> str | None:
    refs = github_json(f"repos/{repository}/git/matching-refs/tags/{quote(version, safe='')}")
    if not isinstance(refs, list):
        raise ValueError("Unexpected GitHub tag-list response; refusing to publish")
    matches = []
    for item in refs:
        if not isinstance(item, dict) or not isinstance(item.get("ref"), str):
            raise ValueError("Unexpected GitHub tag entry; refusing to publish")
        if item["ref"] == "refs/tags/" + version:
            matches.append(item)
    if not matches:
        return None
    if len(matches) != 1:
        raise ValueError("Ambiguous GitHub tag identity; refusing to publish")
    current = matches[0].get("object")
    visited = set()
    for _ in range(16):
        if not isinstance(current, dict):
            raise ValueError("Missing GitHub tag target; refusing to publish")
        kind, object_id = current.get("type"), current.get("sha")
        if not isinstance(object_id, str) or not re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", object_id):
            raise ValueError("Invalid GitHub tag object identity; refusing to publish")
        if object_id in visited:
            raise ValueError("Cyclic GitHub annotated tag; refusing to publish")
        visited.add(object_id)
        if kind == "commit":
            return object_id
        if kind != "tag":
            raise ValueError("GitHub tag does not point to a commit; refusing to publish")
        annotated = github_json(f"repos/{repository}/git/tags/{object_id}")
        if not isinstance(annotated, dict):
            raise ValueError("Unexpected GitHub annotated-tag response; refusing to publish")
        current = annotated.get("object")
    raise ValueError("GitHub annotated tag nesting exceeds the verification limit")


def verified_assets(root: Path, version: str, commit: str) -> list[Path]:
    directory = root / "dist/releases"
    prefix = "healthbank_" + version.removeprefix("v") + "_"
    archives = [directory / (prefix + platform + ".tar.gz") for platform in ("linux_amd64", "linux_arm64", "source")]
    manifest = directory / "SHA256SUMS"
    for file in [*archives, manifest]:
        if not stat.S_ISREG(file.lstat().st_mode):
            raise ValueError(f"Release asset must be a regular file: {file.name}")
    expected = {}
    for line in manifest.read_text(encoding="utf-8").splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([^/\\]+)", line)
        if match is None or match[2] in expected:
            raise ValueError("Invalid or duplicate release checksum entry")
        expected[match[2]] = match[1]
    if set(expected) != {file.name for file in archives}:
        raise ValueError("Release checksums must name exactly this version's three archives")
    for file in archives:
        with file.open("rb") as stream:
            actual = hashlib.file_digest(stream, "sha256").hexdigest()
        if actual != expected[file.name]:
            raise ValueError(f"Release checksum mismatch: {file.name}")
    for file in archives[:2]:
        with tarfile.open(file, "r:gz") as archive:
            contents = {}
            for name in ("VERSION", "BUILDINFO"):
                member = archive.getmember(name)
                if not member.isfile() or member.size > 65536:
                    raise ValueError(f"Invalid release identity document: {file.name}")
                with archive.extractfile(member) as stream:
                    contents[name] = stream.read().decode("utf-8")
            fields = {}
            for line in contents["BUILDINFO"].splitlines():
                if "=" in line:
                    key, value = line.split("=", 1)
                    if key in fields:
                        raise ValueError("Duplicate release identity field")
                    fields[key] = value
            if contents["VERSION"].strip() != version or fields.get("version") != version or fields.get("commit") != commit:
                raise ValueError(f"Release archive identity differs from the selected version or commit: {file.name}")
    return [*archives, manifest]


def release_notes(root: Path, version: str) -> str:
    selected = []
    for line in (root / "CHANGELOG.md").read_text(encoding="utf-8-sig").splitlines():
        if line.startswith("## " + version + " "):
            selected.append(line)
        elif selected and line.startswith("## "):
            break
        elif selected:
            selected.append(line)
    if len(selected) < 2 or not any(line.strip() for line in selected[1:]):
        raise ValueError(f"Missing release notes for {version}")
    return "\n".join(selected).rstrip() + "\n"


def publish(root: Path, version: str, expected_commit: str, mode: str) -> None:
    version = normalize_version(version)
    if mode not in {"candidate", "tag"}:
        raise ValueError("Publication mode must be candidate or tag")
    if mode == "candidate" and not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+-rc\.[0-9]+", version):
        raise ValueError("Main-branch publication requires an explicit RC version")
    repository = os.environ.get("GH_REPO", "")
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        raise ValueError("GH_REPO must identify the expected owner/repository")
    head = git_output(root, "rev-parse", "--verify", "HEAD^{commit}").decode().strip()
    target = git_output(root, "rev-parse", "--verify", "--end-of-options", expected_commit + "^{commit}").decode().strip()
    if target != head:
        raise ValueError("Publication commit differs from the built checkout HEAD")
    assets = verified_assets(root, version, head)
    notes = release_notes(root, version) if mode == "candidate" else ""
    exists = release_exists(repository, version)
    actual_tag_commit = tag_commit(repository, version)
    if actual_tag_commit is not None and actual_tag_commit != head:
        raise ValueError("Existing release tag points to a different commit; refusing to publish")
    if exists:
        if mode == "candidate":
            raise ValueError(f"Release already exists: {version}; refusing to replace its publication state or assets")
        print(f"Release {version} already exists; leaving its publication state and assets unchanged.")
        return
    if actual_tag_commit is None:
        if mode == "tag":
            raise ValueError("The pushed release tag is missing; refusing to create it from an uncertain event")
        github_json(f"repos/{repository}/git/refs", "--method", "POST",
                    "-f", "ref=refs/tags/" + version, "-f", "sha=" + head)
        # Verify the created reference through the same read path before publication.
        if tag_commit(repository, version) != head:
            raise ValueError("Created release tag does not match the built source commit")
    arguments = ["gh", "release", "create", version, *map(str, assets),
                 "--verify-tag", "--target", head, "--title", "Healthbank " + version]
    if "-" in version.split("+", 1)[0]:
        arguments.extend(["--prerelease", "--latest=false"])
    if mode == "tag":
        subprocess.run([*arguments, "--draft", "--generate-notes"], check=True)
        return
    with tempfile.TemporaryDirectory(prefix="healthbank-release-notes-") as temporary:
        path = Path(temporary) / "release-notes.md"
        path.write_text(notes, encoding="utf-8")
        subprocess.run([*arguments, "--notes-file", str(path)], check=True)


if __name__ == "__main__":
    publish(Path(__file__).resolve().parent.parent, sys.argv[1], sys.argv[2], sys.argv[3])

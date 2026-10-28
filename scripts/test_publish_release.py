#!/usr/bin/env python3
# Copyright 2026 bestkakkoii llc.
# SPDX-License-Identifier: MIT
"""Exercise publication through a synthetic gh executable; never contact GitHub."""

from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import publish_release
import release


VERSION = "v1.2.3-rc.1"
EPOCH = 1700000000
FAKE_GH = r'''
import json, os, pathlib, sys
state_file = pathlib.Path(os.environ["SYNTHETIC_GH_STATE"])
log_file = pathlib.Path(os.environ["SYNTHETIC_GH_LOG"])
state = json.loads(state_file.read_text())
args = sys.argv[1:]
with log_file.open("a") as stream:
    stream.write(json.dumps(args) + "\n")
if args[:2] == ["release", "create"]:
    print("synthetic publication recorded; no network")
    sys.exit(0)
if args[0] != "api":
    raise SystemExit("unexpected synthetic command")
endpoint = args[1]
if "/releases?" in endpoint:
    if state.get("release_error"):
        raise SystemExit(17)
    print(json.dumps(state.get("pages", [[]])))
elif "/git/matching-refs/" in endpoint:
    if state.get("tag_error"):
        raise SystemExit(18)
    target = state.get("tag")
    refs = [] if target is None else [{"ref": "refs/tags/" + state["version"], "object": target}]
    print(json.dumps(refs))
elif "/git/tags/" in endpoint:
    if state.get("annotation_error"):
        raise SystemExit(19)
    print(json.dumps({"object": state["annotations"][endpoint.rsplit("/", 1)[1]]}))
elif endpoint.endswith("/git/refs") and "POST" in args:
    if state.get("create_tag_error"):
        raise SystemExit(20)
    commit = next(value.removeprefix("sha=") for value in args if value.startswith("sha="))
    state["tag"] = {"type": "commit", "sha": state.get("created_commit", commit)}
    state_file.write_text(json.dumps(state))
    print(json.dumps({"object": state["tag"]}))
else:
    raise SystemExit("unexpected synthetic API endpoint: " + endpoint)
'''


class ReleasePublicationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.base = Path(self.temporary.name)
        self.root = self.base / "source"
        self.root.mkdir()
        (self.root / "README.md").write_text("synthetic source\n")
        (self.root / "CHANGELOG.md").write_text(f"## {VERSION} — synthetic fixture\n\nSynthetic changes.\n\n## v1.2.2 — previous\nOld changes.\n")
        subprocess.run(["git", "init", "-q", str(self.root)], check=True)
        subprocess.run(["git", "-C", str(self.root), "add", "."], check=True)
        subprocess.run(["git", "-C", str(self.root), "-c", "core.hooksPath=/dev/null",
                        "-c", "user.name=Synthetic release fixture", "-c", "user.email=synthetic@example.invalid",
                        "commit", "-qm", "synthetic source"], check=True)
        self.commit = subprocess.check_output(["git", "-C", str(self.root), "rev-parse", "HEAD"], text=True).strip()
        self.state = self.base / "state.json"
        self.log = self.base / "commands.jsonl"
        self.configure()
        executable = self.base / "bin/gh"
        executable.parent.mkdir()
        executable.write_text("#!" + sys.executable + "\n" + FAKE_GH)
        executable.chmod(0o755)
        self.environment = patch.dict(os.environ, {
            "PATH": str(executable.parent) + os.pathsep + os.environ.get("PATH", ""),
            "GH_REPO": "synthetic/healthbank", "GH_TOKEN": "synthetic-not-a-token",
            "SYNTHETIC_GH_STATE": str(self.state), "SYNTHETIC_GH_LOG": str(self.log),
        })
        self.environment.start()
        self.addCleanup(self.environment.stop)
        self.assets = self.root / "dist/releases"
        self.assets.mkdir(parents=True)
        self.write_archives(self.commit)

    def configure(self, **values: object) -> None:
        self.state.write_text(json.dumps({"version": VERSION, **values}))

    def write_archives(self, commit: str) -> None:
        staging = self.base / "binary"
        staging.mkdir(exist_ok=True)
        (staging / "README.md").write_text("synthetic source\n")
        (staging / "healthbank").write_text("synthetic executable\n")
        (staging / "VERSION").write_text(VERSION + "\n")
        (staging / "BUILDINFO").write_text(f"version={VERSION}\ncommit={commit}\n")
        prefix = "healthbank_" + VERSION.removeprefix("v") + "_"
        for arch in ("amd64", "arm64"):
            release.create_archive("binary", staging, self.assets / (prefix + "linux_" + arch + ".tar.gz"), EPOCH, "")
        release.create_archive("source", staging, self.assets / (prefix + "source.tar.gz"), EPOCH, "healthbank_source")
        release.checksums(self.assets, prefix)

    def commands(self) -> list[list[str]]:
        if not self.log.exists():
            return []
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def assert_no_mutations(self) -> None:
        for arguments in self.commands():
            self.assertNotEqual(arguments[:2], ["release", "create"])
            self.assertNotIn("POST", arguments)

    def test_candidate_creates_verified_tag_and_uploads_only_selected_assets(self) -> None:
        (self.assets / "healthbank_0.0.1_linux_amd64.tar.gz").write_text("stale unrelated version\n")
        publish_release.publish(self.root, VERSION, self.commit, "candidate")
        commands = self.commands()
        created = [args for args in commands if args[:2] == ["release", "create"]]
        self.assertEqual(len(created), 1)
        arguments = created[0]
        self.assertIn("--verify-tag", arguments)
        self.assertNotIn("--draft", arguments)
        self.assertEqual(arguments[arguments.index("--target") + 1], self.commit)
        self.assertEqual({Path(value).name for value in arguments if value.endswith((".tar.gz", "SHA256SUMS"))}, {
            "healthbank_1.2.3-rc.1_linux_amd64.tar.gz", "healthbank_1.2.3-rc.1_linux_arm64.tar.gz",
            "healthbank_1.2.3-rc.1_source.tar.gz", "SHA256SUMS",
        })

    def test_existing_lightweight_or_annotated_tag_must_match_source_commit(self) -> None:
        for annotated in (False, True):
            with self.subTest(annotated=annotated):
                tag = {"type": "commit", "sha": "b" * 40}
                values = {}
                if annotated:
                    values["annotations"] = {"a" * 40: tag}
                    tag = {"type": "tag", "sha": "a" * 40}
                self.configure(tag=tag, **values)
                with self.assertRaisesRegex(ValueError, "different commit"):
                    publish_release.publish(self.root, VERSION, self.commit, "candidate")
                self.assert_no_mutations()

    def test_nested_annotated_tag_is_peeled_and_pushed_tag_stays_draft(self) -> None:
        self.configure(tag={"type": "tag", "sha": "a" * 40}, annotations={
            "a" * 40: {"type": "tag", "sha": "b" * 40},
            "b" * 40: {"type": "commit", "sha": self.commit},
        })
        publish_release.publish(self.root, VERSION, self.commit, "tag")
        creation = next(args for args in self.commands() if args[:2] == ["release", "create"])
        self.assertIn("--draft", creation)
        self.assertFalse(any("POST" in args for args in self.commands()))

    def test_api_failures_never_mean_release_or_tag_absence(self) -> None:
        for failure in ("release_error", "tag_error", "annotation_error"):
            with self.subTest(failure=failure):
                self.configure(**{failure: True}, tag={"type": "tag", "sha": "a" * 40})
                with self.assertRaises(subprocess.CalledProcessError):
                    publish_release.publish(self.root, VERSION, self.commit, "candidate")
                self.assert_no_mutations()

    def test_existing_release_is_never_modified(self) -> None:
        self.configure(pages=[[{"tag_name": VERSION}]], tag={"type": "commit", "sha": self.commit})
        with self.assertRaisesRegex(ValueError, "Release already exists"):
            publish_release.publish(self.root, VERSION, self.commit, "candidate")
        publish_release.publish(self.root, VERSION, self.commit, "tag")
        self.assert_no_mutations()

    def test_invalid_cyclic_or_missing_tag_blocks_tag_publication(self) -> None:
        for tag in (None, {"type": "tree", "sha": "a" * 40}, {"type": "tag", "sha": "a" * 40}):
            with self.subTest(tag=tag):
                self.configure(tag=tag, annotations={"a" * 40: {"type": "tag", "sha": "a" * 40}})
                with self.assertRaises(ValueError):
                    publish_release.publish(self.root, VERSION, self.commit, "tag")
                self.assert_no_mutations()

    def test_failed_or_wrong_new_tag_never_publishes_release(self) -> None:
        for values in ({"create_tag_error": True}, {"created_commit": "b" * 40}):
            with self.subTest(values=values):
                self.configure(**values)
                with self.assertRaises((ValueError, subprocess.CalledProcessError)):
                    publish_release.publish(self.root, VERSION, self.commit, "candidate")
                self.assertFalse(any(args[:2] == ["release", "create"] for args in self.commands()))

    def test_tampered_or_wrong_commit_assets_fail_before_any_github_call(self) -> None:
        archive = self.assets / "healthbank_1.2.3-rc.1_linux_amd64.tar.gz"
        archive.write_bytes(archive.read_bytes() + b"tampered")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            publish_release.publish(self.root, VERSION, self.commit, "candidate")
        self.write_archives("b" * 40)
        with self.assertRaisesRegex(ValueError, "identity differs"):
            publish_release.publish(self.root, VERSION, self.commit, "candidate")
        self.assertEqual(self.commands(), [])

    def test_missing_notes_and_malformed_release_list_fail_closed(self) -> None:
        (self.root / "CHANGELOG.md").write_text("## " + VERSION + " — empty\n")
        with self.assertRaisesRegex(ValueError, "Missing release notes"):
            publish_release.publish(self.root, VERSION, self.commit, "candidate")
        self.assertEqual(self.commands(), [])
        self.configure(pages={"unexpected": "object"})
        with self.assertRaisesRegex(ValueError, "release-list response"):
            publish_release.publish(self.root, VERSION, self.commit, "tag")
        self.assert_no_mutations()


if __name__ == "__main__":
    unittest.main(verbosity=2)

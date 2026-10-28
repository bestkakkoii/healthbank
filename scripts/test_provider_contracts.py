#!/usr/bin/env python3
"""Synthetic regressions for the public-contract checker; no network access.

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.
"""

import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from urllib.request import Request

import check_provider_contracts as checker


def app_script(name="IHKE3202S01", digest="12345678"):
    return ('function source(E){return t.p+"js/"+({' + name + ':"' + name + '"}[E]||E)+"."+{' + name + ':"' + digest + '"}[E]+".js"}').encode()


def digest(data):
    return hashlib.sha256(data).hexdigest()


def synthetic_registry(script, body):
    entry = b'<script src="js/app.11111111.js"></script>'
    return {"registry_id": "synthetic", "contract_revision": "synthetic.1",
            "public_graph": {"entry_sha256": digest(entry)},
            "app_asset": {"file": "app.11111111.js", "sha256": digest(script)},
            "entry_assets": [{"file": "app.11111111.js", "path": "/IHKE3000/js/app.11111111.js", "sha256": digest(script)}],
            "assets": [{"module": "IHKE3202S01", "file": "IHKE3202S01.12345678.js", "sha256": digest(body)}]}


class PublicContractTests(unittest.TestCase):
    def test_repository_registry_is_valid_without_network(self):
        registry = checker.read_registry(Path(__file__).resolve().parents[1] / "internal/nhi/provider-contracts.json")
        inventory = json.loads((Path(__file__).resolve().parents[1] / "docs/provider-source-inventory.json").read_text(encoding="utf-8"))
        self.assertEqual({item["file"] for item in registry["assets"] + registry["entry_assets"]}, {item["file"] for item in inventory["assets"]})
        self.assertEqual(len(registry["assets"]), registry["public_graph"]["lazy_chunks"])
        self.assertEqual({item["id"] for item in registry["contracts"]}, set(inventory["provider_categories"]))

    def test_current_asset_map_drift_cannot_be_hidden_by_accessible_old_bundle(self):
        previous = app_script()
        current = app_script(digest="87654321")
        old_body, new_body = b"old public contract", b"new public contract"
        registry = synthetic_registry(previous, old_body)
        requests = []

        def fetch(path):
            requests.append(path)
            return {"/IHKE3000/": b'<script src="/IHKE3000/js/app.22222222.js"></script>', "/IHKE3000/js/app.22222222.js": current,
                    "/IHKE3000/js/IHKE3202S01.87654321.js": new_body,
                    "/IHKE3000/js/IHKE3202S01.12345678.js": old_body}[path]

        result = checker.online_check(registry, fetch)
        self.assertEqual(result["status"], "changed")
        self.assertEqual(result["assets"][0]["current_file"], "IHKE3202S01.87654321.js")
        self.assertNotIn("/IHKE3000/js/IHKE3202S01.12345678.js", requests)

    def test_unchanged_and_missing_module_states_are_distinct(self):
        script = app_script()
        body = b"synthetic public bundle"
        registry = synthetic_registry(script, body)
        def fetch(path):
            if path == "/IHKE3000/": return b'<script src="js/app.11111111.js"></script>'
            return script if path.endswith("app.11111111.js") else body
        self.assertEqual(checker.online_check(registry, fetch)["status"], "unchanged")
        registry["assets"][0]["module"] = "IHKE9999S01"
        result = checker.online_check(registry, fetch)
        self.assertEqual(result["status"], "changed")
        self.assertEqual(result["assets"][0]["error_code"], "module_absent_from_current_map")

    def test_new_entry_scripts_and_unclassified_chunks_cannot_pass_as_unchanged(self):
        previous, body = app_script(), b"synthetic source"
        current = previous.replace(b'IHKE3202S01:"12345678"', b'IHKE3202S01:"12345678",IHKE9999S01:"87654321"')
        registry = synthetic_registry(previous, body)
        entry = b'<script src="js/app.11111111.js"></script><script src="js/new-source.js"></script>'
        def fetch(path):
            if path == checker.ENTRY_PATH: return entry
            if path.endswith("app.11111111.js"): return current
            if path.endswith("IHKE3202S01.12345678.js"): return body
            raise AssertionError("Unreviewed source script must not be executed or silently accepted")
        result = checker.online_check(registry, fetch)
        self.assertEqual(result["status"], "changed")
        self.assertEqual(result["lazy_module_set"]["added"], ["IHKE9999S01"])
        self.assertEqual(result["entry_script_set"]["added"], ["/IHKE3000/js/new-source.js"])

    def test_inline_entry_changes_and_mutable_helper_changes_require_review(self):
        script, body, helper = app_script(), b"synthetic source", b"old helper"
        entry = b'<script src="js/app.11111111.js"></script><script src="js/helper.js"></script>'
        registry = synthetic_registry(script, body)
        registry["public_graph"]["entry_sha256"] = digest(entry)
        registry["entry_assets"].append({"file": "helper.js", "path": "/IHKE3000/js/helper.js", "sha256": digest(helper)})
        def fetch(path):
            if path == checker.ENTRY_PATH: return entry + b'<script>new inline behavior</script>'
            if path.endswith("app.11111111.js"): return script
            if path.endswith("helper.js"): return b"changed helper"
            return body
        result = checker.online_check(registry, fetch)
        self.assertEqual(result["status"], "changed")
        self.assertEqual(result["entry_document"]["status"], "changed")
        self.assertEqual(result["entry_assets"][1]["status"], "changed")

    def test_registry_graph_counts_and_duplicate_entry_scripts_are_rejected(self):
        value = json.loads((Path(__file__).resolve().parents[1] / "internal/nhi/provider-contracts.json").read_text(encoding="utf-8"))
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "registry.json"
            value["public_graph"]["unique_js_assets"] += 1
            path.write_text(json.dumps(value), encoding="utf-8")
            with self.assertRaises(checker.ContractCheckError): checker.read_registry(path)
            value["public_graph"]["unique_js_assets"] -= 1
            value["entry_assets"].append(value["entry_assets"][0])
            path.write_text(json.dumps(value), encoding="utf-8")
            with self.assertRaises(checker.ContractCheckError): checker.read_registry(path)

    def test_network_failure_is_inconclusive_and_does_not_leak_exception_content(self):
        def fail(_): raise OSError("SYNTHETIC_PRIVATE_ERROR_CONTENT")
        result = checker.online_check({"registry_id": "synthetic", "contract_revision": "synthetic.1"}, fail)
        self.assertEqual(result["status"], "unavailable")
        self.assertEqual(result["error_code"], "public_read_failed")
        self.assertNotIn("SYNTHETIC_PRIVATE", str(result))

    def test_changed_javascript_syntax_and_duplicate_map_keys_are_not_executed(self):
        for script in [b"throw new Error('must not execute')", app_script().replace(b'"12345678"', b'evil()'), app_script().replace(b'IHKE3202S01:"12345678"', b'IHKE3202S01:"12345678",IHKE3202S01:"87654321"')]:
            with self.assertRaises(checker.ContractCheckError): checker.current_asset_map(script)

    def test_only_fixed_public_assets_can_be_fetched_or_redirected(self):
        for path in ["/api/ihke3000/test", "/IHKE3000/js/../../secret", "https://example.com/file.js", "/IHKE3000/js/app.12345678.js?secret=value"]:
            with self.assertRaises(checker.ContractCheckError): checker.public_url(path)
        redirect = checker.FixedOriginRedirects()
        for url in ["https://example.com/a.js", "http://myhealthbank.nhi.gov.tw/IHKE3000/", "https://myhealthbank.nhi.gov.tw/api/private", "https://myhealthbank.nhi.gov.tw/IHKE3000/?id=synthetic"]:
            with self.assertRaises(checker.ContractCheckError): redirect.redirect_request(Request(checker.ORIGIN + checker.ENTRY_PATH), None, 302, "redirect", {}, url)
        with self.assertRaises(checker.ContractCheckError): checker.current_app_path(b'<script src="https://example.com/app.12345678.js"></script>')


if __name__ == "__main__":
    unittest.main()

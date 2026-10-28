#!/usr/bin/env python3
"""Synthetic regressions for the public-contract checker; no network access.

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.
"""

import hashlib
from pathlib import Path
import unittest
from urllib.request import Request

import check_provider_contracts as checker


def app_script(name="IHKE3202S01", digest="12345678"):
    return ('function source(E){return t.p+"js/"+({' + name + ':"' + name + '"}[E]||E)+"."+{' + name + ':"' + digest + '"}[E]+".js"}').encode()


def digest(data):
    return hashlib.sha256(data).hexdigest()


class PublicContractTests(unittest.TestCase):
    def test_repository_registry_is_valid_without_network(self):
        registry = checker.read_registry(Path(__file__).resolve().parents[1] / "internal/nhi/provider-contracts.json")
        self.assertEqual(len(registry["contracts"]), 38)

    def test_current_asset_map_drift_cannot_be_hidden_by_accessible_old_bundle(self):
        previous = app_script()
        current = app_script(digest="87654321")
        old_body, new_body = b"old public contract", b"new public contract"
        registry = {"registry_id": "synthetic", "contract_revision": "synthetic.1", "app_asset": {"file": "app.11111111.js", "sha256": digest(previous)}, "assets": [{"module": "IHKE3202S01", "file": "IHKE3202S01.12345678.js", "sha256": digest(old_body)}]}
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
        registry = {"registry_id": "synthetic", "contract_revision": "synthetic.1", "app_asset": {"file": "app.11111111.js", "sha256": digest(script)}, "assets": [{"module": "IHKE3202S01", "file": "IHKE3202S01.12345678.js", "sha256": digest(body)}]}
        def fetch(path):
            if path == "/IHKE3000/": return b'<script src="js/app.11111111.js"></script>'
            return script if path.endswith("app.11111111.js") else body
        self.assertEqual(checker.online_check(registry, fetch)["status"], "unchanged")
        registry["assets"][0]["module"] = "IHKE9999S01"
        result = checker.online_check(registry, fetch)
        self.assertEqual(result["status"], "changed")
        self.assertEqual(result["assets"][0]["error_code"], "module_absent_from_current_map")

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

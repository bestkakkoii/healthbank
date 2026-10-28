#!/usr/bin/env python3
"""Read-only observed SPA drift check; never authenticates or executes JavaScript.

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.
"""

from __future__ import annotations

import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import hashlib
from html.parser import HTMLParser
import json
from pathlib import Path
import re
import sys
from urllib.parse import urljoin, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener

ORIGIN = "https://myhealthbank.nhi.gov.tw"
ENTRY_PATH = "/IHKE3000/"
MAX_PUBLIC_BYTES = 8 * 1024 * 1024
ASSET = re.compile(r"[A-Za-z0-9_~.-]+\.[a-f0-9]{8,64}\.js\Z")
IDENTIFIER = re.compile(r"[a-z][a-z0-9_]{0,63}\Z")
HASH = re.compile(r"[a-f0-9]{64}\Z")


class ContractCheckError(ValueError):
    """An unrecognized public contract is evidence to review, not to guess."""


def duplicate_safe_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ContractCheckError("duplicate_registry_key")
        result[key] = value
    return result


def read_registry(path: Path) -> dict:
    raw = path.read_bytes()
    if len(raw) > 2 * 1024 * 1024:
        raise ContractCheckError("registry_too_large")
    value = json.loads(raw.decode("utf-8"), object_pairs_hook=duplicate_safe_object)
    if (value.get("schema_version") != 1 or value.get("origin") != ORIGIN
            or value.get("entry_path") != ENTRY_PATH
            or not isinstance(value.get("contract_revision"), str)):
        raise ContractCheckError("registry_identity_invalid")
    assets = value.get("assets")
    if not isinstance(assets, list) or not 1 <= len(assets) <= 160:
        raise ContractCheckError("registry_assets_invalid")
    modules = set()
    for asset in [value.get("app_asset", {})] + assets:
        if not ASSET.fullmatch(asset.get("file", "")) or not HASH.fullmatch(asset.get("sha256", "")):
            raise ContractCheckError("registry_asset_invalid")
    if not value["app_asset"]["file"].startswith("app."):
        raise ContractCheckError("registry_app_invalid")
    for asset in assets:
        module = asset.get("module")
        if not isinstance(module, str) or module != asset["file"].rsplit(".", 2)[0] or module in modules:
            raise ContractCheckError("registry_module_invalid")
        modules.add(module)
    ids = set()
    contracts = value.get("contracts")
    if not isinstance(contracts, list) or not contracts or len(contracts) > 100:
        raise ContractCheckError("registry_contracts_invalid")
    filenames = {asset["file"] for asset in assets}
    for contract in contracts:
        name = contract.get("id", "")
        if not IDENTIFIER.fullmatch(name) or name in ids or contract.get("range_mode") not in ("source", "date_search"):
            raise ContractCheckError("registry_category_invalid")
        ids.add(name)
        if not contract.get("source_assets") or any(item not in filenames for item in contract["source_assets"]):
            raise ContractCheckError("registry_source_locus_invalid")
    return value


def public_url(path: str) -> str:
    if path != ENTRY_PATH and not re.fullmatch(r"/IHKE3000/js/[A-Za-z0-9_~.-]+\.js", path):
        raise ContractCheckError("nonpublic_path_rejected")
    return ORIGIN + path


class FixedOriginRedirects(HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, new_url):
        parsed = urlsplit(new_url)
        if parsed.scheme != "https" or parsed.netloc != "myhealthbank.nhi.gov.tw" or parsed.query or parsed.fragment:
            raise ContractCheckError("off_origin_redirect_rejected")
        public_url(parsed.path)
        return super().redirect_request(request, fp, code, message, headers, new_url)


def fetch_public(path: str) -> bytes:
    # No CookieJar, Authorization, credentials, API calls or request bodies.
    request = Request(public_url(path), headers={"User-Agent": "Healthbank-public-contract-check/1", "Accept": "text/html,application/javascript,text/javascript", "Cache-Control": "no-cache"})
    opener = build_opener(FixedOriginRedirects())
    with opener.open(request, timeout=20) as response:
        data = response.read(MAX_PUBLIC_BYTES + 1)
        if response.status != 200 or len(data) > MAX_PUBLIC_BYTES:
            raise ContractCheckError("public_response_invalid")
        data.decode("utf-8", errors="strict")
        return data


class EntryScripts(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.paths = []

    def handle_starttag(self, tag, attrs):
        if tag != "script":
            return
        sources = [value for key, value in attrs if key == "src"]
        if len(sources) != 1 or sources[0] is None:
            return
        parsed = urlsplit(urljoin(ORIGIN + ENTRY_PATH, sources[0]))
        if (parsed.scheme == "https" and parsed.netloc == "myhealthbank.nhi.gov.tw"
                and not parsed.query and not parsed.fragment
                and re.fullmatch(r"/IHKE3000/js/app\.[a-f0-9]{8,64}\.js", parsed.path)):
            self.paths.append(parsed.path)


def current_app_path(html: bytes) -> str:
    parser = EntryScripts()
    parser.feed(html.decode("utf-8", errors="strict"))
    if len(parser.paths) != 1:
        raise ContractCheckError("current_app_asset_unrecognized")
    return parser.paths[0]


def javascript_string_map(source: str) -> dict[str, str]:
    # Deliberately parse only the observed plain-string map. No eval/JS runtime.
    pair = re.compile(r'(?:"([A-Za-z0-9_~.-]+)"|([A-Za-z_$][A-Za-z0-9_$]*))\s*:\s*"([A-Za-z0-9_~.-]+)"')
    result = {}
    position = 0
    while position < len(source):
        found = pair.match(source, position)
        if found is None:
            raise ContractCheckError("asset_map_syntax_changed")
        key = found.group(1) or found.group(2)
        if key in result:
            raise ContractCheckError("duplicate_asset_map_key")
        result[key] = found.group(3)
        if len(result) > 2048:
            raise ContractCheckError("asset_map_too_large")
        position = found.end()
        if position < len(source):
            if source[position] != ",":
                raise ContractCheckError("asset_map_syntax_changed")
            position += 1
    return result


def current_asset_map(script: bytes) -> dict[str, str]:
    source = script.decode("utf-8", errors="strict")
    pattern = re.compile(r'"js/"\+\(\{(?P<names>[^{}]*)\}\[(?P<index>[A-Za-z_$][A-Za-z0-9_$]*)\]\|\|(?P=index)\)\+"\."\+\{(?P<hashes>[^{}]*)\}\[(?P=index)\]\+"\.js"')
    found = list(pattern.finditer(source))
    if len(found) != 1:
        raise ContractCheckError("current_asset_map_unrecognized")
    names = javascript_string_map(found[0]["names"])
    hashes = javascript_string_map(found[0]["hashes"])
    result = {}
    for key, digest in hashes.items():
        if not re.fullmatch(r"[a-f0-9]{8,64}", digest):
            raise ContractCheckError("asset_map_hash_invalid")
        filename = names.get(key, key) + "." + digest + ".js"
        if not ASSET.fullmatch(filename):
            raise ContractCheckError("asset_map_filename_invalid")
        result[key] = filename
    return result


def online_check(registry: dict, fetch=fetch_public) -> dict:
    report = {"registry_id": registry["registry_id"], "contract_revision": registry["contract_revision"], "checked_at": datetime.now(timezone.utc).isoformat(), "scope": "credentialless public entry and current asset map; no account/API verification", "status": "unchanged", "assets": []}
    try:
        path = current_app_path(fetch(ENTRY_PATH))
        script = fetch(path)
        actual_map = current_asset_map(script)
    except (OSError, ValueError) as error:
        report.update(status="unavailable", error_code=error.args[0] if isinstance(error, ContractCheckError) else "public_read_failed")
        return report
    app = {"module": "app", "file": path.rsplit("/", 1)[1], "sha256": hashlib.sha256(script).hexdigest()}
    app["status"] = "unchanged" if app["file"] == registry["app_asset"]["file"] and app["sha256"] == registry["app_asset"]["sha256"] else "changed"
    report["app_asset"] = app

    def inspect(asset):
        current = actual_map.get(asset["module"])
        item = {"module": asset["module"], "expected_file": asset["file"], "current_file": current}
        if current is None:
            return dict(item, status="changed", error_code="module_absent_from_current_map")
        try:
            body = fetch("/IHKE3000/js/" + current)
            digest = hashlib.sha256(body).hexdigest()
            return dict(item, status="unchanged" if current == asset["file"] and digest == asset["sha256"] else "changed", sha256=digest)
        except (OSError, ValueError) as error:
            return dict(item, status="unavailable", error_code=error.args[0] if isinstance(error, ContractCheckError) else "public_read_failed")

    with ThreadPoolExecutor(max_workers=4) as executor:
        report["assets"] = list(executor.map(inspect, registry["assets"]))
    statuses = [app["status"]] + [item["status"] for item in report["assets"]]
    report["status"] = "changed" if "changed" in statuses else "unavailable" if "unavailable" in statuses else "unchanged"
    return report


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--registry", type=Path, default=Path(__file__).resolve().parents[1] / "internal/nhi/provider-contracts.json")
    parser.add_argument("--online", action="store_true", help="Read the current public entry and its referenced assets; never log in or call medical APIs")
    parser.add_argument("--output", type=Path, help="Optional JSON evidence report; no public response bodies are saved")
    args = parser.parse_args()
    try:
        registry = read_registry(args.registry)
        result = online_check(registry) if args.online else {"status": "registry_valid", "contract_revision": registry["contract_revision"], "categories": len(registry["contracts"]), "assets": len(registry["assets"]), "network_checked": False}
    except (OSError, ValueError, KeyError, TypeError) as error:
        result = {"status": "invalid", "error_code": error.args[0] if isinstance(error, ContractCheckError) else "registry_read_failed"}
    output = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(output, encoding="utf-8")
    else:
        sys.stdout.write(output)
    return 0 if result["status"] in ("unchanged", "registry_valid") else 2 if result["status"] == "changed" else 3


if __name__ == "__main__":
    raise SystemExit(main())

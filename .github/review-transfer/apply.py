# Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
# Temporary, content-addressed transport; removed before integration review.
import base64
import hashlib
import json
import lzma
from pathlib import Path

root = Path.cwd().resolve()
transport = root / '.github/review-transfer'
encoded = ''.join((transport / name).read_text().strip() for name in ['part-a.b64', 'part-b.b64', 'part-c.b64'])
raw = lzma.decompress(base64.b64decode(encoded, validate=True))
expected = '71c4aebe9096a860243f37d542e32eea0d2ea40eeab3789c8694587d6bcd1e44'
if hashlib.sha256(raw).hexdigest() != expected:
    raise RuntimeError('Review transport checksum mismatch; no application files changed')
manifest = json.loads(raw)
if manifest['format'] != 1:
    raise RuntimeError('Unsupported review format')

def safe_path(name):
    relative = Path(name)
    if relative.is_absolute() or '..' in relative.parts or relative.parts[0] == '.github':
        raise RuntimeError('Invalid review destination: ' + name)
    path = root / relative
    if not path.resolve().is_relative_to(root) or path.is_symlink():
        raise RuntimeError('Unsafe review destination: ' + name)
    return path

for entry in manifest['check_existing']:
    path = safe_path(entry['path'])
    if hashlib.sha256(path.read_bytes()).hexdigest() != entry['after']:
        raise RuntimeError('Uploaded file differs from verified local source: ' + entry['path'])

pending = []
for entry in manifest['files']:
    path = safe_path(entry['path'])
    if entry['before'] is None:
        if path.exists():
            raise RuntimeError('New review destination already exists: ' + entry['path'])
        original = b''
    else:
        original = path.read_bytes()
        if hashlib.sha256(original).hexdigest() != entry['before']:
            raise RuntimeError('Review baseline changed: ' + entry['path'])
    lines = original.decode('utf-8').splitlines(keepends=True)
    previous = len(lines)
    for start, end, replacement in reversed(entry['edits']):
        if not 0 <= start <= end <= previous:
            raise RuntimeError('Invalid review edit range: ' + entry['path'])
        lines[start:end] = replacement.splitlines(keepends=True)
        previous = start
    result = ''.join(lines).encode('utf-8')
    if hashlib.sha256(result).hexdigest() != entry['after']:
        raise RuntimeError('Review result checksum mismatch: ' + entry['path'])
    pending.append((path, result))

for path, result in pending:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(result)
    print('Verified source:', path.relative_to(root))
print('All uploaded and applied files match the local verification snapshot.')

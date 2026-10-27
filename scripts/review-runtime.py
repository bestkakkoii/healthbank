# Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.
"""Review-branch helper; exact source archives are built before invoking this file."""
import datetime
import gzip
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tarfile

root = Path.cwd()
temp = Path(os.environ['RUNNER_TEMP'])
revision = 'e9df86ca6991b779ee50bc744ad8bbdeefa9bae6'
mysql_revision = 'c9e0863414a0e47bd708fcbc93d4adc611d15a5e'
lock = json.loads((root / 'dependencies.lock.json').read_text())
for package in ['react', 'react-dom', 'scheduler']:
    source = temp / f'react-{revision}' / 'build/node_modules' / package
    if not (source / 'package.json').is_file():
        raise RuntimeError(f'Missing built package: {source}')
    destination = root / 'web/vendor' / f'{package}-main.tgz'
    with destination.open('wb') as raw, gzip.GzipFile(fileobj=raw, mode='wb', mtime=0, filename='') as zipped, tarfile.open(fileobj=zipped, mode='w') as archive:
        for path in sorted(source.rglob('*')):
            if path.is_file():
                info = archive.gettarinfo(str(path), arcname='package/' + path.relative_to(source).as_posix())
                info.uid = info.gid = 0
                info.uname = info.gname = ''
                info.mtime = 0
                info.mode = 0o644
                with path.open('rb') as content:
                    archive.addfile(info, content)
    previous = lock['vendored_packages'][destination.name]
    digest = hashlib.sha256(destination.read_bytes()).hexdigest()
    lock['vendored_packages'][destination.name] = digest
    bootstrap = root / 'scripts/bootstrap-upstream.sh'
    bootstrap.write_text(bootstrap.read_text().replace(previous, digest))

module = json.loads(subprocess.check_output(['go', 'list', '-mod=mod', '-m', '-json', 'github.com/go-sql-driver/mysql'], text=True))
if not module.get('Sum'):
    raise RuntimeError('Updated MySQL module has no verified Go checksum')
for source in lock['sources']:
    if source['name'] == 'React / React DOM / Scheduler':
        source['commit'] = revision
        source['archive_sha256'] = hashlib.sha256((temp / 'react.tar.gz').read_bytes()).hexdigest()
        source['package_versions'] = [json.loads((temp / f'react-{revision}' / 'build/node_modules' / name / 'package.json').read_text())['version'] for name in ['react', 'react-dom', 'scheduler']]
    if source['name'] == 'MySQL Go driver':
        source['commit'] = mysql_revision
for section in ['go_runtime_modules', 'go_module_inventory']:
    for entry in lock[section]:
        if entry['module'] == 'github.com/go-sql-driver/mysql':
            entry.update(version=module['Version'], sum=module['Sum'], upstream_commit=mysql_revision)
            if 'upstream_head_version' in entry:
                entry['upstream_head_version'] = module['Version']

package_lock = root / 'web/package-lock.json'
packages = json.loads(package_lock.read_text())
for name in ['react', 'react-dom', 'scheduler']:
    packages['packages'].pop('node_modules/' + name, None)
package_lock.write_text(json.dumps(packages, indent=2) + '\n')
subprocess.run(['npm', '--prefix', 'web', 'install', '--package-lock-only', '--ignore-scripts'], check=True)
packages = json.loads(package_lock.read_text())['packages']
for entry in lock['frontend_direct_packages']:
    if entry['name'] in ['react', 'react-dom', 'scheduler']:
        package = packages['node_modules/' + entry['name']]
        entry.update(upstream_commit=revision)
        for key in ['version', 'integrity', 'resolved']:
            entry[key] = package[key]
lock['runtime_refresh'] = {
    'checked_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
    'react_commit': revision, 'mysql_commit': mysql_revision,
    'scope': 'Runtime-only refresh; earlier full-graph audit timestamps remain historical.',
    'react_build_java': subprocess.check_output(['java', '--version'], text=True).strip(),
    'compatibility_exception': 'React pinned Closure Compiler rejects stderr emitted by the runner default JDK. Isolate its source build to available Temurin 21; do not suppress stderr or alter application checks.'
}
(root / 'dependencies.lock.json').write_text(json.dumps(lock, indent=2, ensure_ascii=False) + '\n')

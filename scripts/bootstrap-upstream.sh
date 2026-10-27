#!/usr/bin/env bash
# Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
# Developer/CI toolchain bootstrap; end users run the precompiled executable.
set -euo pipefail
repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
task_tools="$repo_root/.tools"
go_revision="41cb39b794bfe81144930746ccf72440a0382adb"
ts_revision="49e69830214c55a8cc361aa3c4e7412d0afd02fe"
esbuild_revision="f6058f8364fe7ab91ca57a83e02577ed74c9cae4"
govulncheck_revision="e672cbe8fbc8c76e28fcb6db0efd9ce354e0dac5"
node_version="26.12.1"
npm_version="12.2.1"
mkdir -p "$task_tools/downloads" "$task_tools/bin" "$task_tools/build" "$task_tools/toolchains"
unset GOROOT
export GOENV=off GOWORK=off GOTOOLCHAIN=local
export GOCACHE="$task_tools/cache/go-build"
export GOPATH="$task_tools/cache/gopath"
export GOMAXPROCS="${HEALTHBANK_BUILD_JOBS:-4}"
for command in curl python3 tar; do command -v "$command" >/dev/null; done
if [[ "$(uname -s)" != Linux || "$(uname -m)" != x86_64 ]]; then
  printf '%s\n' 'This pinned bootstrap targets Linux x86_64 build hosts. Linux arm64 application releases are cross-compiled.' >&2
  exit 2
fi
verify_sha256() {
  python3 - "$1" "$2" <<'PYHASH'
import hashlib,pathlib,sys
p=pathlib.Path(sys.argv[1])
if hashlib.sha256(p.read_bytes()).hexdigest()!=sys.argv[2]:
    raise SystemExit("Source archive checksum mismatch: "+str(p)+"; retain it for investigation and download the pinned source again.")
PYHASH
}
download_verified() {
  local download_url="$1" download_file="$2" expected_sha="$3" temporary_file
  if [[ -f "$download_file" ]]; then
    verify_sha256 "$download_file" "$expected_sha"
    return
  fi
  temporary_file="$(mktemp "$download_file.part.XXXXXX")"
  if ! curl --fail --location --retry 2 --connect-timeout 20 --max-time 300 "$download_url" -o "$temporary_file"; then
    printf 'Source download failed; incomplete bytes retained at %s.\n' "$temporary_file" >&2
    return 1
  fi
  verify_sha256 "$temporary_file" "$expected_sha"
  mv -- "$temporary_file" "$download_file"
}
activate_toolchain() {
  local target="$1" link="$2" temporary_link
  # Preserve older directory-based installations when moving to immutable paths.
  if [[ -d "$link" && ! -L "$link" ]]; then
    printf 'Existing tool path %s is a directory. Move it aside and rerun; it has not been overwritten.\n' "$link" >&2
    return 1
  fi
  temporary_link="$link.next.$$"
  ln -s -- "$target" "$temporary_link"
  mv -Tf -- "$temporary_link" "$link"
}
node_install="$task_tools/toolchains/node-v$node_version-linux-x64"
if [[ ! -x "$node_install/bin/node" ]]; then
  node_archive="$task_tools/downloads/node-v$node_version-linux-x64.tar.xz"
  download_verified "https://nodejs.org/dist/v$node_version/node-v$node_version-linux-x64.tar.xz" "$node_archive" "6170b2d474e1c626f43cff3adc118df9fe3b573258a4fc3d850c8dbc6b079bbd"
  tar -xJf "$node_archive" -C "$task_tools/toolchains"
fi
if [[ "$("$node_install/bin/node" --version)" != "v$node_version" ]]; then
  printf '%s\n' 'Installed Node.js does not match the pinned version.' >&2
  exit 2
fi
npm_install="$task_tools/toolchains/npm-$npm_version"
if [[ ! -f "$npm_install/package/bin/npm-cli.js" ]]; then
  npm_archive="$task_tools/downloads/npm-$npm_version.tgz"
  download_verified "https://registry.npmjs.org/npm/-/npm-$npm_version.tgz" "$npm_archive" "228bd64aaef23270dbdbd2cf3d5ef5c467602ef38c4066246cef1984adab9a67"
  mkdir -p "$npm_install"
  tar -xzf "$npm_archive" -C "$npm_install"
fi
activate_toolchain "$node_install/bin/node" "$task_tools/bin/node"
activate_toolchain "$npm_install/package/bin/npm-cli.js" "$task_tools/bin/npm"
activate_toolchain "$npm_install/package/bin/npx-cli.js" "$task_tools/bin/npx"
export PATH="$task_tools/bin:$PATH"
if [[ "$(npm --version)" != "$npm_version" ]]; then
  printf '%s\n' 'Installed npm does not match the pinned version.' >&2
  exit 2
fi
go_install="$task_tools/toolchains/go-$go_revision"
if [[ ! -x "$go_install/bin/go" ]]; then
  bootstrap_archive="$task_tools/downloads/go1.27.3.linux-amd64.tar.gz"
  download_verified "https://go.dev/dl/go1.27.3.linux-amd64.tar.gz" "$bootstrap_archive" "65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40"
  mkdir -p "$task_tools/bootstrap"
  tar -xzf "$bootstrap_archive" -C "$task_tools/bootstrap"
  go_archive="$task_tools/downloads/go-$go_revision.tar.gz"
  download_verified "https://codeload.github.com/golang/go/tar.gz/$go_revision" "$go_archive" "73ac97a98dc60355c3f4eb205bc032ddc3191f6ea8a0aaff31cf02f79d55191e"
  tar -xzf "$go_archive" -C "$task_tools/build"
  printf 'devel go1.28-%s\n' "$go_revision" > "$task_tools/build/go-$go_revision/VERSION"
  (cd "$task_tools/build/go-$go_revision/src"; GOROOT_BOOTSTRAP="$task_tools/bootstrap/go" ./make.bash)
  mv -- "$task_tools/build/go-$go_revision" "$go_install"
fi
if ! "$go_install/bin/go" version | python3 -c "import sys; sys.exit(0 if '$go_revision' in sys.stdin.read() else 1)"; then
  printf '%s\n' 'Installed Go does not match the pinned revision.' >&2
  exit 2
fi
activate_toolchain "$go_install" "$task_tools/go"
export PATH="$task_tools/go/bin:$PATH"
tool_uses_pinned_go() {
  local binary="$1"
  [[ -x "$binary" ]] && "$go_install/bin/go" version -m "$binary" | python3 -c "import sys; sys.exit(0 if '$go_revision' in sys.stdin.readline() else 1)"
}
ts_build_key="$ts_revision+go-$go_revision"
ts_install="$task_tools/toolchains/tsc-$ts_build_key"
if ! tool_uses_pinned_go "$ts_install"; then
  ts_archive="$task_tools/downloads/typescript-$ts_revision.tar.gz"
  download_verified "https://codeload.github.com/microsoft/TypeScript/tar.gz/$ts_revision" "$ts_archive" "470cc78cd0144272f1b9437055437ba4e0147c0bc53db4a9759b55933a94d61d"
  tar -xzf "$ts_archive" -C "$task_tools/build"
  (cd "$task_tools/build/TypeScript-$ts_revision/tsc"; "$go_install/bin/go" build -buildvcs=false -o "$ts_install.next" ./cmd/tsc)
  tool_uses_pinned_go "$ts_install.next"
  mv -- "$ts_install.next" "$ts_install"
fi
activate_toolchain "$ts_install" "$task_tools/bin/tsc"
printf '%s\n' "$ts_build_key" > "$task_tools/tsc.revision"
esbuild_build_key="$esbuild_revision+go-$go_revision"
esbuild_install="$task_tools/toolchains/esbuild-$esbuild_build_key"
if ! tool_uses_pinned_go "$esbuild_install"; then
  mkdir -p "$task_tools/build/esbuild-bin"
  GOBIN="$task_tools/build/esbuild-bin" "$go_install/bin/go" install "github.com/evanw/esbuild/cmd/esbuild@$esbuild_revision"
  tool_uses_pinned_go "$task_tools/build/esbuild-bin/esbuild"
  mv -- "$task_tools/build/esbuild-bin/esbuild" "$esbuild_install"
fi
activate_toolchain "$esbuild_install" "$task_tools/bin/esbuild"
printf '%s\n' "$esbuild_build_key" > "$task_tools/esbuild.revision"
govulncheck_build_key="$govulncheck_revision+go-$go_revision"
govulncheck_install="$task_tools/toolchains/govulncheck-$govulncheck_build_key"
if ! tool_uses_pinned_go "$govulncheck_install"; then
  mkdir -p "$task_tools/build/govulncheck-bin"
  GOBIN="$task_tools/build/govulncheck-bin" "$go_install/bin/go" install "golang.org/x/vuln/cmd/govulncheck@$govulncheck_revision"
  tool_uses_pinned_go "$task_tools/build/govulncheck-bin/govulncheck"
  mv -- "$task_tools/build/govulncheck-bin/govulncheck" "$govulncheck_install"
fi
activate_toolchain "$govulncheck_install" "$task_tools/bin/govulncheck"
# Upstream npm archives are checked in and lockfile-verified.
python3 - "$repo_root" <<'PYREACT'
import hashlib,pathlib,sys
root=pathlib.Path(sys.argv[1])/'web/vendor'
expected={
 'react-main.tgz':'c1d9d0e081f89765454b79b8b7aa81d2d42ae98c06a96914d952975bbda8b2a5',
 'react-dom-main.tgz':'efaaf57f2b0f8e39b27b1759c0d08401f29ac670ce0877b808cd17928d35319d',
 'scheduler-main.tgz':'c64e8d8cb716ae3bcef457a833127ab9ad5c55d7fbf3bb89030f368a2f81e17c',
 'eslint-main.tgz':'eb1cd508a65aaf6c0493eeaf72eefac27cffb3606d7d375d41f2e258c2311e43',
 'eslint-js-main.tgz':'8ecd352ee99b1c8291bee007f43d58356ae5057782de9635563aec2e604dcee2',
}
for name,digest in expected.items():
    if hashlib.sha256((root/name).read_bytes()).hexdigest()!=digest:
        raise SystemExit("Vendored package checksum mismatch: "+name)
PYREACT
node --version
npm --version
go version
"$task_tools/bin/tsc" --version
"$task_tools/bin/esbuild" --version
printf '%s\n' 'Pinned tools are ready. Add .tools/bin and .tools/go/bin to PATH, then run npm --prefix web ci and npm --prefix web run build.'

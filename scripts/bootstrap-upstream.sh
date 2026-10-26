#!/usr/bin/env bash
# Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
# Developer/CI toolchain bootstrap; end users run the precompiled executable.
set -euo pipefail
repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
task_tools="$repo_root/.tools"
go_revision="dac9fe5ea9bed1733cb27fbd91aa43457de980b4"
ts_revision="49e69830214c55a8cc361aa3c4e7412d0afd02fe"
esbuild_revision="f6058f8364fe7ab91ca57a83e02577ed74c9cae4"
mkdir -p "$task_tools/downloads" "$task_tools/bin" "$task_tools/build"
export GOCACHE="$task_tools/cache/go-build"
export GOPATH="$task_tools/cache/gopath"
export GOTOOLCHAIN=local
export GOMAXPROCS="${HEALTHBANK_BUILD_JOBS:-4}"
for command in curl python3 tar node npm; do command -v "$command" >/dev/null; done
if [[ "$(uname -s)" != Linux || "$(uname -m)" != x86_64 ]]; then
  printf '%s\n' 'This pinned bootstrap targets Linux x86_64 build hosts. Linux arm64 application releases are cross-compiled.' >&2
  exit 2
fi
download_verified() {
  local download_url="$1" download_file="$2" expected_sha="$3"
  if [[ ! -f "$download_file" ]]; then
    curl --fail --location --retry 2 --connect-timeout 20 --max-time 300 "$download_url" -o "$download_file"
  fi
  python3 - "$download_file" "$expected_sha" <<'PY'
import hashlib,pathlib,sys
p=pathlib.Path(sys.argv[1])
if hashlib.sha256(p.read_bytes()).hexdigest()!=sys.argv[2]:
    raise SystemExit("Source archive checksum mismatch: "+p.name)
PY
}
if [[ ! -x "$task_tools/go/bin/go" ]]; then
  download_verified "https://go.dev/dl/go1.27.3.linux-amd64.tar.gz" "$task_tools/downloads/go-bootstrap.tar.gz" "65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40"
  mkdir -p "$task_tools/bootstrap"
  tar -xzf "$task_tools/downloads/go-bootstrap.tar.gz" -C "$task_tools/bootstrap"
  download_verified "https://codeload.github.com/golang/go/tar.gz/$go_revision" "$task_tools/downloads/go-main.tar.gz" "c23747152b90adc07fecc0d068bc2b95c6a5bf51639547da553cda151996ae9f"
  tar -xzf "$task_tools/downloads/go-main.tar.gz" -C "$task_tools/build"
  printf 'devel go1.28-%s\n' "$go_revision" > "$task_tools/build/go-$go_revision/VERSION"
  (cd "$task_tools/build/go-$go_revision/src"; GOROOT_BOOTSTRAP="$task_tools/bootstrap/go" ./make.bash)
  mv "$task_tools/build/go-$go_revision" "$task_tools/go"
fi
if ! "$task_tools/go/bin/go" version | python3 -c "import sys; sys.exit(0 if '$go_revision' in sys.stdin.read() else 1)"; then
  printf '%s\n' 'Existing .tools/go does not match the pinned revision.' >&2
  exit 2
fi
export PATH="$task_tools/go/bin:$task_tools/bin:$PATH"
if [[ ! -x "$task_tools/bin/tsc" || "$(cat "$task_tools/tsc.revision" 2>/dev/null || true)" != "$ts_revision" ]]; then
  download_verified "https://codeload.github.com/microsoft/TypeScript/tar.gz/$ts_revision" "$task_tools/downloads/typescript-main.tar.gz" "470cc78cd0144272f1b9437055437ba4e0147c0bc53db4a9759b55933a94d61d"
  tar -xzf "$task_tools/downloads/typescript-main.tar.gz" -C "$task_tools/build"
  (cd "$task_tools/build/TypeScript-$ts_revision/tsc"; go build -buildvcs=false -o "$task_tools/bin/tsc" ./cmd/tsc)
  printf '%s\n' "$ts_revision" > "$task_tools/tsc.revision"
fi
if [[ ! -x "$task_tools/bin/esbuild" || "$(cat "$task_tools/esbuild.revision" 2>/dev/null || true)" != "$esbuild_revision" ]]; then
  GOBIN="$task_tools/bin" go install "github.com/evanw/esbuild/cmd/esbuild@$esbuild_revision"
  printf '%s\n' "$esbuild_revision" > "$task_tools/esbuild.revision"
fi
# React's source-built npm archives are checked in and lockfile-verified.
python3 - "$repo_root" <<'PY'
import hashlib,pathlib,sys
root=pathlib.Path(sys.argv[1])/'web/vendor'
expected={
 'react-main.tgz':'c1d9d0e081f89765454b79b8b7aa81d2d42ae98c06a96914d952975bbda8b2a5',
 'react-dom-main.tgz':'02f22648ec195371d9772c667cda0ce211eeefeac57e746a3d4d4d151996dfcb',
 'scheduler-main.tgz':'c64e8d8cb716ae3bcef457a833127ab9ad5c55d7fbf3bb89030f368a2f81e17c',
}
for name,digest in expected.items():
    if hashlib.sha256((root/name).read_bytes()).hexdigest()!=digest:
        raise SystemExit("Vendored React package checksum mismatch: "+name)
PY
go version
"$task_tools/bin/tsc" --version
"$task_tools/bin/esbuild" --version
printf '%s\n' 'Pinned tools are ready. Run npm --prefix web ci, then npm --prefix web run build.'

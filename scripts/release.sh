#!/usr/bin/env bash
# Copyright 2026 bestkakkoii llc.
# SPDX-License-Identifier: MIT
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
go_command="${GO:-$repo_root/.tools/go/bin/go}"
python_command="${PYTHON:-python3}"
node_command="${NODE:-$repo_root/.tools/bin/node}"
command -v "$python_command" >/dev/null
version="$("$python_command" "$repo_root/scripts/release.py" version "${1:-${VERSION:-}}")"
archive_version="${version#v}"
output_dir="${RELEASE_DIR:-$repo_root/dist/releases}"
identity="$("$python_command" "$repo_root/scripts/release.py" preflight "$repo_root" "${COMMIT:-}" "${SOURCE_DATE_EPOCH:-}")"
IFS=$'\t' read -r commit source_epoch <<< "$identity"
if [[ -n "${GOFLAGS:-}" || -n "${GOEXPERIMENT:-}" ]]; then
  printf '%s\n' 'Official releases require default GOFLAGS and GOEXPERIMENT; use go build separately for custom builds.' >&2
  exit 2
fi
unset GOROOT
export GOENV=off GOWORK=off GOTOOLCHAIN=local GO111MODULE=on
release_root_documents=(
  LICENSE README.md README_AI.md AGENTS.md CHANGELOG.md CONTRIBUTING.md
  DEPENDENCIES.md THIRD_PARTY_NOTICES.md SECURITY.md dependencies.lock.json
)
release_documents=(
  docs/deployment.zh-TW.md
  docs/clinical-reference.zh-TW.md
  docs/comparison-engine.md
  docs/privacy.zh-TW.md
  docs/acceptance.zh-TW.md
  docs/nhi_protocol.md
  docs/provider-data.md
  docs/provider-documents.md
  docs/data-workspace.zh-TW.md
  docs/archive-restoration.zh-TW.md
  docs/device-health-cache.zh-TW.md
  docs/provider-source-inventory.json
  docs/browser-acceptance.md
  docs/verification.zh-TW.md
  docs/quality-audit.zh-TW.md
  docs/deep-audit.zh-TW.md
  docs/roadmap.md
)
release_document_assets=(
  docs/assets/healthbank-banner.svg
  docs/screenshots/history-comparison.jpg
  web/tests/browser/toolchain.json
)

command -v "$go_command" >/dev/null
command -v "$node_command" >/dev/null
go_version="$("$go_command" version)"
"$python_command" "$repo_root/scripts/release.py" verify-go "$repo_root" "$go_version"
"$python_command" -W error "$repo_root/scripts/test_release.py"
if [[ ! -s "$repo_root/internal/webui/dist/index.html" ]]; then
  printf '%s\n' 'Compiled frontend is missing. Build the frontend into internal/webui/dist before releasing.' >&2
  exit 1
fi
"$node_command" "$repo_root/scripts/build-web.mjs" --verify
for required in go.mod go.sum deploy/nginx.healthbank.conf "${release_root_documents[@]}" "${release_documents[@]}" "${release_document_assets[@]}"; do
  if [[ ! -f "$repo_root/$required" ]]; then
    printf 'Required release file is missing: %s\n' "$required" >&2
    exit 1
  fi
done

build_time="$("$python_command" -c 'import datetime,sys; print(datetime.datetime.fromtimestamp(int(sys.argv[1]),datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"))' "$source_epoch")"
mkdir -p -- "$output_dir"
output_dir="$(cd -- "$output_dir" && pwd)"
staging_dir="$(mktemp -d)"
trap 'rm -rf -- "$staging_dir"' EXIT

cd -- "$repo_root"
"$go_command" mod verify
for arch in amd64 arm64; do
  package_name="healthbank_${archive_version}_linux_${arch}"
  package_dir="$staging_dir/$package_name"
  mkdir -p -- "$package_dir/docs" "$package_dir/deploy"
  printf 'Building %s (%s)...\n' "$package_name" "$commit"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" GOAMD64=v1 GOARM64=v8.0 GOWORK=off \
    "$go_command" build -mod=readonly -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=$version -X main.commit=$commit -X main.buildTime=$build_time" \
    -o "$package_dir/healthbank" ./cmd/healthbank
  chmod 0755 "$package_dir/healthbank"
  cp -- "${release_root_documents[@]}" "$package_dir/"
  cp -- "${release_documents[@]}" "$package_dir/docs/"
  for asset in "${release_document_assets[@]}"; do
    mkdir -p -- "$package_dir/$(dirname -- "$asset")"
    cp -- "$asset" "$package_dir/$asset"
  done
  cp -- deploy/nginx.healthbank.conf "$package_dir/deploy/"
  for optional in NOTICE; do
    if [[ -f "$optional" ]]; then
      cp -- "$optional" "$package_dir/"
    fi
  done
  cp -R -- licenses "$package_dir/"
  printf '%s\n' "$version" > "$package_dir/VERSION"
  {
    printf 'version=%s\ncommit=%s\nbuild_time=%s\nsource_date_epoch=%s\n' "$version" "$commit" "$build_time" "$source_epoch"
    printf '%s\n' "$go_version"
    printf 'CGO_ENABLED=0\nGOOS=linux\nGOARCH=%s\n' "$arch"
    if [[ "$arch" == amd64 ]]; then
      printf 'GOAMD64=v1\n'
    else
      printf 'GOARM64=v8.0\n'
    fi
  } > "$package_dir/BUILDINFO"
  "$python_command" scripts/release.py binary "$package_dir" "$output_dir/$package_name.tar.gz" "$source_epoch"
done

"$python_command" scripts/release.py source "$repo_root" "$output_dir/healthbank_${archive_version}_source.tar.gz" "$source_epoch" "healthbank_${archive_version}"
"$python_command" scripts/release.py preflight "$repo_root" "$commit" "$source_epoch" >/dev/null
"$python_command" scripts/release.py checksums "$output_dir" "healthbank_${archive_version}_"
printf 'Release artifacts are ready in %s\n' "$output_dir"
printf '%s\n' 'No files were uploaded or published to GitHub.'

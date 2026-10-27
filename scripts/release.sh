#!/usr/bin/env bash
# Copyright 2026 bestkakkoii llc.
# SPDX-License-Identifier: MIT
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
version="${1:-${VERSION:-v1.0.3-rc.5}}"
if [[ ! "$version" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
  printf '%s\n' 'Version must be a semantic version such as v1.0.0 or v1.0.0-rc.1.' >&2
  exit 2
fi
version="v${version#v}"
archive_version="${version#v}"
output_dir="${RELEASE_DIR:-$repo_root/dist/releases}"
go_command="${GO:-go}"
python_command="${PYTHON:-python3}"
node_command="${NODE:-$repo_root/.tools/bin/node}"
release_root_documents=(
  LICENSE README.md README_AI.md AGENTS.md CHANGELOG.md CONTRIBUTING.md
  DEPENDENCIES.md THIRD_PARTY_NOTICES.md SECURITY.md
)
release_documents=(
  docs/deployment.zh-TW.md
  docs/clinical-reference.zh-TW.md
  docs/nhi_protocol.md
  docs/verification.zh-TW.md
  docs/quality-audit.zh-TW.md
  docs/roadmap.md
)
release_document_assets=(
  docs/assets/healthbank-banner.svg
  docs/screenshots/history-comparison.jpg
)

command -v "$go_command" >/dev/null
command -v "$python_command" >/dev/null
command -v "$node_command" >/dev/null
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

commit="${COMMIT:-}"
if [[ -z "$commit" ]]; then
  commit="$(git -C "$repo_root" rev-parse --verify HEAD 2>/dev/null || printf 'uncommitted')"
fi
if [[ ! "$commit" =~ ^[0-9a-zA-Z._-]+$ ]]; then
  printf '%s\n' 'COMMIT must contain only letters, digits, dots, underscores or dashes.' >&2
  exit 2
fi
source_epoch="${SOURCE_DATE_EPOCH:-$(date -u +%s)}"
if [[ ! "$source_epoch" =~ ^[0-9]+$ ]]; then
  printf '%s\n' 'SOURCE_DATE_EPOCH must be an integer Unix timestamp.' >&2
  exit 2
fi
build_time="$("$python_command" -c 'import datetime,sys; print(datetime.datetime.fromtimestamp(int(sys.argv[1]),datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"))' "$source_epoch")"
mkdir -p -- "$output_dir"
output_dir="$(cd -- "$output_dir" && pwd)"
staging_dir="$(mktemp -d)"
trap 'rm -rf -- "$staging_dir"' EXIT

cd -- "$repo_root"
for arch in amd64 arm64; do
  package_name="healthbank_${archive_version}_linux_${arch}"
  package_dir="$staging_dir/$package_name"
  mkdir -p -- "$package_dir/docs" "$package_dir/deploy"
  printf 'Building %s (%s)...\n' "$package_name" "$commit"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" GOAMD64=v1 GOARM64=v8.0 GOWORK=off \
    "$go_command" build -trimpath -buildvcs=false \
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
  for optional in NOTICE dependencies.lock.json; do
    if [[ -f "$optional" ]]; then
      cp -- "$optional" "$package_dir/"
    fi
  done
  if [[ -d licenses ]]; then
    cp -R -- licenses "$package_dir/"
  fi
  printf '%s\n' "$version" > "$package_dir/VERSION"
  {
    printf 'version=%s\ncommit=%s\nbuild_time=%s\nsource_date_epoch=%s\n' "$version" "$commit" "$build_time" "$source_epoch"
    "$go_command" version
    printf 'CGO_ENABLED=0\nGOOS=linux\nGOARCH=%s\n' "$arch"
  } > "$package_dir/BUILDINFO"
  "$python_command" scripts/release.py binary "$package_dir" "$output_dir/$package_name.tar.gz" "$source_epoch"
done

"$python_command" scripts/release.py source "$repo_root" "$output_dir/healthbank_${archive_version}_source.tar.gz" "$source_epoch" "healthbank_${archive_version}"
"$python_command" scripts/release.py checksums "$output_dir" "healthbank_${archive_version}_"
printf 'Release artifacts are ready in %s\n' "$output_dir"
printf '%s\n' 'No files were uploaded or published to GitHub.'

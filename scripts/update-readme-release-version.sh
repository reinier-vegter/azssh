#!/usr/bin/env bash

set -euo pipefail

usage() {
  echo "Usage: $0 <version> [--check]" >&2
  exit 2
}

version="${1:-}"
mode="${2:-update}"

[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || usage
[[ "$mode" == "update" || "$mode" == "--check" ]] || usage

if [[ "$mode" == "update" ]]; then
  VERSION="$version" perl -0pi -e '
    s{
      https://github\.com/reinier-vegter/azssh/releases/download/v[^/]+/
      azssh_v[^_]+_(linux|darwin)_(amd64|arm64)\.gz
    }{"https://github.com/reinier-vegter/azssh/releases/download/$ENV{VERSION}/azssh_$ENV{VERSION}_$1_$2.gz"}gex;
  ' README.md
fi

expected_links=0
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  os="${target%/*}"
  arch="${target#*/}"
  expected="https://github.com/reinier-vegter/azssh/releases/download/${version}/azssh_${version}_${os}_${arch}.gz"
  if ! grep -Fq "$expected" README.md; then
    echo "README is missing release link: $expected" >&2
    exit 1
  fi
  ((expected_links += 1))
done

actual_links="$(grep -Eo 'https://github\.com/reinier-vegter/azssh/releases/download/v[^/]+/azssh_v[^_]+_(linux|darwin)_(amd64|arm64)\.gz' README.md | sort -u || true)"
actual_link_count="$(printf '%s\n' "$actual_links" | sed '/^$/d' | wc -l)"
if [[ "$actual_link_count" -ne "$expected_links" ]]; then
  echo "README contains unexpected release download links:" >&2
  printf '%s\n' "$actual_links" >&2
  exit 1
fi

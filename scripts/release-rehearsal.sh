#!/usr/bin/env sh
set -eu

usage() {
	echo "usage: $0 <tag> [workspace]" >&2
	echo "set GH_RELEASE_REPO to override the default ersinkoc/Kronos repository" >&2
}

tag="${1:-}"
if [ -z "$tag" ]; then
	usage
	exit 2
fi

workspace="${2:-release-evidence/$tag}"
repo="${GH_RELEASE_REPO:-${GITHUB_REPOSITORY:-ersinkoc/Kronos}}"
assets_dir="$workspace/assets"
evidence_dir="$workspace/evidence"

if ! command -v gh >/dev/null 2>&1; then
	echo "gh is required to download release assets and verify attestations" >&2
	exit 1
fi

mkdir -p "$assets_dir" "$evidence_dir"

gh release download "$tag" --repo "$repo" --dir "$assets_dir"

if [ -z "${GH_ATTESTATION_REPO:-}" ]; then
	export GH_ATTESTATION_REPO="$repo"
fi
export KRONOS_RELEASE_TAG="$tag"

./scripts/archive-release-evidence.sh "$assets_dir" "$evidence_dir"

echo "release rehearsal evidence archived in $evidence_dir"

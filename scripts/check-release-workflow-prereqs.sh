#!/usr/bin/env sh
set -eu

repo="${GH_RELEASE_REPO:-${GITHUB_REPOSITORY:-ersinkoc/Kronos}}"
secret_name="${KRONOS_RELEASE_TAG_PUBLIC_KEY_SECRET:-KRONOS_RELEASE_TAG_PUBLIC_KEY}"
signer_fingerprint="${KRONOS_RELEASE_TAG_SIGNER_FINGERPRINT:-5A9E4321B35B583DAFE3DC11F9936A44B1CF413C}"

if ! command -v gh >/dev/null 2>&1; then
	echo "gh is required to check release workflow prerequisites" >&2
	exit 1
fi

if [ -z "$repo" ]; then
	echo "GH_RELEASE_REPO or GITHUB_REPOSITORY is required" >&2
	exit 1
fi

if ! gh secret list --repo "$repo" | awk '{print $1}' | grep -qx "$secret_name"; then
	echo "GitHub secret missing for tagged releases: $secret_name in $repo" >&2
	echo "Set it to the trusted armored public GPG key for release tag signer $signer_fingerprint." >&2
	exit 1
fi

echo "release workflow prerequisites OK for $repo"

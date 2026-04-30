#!/usr/bin/env sh
set -eu

release_dir="${1:-bin}"
evidence_dir="${2:-release-evidence}"
repo="${GH_ATTESTATION_REPO:-}"
workflow="${GH_ATTESTATION_WORKFLOW:-.github/workflows/release.yml}"
release_tag="${KRONOS_RELEASE_TAG:-unknown}"

if [ ! -d "$release_dir" ]; then
	echo "release directory not found: $release_dir" >&2
	exit 1
fi

mkdir -p "$evidence_dir"

run_and_capture() {
	name="$1"
	shift
	log="$evidence_dir/$name.log"
	"$@" >"$log" 2>&1
}

run_and_capture checksum ./scripts/verify-release.sh "$release_dir"
run_and_capture signatures ./scripts/verify-signatures.sh "$release_dir"
if [ "$release_tag" != "unknown" ]; then
	run_and_capture tag-signature ./scripts/verify-release-tag.sh "$release_tag"
else
	printf '%s\n' "KRONOS_RELEASE_TAG not set; release tag signature verification not run." >"$evidence_dir/tag-signature.log"
fi

digests="$evidence_dir/artifact-digests.txt"
: >"$digests"
found=0
for artifact in "$release_dir"/kronos-*; do
	[ -f "$artifact" ] || continue
	case "$artifact" in
		*.sha256 | *.json | *.sig | *.pem | *.bundle | *.tmp) continue ;;
	esac
	found=1
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$artifact" >>"$digests"
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$artifact" >>"$digests"
	else
		echo "sha256sum or shasum is required to archive release evidence" >&2
		exit 1
	fi
done

if [ "$found" -eq 0 ]; then
	echo "no release payloads found in $release_dir" >&2
	exit 1
fi

attestation_log="$evidence_dir/attestations.log"
artifact_abs_path() {
	artifact_path="$1"
	artifact_dir=$(dirname "$artifact_path")
	artifact_base=$(basename "$artifact_path")
	printf '%s/%s\n' "$(CDPATH= cd "$artifact_dir" && pwd -P)" "$artifact_base"
}

verify_attestation() {
	artifact="$1"
	label="$2"
	predicate_type="$3"
	cosign_type="$4"

	echo "## $artifact $label"
	if gh attestation verify "$artifact" --repo "$repo" --signer-workflow "$workflow" --predicate-type "$predicate_type"; then
		echo
		return 0
	fi

	echo "gh attestation verify failed; retrying with cosign bundle verification"
	if ! command -v cosign >/dev/null 2>&1; then
		echo "cosign is required when GitHub attestation verification needs bundle fallback" >&2
		return 1
	fi

	artifact_abs=$(artifact_abs_path "$artifact")
	workflow_identity="https://github.com/${repo}/${workflow}@.*"
	tmp_dir=$(mktemp -d)
	if ! (cd "$tmp_dir" && gh attestation download "$artifact_abs" --repo "$repo" --predicate-type "$predicate_type"); then
		rm -rf "$tmp_dir"
		return 1
	fi
	bundle=$(find "$tmp_dir" -type f -name 'sha256:*.jsonl' | head -n 1)
	if [ -z "$bundle" ]; then
		echo "no downloaded attestation bundle for $artifact" >&2
		rm -rf "$tmp_dir"
		return 1
	fi
	sed -n '1p' "$bundle" >"$tmp_dir/bundle.json"
	cosign verify-blob-attestation \
		--bundle "$tmp_dir/bundle.json" \
		--certificate-identity-regexp "$workflow_identity" \
		--certificate-oidc-issuer https://token.actions.githubusercontent.com \
		--type "$cosign_type" \
		"$artifact_abs"
	rm -rf "$tmp_dir"
	echo
}

if [ -n "$repo" ]; then
	if ! command -v gh >/dev/null 2>&1; then
		echo "gh is required when GH_ATTESTATION_REPO is set" >&2
		exit 1
	fi
	: >"$attestation_log"
	for artifact in "$release_dir"/kronos-*; do
		[ -f "$artifact" ] || continue
		case "$artifact" in
			*.sha256 | *.json | *.sig | *.pem | *.bundle | *.tmp) continue ;;
		esac
		{
			verify_attestation "$artifact" provenance https://slsa.dev/provenance/v1 slsaprovenance1
			verify_attestation "$artifact" SBOM https://spdx.dev/Document/v2.3 spdx
		} >>"$attestation_log" 2>&1
	done
else
	printf '%s\n' "GH_ATTESTATION_REPO not set; attestation verification not run." >"$attestation_log"
fi

summary="$evidence_dir/summary.txt"
{
	echo "release_tag=$release_tag"
	echo "release_dir=$release_dir"
	echo "evidence_dir=$evidence_dir"
	echo "git_commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
	echo "verified_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	echo "checksum_log=checksum.log"
	echo "signature_log=signatures.log"
	echo "tag_signature_log=tag-signature.log"
	echo "attestation_log=attestations.log"
	echo "digests=artifact-digests.txt"
} >"$summary"

./scripts/verify-release-evidence.sh "$evidence_dir" >/dev/null

echo "release evidence archived in $evidence_dir"

package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseRehearsalDownloadsAndArchivesEvidence(t *testing.T) {
	t.Parallel()

	root := filepath.Dir(mustGetwd(t))
	workspace := filepath.Join(t.TempDir(), "rehearsal")
	binDir := t.TempDir()
	argsPath := filepath.Join(t.TempDir(), "gh.args")

	fakeGH := filepath.Join(binDir, "gh")
	ghScript := `#!/usr/bin/env sh
set -eu
printf '%s\n' "$*" >> "$GH_ARGS_OUT"
if [ "$1" = "release" ] && [ "$2" = "download" ]; then
	dir=""
	while [ "$#" -gt 0 ]; do
		if [ "$1" = "--dir" ]; then
			shift
			dir="$1"
			break
		fi
		shift
	done
	mkdir -p "$dir"
	printf '%s\n' "test release artifact" > "$dir/kronos-linux-amd64"
	chmod +x "$dir/kronos-linux-amd64"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$dir/kronos-linux-amd64" > "$dir/kronos-linux-amd64.sha256"
	else
		shasum -a 256 "$dir/kronos-linux-amd64" > "$dir/kronos-linux-amd64.sha256"
	fi
	for path in \
		"$dir/kronos-linux-amd64.sig" \
		"$dir/kronos-linux-amd64.pem" \
		"$dir/kronos-provenance.json" \
		"$dir/kronos-provenance.json.sig" \
		"$dir/kronos-provenance.json.pem" \
		"$dir/kronos-sbom.json" \
		"$dir/kronos-sbom.json.sig" \
		"$dir/kronos-sbom.json.pem"
	do
		printf '%s\n' "payload" > "$path"
	done
	exit 0
fi
if [ "$1" = "attestation" ] && [ "$2" = "verify" ]; then
	echo "attestation OK"
	exit 0
fi
echo "unexpected gh invocation: $*" >&2
exit 1
`
	if err := os.WriteFile(fakeGH, []byte(ghScript), 0o755); err != nil {
		t.Fatalf("WriteFile(fake gh) error = %v", err)
	}

	fakeCosign := filepath.Join(binDir, "cosign")
	if err := os.WriteFile(fakeCosign, []byte("#!/usr/bin/env sh\necho cosign \"$@\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fake cosign) error = %v", err)
	}
	fakeGit := filepath.Join(binDir, "git")
	gitScript := `#!/usr/bin/env sh
set -eu
case "$1" in
	rev-parse)
		if [ "$2" = "--git-dir" ]; then
			printf '%s\n' .git
			exit 0
		fi
		if [ "$2" = "--short" ]; then
			printf '%s\n' abc1234
			exit 0
		fi
		if [ "$2" = "-q" ] && [ "$3" = "--verify" ]; then
			exit 0
		fi
		;;
	ls-remote)
		exit 0
		;;
	verify-tag)
		printf '%s\n' "tag OK"
		exit 0
		;;
esac
echo "unexpected git invocation: $*" >&2
exit 1
`
	if err := os.WriteFile(fakeGit, []byte(gitScript), 0o755); err != nil {
		t.Fatalf("WriteFile(fake git) error = %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(root, "scripts", "release-rehearsal.sh"), "v0.0.0-test", workspace)
	cmd.Dir = root
	cmd.Env = append(cleanEnv(os.Environ(), "GH_RELEASE_REPO", "GH_ATTESTATION_REPO", "GITHUB_REPOSITORY", "KRONOS_RELEASE_TAG", "COSIGN_CERTIFICATE_IDENTITY_REGEXP"),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GH_ARGS_OUT="+argsPath,
		"GH_RELEASE_REPO=example/kronos",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release-rehearsal.sh error = %v\n%s", err, output)
	}

	summary, err := os.ReadFile(filepath.Join(workspace, "evidence", "summary.txt"))
	if err != nil {
		t.Fatalf("ReadFile(summary) error = %v", err)
	}
	if !strings.Contains(string(summary), "release_tag=v0.0.0-test") ||
		!strings.Contains(string(summary), "tag_signature_log=tag-signature.log") ||
		!strings.Contains(string(summary), "attestation_log=attestations.log") {
		t.Fatalf("summary missing release rehearsal metadata:\n%s", summary)
	}

	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("ReadFile(gh args) error = %v", err)
	}
	if !strings.Contains(string(args), "release download v0.0.0-test --repo example/kronos --dir ") ||
		!strings.Contains(string(args), "attestation verify") {
		t.Fatalf("gh was not invoked for release download and attestation verification:\n%s", args)
	}
}

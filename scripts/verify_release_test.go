package scripts_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVerifyReleaseIgnoresSignatureSidecars(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	artifact := filepath.Join(dir, "kronos-linux-amd64")
	content := []byte("test release artifact\n")
	if err := os.WriteFile(artifact, content, 0o755); err != nil {
		t.Fatalf("WriteFile(artifact) error = %v", err)
	}
	sum := sha256.Sum256(content)
	checksum := fmt.Sprintf("%x  %s\n", sum, artifact)
	if err := os.WriteFile(artifact+".sha256", []byte(checksum), 0o644); err != nil {
		t.Fatalf("WriteFile(checksum) error = %v", err)
	}
	for _, path := range []string{
		artifact + ".sig",
		artifact + ".pem",
		filepath.Join(dir, "kronos-provenance.json"),
		filepath.Join(dir, "kronos-provenance.json.sig"),
		filepath.Join(dir, "kronos-provenance.json.pem"),
		filepath.Join(dir, "kronos-sbom.json"),
		filepath.Join(dir, "kronos-sbom.json.sig"),
		filepath.Join(dir, "kronos-sbom.json.pem"),
	} {
		if err := os.WriteFile(path, []byte("sidecar\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	runVerifyRelease(t, dir)
}

func TestVerifyReleaseIgnoresChecksumFilename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	artifact := filepath.Join(dir, "kronos-linux-amd64")
	content := []byte("test release artifact\n")
	if err := os.WriteFile(artifact, content, 0o755); err != nil {
		t.Fatalf("WriteFile(artifact) error = %v", err)
	}
	sum := sha256.Sum256(content)
	checksum := fmt.Sprintf("%x  bin/kronos-linux-amd64\n", sum)
	if err := os.WriteFile(artifact+".sha256", []byte(checksum), 0o644); err != nil {
		t.Fatalf("WriteFile(checksum) error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(dir, "kronos-provenance.json"),
		filepath.Join(dir, "kronos-sbom.json"),
	} {
		if err := os.WriteFile(path, []byte("metadata\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	runVerifyRelease(t, dir)
}

func runVerifyRelease(t *testing.T, dir string) {
	t.Helper()

	root := filepath.Dir(mustGetwd(t))
	cmd := exec.Command("sh", filepath.Join(root, "scripts", "verify-release.sh"), dir)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify-release.sh error = %v\n%s", err, output)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	return wd
}

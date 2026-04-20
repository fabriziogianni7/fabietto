package handbook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_ok(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "manifest.txt"), "a.md\nb.md\n")
	mustWrite(t, filepath.Join(dir, "a.md"), "alpha")
	mustWrite(t, filepath.Join(dir, "b.md"), "beta")

	out, err := Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "alpha" + separator + "beta"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestLoad_exclude(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "manifest.txt"), "a.md\nwallet.md\n")
	mustWrite(t, filepath.Join(dir, "a.md"), "one")
	mustWrite(t, filepath.Join(dir, "wallet.md"), "two")

	out, err := Load(dir, map[string]bool{"wallet.md": true})
	if err != nil {
		t.Fatal(err)
	}
	if out != "one" {
		t.Fatalf("got %q want one", out)
	}
}

func TestLoad_missingManifest(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, nil)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing manifest error, got %v", err)
	}
}

func TestLoad_missingListedFile(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "manifest.txt"), "ghost.md\n")

	_, err := Load(dir, nil)
	if err == nil || !strings.Contains(err.Error(), "ghost.md") {
		t.Fatalf("expected missing file error, got %v", err)
	}
}

func TestLoad_manifestCommentAndBlank(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "manifest.txt"), "# intro\n\nx.md\n")
	mustWrite(t, filepath.Join(dir, "x.md"), "ok")

	out, err := Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Fatalf("got %q", out)
	}
}

func TestLoad_pathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "manifest.txt"), "../outside.md\n")

	_, err := Load(dir, nil)
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected escape error, got %v", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileRejectsOutOfRootPath(t *testing.T) {
	tmp := t.TempDir()
	outsideFile := filepath.Join(tmp, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to create outside file: %v", err)
	}

	_, err := readFile(outsideFile)
	if err == nil {
		t.Fatalf("expected out-of-root read rejection")
	}
	if !strings.Contains(err.Error(), "outside allowed workspace roots") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteFileRejectsTraversalOutOfRoot(t *testing.T) {
	_, err := writeFile("../../escape.txt", "nope")
	if err == nil {
		t.Fatalf("expected traversal rejection")
	}
	if !strings.Contains(err.Error(), "outside allowed workspace roots") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCommandRejectsMetacharactersAndChaining(t *testing.T) {
	out, err := runCommand("ls; pwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "permission denied") {
		t.Fatalf("expected permission denied, got %q", out)
	}
}

func TestRunCommandAllowlistDenyAndAllowBehavior(t *testing.T) {
	denyOut, err := runCommand("python --version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(strings.ToLower(denyOut), "allowlist") {
		t.Fatalf("expected allowlist denial, got %q", denyOut)
	}

	allowOut, err := runCommand("pwd")
	if err != nil {
		t.Fatalf("expected allowed command to run, got error: %v", err)
	}
	if strings.TrimSpace(allowOut) == "" {
		t.Fatalf("expected output for allowed command")
	}
}

func TestApprovalNormalizationAndReplay(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	tempWD := t.TempDir()
	if err := os.Chdir(tempWD); err != nil {
		t.Fatalf("failed to chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(origWD)
	}()

	if err := ApproveCommand("GO   test   ./..."); err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if !IsApproved("go test ./...", mustLoadApprovals(t)) {
		t.Fatalf("expected normalized approval match")
	}

	// Replay with formatting differences should not create a duplicate approval.
	if err := ApproveCommand("go test    ./..."); err != nil {
		t.Fatalf("second approve failed: %v", err)
	}
	approved := mustLoadApprovals(t)
	if len(approved) != 1 {
		t.Fatalf("expected single normalized approval, got %d (%v)", len(approved), approved)
	}
	if approved[0] != "go test ./..." {
		t.Fatalf("expected normalized stored approval, got %q", approved[0])
	}
}

func TestApproveCommandRejectsNonApprovalAllowlistExecutable(t *testing.T) {
	if err := ApproveCommand("ls -la"); err == nil {
		t.Fatalf("expected rejection for non-approval allowlist command")
	}
}

func TestRunCommand_ApprovalDenyApproveExecuteCycle(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	tempWD := t.TempDir()
	if err := os.Chdir(tempWD); err != nil {
		t.Fatalf("failed to chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	denied, err := runCommand("go version")
	if err != nil {
		t.Fatalf("unexpected deny error: %v", err)
	}
	if !strings.Contains(strings.ToLower(denied), "permission denied") {
		t.Fatalf("expected denial before approval, got %q", denied)
	}

	if err := ApproveCommand("go version"); err != nil {
		t.Fatalf("approve command failed: %v", err)
	}

	executed, err := runCommand("go version")
	if err != nil {
		t.Fatalf("unexpected execution error after approval: %v", err)
	}
	if !strings.Contains(strings.ToLower(executed), "go version") {
		t.Fatalf("expected go version output after approval, got %q", executed)
	}
}

func mustLoadApprovals(t *testing.T) []string {
	t.Helper()
	approved, err := LoadApprovals()
	if err != nil {
		t.Fatalf("load approvals failed: %v", err)
	}
	return approved
}

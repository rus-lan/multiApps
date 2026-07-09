package clone

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
}

// makeBareRepo creates a source repo with files, commits them, and returns
// a file:// URL to a bare clone of it.
func makeBareRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	src := t.TempDir()
	if _, err := RunGit(src, "init", "-b", "main"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if _, err := RunGit(src, "-c", "user.name=test", "-c", "user.email=test@test", "add", "-A"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := RunGit(src, "-c", "user.name=test", "-c", "user.email=test@test", "commit", "-m", "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	bareParent := t.TempDir()
	bare := filepath.Join(bareParent, "bare.git")
	if _, err := RunGit("", "clone", "--bare", src, bare); err != nil {
		t.Fatalf("git clone --bare: %v", err)
	}
	return "file://" + bare
}

func TestRunGit_Success(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	out, err := RunGit(dir, "init", "-b", "main")
	if err != nil {
		t.Fatalf("RunGit: %v", err)
	}
	if out == "" {
		t.Error("want non-empty output from git init")
	}
}

func TestRunGit_Failure(t *testing.T) {
	requireGit(t)
	_, err := RunGit(t.TempDir(), "not-a-real-subcommand")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestClone_NoBranch(t *testing.T) {
	requireGit(t)
	url := makeBareRepo(t, map[string]string{"file.txt": "hello"})

	dest := filepath.Join(t.TempDir(), "clone")
	if err := Clone(url, dest, ""); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "file.txt")); err != nil {
		t.Errorf("cloned file missing: %v", err)
	}

	out, err := RunGit(dest, "rev-parse", "--is-shallow-repository")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	if strings.TrimSpace(out) != "false" {
		t.Errorf("clone is shallow, want full clone")
	}
}

func TestClone_Branch(t *testing.T) {
	requireGit(t)

	src := t.TempDir()
	if _, err := RunGit(src, "init", "-b", "main"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("main"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := RunGit(src, "-c", "user.name=test", "-c", "user.email=test@test", "add", "-A"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := RunGit(src, "-c", "user.name=test", "-c", "user.email=test@test", "commit", "-m", "init"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := RunGit(src, "checkout", "-b", "feature"); err != nil {
		t.Fatalf("checkout -b: %v", err)
	}

	bare := filepath.Join(t.TempDir(), "bare.git")
	if _, err := RunGit("", "clone", "--bare", src, bare); err != nil {
		t.Fatalf("clone --bare: %v", err)
	}
	url := "file://" + bare

	dest := filepath.Join(t.TempDir(), "clone")
	if err := Clone(url, dest, "feature"); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	out, err := RunGit(dest, "branch", "--show-current")
	if err != nil {
		t.Fatalf("branch --show-current: %v", err)
	}
	if strings.TrimSpace(out) != "feature" {
		t.Errorf("checked out branch %q, want feature", strings.TrimSpace(out))
	}
}

func TestClone_BadURL(t *testing.T) {
	requireGit(t)
	err := Clone("file:///does/not/exist.git", filepath.Join(t.TempDir(), "clone"), "")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestEnsureExclude(t *testing.T) {
	requireGit(t)
	url := makeBareRepo(t, map[string]string{"file.txt": "hello"})

	dest := filepath.Join(t.TempDir(), "clone")
	if err := Clone(url, dest, ""); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if err := EnsureExclude(dest); err != nil {
		t.Fatalf("EnsureExclude: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	for _, want := range []string{"CLAUDE.md", "AGENTS.md"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("exclude file missing %q, got:\n%s", want, data)
		}
	}

	// Running twice must not duplicate lines.
	if err := EnsureExclude(dest); err != nil {
		t.Fatalf("EnsureExclude (2nd run): %v", err)
	}
	data2, err := os.ReadFile(filepath.Join(dest, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if strings.Count(string(data2), "CLAUDE.md") != 1 {
		t.Errorf("CLAUDE.md duplicated, got:\n%s", data2)
	}
	if string(data) != string(data2) {
		t.Errorf("exclude changed on 2nd run:\nfirst:\n%s\nsecond:\n%s", data, data2)
	}
}

func TestEnsureExclude_PreservesExistingContent(t *testing.T) {
	requireGit(t)
	url := makeBareRepo(t, map[string]string{"file.txt": "hello"})

	dest := filepath.Join(t.TempDir(), "clone")
	if err := Clone(url, dest, ""); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	excludePath := filepath.Join(dest, ".git", "info", "exclude")
	if err := os.WriteFile(excludePath, []byte("*.log\n"), 0o644); err != nil {
		t.Fatalf("write exclude: %v", err)
	}

	if err := EnsureExclude(dest); err != nil {
		t.Fatalf("EnsureExclude: %v", err)
	}
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(data), "*.log") {
		t.Errorf("existing content lost, got:\n%s", data)
	}
	if !strings.Contains(string(data), "CLAUDE.md") || !strings.Contains(string(data), "AGENTS.md") {
		t.Errorf("new lines missing, got:\n%s", data)
	}
}

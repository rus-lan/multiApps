package workspace

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rus-lan/multiApps/internal/clone"
	"github.com/rus-lan/multiApps/internal/config"
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

func mustRunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := clone.RunGit(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	mustRunGit(t, dir, "-c", "user.name=test", "-c", "user.email=test@test", "add", "-A")
	mustRunGit(t, dir, "-c", "user.name=test", "-c", "user.email=test@test", "commit", "-m", msg)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// makeBareRepo creates a source repo with files, commits them, and returns
// a file:// URL to a bare clone named name+".git" — the name controls the
// dir mapps.DirFromURL derives, so tests can force or avoid collisions.
func makeBareRepo(t *testing.T, name string, files map[string]string) string {
	t.Helper()

	src := t.TempDir()
	mustRunGit(t, src, "init", "-b", "main")
	for fname, content := range files {
		writeTestFile(t, filepath.Join(src, fname), content)
	}
	commitAll(t, src, "init")

	bareParent := t.TempDir()
	bare := filepath.Join(bareParent, name+".git")
	mustRunGit(t, "", "clone", "--bare", src, bare)
	return "file://" + bare
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	fnErr := fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	return buf.String(), fnErr
}

func TestInit_TwoURLs(t *testing.T) {
	requireGit(t)

	goURL := makeBareRepo(t, "api", map[string]string{"go.mod": "module api\n"})
	unknownURL := makeBareRepo(t, "thing", map[string]string{"file.txt": "hello"})

	root := t.TempDir()
	if err := Init(root, []string{goURL, unknownURL}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	goDir, err := config.DirFromURL(goURL)
	if err != nil {
		t.Fatalf("DirFromURL: %v", err)
	}
	unknownDir, err := config.DirFromURL(unknownURL)
	if err != nil {
		t.Fatalf("DirFromURL: %v", err)
	}

	for _, dir := range []string{goDir, unknownDir} {
		excludePath := filepath.Join(root, "apps", dir, ".git", "info", "exclude")
		data, err := os.ReadFile(excludePath)
		if err != nil {
			t.Fatalf("read exclude for %s: %v", dir, err)
		}
		if !strings.Contains(string(data), "CLAUDE.md") || !strings.Contains(string(data), "AGENTS.md") {
			t.Errorf("exclude for %s missing entries, got:\n%s", dir, data)
		}
	}

	mkData, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	mk := string(mkData)
	if !strings.Contains(mk, "# ---- "+goDir+" (go) ----") {
		t.Errorf("Makefile missing go stack section for %s, got:\n%s", goDir, mk)
	}
	if !strings.Contains(mk, "# ---- "+unknownDir+" (unknown) ----") {
		t.Errorf("Makefile missing unknown stack section for %s, got:\n%s", unknownDir, mk)
	}

	info, err := os.Stat(filepath.Join(root, "mk"))
	if err != nil || !info.IsDir() {
		t.Fatalf("mk/ directory missing: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "mk"))
	if err != nil {
		t.Fatalf("read mk/: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("mk/ not empty: %v", entries)
	}

	for _, p := range []string{
		filepath.Join(root, ".claude", "skills", "mapps", "SKILL.md"),
		filepath.Join(root, ".opencode", "commands", "mapps.md"),
		filepath.Join(root, "PROMPT-map.md"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("wrapper file missing: %s", p)
		}
	}

	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), "apps/") {
		t.Errorf(".gitignore missing apps/, got:\n%s", gitignore)
	}

	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Errorf("root .git missing: %v", err)
	}
}

func TestInit_Idempotent(t *testing.T) {
	requireGit(t)

	url1 := makeBareRepo(t, "api", map[string]string{"go.mod": "module api\n"})
	url2 := makeBareRepo(t, "thing", map[string]string{"file.txt": "hello"})

	root := t.TempDir()
	if err := Init(root, []string{url1, url2}); err != nil {
		t.Fatalf("Init (1st): %v", err)
	}

	listPath := filepath.Join(root, "repos.list")
	before, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatalf("read repos.list: %v", err)
	}

	dir1, err := config.DirFromURL(url1)
	if err != nil {
		t.Fatalf("DirFromURL: %v", err)
	}
	excludePath := filepath.Join(root, "apps", dir1, ".git", "info", "exclude")
	excludeBefore, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}

	output, err := captureStdout(t, func() error {
		return Init(root, []string{url1, url2})
	})
	if err != nil {
		t.Fatalf("Init (2nd): %v", err)
	}
	if !strings.Contains(output, "cloned 0, skipped 2, failed 0") {
		t.Errorf("2nd Init output = %q, want cloned 0, skipped 2, failed 0", output)
	}

	after, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatalf("read repos.list (2nd): %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("repos.list changed on 2nd Init:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	excludeAfter, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude (2nd): %v", err)
	}
	if string(excludeBefore) != string(excludeAfter) {
		t.Errorf("exclude changed on 2nd Init:\nbefore:\n%s\nafter:\n%s", excludeBefore, excludeAfter)
	}
}

func TestAdd_ExplicitDirAndBranch(t *testing.T) {
	requireGit(t)

	src := t.TempDir()
	mustRunGit(t, src, "init", "-b", "main")
	writeTestFile(t, filepath.Join(src, "file.txt"), "main")
	commitAll(t, src, "init")
	mustRunGit(t, src, "checkout", "-b", "feature")
	writeTestFile(t, filepath.Join(src, "feature.txt"), "feature")
	commitAll(t, src, "feature commit")

	bare := filepath.Join(t.TempDir(), "third.git")
	mustRunGit(t, "", "clone", "--bare", src, bare)
	url := "file://" + bare

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "mk"), 0o755); err != nil {
		t.Fatalf("MkdirAll mk: %v", err)
	}
	customPath := filepath.Join(root, "mk", "custom.mk")
	if err := os.WriteFile(customPath, []byte("deploy-custom:\n\techo deploy\n"), 0o644); err != nil {
		t.Fatalf("write custom.mk: %v", err)
	}
	customBefore, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("read custom.mk: %v", err)
	}

	if err := Add(root, url, "third-app", "feature"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	listData, err := os.ReadFile(filepath.Join(root, "repos.list"))
	if err != nil {
		t.Fatalf("read repos.list: %v", err)
	}
	if !strings.Contains(string(listData), url+" third-app feature") {
		t.Errorf("repos.list missing appended line, got:\n%s", listData)
	}

	branchOut, err := clone.RunGit(filepath.Join(root, "apps", "third-app"), "branch", "--show-current")
	if err != nil {
		t.Fatalf("branch --show-current: %v", err)
	}
	if strings.TrimSpace(branchOut) != "feature" {
		t.Errorf("checked out branch %q, want feature", strings.TrimSpace(branchOut))
	}

	mkData, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	if !strings.Contains(string(mkData), "build-third-app:") {
		t.Errorf("Makefile missing target for third-app, got:\n%s", mkData)
	}

	customAfter, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("read custom.mk (after): %v", err)
	}
	if string(customBefore) != string(customAfter) {
		t.Errorf("mk/custom.mk was modified")
	}
}

func TestInit_CollisionFailsBeforeCloning(t *testing.T) {
	requireGit(t)

	urlA := makeBareRepo(t, "same", map[string]string{"a.txt": "a"})
	urlB := makeBareRepo(t, "same", map[string]string{"b.txt": "b"})

	root := t.TempDir()
	err := Init(root, []string{urlA, urlB})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "dir collision") {
		t.Errorf("error %q does not mention dir collision", err.Error())
	}

	entries, statErr := os.ReadDir(filepath.Join(root, "apps"))
	if statErr == nil && len(entries) != 0 {
		t.Errorf("apps/ should have no new directories, got: %v", entries)
	}
}

func TestInit_PartialFailure(t *testing.T) {
	requireGit(t)

	goodURL := makeBareRepo(t, "good", map[string]string{"go.mod": "module good\n"})
	badURL := "file:///nonexistent/nope.git"

	root := t.TempDir()
	err := Init(root, []string{goodURL, badURL})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), badURL) {
		t.Errorf("error %q does not mention the bad url %q", err.Error(), badURL)
	}

	goodDir, err := config.DirFromURL(goodURL)
	if err != nil {
		t.Fatalf("DirFromURL: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "apps", goodDir)); statErr != nil {
		t.Errorf("good repo not cloned: %v", statErr)
	}

	mkData, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	if !strings.Contains(string(mkData), "build-"+goodDir+":") {
		t.Errorf("Makefile missing target for good repo, got:\n%s", mkData)
	}

	listData, err := os.ReadFile(filepath.Join(root, "repos.list"))
	if err != nil {
		t.Fatalf("read repos.list: %v", err)
	}
	if !strings.Contains(string(listData), goodURL) || !strings.Contains(string(listData), badURL) {
		t.Errorf("repos.list missing an entry, got:\n%s", listData)
	}
}

func TestInit_ParseErrorDoesNothing(t *testing.T) {
	root := t.TempDir()
	listPath := filepath.Join(root, "repos.list")
	if err := os.WriteFile(listPath, []byte("garbage one two three four\n"), 0o644); err != nil {
		t.Fatalf("write repos.list: %v", err)
	}

	err := Init(root, nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), ":1:") {
		t.Errorf("error %q does not contain line number", err.Error())
	}

	if _, statErr := os.Stat(filepath.Join(root, "apps")); !os.IsNotExist(statErr) {
		t.Errorf("apps/ should not have been created")
	}
}

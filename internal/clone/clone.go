// Package clone shells out to the system git binary to clone repos and
// keep their .git/info/exclude in sync.
package clone

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunGit runs git with args in dir (empty dir means the current
// directory) and returns its combined output. A failure's error wraps the
// last non-empty output line so callers can report git's own message.
func RunGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, lastLine(out))
	}
	return string(out), nil
}

// Clone runs a full (never shallow) git clone of url into dest, checking
// out branch when set.
func Clone(url, dest, branch string) error {
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, "--", url, dest)

	cmd := exec.Command("git", args...)
	var tail bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &tail)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clone %s: %w: %s", url, err, lastLine(tail.Bytes()))
	}
	return nil
}

// EnsureExclude adds CLAUDE.md and AGENTS.md to repoDir's
// .git/info/exclude, creating it if needed. Running it twice changes
// nothing: existing lines are never duplicated or touched.
func EnsureExclude(repoDir string) error {
	out, err := RunGit(repoDir, "rev-parse", "--git-dir")
	if err != nil {
		return err
	}
	gitDir := strings.TrimSpace(out)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoDir, gitDir)
	}

	infoDir := filepath.Join(gitDir, "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return err
	}

	excludePath := filepath.Join(infoDir, "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	existing := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		existing[strings.TrimRight(line, "\r")] = true
	}

	var toAdd []string
	for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
		if !existing[name] {
			toAdd = append(toAdd, name)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	for _, name := range toAdd {
		if _, err := f.WriteString(name + "\n"); err != nil {
			return err
		}
	}
	return nil
}

// IsEmptyRemote reports whether the repo at url has no refs at all. A
// failed ls-remote (unreachable url, auth) is returned as an error — that
// is a normal clone failure, not emptiness.
func IsEmptyRemote(url string) (bool, error) {
	out, err := RunGit("", "ls-remote", "--", url)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// HasCommits reports whether the clone at repoDir has at least one commit.
func HasCommits(repoDir string) bool {
	_, err := RunGit(repoDir, "rev-parse", "HEAD")
	return err == nil
}

// HasUncommitted reports whether the clone at repoDir has uncommitted
// changes, including untracked files.
func HasUncommitted(repoDir string) (bool, error) {
	out, err := RunGit(repoDir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// HasUnpushed reports whether the clone at repoDir has commits that are
// not on its upstream. A branch with commits but no upstream counts as
// unpushed — nothing has been pushed anywhere.
func HasUnpushed(repoDir string) (bool, error) {
	if !HasCommits(repoDir) {
		return false, nil
	}
	if _, err := RunGit(repoDir, "rev-parse", "--abbrev-ref", "@{u}"); err != nil {
		return true, nil
	}
	out, err := RunGit(repoDir, "rev-list", "@{u}..HEAD")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return strings.TrimSpace(lines[i])
		}
	}
	return ""
}

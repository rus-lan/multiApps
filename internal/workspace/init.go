// Package workspace orchestrates mapps init and mapps add: it ties
// together config, clone, makefile, and wrappers into the two CLI
// commands.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rus-lan/multiApps/internal/clone"
	"github.com/rus-lan/multiApps/internal/config"
	"github.com/rus-lan/multiApps/internal/makefile"
	"github.com/rus-lan/multiApps/internal/wrappers"
)

type cloneFailure struct {
	url    string
	reason string
}

// Init sets up or updates the workspace at root: creates repos.list if
// needed, appends urls, clones every repo not yet cloned, and
// (re)generates the Makefile and wrapper files. Every step is safe to
// re-run.
func Init(root string, urls []string) error {
	listPath := filepath.Join(root, "repos.list")
	if err := ensureRepoList(listPath); err != nil {
		return err
	}

	repos, err := config.Load(listPath)
	if err != nil {
		return err
	}

	for _, url := range urls {
		if !validURL(url) {
			return fmt.Errorf("argument %q: not a git url", url)
		}
		dir, err := config.DirFromURL(url)
		if err != nil {
			return fmt.Errorf("argument %q: %w", url, err)
		}
		if containsURL(repos, url) {
			continue
		}
		if err := config.AppendRepo(listPath, config.Repo{URL: url}); err != nil {
			return fmt.Errorf("append %q to repos.list: %w", url, err)
		}
		repos = append(repos, config.Repo{URL: url, Dir: dir})
	}

	if err := config.CheckCollisions(repos); err != nil {
		return err
	}

	appsRoot := filepath.Join(root, "apps")
	if err := os.MkdirAll(appsRoot, 0o755); err != nil {
		return err
	}

	var cloned, skipped int
	var failures []cloneFailure

	for _, r := range repos {
		dir := filepath.Join(appsRoot, r.Dir)
		switch {
		case exists(filepath.Join(dir, ".git")):
			fmt.Printf("skip %s (already cloned)\n", r.Dir)
			skipped++
			if err := clone.EnsureExclude(dir); err != nil {
				return fmt.Errorf("ensure exclude for %s: %w", r.Dir, err)
			}
		case exists(dir):
			fmt.Printf("apps/%s exists but is not a git clone — remove it to let mapps clone\n", r.Dir)
			skipped++
		default:
			fmt.Printf("cloning %s -> apps/%s\n", r.URL, r.Dir)
			if err := clone.Clone(r.URL, dir, r.Branch); err != nil {
				failures = append(failures, cloneFailure{url: r.URL, reason: err.Error()})
				continue
			}
			cloned++
			if err := clone.EnsureExclude(dir); err != nil {
				return fmt.Errorf("ensure exclude for %s: %w", r.Dir, err)
			}
		}
	}

	if err := makefile.Write(root, detectTargets(appsRoot, repos)); err != nil {
		return fmt.Errorf("write Makefile: %w", err)
	}

	if err := wrappers.Write(root); err != nil {
		return fmt.Errorf("write wrapper files: %w", err)
	}

	if err := ensureGitignore(root); err != nil {
		return err
	}

	if err := ensureWorkspaceGit(root); err != nil {
		return err
	}

	fmt.Printf("cloned %d, skipped %d, failed %d\n", cloned, skipped, len(failures))
	for _, f := range failures {
		fmt.Printf("  %s: %s\n", f.url, f.reason)
	}
	if len(failures) > 0 {
		var reasons []string
		for _, f := range failures {
			reasons = append(reasons, fmt.Sprintf("%s: %s", f.url, f.reason))
		}
		return fmt.Errorf("%d repo(s) failed to clone: %s", len(failures), strings.Join(reasons, "; "))
	}
	return nil
}

func ensureRepoList(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(config.Header), 0o644); err != nil {
			return fmt.Errorf("create repos.list: %w", err)
		}
	} else if err != nil {
		return err
	}
	return nil
}

func validURL(url string) bool {
	return strings.ContainsAny(url, "/:") && !strings.HasPrefix(url, "-")
}

func validDir(dir string) bool {
	return !strings.Contains(dir, "/") && dir != "." && dir != ".." && !strings.HasPrefix(dir, "-")
}

func validBranch(branch string) bool {
	return !strings.HasPrefix(branch, "-")
}

func containsURL(repos []config.Repo, url string) bool {
	for _, r := range repos {
		if r.URL == url {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// detectTargets runs stack detection only for repos whose apps/<dir>
// already exists — a target for a missing dir would fail anyway.
func detectTargets(appsRoot string, repos []config.Repo) []makefile.Target {
	var targets []makefile.Target
	for _, r := range repos {
		if exists(filepath.Join(appsRoot, r.Dir)) {
			targets = append(targets, makefile.Detect(appsRoot, r.Dir))
		}
	}
	return targets
}

func ensureGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte("apps/\n"), 0o644)
	}
	if err != nil {
		return err
	}

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "apps/" || trimmed == "apps" {
			return nil
		}
	}

	content := string(data)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "apps/\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

// ensureWorkspaceGit runs git init in root when it is not yet a git repo.
// A .git file (worktree) is left alone.
func ensureWorkspaceGit(root string) error {
	if exists(filepath.Join(root, ".git")) {
		return nil
	}
	_, err := clone.RunGit(root, "init")
	return err
}

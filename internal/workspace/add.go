package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rus-lan/multiApps/internal/clone"
	"github.com/rus-lan/multiApps/internal/config"
	"github.com/rus-lan/multiApps/internal/makefile"
)

// Add appends one repo to repos.list, clones it, and regenerates the
// Makefile. It does not write wrapper files, .gitignore, or run git init
// in the workspace root — that is Init's job, and the wrapper prompt
// always runs init first anyway.
func Add(root, url, dir, branch string) error {
	if !validURL(url) {
		return fmt.Errorf("argument %q: not a git url", url)
	}
	if dir != "" && !validDir(dir) {
		return fmt.Errorf("argument %q: bad dir", dir)
	}
	if branch != "" && !validBranch(branch) {
		return fmt.Errorf("argument %q: bad branch", branch)
	}

	listPath := filepath.Join(root, "repos.list")
	if err := ensureRepoList(listPath); err != nil {
		return err
	}

	repos, err := config.Load(listPath)
	if err != nil {
		return err
	}

	for _, r := range repos {
		if r.URL == url {
			return fmt.Errorf("already in repos.list (line %d)", r.Line)
		}
	}

	original := config.Repo{URL: url, Dir: dir, Branch: branch}
	derived := original
	if derived.Dir == "" {
		d, err := config.DirFromURL(url)
		if err != nil {
			return fmt.Errorf("argument %q: %w", url, err)
		}
		derived.Dir = d
	}

	if err := config.CheckCollisions(append(append([]config.Repo{}, repos...), derived)); err != nil {
		return err
	}

	if err := config.AppendRepo(listPath, original); err != nil {
		return fmt.Errorf("append %q to repos.list: %w", url, err)
	}

	appsRoot := filepath.Join(root, "apps")
	if err := os.MkdirAll(appsRoot, 0o755); err != nil {
		return err
	}

	destDir := filepath.Join(appsRoot, derived.Dir)
	var cloneErr error
	switch {
	case exists(filepath.Join(destDir, ".git")):
		fmt.Printf("skip %s (already cloned)\n", derived.Dir)
		if err := clone.EnsureExclude(destDir); err != nil {
			return fmt.Errorf("ensure exclude for %s: %w", derived.Dir, err)
		}
	case exists(destDir):
		fmt.Printf("apps/%s exists but is not a git clone — remove it to let mapps clone\n", derived.Dir)
	default:
		fmt.Printf("cloning %s -> apps/%s\n", derived.URL, derived.Dir)
		if err := clone.Clone(derived.URL, destDir, derived.Branch); err != nil {
			cloneErr = fmt.Errorf("clone %s: %w", derived.URL, err)
		} else if err := clone.EnsureExclude(destDir); err != nil {
			return fmt.Errorf("ensure exclude for %s: %w", derived.Dir, err)
		}
	}

	if err := makefile.Write(root, detectTargets(appsRoot, append(repos, derived))); err != nil {
		return fmt.Errorf("write Makefile: %w", err)
	}

	return cloneErr
}

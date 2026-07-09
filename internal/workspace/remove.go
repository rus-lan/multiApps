package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rus-lan/multiApps/internal/clone"
	"github.com/rus-lan/multiApps/internal/config"
	"github.com/rus-lan/multiApps/internal/makefile"
)

// Remove drops one repo from the workspace: its repos.list line, its
// apps/<name> clone, and its mk/<name>.mk override, then regenerates the
// Makefile. Unless force is set it refuses while the clone holds
// uncommitted changes or unpushed commits, before changing anything.
func Remove(root, name string, force bool) error {
	if !validDir(name) {
		return fmt.Errorf("argument %q: bad dir", name)
	}

	listPath := filepath.Join(root, "repos.list")
	repos, err := config.Load(listPath)
	if err != nil {
		return err
	}

	found := false
	for _, r := range repos {
		if r.Dir == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%q is not in repos.list", name)
	}

	appsRoot := filepath.Join(root, "apps")
	dir := filepath.Join(appsRoot, name)

	if !force && exists(filepath.Join(dir, ".git")) {
		dirty, err := clone.HasUncommitted(dir)
		if err != nil {
			return err
		}
		if dirty {
			return fmt.Errorf("apps/%s has uncommitted changes — commit or stash them, or re-run with --force", name)
		}
		unpushed, err := clone.HasUnpushed(dir)
		if err != nil {
			return err
		}
		if unpushed {
			return fmt.Errorf("apps/%s has unpushed commits — push them, or re-run with --force", name)
		}
	}

	removed, ok, err := config.RemoveRepo(listPath, name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%q is not in repos.list", name)
	}
	fmt.Printf("removed from repos.list: %s\n", strings.TrimSpace(removed))

	if exists(dir) {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		fmt.Printf("removed apps/%s\n", name)
	}

	mkPath := filepath.Join(root, "mk", name+".mk")
	if exists(mkPath) {
		if err := os.Remove(mkPath); err != nil {
			return err
		}
		fmt.Printf("removed mk/%s.mk\n", name)
	}

	var remaining []config.Repo
	for _, r := range repos {
		if r.Dir != name {
			remaining = append(remaining, r)
		}
	}
	if err := makefile.Write(root, detectTargets(appsRoot, remaining)); err != nil {
		return fmt.Errorf("write Makefile: %w", err)
	}
	return nil
}

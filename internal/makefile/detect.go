// Package makefile detects each app's build stack and renders the root
// Makefile from it.
package makefile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/rus-lan/multiApps/internal/config"
)

// Target is one repo's row in the generated Makefile.
type Target struct {
	Dir   string // folder name under apps/
	Var   string // make-safe variable suffix
	Stack string // go | node | rust | make | unknown
	Build string // command run from inside apps/<dir>
	Run   string
	Test  string
}

var varUnsafe = regexp.MustCompile(`[^A-Za-z0-9_]`)

// VarName sanitizes dir into the make variable suffix Detect uses for it:
// every character outside [A-Za-z0-9_] becomes "_".
func VarName(dir string) string {
	return varUnsafe.ReplaceAllString(dir, "_")
}

// CheckVarCollisions fails when two repos' dirs sanitize to the same make
// variable name — e.g. "web-app" and "web.app" both become "web_app". Two
// such repos would make Detect's caller emit duplicate BUILD_/RUN_/TEST_
// variables in the generated Makefile, and GNU make silently keeps only
// the last one. It runs over the full list before the Makefile is
// generated.
func CheckVarCollisions(repos []config.Repo) error {
	seen := make(map[string]string, len(repos))
	for _, r := range repos {
		v := VarName(r.Dir)
		if first, ok := seen[v]; ok && first != r.Dir {
			return fmt.Errorf("dirs %q and %q map to the same make variable %q — set an explicit dir for one of them in repos.list",
				first, r.Dir, v)
		}
		seen[v] = r.Dir
	}
	return nil
}

// Detect looks at apps/<dir> markers and returns its build/run/test
// commands. First matching marker wins: go.mod, package.json, Cargo.toml,
// then a Makefile of its own, else unknown.
func Detect(appsRoot, dir string) Target {
	root := filepath.Join(appsRoot, dir)
	v := VarName(dir)

	if exists(filepath.Join(root, "go.mod")) {
		return Target{
			Dir: dir, Var: v, Stack: "go",
			Build: "go build ./...",
			Run:   "go run .",
			Test:  "go test ./...",
		}
	}

	if exists(filepath.Join(root, "package.json")) {
		return detectNode(root, dir, v)
	}

	if exists(filepath.Join(root, "Cargo.toml")) {
		return Target{
			Dir: dir, Var: v, Stack: "rust",
			Build: "cargo build",
			Run:   "cargo run",
			Test:  "cargo test",
		}
	}

	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		if exists(filepath.Join(root, name)) {
			return Target{
				Dir: dir, Var: v, Stack: "make",
				Build: "$(MAKE) build",
				Run:   "$(MAKE) run",
				Test:  "$(MAKE) test",
			}
		}
	}

	return unknownTarget(dir, v)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type packageJSON struct {
	Scripts map[string]string `json:"scripts"`
}

func detectNode(root, dir, v string) Target {
	pm := "npm"
	switch {
	case exists(filepath.Join(root, "pnpm-lock.yaml")):
		pm = "pnpm"
	case exists(filepath.Join(root, "yarn.lock")):
		pm = "yarn"
	case exists(filepath.Join(root, "package-lock.json")):
		pm = "npm"
	}

	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return unknownTarget(dir, v)
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return unknownTarget(dir, v)
	}

	t := Target{Dir: dir, Var: v, Stack: "node"}

	if _, ok := pkg.Scripts["build"]; ok {
		t.Build = pm + " run build"
	} else {
		t.Build = missingScript("build", dir, v, "BUILD")
	}

	switch {
	case scriptExists(pkg.Scripts, "start"):
		t.Run = pm + " run start"
	case scriptExists(pkg.Scripts, "dev"):
		t.Run = pm + " run dev"
	default:
		t.Run = fmt.Sprintf(`echo "no 'start' or 'dev' script in package.json — set RUN_%s in mk/%s.mk"`, v, dir)
	}

	if scriptExists(pkg.Scripts, "test") {
		t.Test = pm + " run test"
	} else {
		t.Test = missingScript("test", dir, v, "TEST")
	}

	return t
}

func scriptExists(scripts map[string]string, name string) bool {
	_, ok := scripts[name]
	return ok
}

func unknownTarget(dir, v string) Target {
	return Target{
		Dir: dir, Var: v, Stack: "unknown",
		Build: unknownPlaceholder(dir, v, "BUILD"),
		Run:   unknownPlaceholder(dir, v, "RUN"),
		Test:  unknownPlaceholder(dir, v, "TEST"),
	}
}

func unknownPlaceholder(dir, v, verb string) string {
	return fmt.Sprintf(`echo "unknown stack in apps/%s — set %s_%s in mk/%s.mk"`, dir, verb, v, dir)
}

func missingScript(script, dir, v, verb string) string {
	return fmt.Sprintf(`echo "no '%s' script in package.json — set %s_%s in mk/%s.mk"`, script, verb, v, dir)
}

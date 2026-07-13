package makefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rus-lan/multiApps/internal/config"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestDetect_GoMarkerAlone(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "api", "go.mod"), "module api\n")

	got := Detect(root, "api")
	if got.Stack != "go" {
		t.Fatalf("Stack = %q, want go", got.Stack)
	}
	if got.Build != "go build ./..." || got.Run != "go run ." || got.Test != "go test ./..." {
		t.Errorf("got %+v", got)
	}
}

func TestDetect_RustMarkerAlone(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "svc", "Cargo.toml"), "[package]\n")

	got := Detect(root, "svc")
	if got.Stack != "rust" {
		t.Fatalf("Stack = %q, want rust", got.Stack)
	}
	if got.Build != "cargo build" || got.Run != "cargo run" || got.Test != "cargo test" {
		t.Errorf("got %+v", got)
	}
}

func TestDetect_MakeMarkerAlone(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "own", "Makefile"), "build:\n\techo hi\n")

	got := Detect(root, "own")
	if got.Stack != "make" {
		t.Fatalf("Stack = %q, want make", got.Stack)
	}
	if got.Build != "$(MAKE) build" || got.Run != "$(MAKE) run" || got.Test != "$(MAKE) test" {
		t.Errorf("got %+v", got)
	}
}

func TestDetect_GoWinsOverMake(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "both", "go.mod"), "module both\n")
	writeFile(t, filepath.Join(root, "both", "Makefile"), "build:\n\techo hi\n")

	got := Detect(root, "both")
	if got.Stack != "go" {
		t.Fatalf("Stack = %q, want go (go.mod must win)", got.Stack)
	}
}

func TestDetect_Unknown(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	got := Detect(root, "empty")
	if got.Stack != "unknown" {
		t.Fatalf("Stack = %q, want unknown", got.Stack)
	}
	for _, cmd := range []string{got.Build, got.Run, got.Test} {
		if !strings.Contains(cmd, "BUILD_"+got.Var) && !strings.Contains(cmd, "RUN_"+got.Var) && !strings.Contains(cmd, "TEST_"+got.Var) {
			t.Errorf("placeholder %q does not name a variable for %q", cmd, got.Var)
		}
		if !strings.Contains(cmd, "mk/empty.mk") {
			t.Errorf("placeholder %q does not name mk/empty.mk", cmd)
		}
	}
}

func TestDetect_NodePackageManagers(t *testing.T) {
	cases := []struct {
		lockfile string
		wantPM   string
	}{
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"package-lock.json", "npm"},
		{"", "npm"},
	}
	for _, c := range cases {
		t.Run(c.wantPM+"_"+c.lockfile, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "web", "package.json"), `{"scripts":{"build":"tsc","start":"node .","test":"jest"}}`)
			if c.lockfile != "" {
				writeFile(t, filepath.Join(root, "web", c.lockfile), "")
			}

			got := Detect(root, "web")
			if got.Stack != "node" {
				t.Fatalf("Stack = %q, want node", got.Stack)
			}
			want := c.wantPM + " run build"
			if got.Build != want {
				t.Errorf("Build = %q, want %q", got.Build, want)
			}
		})
	}
}

func TestDetect_NodeMissingScripts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "web", "package.json"), `{"scripts":{}}`)

	got := Detect(root, "web")
	if got.Stack != "node" {
		t.Fatalf("Stack = %q, want node", got.Stack)
	}
	if !strings.Contains(got.Build, "no 'build' script") {
		t.Errorf("Build placeholder = %q", got.Build)
	}
	if !strings.Contains(got.Run, "no 'start' or 'dev' script") {
		t.Errorf("Run placeholder = %q", got.Run)
	}
	if !strings.Contains(got.Test, "no 'test' script") {
		t.Errorf("Test placeholder = %q", got.Test)
	}
}

func TestDetect_NodeStartPreferredOverDev(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "web", "package.json"), `{"scripts":{"start":"node .","dev":"node --watch ."}}`)

	got := Detect(root, "web")
	if got.Run != "npm run start" {
		t.Errorf("Run = %q, want npm run start (start preferred over dev)", got.Run)
	}
}

func TestDetect_NodeDevFallback(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "web", "package.json"), `{"scripts":{"dev":"node --watch ."}}`)

	got := Detect(root, "web")
	if got.Run != "npm run dev" {
		t.Errorf("Run = %q, want npm run dev", got.Run)
	}
}

func TestDetect_NodeBrokenJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "web", "package.json"), `{not valid json`)

	got := Detect(root, "web")
	if got.Stack != "unknown" {
		t.Fatalf("Stack = %q, want unknown for broken package.json", got.Stack)
	}
}

func TestCheckVarCollisions(t *testing.T) {
	colliding := []config.Repo{
		{URL: "git@github.com:a/web-app.git", Dir: "web-app"},
		{URL: "git@github.com:b/web.app.git", Dir: "web.app"},
	}
	err := CheckVarCollisions(colliding)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	for _, want := range []string{"web-app", "web.app", "web_app"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}

	safe := []config.Repo{
		{URL: "git@github.com:a/api.git", Dir: "api"},
		{URL: "git@github.com:b/web.git", Dir: "web"},
	}
	if err := CheckVarCollisions(safe); err != nil {
		t.Errorf("non-colliding dirs should not error, got: %v", err)
	}
}

func TestRender_TwoRepos(t *testing.T) {
	targets := []Target{
		{Dir: "api", Var: "api", Stack: "go", Build: "go build ./...", Run: "go run .", Test: "go test ./..."},
		{Dir: "my-web", Var: "my_web", Stack: "node", Build: "npm run build", Run: "npm run start", Test: "npm run test"},
	}

	out, err := Render(targets)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, "build-my-web:") {
		t.Error("missing build-my-web: target")
	}
	if !strings.Contains(got, "BUILD_my_web = npm run build") {
		t.Error("missing BUILD_my_web variable")
	}
	if !strings.Contains(got, "\n\tcd apps/") {
		t.Error("recipe lines are not tab-indented")
	}
	if !strings.Contains(got, "MSG is empty") {
		t.Error("missing MSG guard in commit target")
	}
	if !strings.HasSuffix(strings.TrimRight(got, "\n"), "-include mk/*.mk") {
		t.Errorf("Makefile does not end with -include mk/*.mk, got tail:\n%s", got[len(got)-60:])
	}
	for _, forbidden := range []string{"run-all", "commit-all", "push-all"} {
		if strings.Contains(got, forbidden+":") {
			t.Errorf("Makefile must not contain %s target", forbidden)
		}
	}
}

func TestRender_ZeroRepos(t *testing.T) {
	out, err := Render(nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := string(out)
	for _, want := range []string{"build-all:", "status:", "list:", ".PHONY"} {
		if !strings.Contains(got, want) {
			t.Errorf("zero-repo Makefile missing %q, got:\n%s", want, got)
		}
	}
}

func TestWrite(t *testing.T) {
	root := t.TempDir()
	targets := []Target{
		{Dir: "api", Var: "api", Stack: "go", Build: "go build ./...", Run: "go run .", Test: "go test ./..."},
	}
	if err := Write(root, targets); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Makefile")); err != nil {
		t.Errorf("Makefile not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "mk")); err != nil {
		t.Errorf("mk/ not created: %v", err)
	}
}

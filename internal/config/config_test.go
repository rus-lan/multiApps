package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_SkipsBlankAndComments(t *testing.T) {
	in := "\n# a comment\n   \ngit@github.com:user/api.git\n# another\nhttps://github.com/user/web.git\n"
	repos, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("got %d repos, want 2", len(repos))
	}
	if repos[0].Line != 4 {
		t.Errorf("repos[0].Line = %d, want 4", repos[0].Line)
	}
	if repos[1].Line != 6 {
		t.Errorf("repos[1].Line = %d, want 6", repos[1].Line)
	}
}

func TestParse_FieldCounts(t *testing.T) {
	cases := []struct {
		name   string
		line   string
		url    string
		dir    string
		branch string
	}{
		{"one field", "git@github.com:user/api.git", "git@github.com:user/api.git", "api", ""},
		{"two fields", "git@github.com:user/api.git my-api", "git@github.com:user/api.git", "my-api", ""},
		{"three fields", "git@github.com:user/api.git my-api main", "git@github.com:user/api.git", "my-api", "main"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repos, err := Parse(strings.NewReader(c.line))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(repos) != 1 {
				t.Fatalf("got %d repos, want 1", len(repos))
			}
			r := repos[0]
			if r.URL != c.url || r.Dir != c.dir || r.Branch != c.branch {
				t.Errorf("got %+v, want URL=%q Dir=%q Branch=%q", r, c.url, c.dir, c.branch)
			}
		})
	}
}

func TestParse_TooManyFields(t *testing.T) {
	_, err := Parse(strings.NewReader("git@github.com:user/api.git my-api main extra"))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), ":1:") {
		t.Errorf("error %q does not contain line number", err.Error())
	}
}

func TestParse_BadURL(t *testing.T) {
	cases := []string{"banana", "-flag-looking-url"}
	for _, url := range cases {
		t.Run(url, func(t *testing.T) {
			_, err := Parse(strings.NewReader(url))
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), ":1:") {
				t.Errorf("error %q does not contain line number", err.Error())
			}
		})
	}
}

func TestParse_BadDir(t *testing.T) {
	cases := []string{
		"git@github.com:user/api.git a/b",
		"git@github.com:user/api.git .",
		"git@github.com:user/api.git ..",
		"git@github.com:user/api.git -weird",
	}
	for _, line := range cases {
		t.Run(line, func(t *testing.T) {
			_, err := Parse(strings.NewReader(line))
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), "bad dir") {
				t.Errorf("error %q does not mention bad dir", err.Error())
			}
		})
	}
}

func TestParse_BadBranch(t *testing.T) {
	_, err := Parse(strings.NewReader("git@github.com:user/api.git my-api -weird"))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), ":1:") {
		t.Errorf("error %q does not contain line number", err.Error())
	}
}

func TestDirFromURL(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"git@github.com:user/api.git", "api", false},
		{"https://github.com/user/web.git", "web", false},
		{"file:///tmp/x/bare.git", "bare", false},
		{"https://host/user/thing/", "thing", false},
		{"git@github.com:user/nogit", "nogit", false},
		{"git@host:.git", "", true},
	}
	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			got, err := DirFromURL(c.url)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got dir %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DirFromURL: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestCheckCollisions(t *testing.T) {
	colliding := []Repo{
		{URL: "git@github.com:a/api.git", Dir: "api", Line: 3},
		{URL: "git@github.com:b/api.git", Dir: "api", Line: 7},
	}
	err := CheckCollisions(colliding)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	for _, want := range []string{"line 3", "line 7", "api"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}

	rescued := []Repo{
		{URL: "git@github.com:a/api.git", Dir: "api", Line: 3},
		{URL: "git@github.com:b/api.git", Dir: "api-2", Line: 7},
	}
	if err := CheckCollisions(rescued); err != nil {
		t.Errorf("explicit dir should rescue collision, got: %v", err)
	}
}

func TestCheckCollisions_CLIArgEntry(t *testing.T) {
	colliding := []Repo{
		{URL: "git@github.com:a/api.git", Dir: "api", Line: 3},
		{URL: "git@github.com:b/api.git", Dir: "api", Line: 0},
	}
	err := CheckCollisions(colliding)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if strings.Contains(err.Error(), "line 0") {
		t.Errorf("error %q should not say %q for a CLI-arg entry", err.Error(), "line 0")
	}
	if !strings.Contains(err.Error(), "argument") {
		t.Errorf("error %q should label the Line==0 entry as an argument", err.Error())
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error %q should still say %q for the repos.list entry", err.Error(), "line 3")
	}
}

func TestAppendRepo_CreatesFileWithHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.list")
	if err := AppendRepo(path, Repo{URL: "git@github.com:user/api.git"}); err != nil {
		t.Fatalf("AppendRepo: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.HasPrefix(string(data), Header) {
		t.Errorf("file does not start with header, got:\n%s", data)
	}
	if !strings.HasSuffix(string(data), "git@github.com:user/api.git\n") {
		t.Errorf("file does not end with appended line, got:\n%s", data)
	}
}

func TestAppendRepo_AddsMissingTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.list")
	if err := os.WriteFile(path, []byte("git@github.com:user/api.git"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := AppendRepo(path, Repo{URL: "git@github.com:user/web.git"}); err != nil {
		t.Fatalf("AppendRepo: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "git@github.com:user/api.git\ngit@github.com:user/web.git\n"
	if string(data) != want {
		t.Errorf("got:\n%s\nwant:\n%s", data, want)
	}
}

func TestAppendRepo_KeepsExistingTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.list")
	if err := os.WriteFile(path, []byte("git@github.com:user/api.git\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := AppendRepo(path, Repo{URL: "git@github.com:user/web.git", Dir: "my-web", Branch: "main"}); err != nil {
		t.Fatalf("AppendRepo: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "git@github.com:user/api.git\ngit@github.com:user/web.git my-web main\n"
	if string(data) != want {
		t.Errorf("got:\n%s\nwant:\n%s", data, want)
	}
}

func removeRepoFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repos.list")
	content := Header + "# keep me\n\ngit@github.com:user/api.git\nhttps://github.com/user/web.git my-web main\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestRemoveRepo_DropsDerivedDirLine(t *testing.T) {
	path := removeRepoFixture(t)

	removed, ok, err := RemoveRepo(path, "api")
	if err != nil {
		t.Fatalf("RemoveRepo: %v", err)
	}
	if !ok {
		t.Fatal("want found true")
	}
	if removed != "git@github.com:user/api.git" {
		t.Errorf("removed = %q, want %q", removed, "git@github.com:user/api.git")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := Header + "# keep me\n\nhttps://github.com/user/web.git my-web main\n"
	if string(data) != want {
		t.Errorf("got:\n%s\nwant:\n%s", data, want)
	}
}

func TestRemoveRepo_DropsExplicitDirLineByDir(t *testing.T) {
	path := removeRepoFixture(t)

	removed, ok, err := RemoveRepo(path, "my-web")
	if err != nil {
		t.Fatalf("RemoveRepo: %v", err)
	}
	if !ok {
		t.Fatal("want found true")
	}
	if removed != "https://github.com/user/web.git my-web main" {
		t.Errorf("removed = %q, want %q", removed, "https://github.com/user/web.git my-web main")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "git@github.com:user/api.git") {
		t.Errorf("api line should stay, got:\n%s", data)
	}
	if strings.Contains(string(data), "my-web") {
		t.Errorf("my-web line should be gone, got:\n%s", data)
	}
}

func TestRemoveRepo_URLBasenameDoesNotMatch(t *testing.T) {
	path := removeRepoFixture(t)

	_, ok, err := RemoveRepo(path, "web")
	if err != nil {
		t.Fatalf("RemoveRepo: %v", err)
	}
	if ok {
		t.Error("want found false for URL basename, only the effective dir should match")
	}
}

func TestRemoveRepo_NotFound(t *testing.T) {
	path := removeRepoFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	removed, ok, err := RemoveRepo(path, "nope")
	if err != nil {
		t.Fatalf("RemoveRepo: %v", err)
	}
	if ok {
		t.Error("want found false")
	}
	if removed != "" {
		t.Errorf("removed = %q, want empty", removed)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("file changed on no match:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestRemoveRepo_ParseErrorPropagates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.list")
	content := "garbage one two three four\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err := RemoveRepo(path, "anything")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), ":1:") {
		t.Errorf("error %q does not contain line number", err.Error())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != content {
		t.Errorf("file changed on parse error, got:\n%s", data)
	}
}

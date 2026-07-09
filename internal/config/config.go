// Package config reads and writes repos.list, the workspace's list of
// repositories to clone.
package config

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// Header is written at the top of a freshly created repos.list.
const Header = "# mapps repos list\n# format: <url> [dir] [branch]\n"

// Repo is one entry parsed from (or destined for) repos.list.
type Repo struct {
	URL    string
	Dir    string // always set: explicit field or derived from URL
	Branch string // "" means remote default branch
	Line   int    // 1-based line in repos.list; 0 for CLI-arg entries
}

// Load reads and parses the repos.list file at path.
func Load(path string) ([]Repo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads repos.list lines from r, one repo per non-comment, non-blank
// line. Errors carry the 1-based line number.
func Parse(r io.Reader) ([]Repo, error) {
	var repos []Repo
	scanner := bufio.NewScanner(r)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}

		fields := strings.Fields(text)
		if len(fields) > 3 {
			return nil, fmt.Errorf("repos.list:%d: too many fields (want: url [dir] [branch])", line)
		}

		url := fields[0]
		if (!strings.ContainsAny(url, "/:")) || strings.HasPrefix(url, "-") {
			return nil, fmt.Errorf("repos.list:%d: not a git url: %q", line, url)
		}

		repo := Repo{URL: url, Line: line}

		if len(fields) >= 2 {
			dir := fields[1]
			if strings.Contains(dir, "/") || dir == "." || dir == ".." || strings.HasPrefix(dir, "-") {
				return nil, fmt.Errorf("repos.list:%d: bad dir: %q", line, dir)
			}
			repo.Dir = dir
		}

		if len(fields) == 3 {
			branch := fields[2]
			if strings.HasPrefix(branch, "-") {
				return nil, fmt.Errorf("repos.list:%d: bad branch: %q", line, branch)
			}
			repo.Branch = branch
		}

		if repo.Dir == "" {
			dir, err := DirFromURL(url)
			if err != nil {
				return nil, fmt.Errorf("repos.list:%d: %w", line, err)
			}
			repo.Dir = dir
		}

		repos = append(repos, repo)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return repos, nil
}

// DirFromURL derives a folder name from a git URL: the last path segment
// (ssh scp-style URLs split on ':' when there is no '/'), with a trailing
// ".git" trimmed.
func DirFromURL(url string) (string, error) {
	trimmed := strings.TrimRight(url, "/")

	var base string
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		base = trimmed[i+1:]
	} else if i := strings.LastIndex(trimmed, ":"); i >= 0 {
		base = trimmed[i+1:]
	} else {
		base = trimmed
	}
	base = strings.TrimSuffix(base, ".git")

	if base == "" || base == "." || base == ".." || strings.Contains(base, "/") {
		return "", fmt.Errorf("cannot derive dir from url %q", url)
	}
	return base, nil
}

// CheckCollisions fails when two repos would land in the same apps/<dir>.
// It runs over the full list before any cloning happens.
func CheckCollisions(repos []Repo) error {
	seen := make(map[string]Repo, len(repos))
	for _, r := range repos {
		if first, ok := seen[r.Dir]; ok {
			return fmt.Errorf("dir collision: %q wanted by %s (%s) and %s (%s) — set an explicit dir for one of them in repos.list",
				r.Dir, location(first), first.URL, location(r), r.URL)
		}
		seen[r.Dir] = r
	}
	return nil
}

// location describes where a Repo came from for error messages: its
// repos.list line, or "argument" for a CLI-arg entry that has no line.
func location(r Repo) string {
	if r.Line == 0 {
		return "argument"
	}
	return fmt.Sprintf("line %d", r.Line)
}

// AppendRepo appends one line to repos.list: url, then dir if r.Dir was set
// explicitly, then branch if set. It creates the file with the Header when
// it does not exist, and adds a trailing newline before appending when the
// existing content is missing one.
func AppendRepo(path string, r Repo) error {
	line := r.URL
	if r.Dir != "" {
		line += " " + r.Dir
	}
	if r.Branch != "" {
		line += " " + r.Branch
	}
	line += "\n"

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("open repos.list: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	if info.Size() == 0 {
		if _, err := f.WriteString(Header); err != nil {
			return err
		}
	} else {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, info.Size()-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			if _, err := f.WriteAt([]byte("\n"), info.Size()); err != nil {
				return err
			}
		}
	}

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	_, err = f.WriteString(line)
	return err
}

// RemoveRepo rewrites repos.list, dropping the first line whose dir
// (explicit or derived) equals name. Every other line — comments, blank
// lines, the header — is kept byte for byte. It returns the removed raw
// line and whether a match was found; on no match the file is untouched.
func RemoveRepo(path, name string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	repos, err := Parse(bytes.NewReader(data))
	if err != nil {
		return "", false, err
	}

	target := 0
	for _, r := range repos {
		if r.Dir == name {
			target = r.Line
			break
		}
	}
	if target == 0 {
		return "", false, nil
	}

	lines := strings.Split(string(data), "\n")
	removed := lines[target-1]
	lines = append(lines[:target-1], lines[target:]...)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", false, err
	}
	return removed, true, nil
}

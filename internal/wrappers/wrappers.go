// Package wrappers writes the LLM wrapper prompts (Claude Code skill,
// opencode command, generic prompt) into a workspace.
package wrappers

import (
	"os"
	"path/filepath"

	"github.com/rus-lan/multiApps/templates"
)

type file struct {
	template string
	dest     string
}

var files = []file{
	{"claude-skill.md", filepath.Join(".claude", "skills", "mapps", "SKILL.md")},
	{"opencode-command.md", filepath.Join(".opencode", "commands", "mapps.md")},
	{"prompt-map.md", "PROMPT-map.md"},
}

// Write copies the three embedded wrapper templates into root, creating
// their parent directories as needed. It always overwrites: the files are
// tool-owned and regeneration must be safe.
func Write(root string) error {
	for _, f := range files {
		data, err := templates.Files.ReadFile(f.template)
		if err != nil {
			return err
		}
		dest := filepath.Join(root, f.dest)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

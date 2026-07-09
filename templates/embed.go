package templates

import "embed"

//go:embed claude-skill.md opencode-command.md prompt-map.md makefile.tmpl
var Files embed.FS

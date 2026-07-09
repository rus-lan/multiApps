package makefile

import (
	"bytes"
	"os"
	"path/filepath"
	"text/template"

	"github.com/rus-lan/multiApps/templates"
)

type templateData struct {
	Repos []Target
}

// Render builds the root Makefile content from the given targets, in the
// order given (stable output; regeneration diffs stay readable).
func Render(repos []Target) ([]byte, error) {
	tmpl, err := template.ParseFS(templates.Files, "makefile.tmpl")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData{Repos: repos}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Write renders and overwrites <root>/Makefile, creating <root>/mk if it
// does not exist yet. It never writes any file inside mk/.
func Write(root string, repos []Target) error {
	data, err := Render(repos)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "mk"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "Makefile"), data, 0o644)
}

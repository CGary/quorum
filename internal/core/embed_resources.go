package core

import (
	"io/fs"
	"os"
	"path/filepath"
)

// embeddedAgents holds the .agents bundle compiled into the binary. Package main
// registers it via SetEmbeddedAgents. It is the hermetic fallback so the CLI
// works in projects that were never `quorum init`-ed and on machines where the
// source tree is absent (e.g. an installed binary, a -trimpath build, or a moved
// repo). The FS is rooted at the .agents directory: it contains skills/,
// schemas/, templates/, policies/, prompts/, config.yaml, ...
var embeddedAgents fs.FS

// SetEmbeddedAgents registers the embedded .agents bundle (rooted at .agents).
func SetEmbeddedAgents(f fs.FS) { embeddedAgents = f }

// EmbeddedAgentFile reads a single file (slash-relative to the .agents root)
// from the embedded bundle, e.g. "templates/report.yaml".
func EmbeddedAgentFile(rel string) ([]byte, bool) {
	if embeddedAgents == nil {
		return nil, false
	}
	b, err := fs.ReadFile(embeddedAgents, rel)
	if err != nil {
		return nil, false
	}
	return b, true
}

// EmbeddedAgentsDir materializes the embedded .agents bundle into a fresh temp
// directory and returns its path, so path-based scaffolding (CopyDir) can use it
// transparently. Returns ("", false) if nothing is embedded or extraction fails.
func EmbeddedAgentsDir() (string, bool) {
	if embeddedAgents == nil {
		return "", false
	}
	dir, err := os.MkdirTemp("", "quorum-agents-")
	if err != nil {
		return "", false
	}
	err = fs.WalkDir(embeddedAgents, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		target := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		b, readErr := fs.ReadFile(embeddedAgents, p)
		if readErr != nil {
			return readErr
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, b, 0644)
	})
	if err != nil {
		os.RemoveAll(dir)
		return "", false
	}
	return dir, true
}

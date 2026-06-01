package main

import (
	"embed"
	"io/fs"

	"quorum/internal/core"
)

// embeddedAgents bundles the canonical .agents resources (skills, schemas,
// templates, policies, prompts, config) into the binary. This makes the CLI
// hermetic: `quorum report new` and `quorum init` no longer depend on a source
// tree existing at a build-time path. `all:` includes dot/underscore files.
//
//go:embed all:.agents
var embeddedAgents embed.FS

func init() {
	if sub, err := fs.Sub(embeddedAgents, ".agents"); err == nil {
		core.SetEmbeddedAgents(sub)
	}
}

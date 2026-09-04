package core

import (
	"sort"
	"strings"
)

// CatalogDelta is the result of diffing a transport's declared model catalog
// against the live model list reported by the transport's own binary.
type CatalogDelta struct {
	// DeclaredDead holds canonical model keys whose declared model_arg display
	// name was absent from the live catalog. Sorted deterministically.
	DeclaredDead []string `json:"declared_dead"`
	// LiveUndeclared holds display names present in the live catalog but not
	// declared as any model_arg in the agents.yaml models map. Sorted.
	LiveUndeclared []string `json:"live_undeclared"`
}

// ParseAvailableModels scans output for an "Available models:" header line and
// collects subsequent non-blank trimmed lines as display names until a blank
// line or EOF. ok is false when the header is absent; in that case names is nil.
// Leading/trailing whitespace is trimmed per line so the function is robust to
// future agy indentation changes (blueprint risk note 2026-09-03).
func ParseAvailableModels(output string) (names []string, ok bool) {
	lines := strings.Split(output, "\n")
	headerFound := false
	for _, line := range lines {
		if !headerFound {
			if strings.TrimSpace(line) == "Available models:" {
				headerFound = true
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			// blank line signals end of the block
			break
		}
		names = append(names, trimmed)
	}
	if !headerFound {
		return nil, false
	}
	return names, true
}

// DiffCatalog computes the delta between a declared model map and a live list.
// declared maps canonical model key -> model_arg display name (e.g.
// "google/gemini-3.6-flash-low" -> "Gemini 3.6 Flash (Low)").
// live is the ordered list of display names returned by ParseAvailableModels.
// Both DeclaredDead and LiveUndeclared in the returned CatalogDelta are sorted
// deterministically.
func DiffCatalog(declared map[string]string, live []string) CatalogDelta {
	// Build a set of live display names for O(1) lookup.
	liveSet := make(map[string]bool, len(live))
	for _, name := range live {
		liveSet[name] = true
	}

	// Build a set of declared display name values for O(1) lookup.
	declaredValues := make(map[string]bool, len(declared))
	for _, v := range declared {
		declaredValues[v] = true
	}

	// Collect declared keys whose display name is absent from live.
	var dead []string
	for key, displayName := range declared {
		if !liveSet[displayName] {
			dead = append(dead, key)
		}
	}

	// Collect live names absent from the declared value set.
	var undeclared []string
	for _, name := range live {
		if !declaredValues[name] {
			undeclared = append(undeclared, name)
		}
	}

	sort.Strings(dead)
	sort.Strings(undeclared)

	return CatalogDelta{
		DeclaredDead:   dead,
		LiveUndeclared: undeclared,
	}
}

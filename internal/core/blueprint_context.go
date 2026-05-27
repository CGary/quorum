package core

import (
	"path/filepath"
	"sort"
	"strings"
)

func EnrichBlueprintWithRetrievers(blueprint map[string]any, projectRoot string, maxHops int) map[string]any {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		root = projectRoot
	}
	
	enriched := make(map[string]any)
	for k, v := range blueprint {
		enriched[k] = v
	}

	var affected []string
	if aAny, ok := asSlice(enriched["affected_files"]); ok {
		for _, v := range aAny {
			if s, ok := v.(string); ok {
				affected = append(affected, s)
			}
		}
	}
	
	var dependencies []string
	if dAny, ok := asSlice(enriched["dependencies"]); ok {
		for _, v := range dAny {
			if s, ok := v.(string); ok {
				dependencies = append(dependencies, s)
			}
		}
	}

	affected = normalizedUnique(affected, root)
	dependencies = normalizedUnique(dependencies, root)
	seedFiles := absoluteSeedFiles(affected, root)
	
	neighborFiles := astNeighbors(seedFiles, root)
	importFiles := importExpand(seedFiles, root, maxHops)

	enriched["affected_files"] = mergePaths(affected, neighborFiles, root)
	enriched["dependencies"] = mergePaths(dependencies, importFiles, root)
	return enriched
}

func absoluteSeedFiles(paths []string, root string) []string {
	var seeds []string
	for _, rel := range paths {
		candidate := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := filepath.Abs(candidate); err == nil { // In Go filepath.Join with absolute root gives absolute path
			seeds = append(seeds, candidate)
		}
	}
	return seeds
}

func mergePaths(existing []string, discovered []string, root string) []string {
	merged := normalizedUnique(existing, root)
	seen := make(map[string]bool)
	for _, m := range merged {
		seen[m] = true
	}
	disc := normalizedUnique(discovered, root)
	sort.Strings(disc)
	for _, p := range disc {
		if !seen[p] {
			merged = append(merged, p)
			seen[p] = true
		}
	}
	return merged
}

func normalizedUnique(paths []string, root string) []string {
	var normalized []string
	seen := make(map[string]bool)
	for _, raw := range paths {
		rel := projectRelative(raw, root)
		if rel == "" || seen[rel] {
			continue
		}
		normalized = append(normalized, rel)
		seen[rel] = true
	}
	return normalized
}

func projectRelative(raw, root string) string {
	if raw == "" {
		return ""
	}
	path := filepath.FromSlash(raw)
	abs := path
	if !filepath.IsAbs(path) {
		abs = filepath.Join(root, path)
	}
	
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

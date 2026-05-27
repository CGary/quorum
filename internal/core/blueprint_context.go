package core

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// === AST Neighbors ===

var astSkipDirs = []string{"node_modules", "vendor", "dist", "build", ".git", "__pycache__"}

var symbolPatterns = map[string]*regexp.Regexp{
	".ts":  regexp.MustCompile(`export\s+(?:default\s+)?(?:function|class|const|type|interface|enum)\s+(\w+)`),
	".tsx": regexp.MustCompile(`export\s+(?:default\s+)?(?:function|class|const|type|interface|enum)\s+(\w+)`),
	".js":  regexp.MustCompile(`export\s+(?:default\s+)?(?:function|class|const)\s+(\w+)`),
	".py":  regexp.MustCompile(`(?m)^(?:def|class)\s+(\w+)`),
	".go":  regexp.MustCompile(`(?m)^func\s+(?:\(\w+\s+\*?\w+\)\s+)?(\w+)`),
}

func extractSymbols(path string) []string {
	ext := filepath.Ext(path)
	pattern, ok := symbolPatterns[ext]
	if !ok {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	matches := pattern.FindAllSubmatch(content, -1)
	var symbols []string
	for _, match := range matches {
		if len(match) > 1 {
			symbols = append(symbols, string(match[1]))
		}
	}
	return symbols
}

func findReferences(symbols []string, root string, excludeFiles []string) []string {
	if len(symbols) == 0 {
		return nil
	}
	args := []string{"--files-with-matches", "--type-add", "code:*.{ts,tsx,js,jsx,py,go}", "--type", "code"}
	for _, skip := range astSkipDirs {
		args = append(args, "--glob", "!"+skip)
	}
	
	var escapedSymbols []string
	for _, s := range symbols {
		escapedSymbols = append(escapedSymbols, regexp.QuoteMeta(s))
	}
	args = append(args, strings.Join(escapedSymbols, "|"), root)

	cmd := exec.Command("rg", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		return nil
	}

	excludeSet := make(map[string]bool)
	for _, f := range excludeFiles {
		abs, err := filepath.Abs(f)
		if err == nil {
			excludeSet[filepath.ToSlash(abs)] = true
		}
	}

	var files []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		abs, err := filepath.Abs(line)
		if err == nil {
			if !excludeSet[filepath.ToSlash(abs)] {
				files = append(files, line)
			}
		}
	}
	return files
}

func astNeighbors(seedFiles []string, root string) []string {
	allRefs := make(map[string]bool)
	for _, seed := range seedFiles {
		if _, err := os.Stat(seed); err != nil {
			continue
		}
		symbols := extractSymbols(seed)
		refs := findReferences(symbols, root, seedFiles)
		for _, ref := range refs {
			allRefs[ref] = true
		}
	}
	var res []string
	for r := range allRefs {
		res = append(res, r)
	}
	sort.Strings(res)
	return res
}

// === Import Graph ===

var importPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:import|export)\s+.*?from\s+['"]([^'"]+)['"]`),
	regexp.MustCompile(`require\s*\(\s*['"]([^'"]+)['"]\s*\)`),
	regexp.MustCompile(`(?m)^(?:from\s+([\w.]+)\s+import|import\s+([\w.]+))`),
	regexp.MustCompile(`"([\w./\-]+)"`),
}

var igSkipDirs = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "build": true, ".git": true, "__pycache__": true}
var igExtensions = map[string]bool{".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".py": true, ".go": true}

func resolveImport(importer, raw, root string) string {
	if strings.HasPrefix(raw, ".") {
		candidate := filepath.Join(filepath.Dir(importer), raw)
		for ext := range igExtensions {
			withExt := candidate + ext
			if _, err := os.Stat(withExt); err == nil {
				return withExt
			}
			index := filepath.Join(candidate, "index"+ext)
			if _, err := os.Stat(index); err == nil {
				return index
			}
		}
	}
	return ""
}

func importExpand(seedFiles []string, root string, maxHops int) []string {
	rootPath, _ := filepath.Abs(root)
	visited := make(map[string]bool)
	frontier := make(map[string]bool)

	for _, f := range seedFiles {
		p, err := filepath.Abs(f)
		if err == nil {
			if _, err := os.Stat(p); err == nil {
				frontier[p] = true
			}
		}
	}

	for i := 0; i < maxHops; i++ {
		nextFrontier := make(map[string]bool)
		for path := range frontier {
			if visited[path] {
				continue
			}
			skip := false
			parts := strings.Split(filepath.ToSlash(path), "/")
			for _, part := range parts {
				if igSkipDirs[part] {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			visited[path] = true
			if !igExtensions[filepath.Ext(path)] {
				continue
			}
			contentBytes, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			content := string(contentBytes)
			for _, pattern := range importPatterns {
				matches := pattern.FindAllStringSubmatch(content, -1)
				for _, match := range matches {
					raw := ""
					for _, g := range match[1:] {
						if g != "" {
							raw = g
							break
						}
					}
					if raw == "" {
						continue
					}
					resolved := resolveImport(path, raw, rootPath)
					if resolved != "" && !visited[resolved] {
						nextFrontier[resolved] = true
					}
				}
			}
		}
		frontier = nextFrontier
	}

	var res []string
	for p := range visited {
		res = append(res, p)
	}
	sort.Strings(res)
	return res
}

package core

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

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

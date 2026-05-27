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

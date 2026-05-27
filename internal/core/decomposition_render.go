package core

import (
	"fmt"
	"sort"
	"strings"
)

// RenderAsciiDag renders a deterministic ASCII map for a decomposition dependency DAG.
// The renderer is presentation-only: it does not validate, mutate, or persist
// task state. Unknown dependency IDs are ignored for level calculation but are
// still shown in the edge list so callers can see exactly what was provided.
func RenderAsciiDag(decomposition []any) string {
	if len(decomposition) == 0 {
		return ""
	}

	var childIDs []string
	depsByChild := make(map[string][]string)
	
	for _, entryAny := range decomposition {
		if entry, ok := entryAny.(map[string]any); ok {
			if childID, ok := entry["child_id"].(string); ok && childID != "" {
				childIDs = append(childIDs, childID)
				if depsAny, ok := asSlice(entry["depends_on"]); ok {
					var deps []string
					for _, depAny := range depsAny {
						if dep, ok := depAny.(string); ok && dep != "" {
							deps = append(deps, dep)
						}
					}
					sort.Strings(deps)
					depsByChild[childID] = deps
				}
			}
		}
	}

	if len(childIDs) == 0 {
		return ""
	}
	
	sort.Strings(childIDs)

	known := make(map[string]bool)
	for _, id := range childIDs {
		known[id] = true
	}

	levelCache := make(map[string]int)
	visiting := make(map[string]bool)

	var levelFor func(string) int
	levelFor = func(childID string) int {
		if lvl, ok := levelCache[childID]; ok {
			return lvl
		}
		if visiting[childID] {
			return 0
		}
		visiting[childID] = true
		
		maxDepLevel := -1
		for _, dep := range depsByChild[childID] {
			if known[dep] {
				depLvl := levelFor(dep)
				if depLvl > maxDepLevel {
					maxDepLevel = depLvl
				}
			}
		}
		
		level := 0
		if maxDepLevel >= 0 {
			level = maxDepLevel + 1
		}
		
		delete(visiting, childID)
		levelCache[childID] = level
		return level
	}

	levels := make(map[int][]string)
	for _, id := range childIDs {
		lvl := levelFor(id)
		levels[lvl] = append(levels[lvl], id)
	}

	var orderedLevels []int
	for lvl := range levels {
		orderedLevels = append(orderedLevels, lvl)
	}
	sort.Ints(orderedLevels)
	
	for _, lvl := range orderedLevels {
		sort.Strings(levels[lvl])
	}

	var headers []string
	var childColumns [][]string
	for _, lvl := range orderedLevels {
		headers = append(headers, fmt.Sprintf("L%d", lvl))
		var col []string
		for _, id := range levels[lvl] {
			col = append(col, fmt.Sprintf("[%s]", id))
		}
		childColumns = append(childColumns, col)
	}

	widths := make([]int, len(headers))
	rowCount := 0
	for i, header := range headers {
		w := len(header)
		for _, child := range childColumns[i] {
			if len(child) > w {
				w = len(child)
			}
		}
		widths[i] = w
		if len(childColumns[i]) > rowCount {
			rowCount = len(childColumns[i])
		}
	}

	var lines []string
	lines = append(lines, "Decomposition DAG:")
	lines = append(lines, "  order: "+strings.Join(headers, " -> "))
	
	var headerLine []string
	for i, header := range headers {
		headerLine = append(headerLine, fmt.Sprintf("%-*s", widths[i], header))
	}
	lines = append(lines, "  "+strings.TrimRight(strings.Join(headerLine, "  "), " "))

	for r := 0; r < rowCount; r++ {
		var rowCells []string
		for i, col := range childColumns {
			cell := ""
			if r < len(col) {
				cell = col[r]
			}
			rowCells = append(rowCells, fmt.Sprintf("%-*s", widths[i], cell))
		}
		lines = append(lines, "  "+strings.TrimRight(strings.Join(rowCells, "  "), " "))
	}

	type edge struct {
		from, to string
	}
	var edges []edge
	for _, id := range childIDs {
		for _, dep := range depsByChild[id] {
			edges = append(edges, edge{dep, id})
		}
	}
	
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].from != edges[j].from {
			return edges[i].from < edges[j].from
		}
		return edges[i].to < edges[j].to
	})

	lines = append(lines, "  edges:")
	if len(edges) > 0 {
		for _, e := range edges {
			lines = append(lines, fmt.Sprintf("    %s -> %s", e.from, e.to))
		}
	} else {
		lines = append(lines, "    (none)")
	}

	return strings.Join(lines, "\n")
}

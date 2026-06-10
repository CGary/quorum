package search

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hsme/core/src/core/indexer"
)

func GraphSearch(ctx context.Context, db *sql.DB, query string, limit int) ([]SearchResult, error) {
	// Minimal graph search logic: find nodes matching name and return their evidence memories
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT e.memory_id, 1.0
		FROM kg_nodes n
		JOIN kg_edge_evidence e ON n.id = e.source_node_id OR n.id = e.target_node_id
		WHERE n.canonical_name LIKE ?
		LIMIT ?
	`, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var res SearchResult
		if err := rows.Scan(&res.ID, &res.Score); err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

type DependencyEdge struct {
	SourceID     int64  `json:"source_id"`
	TargetID     int64  `json:"target_id"`
	RelationType string `json:"relation_type"`
	MemoryID     int64  `json:"memory_id"`
}

type TraceResult struct {
	Entity    string                   `json:"entity"`
	Nodes     []map[string]interface{} `json:"nodes"`
	Edges     []DependencyEdge         `json:"edges"`
	Truncated bool                     `json:"truncated"`
}

func TraceDependencies(ctx context.Context, db *sql.DB, entityName string, direction string, maxDepth int, maxNodes int) (*TraceResult, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	if maxNodes <= 0 {
		maxNodes = 100
	}

	// Simetría Semántica: Usamos el mismo algoritmo que el indexador.
	searchName, _ := indexer.CanonicalizeName(entityName)
	fuzzyName := "%" + entityName + "%"

	// Recolectamos todos los nodos alcanzables, pero ordenados por profundidad mínima
	// y con deduplicación explícita por nodo al final. Esto permite aplicar un hard cap
	// determinista de max_nodes y marcar truncation correctamente.
	query := `
		WITH RECURSIVE trace(id, depth) AS (
			SELECT id, 0
			  FROM kg_nodes
			 WHERE canonical_name = ?
			    OR display_name LIKE ?
			UNION
			SELECT
				CASE WHEN t.id = e.source_node_id THEN e.target_node_id ELSE e.source_node_id END,
				t.depth + 1
			  FROM kg_edge_evidence e
			  JOIN trace t ON (t.id = e.source_node_id OR t.id = e.target_node_id)
			 WHERE t.depth < ?
			   AND (
					(? = 'both') OR
					(? = 'downstream' AND t.id = e.source_node_id) OR
					(? = 'upstream' AND t.id = e.target_node_id)
			   )
		)
		SELECT id, MIN(depth) AS min_depth
		  FROM trace
		 GROUP BY id
		 ORDER BY min_depth, id
	`
	rows, err := db.QueryContext(ctx, query, searchName, fuzzyName, maxDepth, direction, direction, direction)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &TraceResult{
		Entity: entityName,
		Nodes:  []map[string]interface{}{},
		Edges:  []DependencyEdge{},
	}

	selectedIDs := make([]int64, 0, maxNodes)
	reachableIDs := make(map[int64]bool, maxNodes)
	for rows.Next() {
		var id int64
		var minDepth int
		if err := rows.Scan(&id, &minDepth); err != nil {
			return nil, err
		}
		if len(selectedIDs) < maxNodes {
			selectedIDs = append(selectedIDs, id)
			reachableIDs[id] = true
			continue
		}
		result.Truncated = true
	}

	if len(selectedIDs) == 0 {
		return result, nil
	}

	// 1. Obtener detalles de los nodos seleccionados preservando el orden.
	for _, nodeID := range selectedIDs {
		var nodeType, displayName string
		err := db.QueryRowContext(ctx, "SELECT type, display_name FROM kg_nodes WHERE id = ?", nodeID).Scan(&nodeType, &displayName)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, err
		}
		result.Nodes = append(result.Nodes, map[string]interface{}{
			"id":   nodeID,
			"type": nodeType,
			"name": displayName,
		})
	}

	// 2. Obtener todas las aristas entre nodos seleccionados en una sola query.
	args := make([]any, 0, len(selectedIDs)*2)
	placeholders := make([]string, len(selectedIDs))
	for i, id := range selectedIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	inClause := strings.Join(placeholders, ",")
	args = append(args, args[:len(selectedIDs)]...)
	edgeRows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT source_node_id, target_node_id, relation_type, memory_id
		  FROM kg_edge_evidence
		 WHERE source_node_id IN (%s)
		   AND target_node_id IN (%s)
		 ORDER BY source_node_id, target_node_id, relation_type, memory_id
	`, inClause, inClause), args...)
	if err != nil {
		return nil, err
	}
	defer edgeRows.Close()

	seenEdges := make(map[string]bool)
	for edgeRows.Next() {
		var edge DependencyEdge
		if err := edgeRows.Scan(&edge.SourceID, &edge.TargetID, &edge.RelationType, &edge.MemoryID); err != nil {
			return nil, err
		}
		if !reachableIDs[edge.SourceID] || !reachableIDs[edge.TargetID] {
			continue
		}
		key := fmt.Sprintf("%d:%d:%s:%d", edge.SourceID, edge.TargetID, edge.RelationType, edge.MemoryID)
		if seenEdges[key] {
			continue
		}
		seenEdges[key] = true
		result.Edges = append(result.Edges, edge)
	}

	return result, nil
}

package search

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

const embedTimeout = 10 * time.Second

func sanitizeFTS(q string) string {
	fields := strings.Fields(q)
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(parts, " ")
}

type Embedder interface {
	GenerateVector(ctx context.Context, text string) ([]float32, error)
	Dimension() int
	ModelID() string
}

type ChunkHighlight struct {
	ChunkID    int64  `json:"chunk_id"`
	ChunkIndex int    `json:"chunk_index"`
	Text       string `json:"text"`
}

type MemorySearchResult struct {
	MemoryID       int64            `json:"memory_id"`
	Score          float64          `json:"score"`
	IsSuperseded   bool             `json:"is_superseded"`
	VectorCoverage string           `json:"vector_coverage"`
	Highlights     []ChunkHighlight `json:"highlights"`
}

type ExactMatchResult struct {
	MemoryID   int64   `json:"memory_id"`
	ChunkID    int64   `json:"chunk_id"`
	ChunkIndex int     `json:"chunk_index"`
	Text       string  `json:"text"`
	Score      float64 `json:"-"`
}

type SearchResult struct {
	ID    int64
	Score float64
}

type recencyCandidate struct {
	memoryID     int64
	chunkID      int64
	chunkIndex   int
	chunkText    string
	memoryStatus string
	createdAt    time.Time
	score        float64
}

func hasRecencyIntent(query string) bool {
	q := strings.ToLower(query)
	markers := []string{"latest", "recent", "last", "most recent", "today", "yesterday"}
	for _, marker := range markers {
		if strings.Contains(q, marker) {
			return true
		}
	}
	return false
}

func topicTerms(query string) []string {
	stop := map[string]struct{}{
		"what": {}, "did": {}, "we": {}, "do": {}, "the": {}, "last": {}, "latest": {}, "recent": {},
		"most": {}, "today": {}, "yesterday": {}, "about": {}, "changes": {}, "change": {}, "made": {},
		"session": {}, "sessions": {}, "decision": {}, "decisions": {}, "bugfix": {}, "bugfixes": {},
		"fix": {}, "architecture": {}, "note": {}, "notes": {}, "to": {}, "in": {}, "for": {}, "and": {},
	}
	cleaned := strings.NewReplacer("?", " ", ",", " ", ":", " ", ";", " ", "(", " ", ")", " ").Replace(strings.ToLower(query))
	var terms []string
	for _, field := range strings.Fields(cleaned) {
		if len(field) < 3 {
			continue
		}
		if _, ok := stop[field]; ok {
			continue
		}
		terms = append(terms, field)
	}
	return terms
}

func inferredSourceTypes(query string) []string {
	q := strings.ToLower(query)
	switch {
	case strings.Contains(q, "session"):
		return []string{"session_summary"}
	case strings.Contains(q, "bugfix") || strings.Contains(q, " fix") || strings.HasSuffix(q, "fix"):
		return []string{"bugfix"}
	case strings.Contains(q, "decision"):
		return []string{"decision"}
	case strings.Contains(q, "architecture") || strings.Contains(q, "worker remediation"):
		return []string{"architecture"}
	case strings.Contains(q, "schema"):
		return []string{"architecture", "bugfix", "decision"}
	default:
		return nil
	}
}

func recencyCandidates(ctx context.Context, db *sql.DB, query string, limit int, project string) ([]recencyCandidate, error) {
	if limit <= 0 || !hasRecencyIntent(query) {
		return nil, nil
	}

	terms := topicTerms(query)
	sourceTypes := inferredSourceTypes(query)
	clauses := []string{"m.status = 'active'", "m.superseded_by IS NULL"}
	args := []any{}
	if project != "" {
		clauses = append(clauses, "m.project = ?")
		args = append(args, project)
	}
	if len(sourceTypes) > 0 {
		placeholders := strings.Repeat(",?", len(sourceTypes))[1:]
		clauses = append(clauses, fmt.Sprintf("m.source_type IN (%s)", placeholders))
		for _, sourceType := range sourceTypes {
			args = append(args, sourceType)
		}
	}
	for _, term := range terms {
		clauses = append(clauses, "(lower(m.raw_content) LIKE ? OR lower(m.project) LIKE ? OR lower(m.source_type) LIKE ?)")
		like := "%" + term + "%"
		args = append(args, like, like, like)
	}

	args = append(args, limit)
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT m.id, c.id, c.chunk_index, c.chunk_text, m.status, m.created_at
		  FROM memories m
		  JOIN memory_chunks c ON c.memory_id = m.id
		 WHERE %s
		   AND c.chunk_index = (SELECT MIN(c2.chunk_index) FROM memory_chunks c2 WHERE c2.memory_id = m.id)
		 ORDER BY datetime(m.created_at) DESC, m.id DESC
		 LIMIT ?`, strings.Join(clauses, " AND ")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := []recencyCandidate{}
	for rows.Next() {
		var c recencyCandidate
		if err := rows.Scan(&c.memoryID, &c.chunkID, &c.chunkIndex, &c.chunkText, &c.memoryStatus, &c.createdAt); err != nil {
			return nil, err
		}
		c.score = 1.0 - (float64(len(candidates)) * 0.001)
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

func FuzzySearch(ctx context.Context, db *sql.DB, embedder Embedder, query string, limit int, project string) ([]MemorySearchResult, error) {
	lexicalResults, err := LexicalSearch(ctx, db, query, limit*2, project)
	if err != nil {
		return nil, err
	}

	var vectorResults []SearchResult
	vectorAvailable := false
	if embedder != nil {
		embedCtx, cancel := context.WithTimeout(ctx, embedTimeout)
		vector, err := embedder.GenerateVector(embedCtx, query)
		cancel()
		if err == nil {
			vectorResults, err = VectorSearch(ctx, db, vector, limit*2, project)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error en búsqueda vectorial: %v\n", err)
				vectorResults = nil
			} else {
				vectorAvailable = true
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error generando vector para búsqueda: %v\n", err)
		}
	}

	fusedChunks := RRF(limit*2, lexicalResults, vectorResults)
	if len(fusedChunks) == 0 {
		return nil, nil
	}

	chunkIDs := make([]any, len(fusedChunks))
	for i, c := range fusedChunks {
		chunkIDs[i] = c.ID
	}
	placeholders := strings.Repeat(",?", len(fusedChunks))[1:]
	chunkRows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT c.id, c.memory_id, c.chunk_index, c.chunk_text, m.status, m.created_at
		  FROM memory_chunks c
		  JOIN memories m ON m.id = c.memory_id
		 WHERE c.id IN (%s)`, placeholders), chunkIDs...)
	if err != nil {
		return nil, fmt.Errorf("batch chunk lookup: %w", err)
	}

	type chunkMeta struct {
		memoryID     int64
		chunkIndex   int
		chunkText    string
		memoryStatus string
		createdAt    time.Time
	}
	chunkByID := make(map[int64]chunkMeta, len(fusedChunks))
	for chunkRows.Next() {
		var id int64
		var cm chunkMeta
		if err := chunkRows.Scan(&id, &cm.memoryID, &cm.chunkIndex, &cm.chunkText, &cm.memoryStatus, &cm.createdAt); err != nil {
			chunkRows.Close()
			return nil, err
		}
		chunkByID[id] = cm
	}
	chunkRows.Close()

	memoryScores := make(map[int64]float64)
	memoryHighlights := make(map[int64][]ChunkHighlight)
	memoryStatus := make(map[int64]string)

	now := time.Now()
	recencyIntent := hasRecencyIntent(query)

	for _, chunk := range fusedChunks {
		cm, ok := chunkByID[chunk.ID]
		if !ok {
			continue
		}
		memoryStatus[cm.memoryID] = cm.memoryStatus

		score := chunk.Score
		if GlobalDecayConfig.Enabled && recencyIntent {
			age := AgeInDays(now, cm.createdAt)
			score = score * DecayFactor(age, GlobalDecayConfig.HalfLifeDays)
		}

		if score > memoryScores[cm.memoryID] {
			memoryScores[cm.memoryID] = score
		}
		if len(memoryHighlights[cm.memoryID]) < 3 {
			memoryHighlights[cm.memoryID] = append(memoryHighlights[cm.memoryID], ChunkHighlight{
				ChunkID:    chunk.ID,
				ChunkIndex: cm.chunkIndex,
				Text:       cm.chunkText,
			})
		}
	}

	if GlobalDecayConfig.Enabled && recencyIntent {
		candidates, err := recencyCandidates(ctx, db, query, limit, project)
		if err != nil {
			return nil, fmt.Errorf("recency candidates: %w", err)
		}
		for _, candidate := range candidates {
			memoryStatus[candidate.memoryID] = candidate.memoryStatus
			if candidate.score > memoryScores[candidate.memoryID] {
				memoryScores[candidate.memoryID] = candidate.score
			}
			if len(memoryHighlights[candidate.memoryID]) < 3 {
				memoryHighlights[candidate.memoryID] = append(memoryHighlights[candidate.memoryID], ChunkHighlight{
					ChunkID:    candidate.chunkID,
					ChunkIndex: candidate.chunkIndex,
					Text:       candidate.chunkText,
				})
			}
		}
	}

	coverageByMemory := make(map[int64]string, len(memoryScores))
	if len(memoryScores) > 0 {
		memIDs := make([]any, 0, len(memoryScores))
		for id := range memoryScores {
			memIDs = append(memIDs, id)
		}
		memPlaceholders := strings.Repeat(",?", len(memIDs))[1:]
		covRows, err := db.QueryContext(ctx, fmt.Sprintf(`
			SELECT c.memory_id, COUNT(c.id) AS total,
			       SUM(CASE WHEN v.rowid IS NOT NULL THEN 1 ELSE 0 END) AS with_vec
			  FROM memory_chunks c
			  LEFT JOIN memory_chunks_vec v ON v.rowid = c.id
			 WHERE c.memory_id IN (%s)
			 GROUP BY c.memory_id`, memPlaceholders), memIDs...)
		if err != nil {
			return nil, fmt.Errorf("batch coverage lookup: %w", err)
		}
		for covRows.Next() {
			var mid int64
			var total, withVec int
			if err := covRows.Scan(&mid, &total, &withVec); err != nil {
				covRows.Close()
				return nil, err
			}
			switch {
			case total > 0 && withVec == total:
				coverageByMemory[mid] = "complete"
			case withVec > 0:
				coverageByMemory[mid] = "partial"
			default:
				coverageByMemory[mid] = "none"
			}
		}
		covRows.Close()
	}

	var results []MemorySearchResult
	for memoryID, score := range memoryScores {
		isSuperseded := memoryStatus[memoryID] == "superseded"
		finalScore := score
		if isSuperseded {
			finalScore *= 0.5
		}
		coverage, ok := coverageByMemory[memoryID]
		if !ok {
			coverage = "none"
		}
		// Si tenemos vectores pero la búsqueda no pudo usarlos, la cobertura
		// efectiva para este resultado es partial (solo lexical).
		if coverage == "complete" && !vectorAvailable {
			coverage = "partial"
		}
		results = append(results, MemorySearchResult{
			MemoryID:       memoryID,
			Score:          finalScore,
			IsSuperseded:   isSuperseded,
			VectorCoverage: coverage,
			Highlights:     memoryHighlights[memoryID],
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func LexicalSearch(ctx context.Context, db *sql.DB, query string, limit int, project string) ([]SearchResult, error) {
	safe := sanitizeFTS(query)
	if safe == "" {
		return nil, nil
	}

	var rows *sql.Rows
	var err error
	if project != "" {
		rows, err = db.QueryContext(ctx, `
			SELECT f.rowid, f.rank 
			FROM memory_chunks_fts f
			JOIN memory_chunks c ON c.id = f.rowid
			JOIN memories m ON m.id = c.memory_id
			WHERE f.chunk_text MATCH ? AND m.project = ?
			ORDER BY f.rank LIMIT ?`, safe, project, limit)
	} else {
		rows, err = db.QueryContext(ctx, "SELECT rowid, rank FROM memory_chunks_fts WHERE chunk_text MATCH ? ORDER BY rank LIMIT ?", safe, limit)
	}
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

func VectorSearch(ctx context.Context, db *sql.DB, vector []float32, limit int, project string) ([]SearchResult, error) {
	blob, err := vec.SerializeFloat32(vector)
	if err != nil {
		return nil, err
	}

	var rows *sql.Rows
	if project != "" {
		// 1. Get KNN candidates
		candidateRows, err := db.QueryContext(ctx, "SELECT rowid FROM memory_chunks_vec WHERE embedding MATCH ? LIMIT ?", blob, limit*10)
		if err != nil {
			return nil, err
		}
		defer candidateRows.Close()

		var candidates []any
		for candidateRows.Next() {
			var id int64
			if err := candidateRows.Scan(&id); err != nil {
				return nil, err
			}
			candidates = append(candidates, id)
		}
		if len(candidates) == 0 {
			return nil, nil
		}

		// 2. Filter candidates by project
		placeholders := strings.Repeat(",?", len(candidates))[1:]
		args := append(candidates, project)
		rows, err = db.QueryContext(ctx, fmt.Sprintf(`
			SELECT c.id 
			FROM memory_chunks c
			JOIN memories m ON m.id = c.memory_id
			WHERE c.id IN (%s) AND m.project = ?`, placeholders), args...)
	} else {
		rows, err = db.QueryContext(ctx, "SELECT rowid FROM memory_chunks_vec WHERE embedding MATCH ? LIMIT ?", blob, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []SearchResult
	for rows.Next() {
		var res SearchResult
		if err := rows.Scan(&res.ID); err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

func ExactSearch(ctx context.Context, db *sql.DB, keyword string, limit int, project string) ([]ExactMatchResult, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}

	seen := make(map[int64]struct{})
	results, err := exactSearchFTS(ctx, db, keyword, limit, seen, project)
	if err != nil {
		return nil, err
	}

	fallbackLimit := limit
	if GlobalDecayConfig.Enabled {
		fallbackLimit = limit * 5
	} else if len(results) >= limit {
		return results, nil
	} else {
		fallbackLimit = limit - len(results)
	}

	fallback, err := exactSearchSubstring(ctx, db, keyword, fallbackLimit, seen, project)
	if err != nil {
		return nil, err
	}

	combined := append(results, fallback...)

	if GlobalDecayConfig.Enabled {
		sort.Slice(combined, func(i, j int) bool {
			return combined[i].Score < combined[j].Score // Ascending: lower score is better (BM25 is negative)
		})
	}

	if len(combined) > limit {
		combined = combined[:limit]
	}

	return combined, nil
}

func exactSearchFTS(ctx context.Context, db *sql.DB, keyword string, limit int, seen map[int64]struct{}, project string) ([]ExactMatchResult, error) {
	safe := sanitizeFTS(keyword)
	if safe == "" || limit <= 0 {
		return nil, nil
	}

	queryLimit := limit
	if GlobalDecayConfig.Enabled {
		queryLimit = limit * 5
	}

	var rows *sql.Rows
	var err error
	if project != "" {
		rows, err = db.QueryContext(ctx, `
			SELECT c.memory_id, c.id, c.chunk_index, c.chunk_text, bm25(memory_chunks_fts), m.created_at
			FROM memory_chunks_fts f
			JOIN memory_chunks c ON c.id = f.rowid
			JOIN memories m ON m.id = c.memory_id
			WHERE f.chunk_text MATCH ? AND m.project = ?
			ORDER BY c.memory_id, c.chunk_index
			LIMIT ?
		`, safe, project, queryLimit)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT c.memory_id, c.id, c.chunk_index, c.chunk_text, bm25(memory_chunks_fts), m.created_at
			FROM memory_chunks_fts f
			JOIN memory_chunks c ON c.id = f.rowid
			JOIN memories m ON m.id = c.memory_id
			WHERE f.chunk_text MATCH ?
			ORDER BY c.memory_id, c.chunk_index
			LIMIT ?
		`, safe, queryLimit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	var results []ExactMatchResult
	for rows.Next() {
		var res ExactMatchResult
		var bm25 float64
		var createdAt time.Time
		if err := rows.Scan(&res.MemoryID, &res.ChunkID, &res.ChunkIndex, &res.Text, &bm25, &createdAt); err != nil {
			return nil, err
		}
		seen[res.ChunkID] = struct{}{}

		score := bm25
		if GlobalDecayConfig.Enabled {
			age := AgeInDays(now, createdAt)
			score = score * DecayFactor(age, GlobalDecayConfig.HalfLifeDays)
		}
		res.Score = score
		results = append(results, res)
	}
	return results, nil
}

func exactSearchSubstring(ctx context.Context, db *sql.DB, keyword string, limit int, seen map[int64]struct{}, project string) ([]ExactMatchResult, error) {
	if limit <= 0 {
		return nil, nil
	}

	queryLimit := limit + len(seen)
	if GlobalDecayConfig.Enabled {
		queryLimit = limit * 5
	}

	var rows *sql.Rows
	var err error
	if project != "" {
		rows, err = db.QueryContext(ctx, `
			SELECT c.memory_id, c.id, c.chunk_index, c.chunk_text, m.created_at
			FROM memory_chunks c
			JOIN memories m ON m.id = c.memory_id
			WHERE instr(lower(c.chunk_text), lower(?)) > 0 AND m.project = ?
			ORDER BY c.memory_id, c.chunk_index
			LIMIT ?
		`, keyword, project, queryLimit)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT c.memory_id, c.id, c.chunk_index, c.chunk_text, m.created_at
			FROM memory_chunks c
			JOIN memories m ON m.id = c.memory_id
			WHERE instr(lower(c.chunk_text), lower(?)) > 0
			ORDER BY c.memory_id, c.chunk_index
			LIMIT ?
		`, keyword, queryLimit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	var results []ExactMatchResult
	for rows.Next() {
		var res ExactMatchResult
		var createdAt time.Time
		if err := rows.Scan(&res.MemoryID, &res.ChunkID, &res.ChunkIndex, &res.Text, &createdAt); err != nil {
			return nil, err
		}
		if _, ok := seen[res.ChunkID]; ok {
			continue
		}
		seen[res.ChunkID] = struct{}{}

		score := -0.0001
		if GlobalDecayConfig.Enabled {
			age := AgeInDays(now, createdAt)
			score = score * DecayFactor(age, GlobalDecayConfig.HalfLifeDays)
		}
		res.Score = score
		results = append(results, res)

		if !GlobalDecayConfig.Enabled && len(results) >= limit {
			break
		}
	}
	return results, nil
}

func RRF(limit int, resultSets ...[]SearchResult) []SearchResult {
	scores := make(map[int64]float64)
	k := 60.0
	for _, resultSet := range resultSets {
		for i, res := range resultSet {
			scores[res.ID] += 1.0 / (k + float64(i+1))
		}
	}

	var fused []SearchResult
	for id, score := range scores {
		fused = append(fused, SearchResult{ID: id, Score: score})
	}

	sort.Slice(fused, func(i, j int) bool {
		return fused[i].Score > fused[j].Score
	})

	if len(fused) > limit {
		return fused[:limit]
	}
	return fused
}

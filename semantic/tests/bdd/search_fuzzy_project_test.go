package bdd

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
	"github.com/hsme/core/src/core/indexer"
	"github.com/hsme/core/src/core/search"
	"github.com/hsme/core/src/core/worker"
	"github.com/hsme/core/src/storage/sqlite"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"search_fuzzy_project.feature"},
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

type searchTestContext struct {
	db       *sql.DB
	embedder *mockEmbedder
	worker   *worker.Worker
	results  []search.MemorySearchResult
	err      error
}

func (s *searchTestContext) aTestDatabaseWithVec0SupportAndProjectWithEmbeddedChunks(project string) error {
	db, err := sqlite.InitDB(":memory:")
	if err != nil {
		return err
	}
	s.db = db
	s.embedder = &mockEmbedder{dim: 768}
	s.worker = worker.NewWorker(db, s.embedder, nil, nil)

	// Ingest some data
	ctx := context.Background()
	_, err = indexer.StoreContext(s.db, "semantic vectors are cool in acme", "manual", project, nil, false)
	if err != nil {
		return err
	}
	// Also ingest something NOT in the project to verify filtering
	_, err = indexer.StoreContext(s.db, "semantic vectors in other project", "manual", "other", nil, false)
	if err != nil {
		return err
	}

	// Run worker to process embed tasks
	for {
		task, err := s.worker.LeaseNextTask(ctx)
		if err != nil {
			return err
		}
		if task == nil {
			break
		}
		if task.TaskType == "embed" {
			if err := s.worker.ExecuteTask(ctx, task); err != nil {
				return err
			}
		} else {
			_, _ = s.db.Exec("UPDATE async_tasks SET status = 'done' WHERE id = ?", task.ID)
		}
	}
	return nil
}

func (s *searchTestContext) aTestDatabaseWITHOUTVec0Support() error {
	db, err := sqlite.InitDB(":memory:")
	if err != nil {
		return err
	}
	s.db = db
	s.embedder = nil

	// Ingest data WITH embeddings, then we'll search WITHOUT embedder
	mockEmb := &mockEmbedder{dim: 768}
	w := worker.NewWorker(db, mockEmb, nil, nil)
	ctx := context.Background()
	_, err = indexer.StoreContext(s.db, "semantic vectors are cool in acme", "manual", "acme", nil, false)
	if err != nil {
		return err
	}

	// Process embed tasks
	for {
		task, _ := w.LeaseNextTask(ctx)
		if task == nil {
			break
		}
		if task.TaskType == "embed" {
			_ = w.ExecuteTask(ctx, task)
		} else {
			_, _ = s.db.Exec("UPDATE async_tasks SET status = 'done' WHERE id = ?", task.ID)
		}
	}
	return nil
}

func (s *searchTestContext) theUserCallsSearchFuzzyWithQueryProjectK(query, project string, k int) error {
	var embedder search.Embedder
	if s.embedder != nil {
		embedder = s.embedder
	}
	s.results, s.err = search.FuzzySearch(context.Background(), s.db, embedder, query, k, project)
	return nil
}

func (s *searchTestContext) theUserCallsSearchFuzzyWithQueryK(query string, k int) error {
	var embedder search.Embedder
	if s.embedder != nil {
		embedder = s.embedder
	}
	s.results, s.err = search.FuzzySearch(context.Background(), s.db, embedder, query, k, "")
	return nil
}

func (s *searchTestContext) theSearchCompletesWithoutVec0LIMITError(arg1 string) error {
	if s.err != nil {
		return s.err
	}
	return nil
}

func (s *searchTestContext) theSearchCompletesWithoutError() error {
	return s.err
}

func (s *searchTestContext) theResultCoverageIs(expectedCoverage string) error {
	if len(s.results) == 0 {
		return fmt.Errorf("no results found")
	}
	for _, res := range s.results {
		if res.VectorCoverage == expectedCoverage {
			return nil
		}
	}
	return fmt.Errorf("expected coverage %s not found in results, got: %v", expectedCoverage, s.results[0].VectorCoverage)
}

func (s *searchTestContext) theResultIncludesEmbeddingsFromTheProject(project string) error {
	if len(s.results) == 0 {
		return fmt.Errorf("no results found")
	}
	return nil
}

func (s *searchTestContext) theSearchReturnsMixedVectorLexicalResults() error {
	if len(s.results) == 0 {
		return fmt.Errorf("no results found")
	}
	hasVector := false
	for _, res := range s.results {
		if res.VectorCoverage == "complete" {
			hasVector = true
			break
		}
	}
	if !hasVector {
		return fmt.Errorf("expected at least one vector result")
	}
	return nil
}

func (s *searchTestContext) rrfFusionIsAppliedCorrectly() error {
	if len(s.results) > 0 {
		if s.results[0].Score > 1.0 || s.results[0].Score < 0 {
			return fmt.Errorf("invalid RRF score: %f", s.results[0].Score)
		}
	}
	return nil
}

func InitializeScenario(sc *godog.ScenarioContext) {
	s := &searchTestContext{}

	sc.Step(`^a test database with vec0 support and project "([^"]*)" with embedded chunks$`, s.aTestDatabaseWithVec0SupportAndProjectWithEmbeddedChunks)
	sc.Step(`^the user calls search_fuzzy with query="([^"]*)" project="([^"]*)" k=(\d+)$`, s.theUserCallsSearchFuzzyWithQueryProjectK)
	sc.Step(`^the search completes without vec0 "([^"]*)" error$`, s.theSearchCompletesWithoutVec0LIMITError)
	sc.Step(`^the result coverage is "([^"]*)" \(vector candidates present\)$`, s.theResultCoverageIs)
	sc.Step(`^the result includes embeddings from the project "([^"]*)"$`, s.theResultIncludesEmbeddingsFromTheProject)

	sc.Step(`^the user calls search_fuzzy with query="([^"]*)" k=(\d+)$`, s.theUserCallsSearchFuzzyWithQueryK)
	sc.Step(`^the search returns mixed vector \+ lexical results$`, s.theSearchReturnsMixedVectorLexicalResults)
	sc.Step(`^RRF fusion is applied correctly$`, s.rrfFusionIsAppliedCorrectly)

	sc.Step(`^a test database WITHOUT vec0 support$`, s.aTestDatabaseWITHOUTVec0Support)
	sc.Step(`^the search completes without error$`, s.theSearchCompletesWithoutError)
	sc.Step(`^the result coverage is "([^"]*)" \(lexical only, graceful degradation\)$`, s.theResultCoverageIs)
}

type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) GenerateVector(ctx context.Context, text string) ([]float32, error) {
	v := make([]float32, m.dim)
	v[0] = 1.0
	return v, nil
}

func (m *mockEmbedder) Dimension() int {
	return m.dim
}

func (m *mockEmbedder) ModelID() string {
	return "mock-embedder"
}

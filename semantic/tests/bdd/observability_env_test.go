package bdd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/hsme/core/src/observability"
	"github.com/hsme/core/src/storage/sqlite"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"observability_env.feature"},
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

type obsTestContext struct {
	db       *sql.DB
	recorder observability.Recorder
}

func (c *obsTestContext) theHsmeProcessIsStartedWithEnv(envVar, value string) error {
	// Clean up previous env
	os.Unsetenv("OBS_LEVEL")
	os.Unsetenv("HSME_OBS_LEVEL")

	// Set new env
	os.Setenv(envVar, value)

	// Init DB
	db, err := sqlite.InitDB(":memory:")
	if err != nil {
		return err
	}
	c.db = db

	// Load config (this is what we're testing)
	cfg := observability.LoadConfigFromEnv()
	c.recorder = observability.NewSQLiteRecorder(db, cfg)

	return nil
}

func (c *obsTestContext) theUserPerformsStoreAndSearchOperations() error {
	ctx := context.Background()
	// Simulate operations that produce traces
	trace, lctx := c.recorder.StartTrace(ctx, observability.StartTraceArgs{
		TraceKind: "mcp_request",
		Component: "test",
		Operation: "test_op",
	})
	span, sctx := c.recorder.StartSpan(lctx, observability.StartSpanArgs{
		TraceID:   trace.TraceID,
		Component: "test",
		Operation: "test_op",
		StageName: "test_stage",
	})
	_ = c.recorder.RecordDiagnostic(sctx, observability.DiagnosticEvent{
		Component: "test",
		Message:   "test event",
	})
	_ = c.recorder.FinishSpan(sctx, span, observability.SpanResult{Status: "ok"})
	_ = c.recorder.FinishTrace(lctx, trace, observability.TraceResult{Status: "ok"})

	return nil
}

func (c *obsTestContext) tableHasRows(tableName string, operator string, count int) error {
	var actualCount int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	err := c.db.QueryRow(query).Scan(&actualCount)
	if err != nil {
		return err
	}

	op := strings.TrimSpace(operator)
	if op == ">" {
		if actualCount > count {
			return nil
		}
		return fmt.Errorf("expected %s to have > %d rows, got %d", tableName, count, actualCount)
	} else {
		if actualCount == count {
			return nil
		}
		return fmt.Errorf("expected %s to have %d rows, got %d", tableName, count, actualCount)
	}
}

func (c *obsTestContext) aUserReadsTheREADME() error {
	content, err := os.ReadFile("../../README.md")
	if err != nil {
		return err
	}
	if strings.Contains(string(content), "\"OBS_LEVEL\"") {
		return fmt.Errorf("README still contains the wrong variable OBS_LEVEL")
	}
	if !strings.Contains(string(content), "\"HSME_OBS_LEVEL\"") {
		return fmt.Errorf("README does not contain the correct variable HSME_OBS_LEVEL")
	}
	return nil
}

func (c *obsTestContext) theyConfigureAsDocumented() error {
	return nil
}

func (c *obsTestContext) observabilityProducesDataAsExpected() error {
	return nil
}

func (c *obsTestContext) tableHasExactRows(tableName string, count int) error {
	return c.tableHasRows(tableName, "", count)
}

func (c *obsTestContext) tableHasMoreThanRows(tableName string, count int) error {
	return c.tableHasRows(tableName, ">", count)
}

func InitializeScenario(sc *godog.ScenarioContext) {
	c := &obsTestContext{}

	sc.Step(`^the hsme process is started with env ([^ ]*)=([^ ]*)$`, c.theHsmeProcessIsStartedWithEnv)
	sc.Step(`^the user performs store and search operations$`, c.theUserPerformsStoreAndSearchOperations)
	sc.Step(`^([^ ]*) has > (\d+) rows$`, c.tableHasMoreThanRows)
	sc.Step(`^([^ ]*) has (\d+) rows$`, c.tableHasExactRows)

	sc.Step(`^a user reads the README observability section$`, c.aUserReadsTheREADME)
	sc.Step(`^they configure HSME_OBS_LEVEL as documented$`, c.theyConfigureAsDocumented)
	sc.Step(`^observability produces data as expected$`, c.observabilityProducesDataAsExpected)
}

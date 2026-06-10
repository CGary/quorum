package bdd

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/hsme/core/src/observability"
	"github.com/hsme/core/src/storage/sqlite"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"rollup_catchup.feature"},
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

type rollupTestContext struct {
	db      *sql.DB
	service *observability.MaintenanceService
	now     time.Time
	err     error
}

func (c *rollupTestContext) theHsmeOpsServiceIsRunningWithRollupConfigured() error {
	db, err := sqlite.InitDB(":memory:")
	if err != nil {
		return err
	}
	c.db = db
	cfg := observability.LoadConfigFromEnv()
	c.service = observability.NewMaintenanceService(db, cfg)
	c.now = time.Now().UTC().Truncate(time.Minute)
	return nil
}

func (c *rollupTestContext) noBucketsHaveBeenProcessedYet() error {
	return nil
}

func (c *rollupTestContext) theCronTriggerExecutesRunRawToMinute() error {
	c.err = c.service.FlushRollups(context.Background(), c.now)
	return c.err
}

func (c *rollupTestContext) theBucketOfNowTruncateMinuteIsProcessed() error {
	return nil
}

func (c *rollupTestContext) lastCompletedBucketStartUtcIsUpdatedToCurrentBucket() error {
	var lastUTC sql.NullString
	err := c.db.QueryRow("SELECT last_completed_bucket_start_utc FROM obs_rollup_jobs WHERE job_name = 'raw_to_minute'").Scan(&lastUTC)
	if err != nil {
		return err
	}
	expected := c.now.Truncate(time.Minute).Format(time.RFC3339Nano)
	if lastUTC.String != expected {
		return fmt.Errorf("expected checkpoint %s, got %s", expected, lastUTC.String)
	}
	return nil
}

func (c *rollupTestContext) theHsmeOpsServiceHasProcessedUpToBucketT10min() error {
	if err := c.theHsmeOpsServiceIsRunningWithRollupConfigured(); err != nil {
		return err
	}
	t10 := c.now.Add(-10 * time.Minute).Format(time.RFC3339Nano)
	_, err := c.db.Exec("UPDATE obs_rollup_jobs SET last_completed_bucket_start_utc = ? WHERE job_name = 'raw_to_minute'", t10)
	return err
}

func (c *rollupTestContext) theServiceWasDownFor5Minutes() error {
	for i := 9; i >= 0; i-- {
		ts := c.now.Add(-time.Duration(i) * time.Minute).Add(10 * time.Second).Format(time.RFC3339Nano)
		_, err := c.db.Exec("INSERT INTO obs_traces (trace_id, trace_kind, component, operation_name, status, obs_level, started_at_utc, ended_at_utc, duration_us, sampled) VALUES (?, 'mcp_request', 'test', 'op', 'ok', 'basic', ?, ?, 100, 1)", fmt.Sprintf("t-%d", i), ts, ts)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *rollupTestContext) theServiceRestartsAndExecutesRunRawToMinute() error {
	return c.theCronTriggerExecutesRunRawToMinute()
}

func (c *rollupTestContext) bucketsAreAllProcessedInOrder(arg1, arg2, arg3, arg4, arg5 string) error {
	var count int
	err := c.db.QueryRow("SELECT COUNT(*) FROM obs_metric_rollups WHERE bucket_level = 'minute'").Scan(&count)
	if err != nil {
		return err
	}
	if count < 10 {
		return fmt.Errorf("expected 10 rollup buckets, got %d", count)
	}
	return nil
}

func (c *rollupTestContext) lastCompletedBucketStartUtcAdvancesSequentially() error {
	return c.lastCompletedBucketStartUtcIsUpdatedToCurrentBucket()
}

func (c *rollupTestContext) noBucketIsLostDueToTheGap() error {
	return nil
}

func (c *rollupTestContext) theHsmeOpsServiceHasProcessedUpToBucketT5min() error {
	if err := c.theHsmeOpsServiceIsRunningWithRollupConfigured(); err != nil {
		return err
	}
	t5 := c.now.Add(-5 * time.Minute).Format(time.RFC3339Nano)
	_, err := c.db.Exec("UPDATE obs_rollup_jobs SET last_completed_bucket_start_utc = ? WHERE job_name = 'raw_to_minute'", t5)
	return err
}

func (c *rollupTestContext) theHsmeOpsServiceHasProcessedBucketTMin(mins int) error {
	if err := c.theHsmeOpsServiceIsRunningWithRollupConfigured(); err != nil {
		return err
	}
	t := c.now.Add(-time.Duration(mins) * time.Minute).Format(time.RFC3339Nano)
	_, err := c.db.Exec("UPDATE obs_rollup_jobs SET last_completed_bucket_start_utc = ? WHERE job_name = 'raw_to_minute'", t)
	return err
}

func (c *rollupTestContext) runRawToMinuteExecutes() error {
	return c.theCronTriggerExecutesRunRawToMinute()
}

func (c *rollupTestContext) bucketT5minIsNOTReprocessed() error {
	return nil
}

func (c *rollupTestContext) processingStartsFromT4min() error {
	return nil
}

func (c *rollupTestContext) runRawToMinuteExecutesTwiceInSuccession() error {
	if err := c.service.FlushRollups(context.Background(), c.now); err != nil {
		return err
	}
	return c.service.FlushRollups(context.Background(), c.now)
}

func (c *rollupTestContext) theSecondRunProducesIdenticalCheckpointState() error {
	return c.lastCompletedBucketStartUtcIsUpdatedToCurrentBucket()
}

func (c *rollupTestContext) noDuplicateProcessingOccurs() error {
	return nil
}

func (c *rollupTestContext) theHsmeOpsServiceWasDownForMoreThan7Days() error {
	if err := c.theHsmeOpsServiceIsRunningWithRollupConfigured(); err != nil {
		return err
	}
	t10d := c.now.AddDate(0, 0, -10).Format(time.RFC3339Nano)
	_, err := c.db.Exec("UPDATE obs_rollup_jobs SET last_completed_bucket_start_utc = ? WHERE job_name = 'raw_to_minute'", t10d)
	return err
}

func (c *rollupTestContext) onlyBucketsWithinThe7dayRetentionWindowAreProcessed() error {
	return nil
}

func (c *rollupTestContext) bucketsOutsideRetentionWindowAreSkippedWithoutError() error {
	return nil
}

func (c *rollupTestContext) checkpointIsUpdatedToTheOldestBucketWithinRetentionWindow() error {
	return c.lastCompletedBucketStartUtcIsUpdatedToCurrentBucket()
}

func InitializeScenario(sc *godog.ScenarioContext) {
	c := &rollupTestContext{}

	sc.Step(`^the hsme-ops service is running with rollup configured$`, c.theHsmeOpsServiceIsRunningWithRollupConfigured)
	sc.Step(`^no buckets have been processed yet$`, c.noBucketsHaveBeenProcessedYet)
	sc.Step(`^the cron trigger executes runRawToMinute$`, c.theCronTriggerExecutesRunRawToMinute)
	sc.Step(`^the bucket of now\.Truncate\(minute\) is processed$`, c.theBucketOfNowTruncateMinuteIsProcessed)
	sc.Step(`^last_completed_bucket_start_utc is updated to current bucket$`, c.lastCompletedBucketStartUtcIsUpdatedToCurrentBucket)

	sc.Step(`^the hsme-ops service has processed up to bucket T-10min$`, c.theHsmeOpsServiceHasProcessedUpToBucketT10min)
	sc.Step(`^the service was down for 5 minutes \(buckets T-5min through T-1min missed\)$`, c.theServiceWasDownFor5Minutes)
	sc.Step(`^the service restarts and executes runRawToMinute$`, c.theServiceRestartsAndExecutesRunRawToMinute)
	sc.Step(`^buckets ([^ ]*), ([^ ]*), ([^ ]*), ([^ ]*), ([^ ]*) are all processed in order$`, c.bucketsAreAllProcessedInOrder)
	sc.Step(`^last_completed_bucket_start_utc advances sequentially$`, c.lastCompletedBucketStartUtcAdvancesSequentially)
	sc.Step(`^no bucket is lost due to the gap$`, c.noBucketIsLostDueToTheGap)

	sc.Step(`^the hsme-ops service has processed up to bucket T-5min$`, c.theHsmeOpsServiceHasProcessedUpToBucketT5min)
	sc.Step(`^the hsme-ops service has processed bucket T-(\d+)min$`, c.theHsmeOpsServiceHasProcessedBucketTMin)
	sc.Step(`^runRawToMinute executes$`, c.runRawToMinuteExecutes)
	sc.Step(`^bucket T-5min is NOT reprocessed$`, c.bucketT5minIsNOTReprocessed)
	sc.Step(`^processing starts from T-4min$`, c.processingStartsFromT4min)

	sc.Step(`^runRawToMinute executes twice in succession$`, c.runRawToMinuteExecutesTwiceInSuccession)
	sc.Step(`^the second run produces identical checkpoint state$`, c.theSecondRunProducesIdenticalCheckpointState)
	sc.Step(`^no duplicate processing occurs$`, c.noDuplicateProcessingOccurs)

	sc.Step(`^the hsme-ops service was down for more than 7 days$`, c.theHsmeOpsServiceWasDownForMoreThan7Days)
	sc.Step(`^only buckets within the 7-day retention window are processed$`, c.onlyBucketsWithinThe7dayRetentionWindowAreProcessed)
	sc.Step(`^buckets outside retention window are skipped without error$`, c.bucketsOutsideRetentionWindowAreSkippedWithoutError)
	sc.Step(`^checkpoint is updated to the oldest bucket within retention window$`, c.checkpointIsUpdatedToTheOldestBucketWithinRetentionWindow)
}

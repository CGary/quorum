package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type ObservabilityPolicy struct {
	ID              int64
	PolicyName      string
	ScopeKind       string
	BucketLevel     sql.NullString
	KeepDays        int
	SampleRate      sql.NullFloat64
	SlowThresholdUS sql.NullInt64
	Enabled         bool
}

type RollupJobState struct {
	ID                          int64
	JobName                     string
	SourceScope                 string
	LastCompletedBucketStartUTC sql.NullString
	LastRunStartedAtUTC         sql.NullString
	LastRunFinishedAtUTC        sql.NullString
	LastStatus                  string
	LastError                   sql.NullString
}

func LoadObservabilityPolicies(ctx context.Context, db *sql.DB) ([]ObservabilityPolicy, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, policy_name, scope_kind, bucket_level, keep_days, sample_rate, slow_threshold_us, enabled FROM obs_retention_policies WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query observability policies: %w", err)
	}
	defer rows.Close()
	var policies []ObservabilityPolicy
	for rows.Next() {
		var p ObservabilityPolicy
		var enabled int
		if err := rows.Scan(&p.ID, &p.PolicyName, &p.ScopeKind, &p.BucketLevel, &p.KeepDays, &p.SampleRate, &p.SlowThresholdUS, &enabled); err != nil {
			return nil, fmt.Errorf("scan observability policy: %w", err)
		}
		p.Enabled = enabled == 1
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func LoadRollupJobs(ctx context.Context, db *sql.DB) ([]RollupJobState, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, job_name, source_scope, last_completed_bucket_start_utc, last_run_started_at_utc, last_run_finished_at_utc, last_status, last_error FROM obs_rollup_jobs ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query rollup jobs: %w", err)
	}
	defer rows.Close()
	var jobs []RollupJobState
	for rows.Next() {
		var job RollupJobState
		if err := rows.Scan(&job.ID, &job.JobName, &job.SourceScope, &job.LastCompletedBucketStartUTC, &job.LastRunStartedAtUTC, &job.LastRunFinishedAtUTC, &job.LastStatus, &job.LastError); err != nil {
			return nil, fmt.Errorf("scan rollup job: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func WithImmediateTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

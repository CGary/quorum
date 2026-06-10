package observability

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type MaintenanceService struct {
	db  *sql.DB
	cfg Config
}

func NewMaintenanceService(db *sql.DB, cfg Config) *MaintenanceService {
	return &MaintenanceService{db: db, cfg: cfg}
}

func (m *MaintenanceService) FlushRollups(ctx context.Context, now time.Time) error {
	if err := m.runRawToMinute(ctx, now); err != nil {
		return err
	}
	if err := m.runDerivedRollup(ctx, now, "minute", "hour", "minute_to_hour"); err != nil {
		return err
	}
	return m.runDerivedRollup(ctx, now, "hour", "day", "hour_to_day")
}

func (m *MaintenanceService) RunRetention(ctx context.Context, now time.Time) error {
	return sqlTx(ctx, m.db, func(tx *sql.Tx) error {
		if err := m.updateJobStatus(ctx, tx, "retention_cleanup", "running", "", now, false); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT scope_kind, bucket_level, keep_days FROM obs_retention_policies WHERE enabled = 1 ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		type policy struct {
			scope    string
			bucket   sql.NullString
			keepDays int
		}
		var policies []policy
		for rows.Next() {
			var p policy
			if err := rows.Scan(&p.scope, &p.bucket, &p.keepDays); err != nil {
				return err
			}
			policies = append(policies, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, p := range policies {
			cutoff := now.UTC().AddDate(0, 0, -p.keepDays).Format(time.RFC3339Nano)
			switch p.scope {
			case "events":
				if _, err := tx.ExecContext(ctx, `DELETE FROM obs_events WHERE created_at_utc < ?`, cutoff); err != nil {
					return err
				}
			case "traces":
				if _, err := tx.ExecContext(ctx, `DELETE FROM obs_traces WHERE started_at_utc < ?`, cutoff); err != nil {
					return err
				}
			case "spans":
				if _, err := tx.ExecContext(ctx, `DELETE FROM obs_spans WHERE started_at_utc < ?`, cutoff); err != nil {
					return err
				}
			case "rollups":
				if _, err := tx.ExecContext(ctx, `DELETE FROM obs_metric_rollups WHERE bucket_level = ? AND bucket_start_utc < ?`, p.bucket.String, cutoff); err != nil {
					return err
				}
			}
		}
		return m.updateJobStatus(ctx, tx, "retention_cleanup", "ok", now.UTC().Format(time.RFC3339Nano), now, true)
	})
}

func (m *MaintenanceService) runRawToMinute(ctx context.Context, now time.Time) error {
	return m.runJobWithCatchup(ctx, "raw_to_minute", now, time.Minute, m.cfg.RawRetentionDays, func(ctx context.Context, tx *sql.Tx, bucketStart time.Time) error {
		bucketEnd := bucketStart.Add(time.Minute)
		rows, err := tx.QueryContext(ctx, `SELECT component, operation_name, ifnull(tool_name,''), ifnull(task_type,''), trace_kind, COUNT(*), SUM(CASE WHEN status='ok' THEN 1 ELSE 0 END), SUM(CASE WHEN status!='ok' THEN 1 ELSE 0 END), SUM(CASE WHEN sampled=1 THEN 1 ELSE 0 END), SUM(duration_us), MAX(duration_us), MAX(started_at_utc) FROM obs_traces WHERE started_at_utc >= ? AND started_at_utc < ? GROUP BY component, operation_name, tool_name, task_type, trace_kind`, bucketStart.Format(time.RFC3339Nano), bucketEnd.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var component, operation, toolName, taskType, traceKind string
			var total, success, failure, sampled, durationTotal, durationMax int64
			var lastSource sql.NullString
			if err := rows.Scan(&component, &operation, &toolName, &taskType, &traceKind, &total, &success, &failure, &sampled, &durationTotal, &durationMax, &lastSource); err != nil {
				return err
			}
			durations, err := m.fetchTraceDurations(ctx, tx, bucketStart, bucketEnd, component, operation, toolName, taskType, traceKind)
			if err != nil {
				return err
			}
			p50, p95, p99 := aggregatePercentiles(durations)
			slowCount, err := m.countSlowEvents(ctx, tx, bucketStart, bucketEnd, component, operation, toolName, taskType)
			if err != nil {
				return err
			}
			if err := upsertRollupBucket(ctx, tx, "minute", bucketStart.Format(time.RFC3339Nano), component, operation, toolName, taskType, traceKind, map[string]int64{"total_count": total, "success_count": success, "error_count": failure, "slow_count": slowCount, "sampled_count": sampled, "duration_total_us": durationTotal, "duration_max_us": durationMax, "p50_us": p50, "p95_us": p95, "p99_us": p99, "queue_delay_total_us": 0, "bytes_in_total": 0, "bytes_out_total": 0, "rows_read_total": 0, "rows_written_total": 0}, lastSource.String); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

func (m *MaintenanceService) runDerivedRollup(ctx context.Context, now time.Time, sourceLevel, targetLevel, jobName string) error {
	var bucketSize time.Duration
	var retentionDays int
	switch targetLevel {
	case "hour":
		bucketSize = time.Hour
		retentionDays = m.cfg.MinuteRetentionDays
	case "day":
		bucketSize = 24 * time.Hour
		retentionDays = m.cfg.HourRetentionDays
	default:
		return fmt.Errorf("unsupported target level: %s", targetLevel)
	}

	return m.runJobWithCatchup(ctx, jobName, now, bucketSize, retentionDays, func(ctx context.Context, tx *sql.Tx, bucketStart time.Time) error {
		bucketEnd := bucketStart.Add(bucketSize)
		rows, err := tx.QueryContext(ctx, `SELECT component, operation_name, ifnull(tool_name,''), ifnull(task_type,''), trace_kind, SUM(total_count), SUM(success_count), SUM(error_count), SUM(slow_count), SUM(sampled_count), SUM(duration_total_us), MAX(duration_max_us), MAX(last_source_event_at_utc) FROM obs_metric_rollups WHERE bucket_level = ? AND bucket_start_utc >= ? AND bucket_start_utc < ? GROUP BY component, operation_name, tool_name, task_type, trace_kind`, sourceLevel, bucketStart.Format(time.RFC3339Nano), bucketEnd.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var component, operation, toolName, taskType, traceKind string
			var total, success, failure, slow, sampled, durationTotal, durationMax int64
			var lastSource sql.NullString
			if err := rows.Scan(&component, &operation, &toolName, &taskType, &traceKind, &total, &success, &failure, &slow, &sampled, &durationTotal, &durationMax, &lastSource); err != nil {
				return err
			}
			durations, err := m.fetchRollupDurations(ctx, tx, sourceLevel, bucketStart, bucketEnd, component, operation, toolName, taskType, traceKind)
			if err != nil {
				return err
			}
			p50, p95, p99 := aggregatePercentiles(durations)
			if err := upsertRollupBucket(ctx, tx, targetLevel, bucketStart.Format(time.RFC3339Nano), component, operation, toolName, taskType, traceKind, map[string]int64{"total_count": total, "success_count": success, "error_count": failure, "slow_count": slow, "sampled_count": sampled, "duration_total_us": durationTotal, "duration_max_us": durationMax, "p50_us": p50, "p95_us": p95, "p99_us": p99, "queue_delay_total_us": 0, "bytes_in_total": 0, "bytes_out_total": 0, "rows_read_total": 0, "rows_written_total": 0}, lastSource.String); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

func (m *MaintenanceService) runJobWithCatchup(ctx context.Context, jobName string, now time.Time, bucketSize time.Duration, retentionDays int, processFn func(context.Context, *sql.Tx, time.Time) error) error {
	lastCheckpoint, err := m.getLastCompletedBucket(ctx, jobName)
	if err != nil {
		return err
	}

	currentBucket := now.UTC().Truncate(bucketSize)

	// If no checkpoint, start from current bucket (or one before to be safe)
	if lastCheckpoint.IsZero() {
		lastCheckpoint = currentBucket.Add(-bucketSize)
	}

	// Retention cutoff guard
	retentionCutoff := now.UTC().AddDate(0, 0, -retentionDays)
	if lastCheckpoint.Before(retentionCutoff) {
		lastCheckpoint = retentionCutoff.Truncate(bucketSize)
	}

	// Catch-up loop
	for ts := lastCheckpoint.Add(bucketSize); !ts.After(currentBucket); ts = ts.Add(bucketSize) {
		err := sqlTx(ctx, m.db, func(tx *sql.Tx) error {
			if err := m.updateJobStatus(ctx, tx, jobName, "running", "", now, false); err != nil {
				return err
			}
			if err := processFn(ctx, tx, ts); err != nil {
				return err
			}
			return m.updateJobStatus(ctx, tx, jobName, "ok", ts.Format(time.RFC3339Nano), now, true)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *MaintenanceService) fetchTraceDurations(ctx context.Context, tx *sql.Tx, start, end time.Time, component, operation, toolName, taskType, traceKind string) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT duration_us FROM obs_traces WHERE started_at_utc >= ? AND started_at_utc < ? AND component = ? AND operation_name = ? AND ifnull(tool_name,'') = ? AND ifnull(task_type,'') = ? AND trace_kind = ?`, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano), component, operation, toolName, taskType, traceKind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var d int64
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (m *MaintenanceService) fetchRollupDurations(ctx context.Context, tx *sql.Tx, level string, start, end time.Time, component, operation, toolName, taskType, traceKind string) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT ifnull(p95_us,0) FROM obs_metric_rollups WHERE bucket_level = ? AND bucket_start_utc >= ? AND bucket_start_utc < ? AND component = ? AND operation_name = ? AND ifnull(tool_name,'') = ? AND ifnull(task_type,'') = ? AND trace_kind = ?`, level, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano), component, operation, toolName, taskType, traceKind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var d int64
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (m *MaintenanceService) countSlowEvents(ctx context.Context, tx *sql.Tx, start, end time.Time, component, operation, toolName, taskType string) (int64, error) {
	var count int64
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM obs_events WHERE event_kind = 'slow_operation' AND created_at_utc >= ? AND created_at_utc < ? AND component = ? AND operation_name = ? AND ifnull(tool_name,'') = ? AND ifnull(task_type,'') = ?`, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano), component, operation, toolName, taskType).Scan(&count)
	return count, err
}

func (m *MaintenanceService) updateJobStatus(ctx context.Context, tx *sql.Tx, jobName, status, bucket string, now time.Time, finished bool) error {
	finishedVal := any(nil)
	completedBucket := any(nil)
	if finished {
		finishedVal = now.UTC().Format(time.RFC3339Nano)
		if bucket != "" {
			completedBucket = bucket
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE obs_rollup_jobs SET last_run_started_at_utc = COALESCE(last_run_started_at_utc, ?), last_run_finished_at_utc = COALESCE(?, last_run_finished_at_utc), last_completed_bucket_start_utc = COALESCE(?, last_completed_bucket_start_utc), last_status = ?, last_error = NULL, updated_at_utc = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE job_name = ?`, now.UTC().Format(time.RFC3339Nano), finishedVal, completedBucket, status, jobName)
	return err
}

func sqlTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
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

func (m *MaintenanceService) getLastCompletedBucket(ctx context.Context, jobName string) (time.Time, error) {
	var lastUTC sql.NullString
	err := m.db.QueryRowContext(ctx, `SELECT last_completed_bucket_start_utc FROM obs_rollup_jobs WHERE job_name = ?`, jobName).Scan(&lastUTC)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	if !lastUTC.Valid || lastUTC.String == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, lastUTC.String)
}

package admin

import (
	"context"
	"database/sql"
	"fmt"
)

// RetryFailedTasks resets failed and exhausted tasks to pending status so they can be re-processed.
func RetryFailedTasks(ctx context.Context, db *sql.DB) (int64, error) {
        res, err := db.ExecContext(ctx, "UPDATE async_tasks SET status = 'pending', attempt_count = 0 WHERE status = 'failed' OR attempt_count >= 5")
        if err != nil {
                return 0, fmt.Errorf("failed to retry tasks: %w", err)
        }
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return affected, nil
}

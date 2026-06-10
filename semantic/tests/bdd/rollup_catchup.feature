Feature: Rollup catch-up for missed buckets

  Scenario: Rollup processes current bucket normally
    Given the hsme-ops service is running with rollup configured
    And no buckets have been processed yet
    When the cron trigger executes runRawToMinute
    Then the bucket of now.Truncate(minute) is processed
    And last_completed_bucket_start_utc is updated to current bucket

  Scenario: Rollup catches up 5 missed buckets after restart
    Given the hsme-ops service has processed up to bucket T-10min
    And the service was down for 5 minutes (buckets T-5min through T-1min missed)
    When the service restarts and executes runRawToMinute
    Then buckets T-5min, T-4min, T-3min, T-2min, T-1min are all processed in order
    And last_completed_bucket_start_utc advances sequentially
    And no bucket is lost due to the gap

  Scenario: Rollup does not reprocess already-completed buckets
    Given the hsme-ops service has processed up to bucket T-5min
    When runRawToMinute executes
    Then bucket T-5min is NOT reprocessed
    And processing starts from T-4min

  Scenario: Idempotency — running twice produces same result
    Given the hsme-ops service has processed bucket T-5min
    When runRawToMinute executes twice in succession
    Then the second run produces identical checkpoint state
    And no duplicate processing occurs

  Scenario: Gap larger than retention window is handled gracefully
    Given the hsme-ops service was down for more than 7 days
    When the service restarts and executes runRawToMinute
    Then only buckets within the 7-day retention window are processed
    And buckets outside retention window are skipped without error
    And checkpoint is updated to the oldest bucket within retention window

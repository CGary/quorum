Feature: Observability environment variable

  Scenario: HSME_OBS_LEVEL produces trace data (correct variable)
    Given the hsme process is started with env HSME_OBS_LEVEL=trace
    And the user performs store and search operations
    Then obs_traces has > 0 rows
    And obs_spans has > 0 rows
    And obs_events has > 0 rows

  Scenario: OBS_LEVEL (wrong) produces no trace data
    Given the hsme process is started with env OBS_LEVEL=trace
    And the user performs store and search operations
    Then obs_traces has 0 rows
    And obs_spans has 0 rows
    And obs_events has 0 rows

  Scenario: README correctly documents HSME_OBS_LEVEL
    Given a user reads the README observability section
    When they configure HSME_OBS_LEVEL as documented
    Then observability produces data as expected

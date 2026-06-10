Feature: search_fuzzy with project filter uses vector search

  Scenario: project filter returns vector candidates (post-fix behavior)
    Given a test database with vec0 support and project "acme" with embedded chunks
    When the user calls search_fuzzy with query="semantic" project="acme" k=10
    Then the search completes without vec0 "LIMIT" error
    And the result coverage is "complete" (vector candidates present)
    And the result includes embeddings from the project "acme"

  Scenario: search without project filter still works (regression check)
    Given a test database with vec0 support and project "acme" with embedded chunks
    When the user calls search_fuzzy with query="semantic" k=10
    Then the search returns mixed vector + lexical results
    And RRF fusion is applied correctly

  Scenario: project filter falls back to lexical when vec0 unavailable
    Given a test database WITHOUT vec0 support
    When the user calls search_fuzzy with query="semantic" project="acme" k=10
    Then the search completes without error
    And the result coverage is "partial" (lexical only, graceful degradation)

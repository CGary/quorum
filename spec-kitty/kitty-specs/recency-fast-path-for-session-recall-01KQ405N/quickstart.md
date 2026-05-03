# Quickstart: Recency Fast Path for Session Recall

**Mission**: `recency-fast-path-for-session-recall-01KQ405N`

This quickstart describes how to validate the new recency tool once implementation lands.

## 1. Build and test

```bash
go test -tags "sqlite_fts5 sqlite_vec" ./tests/modules/...
go test -tags "sqlite_fts5 sqlite_vec" ./...
```

Expected:
- existing tests compile again after the mechanical signature fixes
- the new recall tests pass

## 2. Start HSME normally

```bash
just build
./hsme
```

Or use the already established local workflow for the MCP server.

## 3. Exercise the new tool conceptually

Call the new MCP tool with these cases:

### Latest session for one project

```json
{
  "project": "aibbe",
  "limit": 1
}
```

Expected:
- one result
- newest `session_summary` for `aibbe`
- no embedder latency

### Recent sessions across all projects

```json
{
  "limit": 5
}
```

Expected:
- up to five results
- global reverse-chronological order by `created_at DESC, id DESC`

### Nonexistent project

```json
{
  "project": "nonexistent",
  "limit": 5
}
```

Expected:
- empty result list
- no error

### Limit cap

```json
{
  "limit": 500
}
```

Expected:
- at most 50 results
- no validation error

## 4. Validate protocol guidance

After implementation, confirm `CLAUDE.md` HSME protocol section says to use `recall_recent_session` before `search_fuzzy` for recency-style questions such as:
- "what did we do last session?"
- "latest work on aibbe"
- "recent work this week"

## 5. Regression check

Run or spot-check existing HSME tools to confirm no behavioral regression:
- `search_fuzzy`
- `search_exact`
- `explore_knowledge_graph`

The mission is successful when the new tool is fast, exact for chronological intent, and existing search behavior remains unchanged.

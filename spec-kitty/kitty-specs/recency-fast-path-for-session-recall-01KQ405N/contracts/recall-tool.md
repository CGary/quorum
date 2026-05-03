# Contract: MCP `recall_recent_session`

**Mission**: `recency-fast-path-for-session-recall-01KQ405N`

`recall_recent_session` is a read-only MCP tool that returns the most recent active `session_summary` memories in reverse chronological order, optionally scoped to a single project.

## Input schema

```jsonc
{
  "name": "recall_recent_session",
  "description": "Return the most recent active session_summary memories by created_at descending",
  "inputSchema": {
    "type": "object",
    "properties": {
      "project": {
        "type": "string",
        "description": "Optional. Restrict results to memories.project = this value."
      },
      "limit": {
        "type": "integer",
        "description": "Optional. Number of results requested.",
        "default": 5,
        "minimum": 1,
        "maximum": 50
      }
    },
    "required": []
  }
}
```

## Behavior

- `project` omitted or empty string: search across all projects.
- `project` provided: restrict to `memories.project = <project>`.
- `limit` omitted: defaults to `5`.
- `limit > 50`: silently capped to `50` server-side.
- `limit <= 0`: treated as empty result or clamped defensively before query execution.
- The tool MUST NOT invoke the embedder or any Ollama-dependent path.

## Canonical query

```sql
SELECT id, COALESCE(project, ''), created_at, raw_content
FROM memories
WHERE source_type = 'session_summary'
  AND status = 'active'
  AND superseded_by IS NULL
  -- optional: AND project = ?
ORDER BY created_at DESC, id DESC
LIMIT ?
```

## Output shape

The tool returns the same MCP text-content wrapper style used by existing HSME tools. Each result item should include enough information for the caller to identify and read the summary without additional DB lookups.

Suggested per-result fields:
- `id`
- `project`
- `created_at`
- `raw_content`

An empty match returns an empty result list, not an error.

## Error behavior

- SQL/query failure: standard internal MCP error mapping already used by other HSME tools.
- No dedicated validation error for `limit > 50`; the cap is silent by design.
- Missing/empty `project` is never an error.

## Compatibility guarantees

- Additive tool only; existing tool names and schemas remain unchanged.
- No schema change required for callers of `search_fuzzy`, `search_exact`, or `explore_knowledge_graph`.

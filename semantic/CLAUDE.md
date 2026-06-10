# HSME Protocol

When interacting with the Hybrid Semantic Memory Engine (HSME):

- **Recency Queries**: For questions whose answer depends on chronological order rather than semantic relevance (e.g., "what did we do last session?", "last session", "recent work", "what did we do last?", "last time"), ALWAYS call the `recall_recent_session` MCP tool FIRST before falling back to `search_fuzzy`.
- `recall_recent_session` returns the most recent `session_summary` memories, ordered by real time, bypassing semantic relevance.
- **Semantic Queries**: Use `search_fuzzy` or `search_exact` for topic exploration or specific technical facts.
- **Tracing**: Use `explore_knowledge_graph` to trace entity dependencies.

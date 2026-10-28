<!-- codebase-memory-mcp tools -->
## MCP Tools: codebase-memory-mcp

**This project has a knowledge graph. Start with the codebase-memory-mcp
tools to narrow scope, then read the source.** The graph is cheaper than scanning files and
gives you structural context (callers, dependents, test coverage) that file search cannot.

Project name: `home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm`. Pass it as `project`
on every call. `list_projects` also shows stale duplicate indexes: do not use those.

### When to use graph tools FIRST

- **Exploring code**: `search_graph` or `query_graph` instead of Grep
- **Understanding impact**: `trace_path` instead of manually tracing imports
- **Code review**: `detect_changes` + `get_code_snippet` instead of reading entire files
- **Finding relationships**: `trace_path` for callers/callees, `query_graph` for multi-hop patterns
- **Architecture questions**: `get_architecture` (aspects: `clusters`, `layers`, `hotspots`)

### Verify in the source

- Narrow scope with the graph, then read the source. Do not change code from graph output alone.
- For any non-trivial change, read the implementation and the relevant tests before concluding.
- Verify the exact source when touching behavior, database logic, migrations, retries, fallbacks,
  recovery, or compatibility code.
- When the graph and the source disagree, the source wins. The graph may be stale or may not
  model that relationship.
- An empty graph result can mean "not indexed" or "not statically visible", not "does not exist".
- Run `check_index_coverage` on every cited path, and on scopes behind negative or exhaustive claims.
- NOT indexed, read/grep directly: `deploy/`, `.claude/`, `node_modules/`, `web/dashboard/dist`.

### Key Tools

| Tool | Use when |
| ------ | ---------- |
| `search_graph` | Finding functions/classes by name or keyword |
| `query_graph` | Multi-hop patterns over the graph (Cypher-like) |
| `trace_path` | Tracing callers, callees and blast radius of a change |
| `get_code_snippet` | Need exact source for a symbol — token-efficient |
| `detect_changes` | Reviewing code changes — maps a diff to affected symbols |
| `get_architecture` | Understanding high-level codebase structure |
| `search_code` | Literal or non-code text search |
| `check_index_coverage` | Before trusting a path or a negative result |
| `index_status` | Project health, node/edge counts, parse gaps |

### Workflow

1. Call `list_projects` once, then use the project name above.
2. The index auto-refreshes in the background; call `index_repository` only if the repo is not
   indexed or after a large external update.
3. Use `detect_changes` for code review.
4. Use `trace_path` to understand impact.
5. Use `search_graph` for tests of a symbol to check coverage.
<!-- /codebase-memory-mcp tools -->

---
name: plane-mcp
description: "Plane project management — work items, states, projects, modules, cycles, comments. Use this skill when the user asks about Plane work items, moving an issue to a state (In Progress, In Review, Done), adding a completion comment, listing projects, or looking up a work item by short-id sequence. Provides 13 MCP tools (server mode) plus a CLI (shell/scripts). Triggers: 'plane', 'plane work item', 'move to in review', 'mark as done', 'add comment to issue', 'list projects', 'work item', 'issue tracker', 'create work item', 'update work item', 'plane state', 'plane project'."
---

# plane-mcp

A fast, single-binary wrapper around the Plane REST API. Replaces the
deprecated Node.js [@makeplane/plane-mcp-server](https://github.com/makeplane/plane-mcp-server)
with a Go binary that starts in <50ms (vs ~500ms for `npx` subprocess).

Runs in two modes from the same binary:

1. **MCP server** (default) — exposes 13 focused tools over stdio for
   AI agents (Claude Code, opencode, Cursor, etc.)
2. **CLI** — single-shot commands for humans and shell scripts

Both share the same internal client, ops layer, and config.

## When to use this skill

Reach for `plane-mcp` when the user wants to:

- **Read** a work item: "what's the status of `<PROJECT>-5`?", "show me `<PROJECT>-12`"
- **Move** a work item through its state machine: "mark as In Review", "move to Done"
- **Comment** on a work item: "add a comment saying MR opened"
- **Update** fields: "change the priority to urgent", "set the target date to 2026-07-01"
- **List** projects or work items: "what projects exist?", "show me all open items"
- **Discover** states: "what states does this project have?"
- **Health check**: "is the Plane connection working?"

Don't use it for analytics, time tracking, notifications, or OAuth-based
Plane Apps — those aren't on the read or write path of this wrapper.

## MCP tool reference (use from agents)

When running through an MCP client, prefer the MCP tool name. The CLI
is for humans and scripts.

| Tool | Args | Purpose |
|---|---|---|
| `list_projects` | (none) | List all projects in the workspace |
| `get_project_by_identifier` | `identifier` (e.g. `"PROJ"`) | Look up a project by its short ID |
| `get_work_item_by_sequence` | `identifier`, `sequence_id` (e.g. `"PROJ"`, `"5"`) | Look up `PROJ-5` |
| `get_work_item_by_id` | `work_item_id` (UUID) | Look up by UUID |
| `list_work_items` | `project_id`, optional `state`, `assignee` | List items in a project |
| `create_work_item` | `project_id`, `name`, `description_html`, optional fields | Create a new work item |
| `update_work_item` | `work_item_id`, partial fields | Update fields (title, priority, state, etc.) |
| `update_work_item_state` | `work_item_id`, `name` (state name) | Move to a state by name (e.g. `"In Review"`) |
| `add_work_item_comment` | `work_item_id`, `comment_html` | Add an HTML comment |
| `list_states` | `project_id` | List states in a project |
| `list_modules` | `project_id` | List modules |
| `list_cycles` | `project_id` | List cycles |
| `health` | (none) | Health check + cache stats |

## CLI reference (use from shell/scripts)

```bash
# Authentication via env (recommended)
export PLANE_API_KEY=<your-api-key>
export PLANE_WORKSPACE_SLUG=<your-workspace-slug>
export PLANE_API_HOST_URL=https://api.plane.so  # or your self-hosted URL

# Read
plane-mcp projects
plane-mcp project <PROJECT_ID>
plane-mcp items <PROJECT_ID>
plane-mcp item <PROJECT_ID> <SEQ>
plane-mcp states <PROJECT_ID>
plane-mcp health

# Write
plane-mcp state <PROJECT_ID> <SEQ> "In Review"
plane-mcp comment <PROJECT_ID> <SEQ> "MR opened: https://..."
plane-mcp update <PROJECT_ID> <SEQ> -priority high -title "New title"
plane-mcp update <PROJECT_ID> <SEQ> -state "Done" -target-date 2026-07-15

# Machine-readable (for jq, scripts)
plane-mcp -format=json item <PROJECT_ID> <SEQ>
plane-mcp -format=json items <PROJECT_ID> | jq '.[].name'
```

### Subcommands

| Command | Args | Purpose |
|---|---|---|
| `mcp` | (none) | Start MCP stdio server (default if no command) |
| `projects` | (none) | List all projects |
| `project` | `<PROJECT_ID>` | Show project details |
| `items` | `<PROJECT_ID>` | List work items in a project |
| `item` | `<PROJECT_ID> <SEQ>` | Show work item details |
| `states` | `<PROJECT_ID>` | List states in a project |
| `state` | `<PROJECT_ID> <SEQ> <NAME>` | Move to state (looks up state ID by name) |
| `comment` | `<PROJECT_ID> <SEQ> <TEXT>` | Add an HTML comment |
| `update` | `<PROJECT_ID> <SEQ> [flags]` | Update fields |
| `health` | (none) | Health check |

### `update` flags

| Flag | Description |
|---|---|
| `-title` | New title |
| `-state` | New state (looked up by name) |
| `-priority` | `urgent`, `high`, `medium`, `low`, or `none` |
| `-description` | New description (HTML) |
| `-target-date` | Target date (`YYYY-MM-DD`) |
| `-start-date` | Start date (`YYYY-MM-DD`) |
| `-assignees` | Comma-separated assignee UUIDs |

Only flags you set are sent. Run `plane-mcp update -h` for the most
current flag list.

### Global flags

| Flag | Default | Env var |
|---|---|---|
| `-api-key` | — | `PLANE_API_KEY` |
| `-workspace` | — | `PLANE_WORKSPACE_SLUG` |
| `-base-url` | `https://api.plane.so` | `PLANE_API_HOST_URL` |
| `-format` | `text` | — (`text` or `json`) |
| `-version` | — | — (print version, exit) |

## Common workflows

### 1. Move a work item to In Review (most common)

**MCP:**
```
update_work_item_state(work_item_id=<uuid>, name="In Review")
```

**CLI:**
```bash
plane-mcp state <PROJECT_ID> 5 "In Review"
```

If you don't have the UUID, look it up first:
```
get_work_item_by_sequence(identifier="<PROJECT_ID>", sequence_id="5")
```

### 2. Add a completion comment

**MCP:**
```
add_work_item_comment(work_item_id=<uuid>, comment_html="<p>MR opened: ...</p>")
```

**CLI:**
```bash
plane-mcp comment <PROJECT_ID> 5 "MR opened: https://..."
```

Plain text is auto-wrapped in `<p>` tags.

### 3. Change multiple fields at once

**MCP** — use `update_work_item` with partial fields (all optional):
```
update_work_item(
  work_item_id=<uuid>,
  name="Revised title",
  priority="urgent",
  target_date="2026-07-01"
)
```

**CLI** — flags are independent, only set ones are sent:
```bash
plane-mcp update <PROJECT_ID> 5 \
  -title "Revised title" \
  -priority urgent \
  -target-date 2026-07-01
```

### 4. Discover state names before transitioning

State names vary per project. Always list first if unsure:

**MCP:** `list_states(project_id=<uuid>)` → returns name/group/color
**CLI:** `plane-mcp states <PROJECT_ID>`

The `update_work_item_state` tool / `state` CLI command also does
this lookup internally, so you usually don't need the discovery step.
But it helps when explaining to the user what their options are.

### 5. Find all items in a given state

**MCP** — `list_work_items` with state filter:
```
list_work_items(project_id=<uuid>, state=<state_id>)
```

**CLI:**
```bash
plane-mcp -format=json items <PROJECT_ID> | jq '[.[] | select(.state_detail.name=="In Review")] | length'
```

## Configuration

| Env var | Flag | Default | Required |
|---|---|---|---|
| `PLANE_API_KEY` | `-api-key` | — | yes |
| `PLANE_WORKSPACE_SLUG` | `-workspace` | — | yes |
| `PLANE_API_HOST_URL` | `-base-url` | `https://api.plane.so` | no (use for self-hosted) |

Generate an API key at **Plane → Profile Settings → API Tokens**.

## Wiring as an MCP server

For opencode, Claude Code, Cursor, or any stdio-based MCP client,
add the binary to your MCP config:

```json
{
  "mcpServers": {
    "plane": {
      "command": "plane-mcp",
      "args": [],
      "env": {
        "PLANE_API_KEY": "<your-api-key>",
        "PLANE_WORKSPACE_SLUG": "<your-workspace-slug>",
        "PLANE_API_HOST_URL": "<your-base-url-or-empty>"
      }
    }
  }
}
```

Once the client restarts, the 13 tools above become available to the
model. No Python/Node.js dependency — the binary is self-contained.

## Build & install

### From source

```bash
git clone https://github.com/bizbox-asia/plane-mcp
cd plane-mcp
make build              # current OS/arch → ./bin/plane-mcp
make build-all          # 9 platforms (linux/darwin/windows/freebsd)
make release            # all platforms + SHA-256 checksums
make test               # unit tests
sudo cp bin/plane-mcp /usr/local/bin/
```

### Cross-platform

| OS | Architectures | Binary |
|---|---|---|
| `darwin` | amd64, arm64 | `plane-mcp` |
| `linux` | 386, amd64, arm64 | `plane-mcp` |
| `windows` | 386, amd64 | `plane-mcp.exe` |
| `freebsd` | amd64, arm64 | `plane-mcp` |

All binaries are statically linked (`CGO_ENABLED=0`), stripped, and
reproducible. Sizes are ~7-8 MB each.

### Verify a release

```bash
shasum -a 256 -c checksums.txt
./plane-mcp -version
```

## Tips and gotchas

- **State transitions are by name, not ID.** "In Review" works, but
  the internal UUID doesn't. The `update_work_item_state` tool looks
  up the ID for you.
- **Updates are partial.** Sending `update_work_item` with only
  `priority` set will leave everything else alone. The PATCH semantics
  are: only fields you provide are changed.
- **No DELETE / no archive.** This wrapper intentionally doesn't
  expose destructive operations. Use the Plane UI for those.
- **Cache TTL is 5 min** for project lookups. If you create a new
  project via the UI, wait 5 min (or restart the MCP server) before
  it's visible.
- **Rate limits:** Plane returns 429 with `X-RateLimit-Reset`. The
  client surfaces the error but doesn't auto-retry — escalate to the
  user.
- **Self-hosted Plane** needs `-base-url` (or `PLANE_API_HOST_URL`)
  set to your instance, e.g. `https://plane.example.com`.
- **Version metadata** is in the binary — `plane-mcp -version` shows
  `version commit buildDate`. Useful when debugging.

## Project identifiers (the short ID)

Work items in Plane are addressed two ways:

1. **By short ID** — `PROJ-5` (human-friendly, what the UI shows)
2. **By UUID** — `004c7d32-f895-48f5-817e-86f9b116b17e` (what the
   API uses internally)

`get_work_item_by_sequence` (MCP) and `item <PROJECT> <SEQ>` (CLI)
accept the short form. All write tools require the UUID — look it up
first via `get_work_item_by_sequence` or by listing items.

The `PROJ` prefix is whatever you set as the project's `identifier`
in Plane. Every project has a unique 2-6 character prefix.

## Differences from the official Node.js server

| Aspect | Node.js (deprecated) | This wrapper |
|---|---|---|
| Startup | ~500ms (`npx` subprocess) | ~30ms (compiled binary) |
| Connection reuse | New TCP per call | Single HTTP client |
| Tool surface | 100+ tools (Pydantic SDK) | 13 focused tools |
| Auth | API key only | API key only |
| Self-hosted | Yes (env var) | Yes (env var) |
| OAuth | No | No — use `mcp.plane.so` for OAuth |
| Cache layer | None | Project lookup cache (5m TTL) |

## Architecture (for contributors)

```
cmd/plane-mcp/         # main entry point + subcommand router
  main.go              #   global flags, mcp|cli dispatch
  cli.go               #   subcommand implementations
  output.go            #   text/JSON formatters
cmd/plane-mcp-smoke/   # integration test against real Plane API
internal/
  client/              # thin HTTP client
  models/              # data types
  ops/                 # high-level operations
  server/              # MCP server (13 tool handlers)
Makefile               # build / test / release
```

The same `cmd/plane-mcp/main.go` detects whether `os.Args` contains a
known subcommand (`projects`, `item`, `state`, etc.) and routes to
`cli.go`. With no subcommand (or `mcp`), it starts the MCP server.
This means a single binary serves both AI agents and humans.

## Adding a new tool

Each Plane operation gets three layers, in this order:

1. **Ops method** — `internal/ops/ops.go`. Thin wrapper around the
   client, adds caching and convenience. One test in
   `internal/ops/ops_test.go` using `httptest.Server`.
2. **MCP tool handler** — `internal/server/server.go`. Glue between
   MCP tool schema and the ops method. Optional; only needed if the
   tool is exposed to agents.
3. **CLI subcommand** — `cmd/plane-mcp/cli.go`. Optional; only needed
   for human/script use. Follow the existing pattern: a `cmdX`
   function that parses its own flags, calls the ops method, and
   prints via the `writer`. Wire it up in `runCLI`.

Update this file whenever the tool surface or CLI changes.

## License

MIT (or your chosen license — update as needed).

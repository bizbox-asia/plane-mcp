# Plane MCP — Thin Go-based MCP server for Plane

A fast, single-binary MCP server for the Plane REST API. Replaces the
deprecated Node.js [@makeplane/plane-mcp-server](https://github.com/makeplane/plane-mcp-server).

## Why?

The official Node.js MCP server has two problems for our use case:

| Problem | Node.js (deprecated) | This wrapper |
|---|---|---|
| Startup time per call | ~500ms (`npx` subprocess) | ~30ms (compiled binary) |
| Connection reuse | New TCP per call | Single HTTP client |
| Tool surface | 100+ tools (Pydantic SDK) | 13 focused tools |
| API base | `PLANE_API_HOST_URL` (confusing) | `PLANE_API_HOST_URL` (same) |
| Cache layer | None | Project lookup cache (5m TTL) |

## Build

The project ships with a Makefile that handles both single-platform
builds and cross-compilation. Plain `go build` works too, but the
Makefile injects version metadata via LDFLAGS.

```bash
# Build for the current OS/arch
make build

# Cross-compile a single target
make dist/linux/amd64/plane-mcp
make dist/darwin/arm64/plane-mcp
make dist/windows/amd64/plane-mcp

# Build all 9 supported platforms
make build-all

# Full release: all platforms + SHA-256 checksums
make release
```

Supported targets (output directory: `dist/<os>/<arch>/<binary>`):

| OS | Architectures | Binary |
|---|---|---|
| `darwin` | amd64, arm64 | `plane-mcp` |
| `linux` | 386, amd64, arm64 | `plane-mcp` |
| `windows` | 386, amd64 | `plane-mcp.exe` |
| `freebsd` | amd64, arm64 | `plane-mcp` |

All binaries are statically linked (`CGO_ENABLED=0`), stripped
(`-s -w`), and reproducible (`-trimpath`). Sizes are ~7-8 MB each.

To add a new target, append `OS/ARCH` to the `PLATFORMS` list in the
Makefile. For Windows-style extensions, append `OS/ARCH/.exe`.

## Release to GitHub

The `scripts/release.sh` script wraps `make release` and publishes
the artifacts to a GitHub release via the [`gh` CLI](https://cli.github.com/).

### One-time setup

```bash
# Install the gh CLI
brew install gh        # macOS
# or: https://cli.github.com/manual/installation

# Authenticate
gh auth login
```

### Release workflow

```bash
# 1. Make sure your changes are committed and pushed
git status              # should be clean
git push origin main

# 2. Dry-run first to verify what will be uploaded
make github-release-dry-run VERSION=v1.2.3

# 3. Publish for real
make github-release VERSION=v1.2.3

# Or call the script directly for more control
./scripts/release.sh v1.2.3 --notes-file CHANGELOG.md
./scripts/release.sh v1.2.3-rc1 --prerelease
./scripts/release.sh v1.2.3 --draft                    # save as draft
./scripts/release.sh v1.2.3 --repo your-org/plane-mcp  # cross-repo
```

The script:
1. Validates inputs (semver, `gh` auth, clean tree, no existing release)
2. Runs `make release` to build all 9 platforms + SHA-256 checksums
3. Creates a GitHub release with the version tag
4. Uploads all binaries + `checksums.txt` as release assets
5. Prints the release URL

### Flags

| Flag | Purpose |
|---|---|
| `--repo OWNER/NAME` | Publish to a specific repo (default: current repo) |
| `--prerelease` | Mark the release as a pre-release |
| `--draft` | Create as a draft (not visible to users until published) |
| `--dry-run` | Build but don't upload |
| `--skip-build` | Use existing `dist/release-<version>/` (no rebuild) |
| `--notes-file PATH` | Release notes from a file (default: auto-generated) |
| `--no-color` | Disable colored output |

### Pre-flight checks

The script refuses to run if:
- `gh` is not installed or not authenticated
- The working tree has uncommitted changes
- A GitHub release with that version already exists
- A local git tag with that version already exists
- The version is not valid semver

### Verifying a release

```bash
# Download and verify checksums
gh release download v1.2.3
shasum -a 256 -c checksums.txt

# Or verify in one step
gh release verify v1.2.3
```

## Run (standalone)

```bash
PLANE_API_KEY=plane_api_xxx \
PLANE_WORKSPACE_SLUG=erp \
PLANE_API_HOST_URL=https://replace-plane.example.com \
./bin/plane-mcp
```

The server speaks MCP over stdio. Pipe JSON-RPC requests in, get
JSON-RPC responses out.

## CLI mode (humans and scripts)

The same binary doubles as a one-shot CLI. With no subcommand it starts
the MCP server; with a subcommand it runs that command and exits.

```bash
# Authentication via env (same as MCP mode)
export PLANE_API_KEY=plane_api_xxx
export PLANE_WORKSPACE_SLUG=erp
export PLANE_API_HOST_URL=https://plane.vn.helium.vn

# Read
./bin/plane-mcp projects                  # list all projects
./bin/plane-mcp project SAGA              # show project details
./bin/plane-mcp items SAGA                # list work items
./bin/plane-mcp item SAGA 5               # show SAGA-5 details
./bin/plane-mcp states SAGA               # list states
./bin/plane-mcp health                    # health check

# Write
./bin/plane-mcp state SAGA 5 "In Review"
./bin/plane-mcp comment SAGA 5 "MR opened"
./bin/plane-mcp update SAGA 5 -priority high
./bin/plane-mcp update SAGA 5 -title "New title" -state "Done"

# Machine-readable output (for jq, scripts)
./bin/plane-mcp -format=json item SAGA 5
./bin/plane-mcp -format=json items SAGA | jq '.[].sequence_id'
```

### Subcommand reference

| Command | Args | Purpose |
|---|---|---|
| `mcp` | (none) | Start MCP stdio server (default) |
| `projects` | (none) | List all projects |
| `project` | `<ID>` | Show project details |
| `items` | `<PROJECT>` | List work items in a project |
| `item` | `<PROJECT> <SEQ>` | Show work item details |
| `states` | `<PROJECT>` | List states in a project |
| `state` | `<PROJECT> <SEQ> <NAME>` | Move to state (looks up state ID) |
| `comment` | `<PROJECT> <SEQ> <TEXT>` | Add an HTML comment |
| `update` | `<PROJECT> <SEQ> [flags]` | Update fields |
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

Only flags you set are sent. Run `./bin/plane-mcp update -h` for the
most current flag list.

### Global flags

| Flag | Default | Env var |
|---|---|---|
| `-api-key` | — | `PLANE_API_KEY` |
| `-workspace` | — | `PLANE_WORKSPACE_SLUG` |
| `-base-url` | `https://api.plane.so` | `PLANE_API_HOST_URL` |
| `-format` | `text` | — (`text` or `json`) |
| `-version` | — | — (print version, exit) |

## Configure in opencode

Update `.opencode/opencode.json`:

```json
{
  "mcp": {
    "plane": {
      "type": "local",
      "command": ["./tools/plane-mcp/bin/plane-mcp"],
      "enabled": true,
      "env": {
        "PLANE_API_KEY": "${PLANE_API_KEY}",
        "PLANE_WORKSPACE_SLUG": "${PLANE_WORKSPACE_SLUG}",
        "PLANE_API_HOST_URL": "${PLANE_API_HOST_URL}"
      }
    }
  }
}
```

## Tools exposed (16)

| Tool | Description |
|---|---|
| `list_projects` | List all projects in the workspace |
| `get_project_by_identifier` | Look up a project by its short ID (e.g. "SAGA") |
| `get_work_item_by_sequence` | Look up a work item by SAGA-N style identifier |
| `get_work_item_by_id` | Look up a work item by UUID |
| `list_work_items` | List work items in a project (cursor pagination) |
| `create_work_item` | Create a new work item |
| `update_work_item` | Partial update of a work item |
| `update_work_item_state` | Move work item to a new state by name (looks up state ID) |
| `add_work_item_comment` | Add an HTML comment to a work item |
| `list_states` | List states in a project |
| `list_modules` | List modules in a project |
| `list_cycles` | List cycles in a project |
| `create_project` | Create a new project in the workspace |
| `create_module` | Create a new module in a project |
| `create_cycle` | Create a new cycle in a project |
| `health` | Health check (workspace, cache stats, timestamp) |

## Architecture

```
cmd/plane-mcp/         # main entry point + subcommand router
  main.go              #   global flags, mcp|cli dispatch
  cli.go               #   subcommand implementations
  output.go            #   text/JSON formatters
internal/
  client/              # thin HTTP client for Plane REST API
  models/              # data structures
  ops/                 # high-level operations (GetProjectByIdentifier, etc.)
  server/              # MCP server with 16 tool handlers
cmd/plane-mcp-smoke/   # integration test against real Plane API
```

The same `cmd/plane-mcp/main.go` detects whether `os.Args` contains a
known subcommand (`projects`, `item`, `state`, etc.) and routes to
`cli.go`. With no subcommand (or `mcp`), it starts the MCP server.
This means a single binary serves both AI agents and humans.

## Testing

```bash
# Unit tests
go test ./...

# Smoke test (requires real API key)
PLANE_API_KEY=plane_api_xxx PLANE_WORKSPACE_SLUG=erp \
  go run ./cmd/plane-mcp-smoke
```

## Configuration

| Env var | Required | Default | Description |
|---|---|---|---|
| `PLANE_API_KEY` | yes | — | API key from Plane Profile Settings |
| `PLANE_WORKSPACE_SLUG` | yes | — | Workspace slug (e.g. "erp") |
| `PLANE_API_HOST_URL` | no | `https://api.plane.so` | API base URL (for self-hosted) |
| `PLANE_CACHE_TTL` | no | 5m | Project cache TTL |

## Differences from the official server

1. **No OAuth support** — this server is API-key only. If you need OAuth
   for Plane Apps, use the official remote MCP at `mcp.plane.so`.
2. **No 100+ tool surface** — we only expose the 13 tools needed for
   the saga work-item flow. Add more as needed in `internal/server/server.go`.
3. **Self-hosted friendly** — point `PLANE_API_HOST_URL` at your own
   Plane instance.

## Adding a new tool

Each Plane operation gets three layers, in this order:

1. **Ops method** — `internal/ops/ops.go`. Thin wrapper around the
   client, adds caching and convenience. One test in
   `internal/ops/ops_test.go` using `httptest.Server`.
2. **MCP tool handler** — `internal/server/server.go`. Glue between
   MCP tool schema and the ops method. Optional; only needed if the
   tool is exposed to agents.
3. **CLI subcommand** — `cmd/plane-mcp/cli.go`. Optional; only needed
   for human/script use. Follow the existing pattern: a `cmdX` function
   that parses its own flags, calls the ops method, and prints via
   the `writer`. Wire it up in `runCLI`.

The skill file at `.opencode/skills/plane-mcp/SKILL.md` should be
updated whenever the tool surface or CLI changes.

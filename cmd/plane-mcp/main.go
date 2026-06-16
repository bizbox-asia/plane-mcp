// Command plane-mcp is a thin, fast Plane REST API client that runs in
// one of two modes:
//
//   1. MCP server (default): speaks Model Context Protocol over stdio
//      for AI agents like opencode. 13 focused tools for the saga
//      work-item flow.
//   2. CLI: direct command-line interface for humans and shell scripts.
//      `plane-mcp <command> [args]` runs a single operation and exits.
//
// Both modes share the same internal HTTP client, ops layer, and
// configuration. Run `plane-mcp help` to see CLI usage.
//
// Configuration via env vars (or -api-key / -workspace / -base-url flags):
//   - PLANE_API_KEY         (required)
//   - PLANE_WORKSPACE_SLUG  (required)
//   - PLANE_API_HOST_URL    (optional, defaults to https://api.plane.so)
//
// Usage:
//   plane-mcp                                            # start MCP server
//   plane-mcp projects                                   # list projects
//   plane-mcp item SAGA 5                                # get SAGA-5
//   plane-mcp state SAGA 5 "In Review"                   # change state
//   plane-mcp comment SAGA 5 "MR opened"                 # add comment
//   plane-mcp -format=json items SAGA | jq '.[].sequence_id'
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/server"
)

// Build-time variables. Overridden via -ldflags by the Makefile.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, errOut io.Writer) int {
	// Define global flags. We always define -version so users can ask
	// for it without specifying a subcommand.
	fs := flag.NewFlagSet("plane-mcp", flag.ContinueOnError)
	fs.SetOutput(errOut)
	apiKey := fs.String("api-key", envOr("PLANE_API_KEY", ""), "Plane API key (env: PLANE_API_KEY)")
	workspace := fs.String("workspace", envOr("PLANE_WORKSPACE_SLUG", ""), "Workspace slug (env: PLANE_WORKSPACE_SLUG)")
	baseURL := fs.String("base-url", envOr("PLANE_API_HOST_URL", "https://api.plane.so"), "API base URL (env: PLANE_API_HOST_URL)")
	format := fs.String("format", "text", "Output format: text|json")
	showVersion := fs.Bool("version", false, "Print version and exit")

	// Parse all args. flag.Parse stops at the first non-flag arg, so
	// anything left in fs.Args() is the subcommand + its positional args.
	// This handles both -key value and -key=value transparently.
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		fmt.Fprintf(errOut, "plane-mcp: %v\n", err)
		printGlobalUsage(errOut, fs)
		return 2
	}

	if *showVersion {
		fmt.Fprintf(errOut, "plane-mcp %s (commit=%s, buildDate=%s)\n", version, commit, buildDate)
		return 0
	}

	remaining := fs.Args()
	subCmd := ""
	var subArgs []string
	if len(remaining) > 0 {
		subCmd = remaining[0]
		subArgs = remaining[1:]
	}

	// Default subcommand: "mcp" (stdio JSON-RPC server). This keeps
	// `plane-mcp` (no args) backward-compatible with the opencode.json
	// config that just calls "./bin/plane-mcp".
	if subCmd == "" {
		subCmd = "mcp"
	}
	if subCmd == "help" || subCmd == "-h" || subCmd == "--help" {
		printHelp(errOut, fs)
		return 0
	}

	// Build config from flags/env.
	cfg := cliConfig{
		APIKey:    *apiKey,
		Workspace: *workspace,
		BaseURL:   *baseURL,
		Format:    *format,
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}

	switch subCmd {
	case "mcp":
		return runMCP(cfg, errOut)
	default:
		return runCLI(cfg, subCmd, subArgs, errOut)
	}
}

func runMCP(cfg cliConfig, errOut io.Writer) int {
	fmt.Fprintf(errOut, "plane-mcp %s (commit=%s, buildDate=%s)\n",
		cfg.Version, cfg.Commit, cfg.BuildDate)
	if cfg.APIKey == "" {
		fmt.Fprintln(errOut, "plane-mcp: PLANE_API_KEY is required")
		return 1
	}
	if cfg.Workspace == "" {
		fmt.Fprintln(errOut, "plane-mcp: PLANE_WORKSPACE_SLUG is required")
		return 1
	}

	srv, err := server.New(cfg.APIKey, cfg.Workspace, cfg.BaseURL)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: %v\n", err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(errOut, "plane-mcp: starting (workspace=%s, base=%s)\n",
		cfg.Workspace, cfg.BaseURL)
	if err := srv.Serve(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(errOut, "plane-mcp: serve error: %v\n", err)
		return 1
	}
	fmt.Fprintln(errOut, "plane-mcp: stopped")
	return 0
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func printGlobalUsage(errOut io.Writer, fs *flag.FlagSet) {
	fmt.Fprintf(errOut, "\nUsage: plane-mcp [global flags] <command> [args]\n\n")
	fmt.Fprintf(errOut, "Run 'plane-mcp help' for the full command list.\n")
	fs.PrintDefaults()
}

func printHelp(errOut io.Writer, fs *flag.FlagSet) {
	fmt.Fprintf(errOut, `plane-mcp — Plane REST API client and MCP server

Usage:
  plane-mcp [global flags] <command> [args]

Global flags:
`)
	fs.PrintDefaults()
	fmt.Fprintf(errOut, `
Commands:
  mcp                       Start MCP stdio server (default if no command)
  help                      Print this help

  projects                  List all projects in the workspace
  project <ID>              Show project details (by short identifier)
  items <PROJECT>           List work items in a project
  item <PROJECT> <SEQ>      Show work item details (e.g. SAGA 5)
  states <PROJECT>          List states in a project
  state <PROJECT> <SEQ> <NAME>   Change a work item's state by name
  comment <PROJECT> <SEQ> <TEXT>  Add a comment to a work item
  update <PROJECT> <SEQ> [flags]  Update fields on a work item
  health                    Check the connection to Plane

Examples:
  # List projects (text)
  plane-mcp projects

  # Get a work item as JSON
  plane-mcp -format=json item SAGA 5

  # Move a work item to "In Review"
  plane-mcp state SAGA 5 "In Review"

  # Add a comment
  plane-mcp comment SAGA 5 "MR opened: https://..."

  # Update title and priority
  plane-mcp update SAGA 5 -title "New title" -priority high

  # Start the MCP server (the default behavior)
  plane-mcp mcp
`)
}

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/client"
	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/models"
	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/ops"
)

// cliConfig is the resolved configuration for a CLI/MCP invocation.
type cliConfig struct {
	APIKey    string
	Workspace string
	BaseURL   string
	Format    string // "text" or "json"
	Version   string
	Commit    string
	BuildDate string
}

// runCLI dispatches to the right subcommand handler. Returns the
// process exit code.
func runCLI(cfg cliConfig, subCmd string, args []string, errOut io.Writer) int {
	if cfg.APIKey == "" {
		fmt.Fprintln(errOut, "plane-mcp: PLANE_API_KEY is required (-api-key flag or env var)")
		return 1
	}
	if cfg.Workspace == "" {
		fmt.Fprintln(errOut, "plane-mcp: PLANE_WORKSPACE_SLUG is required (-workspace flag or env var)")
		return 1
	}

	c := client.New(client.Config{
		BaseURL:  cfg.BaseURL,
		APIKey:   cfg.APIKey,
		CacheTTL: 5 * time.Minute,
	})
	o := ops.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	w := newWriter(cfg.Format, os.Stdout)
	defer w.close()

	switch subCmd {
	case "projects":
		return cmdProjects(ctx, o, cfg, w, args, errOut)
	case "project":
		return cmdProject(ctx, o, cfg, w, args, errOut)
	case "items":
		return cmdItems(ctx, o, cfg, w, args, errOut)
	case "item":
		return cmdItem(ctx, o, cfg, w, args, errOut)
	case "states":
		return cmdStates(ctx, o, cfg, w, args, errOut)
	case "state":
		return cmdState(ctx, o, cfg, w, args, errOut)
	case "comment":
		return cmdComment(ctx, o, cfg, w, args, errOut)
	case "update":
		return cmdUpdate(ctx, o, cfg, w, args, errOut)
	case "health":
		return cmdHealth(ctx, o, cfg, w, errOut)
	default:
		fmt.Fprintf(errOut, "plane-mcp: unknown command %q\n\nRun 'plane-mcp help' for usage.\n", subCmd)
		return 2
	}
}

// ---- Subcommand handlers ----

func cmdProjects(ctx context.Context, o *ops.Ops, cfg cliConfig, w *writer, args []string, errOut io.Writer) int {
	projects, err := o.ListProjects(ctx, cfg.Workspace)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: list projects: %v\n", err)
		return 1
	}
	if cfg.Format == "json" {
		return w.writeJSON(projects)
	}
	for _, p := range projects {
		fmt.Fprintf(w.out, "%-6s  %s\n", p.Identifier, p.Name)
	}
	return 0
}

func cmdProject(ctx context.Context, o *ops.Ops, cfg cliConfig, w *writer, args []string, errOut io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(errOut, "plane-mcp: project: missing <ID> argument")
		return 2
	}
	p, err := o.GetProjectByIdentifier(ctx, cfg.Workspace, args[0])
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: get project: %v\n", err)
		return 1
	}
	if cfg.Format == "json" {
		return w.writeJSON(p)
	}
	fmt.Fprintf(w.out, "ID          %s\n", p.ID)
	fmt.Fprintf(w.out, "Identifier  %s\n", p.Identifier)
	fmt.Fprintf(w.out, "Name        %s\n", p.Name)
	fmt.Fprintf(w.out, "Description %s\n", truncate(p.Description, 200))
	fmt.Fprintf(w.out, "Created     %s\n", p.CreatedAt.Format(time.RFC3339))
	return 0
}

func cmdItems(ctx context.Context, o *ops.Ops, cfg cliConfig, w *writer, args []string, errOut io.Writer) int {
	projectID, err := requireArg(args, 0, "items <PROJECT>")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	p, err := o.GetProjectByIdentifier(ctx, cfg.Workspace, projectID)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: get project: %v\n", err)
		return 1
	}
	resp, err := o.ListWorkItems(ctx, cfg.Workspace, p.ID, client.PerPage(100))
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: list items: %v\n", err)
		return 1
	}
	if cfg.Format == "json" {
		return w.writeJSON(resp.Results)
	}
	for _, wi := range resp.Results {
		state := ""
		if wi.StateDetail != nil {
			state = wi.StateDetail.Name
		}
		fmt.Fprintf(w.out, "%s-%d  [%-10s]  %s\n", p.Identifier, wi.SequenceID, state, wi.Name)
	}
	return 0
}

func cmdItem(ctx context.Context, o *ops.Ops, cfg cliConfig, w *writer, args []string, errOut io.Writer) int {
	projectID, err := requireArg(args, 0, "item <PROJECT> <SEQ>")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	seq, err := requireArg(args, 1, "item <PROJECT> <SEQ>")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	wi, err := o.GetWorkItemBySequence(ctx, cfg.Workspace, projectID, seq)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: get item: %v\n", err)
		return 1
	}
	if cfg.Format == "json" {
		return w.writeJSON(wi)
	}
	state := ""
	if wi.StateDetail != nil {
		state = wi.StateDetail.Name
	}
	fmt.Fprintf(w.out, "%s-%d  %s\n", projectID, wi.SequenceID, wi.Name)
	fmt.Fprintf(w.out, "  ID        %s\n", wi.ID)
	fmt.Fprintf(w.out, "  State     %s\n", state)
	fmt.Fprintf(w.out, "  Priority  %s\n", wi.Priority)
	fmt.Fprintf(w.out, "  Created   %s\n", wi.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(w.out, "  Updated   %s\n", wi.UpdatedAt.Format(time.RFC3339))
	if wi.Description != "" {
		fmt.Fprintf(w.out, "\n%s\n", truncate(wi.Description, 500))
	}
	return 0
}

func cmdStates(ctx context.Context, o *ops.Ops, cfg cliConfig, w *writer, args []string, errOut io.Writer) int {
	projectID, err := requireArg(args, 0, "states <PROJECT>")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	p, err := o.GetProjectByIdentifier(ctx, cfg.Workspace, projectID)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: get project: %v\n", err)
		return 1
	}
	states, err := o.ListStates(ctx, cfg.Workspace, p.ID)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: list states: %v\n", err)
		return 1
	}
	if cfg.Format == "json" {
		return w.writeJSON(states)
	}
	for _, s := range states {
		fmt.Fprintf(w.out, "%-15s  group=%-10s  color=%s\n", s.Name, s.Group, s.Color)
	}
	return 0
}

func cmdState(ctx context.Context, o *ops.Ops, cfg cliConfig, w *writer, args []string, errOut io.Writer) int {
	projectID, err := requireArg(args, 0, "state <PROJECT> <SEQ> <NAME>")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	seq, err := requireArg(args, 1, "state <PROJECT> <SEQ> <NAME>")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	stateName, err := requireArg(args, 2, "state <PROJECT> <SEQ> <NAME>")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	p, err := o.GetProjectByIdentifier(ctx, cfg.Workspace, projectID)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: get project: %v\n", err)
		return 1
	}
	wi, err := o.GetWorkItemBySequence(ctx, cfg.Workspace, projectID, seq)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: get item: %v\n", err)
		return 1
	}
	updated, err := o.UpdateWorkItemState(ctx, cfg.Workspace, p.ID, wi.ID, stateName)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: update state: %v\n", err)
		return 1
	}
	if cfg.Format == "json" {
		return w.writeJSON(updated)
	}
	newState := ""
	if updated.StateDetail != nil {
		newState = updated.StateDetail.Name
	}
	fmt.Fprintf(w.out, "%s-%d → %s\n", projectID, updated.SequenceID, newState)
	return 0
}

func cmdComment(ctx context.Context, o *ops.Ops, cfg cliConfig, w *writer, args []string, errOut io.Writer) int {
	projectID, err := requireArg(args, 0, "comment <PROJECT> <SEQ> <TEXT>")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	seq, err := requireArg(args, 1, "comment <PROJECT> <SEQ> <TEXT>")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	if len(args) < 3 {
		fmt.Fprintln(errOut, "plane-mcp: comment: missing <TEXT>")
		return 2
	}
	text := strings.Join(args[2:], " ")
	p, err := o.GetProjectByIdentifier(ctx, cfg.Workspace, projectID)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: get project: %v\n", err)
		return 1
	}
	wi, err := o.GetWorkItemBySequence(ctx, cfg.Workspace, projectID, seq)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: get item: %v\n", err)
		return 1
	}
	// Wrap plain text in <p> tags so it renders nicely in Plane.
	html := "<p>" + escapeHTML(text) + "</p>"
	c, err := o.AddComment(ctx, cfg.Workspace, p.ID, wi.ID, html)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: add comment: %v\n", err)
		return 1
	}
	if cfg.Format == "json" {
		return w.writeJSON(c)
	}
	fmt.Fprintf(w.out, "added comment to %s-%d (id=%s)\n", projectID, wi.SequenceID, c.ID)
	return 0
}

func cmdUpdate(ctx context.Context, o *ops.Ops, cfg cliConfig, w *writer, args []string, errOut io.Writer) int {
	projectID, err := requireArg(args, 0, "update <PROJECT> <SEQ> [flags]")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	seq, err := requireArg(args, 1, "update <PROJECT> <SEQ> [flags]")
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}

	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(errOut)
	title := fs.String("title", "", "New title")
	state := fs.String("state", "", "New state (looked up by name)")
	priority := fs.String("priority", "", "New priority (urgent|high|medium|low|none)")
	description := fs.String("description", "", "New description (HTML)")
	targetDate := fs.String("target-date", "", "New target date (YYYY-MM-DD)")
	startDate := fs.String("start-date", "", "New start date (YYYY-MM-DD)")
	assignees := fs.String("assignees", "", "Comma-separated assignee UUIDs")
	if err := fs.Parse(args[2:]); err != nil {
		return 2
	}

	// Validate that at least one field was set BEFORE making any API
	// calls. Otherwise a typo'd command would hit the network and then
	// fail with a confusing connection error instead of a usage hint.
	if *title == "" && *state == "" && *priority == "" && *description == "" &&
		*targetDate == "" && *startDate == "" && *assignees == "" {
		fmt.Fprintln(errOut, "plane-mcp: update: at least one field flag is required (e.g. -title, -state, -priority)")
		fmt.Fprintln(errOut, "Run 'plane-mcp help' or pass -h to see all flags.")
		return 2
	}

	p, err := o.GetProjectByIdentifier(ctx, cfg.Workspace, projectID)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: get project: %v\n", err)
		return 1
	}
	wi, err := o.GetWorkItemBySequence(ctx, cfg.Workspace, projectID, seq)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: get item: %v\n", err)
		return 1
	}

	// Build partial update. We only set fields that were provided.
	update := models.WorkItemUpdate{}
	if *title != "" {
		s := *title
		update.Name = &s
	}
	if *priority != "" {
		s := *priority
		update.Priority = &s
	}
	if *description != "" {
		s := *description
		update.Description = &s
	}
	if *targetDate != "" {
		s := *targetDate
		update.TargetDate = &s
	}
	if *startDate != "" {
		s := *startDate
		update.StartDate = &s
	}
	if *assignees != "" {
		ids := strings.Split(*assignees, ",")
		cleaned := make([]string, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id != "" {
				cleaned = append(cleaned, id)
			}
		}
		update.Assignees = &cleaned
	}
	if *state != "" {
		// State needs a lookup by name → resolve to state ID first.
		states, err := o.ListStates(ctx, cfg.Workspace, p.ID)
		if err != nil {
			fmt.Fprintf(errOut, "plane-mcp: list states: %v\n", err)
			return 1
		}
		stateID := ""
		for _, s := range states {
			if strings.EqualFold(s.Name, *state) {
				stateID = s.ID
				break
			}
		}
		if stateID == "" {
			fmt.Fprintf(errOut, "plane-mcp: state %q not found in project %s\n", *state, projectID)
			return 1
		}
		update.State = &stateID
	}

	updated, err := o.UpdateWorkItem(ctx, cfg.Workspace, p.ID, wi.ID, update)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: update item: %v\n", err)
		return 1
	}
	if cfg.Format == "json" {
		return w.writeJSON(updated)
	}
	fmt.Fprintf(w.out, "updated %s-%d\n", projectID, updated.SequenceID)
	return 0
}

func cmdHealth(ctx context.Context, o *ops.Ops, cfg cliConfig, w *writer, errOut io.Writer) int {
	// Cheap liveness: just list projects with per_page=1.
	projects, err := o.ListProjects(ctx, cfg.Workspace)
	if err != nil {
		fmt.Fprintf(errOut, "plane-mcp: health check failed: %v\n", err)
		return 1
	}
	result := map[string]any{
		"ok":              true,
		"workspace":       cfg.Workspace,
		"base_url":        cfg.BaseURL,
		"project_count":   len(projects),
		"cache_projects":  o.CacheStats(),
		"version":         cfg.Version,
		"commit":          cfg.Commit,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	}
	if cfg.Format == "json" {
		return w.writeJSON(result)
	}
	fmt.Fprintf(w.out, "ok             true\n")
	fmt.Fprintf(w.out, "workspace      %s\n", cfg.Workspace)
	fmt.Fprintf(w.out, "base_url       %s\n", cfg.BaseURL)
	fmt.Fprintf(w.out, "project_count  %d\n", len(projects))
	fmt.Fprintf(w.out, "version        %s\n", cfg.Version)
	return 0
}

// ---- helpers ----

func requireArg(args []string, idx int, usage string) (string, error) {
	if idx >= len(args) {
		return "", fmt.Errorf("plane-mcp: missing argument\nusage: %s", usage)
	}
	return args[idx], nil
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
}

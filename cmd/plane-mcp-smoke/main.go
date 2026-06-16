// Command plane-mcp-smoke is a manual integration test that
// exercises the plane-mcp binary against the real Plane API.
// Run with:
//   PLANE_API_KEY=xxx PLANE_WORKSPACE_SLUG=erp \
//     go run ./cmd/plane-mcp-smoke
//
// It sends a single MCP request via stdio and prints the response.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/client"
	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/ops"
)

func main() {
	apiKey := os.Getenv("PLANE_API_KEY")
	workspace := os.Getenv("PLANE_WORKSPACE_SLUG")
	baseURL := os.Getenv("PLANE_API_HOST_URL")
	if baseURL == "" {
		baseURL = "https://api.plane.so"
	}
	if apiKey == "" || workspace == "" {
		fmt.Fprintln(os.Stderr, "PLANE_API_KEY and PLANE_WORKSPACE_SLUG are required")
		os.Exit(1)
	}

	// Quick ops-level check first (no MCP overhead).
	fmt.Println("=== Direct ops check ===")
	c := client.New(client.Config{BaseURL: baseURL, APIKey: apiKey, CacheTTL: time.Minute})
	o := ops.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t0 := time.Now()
	projects, err := o.ListProjects(ctx, workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListProjects failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Listed %d projects in %s\n", len(projects), time.Since(t0))
	for _, p := range projects {
		fmt.Printf("  - %s (%s): %s\n", p.Identifier, p.ID, p.Name)
	}

	// Find a project to use for further checks.
	if len(projects) == 0 {
		fmt.Fprintln(os.Stderr, "no projects found, skipping deeper checks")
		return
	}
	target := projects[0]
	fmt.Printf("\n=== Listing work items in %s ===\n", target.Identifier)
	wi, err := o.ListWorkItems(ctx, workspace, target.ID, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListWorkItems failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Found %d work items (total: %d)\n", len(wi.Results), wi.TotalResults)
	for i, item := range wi.Results {
		fmt.Printf("  - %s-%d: %s [%s]\n", target.Identifier, item.SequenceID, item.Name, item.State)
		if i >= 4 {
			fmt.Printf("  - ... and %d more\n", len(wi.Results)-5)
			break
		}
	}

	// Now exercise the binary via stdio. The MCP stdio protocol
	// requires newline-delimited JSON. We write to stdin, close it,
	// and read stdout via Output().
	fmt.Println("\n=== MCP binary smoke test ===")
	binPath := os.Getenv("PLANE_MCP_BIN")
	if binPath == "" {
		binPath = "./bin/plane-mcp"
	}
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "binary not found at %s (run `go build -o bin/plane-mcp ./cmd/plane-mcp` first)\n", binPath)
		os.Exit(1)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()
	cmd := exec.CommandContext(ctx2, binPath)
	cmd.Env = append(os.Environ(),
		"PLANE_API_KEY="+apiKey,
		"PLANE_WORKSPACE_SLUG="+workspace,
		"PLANE_API_HOST_URL="+baseURL,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin pipe: %v\n", err)
		os.Exit(1)
	}
	go func() {
		defer stdin.Close()
		_, _ = stdin.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"))
	}()
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "binary exec failed: %v\n", err)
		os.Exit(1)
	}
	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "binary returned invalid JSON: %v\noutput: %s\n", err, string(out))
		os.Exit(1)
	}
	fmt.Printf("Binary registered %d tools\n", len(resp.Result.Tools))
	if len(resp.Result.Tools) < 5 {
		fmt.Fprintf(os.Stderr, "expected at least 5 tools, got %d\noutput: %s\n", len(resp.Result.Tools), string(out))
		os.Exit(1)
	}
	for _, t := range resp.Result.Tools {
		fmt.Printf("  - %s\n", t.Name)
	}
}

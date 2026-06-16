// Package server implements the Plane MCP server using stdio transport.
//
// Exposes 13 tools for the saga work-item flow:
//   list_projects, get_project_by_identifier,
//   get_work_item_by_sequence, get_work_item_by_id, list_work_items,
//   create_work_item, update_work_item, update_work_item_state,
//   add_work_item_comment, list_states, list_modules, list_cycles, health.
//
// Config via env: PLANE_API_KEY, PLANE_WORKSPACE_SLUG, PLANE_API_HOST_URL.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/client"
	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/models"
	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/ops"
)

type Server struct {
	mcpServer *server.MCPServer
	ops       *ops.Ops
	workspace string
}

func New(apiKey, workspace, baseURL string) (*Server, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("PLANE_API_KEY is required")
	}
	if workspace == "" {
		return nil, fmt.Errorf("PLANE_WORKSPACE_SLUG is required")
	}
	c := client.New(client.Config{
		BaseURL:  baseURL,
		APIKey:   apiKey,
		CacheTTL: 5 * time.Minute,
	})
	o := ops.New(c)
	s := &Server{ops: o, workspace: workspace}
	s.mcpServer = server.NewMCPServer("plane-mcp", "1.0.0", server.WithToolCapabilities(false))
	s.registerTools()
	return s, nil
}

func (s *Server) Serve(ctx context.Context) error {
	stdioServer := server.NewStdioServer(s.mcpServer)
	return stdioServer.Listen(ctx, os.Stdin, os.Stdout)
}

func (s *Server) registerTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_projects", mcp.WithDescription("List all projects in the current workspace.")),
		s.handleListProjects,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("get_project_by_identifier",
			mcp.WithDescription("Get a project by its short identifier (e.g. 'SAGA')."),
			mcp.WithString("identifier", mcp.Required())),
		s.handleGetProjectByIdentifier,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("get_work_item_by_sequence",
			mcp.WithDescription("Get a work item by sequence (e.g. 'SAGA-5')."),
			mcp.WithString("identifier", mcp.Required()),
			mcp.WithString("sequence", mcp.Required())),
		s.handleGetWorkItemBySequence,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("get_work_item_by_id",
			mcp.WithDescription("Get a work item by UUID."),
			mcp.WithString("project_id", mcp.Required()),
			mcp.WithString("work_item_id", mcp.Required())),
		s.handleGetWorkItemByID,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_work_items",
			mcp.WithDescription("List work items in a project with cursor pagination."),
			mcp.WithString("project_id", mcp.Required()),
			mcp.WithNumber("per_page", mcp.Description("Items per page (1-100, default 100)")),
			mcp.WithString("cursor", mcp.Description("Pagination cursor"))),
		s.handleListWorkItems,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("create_work_item",
			mcp.WithDescription("Create a new work item."),
			mcp.WithString("project_id", mcp.Required()),
			mcp.WithObject("input", mcp.Required(),
				mcp.Properties(map[string]any{
					"name":        map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
					"state":       map[string]any{"type": "string"},
					"assignees":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"labels":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"priority":    map[string]any{"type": "string"},
				}),
			),
		),
		s.handleCreateWorkItem,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_work_item",
			mcp.WithDescription("Partial update of a work item."),
			mcp.WithString("project_id", mcp.Required()),
			mcp.WithString("work_item_id", mcp.Required()),
			mcp.WithObject("updates", mcp.Required(),
				mcp.Properties(map[string]any{
					"name":      map[string]any{"type": "string"},
					"state":     map[string]any{"type": "string"},
					"priority":  map[string]any{"type": "string"},
					"assignees": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				}),
			),
		),
		s.handleUpdateWorkItem,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("update_work_item_state",
			mcp.WithDescription("Move a work item to a new state by name."),
			mcp.WithString("project_id", mcp.Required()),
			mcp.WithString("work_item_id", mcp.Required()),
			mcp.WithString("state_name", mcp.Required())),
		s.handleUpdateWorkItemState,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("add_work_item_comment",
			mcp.WithDescription("Add an HTML comment to a work item."),
			mcp.WithString("project_id", mcp.Required()),
			mcp.WithString("work_item_id", mcp.Required()),
			mcp.WithString("html", mcp.Required())),
		s.handleAddComment,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_states",
			mcp.WithDescription("List all states in a project."),
			mcp.WithString("project_id", mcp.Required())),
		s.handleListStates,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_modules",
			mcp.WithDescription("List all modules in a project."),
			mcp.WithString("project_id", mcp.Required())),
		s.handleListModules,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_cycles",
			mcp.WithDescription("List all cycles in a project."),
			mcp.WithString("project_id", mcp.Required())),
		s.handleListCycles,
	)
	s.mcpServer.AddTool(
		mcp.NewTool("health",
			mcp.WithDescription("Check Plane MCP server health.")),
		s.handleHealth,
	)
}

func (s *Server) handleListProjects(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p, err := s.ops.ListProjects(ctx, s.workspace)
	return wrapResult(p, err)
}

func (s *Server) handleGetProjectByIdentifier(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("identifier")
	if err != nil {
		return errResult(err)
	}
	p, err := s.ops.GetProjectByIdentifier(ctx, s.workspace, id)
	return wrapResult(p, err)
}

func (s *Server) handleGetWorkItemBySequence(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("identifier")
	if err != nil {
		return errResult(err)
	}
	seq, err := req.RequireString("sequence")
	if err != nil {
		return errResult(err)
	}
	wi, err := s.ops.GetWorkItemBySequence(ctx, s.workspace, id, seq)
	return wrapResult(wi, err)
}

func (s *Server) handleGetWorkItemByID(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid, err := req.RequireString("project_id")
	if err != nil {
		return errResult(err)
	}
	wid, err := req.RequireString("work_item_id")
	if err != nil {
		return errResult(err)
	}
	wi, err := s.ops.GetWorkItemByID(ctx, s.workspace, pid, wid)
	return wrapResult(wi, err)
}

func (s *Server) handleListWorkItems(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid, err := req.RequireString("project_id")
	if err != nil {
		return errResult(err)
	}
	args := req.GetArguments()
	vals := url.Values{}
	if v, ok := args["per_page"]; ok {
		if n, ok := v.(float64); ok {
			vals.Set("per_page", fmt.Sprintf("%d", int(n)))
		}
	}
	if v, ok := args["cursor"]; ok {
		if s2, ok := v.(string); ok && s2 != "" {
			vals.Set("cursor", s2)
		}
	}
	resp, err := s.ops.ListWorkItems(ctx, s.workspace, pid, vals)
	return wrapResult(resp, err)
}

func (s *Server) handleCreateWorkItem(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid, err := req.RequireString("project_id")
	if err != nil {
		return errResult(err)
	}
	args := req.GetArguments()
	inputRaw, ok := args["input"]
	if !ok {
		return errResult(fmt.Errorf("input is required"))
	}
	inputBytes, err := json.Marshal(inputRaw)
	if err != nil {
		return errResult(err)
	}
	var input models.WorkItemCreate
	if err := json.Unmarshal(inputBytes, &input); err != nil {
		return errResult(err)
	}
	wi, err := s.ops.CreateWorkItem(ctx, s.workspace, pid, input)
	return wrapResult(wi, err)
}

func (s *Server) handleUpdateWorkItem(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid, err := req.RequireString("project_id")
	if err != nil {
		return errResult(err)
	}
	wid, err := req.RequireString("work_item_id")
	if err != nil {
		return errResult(err)
	}
	args := req.GetArguments()
	updatesRaw, ok := args["updates"]
	if !ok {
		return errResult(fmt.Errorf("updates is required"))
	}
	updatesBytes, err := json.Marshal(updatesRaw)
	if err != nil {
		return errResult(err)
	}
	var update models.WorkItemUpdate
	if err := json.Unmarshal(updatesBytes, &update); err != nil {
		return errResult(err)
	}
	wi, err := s.ops.UpdateWorkItem(ctx, s.workspace, pid, wid, update)
	return wrapResult(wi, err)
}

func (s *Server) handleUpdateWorkItemState(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid, err := req.RequireString("project_id")
	if err != nil {
		return errResult(err)
	}
	wid, err := req.RequireString("work_item_id")
	if err != nil {
		return errResult(err)
	}
	stateName, err := req.RequireString("state_name")
	if err != nil {
		return errResult(err)
	}
	wi, err := s.ops.UpdateWorkItemState(ctx, s.workspace, pid, wid, stateName)
	return wrapResult(wi, err)
}

func (s *Server) handleAddComment(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid, err := req.RequireString("project_id")
	if err != nil {
		return errResult(err)
	}
	wid, err := req.RequireString("work_item_id")
	if err != nil {
		return errResult(err)
	}
	html, err := req.RequireString("html")
	if err != nil {
		return errResult(err)
	}
	c, err := s.ops.AddComment(ctx, s.workspace, pid, wid, html)
	return wrapResult(c, err)
}

func (s *Server) handleListStates(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid, err := req.RequireString("project_id")
	if err != nil {
		return errResult(err)
	}
	s2, err := s.ops.ListStates(ctx, s.workspace, pid)
	return wrapResult(s2, err)
}

func (s *Server) handleListModules(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid, err := req.RequireString("project_id")
	if err != nil {
		return errResult(err)
	}
	m, err := s.ops.ListModules(ctx, s.workspace, pid)
	return wrapResult(m, err)
}

func (s *Server) handleListCycles(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pid, err := req.RequireString("project_id")
	if err != nil {
		return errResult(err)
	}
	c, err := s.ops.ListCycles(ctx, s.workspace, pid)
	return wrapResult(c, err)
}

func (s *Server) handleHealth(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return wrapResult(map[string]any{
		"status":        "ok",
		"workspace":     s.workspace,
		"project_cache": s.ops.CacheStats(),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}, nil)
}

// wrapResult is a helper that converts (value, error) into a
// properly-shaped MCP result. Returns a single (*mcp.CallToolResult, error)
// tuple to satisfy the mcp-go handler signature.
func wrapResult(v any, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.NewTextContent(err.Error()),
			},
		}, nil
	}
	b, mErr := json.MarshalIndent(v, "", "  ")
	if mErr != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.NewTextContent(mErr.Error()),
			},
		}, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(string(b)),
		},
	}, nil
}

// errResult is a convenience for handlers that hit a parameter
// validation error before invoking any ops.
func errResult(err error) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.NewTextContent(err.Error()),
		},
	}, nil
}

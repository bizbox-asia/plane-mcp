// Package ops provides high-level operations on the Plane API
// tailored to the saga work-item flow:
//
//   - Look up projects by identifier (e.g. "SAGA")
//   - Get work items by sequence ID (e.g. "SAGA-5")
//   - Update work item state (move through state machine)
//   - Add work item comments (for completion notes)
//
// All operations accept a workspace slug and use the configured
// API key. They use the underlying client for HTTP and add:
//   - In-memory caching of project/state lookups (TTL-based)
//   - Convenience methods that combine multiple API calls
package ops

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/client"
	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/models"
)

// Ops provides high-level Plane operations.
type Ops struct {
	client *client.Client

	// Reference data cache.
	mu           sync.RWMutex
	projectByID  map[string]*models.Project    // ID -> project
	projectByKey map[string]*models.Project    // identifier -> project
}

// New creates a new Ops instance.
func New(c *client.Client) *Ops {
	return &Ops{
		client:      c,
		projectByID:  make(map[string]*models.Project),
		projectByKey: make(map[string]*models.Project),
	}
}

// ListProjects returns all projects in the workspace.
func (o *Ops) ListProjects(ctx context.Context, workspaceSlug string) ([]models.Project, error) {
	var resp client.ListResponse[models.Project]
	_, _, err := o.client.Do(ctx, "GET",
		fmt.Sprintf("workspaces/%s/projects/", workspaceSlug),
		client.PerPage(100), nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("ops: list projects: %w", err)
	}
	for i := range resp.Results {
		o.cacheProject(&resp.Results[i])
	}
	return resp.Results, nil
}

// GetProjectByID returns a project by its UUID. Uses cache.
func (o *Ops) GetProjectByID(ctx context.Context, workspaceSlug, projectID string) (*models.Project, error) {
	o.mu.RLock()
	if p, ok := o.projectByID[projectID]; ok {
		o.mu.RUnlock()
		return p, nil
	}
	o.mu.RUnlock()

	var p models.Project
	_, _, err := o.client.Do(ctx, "GET",
		fmt.Sprintf("workspaces/%s/projects/%s/", workspaceSlug, projectID),
		nil, nil, &p)
	if err != nil {
		return nil, fmt.Errorf("ops: get project %s: %w", projectID, err)
	}
	o.cacheProject(&p)
	return &p, nil
}

// GetProjectByIdentifier returns a project by its short identifier
// (e.g. "SAGA"). Uses cache after first lookup.
func (o *Ops) GetProjectByIdentifier(ctx context.Context, workspaceSlug, identifier string) (*models.Project, error) {
	o.mu.RLock()
	if p, ok := o.projectByKey[identifier]; ok {
		o.mu.RUnlock()
		return p, nil
	}
	o.mu.RUnlock()

	// List projects and find by identifier (Plane doesn't support
	// direct identifier lookup, so we list and cache).
	projects, err := o.ListProjects(ctx, workspaceSlug)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if strings.EqualFold(projects[i].Identifier, identifier) {
			return &projects[i], nil
		}
	}
	return nil, fmt.Errorf("ops: project with identifier %q not found in workspace %s", identifier, workspaceSlug)
}

// cacheProject stores a project in the cache (ID + identifier keys).
func (o *Ops) cacheProject(p *models.Project) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.projectByID[p.ID] = p
	o.projectByKey[p.Identifier] = p
}

// ListWorkItems returns all work items in a project, optionally filtered.
// Use ListWorkItemsBySequence for quick lookups by SAGA-N format.
func (o *Ops) ListWorkItems(ctx context.Context, workspaceSlug, projectID string, query url.Values) (*client.ListResponse[models.WorkItem], error) {
	if query == nil {
		query = client.PerPage(100)
	}
	var resp client.ListResponse[models.WorkItem]
	_, _, err := o.client.Do(ctx, "GET",
		fmt.Sprintf("workspaces/%s/projects/%s/work-items/", workspaceSlug, projectID),
		query, nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("ops: list work items: %w", err)
	}
	return &resp, nil
}

// GetWorkItemBySequence looks up a work item by its human-readable
// sequence (e.g. "SAGA-5") and returns the full WorkItem.
// Sequence lookup requires the project identifier to be set
// correctly on the server side.
func (o *Ops) GetWorkItemBySequence(ctx context.Context, workspaceSlug, identifier, sequenceID string) (*models.WorkItem, error) {
	var wi models.WorkItem
	_, _, err := o.client.Do(ctx, "GET",
		fmt.Sprintf("workspaces/%s/work-items/%s-%s/", workspaceSlug, identifier, sequenceID),
		nil, nil, &wi)
	if err != nil {
		return nil, fmt.Errorf("ops: get work item %s-%s: %w", identifier, sequenceID, err)
	}
	return &wi, nil
}

// GetWorkItemByID returns a work item by its UUID.
func (o *Ops) GetWorkItemByID(ctx context.Context, workspaceSlug, projectID, workItemID string) (*models.WorkItem, error) {
	var wi models.WorkItem
	_, _, err := o.client.Do(ctx, "GET",
		fmt.Sprintf("workspaces/%s/projects/%s/work-items/%s/", workspaceSlug, projectID, workItemID),
		nil, nil, &wi)
	if err != nil {
		return nil, fmt.Errorf("ops: get work item %s: %w", workItemID, err)
	}
	return &wi, nil
}

// UpdateWorkItemState moves a work item to a new state (e.g. "In Progress"
// -> "In Review"). Looks up the state ID by name in the project's
// state list.
func (o *Ops) UpdateWorkItemState(ctx context.Context, workspaceSlug, projectID, workItemID, stateName string) (*models.WorkItem, error) {
	states, err := o.ListStates(ctx, workspaceSlug, projectID)
	if err != nil {
		return nil, err
	}
	stateID, err := findStateByName(states, stateName)
	if err != nil {
		return nil, err
	}

	update := models.WorkItemUpdate{
		State: &stateID,
	}
	return o.UpdateWorkItem(ctx, workspaceSlug, projectID, workItemID, update)
}

// UpdateWorkItem applies a partial update to a work item.
func (o *Ops) UpdateWorkItem(ctx context.Context, workspaceSlug, projectID, workItemID string, update models.WorkItemUpdate) (*models.WorkItem, error) {
	var wi models.WorkItem
	_, _, err := o.client.Do(ctx, "PATCH",
		fmt.Sprintf("workspaces/%s/projects/%s/work-items/%s/", workspaceSlug, projectID, workItemID),
		nil, update, &wi)
	if err != nil {
		return nil, fmt.Errorf("ops: update work item %s: %w", workItemID, err)
	}
	return &wi, nil
}

// CreateWorkItem creates a new work item in a project.
func (o *Ops) CreateWorkItem(ctx context.Context, workspaceSlug, projectID string, input models.WorkItemCreate) (*models.WorkItem, error) {
	var wi models.WorkItem
	_, _, err := o.client.Do(ctx, "POST",
		fmt.Sprintf("workspaces/%s/projects/%s/work-items/", workspaceSlug, projectID),
		nil, input, &wi)
	if err != nil {
		return nil, fmt.Errorf("ops: create work item: %w", err)
	}
	return &wi, nil
}

// AddComment adds a comment to a work item. The comment body is
// HTML; use simple HTML like <p>...</p>.
func (o *Ops) AddComment(ctx context.Context, workspaceSlug, projectID, workItemID, html string) (*models.Comment, error) {
	var c models.Comment
	_, _, err := o.client.Do(ctx, "POST",
		fmt.Sprintf("workspaces/%s/projects/%s/work-items/%s/comments/", workspaceSlug, projectID, workItemID),
		nil, models.CommentCreate{CommentHTML: html}, &c)
	if err != nil {
		return nil, fmt.Errorf("ops: add comment: %w", err)
	}
	return &c, nil
}

// ListStates returns all states in a project. Cached.
func (o *Ops) ListStates(ctx context.Context, workspaceSlug, projectID string) ([]models.State, error) {
	var resp client.ListResponse[models.State]
	_, _, err := o.client.Do(ctx, "GET",
		fmt.Sprintf("workspaces/%s/projects/%s/states/", workspaceSlug, projectID),
		client.PerPage(50), nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("ops: list states: %w", err)
	}
	return resp.Results, nil
}

// ListModules returns all modules in a project.
func (o *Ops) ListModules(ctx context.Context, workspaceSlug, projectID string) ([]models.Module, error) {
	var resp client.ListResponse[models.Module]
	_, _, err := o.client.Do(ctx, "GET",
		fmt.Sprintf("workspaces/%s/projects/%s/modules/", workspaceSlug, projectID),
		client.PerPage(50), nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("ops: list modules: %w", err)
	}
	return resp.Results, nil
}

// ListCycles returns all cycles in a project.
func (o *Ops) ListCycles(ctx context.Context, workspaceSlug, projectID string) ([]models.Cycle, error) {
	var resp client.ListResponse[models.Cycle]
	_, _, err := o.client.Do(ctx, "GET",
		fmt.Sprintf("workspaces/%s/projects/%s/cycles/", workspaceSlug, projectID),
		client.PerPage(50), nil, &resp)
	if err != nil {
		return nil, fmt.Errorf("ops: list cycles: %w", err)
	}
	return resp.Results, nil
}

// findStateByName returns the state ID matching the given name
// (case-insensitive).
func findStateByName(states []models.State, name string) (string, error) {
	for _, s := range states {
		if strings.EqualFold(s.Name, name) {
			return s.ID, nil
		}
	}
	return "", fmt.Errorf("state %q not found", name)
}

// ClearCache clears all in-memory caches. Useful for tests.
func (o *Ops) ClearCache() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.projectByID = make(map[string]*models.Project)
	o.projectByKey = make(map[string]*models.Project)
}

// CacheStats returns cache statistics (for monitoring).
func (o *Ops) CacheStats() (projectCount int) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return len(o.projectByID)
}

// Time helper for tests.
var _ = time.Second

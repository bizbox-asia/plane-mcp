// Package models contains the Plane data structures we work with.
// Fields are kept minimal — we only include what our work-item
// flow needs.
package models

import "time"

// Project represents a Plane project (one per Plane project).
type Project struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Identifier        string    `json:"identifier"` // e.g. "SAGA"
	Description       string    `json:"description"`
	Network           int       `json:"network"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// State represents a work item state (e.g. "Backlog", "In Progress").
// State.Group is "backlog" | "unstarted" | "started" | "completed" | "cancelled".
type State struct {
	ID         string  `json:"id"`
	Name      string  `json:"name"`
	Color     string  `json:"color"`
	Group     string  `json:"group"`
	Sequence  float64 `json:"sequence"`
	Default   bool    `json:"default"`
}

// Module represents a work item module (e.g. "Saga").
type Module struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Description string `json:"description"`
}

// Cycle represents a sprint/iteration.
type Cycle struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	StartDate  string    `json:"start_date"`
	EndDate    string    `json:"end_date"`
	OwnedBy    string    `json:"owned_by"`
}

// Label represents a work item label.
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Member represents a workspace/project member.
type Member struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	DisplayName string `json:"display_name"`
}

// WorkItem represents a work item (issue).
type WorkItem struct {
	ID                    string         `json:"id"`
	SequenceID            int            `json:"sequence_id"`
	ProjectID             string         `json:"project_id"`
	WorkspaceID           string         `json:"workspace_id"`
	Name                  string         `json:"name"`
	Description           string         `json:"description"`
	DescriptionHTML       string         `json:"description_html"`
	State                 string         `json:"state"`       // state ID
	StateDetail           *State         `json:"state_detail"`
	Assignees             []string       `json:"assignees"`
	Labels                []string       `json:"labels"`
	Priority              string         `json:"priority"`
	StartDate             string         `json:"start_date"`
	TargetDate            string         `json:"target_date"`
	Module                string         `json:"module"`       // module ID
	Cycle                 string         `json:"cycle"`        // cycle ID
	Parent                string         `json:"parent"`       // parent issue ID
	EstimatePoint         *int           `json:"estimate_point"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	CompletedAt           *time.Time     `json:"completed_at"`
	StartedAt             *time.Time     `json:"started_at"`
	TargetDateFinal       *time.Time     `json:"target_date_final"`
	Project                string         `json:"project"`      // project ID (alias)
	TypeID                string         `json:"type_id"`
	Metadata               map[string]any `json:"metadata"`
}

// WorkItemCreate is the payload for creating a work item.
type WorkItemCreate struct {
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	DescriptionHTML string         `json:"description_html,omitempty"`
	State           string         `json:"state,omitempty"`
	Assignees       []string       `json:"assignees,omitempty"`
	Labels          []string       `json:"labels,omitempty"`
	Priority        string         `json:"priority,omitempty"`
	StartDate       string         `json:"start_date,omitempty"`
	TargetDate      string         `json:"target_date,omitempty"`
	Module          string         `json:"module,omitempty"`
	Cycle           string         `json:"cycle,omitempty"`
}

// WorkItemUpdate is the payload for partial-updating a work item.
// Use a pointer-to-string to distinguish "not set" from "set to empty".
type WorkItemUpdate struct {
	Name            *string         `json:"name,omitempty"`
	Description     *string         `json:"description,omitempty"`
	State           *string         `json:"state,omitempty"`
	Assignees       *[]string       `json:"assignees,omitempty"`
	Labels          *[]string       `json:"labels,omitempty"`
	Priority        *string         `json:"priority,omitempty"`
	StartDate       *string         `json:"start_date,omitempty"`
	TargetDate      *string         `json:"target_date,omitempty"`
	Module          *string         `json:"module,omitempty"`
	Cycle           *string         `json:"cycle,omitempty"`
}

// Comment is a work item comment.
type Comment struct {
	ID          string    `json:"id"`
	Issue       string    `json:"issue"`
	Workspace   string    `json:"workspace"`
	Project     string    `json:"project"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CommentHTML string    `json:"comment_html"`
}

// CommentCreate is the payload for creating a comment.
type CommentCreate struct {
	CommentHTML string `json:"comment_html"`
}

// ProjectCreate is the payload for creating a project.
type ProjectCreate struct {
	Name        string `json:"name"`
	Identifier  string `json:"identifier"`
	Description string `json:"description,omitempty"`
	Network     int    `json:"network,omitempty"`
}

// ModuleCreate is the payload for creating a module.
type ModuleCreate struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Members     []string `json:"members,omitempty"`
}

// CycleCreate is the payload for creating a cycle.
type CycleCreate struct {
	Name      string `json:"name"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
}

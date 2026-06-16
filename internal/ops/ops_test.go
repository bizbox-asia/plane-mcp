package ops

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/client"
	"git.helium.vn/bizbox/erp/tools/plane-mcp/internal/models"
)

// mockPlaneServer returns a httptest.Server that responds to the
// endpoints the ops package uses. Each call to mockPlaneServer
// returns a fresh server — callers should defer srv.Close().
func mockPlaneServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func TestOps_ListProjects_CachesResults(t *testing.T) {
	var callCount int
	srv := mockPlaneServer(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.ListResponse[map[string]any]{
			Results: []map[string]any{
				{"id": "p1", "name": "SAGA", "identifier": "SAGA"},
			},
		})
	})
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL, APIKey: "k"})
	o := New(c)

	// First call: should hit the server.
	if _, err := o.ListProjects(context.Background(), "ws"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}

	// Second call: should hit the cache (ListProjects always refetches
	// in current implementation, but GetProjectByIdentifier uses the
	// cache after a list).
	if _, err := o.ListProjects(context.Background(), "ws"); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 server calls (ListProjects refetches), got %d", callCount)
	}

	// Third call: should use the cache (only 2 server calls total).
	p, err := o.GetProjectByIdentifier(context.Background(), "ws", "SAGA")
	if err != nil {
		t.Fatalf("cached call: %v", err)
	}
	if p.ID != "p1" {
		t.Errorf("expected project p1, got %s", p.ID)
	}
	if callCount != 2 {
		t.Errorf("expected cache hit (2 server calls), got %d", callCount)
	}
}

func TestOps_GetProjectByIdentifier_NotFound(t *testing.T) {
	srv := mockPlaneServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.ListResponse[map[string]any]{})
	})
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL, APIKey: "k"})
	o := New(c)

	_, err := o.GetProjectByIdentifier(context.Background(), "ws", "MISSING")
	if err == nil {
		t.Fatal("expected error for missing project")
	}
	if !strings.Contains(err.Error(), "MISSING") {
		t.Errorf("expected error to mention MISSING, got: %v", err)
	}
}

func TestOps_GetWorkItemBySequence(t *testing.T) {
	srv := mockPlaneServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/v1/workspaces/ws/work-items/SAGA-5/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "wi-1",
			"sequence_id":  5,
			"name":         "SAGA-005",
			"project_id":   "p1",
			"workspace_id": "ws",
		})
	})
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL, APIKey: "k"})
	o := New(c)
	wi, err := o.GetWorkItemBySequence(context.Background(), "ws", "SAGA", "5")
	if err != nil {
		t.Fatalf("GetWorkItemBySequence: %v", err)
	}
	if wi.SequenceID != 5 {
		t.Errorf("expected sequence_id=5, got %d", wi.SequenceID)
	}
	if wi.Name != "SAGA-005" {
		t.Errorf("expected name=SAGA-005, got %s", wi.Name)
	}
}

func TestOps_UpdateWorkItemState_LooksUpStateID(t *testing.T) {
	stateCalls := 0
	updateCalls := 0
	srv := mockPlaneServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/states/"):
			stateCalls++
			_ = json.NewEncoder(w).Encode(client.ListResponse[models.State]{
				Results: []models.State{
					{ID: "state-backlog", Name: "Backlog", Group: "backlog"},
					{ID: "state-in-review", Name: "In Review", Group: "started"},
				},
			})
		case strings.Contains(r.URL.Path, "/work-items/wi-1/"):
			updateCalls++
			if r.Method != "PATCH" {
				t.Errorf("expected PATCH, got %s", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"state":"state-in-review"`) {
				t.Errorf("expected state=state-in-review in body, got: %s", string(body))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "wi-1", "state": "state-in-review"})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	})
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL, APIKey: "k"})
	o := New(c)
	wi, err := o.UpdateWorkItemState(context.Background(), "ws", "p1", "wi-1", "In Review")
	if err != nil {
		t.Fatalf("UpdateWorkItemState: %v", err)
	}
	if wi.State != "state-in-review" {
		t.Errorf("expected state=state-in-review, got %s", wi.State)
	}
	if stateCalls != 1 {
		t.Errorf("expected 1 state call, got %d", stateCalls)
	}
	if updateCalls != 1 {
		t.Errorf("expected 1 update call, got %d", updateCalls)
	}
}

func TestOps_UpdateWorkItemState_StateNotFound(t *testing.T) {
	srv := mockPlaneServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.ListResponse[models.State]{})
	})
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL, APIKey: "k"})
	o := New(c)
	_, err := o.UpdateWorkItemState(context.Background(), "ws", "p1", "wi-1", "NonExistent")
	if err == nil {
		t.Fatal("expected error for missing state")
	}
	if !strings.Contains(err.Error(), "NonExistent") {
		t.Errorf("expected error to mention NonExistent, got: %v", err)
	}
}

func TestOps_AddComment(t *testing.T) {
	srv := mockPlaneServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/comments/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		// Body is JSON-encoded: {"comment_html":"<p>Hello</p>"}
		// so the < and > are JSON-escaped to \u003c and \u003e.
		if !strings.Contains(string(body), `\u003cp\u003eHello\u003c/p\u003e`) {
			t.Errorf("expected comment HTML in JSON body, got: %s", string(body))
		}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "c-1"})
	})
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL, APIKey: "k"})
	o := New(c)
	c2, err := o.AddComment(context.Background(), "ws", "p1", "wi-1", "<p>Hello</p>")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if c2.ID != "c-1" {
		t.Errorf("expected id=c-1, got %s", c2.ID)
	}
}

func TestOps_ListStates(t *testing.T) {
	srv := mockPlaneServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.ListResponse[models.State]{
			Results: []models.State{
				{ID: "s1", Name: "Backlog", Group: "backlog"},
				{ID: "s2", Name: "In Progress", Group: "started"},
			},
		})
	})
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL, APIKey: "k"})
	o := New(c)
	states, err := o.ListStates(context.Background(), "ws", "p1")
	if err != nil {
		t.Fatalf("ListStates: %v", err)
	}
	if len(states) != 2 {
		t.Errorf("expected 2 states, got %d", len(states))
	}
	if states[0].Name != "Backlog" {
		t.Errorf("expected first state=Backlog, got %s", states[0].Name)
	}
}

func TestOps_ListWorkItems_PassesQueryParams(t *testing.T) {
	var gotQuery url.Values
	srv := mockPlaneServer(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.ListResponse[map[string]any]{})
	})
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL, APIKey: "k"})
	o := New(c)
	q := url.Values{}
	q.Set("per_page", "50")
	if _, err := o.ListWorkItems(context.Background(), "ws", "p1", q); err != nil {
		t.Fatalf("ListWorkItems: %v", err)
	}
	if gotQuery.Get("per_page") != "50" {
		t.Errorf("expected per_page=50, got %q", gotQuery.Get("per_page"))
	}
}

func TestOps_ClearCache(t *testing.T) {
	srv := mockPlaneServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.ListResponse[map[string]any]{
			Results: []map[string]any{{"id": "p1", "identifier": "P1"}},
		})
	})
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL, APIKey: "k"})
	o := New(c)
	if _, err := o.ListProjects(context.Background(), "ws"); err != nil {
		t.Fatal(err)
	}
	if o.CacheStats() == 0 {
		t.Fatal("expected cache to be populated")
	}
	o.ClearCache()
	if o.CacheStats() != 0 {
		t.Errorf("expected cache to be cleared, got %d", o.CacheStats())
	}
}

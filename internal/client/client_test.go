package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClient_Do_GET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "test-key" {
			t.Errorf("expected X-API-Key=test-key, got %q", got)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workspaces/ws/projects/p1/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("per_page") != "50" {
			t.Errorf("expected per_page=50, got %q", r.URL.Query().Get("per_page"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"id":"p1","name":"SAGA"}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "test-key"})
	var out map[string]any
	_, code, err := c.Do(context.Background(), "GET", "workspaces/ws/projects/p1/",
		PerPage(50), nil, &out)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if code != 200 {
		t.Errorf("expected status 200, got %d", code)
	}
	if out["id"] != "p1" {
		t.Errorf("expected id=p1, got %v", out["id"])
	}
}

func TestClient_Do_POST(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected Content-Type=application/json, got %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(201)
		_, _ = io.WriteString(w, `{"id":"new","name":"created"}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "test-key"})
	body := map[string]any{"name": "new"}
	var out map[string]any
	_, code, err := c.Do(context.Background(), "POST", "workspaces/ws/projects/",
		nil, body, &out)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if code != 201 {
		t.Errorf("expected status 201, got %d", code)
	}
	if !strings.Contains(receivedBody, `"name":"new"`) {
		t.Errorf("expected body to contain name=new, got %q", receivedBody)
	}
}

func TestClient_Do_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"error":"not found"}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "test-key"})
	_, code, err := c.Do(context.Background(), "GET", "workspaces/ws/projects/missing/",
		nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if code != 404 {
		t.Errorf("expected status 404, got %d", code)
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected APIError.StatusCode=404, got %d", apiErr.StatusCode)
	}
}

func TestClient_Do_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `not-json`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "test-key"})
	var out map[string]any
	_, _, err := c.Do(context.Background(), "GET", "anything", nil, nil, &out)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestClient_Do_QueryEncoding(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "test-key"})
	q := url.Values{}
	q.Set("per_page", "100")
	q.Set("cursor", "100:1:0")
	_, _, err := c.Do(context.Background(), "GET", "x", q, nil, nil)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if gotQuery.Get("per_page") != "100" {
		t.Errorf("expected per_page=100, got %q", gotQuery.Get("per_page"))
	}
	if gotQuery.Get("cursor") != "100:1:0" {
		t.Errorf("expected cursor=100:1:0, got %q", gotQuery.Get("cursor"))
	}
}

func TestClient_Do_OutPointer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "x"})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "k"})
	// No out pointer: should not error even though there's a body
	_, _, err := c.Do(context.Background(), "GET", "x", nil, nil, nil)
	if err != nil {
		t.Errorf("expected no error with nil out, got: %v", err)
	}
}

func TestPerPage(t *testing.T) {
	v := PerPage(25)
	if v.Get("per_page") != "25" {
		t.Errorf("expected per_page=25, got %s", v.Get("per_page"))
	}
}

func TestPerPageWithCursor(t *testing.T) {
	v := PerPageWithCursor(50, "abc:1:0")
	if v.Get("per_page") != "50" {
		t.Errorf("expected per_page=50, got %s", v.Get("per_page"))
	}
	if v.Get("cursor") != "abc:1:0" {
		t.Errorf("expected cursor=abc:1:0, got %s", v.Get("cursor"))
	}

	v2 := PerPageWithCursor(50, "")
	if v2.Get("cursor") != "" {
		t.Errorf("expected empty cursor to be omitted, got %q", v2.Get("cursor"))
	}
}

func TestNew_Defaults(t *testing.T) {
	c := New(Config{APIKey: "k"})
	if c.BaseURL != "https://api.plane.so" {
		t.Errorf("expected default BaseURL, got %s", c.BaseURL)
	}
	if c.CacheTTL == 0 {
		t.Error("expected non-zero default CacheTTL")
	}
}

func TestNew_CustomBaseURL(t *testing.T) {
	c := New(Config{BaseURL: "https://plane.example.com/", APIKey: "k"})
	if c.BaseURL != "https://plane.example.com" {
		t.Errorf("expected trailing slash stripped, got %s", c.BaseURL)
	}
}

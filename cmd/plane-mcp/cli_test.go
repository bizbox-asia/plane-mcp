package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// ---- run() entry point ----

func TestRun_VersionFlag(t *testing.T) {
	errOut := &bytes.Buffer{}
	code := run([]string{"-version"}, errOut)
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(errOut.String(), "plane-mcp") {
		t.Errorf("expected version banner, got: %s", errOut.String())
	}
}

func TestRun_HelpCommand(t *testing.T) {
	errOut := &bytes.Buffer{}
	code := run([]string{"help"}, errOut)
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	out := errOut.String()
	if !strings.Contains(out, "Commands:") {
		t.Errorf("expected commands section, got: %s", out)
	}
	if !strings.Contains(out, "projects") {
		t.Errorf("expected 'projects' in help, got: %s", out)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "test-key")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_API_HOST_URL", "http://localhost:1")
	errOut := &bytes.Buffer{}
	code := run([]string{"bogus"}, errOut)
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Errorf("expected 'unknown command' in output, got: %s", errOut.String())
	}
}

func TestRun_TwoArgFlagStyle(t *testing.T) {
	// Verify that "-api-key k item SAGA 5" is parsed correctly:
	// the value "k" is consumed as the flag's value, and "item" is
	// the subcommand.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "wi-5", "name": "SAGA-5"})
	}))
	defer srv.Close()

	t.Setenv("PLANE_API_KEY", "")
	t.Setenv("PLANE_WORKSPACE_SLUG", "")
	t.Setenv("PLANE_API_HOST_URL", srv.URL)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	errOut := &bytes.Buffer{}
	code := run([]string{"-api-key", "k", "-workspace", "ws", "item", "SAGA", "5"}, errOut)
	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)

	if code != 0 {
		t.Errorf("expected exit 0, got %d; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(string(out), "SAGA-5") {
		t.Errorf("expected SAGA-5 in output, got: %s", string(out))
	}
}

func TestRun_MissingAPIKey(t *testing.T) {
	// Unset env so we know it's the flag check that fires.
	t.Setenv("PLANE_API_KEY", "")
	t.Setenv("PLANE_WORKSPACE_SLUG", "")
	errOut := &bytes.Buffer{}
	code := run([]string{"projects"}, errOut)
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
	if !strings.Contains(errOut.String(), "PLANE_API_KEY is required") {
		t.Errorf("expected missing-key error, got: %s", errOut.String())
	}
}

// ---- subcommand dispatch (with mock Plane server) ----

func TestRun_Projects_JSONOutput(t *testing.T) {
	// Set up mock that returns a paginated project list.
	projectsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"id": "p1", "identifier": "SAGA", "name": "Saga orchestrator"},
				{"id": "p2", "identifier": "KERNEL", "name": "Kernel"},
			},
		})
	}))
	defer projectsSrv.Close()

	t.Setenv("PLANE_API_KEY", "test-key")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_API_HOST_URL", projectsSrv.URL)

	// Capture stdout by redirecting os.Stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	errOut := &bytes.Buffer{}
	code := run([]string{"-format=json", "projects"}, errOut)

	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)

	if code != 0 {
		t.Errorf("expected exit 0, got %d; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(string(out), "SAGA") {
		t.Errorf("expected SAGA in output, got: %s", string(out))
	}
	if !strings.Contains(string(out), "KERNEL") {
		t.Errorf("expected KERNEL in output, got: %s", string(out))
	}
}

func TestRun_Item_RequiresArgs(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "test-key")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_API_HOST_URL", "http://localhost:1")

	errOut := &bytes.Buffer{}
	code := run([]string{"item"}, errOut)
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "missing argument") {
		t.Errorf("expected 'missing argument', got: %s", errOut.String())
	}
}

func TestRun_Update_RequiresAtLeastOneFlag(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "test-key")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_API_HOST_URL", "http://localhost:1")

	errOut := &bytes.Buffer{}
	code := run([]string{"update", "SAGA", "5"}, errOut)
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "at least one field flag") {
		t.Errorf("expected flag-required error, got: %s", errOut.String())
	}
}

// ---- formatters ----

func TestWriter_WriteJSON(t *testing.T) {
	var buf bytes.Buffer
	w := newWriter("json", &buf)
	code := w.writeJSON(map[string]string{"hello": "world"})
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(buf.String(), `"hello"`) {
		t.Errorf("expected json output, got: %s", buf.String())
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
	if got := truncate("hello world", 5); got != "hell…" {
		t.Errorf("expected 'hell…', got %q", got)
	}
	if got := truncate("  spaces  ", 100); got != "spaces" {
		t.Errorf("expected trimmed 'spaces', got %q", got)
	}
}

func TestEscapeHTML(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"<b>x</b>", "&lt;b&gt;x&lt;/b&gt;"},
		{"a & b", "a &amp; b"},
		{`"q"`, "&quot;q&quot;"},
	}
	for _, c := range cases {
		if got := escapeHTML(c.in); got != c.want {
			t.Errorf("escapeHTML(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

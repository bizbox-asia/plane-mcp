package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// writer formats CLI output as either human-readable text or
// machine-readable JSON, depending on cfg.Format. For text, output
// is line-oriented and goes to os.Stdout. For JSON, the entire
// payload is pretty-printed as a single document.
//
// The writer is intentionally minimal: no progress bars, no colors.
// It exists so the same code path can serve both interactive use
// ("plane-mcp projects") and scripting ("plane-mcp -format=json
// items SAGA | jq ...").
type writer struct {
	out    io.Writer
	format string
	// pretty toggles indented JSON. Always on for CLI.
	pretty bool
}

func newWriter(format string, out io.Writer) *writer {
	return &writer{out: out, format: format, pretty: true}
}

func (w *writer) writeJSON(v any) int {
	enc := json.NewEncoder(w.out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "plane-mcp: encode json: %v\n", err)
		return 1
	}
	return 0
}

// close is a no-op for now; reserved for buffered writers in the future.
func (w *writer) close() {}

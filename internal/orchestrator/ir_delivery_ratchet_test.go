package orchestrator

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIRDeliveryHotFileLineRatchet(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path     string
		maxLines int
	}{
		{path: filepath.Join("..", "types", "evidence_closure.go"), maxLines: 2630},
		{path: "scheduler.go", maxLines: 743},
		// Tightened 9395→9260 after WRITEFIX-1 split the apply/verify
		// wording single-point into write_verify_render.go (which gets
		// its own budget below) — the freed budget must not silently
		// grow back into the god-file.
		{path: "orchestrator.go", maxLines: 9260},
		{path: "write_verify_render.go", maxLines: 420},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read %s: %v", tc.path, err)
			}
			lines := bytes.Count(data, []byte{'\n'})
			if len(data) > 0 && data[len(data)-1] != '\n' {
				lines++
			}
			if lines > tc.maxLines {
				t.Fatalf("%s has %d lines; IR delivery ratchet allows at most %d. Split concern-specific code or update the delivery ledger before expanding this budget.", tc.path, lines, tc.maxLines)
			}
		})
	}
}

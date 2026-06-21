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
		{path: filepath.Join("..", "types", "evidence_closure.go"), maxLines: 2774},
		{path: "scheduler.go", maxLines: 945},
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
				t.Fatalf("%s has %d lines; PRD ratchet allows at most %d", tc.path, lines, tc.maxLines)
			}
		})
	}
}

package agent

import (
	"os"
	"strings"
	"testing"
)

func TestRuntimeHintsDoNotExposeMidLoopMechanismLabel(t *testing.T) {
	for _, path := range []string{
		"explorer.go",
		"../tool/emit_evidence.go",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(src), "MID-LOOP CHECK:") {
			t.Fatalf("%s contains LLM-facing MID-LOOP CHECK label; use a neutral progress label instead", path)
		}
	}
}

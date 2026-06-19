package tool

import (
	"encoding/json"
	"testing"
)

func TestStaticSchemaCacheReturnsClones(t *testing.T) {
	tests := []struct {
		name string
		raw  func() json.RawMessage
	}{
		{name: "emit_evidence", raw: (&EmitEvidence{}).Parameters},
		{name: "emit_investigation_complete", raw: (&EmitInvestigationComplete{}).Parameters},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := tt.raw()
			if !json.Valid(first) {
				t.Fatalf("first schema is not valid JSON")
			}
			if len(first) == 0 {
				t.Fatalf("first schema is empty")
			}
			first[0] = 'x'
			second := tt.raw()
			if !json.Valid(second) {
				t.Fatalf("cached schema was mutated by caller; second=%q", string(second[:min(16, len(second))]))
			}
			if len(second) == 0 || second[0] != '{' {
				t.Fatalf("unexpected cached schema prefix: %q", string(second[:min(16, len(second))]))
			}
		})
	}
}

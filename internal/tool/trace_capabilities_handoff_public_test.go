package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceCapabilitiesPublicDocumentationHandoff(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	for _, params := range []string{`{}`, `{"view":"window_stats","detail":true}`, `{"detail":true}`} {
		t.Run(params, func(t *testing.T) {
			result, err := r.Execute(nil, "trace_capabilities", json.RawMessage(params))
			if err != nil || !result.Success {
				t.Fatalf("HARNESS: catalog call failed: %v", err)
			}
			result = types.AttachToolHandoffCarrier(result)
			wire, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Handoff *struct {
					Documentation *struct {
						Version     int             `json:"version"`
						Schema      string          `json:"schema"`
						ContentHash string          `json:"content_hash"`
						Content     json.RawMessage `json:"content"`
					} `json:"documentation"`
				} `json:"handoff"`
			}
			if err := json.Unmarshal(wire, &got); err != nil {
				t.Fatal(err)
			}
			if got.Handoff == nil || got.Handoff.Documentation == nil {
				t.Fatal("successful static catalog has no typed cross-stage documentation")
			}
			doc := got.Handoff.Documentation
			if doc.Version != 1 || doc.Schema != "trace_capabilities/v1" || doc.ContentHash == "" || string(doc.Content) != result.Summary {
				t.Fatal("handoff omitted or altered the complete catalog")
			}
			if len(result.Observations) != 0 || len(result.Handoff.AcceptedEvidence) != 0 || len(result.Handoff.ObservationRefs) != 0 {
				t.Fatal("static documentation acquired source/runtime evidence")
			}
		})
	}
}

package context

import (
	stdctx "context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLogSourceBindingPromptKeepsVerifiedSourceWithoutRestoringJSONAuthority(t *testing.T) {
	catalog, err := loginput.Prepare(stdctx.Background(), []loginput.Input{{Name: "original/session.log", Data: []byte("header\nobserved failure\n")}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	locations, err := catalog.Locate(stdctx.Background(), []loginput.ExcerptSelector{{Text: "observed failure"}})
	if err != nil || !locations[0].Verified() {
		t.Fatalf("real source lookup failed: %v", err)
	}
	binding := types.NewLogSourceBinding(locations[0])
	for _, restored := range []bool{false, true} {
		value := binding.Clone()
		if restored {
			raw, _ := json.Marshal(value)
			value = &types.LogSourceBinding{}
			if err := json.Unmarshal(raw, value); err != nil {
				t.Fatal(err)
			}
		}
		bundle := &types.LogBundle{Errors: []types.LogError{{Type: "Error", Message: "observed failure", SourceBinding: value}},
			Observations: []types.LogObservation{{Kind: "error", Summary: "interpretation", Evidence: "observed failure", LineStart: 99, SourceBinding: value}}}
		text := formatLogTriageStructured(bundle, nil)
		if restored {
			if !strings.Contains(text, "log_location=unverified_snapshot") || strings.Contains(text, "decoded_lines=2-2") || strings.Contains(text, "log_line=99") {
				t.Fatalf("restored snapshot upgraded coordinates: %s", text)
			}
		} else if strings.Count(text, binding.LocationLabel()) != 2 {
			t.Fatalf("error/observation lost source-bound location: %s", text)
		}
	}
}

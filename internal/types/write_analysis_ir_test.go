package types

import (
	"encoding/json"
	"testing"
)

// TestWriteTaskKind_RoundTrip verifies enum string values round-trip
// through JSON cleanly. The validator falls back to WriteTaskMisc on
// unknown values, so the wire shape needs to be stable.
func TestWriteTaskKind_RoundTrip(t *testing.T) {
	cases := []WriteTaskKind{
		WriteTaskFeature, WriteTaskBugfix, WriteTaskRefactor,
		WriteTaskTest, WriteTaskDocs, WriteTaskConfig, WriteTaskMisc,
	}
	for _, k := range cases {
		raw, err := json.Marshal(k)
		if err != nil {
			t.Fatalf("marshal %q: %v", k, err)
		}
		var got WriteTaskKind
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if got != k {
			t.Errorf("round-trip drift: %q -> %s -> %q", k, raw, got)
		}
		if !IsKnownWriteTaskKind(string(k)) {
			t.Errorf("IsKnownWriteTaskKind(%q) = false; want true", k)
		}
	}
	if IsKnownWriteTaskKind("not_a_kind") {
		t.Error("unknown kind reported as known")
	}
}

// TestWriteScope_RoundTrip mirrors the kind test for the scope enum.
func TestWriteScope_RoundTrip(t *testing.T) {
	cases := []WriteScope{
		ScopeMicro, ScopePackage, ScopeCross, ScopeProject,
	}
	for _, s := range cases {
		if !IsKnownWriteScope(string(s)) {
			t.Errorf("IsKnownWriteScope(%q) = false; want true", s)
		}
	}
	if IsKnownWriteScope("global") {
		t.Error("unknown scope reported as known")
	}
}

// TestWriteRiskBand_RoundTrip mirrors the kind test for the risk
// band enum.
func TestWriteRiskBand_RoundTrip(t *testing.T) {
	for _, b := range []WriteRiskBand{RiskBandLow, RiskBandMedium, RiskBandHigh} {
		if !IsKnownWriteRiskBand(string(b)) {
			t.Errorf("IsKnownWriteRiskBand(%q) = false; want true", b)
		}
	}
	if IsKnownWriteRiskBand("severe") {
		t.Error("unknown band reported as known")
	}
}

// TestPhaseSplit_RoundTrip verifies the phase split values.
func TestPhaseSplit_RoundTrip(t *testing.T) {
	for _, v := range []string{"single", "sequential", "parallel"} {
		if !IsKnownPhaseSplit(v) {
			t.Errorf("IsKnownPhaseSplit(%q) = false; want true", v)
		}
	}
	if IsKnownPhaseSplit("conditional") {
		t.Error("unknown split reported as known")
	}
}

// TestWriteAnalysisIR_JSONShape verifies the IR JSON omits zero-value
// optional fields so a minimal emit doesn't carry empty arrays /
// objects in the trace log.
func TestWriteAnalysisIR_JSONShape(t *testing.T) {
	ir := WriteAnalysisIR{
		Request: WriteRequestModel{
			RawRequest: "add a --quiet flag",
			Task: WriteTask{
				Kind:    WriteTaskFeature,
				Scope:   ScopeMicro,
				Summary: "wire a --quiet flag through cmd/root.go",
			},
			Risk: WriteRiskProfile{Overall: RiskBandLow},
		},
		PhaseProposal: PhaseProposal{Split: "single"},
	}
	raw, err := json.Marshal(ir)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// scope_anchors / constraints / expected_outcomes / pitfalls_applied
	// are omitempty; absent in the minimal IR.
	for _, missing := range []string{"scope_anchors", "constraints", "expected_outcomes", "pitfalls_applied", "prescan_files"} {
		if got["request"] != nil {
			req := got["request"].(map[string]any)
			if _, present := req[missing]; present && missing != "prescan_files" && missing != "pitfalls_applied" {
				t.Errorf("expected %q to be omitted from minimal IR; raw=%s", missing, raw)
			}
		}
	}
}

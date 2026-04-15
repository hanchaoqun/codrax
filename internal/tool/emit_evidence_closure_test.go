package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEmitEvidence_AllowedKindsMatchCanonical asserts the whitelist
// the emit_evidence tool uses at runtime is exactly the set of
// LLM-emittable EvidenceKinds declared in internal/types. Before this
// was a derived list, the whitelist was a hand-written map that had
// already drifted once: the canonical list had 11 kinds while the
// tool's accept map had 6, and there was nothing forcing the two to
// agree. Keep this test as a compile-time guard.
func TestEmitEvidence_AllowedKindsMatchCanonical(t *testing.T) {
	want := make(map[string]bool)
	for _, k := range types.LLMEmittableEvidenceKinds() {
		want[string(k)] = true
	}
	got := make(map[string]bool, len(emitEvidenceAllowedKinds))
	for k := range emitEvidenceAllowedKinds {
		got[k] = true
	}
	if len(got) != len(want) {
		t.Errorf("emitEvidenceAllowedKinds has %d entries, LLMEmittableEvidenceKinds has %d", len(got), len(want))
	}
	for k := range want {
		if !got[k] {
			t.Errorf("emitEvidenceAllowedKinds missing canonical kind %q", k)
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("emitEvidenceAllowedKinds has stray kind %q not in LLMEmittableEvidenceKinds", k)
		}
	}
}

// TestEmitEvidence_SchemaEnumMatchesCanonical asserts the JSON schema
// enum rendered into the tool Parameters() output is exactly the
// LLMEmittableEvidenceKinds set, in canonical order. The schema is
// what the LLM actually sees — a mismatch between the schema enum
// and the runtime whitelist would either reject valid kinds at the
// validator or silently accept kinds the schema "promised" were
// forbidden.
func TestEmitEvidence_SchemaEnumMatchesCanonical(t *testing.T) {
	t2 := &EmitEvidence{}
	var schema map[string]any
	if err := json.Unmarshal(t2.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() did not return valid JSON: %v", err)
	}
	props := schema["properties"].(map[string]any)
	items := props["items"].(map[string]any)
	itemSchema := items["items"].(map[string]any)
	itemProps := itemSchema["properties"].(map[string]any)
	kindField := itemProps["kind"].(map[string]any)
	enumRaw, ok := kindField["enum"].([]any)
	if !ok {
		t.Fatalf("kind.enum is not an array: %T", kindField["enum"])
	}
	got := make([]string, len(enumRaw))
	for i, v := range enumRaw {
		got[i] = v.(string)
	}
	canonical := types.LLMEmittableEvidenceKinds()
	if len(got) != len(canonical) {
		t.Fatalf("schema enum has %d entries, canonical has %d: %v vs %v", len(got), len(canonical), got, canonical)
	}
	for i, k := range canonical {
		if got[i] != string(k) {
			t.Errorf("schema enum[%d] = %q, want %q", i, got[i], string(k))
		}
	}
}

// TestAnalysisQuestionKindValues_MatchesRequirementKinds asserts the
// analysis-skill's question_kind enum values are exactly
// types.AllRequirementKinds() plus the "unknown" sentinel, in that
// order. This binds the analyzer-facing prompt enum to the ERM
// consumer-side enum through the type system instead of through two
// parallel hand-written tables that can silently drift.
func TestAnalysisQuestionKindValues_MatchesRequirementKinds(t *testing.T) {
	got := skill.AnalysisQuestionKindValues()
	canonical := types.AllRequirementKinds()
	// Expected: one entry per RequirementKind, in canonical order,
	// followed by "unknown".
	want := make([]string, 0, len(canonical)+1)
	for _, k := range canonical {
		want = append(want, string(k))
	}
	want = append(want, "unknown")
	if len(got) != len(want) {
		t.Fatalf("AnalysisQuestionKindValues length %d, want %d: got=%v want=%v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AnalysisQuestionKindValues[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

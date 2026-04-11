package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestParseEvidenceItems(t *testing.T) {
	notes := []string{
		"## Evidence from [internal/agent/explorer.go]\n" +
			"- [DIRECT] `Name()` line 12: returns \"explorer\"\n" +
			"- [CONDITIONAL] `Register()` line 20: dispatches IF `Enabled()` == true\n" +
			"- [RELATIONSHIP] `Register` -> `NewExplorer`: calls constructor\n" +
			"- [ABSENT] Expected `LegacyHandler` in registry but NOT found",
	}

	items := parseEvidenceItems(notes, "explorer.llm")
	if len(items) != 4 {
		t.Fatalf("parseEvidenceItems count = %d, want 4", len(items))
	}

	if items[0].Source != "internal/agent/explorer.go" {
		t.Fatalf("first evidence source = %q", items[0].Source)
	}

	var sawConditional bool
	var sawRelationship bool
	var sawAbsent bool
	for _, item := range items {
		switch item.Kind {
		case types.EvidenceConditional:
			sawConditional = item.Condition != ""
		case types.EvidenceRelationship:
			sawRelationship = item.Subject != "" && item.Object != ""
		case types.EvidenceAbsent:
			sawAbsent = item.Predicate == "absent"
		}
	}
	if !sawConditional {
		t.Fatal("expected parsed conditional evidence with condition text")
	}
	if !sawRelationship {
		t.Fatal("expected parsed relationship evidence with subject/object")
	}
	if !sawAbsent {
		t.Fatal("expected parsed absent evidence")
	}
}

func TestMergeEvidenceItemsDedupesByStableID(t *testing.T) {
	a := types.EvidenceItem{
		Kind:       types.EvidenceConcrete,
		Subject:    "Handler.Name",
		Predicate:  "returns",
		Object:     "\"explorer\"",
		Source:     "internal/agent/explorer.go",
		LineStart:  12,
		LineEnd:    12,
		Confidence: 0.7,
	}
	a.ID = types.StableEvidenceID(a.Kind, a.Subject, a.Predicate, a.Object, a.Condition, a.Source, a.LineStart, a.LineEnd)
	b := a
	b.Confidence = 0.9
	b.EvidenceRef = "blob://trace/result.txt"

	merged := mergeEvidenceItems([]types.EvidenceItem{a}, []types.EvidenceItem{b})
	if len(merged) != 1 {
		t.Fatalf("mergeEvidenceItems count = %d, want 1", len(merged))
	}
	if merged[0].Confidence != 0.9 {
		t.Fatalf("merged confidence = %.2f, want 0.9", merged[0].Confidence)
	}
	if merged[0].EvidenceRef != "blob://trace/result.txt" {
		t.Fatalf("merged evidence ref = %q", merged[0].EvidenceRef)
	}
}

func TestBuildCrossReferenceMapFromEvidenceUsesFlowFindings(t *testing.T) {
	findings := []types.FlowFindingDigest{
		{
			ID:         "flow-1",
			Path:       []string{"config.handlers.explorer", "NewExplorer", "Register"},
			Conditions: []string{"enabled == true"},
			Confidence: 0.8,
		},
	}

	out := buildCrossReferenceMapFromEvidence(nil, findings)
	if out == "" {
		t.Fatal("expected cross-reference output from flow findings")
	}
	if want := "config.handlers.explorer -> NewExplorer -> Register"; !containsString(out, want) {
		t.Fatalf("cross-reference output missing %q:\n%s", want, out)
	}
}

func containsString(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

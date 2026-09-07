package types

import (
	"reflect"
	"testing"
)

// r1026 (20260906-195101): the model's ev-60b09ad3717d93d6 and
// parser's ev-d20fbf28abbe9af6 both describe registry.py:31. The actual
// parser emits the canonical REGISTRY object, not the indexed expression.
func b1585LookupEvidence() (EvidenceItem, EvidenceItem) {
	parser := dynamicSelectorTestEvidence("ev-d20fbf28abbe9af6", EvidenceConcrete, AnchorAssignment,
		"cls", "REGISTRY", "resolve", "cls = REGISTRY[name]")
	parser.Source, parser.LineStart, parser.LineEnd = "pipeline/registry.py", 31, 31
	parser.Predicate = "assigns"
	parser.Producer = EvidenceProducerRepoMapDynamicSelectorAssignment
	model := parser
	model.ID, model.Kind, model.Predicate, model.Producer = "ev-60b09ad3717d93d6", EvidenceRelationship, "maps", ""
	return parser, model
}

func b1585WithLookups(lookups ...EvidenceItem) []EvidenceItem {
	base := dynamicSelectorCompleteEvidence()
	return append(append(base[:2:2], base[3:]...), lookups...)
}

func TestB1585LookupCorroborationRealR1026Carriers(t *testing.T) {
	parser, model := b1585LookupEvidence()
	for _, item := range []EvidenceItem{parser, model} {
		if !AssignmentEvidenceEndpointsMatch(item) {
			t.Fatalf("real row must first satisfy the existing source tuple gate: %+v", item)
		}
		got := CompileDynamicSelectorResolutionPaths(b1585WithLookups(item), "run_pipeline")
		if len(got.Candidates) != 1 || len(got.Rejected) != 0 {
			t.Fatalf("each carrier alone must complete the unchanged selector path: %+v", got)
		}
	}
	fullRHS := model
	fullRHS.Object = "REGISTRY[name]"
	for _, tc := range []struct {
		name string
		rows []EvidenceItem
	}{
		{"real_parser_then_model", []EvidenceItem{parser, model}},
		{"real_model_then_parser", []EvidenceItem{model, parser}},
		{"legal_complete_rhs_surface", []EvidenceItem{parser, fullRHS}},
		{"legal_complete_rhs_surface_reverse", []EvidenceItem{fullRHS, parser}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidence := b1585WithLookups(tc.rows...)
			before := append([]EvidenceItem(nil), evidence...)
			got := CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
			if len(got.Candidates) != 1 || len(got.Rejected) != 0 {
				t.Fatalf("same exact assignment occurrence is corroboration, not ambiguity: %+v", got)
			}
			path := got.Candidates[0]
			if path.Status != DynamicSelectorResolutionCandidateOnly || len(path.Hops) != 6 ||
				path.Hops[4].EvidenceID != tc.rows[0].ID || path.Hops[4].Predicate != tc.rows[0].Predicate ||
				path.Hops[4].RelationKind != DiagramRelAssignment || path.Hops[4].ClaimForm != ClaimAssignmentFact ||
				path.Hops[4].FromIdentity != "REGISTRY" || path.Hops[4].ToIdentity != "cls" {
				t.Fatalf("deduplication must retain first evidence and assignment-only authority: %+v", path)
			}
			if !reflect.DeepEqual(before, evidence) {
				t.Fatal("compiler mutated source evidence")
			}
		})
	}
}

func TestB1585LookupCorroborationPreservesExactConflictAxes(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*EvidenceItem)
	}{
		{"different_file", func(e *EvidenceItem) { e.Source = "other/registry.py" }},
		{"different_start", func(e *EvidenceItem) { e.LineStart, e.LineEnd = 32, 32 }},
		{"different_end", func(e *EvidenceItem) { e.LineEnd = 32 }},
		{"different_owner", func(e *EvidenceItem) { e.OwnerSymbol = "other.resolve" }},
		{"different_index", func(e *EvidenceItem) { e.Snippet = "cls = REGISTRY[other]" }},
		{"different_index_call", func(e *EvidenceItem) { e.Snippet = "cls = REGISTRY[choose(name)]" }},
		{"different_index_expression", func(e *EvidenceItem) { e.Snippet = "cls = REGISTRY[name + suffix]" }},
		{"different_full_lhs", func(e *EvidenceItem) { e.Snippet = "cls: type = REGISTRY[name]" }},
		{"multiline_trailing_operation", func(e *EvidenceItem) { e.Snippet += "\ncls = REGISTRY[other]" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser, other := b1585LookupEvidence()
			tc.edit(&other)
			if !AssignmentEvidenceEndpointsMatch(other) {
				t.Fatal("negative must reach the exact existing assignment admission gate")
			}
			for _, rows := range [][]EvidenceItem{{parser, other}, {other, parser}} {
				got := CompileDynamicSelectorResolutionPaths(b1585WithLookups(rows...), "run_pipeline")
				if len(got.Candidates) != 0 || len(got.Rejected) != 1 || got.Rejected[0].Reason != DynamicSelectorRejectAmbiguousLookup {
					t.Fatalf("different source operation must remain ambiguous: %+v", got)
				}
			}
		})
	}
}

func TestB1585LookupCorroborationIncompleteSourceKeepsLegacyMultiplicity(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*EvidenceItem)
	}{
		{"no_source", func(e *EvidenceItem) { e.Source = "" }},
		{"no_start", func(e *EvidenceItem) { e.LineStart = 0 }},
		{"no_end", func(e *EvidenceItem) { e.LineEnd = 0 }},
		{"no_owner", func(e *EvidenceItem) { e.OwnerSymbol = "" }},
		{"no_snippet", func(e *EvidenceItem) { e.Snippet = "" }},
		{"multiline", func(e *EvidenceItem) { e.Snippet += "\nreturn cls()" }},
		{"range", func(e *EvidenceItem) { e.LineEnd = 32 }},
		{"uncitable", func(e *EvidenceItem) { e.GroundingStatus = GroundingUngrounded }},
		{"wrong_object", func(e *EvidenceItem) { e.Object = "OTHER" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser, model := b1585LookupEvidence()
			tc.edit(&parser)
			tc.edit(&model)
			// Directly test the deduplication boundary: malformed/non-citable
			// rows are filtered by the compiler, not newly admitted by this key.
			rows := []dynamicSelectorLookupCandidate{
				{item: parser, owner: "resolve", receiver: "cls", container: "REGISTRY"},
				{item: model, owner: "resolve", receiver: "cls", container: "REGISTRY"},
			}
			if got := uniqueDynamicSelectorLookups(rows); len(got) != 2 || distinctDynamicSelectorLookupShapes(got) != 2 {
				t.Fatalf("incomplete operation proof must retain legacy multiplicity: %+v", got)
			}
		})
	}
}

func TestB1585LookupCorroborationDoesNotTrustMatchingCarrierTupleOverSource(t *testing.T) {
	parser, conflicting := b1585LookupEvidence()
	conflicting.Predicate = parser.Predicate
	conflicting.Kind = parser.Kind
	conflicting.Snippet = "cls = REGISTRY[other]"
	// Same raw S/P/O and coordinates do not authorize erasing a different
	// full indexed operation. The old occurrence key cannot tell these apart.
	got := CompileDynamicSelectorResolutionPaths(b1585WithLookups(parser, conflicting), "run_pipeline")
	if len(got.Candidates) != 0 || len(got.Rejected) != 1 || got.Rejected[0].Reason != DynamicSelectorRejectAmbiguousLookup {
		t.Fatalf("source expression conflict must survive matching raw carrier tuple: %+v", got)
	}
}

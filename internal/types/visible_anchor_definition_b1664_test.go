package types

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// These are typed-evidence projection tests, not a native parser or LLM
// replay. They use the production evidence -> support-entry helper followed
// by the public pre-flight guide builder, preserving the original evidence.
func b1664DefinitionEntry(anchor, subject, object, source string, line int) AnswerSupportEntry {
	return answerSupportEntryForEvidence(EvidenceItem{
		ID: fmt.Sprintf("definition-%d", line), Kind: EvidenceDirect, Scope: ScopeLine,
		AnchorKind: AnchorDefinition, AnchorSymbol: anchor, Subject: subject, Object: object,
		OwnerSymbol: "UnrelatedOwner", Source: source, LineStart: line, LineEnd: line,
		GroundingStatus: GroundingGrounded, GroundingTier: TierLineText,
		SurfaceTerms: []string{anchor, subject, object, "OtherParameter", "unproven_alias"},
	}, "model explanation does not confer declaration identity", "parameter and return types can occur on the declaration line")
}

func b1664Whitelist(entries []AnswerSupportEntry) *VisibleAnchorWhitelist {
	return BuildVisibleAnchorWhitelist(&AnswerSupportPlan{Lanes: []AnswerSupportLane{{Kind: SupportLaneCurrentCodePath, Entries: entries}}}, nil)
}

func TestB1664DefinitionProjectionDoesNotMintParameterFunctions(t *testing.T) {
	for _, tc := range []struct {
		name, anchor, subject, object, source, bare string
	}{
		{"rust binding", "_fastlex", "py::_fastlex", "PyModule", "core/src/lib.rs", "_fastlex"},
		{"qualified rust declaration", "py::tokenize_bytes", "Vec", "MergeTable", "core/src/lib.rs", "tokenize_bytes"},
		{"qualified python declaration", "FastTokenizer.tokenize", "str", "list", "bindings/tokenizer.py", "tokenize"},
		{"qualified cpp declaration", "io::Writer::write", "Request", "Result", "src/writer.cpp", "write"},
		{"qualified go declaration", "service.Handle", "Context", "Response", "service.go", "Handle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := b1664DefinitionEntry(tc.anchor, tc.subject, tc.object, tc.source, 17)
			before, _ := json.Marshal(entry)
			got := b1664Whitelist([]AnswerSupportEntry{entry})
			if len(got.Groundable) != 1 {
				t.Fatalf("only the declaration anchor may acquire this line's definition role; got %+v", got.Groundable)
			}
			e := got.Groundable[0]
			if e.Symbol != tc.bare || e.SourceFile != tc.source || e.SourceLine != 17 || e.Kind != VisibleAnchorKindSymbol || e.GroundingTier != TierLineText || e.Origin != "support_lane:"+string(SupportLaneCurrentCodePath) {
				t.Fatalf("declaration identity/source/neutral role changed: %+v", e)
			}
			wantQualified := tc.anchor
			if tc.bare == tc.anchor {
				wantQualified = ""
			}
			if e.QualifiedSymbol != wantQualified {
				t.Fatalf("qualification must be copied from the anchor, not synthesized from subject/owner: %+v", e)
			}
			wantForms := []string{tc.bare}
			if tc.bare != tc.anchor {
				wantForms = append(wantForms, tc.anchor)
			}
			if !reflect.DeepEqual(e.SurfaceForms, wantForms) {
				t.Fatalf("parameter/return/other SurfaceTerms must not become declaration aliases: got %v want %v", e.SurfaceForms, wantForms)
			}
			after, _ := json.Marshal(entry)
			if string(before) != string(after) {
				t.Fatal("guide narrowed the original support evidence instead of only its projection")
			}
		})
	}
}

func TestB1664DefinitionKindDoesNotGuessDeclarationCategory(t *testing.T) {
	for _, label := range []string{"class Widget", "struct Packet", "type Identifier", "enum Mode", "function Apply"} {
		t.Run(label, func(t *testing.T) {
			entry := b1664DefinitionEntry("Widget", "", "", "declarations.src", 7)
			entry.Text, entry.Detail = label, "pretend this is a function; not typed authority"
			got := b1664Whitelist([]AnswerSupportEntry{entry})
			if len(got.Groundable) != 1 || got.Groundable[0].Kind != VisibleAnchorKindSymbol {
				t.Fatalf("AnchorDefinition does not distinguish a function from other declarations: %+v", got.Groundable)
			}
		})
	}
}

func TestB1664MissingDefinitionAnchorDoesNotBorrowSubjectOrObject(t *testing.T) {
	for _, anchor := range []string{"", " \t ", "not a code identity"} {
		entry := b1664DefinitionEntry(anchor, "Owner.method", "ParameterType", "source.ext", 31)
		before, _ := json.Marshal(entry)
		if got := b1664Whitelist([]AnswerSupportEntry{entry}); !got.IsEmpty() {
			t.Fatalf("missing/invalid declaration anchor must not manufacture a definition suggestion: anchor=%q got=%+v", anchor, got.Groundable)
		}
		after, _ := json.Marshal(entry)
		if string(before) != string(after) {
			t.Fatal("original unprojected evidence changed")
		}
	}
}

func TestB1664NonDefinitionProjectionRemainsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		anchorKind AnchorKind
		kind       VisibleAnchorKind
		want       []string
	}{
		{AnchorCall, VisibleAnchorKindCallSite, []string{"Anchor", "Object"}},
		{AnchorReturn, VisibleAnchorKindReturn, []string{"Anchor", "Object"}},
		{AnchorAssignment, VisibleAnchorKindAssignment, []string{"Anchor", "Object"}},
		{AnchorInitializer, VisibleAnchorKindInitializer, []string{"Anchor", "Object"}},
		{AnchorCondition, VisibleAnchorKindCondition, []string{"Anchor", "Object"}},
		{AnchorImport, VisibleAnchorKindImport, []string{"Anchor", "Object"}},
		{AnchorStringLiteral, VisibleAnchorKindStringLiteral, []string{"Anchor", "Object"}},
		{"", VisibleAnchorKindOther, []string{"Anchor", "Object", "Subject"}},
		{AnchorKind("unknown_future_kind"), VisibleAnchorKindOther, []string{"Anchor"}},
	} {
		t.Run(string(tc.anchorKind), func(t *testing.T) {
			entry := AnswerSupportEntry{AnchorKind: tc.anchorKind, AnchorSymbol: "Anchor", Subject: "Subject", Object: "Object", Source: "src.ext", LineStart: 4, GroundingTier: TierSnippetFuzzy, SurfaceTerms: []string{"Anchor", "legacy_alias", "not an identity"}}
			before, _ := json.Marshal(entry)
			got := b1664Whitelist([]AnswerSupportEntry{entry})
			if len(got.Groundable) != len(tc.want) {
				t.Fatalf("non-definition inventory changed: %+v", got.Groundable)
			}
			for i, e := range got.Groundable {
				if e.Symbol != tc.want[i] || e.Kind != tc.kind || e.SourceFile != "src.ext" || e.SourceLine != 4 || e.GroundingTier != TierSnippetFuzzy || !reflect.DeepEqual(e.SurfaceForms, []string{"Anchor", "legacy_alias"}) {
					t.Fatalf("non-definition role/citation/tier/alias changed: %+v", e)
				}
			}
			after, _ := json.Marshal(entry)
			if string(before) != string(after) {
				t.Fatal("original non-definition support changed")
			}
		})
	}
}

func TestB1664DefinitionProjectionPreservesCapOrderAndDedup(t *testing.T) {
	var entries []AnswerSupportEntry
	for i := 0; i < MaxVisibleAnchorWhitelistEntries+9; i++ {
		entry := b1664DefinitionEntry(fmt.Sprintf("Decl%03d", i), "AParameter", "AReturn", "src.ext", i+1)
		entries = append(entries, entry, entry)
	}
	before, _ := json.Marshal(entries)
	view := &AnswerSemanticView{RequiredMechanismAnchors: []AnswerRequiredAnchor{{Text: "RequiredFirst", Kind: ContractTermSymbol}, {Text: "RequiredSecond", Kind: ContractTermSymbol}}}
	got := BuildVisibleAnchorWhitelist(&AnswerSupportPlan{Lanes: []AnswerSupportLane{{Entries: entries}}}, view)
	if len(got.Required) != 2 || got.Required[0].Symbol != "RequiredFirst" || got.Required[1].Symbol != "RequiredSecond" || len(got.Groundable) != MaxVisibleAnchorWhitelistEntries-2 {
		t.Fatalf("required order or total cap changed: %+v", got)
	}
	for i, e := range got.Groundable {
		if e.Symbol != fmt.Sprintf("Decl%03d", i) || e.SourceLine != i+1 {
			t.Fatalf("definition roles must not spend the cap on parameter aliases; sorted/dedup entry %d=%+v", i, e)
		}
	}
	after, _ := json.Marshal(entries)
	if string(before) != string(after) {
		t.Fatal("bounded projection altered the uncapped evidence inventory")
	}
	// Preserve the existing (bare Symbol, exact source path, line) tuple key;
	// this change does not redesign identity or merge different source rows.
	a := b1664DefinitionEntry("owner.Decl", "", "", "A.ext", 9)
	b := a
	b.Source = "a.ext"
	c := a
	c.LineStart = 10
	if got := b1664Whitelist([]AnswerSupportEntry{a, a, b, c}); len(got.Groundable) != 3 {
		t.Fatalf("exact source/line dedup identity changed: %+v", got.Groundable)
	}
}

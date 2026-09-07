package types

import (
	"reflect"
	"testing"
)

func TestB1604SymbolSelectionOriginLifecycleAndCache(t *testing.T) {
	syms := []AnswerSymbol{{Name: "ConsoleSink", File: "include/logx/console_sink.hpp", Line: 8, Kind: KindType}}
	m := NewMutableState("")
	ir := &AnalysisIR{RequestModel: b1604RelationEnumerationRequest()}
	bus := &BusContext{AnalysisIR: ir, Mutable: m, EvidenceItems: b1604EnumerationEvidence()}
	for _, origin := range []AnswerSymbolSelectionOrigin{AnswerSymbolSelectionUnknown, AnswerSymbolSelectionExplicitItems, AnswerSymbolSelectionInventoryMaterialized, "invalid"} {
		before := m.answerSurfaceRevisionValue()
		m.SetEmittedAnswerSymbolsWithOrigin(syms, CompletenessComplete, origin)
		got, claim, source := m.EmittedAnswerSymbolsWithOrigin()
		want := origin
		if origin == "invalid" {
			want = AnswerSymbolSelectionUnknown
		}
		if !reflect.DeepEqual(got, syms) || claim != CompletenessComplete || source != want || m.answerSurfaceRevisionValue() <= before {
			t.Fatalf("atomic source/claim/revision mismatch for %s: %v %s %s", origin, got, claim, source)
		}
		got[0].Name = "outside mutation"
		plan := BuildAnswerSurfacePlanForBusContext(bus)
		if plan.stepBackboneFromAcceptedSymbolSlate != (origin == AnswerSymbolSelectionExplicitItems) {
			t.Fatalf("same slate with changed producer reused stale authority: %s %+v", origin, plan.StepBackbone)
		}
	}
	m.SetEmittedAnswerSymbolsWithOrigin(syms, CompletenessComplete, AnswerSymbolSelectionExplicitItems)
	fork := m.ForkForExploreDispatch()
	_, _, origin := fork.EmittedAnswerSymbolsWithOrigin()
	if origin != AnswerSymbolSelectionExplicitItems {
		t.Fatal("fork lost the matching slate origin")
	}
	fork.SetEmittedAnswerSymbolsWithOrigin([]AnswerSymbol{{Name: "Other", File: "other.go", Line: 9}}, CompletenessLowerBound, AnswerSymbolSelectionInventoryMaterialized)
	m.MergeExploreFork(fork)
	got, claim, origin := m.EmittedAnswerSymbolsWithOrigin()
	if !reflect.DeepEqual(got, syms) || claim != CompletenessComplete || origin != AnswerSymbolSelectionExplicitItems {
		t.Fatal("explore merge imported a child slate/source outside its existing merge domain")
	}
	m.ResetEmittedAnswerSymbols()
	got, claim, origin = m.EmittedAnswerSymbolsWithOrigin()
	if len(got) != 0 || claim != CompletenessUnknown || origin != AnswerSymbolSelectionUnknown {
		t.Fatal("reset retained producer authority")
	}
	m.SetEmittedAnswerSymbols(syms, CompletenessComplete)
	_, _, origin = m.EmittedAnswerSymbolsWithOrigin()
	if origin != AnswerSymbolSelectionUnknown {
		t.Fatal("legacy setter invented producer provenance")
	}
	m.SetEmittedAnswerSymbolsWithOrigin(nil, CompletenessComplete, AnswerSymbolSelectionExplicitItems)
	_, _, origin = m.EmittedAnswerSymbolsWithOrigin()
	if origin != AnswerSymbolSelectionUnknown {
		t.Fatal("empty slate retained member authority")
	}
	var absent *MutableState
	absent.SetEmittedAnswerSymbolsWithOrigin(syms, CompletenessComplete, AnswerSymbolSelectionExplicitItems)
	got, claim, origin = absent.EmittedAnswerSymbolsWithOrigin()
	if len(got) != 0 || claim != CompletenessUnknown || origin != AnswerSymbolSelectionUnknown {
		t.Fatal("nil state acquired authority")
	}
}

func TestB1604AcceptedSlateExactDeclarationBoundary(t *testing.T) {
	plan := &AnswerSurfacePlan{StepBackbone: []StepSurfaceAnchor{{Name: "ConsoleSink", File: "include/logx/console_sink.hpp", Line: 8, Kind: KindType}}, stepBackboneFromAcceptedSymbolSlate: true}
	base := b1604EnumerationEvidence()[0]
	for _, dimension := range []string{"same", "slash syntax", "wrong identity", "same short tail", "case", "path", "path case", "relative prefix", "absolute path", "line", "import", "unknown anchor"} {
		item := base
		switch dimension {
		case "wrong identity":
			item.AnchorSymbol, item.Subject = "FileSink", "FileSink"
		case "same short tail":
			item.AnchorSymbol = "Other.ConsoleSink"
		case "case":
			item.AnchorSymbol = "consolesink"
		case "path":
			item.Source = "other/console_sink.hpp"
		case "path case":
			item.Source = "include/logx/Console_Sink.hpp"
		case "slash syntax":
			item.Source = `include\logx\console_sink.hpp`
		case "relative prefix":
			item.Source = "./include/logx/console_sink.hpp"
		case "absolute path":
			item.Source = "/workspace/include/logx/console_sink.hpp"
		case "line":
			item.LineStart++
		case "import":
			item.AnchorKind = AnchorImport
		case "unknown anchor":
			item.AnchorKind = ""
		}
		if got := enumerationEvidenceMatchesAcceptedSymbolSlate(plan, item); got != (dimension == "same" || dimension == "slash syntax") {
			t.Fatalf("%s matched=%t", dimension, got)
		}
	}
	for _, symbol := range []AnswerSymbol{{Name: "ConsoleSink", Line: 8, Kind: KindType}, {Name: "ConsoleSink", File: "a.go", Kind: KindType}} {
		m := NewMutableState("")
		m.SetEmittedAnswerSymbolsWithOrigin([]AnswerSymbol{symbol}, CompletenessComplete, AnswerSymbolSelectionExplicitItems)
		got := BuildAnswerSurfacePlan(&AnalysisIR{RequestModel: b1604RelationEnumerationRequest()}, m, nil, nil, nil, b1604EnumerationEvidence())
		if got.stepBackboneFromAcceptedSymbolSlate {
			t.Fatal("unlocated symbol acquired exact member authority")
		}
	}
}

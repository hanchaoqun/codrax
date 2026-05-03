package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── runSymbolAnchorTrackOracle (P1 #3, 2026-05-03) ───────────────────

func newMutableWithRejections(t *testing.T, n int) *types.MutableState {
	t.Helper()
	mut := &types.MutableState{}
	closure := mut.EvidenceClosure()
	if closure == nil {
		t.Fatal("EvidenceClosure() returned nil after init")
	}
	for i := 0; i < n; i++ {
		closure.IncrementSymbolEmitRejection()
	}
	return mut
}

func TestSymbolAnchorTrack_BelowThresholdNoFire(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentEnumerate}
	doc := &types.AnswerDocument{Shape: types.ShapeListOfSymbols}
	mut := newMutableWithRejections(t, 2) // threshold is 3
	if vs := runSymbolAnchorTrackOracle(doc, rm, types.ShapeListOfSymbols, mut); len(vs) != 0 {
		t.Errorf("expected pass below threshold; got %+v", vs)
	}
}

func TestSymbolAnchorTrack_ListOfSymbolsZeroSymbolsFires(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentEnumerate}
	doc := &types.AnswerDocument{Shape: types.ShapeListOfSymbols} // zero symbols
	mut := newMutableWithRejections(t, 3)
	vs := runSymbolAnchorTrackOracle(doc, rm, types.ShapeListOfSymbols, mut)
	if len(vs) != 1 || vs[0].Kind != types.ViolSymbolAnchorMismatch {
		t.Fatalf("expected ViolSymbolAnchorMismatch; got %+v", vs)
	}
	if vs[0].Stage != string(types.StageExtract) {
		t.Errorf("expected Stage=extract; got %q", vs[0].Stage)
	}
	if vs[0].SuspectedRoot.IRField != "explorer_def_region_coverage" {
		t.Errorf("expected IRField=explorer_def_region_coverage; got %q", vs[0].SuspectedRoot.IRField)
	}
}

func TestSymbolAnchorTrack_ListOfSymbolsWithEnoughPasses(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentEnumerate}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeListOfSymbols,
		Symbols: []types.AnswerSymbol{{Name: "A"}, {Name: "B"}, {Name: "C"}},
	}
	mut := newMutableWithRejections(t, 5) // many rejections but answer recovered
	if vs := runSymbolAnchorTrackOracle(doc, rm, types.ShapeListOfSymbols, mut); len(vs) != 0 {
		t.Errorf("expected pass when answer recovered; got %+v", vs)
	}
}

func TestSymbolAnchorTrack_DeclaredCountShortFires(t *testing.T) {
	rm := &types.RequestModel{
		Intent: types.IntentEnumerate,
		EnumerationBoundary: &types.RequestedEnumerationBoundary{
			DeclaredCount: 8,
			SourceQuote:   "8 implementations",
		},
	}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeListOfSymbols,
		Symbols: []types.AnswerSymbol{{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"}, {Name: "E"}}, // 5 of 8
	}
	mut := newMutableWithRejections(t, 3)
	vs := runSymbolAnchorTrackOracle(doc, rm, types.ShapeListOfSymbols, mut)
	if len(vs) != 1 || vs[0].Kind != types.ViolSymbolAnchorMismatch {
		t.Fatalf("expected ViolSymbolAnchorMismatch on declared count short; got %+v", vs)
	}
}

func TestSymbolAnchorTrack_DeclaredCountMetPasses(t *testing.T) {
	rm := &types.RequestModel{
		EnumerationBoundary: &types.RequestedEnumerationBoundary{
			DeclaredCount: 3,
			SourceQuote:   "3 X",
		},
	}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeListOfSymbols,
		Symbols: []types.AnswerSymbol{{Name: "A"}, {Name: "B"}, {Name: "C"}},
	}
	mut := newMutableWithRejections(t, 3)
	if vs := runSymbolAnchorTrackOracle(doc, rm, types.ShapeListOfSymbols, mut); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestSymbolAnchorTrack_UnsupportedShapeNoFire(t *testing.T) {
	rm := &types.RequestModel{}
	doc := &types.AnswerDocument{Shape: types.ShapeValue}
	mut := newMutableWithRejections(t, 5)
	if vs := runSymbolAnchorTrackOracle(doc, rm, types.ShapeValue, mut); len(vs) != 0 {
		t.Errorf("expected pass on unsupported shape; got %+v", vs)
	}
}

func TestSymbolAnchorTrack_NilGuards(t *testing.T) {
	if vs := runSymbolAnchorTrackOracle(nil, &types.RequestModel{}, types.ShapeListOfSymbols, &types.MutableState{}); len(vs) != 0 {
		t.Errorf("expected nil pass on nil doc; got %+v", vs)
	}
	if vs := runSymbolAnchorTrackOracle(&types.AnswerDocument{}, nil, types.ShapeListOfSymbols, &types.MutableState{}); len(vs) != 0 {
		t.Errorf("expected nil pass on nil rm; got %+v", vs)
	}
	if vs := runSymbolAnchorTrackOracle(&types.AnswerDocument{}, &types.RequestModel{}, types.ShapeListOfSymbols, nil); len(vs) != 0 {
		t.Errorf("expected nil pass on nil mut; got %+v", vs)
	}
}

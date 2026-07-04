package types

// trace_note_keys_consumer_pin_test.go — NKR consumer-side pin (B-5 hardened
// ast.Inspect model): every rich-note key referenced at a consumer parse site
// or a legacy-lane note-emission site in THIS package must go through the
// trace_note_keys.go registry.
//
// Three rules, all precise structural signals:
//
//  1. No bare string-literal key argument at any note-parse helper call site
//     in the projection/coverage/ledger consumer files — keys arrive as
//     TraceNoteKey* constants (or as loop/parameter variables that are
//     themselves fed from checked lists).
//  2. Every string literal inside a legacy selection list
//     (traceQuerySelectedRichNotes 2nd arg / traceQueryStateChurnRichNotes
//     range list) must be a REGISTERED key — those list entries are emitted
//     verbatim as wire keys. The churn range list needs its own scan shape:
//     its elements are bare identifiers/keys with no "=", so rule 3's
//     notePattern can never catch a typo there.
//  3. Every "key=…" string literal inside the legacy record/notes builder
//     functions (traceQuery*Record / traceQuery*RichNotes in
//     observation_ledger.go) must have a registered key prefix.
//  4. Ledger-marker lane: the composite marker notes
//     (TraceNoteMarkerAdvisoryPretriage / TraceNoteMarkerLegacySummaryFallback)
//     are appended by appendUniqueObservationString and exact-matched by
//     observationRecordHasRichNote — both call shapes must carry the composite
//     CONSTANT, never a rebuilt string literal, and every composite must
//     decompose into registered ledger_marker rows.
//
// Every rule fatals when its scan matches nothing (matched==0) — a pin that
// stops matching is a pin that silently checks nothing.
//
// Test files keep verbatim key literals on purpose (wire-format double-write)
// and are excluded, exactly like the causal-token registry pins.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"
)

// traceNoteKeyParseHelpers are the key-argument positions (helper name →
// 0-based indices of key params) of every rich-note parse helper in this
// package.
var traceNoteKeyParseHelpers = map[string][]int{
	"traceCausalProjectionRichNoteValue":      {1},
	"traceCausalProjectionRichNoteFloat":      {1},
	"traceCausalProjectionRichNoteInt":        {1},
	"traceCausalProjectionRichNoteFirstFloat": {1, 2, 3, 4},
	"traceCausalProjectionRichNoteFirstInt":   {1, 2, 3, 4},
	"traceObservationRichNoteValue":           {1},
	"traceObservationRichNoteBool":            {1},
	"observationRichNoteHasKey":               {1},
}

var traceNoteKeyConsumerPinFiles = []string{
	"trace_causal_projection.go",
	"trace_observation_coverage.go",
	"observation_ledger.go",
}

func traceNoteKeyPinParse(t *testing.T, name string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return fset, file
}

func TestTraceNoteKeyConsumerCallSitesUseConstants(t *testing.T) {
	for _, name := range traceNoteKeyConsumerPinFiles {
		fset, file := traceNoteKeyPinParse(t, name)
		matched := 0
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			keyArgs, ok := traceNoteKeyParseHelpers[fn.Name]
			if !ok {
				return true
			}
			matched++
			for _, idx := range keyArgs {
				if idx >= len(call.Args) {
					continue
				}
				if lit, isLit := call.Args[idx].(*ast.BasicLit); isLit {
					t.Errorf("%s: %s called with bare key literal %s — use the TraceNoteKey* constant (trace_note_keys.go change protocol)",
						fset.Position(lit.Pos()), fn.Name, lit.Value)
				}
			}
			return true
		})
		if matched == 0 {
			t.Fatalf("%s: no note-parse helper call sites matched — the pin is checking nothing; update traceNoteKeyParseHelpers alongside the refactor", name)
		}
	}
}

func TestTraceNoteKeyLegacySelectionListsRegistered(t *testing.T) {
	selectionLists := 0
	for _, name := range traceNoteKeyConsumerPinFiles {
		fset, file := traceNoteKeyPinParse(t, name)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok || fn.Name != "traceQuerySelectedRichNotes" || len(call.Args) < 2 {
				return true
			}
			lit, ok := call.Args[1].(*ast.CompositeLit)
			if !ok {
				return true
			}
			selectionLists++
			for _, elt := range lit.Elts {
				basic, isLit := elt.(*ast.BasicLit)
				if !isLit {
					continue // TraceNoteKey* constant — registered by construction
				}
				key := strings.Trim(basic.Value, `"`)
				if !TraceNoteKeyRegistered(key) {
					t.Errorf("%s: legacy selection list carries UNREGISTERED wire key %q — register it in trace_note_keys.go first",
						fset.Position(basic.Pos()), key)
				}
			}
			return true
		})
	}
	if selectionLists == 0 {
		t.Fatal("legacy-selection-list scan matched no traceQuerySelectedRichNotes composite lists — the pin is checking nothing; update the scan alongside the refactor")
	}
}

// TestTraceNoteKeyStateChurnRangeListRegistered covers the ONE legacy list
// rule 2 above cannot see and rule 3 can never see: the range list inside
// traceQueryStateChurnRichNotes. Its elements are bare wire keys (no "="),
// emitted verbatim as key+"="+value — a typo'd element ("fragmentz") would
// ride wire silence into a silently-dropped churn note, so every string
// literal ranged over in that function must be a registered key.
func TestTraceNoteKeyStateChurnRangeListRegistered(t *testing.T) {
	fset, file := traceNoteKeyPinParse(t, "observation_ledger.go")
	rangeLists := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "traceQueryStateChurnRichNotes" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			rng, ok := n.(*ast.RangeStmt)
			if !ok {
				return true
			}
			lit, ok := rng.X.(*ast.CompositeLit)
			if !ok {
				return true
			}
			rangeLists++
			for _, elt := range lit.Elts {
				basic, isLit := elt.(*ast.BasicLit)
				if !isLit {
					continue // TraceNoteKey* constant — registered by construction
				}
				key := strings.Trim(basic.Value, `"`)
				if !TraceNoteKeyRegistered(key) {
					t.Errorf("%s: traceQueryStateChurnRichNotes range list carries UNREGISTERED wire key %q — register it in trace_note_keys.go first",
						fset.Position(basic.Pos()), key)
				}
			}
			return true
		})
	}
	if rangeLists == 0 {
		t.Fatal("state-churn range-list scan matched nothing — traceQueryStateChurnRichNotes was renamed or its range list restructured; move the pin with it")
	}
}

// TestTraceNoteKeyLegacyEmittersRegistered walks the legacy text-parse lane's
// record/notes builder functions and checks every "key=…"-shaped string
// literal (the raw note emissions) against the registry.
func TestTraceNoteKeyLegacyEmittersRegistered(t *testing.T) {
	fnPattern := regexp.MustCompile(`^traceQuery.*(Record|RichNotes)$`)
	notePattern := regexp.MustCompile(`^([a-z][a-z0-9_]*)=`)
	fset, file := traceNoteKeyPinParse(t, "observation_ledger.go")
	checked := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !fnPattern.MatchString(fn.Name.Name) {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			basic, ok := n.(*ast.BasicLit)
			if !ok || basic.Kind != token.STRING {
				return true
			}
			value := strings.Trim(basic.Value, "`\"")
			m := notePattern.FindStringSubmatch(value)
			if m == nil {
				return true
			}
			// Sprintf verb directly after '=' or end-of-string or a literal
			// value both count — the KEY prefix is the wire token either way.
			key := m[1]
			checked++
			if !TraceNoteKeyRegistered(key) {
				t.Errorf("%s: legacy emitter %s writes note with UNREGISTERED key %q (literal %s) — register it in trace_note_keys.go first",
					fset.Position(basic.Pos()), fn.Name.Name, key, basic.Value)
			}
			return true
		})
	}
	if checked == 0 {
		t.Fatal("legacy-emitter scan matched no note-shaped literals — the pin is checking nothing; update fnPattern/notePattern alongside the refactor")
	}
}

// TestTraceNoteKeyLedgerMarkerSitesUseConstants is rule 4 (F4 ledger-marker
// lane). The composite markers are appended by appendUniqueObservationString
// (reconcileRuntimeObservationProducerPrecedence,
// legacySummaryToolObservationRecord) and exact-full-note-matched by
// observationRecordHasRichNote (observationRecordRank). A rebuilt literal at
// either end drifts silently — the exact match just stops firing — so BOTH
// call shapes must receive a constant, never a string literal.
func TestTraceNoteKeyLedgerMarkerSitesUseConstants(t *testing.T) {
	fset, file := traceNoteKeyPinParse(t, "observation_ledger.go")
	appendSites, matchSites := 0, 0
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		switch fn.Name {
		case "appendUniqueObservationString":
			appendSites++
		case "observationRecordHasRichNote":
			matchSites++
		default:
			return true
		}
		if len(call.Args) < 2 {
			return true
		}
		if lit, isLit := call.Args[1].(*ast.BasicLit); isLit {
			t.Errorf("%s: %s called with bare note literal %s — use the TraceNoteMarker* / registered-key constant (trace_note_keys.go ledger-marker family)",
				fset.Position(lit.Pos()), fn.Name, lit.Value)
		}
		return true
	})
	if appendSites == 0 || matchSites == 0 {
		t.Fatalf("ledger-marker scan matched appendUniqueObservationString=%d observationRecordHasRichNote=%d call sites — the pin is checking nothing; move it with the refactor", appendSites, matchSites)
	}
}

// TestTraceNoteKeyLedgerMarkersDecomposeToRegistry pins the composite marker
// wire strings themselves: every "; "-separated token must be a registered
// ledger_marker row (bare marker or key=value). A registry rename that leaves
// a composite stale — or a composite edit that invents an unregistered token —
// fails here instead of riding wire silence.
func TestTraceNoteKeyLedgerMarkersDecomposeToRegistry(t *testing.T) {
	for _, marker := range []string{TraceNoteMarkerAdvisoryPretriage, TraceNoteMarkerLegacySummaryFallback} {
		for _, part := range strings.Split(marker, "; ") {
			key := part
			if k, _, ok := strings.Cut(part, "="); ok {
				key = k
			}
			row, ok := TraceNoteKeyLookup(key)
			if !ok {
				t.Errorf("composite ledger marker %q token %q is not a registered key", marker, key)
				continue
			}
			if row.Family != "ledger_marker" {
				t.Errorf("composite ledger marker %q token %q registered under family %q, want ledger_marker", marker, key, row.Family)
			}
		}
	}
}

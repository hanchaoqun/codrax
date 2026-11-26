package tracequery

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"
)

// A blocking-like span row is worded from one of two typed facts: the kind of
// its parsed contention payload, or the basis of its value. These tests pin
// that a row carrying neither is withheld with a typed reason, and that the
// evidence fact mirroring a typed lock rank row carries that row's kind.

func hmc378BlockingRow(comm, kind, basis string) blockingSpanRow {
	return blockingSpanRow{cand: CriticalBlockingCandidate{
		Type: "blocking_span", Thread: ThreadRef{Comm: comm, PID: 1}, DurationMs: 5,
		BlockingKind: kind, BlockingValueBasis: basis,
	}}
}

func TestHMC378KindlessUnbasedBlockingRowIsRefused(t *testing.T) {
	rows := []blockingSpanRow{
		hmc378BlockingRow("neither", "", ""),
		hmc378BlockingRow("wait-segments", "", BlockingValueBasisWaitSegments),
		hmc378BlockingRow("span-envelope", "", BlockingValueBasisSpanEnvelope),
		hmc378BlockingRow("unregistered-basis", "", "wall_clock"),
		hmc378BlockingRow("lock", blockingKindMonitorContention, BlockingValueBasisWaitSegments),
		hmc378BlockingRow("lock-without-basis", blockingKindLockContention, ""),
	}
	kept, refused := refuseUnbasedKindlessBlockingRows(rows)
	var names []string
	for _, row := range kept {
		names = append(names, row.cand.Thread.Comm)
	}
	if want := []string{"wait-segments", "span-envelope", "lock", "lock-without-basis"}; !reflect.DeepEqual(names, want) || refused != 2 {
		t.Fatalf("kept %v refused %d, want %v and 2 refused", names, refused, want)
	}
	caveats := appendBlockingSpanRefusalCaveat([]string{"earlier"}, refused)
	if len(caveats) != 2 || caveats[0] != "earlier" || !strings.HasPrefix(caveats[1], BlockingSpanRefusalWithoutKindOrBasis+": 2 ") {
		t.Fatalf("the refusal must add one caveat carrying its typed reason and count: %q", caveats)
	}
	if caveats := appendBlockingSpanRefusalCaveat([]string{"earlier"}, 0); !reflect.DeepEqual(caveats, []string{"earlier"}) {
		t.Fatalf("no refusal, no caveat: %q", caveats)
	}
	if kept, refused := refuseUnbasedKindlessBlockingRows(nil); len(kept) != 0 || refused != 0 {
		t.Fatalf("no rows: kept %v refused %d", kept, refused)
	}
}

// hmc378BothBasisCapture: main-100 parks in a recorded wait and is woken by the
// collector (its wait segments are measured); ghost-120 records the same wait
// name and has no scheduler row at all (only the span envelope is known).
func hmc378BothBasisCapture() string {
	return hmc377SharedWaitCapture(hmc377RelayWait) +
		"           ghost-120   (  100) [005] ....   1.020000: tracing_mark_write: B|100|" + hmc377RelayWait + "\n" +
		"           ghost-120   (  100) [005] ....   1.090000: tracing_mark_write: E|100\n"
}

// Through the engine entry every recorded wait without a contention payload is
// published with its value basis, on both arms of the value convergence, and
// nothing is refused.
func TestHMC378PublishedPayloadLessBlockingRowsCarryTheirBasis(t *testing.T) {
	idx := buildTraceIndex(t, "hmc378_basis.systrace", hmc378BothBasisCapture())
	res := BuildCriticalBlockingCalls(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2})
	basis := map[int]string{}
	for _, item := range res.Items {
		if item.Type != "blocking_span" {
			continue
		}
		if item.BlockingKind != "" {
			t.Fatalf("fixture: a recorded wait without a contention payload gained a kind: %+v", item)
		}
		basis[item.Thread.PID] = item.BlockingValueBasis
	}
	if basis[100] != BlockingValueBasisWaitSegments || basis[120] != BlockingValueBasisSpanEnvelope {
		t.Fatalf("want main-100 on wait segments and ghost-120 on the span envelope, got %v", basis)
	}
	for _, caveat := range res.Caveats {
		if strings.HasPrefix(caveat, BlockingSpanRefusalWithoutKindOrBasis) {
			t.Fatalf("a row of today's producers was refused: %q", caveat)
		}
	}
}

// The guard stands on the one path every blocking-like span row takes: one
// mint site, one collector that runs the guard, and the blocking view that
// publishes the refusal count. A second publisher of minted rows or a
// collector that skips the guard fails here.
func TestHMC378BlockingSpanRowsPassTheRefusalGuard(t *testing.T) {
	for ident, allowed := range map[string][]string{
		// The inversion cross-check reads the kind and owner of a parsed
		// contention payload as a cover interval and publishes no row.
		"blockingSpanCandidateFromTraceSpan": {"collectBlockingSpanRowsCounted", "demoteLockDominatedInversionCandidates"},
		"refuseUnbasedKindlessBlockingRows":  {"collectBlockingSpanRowsCounted"},
		"collectBlockingSpanRowsCounted":     {"collectBlockingSpanRows", "buildCriticalBlockingCallsFromStats"},
		"appendBlockingSpanRefusalCaveat":    {"buildCriticalBlockingCallsFromStats"},
	} {
		inside, outside := hmc376Consumers(t, ident)
		if len(outside) > 0 {
			t.Fatalf("%s is referenced outside any function declaration: %v", ident, outside)
		}
		want := map[string]bool{}
		for _, fn := range allowed {
			want[fn] = true
		}
		if !reflect.DeepEqual(inside, want) {
			t.Fatalf("%s consumers changed:\n got %v\nwant %v", ident, inside, want)
		}
	}
	if minters, want := hmc378BlockingSpanCandidateMinters(t), map[string]bool{"blockingSpanCandidateFromTraceSpan": true}; !reflect.DeepEqual(minters, want) {
		t.Fatalf("functions that write the blocking_span row token into a blocking candidate changed:\n got %v\nwant %v", minters, want)
	}
}

// hmc378BlockingSpanCandidateMinters returns the functions of this package
// that build a blocking candidate carrying the blocking_span row token.
func hmc378BlockingSpanCandidateMinters(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	minters := map[string]bool{}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if id, ok := lit.Type.(*ast.Ident); !ok || id.Name != "CriticalBlockingCandidate" {
					return true
				}
				for _, elt := range lit.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, _ := kv.Key.(*ast.Ident)
					value, _ := kv.Value.(*ast.BasicLit)
					if key != nil && key.Name == "Type" && value != nil && value.Kind == token.STRING && value.Value == `"blocking_span"` {
						minters[fn.Name.Name] = true
					}
				}
				return true
			})
		}
	}
	return minters
}

// A contention payload and a blocking kind are one fact: every registered
// payload form mints a kind, and a blocking-like name outside the registry
// mints none. So a row without a kind is a row without a payload.
func TestHMC378EveryContentionPayloadFormMintsAKind(t *testing.T) {
	samples := map[string]string{
		"monitor_contention_owner":          lockContentionCustomerMonitorPayload,
		"lock_contention_on":                lockContentionCustomerLockPayload,
		"monitor_contention_owner_embedded": "vendor_xyz: " + lockContentionCustomerMonitorPayload,
		"owner_tid_keyed":                   "vendor mutex hold owner tid=4242",
	}
	for _, form := range lockContentionMorphologyRegistry {
		name, ok := samples[form.Name]
		if !ok {
			t.Fatalf("registered payload form %q has no sample here", form.Name)
		}
		info, parsed := parseLockContentionPayload(name)
		if !parsed || info.Morphology != form.Name {
			t.Fatalf("sample of %q parsed as %q (ok=%v)", form.Name, info.Morphology, parsed)
		}
		cand, admitted := blockingSpanCandidateFromTraceSpan(TraceSpanSummary{Name: name, DurationMs: 1})
		if !admitted || cand.BlockingKind == "" {
			t.Fatalf("payload form %q minted no kind: admitted=%v %+v", form.Name, admitted, cand)
		}
	}
	if len(samples) != len(lockContentionMorphologyRegistry) {
		t.Fatalf("%d samples for %d registered payload forms", len(samples), len(lockContentionMorphologyRegistry))
	}
	cand, admitted := blockingSpanCandidateFromTraceSpan(TraceSpanSummary{Name: hmc377RelayWait, DurationMs: 1})
	if !admitted || cand.BlockingKind != "" || cand.Waiters != 0 || cand.HolderSite != "" || cand.BlockingFromSite != "" || len(cand.HolderHandoff) != 0 {
		t.Fatalf("a recorded wait without a contention payload carries payload fields: admitted=%v %+v", admitted, cand)
	}
}

// The evidence fact that mirrors a typed lock rank row carries the row's kind
// and holder role, so the record minted from it is worded like the row. The
// two fields never reach the wire.
func TestHMC378RankEvidenceFactCarriesItsRowsBlockingKind(t *testing.T) {
	idx := buildTraceIndex(t, "hmc378_twin.systrace", lockRankQ4Trace)
	res := Run(idx, Query{View: "root_cause_rank", PID: 100, TimeStart: 4.99, TimeEnd: 5.115, MaxDepth: 4, MinDurationMs: 0.05, Limit: 16})
	if res.RootCauseRank == nil || len(res.RootCauseRank.Items) == 0 || len(res.EvidencePack) == 0 {
		t.Fatalf("fixture: no ranking or evidence: %+v", res)
	}
	head := res.RootCauseRank.Items[0]
	if head.Type != "blocking_span" || head.BlockingKind != blockingKindMonitorContention || !head.SubjectIsLockHolder {
		t.Fatalf("fixture: the head row must be the resolved holder's lock row: %+v", head)
	}
	rows := append(append([]RootCauseRankItem(nil), res.RootCauseRank.Items...), res.RootCauseRank.AbsorbedItems...)
	lockFacts := 0
	for i, fact := range res.EvidencePack {
		if i >= len(rows) {
			break
		}
		row := rows[i]
		if fact.Object != row.Type || fact.Subject != threadLabel(row.Thread) {
			t.Fatalf("fact %d does not mirror rank row %d: %+v vs %+v", i, i, fact, row)
		}
		if fact.BlockingKind != row.BlockingKind || fact.SubjectIsLockHolder != row.SubjectIsLockHolder {
			t.Fatalf("fact %d carries kind %q holder=%v, its rank row %q holder=%v", i, fact.BlockingKind, fact.SubjectIsLockHolder, row.BlockingKind, row.SubjectIsLockHolder)
		}
		if row.Type != "blocking_span" && (fact.BlockingKind != "" || fact.SubjectIsLockHolder) {
			t.Fatalf("fact %d of a %s row carries a blocking kind: %+v", i, row.Type, fact)
		}
		if fact.BlockingKind != "" {
			lockFacts++
		}
		bare := fact
		bare.BlockingKind, bare.SubjectIsLockHolder = "", false
		raw, err := json.Marshal(fact)
		rawBare, errBare := json.Marshal(bare)
		if err != nil || errBare != nil || string(raw) != string(rawBare) {
			t.Fatalf("the typed kind reached the wire form of fact %d: %s (%v %v)", i, raw, err, errBare)
		}
	}
	if lockFacts == 0 {
		t.Fatalf("no evidence fact mirrors the lock row: %+v", res.EvidencePack)
	}
}

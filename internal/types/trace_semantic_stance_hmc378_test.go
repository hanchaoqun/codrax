package types

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A row that records its thread waiting for other work is decided once, by the
// engine's stance of the marker name, and every merge carries that decision
// forward from its members. The engine is not linked into this package, so the
// tests install a predicate of their own: a span named "wait …" records a wait.

func hmc378StanceForTest(t *testing.T) {
	t.Helper()
	previous := traceSemanticSpanRecordsWait.Load()
	RegisterTraceSemanticSpanRecordsWait(func(name string) bool { return strings.HasPrefix(name, "wait ") })
	t.Cleanup(func() { traceSemanticSpanRecordsWait.Store(previous) })
}

// hmc378CollectionRecord is one collection marker of a thread as the engine
// publishes it: its own line range and interval, the class as the object.
func hmc378CollectionRecord(id, subject, span, relevance string, start, ms float64) ObservationRecord {
	causality := "background"
	notes := []string{
		"span_name=" + span,
		"span_kind=sync",
		"semantic_class=gc_pause",
		"chain_relevance=" + relevance,
		"selected_window=9.000000..12.000000",
	}
	if relevance == "on_chain" {
		causality = "on_wakeup_chain"
		notes = append(notes, "chain_depth=1")
	}
	notes = append(notes, "causality="+causality)
	line := int(start * 1000)
	return ObservationRecord{
		ID: id, Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "trace_semantic_span", ClaimKey: "trace_semantic_span:gc_pause:" + span + ":" + id,
		Subject: subject, Object: "gc_pause", Value: fmt.Sprintf("%.3f", ms), Unit: "ms",
		SupportRefs: []string{"trace.txt"},
		Span:        ObservationSpan{LineStart: line, LineEnd: line + 5, StartTs: start, EndTs: start + ms/1000},
		RichNotes:   notes, Confidence: 0.82,
	}
}

func hmc378Bucket(projection TraceCausalProjection, relevance string) []TraceCausalProjectionNode {
	switch relevance {
	case "on_chain":
		return projection.OnChainCauses
	case "adjacent":
		return projection.AdjacentCauses
	}
	return projection.BackgroundCauses
}

func hmc378Rows(nodes []TraceCausalProjectionNode) string {
	var out []string
	for _, node := range nodes {
		out = append(out, fmt.Sprintf("%s %q ×%d %.3fms waits=%v", node.Subject, node.SpanName, node.MergedCount, node.ImpactMS, TraceCausalProjectionNodeRecordsWait(node)))
	}
	return strings.Join(out, " | ")
}

// One thread records three waits for a collection (4 + 5 + 6 ms) and runs
// three collections itself (2 + 3 + 4 ms), all of one class. The repeated-row
// merge needs three members, and both groups have three: the waits are one
// ×3 row of 15 ms and the collections another of 9 ms. One ×6 row of 24 ms
// would be neither waiting time nor executed work, and it would read as
// whichever member seeded it.
func TestHMC378SameKindMergeKeepsRecordedWaitsApartFromWork(t *testing.T) {
	hmc378StanceForTest(t)
	for _, relevance := range []string{"on_chain", "adjacent", "background"} {
		var records []ObservationRecord
		for i := 0; i < 3; i++ {
			records = append(records,
				hmc378CollectionRecord(fmt.Sprintf("w%d", i), "main-100", "wait for collection", relevance, 10+float64(i)*0.1, 4+float64(i)),
				hmc378CollectionRecord(fmt.Sprintf("x%d", i), "main-100", "run collection", relevance, 10.05+float64(i)*0.1, 2+float64(i)))
		}
		bucket := hmc378Bucket(CompileTraceCausalProjection(ObservationLedger{Records: records}), relevance)
		var waits, work *TraceCausalProjectionNode
		for i := range bucket {
			switch bucket[i].SpanName {
			case "wait for collection":
				waits = &bucket[i]
			case "run collection":
				work = &bucket[i]
			}
		}
		if len(bucket) != 2 || waits == nil || work == nil {
			t.Fatalf("%s: want one row of waits and one row of executed work, got: %s", relevance, hmc378Rows(bucket))
		}
		if waits.MergedCount != 3 || waits.ImpactMS != 15 || !TraceCausalProjectionNodeRecordsWait(*waits) {
			t.Errorf("%s: the three waits are one ×3 row of 15 ms that records a wait, got: %s", relevance, hmc378Rows(bucket))
		}
		if work.MergedCount != 3 || work.ImpactMS != 9 || TraceCausalProjectionNodeRecordsWait(*work) {
			t.Errorf("%s: the three collections are one ×3 row of 9 ms of executed work, got: %s", relevance, hmc378Rows(bucket))
		}
	}
}

// Without the engine's predicate no row is known to record a wait, so the
// merge is exactly what it was: six rows of one thread and class are one row.
func TestHMC378WithoutThePredicateNoRowRecordsAWait(t *testing.T) {
	previous := traceSemanticSpanRecordsWait.Load()
	traceSemanticSpanRecordsWait.Store(nil)
	t.Cleanup(func() { traceSemanticSpanRecordsWait.Store(previous) })
	var records []ObservationRecord
	for i := 0; i < 3; i++ {
		records = append(records,
			hmc378CollectionRecord(fmt.Sprintf("w%d", i), "main-100", "wait for collection", "adjacent", 10+float64(i)*0.1, 4+float64(i)),
			hmc378CollectionRecord(fmt.Sprintf("x%d", i), "main-100", "run collection", "adjacent", 10.05+float64(i)*0.1, 2+float64(i)))
	}
	bucket := CompileTraceCausalProjection(ObservationLedger{Records: records}).AdjacentCauses
	if len(bucket) != 1 || bucket[0].MergedCount != 6 || bucket[0].ImpactMS != 24 || TraceCausalProjectionNodeRecordsWait(bucket[0]) {
		t.Fatalf("no predicate registered: the six rows stay one ×6 row of 24 ms and nothing records a wait, got: %s", hmc378Rows(bucket))
	}
	if TraceCausalProjectionNodeRecordsWait(TraceCausalProjectionNode{Subject: "main-100", SpanName: "wait for collection"}) {
		t.Fatal("no predicate registered: no row can be identified as a wait")
	}
}

// A row of one record is decided by its marker name. A merged row is decided
// by its members and never by the name it inherited: the name is one member's,
// or absent on a cross-thread fold.
func TestHMC378MergedRowStanceComesFromItsMembers(t *testing.T) {
	hmc378StanceForTest(t)
	leaf := func(span string) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{Subject: "main-100", Object: "gc_pause", SpanName: span, ImpactMS: 1}
	}
	merged := func(span string, count int, everyMemberWaits bool) TraceCausalProjectionNode {
		node := leaf(span)
		node.MergedCount, node.MergedRecordsWait = count, everyMemberWaits
		return node
	}
	for _, c := range []struct {
		label string
		node  TraceCausalProjectionNode
		want  bool
	}{
		{"one record named as a wait", leaf("wait for collection"), true},
		{"one record named as work", leaf("run collection"), false},
		{"one record without a span name", leaf(""), false},
		{"a merged row seeded by a wait whose members are not all waits", merged("wait for collection", 3, false), false},
		{"a merged row whose members are all waits", merged("wait for collection", 3, true), true},
		{"a cross-thread fold without a name whose members are all waits", merged("", 4, true), true},
		{"a cross-thread fold without a name with other members", merged("", 4, false), false},
		{"a fold of a single wait, without a name", merged("", 1, true), true},
	} {
		if got := TraceCausalProjectionNodeRecordsWait(c.node); got != c.want {
			t.Errorf("%s: records a wait = %v, want %v", c.label, got, c.want)
		}
	}
	for _, c := range []struct {
		label   string
		members []TraceCausalProjectionNode
		want    bool
	}{
		{"every member is a wait", []TraceCausalProjectionNode{leaf("wait for collection"), leaf("wait for a lock"), merged("", 2, true)}, true},
		{"one member is executed work", []TraceCausalProjectionNode{leaf("wait for collection"), leaf("run collection")}, false},
		{"one member is a fold that is not all waits", []TraceCausalProjectionNode{leaf("wait for collection"), merged("wait for collection", 3, false)}, false},
		{"no member", nil, false},
	} {
		if got := TraceCausalProjectionMembersRecordWait(c.members); got != c.want {
			t.Errorf("%s: members record a wait = %v, want %v", c.label, got, c.want)
		}
		// The same decision over members a merge addresses by index.
		indices := make([]int, len(c.members))
		for i := range indices {
			indices[i] = i
		}
		if got := traceCausalProjectionIndexedMembersRecordWait(c.members, indices); got != c.want {
			t.Errorf("%s: indexed members record a wait = %v, want %v", c.label, got, c.want)
		}
	}
}

// Every fold that builds a row out of other rows carries the stance of its
// members: the bounded-face overflow fold and the unknown-peer background fold
// are rows of waits only when every folded row is one. Both folds have no span
// name, so nothing else could say it.
func TestHMC378CrossThreadFoldsCarryTheStanceOfTheirMembers(t *testing.T) {
	hmc378StanceForTest(t)
	row := func(subject, span string, ms float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Subject: subject, Object: "gc_pause", SpanName: span, SemanticClass: "gc_pause", Role: TraceCausalRoleSemanticSpan,
			Predicate: "trace_semantic_span", ChainRelevance: "background", ImpactMS: ms, CumulativeImpactMS: ms, EvidenceID: "e-" + subject,
		}
	}
	waits := []TraceCausalProjectionNode{row("a-1", "wait for collection", 5), row("b-2", "wait for collection", 4), row("c-3", "wait for collection", 3)}
	mixed := []TraceCausalProjectionNode{row("a-1", "wait for collection", 5), row("b-2", "run collection", 4), row("c-3", "wait for collection", 3)}

	if fold := traceCausalProjectionOverflowFoldRow(waits); fold.SpanName != "" || fold.MergedCount != 3 || !TraceCausalProjectionNodeRecordsWait(fold) {
		t.Errorf("an overflow fold of three waits is a row of waits: %+v", fold)
	}
	if fold := traceCausalProjectionOverflowFoldRow(mixed); fold.MergedCount != 3 || TraceCausalProjectionNodeRecordsWait(fold) {
		t.Errorf("an overflow fold that holds executed work is no row of waits: %+v", fold)
	}
	// A fold of folds reads its member folds' stance, not their absent names.
	inner := traceCausalProjectionOverflowFoldRow(waits[1:])
	if fold := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{waits[0], inner}); fold.MergedCount != 3 || !TraceCausalProjectionNodeRecordsWait(fold) {
		t.Errorf("a fold of a wait and a fold of waits is a row of waits: %+v", fold)
	}
	// The bounded context bucket keeps its first rows and folds the rest.
	bounded := traceCausalProjectionLimitContextNodesFold(append([]TraceCausalProjectionNode{row("keep-9", "run collection", 9)}, waits...), 1, "background")
	if len(bounded) != 2 || bounded[1].MergedCount != 3 || !TraceCausalProjectionNodeRecordsWait(bounded[1]) || TraceCausalProjectionNodeRecordsWait(bounded[0]) {
		t.Errorf("the bucket cap folds the three waits into one row of waits beside the kept row: %s", hmc378Rows(bounded))
	}

	// Unknown-peer background rows fold past the first kept ones.
	unknown := func(subject, span string, ms float64) TraceCausalProjectionNode {
		node := row(subject, span, ms)
		node.Object = "unknown"
		return node
	}
	for _, c := range []struct {
		label string
		span  string
		want  bool
	}{
		{"every folded row is a wait", "wait for collection", true},
		{"one folded row is executed work", "run collection", false},
	} {
		var nodes []TraceCausalProjectionNode
		for i := 0; i < traceCausalProjectionUnknownBackgroundMin+1; i++ {
			span := "wait for collection"
			if i == traceCausalProjectionUnknownBackgroundMin {
				span = c.span // the smallest row, always inside the fold
			}
			nodes = append(nodes, unknown(fmt.Sprintf("t-%d", i), span, float64(100-i)))
		}
		folded := traceCausalProjectionFoldUnknownBackground(nodes)
		last := folded[len(folded)-1]
		if last.MergedCount < 2 || last.SpanName != "" {
			t.Fatalf("%s: the fixture must produce the unknown-peer fold, got: %s", c.label, hmc378Rows(folded))
		}
		if got := TraceCausalProjectionNodeRecordsWait(last); got != c.want {
			t.Errorf("%s: the fold records a wait = %v, want %v", c.label, got, c.want)
		}
	}
}

// The occurrence merge takes whatever rows its caller chose, so nothing but the
// members themselves can say what the merged row is: a row of waits only when
// every member is one, whichever member seeds it.
func TestHMC378OccurrenceMergeReadsEveryMember(t *testing.T) {
	hmc378StanceForTest(t)
	row := func(id, span string, ms float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Subject: "main-100", Object: "gc_pause", SpanName: span, SemanticClass: "gc_pause", Role: TraceCausalRoleSemanticSpan,
			Predicate: "trace_semantic_span", ChainRelevance: "adjacent", ImpactMS: ms, CumulativeImpactMS: ms, EvidenceID: id,
		}
	}
	for _, c := range []struct {
		label string
		rows  []TraceCausalProjectionNode
		want  bool
	}{
		{"two waits", []TraceCausalProjectionNode{row("a", "wait for collection", 5), row("b", "wait for collection", 4)}, true},
		{"a wait seeds a merge with executed work", []TraceCausalProjectionNode{row("a", "wait for collection", 5), row("b", "run collection", 4)}, false},
		{"executed work seeds a merge with a wait", []TraceCausalProjectionNode{row("a", "run collection", 5), row("b", "wait for collection", 4)}, false},
	} {
		merged := TraceCausalProjectionMergeOccurrenceRows(c.rows)
		if merged.MergedCount != 2 {
			t.Fatalf("%s: the fixture must merge two rows, got ×%d", c.label, merged.MergedCount)
		}
		if got := TraceCausalProjectionNodeRecordsWait(merged); got != c.want {
			t.Errorf("%s: the merged row records a wait = %v, want %v", c.label, got, c.want)
		}
	}
	// One row is no merge and keeps its own stance.
	if single := TraceCausalProjectionMergeOccurrenceRows([]TraceCausalProjectionNode{row("a", "wait for collection", 5)}); !TraceCausalProjectionNodeRecordsWait(single) {
		t.Errorf("a single wait handed to the merge stays a wait: %+v", single)
	}
}

// An instant marker folded into the row whose segment it marks brings no time
// of its own, so the row keeps the stance it had.
func TestHMC378MarkerFoldKeepsTheStanceOfItsRow(t *testing.T) {
	hmc378StanceForTest(t)
	for _, c := range []struct {
		span string
		want bool
	}{{"wait for collection", true}, {"run collection", false}} {
		host := TraceCausalProjectionNode{Subject: "main-100", Object: "peer-7", SpanName: c.span, ImpactMS: 5, CumulativeImpactMS: 5, StartTs: 10, EndTs: 10.005, EvidenceID: "host"}
		marker := TraceCausalProjectionNode{Subject: "main-100", Object: "peer-7", StartTs: 10.002, EndTs: 10.002, EvidenceID: "marker"}
		folded := traceCausalProjectionFoldZeroValueMarkerRows([]TraceCausalProjectionNode{host, marker})
		if len(folded) != 1 || folded[0].MergedCount != 2 {
			t.Fatalf("%q: the fixture must fold the marker into its row, got: %s", c.span, hmc378Rows(folded))
		}
		if got := TraceCausalProjectionNodeRecordsWait(folded[0]); got != c.want {
			t.Errorf("%q: the row with its marker records a wait = %v, want %v", c.span, got, c.want)
		}
	}
}

// Every place that makes a merged row decides the row's stance in the same
// function. A merged row without that decision would be read as executed work
// whatever it merged; a new fold that forgets it fails here.
func TestHMC378EveryMergeSiteDecidesTheMergedStance(t *testing.T) {
	// The one merge that needs no decision: the engine publishes it already
	// folded, and it folds wakeup impact rows there, never markers. A row
	// re-read from that publication is no row of waits.
	exempt := map[string]string{
		"types/trace_causal_projection.go:traceCausalProjectionNodeFromRecord": "re-reads a fold the engine published; the engine folds wakeup impact rows there, never markers",
	}
	fset := token.NewFileSet()
	type site struct{ count, stance bool }
	sites := map[string]*site{}
	for _, dir := range []string{".", filepath.Join("..", "tool"), filepath.Join("..", "agent"), filepath.Join("..", "analysis", "tracefinding"), filepath.Join("..", "tracequery")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			pkg := filepath.Base(dir)
			if dir == "." {
				pkg = "types"
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				key := pkg + "/" + name + ":" + fn.Name.Name
				mark := func(field string) {
					s := sites[key]
					if s == nil {
						s = &site{}
						sites[key] = s
					}
					switch field {
					case "MergedCount":
						s.count = true
					case "MergedRecordsWait":
						s.stance = true
					}
				}
				written := func(expr ast.Expr) {
					if sel, ok := expr.(*ast.SelectorExpr); ok {
						mark(sel.Sel.Name)
					}
				}
				ast.Inspect(fn, func(n ast.Node) bool {
					switch stmt := n.(type) {
					case *ast.AssignStmt:
						for _, lhs := range stmt.Lhs {
							written(lhs)
						}
					case *ast.IncDecStmt:
						written(stmt.X)
					case *ast.KeyValueExpr:
						if id, ok := stmt.Key.(*ast.Ident); ok {
							mark(id.Name)
						}
					}
					return true
				})
			}
		}
	}
	var merging, missing []string
	for key, s := range sites {
		if !s.count {
			continue
		}
		merging = append(merging, key)
		if !s.stance && exempt[key] == "" {
			missing = append(missing, key)
		}
	}
	sort.Strings(merging)
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("these functions make a merged row without deciding whether it records a wait: %v", missing)
	}
	want := []string{
		"tool/answer_document_mutation_runtime_rnb5b.go:runtimeTraceProjBuildMicroAnchorFoldRow",
		"types/trace_causal_projection.go:traceCausalProjectionNodeFromRecord",
		"types/trace_causal_projection.go:traceCausalProjectionOverflowFoldRow",
		"types/trace_causal_projection_aggregate.go:traceCausalProjectionFoldUnknownBackground",
		"types/trace_causal_projection_aggregate.go:traceCausalProjectionFoldZeroValueMarkerRows",
		"types/trace_causal_projection_aggregate.go:traceCausalProjectionMergeSameKindMemberValues",
	}
	if strings.Join(merging, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the set of merge sites changed; a new one must decide the stance and be listed here:\n got %v\nwant %v", merging, want)
	}
	for key := range exempt {
		if s := sites[key]; s == nil || !s.count {
			t.Fatalf("exempt site %s no longer makes a merged row", key)
		}
	}
}

// Whether a row records a wait has one decision function, one field that only
// that function reads and only merges write, and a closed list of consumers:
// two display lanes of the reader (the occupancy table's group and the running
// breakdown's pointer), the repeated-row merge key, and the merges themselves.
// None is a gate, a completion path or a score. A new consumer is added here
// on purpose, with what it does with the answer.
func TestHMC378RowStanceConsumersPinned(t *testing.T) {
	symbols := map[string]bool{
		"traceSemanticSpanRecordsWait":                  true,
		"RegisterTraceSemanticSpanRecordsWait":          true,
		"TraceCausalProjectionNodeRecordsWait":          true,
		"TraceCausalProjectionMembersRecordWait":        true,
		"traceCausalProjectionIndexedMembersRecordWait": true,
		"MergedRecordsWait":                             true,
		"runtimeTraceProjSemanticRowRecordsWait":        true,
	}
	want := map[string]string{
		// The registration hook, the decision and the field.
		"types/trace_semantic_seat_yield.go:<package level>":                        "traceSemanticSpanRecordsWait",
		"types/trace_semantic_seat_yield.go:RegisterTraceSemanticSpanRecordsWait":   "traceSemanticSpanRecordsWait",
		"types/trace_semantic_seat_yield.go:TraceCausalProjectionNodeRecordsWait":   "MergedRecordsWait traceSemanticSpanRecordsWait",
		"types/trace_semantic_seat_yield.go:TraceCausalProjectionMembersRecordWait": "TraceCausalProjectionNodeRecordsWait",
		"types/trace_causal_projection.go:<package level>":                          "MergedRecordsWait",
		"tracequery/semantic_platform_vocabulary.go:init":                           "RegisterTraceSemanticSpanRecordsWait",
		// The repeated-row merge key: waits form their own family.
		"types/trace_causal_projection_aggregate.go:traceCausalProjectionAggregateSameKindLane": "TraceCausalProjectionNodeRecordsWait",
		// The merges: each decides the merged row's stance from its members.
		"types/trace_causal_projection_aggregate.go:traceCausalProjectionIndexedMembersRecordWait":  "TraceCausalProjectionNodeRecordsWait",
		"types/trace_causal_projection_aggregate.go:traceCausalProjectionMergeSameKindMemberValues": "MergedRecordsWait traceCausalProjectionIndexedMembersRecordWait",
		"types/trace_causal_projection_aggregate.go:traceCausalProjectionFoldUnknownBackground":     "MergedRecordsWait traceCausalProjectionIndexedMembersRecordWait",
		"types/trace_causal_projection_aggregate.go:traceCausalProjectionFoldZeroValueMarkerRows":   "MergedRecordsWait TraceCausalProjectionNodeRecordsWait",
		"types/trace_causal_projection.go:traceCausalProjectionOverflowFoldRow":                     "MergedRecordsWait TraceCausalProjectionMembersRecordWait",
		"tool/answer_document_mutation_runtime_rnb5b.go:runtimeTraceProjBuildMicroAnchorFoldRow":    "MergedRecordsWait TraceCausalProjectionMembersRecordWait",
		// The reader: one predicate, two display lanes.
		"tool/answer_document_mutation_runtime_tree.go:runtimeTraceProjSemanticRowRecordsWait":   "TraceCausalProjectionNodeRecordsWait",
		"tool/answer_document_mutation_runtime.go:runtimeTraceOccupancySemanticCandidates":       "runtimeTraceProjSemanticRowRecordsWait",
		"tool/answer_document_mutation_runtime_tree.go:runtimeTraceProjFourStateSemanticPointer": "runtimeTraceProjSemanticRowRecordsWait",
	}
	got := map[string]string{}
	fset := token.NewFileSet()
	scan := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, ".."+string(filepath.Separator)))
		for _, decl := range file.Decls {
			scope, node := "<package level>", ast.Node(decl)
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if fn.Body == nil {
					continue
				}
				// The body only: a function's own name is its declaration.
				scope, node = fn.Name.Name, fn.Body
			}
			seen := map[string]bool{}
			ast.Inspect(node, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && symbols[id.Name] {
					seen[id.Name] = true
				}
				return true
			})
			if len(seen) == 0 {
				continue
			}
			names := make([]string, 0, len(seen))
			for name := range seen {
				names = append(names, name)
			}
			sort.Strings(names)
			key := rel + ":" + scope
			if got[key] != "" {
				names = append(strings.Fields(got[key]), names...)
				sort.Strings(names)
			}
			got[key] = strings.Join(names, " ")
		}
		return nil
	}
	for _, root := range []string{"..", filepath.Join("..", "..", "cmd")} {
		if err := filepath.WalkDir(root, scan); err != nil {
			t.Fatal(err)
		}
	}
	var diff []string
	for key, names := range got {
		if want[key] != names {
			diff = append(diff, fmt.Sprintf("%s reads %q, pinned %q", key, names, want[key]))
		}
	}
	for key, names := range want {
		if got[key] == "" {
			diff = append(diff, fmt.Sprintf("%s no longer reads %q", key, names))
		}
	}
	sort.Strings(diff)
	if len(diff) > 0 {
		t.Fatalf("the consumers of a row's stance changed:\n%s", strings.Join(diff, "\n"))
	}
}

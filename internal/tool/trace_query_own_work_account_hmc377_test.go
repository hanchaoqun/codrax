package tool

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 11 (R10P-1, owner ruling R-377-11 ③): per thread and semantic
// family there is one own-work account. The own-work seat and its own semantic
// span record are one account on ◎, the tree and the side file, and a
// projection merge never sums members that carry the same instants.
//
// The analysed thread's own-work family (all of its executes-stance spans of
// one class) and the wait family of the same spans (the members that contain a
// chain wait of the thread) are two disjoint parts of one account: own work =
// member union outside the thread's chain waits, wait part = member union
// inside them, own work + wait part = member union. Every expected number
// below is read off the synthetic capture's own timestamps.
//
// Raw witnesses (round-10 review, b2325eba6): ybb2 (105 元宝 后台销毁 #2, TID
// 58533) ◎ 37.945 + 37.945 against a class union of 40.085 and tree records
// 40.085 + 6.687; w27w2 (105 元宝 终端重启 #2, TID 11018) .root-causes.json
// class_verification effective 197.069 ms for a 196.401 ms window
// (hmc377/fix10/review-population/out/sweep/head/head_w27w2.rootcauses.json).

// hmc377R11SplitFamilyCapture: main-100 verifies three classes in a 200 ms
// window — A [10.010, 10.020) running, B [10.030, 10.050) with a 5 ms sleep
// [10.035, 10.040) that waker-150 ends (a chain wait inside the span), C
// [10.060, 10.065) running. Class union 35 ms; own work 30 ms; wait part 5 ms.
func hmc377R11SplitFamilyCapture(t *testing.T) string {
	t.Helper()
	var c hmc376Trace
	c.run(10.000000, "main", 100, 100, 1)
	c.run(10.000100, "waker", 150, 100, 2)
	c.span(10.010000, 10.020000, "main", 100, 100, 1, "VerifyClass com.example.A")
	c.span(10.030000, 10.050000, "main", 100, 100, 1, "VerifyClass com.example.B")
	c.sleep(10.035000, "main", 100, 100, 1)
	c.add(10.040000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.040010, "main", 100, 100, 1)
	c.span(10.060000, 10.065000, "main", 100, 100, 1, "VerifyClass com.example.C")
	c.sleep(10.100000, "main", 100, 100, 1)
	c.add(10.150000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.150010, "main", 100, 100, 1)
	c.sleep(10.190000, "main", 100, 100, 1)
	return c.write(t)
}

// hmc377R11WholeSpanWaitCapture: main-100 waits 149 ms for a CPU, then
// verifies one class [10.151, 10.251) and spends 98 ms of it waiting for a CPU
// again ([10.152, 10.250), hog-200 runs). Class union 100 ms; wait part 98 ms;
// own work 2 ms. Both parts land on the same tier: at b2325eba6 the compile
// dedupe read the two seats of the one span as one publication and the wait
// part vanished from ◎ and the side file.
func hmc377R11WholeSpanWaitCapture(t *testing.T) string {
	t.Helper()
	var c hmc376Trace
	c.run(10.000000, "main", 100, 100, 1)
	c.add(10.001000, "main", 100, 100, 1, "sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=R+ ==> next_comm=hog next_pid=200 next_prio=100")
	c.add(10.150000, "hog", 200, 200, 1, "sched_switch: prev_comm=hog prev_pid=200 prev_prio=100 prev_state=R+ ==> next_comm=main next_pid=100 next_prio=120")
	c.span(10.151000, 10.251000, "main", 100, 100, 1, "VerifyClass com.example.Whole")
	c.add(10.152000, "main", 100, 100, 1, "sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=R+ ==> next_comm=hog next_pid=200 next_prio=100")
	c.add(10.250000, "hog", 200, 200, 1, "sched_switch: prev_comm=hog prev_pid=200 prev_prio=100 prev_state=R+ ==> next_comm=main next_pid=100 next_prio=120")
	c.sleep(10.252000, "main", 100, 100, 1)
	return c.write(t)
}

// hmc377R11WindowSpanWaitCapture is the w27w2 shape: main-100 verifies one
// class across the whole window [10.100, 10.200) (the span starts at 10.0995
// and ends at 10.2005) and waits for a CPU [10.1001, 10.1990) inside it, then
// briefly again [10.1996, 10.1999). The thread's whole ready wait (99.2 ms)
// holds the first seat; the verification's wait part (98.9 ms) is the second
// seat, its own work the next tier. Class union 100 ms (the window); own work
// + wait part = 100 ms. At b2325eba6 the two records of the one span (same
// value, same span) merged in the ledger, the own-work record's effective
// moved onto the wait record, and the ×N merge summed the wait seat, the
// own-work seat and that record: 98.9 + 1.1 + 1.1 = 101.1 ms for a 100 ms
// window.
func hmc377R11WindowSpanWaitCapture(t *testing.T) string {
	t.Helper()
	var c hmc376Trace
	c.run(10.099000, "main", 100, 100, 1)
	c.span(10.099500, 10.200500, "main", 100, 100, 1, "VerifyClass com.example.Window")
	c.add(10.100100, "main", 100, 100, 1, "sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=R+ ==> next_comm=hog next_pid=200 next_prio=100")
	c.add(10.199000, "hog", 200, 200, 1, "sched_switch: prev_comm=hog prev_pid=200 prev_prio=100 prev_state=R+ ==> next_comm=main next_pid=100 next_prio=120")
	c.add(10.199600, "main", 100, 100, 1, "sched_switch: prev_comm=main prev_pid=100 prev_prio=120 prev_state=R+ ==> next_comm=hog next_pid=200 next_prio=100")
	c.add(10.199900, "hog", 200, 200, 1, "sched_switch: prev_comm=hog prev_pid=200 prev_prio=100 prev_state=R+ ==> next_comm=main next_pid=100 next_prio=120")
	c.sleep(10.201000, "main", 100, 100, 1)
	return c.write(t)
}

type hmc377R11Faces struct {
	projection types.TraceCausalProjection
	model      runtimeTraceProjTreeModel
	candidates []types.TraceFindingCandidateV1
	zh, en     string
}

// hmc377R11Query runs the public trace_query entry (wakeup_chain +
// root_cause_rank, the views the model issues) and builds every face this
// lane checks from the one compiled ledger: the projection, the tree model
// (◎ is built from its rows), the finding contract (the .root-causes.json
// population) and the rendered zh/en documents.
func hmc377R11Query(t *testing.T, path string, start, end float64) hmc377R11Faces {
	t.Helper()
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	var results []types.ToolResult
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": view, "pid": 100,
			"target_scope": "thread", "time_start": start, "time_end": end})
		if !r.Success {
			t.Fatalf("%s: %s", view, r.Summary)
		}
		results = append(results, r)
	}
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
	bus.ToolResults = results
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		t.Fatalf("one projection expected, got %d", len(set.Projections))
	}
	out := hmc377R11Faces{projection: set.Projections[0]}
	out.model = buildRuntimeTraceProjTreeModel(out.projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.SeatFrameCausalityAuthority{})
	if err != nil {
		t.Fatalf("finding contract: %v", err)
	}
	if contract != nil {
		out.candidates = tracefinding.SelectableRootCauseCandidates(contract)
	}
	for _, language := range []string{"zh", "en"} {
		b := newBusForMutationTest()
		b.Language = language
		b.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
		b.ToolResults = results
		doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "probe."}}}
		res, err := ApplyAndPersistMutation(b, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
		if err != nil || !res.Success {
			t.Fatalf("apply %s: %v %s", language, err, res.Summary)
		}
		text := render.RenderAnswerDocument(b.Mutable.AnswerDocumentV2(), language)
		if language == "zh" {
			out.zh = text
		} else {
			out.en = text
		}
	}
	return out
}

// hmc377R11ClassValues returns the value each face reads of the drawn rows of
// main-100's class verification.
func hmc377R11ClassValues(rows []*runtimeTraceProjTreeRow, value func(types.TraceCausalProjectionNode) float64) []float64 {
	return hmc377R11SubjectClassValues(rows, "main-100", value)
}

func hmc377R11SubjectClassValues(rows []*runtimeTraceProjTreeRow, subject string, value func(types.TraceCausalProjectionNode) float64) []float64 {
	var out []float64
	for _, row := range rows {
		node := row.Node
		if !row.HasData || strings.TrimSpace(node.SemanticClass) != "class_verification" ||
			runtimeTraceCausalProjectionCanonicalNode(node.Subject) != runtimeTraceCausalProjectionCanonicalNode(subject) {
			continue
		}
		out = append(out, value(node))
	}
	return out
}

func hmc377R11Sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}

func hmc377R11Has(values []float64, want float64) bool {
	for _, v := range values {
		if math.Abs(v-want) < 0.0015 {
			return true
		}
	}
	return false
}

func TestHMC377R11OwnWorkAccountIsOneAccountOnEveryFace(t *testing.T) {
	faces := hmc377R11Query(t, hmc377R11SplitFamilyCapture(t), 10.0, 10.2)
	const union = 35.0
	t.Run("overview", func(t *testing.T) {
		var population []float64
		for _, entry := range runtimeTraceProjElimBoard(faces.model) {
			node := entry.row.Node
			if strings.TrimSpace(node.SemanticClass) == "class_verification" && runtimeTraceCausalProjectionCanonicalNode(node.Subject) == runtimeTraceCausalProjectionCanonicalNode("main-100") {
				population = append(population, node.EffectiveImpactMS)
			}
		}
		// One own-work account (30) and the wait part of the same spans (5):
		// the own-work seat and its record are never two ◎ members.
		if len(population) != 2 || !hmc377R11Has(population, 30) || !hmc377R11Has(population, 5) || hmc377R11Sum(population) > union+0.003 {
			t.Fatalf("◎ must hold the own-work account once (30) and the wait part once (5), Σ ≤ %.3f; got %v", union, population)
		}
	})
	t.Run("tree", func(t *testing.T) {
		values := hmc377R11ClassValues(runtimeTraceProjSMR1AllRows(&faces.model), runtimeTraceProjNodeDisplayImpact)
		// Every drawn row of the family (rank seats and records alike): the
		// own-work part at 30, the wait part at 5, never a record wall beside
		// the parts it contains.
		if len(values) != 2 || !hmc377R11Has(values, 30) || !hmc377R11Has(values, 5) || hmc377R11Sum(values) > union+0.003 {
			t.Fatalf("the tree must draw each part once at its own instants (30 + 5 ≤ %.3f); got %v", union, values)
		}
	})
	t.Run("side_file", func(t *testing.T) {
		var values []float64
		for _, c := range faces.candidates {
			d := c.Decision
			if d.Token.Token == "class_verification" && d.Magnitude != nil {
				values = append(values, d.Magnitude.Value)
			}
		}
		if len(values) != 2 || !hmc377R11Has(values, 30) || !hmc377R11Has(values, 5) || hmc377R11Sum(values) > union+0.003 {
			t.Fatalf("the side-file population must carry one own-work account (30) and the wait part (5); got %v", values)
		}
	})
}

func TestHMC377R11ProjectionMergeNeverSumsTheSameInstants(t *testing.T) {
	faces := hmc377R11Query(t, hmc377R11WholeSpanWaitCapture(t), 10.0, 10.3)
	const union = 100.0
	t.Run("projection_nodes", func(t *testing.T) {
		buckets := [][]types.TraceCausalProjectionNode{faces.projection.PrimaryRootCauses, faces.projection.OnChainCauses,
			faces.projection.RankedSeats, faces.projection.SemanticSpans}
		for _, bucket := range buckets {
			for _, node := range bucket {
				if strings.TrimSpace(node.SemanticClass) != "class_verification" {
					continue
				}
				if node.EffectiveImpactMS > union+0.0015 || node.ImpactMS > union+0.0015 || node.CumulativeImpactMS > union+0.0015 {
					t.Fatalf("a merged class-verification node publishes more than the class union %.3f: eff=%.3f impact=%.3f cum=%.3f merged=%d (%s)",
						union, node.EffectiveImpactMS, node.ImpactMS, node.CumulativeImpactMS, node.MergedCount, node.EvidenceID)
				}
			}
		}
	})
	t.Run("side_file", func(t *testing.T) {
		var values []float64
		for _, c := range faces.candidates {
			d := c.Decision
			if d.Token.Token == "class_verification" && d.Magnitude != nil {
				values = append(values, d.Magnitude.Value)
			}
		}
		if len(values) != 2 || !hmc377R11Has(values, 98) || !hmc377R11Has(values, 2) || hmc377R11Sum(values) > union+0.003 {
			t.Fatalf("the side file must carry the wait part (98) and the own work (2), Σ ≤ %.3f; got %v", union, values)
		}
	})
	t.Run("overview_and_tree", func(t *testing.T) {
		var population []float64
		for _, entry := range runtimeTraceProjElimBoard(faces.model) {
			node := entry.row.Node
			if strings.TrimSpace(node.SemanticClass) == "class_verification" {
				population = append(population, node.EffectiveImpactMS)
			}
		}
		rows := hmc377R11ClassValues(runtimeTraceProjSMR1AllRows(&faces.model), runtimeTraceProjNodeDisplayImpact)
		if !hmc377R11Has(population, 98) || !hmc377R11Has(population, 2) || hmc377R11Sum(population) > union+0.003 ||
			len(rows) != 2 || hmc377R11Sum(rows) > union+0.003 {
			t.Fatalf("◎ and tree must carry both parts once (98 + 2 ≤ %.3f); ◎ %v, tree %v", union, population, rows)
		}
	})
}

// The reader faces name what each part's value is: the own-work part is the
// members' instants outside the thread's on-chain waits (never their 合计),
// the wait part is the members' instants inside them, and both disclose the
// record wall beside the value.
func TestHMC377R11OwnWorkPartWordsNameItsInstants(t *testing.T) {
	faces := hmc377R11Query(t, hmc377R11SplitFamilyCapture(t), 10.0, 10.2)
	t.Run("zh", func(t *testing.T) {
		for _, want := range []string{
			"有效归因 30.000ms = 链上等待以外(共3条记录,同线程)(窗口投影合计 35.000ms 见明细)",
			"有效归因 5.000ms = 链上计入(窗口投影合计 20.000ms 见明细)",
			"类校验 有效归因 30.000ms = 链上等待以外(共3条记录,同线程)",
			"·链上等待以外(共3条记录,同线程)",
		} {
			if !strings.Contains(faces.zh, want) {
				t.Errorf("zh face lacks %q", want)
			}
		}
		for _, banned := range []string{"有效归因 30.000ms = 合计", "合计30.000ms"} {
			if strings.Contains(faces.zh, banned) {
				t.Errorf("zh face calls the own-work part a family total: %q", banned)
			}
		}
	})
	t.Run("en", func(t *testing.T) {
		for _, want := range []string{
			"= outside on-chain waits (3 records, same thread)",
			"complete window-projection total 35.000ms",
		} {
			if !strings.Contains(faces.en, want) {
				t.Errorf("en face lacks %q", want)
			}
		}
		if strings.Contains(faces.en, "attribution 30.000ms = total") {
			t.Errorf("en face calls the own-work part a family total")
		}
	})
}

// The w27w2 shape: every side-file item of the class stays within the window,
// the class items never add up above the class union, and no projection node
// of the class publishes more than the union.
func TestHMC377R11WindowSpanWaitPublishesWithinTheWindow(t *testing.T) {
	faces := hmc377R11Query(t, hmc377R11WindowSpanWaitCapture(t), 10.1, 10.2)
	const window = 100.0
	t.Run("side_file", func(t *testing.T) {
		var values []float64
		for _, c := range faces.candidates {
			d := c.Decision
			if d.Magnitude == nil || d.Magnitude.Unit != "ms" {
				continue
			}
			if d.Magnitude.Value > window+0.0015 {
				t.Errorf("side-file item %s %.3f ms exceeds the %.3f ms window", d.Token.Token, d.Magnitude.Value, window)
			}
			if d.Token.Token == "class_verification" {
				values = append(values, d.Magnitude.Value)
			}
		}
		if len(values) != 2 || hmc377R11Sum(values) > window+0.003 {
			t.Fatalf("the class's side-file items must be its two parts adding up to at most the %.3f ms union; got %v", window, values)
		}
	})
	t.Run("projection_nodes", func(t *testing.T) {
		for _, bucket := range [][]types.TraceCausalProjectionNode{faces.projection.PrimaryRootCauses, faces.projection.OnChainCauses,
			faces.projection.RankedSeats, faces.projection.SemanticSpans} {
			for _, node := range bucket {
				if strings.TrimSpace(node.SemanticClass) == "class_verification" &&
					(node.EffectiveImpactMS > window+0.0015 || node.ImpactMS > window+0.0015) {
					t.Fatalf("class-verification node eff=%.3f impact=%.3f merged=%d exceeds the %.3f ms window (%s)",
						node.EffectiveImpactMS, node.ImpactMS, node.MergedCount, window, node.EvidenceID)
				}
			}
		}
	})
}

// hmc377R11NearEqualPartsCapture: main-100 verifies one class [10.030,
// 10.130) and sleeps inside it until waker-150 wakes it — waitMs of the span
// is the wait part, the rest own work (equal, or 2% apart either way), union
// 100 ms.
// Two parts with near values are still two parts (no near-duplicate fold of
// one into the other), whichever of them holds the first seat.
func hmc377R11NearEqualPartsCapture(t *testing.T, waitMs float64) string {
	t.Helper()
	var c hmc376Trace
	c.run(10.000000, "main", 100, 100, 1)
	c.run(10.000100, "waker", 150, 100, 2)
	c.span(10.030000, 10.130000, "main", 100, 100, 1, "VerifyClass com.example.Near")
	c.sleep(10.060000, "main", 100, 100, 1)
	c.add(10.060000+waitMs/1000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.060010+waitMs/1000, "main", 100, 100, 1)
	c.sleep(10.180000, "main", 100, 100, 1)
	return c.write(t)
}

func TestHMC377R11NearEqualPartsStayTwoParts(t *testing.T) {
	const union = 100.0
	for _, tc := range []struct {
		name          string
		waitMs, ownMs float64
	}{{"own_work_first", 49.5, 50.5}, {"wait_part_first", 50.5, 49.5}, {"equal_parts", 50, 50}} {
		t.Run(tc.name, func(t *testing.T) {
			faces := hmc377R11Query(t, hmc377R11NearEqualPartsCapture(t, tc.waitMs), 10.0, 10.2)
			var population, side []float64
			for _, entry := range runtimeTraceProjElimBoard(faces.model) {
				if strings.TrimSpace(entry.row.Node.SemanticClass) == "class_verification" {
					population = append(population, entry.row.Node.EffectiveImpactMS)
				}
			}
			for _, c := range faces.candidates {
				if c.Decision.Token.Token == "class_verification" && c.Decision.Magnitude != nil {
					side = append(side, c.Decision.Magnitude.Value)
				}
			}
			for name, values := range map[string][]float64{"overview": population, "side_file": side} {
				if len(values) != 2 || !hmc377R11Has(values, tc.ownMs) || !hmc377R11Has(values, tc.waitMs) || hmc377R11Sum(values) > union+0.003 {
					t.Errorf("%s must carry the own work (%.1f) and the wait part (%.1f) once each; got %v", name, tc.ownMs, tc.waitMs, values)
				}
			}
		})
	}
}

// hmc377R11CollectionWithRecordedWaitCapture: main-100 runs one collection
// [10.010, 10.050) with a sleep [10.020, 10.030) inside it (own work 30, wait
// part 10), and separately records a wait for another collection [10.100,
// 10.150) asleep until waker-150 wakes it (a recorded wait, 50, never own
// work). Same class, three accounts over disjoint instants, union 90 ms.
func hmc377R11CollectionWithRecordedWaitCapture(t *testing.T) string {
	t.Helper()
	var c hmc376Trace
	c.run(10.000000, "main", 100, 100, 1)
	c.run(10.000100, "waker", 150, 100, 2)
	c.span(10.010000, 10.050000, "main", 100, 100, 1, "H:FullGC::RunPhases;GCReason2")
	c.sleep(10.020000, "main", 100, 100, 1)
	c.add(10.030000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.030010, "main", 100, 100, 1)
	c.add(10.100000, "main", 100, 100, 1, "tracing_mark_write: B|100|GC: Wait For Completion Alloc")
	c.sleep(10.100010, "main", 100, 100, 1)
	c.add(10.150000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.150010, "main", 100, 100, 1)
	c.add(10.150020, "main", 100, 100, 1, "tracing_mark_write: E|100")
	c.sleep(10.180000, "main", 100, 100, 1)
	return c.write(t)
}

// A part of an own-work account never joins a ×N same-kind sum: the
// collection's own work keeps its own ◎ entry beside the wait part and the
// recorded wait of the same class.
func TestHMC377R11OwnWorkAccountNeverJoinsASameKindSum(t *testing.T) {
	faces := hmc377R11Query(t, hmc377R11CollectionWithRecordedWaitCapture(t), 10.0, 10.2)
	var population []float64
	for _, entry := range runtimeTraceProjElimBoard(faces.model) {
		node := entry.row.Node
		if strings.TrimSpace(node.SemanticClass) != "gc_pause" {
			continue
		}
		if node.MergedCount > 1 {
			t.Fatalf("the class's accounts were summed into a ×%d row: eff=%.3f", node.MergedCount, node.EffectiveImpactMS)
		}
		population = append(population, node.EffectiveImpactMS)
	}
	if !hmc377R11Has(population, 30) || hmc377R11Sum(population) > 90+0.004 {
		t.Fatalf("◎ must keep the collection's own work (30) as its own entry, Σ ≤ 90; got %v", population)
	}
	var side []float64
	for _, c := range faces.candidates {
		if c.Decision.Token.Token == "gc_pause" && c.Decision.Magnitude != nil {
			side = append(side, c.Decision.Magnitude.Value)
		}
	}
	// The side file keeps the three accounts of the class over their
	// disjoint instants: the recorded wait, the own work, the wait part.
	if len(side) != 3 || !hmc377R11Has(side, 30) || !hmc377R11Has(side, 10) || hmc377R11Sum(side) > 90+0.004 {
		t.Fatalf("the side file must keep the own work (30) and the wait part (10) beside the recorded wait, Σ ≤ 90; got %v", side)
	}
}

// hmc377R11OwnWorkAccountProjection is the representative legend fixture of
// a split own-work account (the ybb2 shape): the own-work record (108
// members, wall 40.085, own work 37.945, rank 1) and the wait record of the
// same spans (one member, wall 6.687, chain-wait part 2.140, rank 7), both
// carrying the engine's account identity, beside one chain row.
func hmc377R11OwnWorkAccountProjection() types.TraceCausalProjection {
	const account = "58533:class_verification:class_verification:0123456789abcdef"
	own := xlane2SemanticFamilyNode("hmc377r11-own", 108, [][2]int{{78636, 79000}, {79397, 80749}, {81000, 121456}}, 37.945, 40.085)
	own.Subject = "unyuan.app.chat-58533"
	own.FamilyMemberLineRanges = nil
	own.FamilyFoldCaliber = "sum_disjoint"
	own.ChainRelevance, own.Causality, own.OnChainBasis = "on_chain", "self_deterministic", "self_deterministic_span"
	own.Rank, own.Tier = 1, "primary"
	own.OwnWorkAccount, own.OwnWorkAccountPart = account, types.TraceOwnWorkAccountPartOwnWork
	wait := xlane2SemanticFamilyNode("hmc377r11-wait", 1, [][2]int{{79397, 80749}}, 2.140, 6.687)
	wait.Subject = "unyuan.app.chat-58533"
	wait.FamilyMemberCount, wait.FamilyMemberLineRanges, wait.FamilyFoldCaliber = 0, nil, ""
	wait.ChainRelevance, wait.Causality = "on_chain", "on_wakeup_chain"
	wait.SemanticChainProjectedMS = 2.140
	wait.Rank, wait.Tier = 7, "tertiary"
	wait.OwnWorkAccount, wait.OwnWorkAccountPart = account, types.TraceOwnWorkAccountPartChainWait
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"PreloadTimeoutT-58693", "unyuan.app.chat-58533"},
		WindowStartTs:           10834.099821,
		WindowEndTs:             10834.620005,
		OnChainCauses: []types.TraceCausalProjectionNode{
			elimChainNode("hmc377r11-dep", "PreloadTimeoutT-58693", "runnable_wait", "runnable", 2, 7.965, 300),
			// A D-state chain row keeps the ⛓ glyph's icon legend lit beside
			// the ◎ channel word (the representative-fixture convention).
			elimChainNode("hmc377r11-dio", "binder:15387_1-58574", "d_state_or_io_wait", "d_sleep", 3, 5.367, 320),
		},
		SemanticSpans: []types.TraceCausalProjectionNode{own, wait},
	}
}

// The fixture draws each part once at its own instants and names the own
// part's value with its own word (also the legend sweep's witness of it).
func TestHMC377R11OwnWorkAccountProjectionFixtureFaces(t *testing.T) {
	for _, zh := range []bool{true, false} {
		t.Run(map[bool]string{true: "zh", false: "en"}[zh], func(t *testing.T) {
			model := buildRuntimeTraceProjTreeModel(hmc377R11OwnWorkAccountProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			values := hmc377R11SubjectClassValues(runtimeTraceProjSMR1AllRows(&model), "unyuan.app.chat-58533", runtimeTraceProjNodeDisplayImpact)
			if len(values) != 2 || !hmc377R11Has(values, 37.945) || !hmc377R11Has(values, 2.140) {
				t.Fatalf("each part once at its own instants (37.945 + 2.140 = 40.085); got %v", values)
			}
			fence := runtimeTraceProjTreeFence(model, zh)
			word := map[bool]string{true: "有效归因 37.945ms = 链上等待以外(共108条记录,同线程)(窗口投影合计 40.085ms 见明细)",
				false: "outside on-chain waits (108 records, same thread)"}[zh]
			if !strings.Contains(fence, word) || !model.Marks.has(runtimeTraceProjMarkFamilyOwnWorkOutsideChainWait) {
				t.Fatalf("the own-work part's line must speak its own word %q and light its legend entry:\n%s", word, fence)
			}
		})
	}
}

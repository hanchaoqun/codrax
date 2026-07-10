package tool

// answer_document_projection_semlead_test.go — SEM-LEAD display-half pins
// (ledger real_trace_campaign_20260705.md §29.7-2, 2026-07-10):
//
//	① board/lead/❶❷❸ fully open for ON-CHAIN semantic rows — the ✦ 语义 row
//	  carrying the engine rank seat joins the shared rank board, wears the
//	  ❶ badge and crowns 主根因 (792-textup "主根因: Texture upload" 追认);
//	② the published effective attribution is the family REAL total — no
//	  boosted 表值 anywhere on the answer surface;
//	③ E9/E13 双席合一 — the rank-lane twin folds into the ✦ row (one E# seat,
//	  merged [E#+E#] bracket, detail 已并入 disclosure), never two rows for
//	  one 11-span family;
//	④ 行1/词值同源 class word — a semantic FAMILY row speaks the typed class
//	  word on the tree 行1 AND the (a) key-metric table, never one member's
//	  span name.
//
// Fixture discipline (引擎实铸): the observation records are minted by the
// REAL engine + the REAL production emission — BuildIndex → BuildWakeupChain /
// BuildRootCauseRank / ComputeWindowStats → traceQueryTypedObservations —
// never hand-written note shapes.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const semLeadToolTextureTrace = `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.000400: tracing_mark_write: B|200|Texture upload(15573) 1140x1856
     worker-200 (100) [002] .... 5.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (100) [002] .... 5.002500: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.002600: tracing_mark_write: B|200|Texture upload(15563) 1140x1140
     worker-200 (100) [002] .... 5.005800: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.006000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.006500: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func semLeadEngineObservations(t *testing.T) []types.ObservationRecord {
	t.Helper()
	path := filepath.Join(t.TempDir(), "semlead_textup.systrace")
	if err := os.WriteFile(path, []byte(semLeadToolTextureTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := tracequery.Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.007, MaxDepth: 4,
		MinDurationMs: 0.05, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	chain := tracequery.BuildWakeupChain(idx, q)
	rank := tracequery.BuildRootCauseRank(idx, q)
	stats := tracequery.ComputeWindowStats(idx, q)
	result := tracequery.Result{WakeupChain: &chain, RootCauseRank: &rank, WindowStats: &stats}
	return traceQueryTypedObservations(result, "semlead_textup.systrace", "payload", "raw", "",
		time.Unix(1751600000, 0).UTC())
}

func TestSemLeadOnChainSemanticFamilySingleSeatCrownedZH(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), semLeadEngineObservations(t), "")

	// ① 主根因 crown through the primary lane, named by subject + class.
	leadLine := ""
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "**主根因:**") {
			leadLine = line
			break
		}
	}
	if leadLine == "" || !strings.Contains(leadLine, "worker-200") || !strings.Contains(leadLine, "Texture upload") {
		t.Fatalf("the on-chain semantic family must crown 主根因 (§29.7-2 ①), got %q in:\n%s", leadLine, md)
	}
	if strings.Contains(md, "未定位到链上主根因;窗口内最大语义优化span") {
		t.Fatalf("a rank-seated semantic lead must use the primary lane, not the tier-4 fallback:\n%s", md)
	}

	// ① ❶ badge lands on the semantic family row (board seat 1), which also
	// wears the rank ordinal + tier word on 行2.
	fenceRow := ""
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "Texture upload ×2") && strings.Contains(line, "✦") {
			fenceRow = line
			break
		}
	}
	if fenceRow == "" || !strings.Contains(fenceRow, "❶") {
		t.Fatalf("the semantic family fence row must wear the ❶ badge, got %q in:\n%s", fenceRow, md)
	}
	if !strings.Contains(md, "确定性优化候选·根因排序#1") {
		t.Fatalf("行2 must carry the tier word + adopted rank seat:\n%s", md)
	}

	// ③ single seat: the rank-lane twin folded in — merged evidence bracket +
	// detail disclosure present, and no second texture row anywhere (the E13
	// witness form: one member's span name impersonating the family).
	if !regexp.MustCompile(`\[E\d+(\(\+\d+\))?\+E\d+(\(\+\d+\))?\]`).MatchString(md) {
		t.Fatalf("the folded rank twin's E# must merge into the [E#+E#] bracket:\n%s", md)
	}
	if !strings.Contains(md, "已并入本行,数值不重复计入") {
		t.Fatalf("the detail block must disclose the folded rank row:\n%s", md)
	}
	if strings.Contains(md, "Texture upload(15573) 1140x1856(Texture upload)") {
		t.Fatalf("the rank-lane twin must not render its own row (E13 双席形):\n%s", md)
	}
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "Texture upload") &&
			(strings.Contains(line, "链上·未接入树") || strings.Contains(line, "链上·深度未解析")) {
			t.Fatalf("no unattached rank-lane texture seat may remain: %q", line)
		}
	}

	// ④ 词值同源: the (a) key-metric table speaks the class word, exactly one
	// texture row, never a member span name in the node cell.
	tableRows := regexp.MustCompile(`(?m)^\|.*Texture upload ×2合计.*$`).FindAllString(md, -1)
	if len(tableRows) != 1 {
		t.Fatalf("the key-metric table must seat the family exactly once with the class word, got %d:\n%s", len(tableRows), md)
	}
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "|") && strings.Contains(line, "Texture upload(15573)") &&
			!strings.Contains(line, "成员") {
			// Roster sub-rows (·成员 …) keep the real span names lossless
			// (§24.7.1 ① 区分键不能丢); every NODE cell speaks the class word.
			t.Fatalf("a table node cell must never speak one member's span name (§29.7-2 ④): %q", line)
		}
	}

	// ② the published value is the REAL family total (5.300ms = 2.100+3.200);
	// the boosted score channel (×2.10 → 11.130ms) never reaches the answer.
	if !strings.Contains(md, "合计5.300ms") {
		t.Fatalf("the family row must publish the real total 合计5.300ms:\n%s", md)
	}
	if strings.Contains(md, "11.130") {
		t.Fatalf("the boosted internal ranking value must never surface (§29.7-2 ②):\n%s", md)
	}
	if strings.Contains(md, "hidden_cost_boost") || strings.Contains(md, "semantic_multiplier") {
		t.Fatalf("internal ranking tokens must never surface:\n%s", md)
	}

	// Roster members stay lossless on the sub-rows (§24.7.1 ① 区分键不能丢).
	if !strings.Contains(md, "成员 Texture upload(15573) 1140x1856") {
		t.Fatalf("the member roster must keep the real span names:\n%s", md)
	}
}

const semLeadTopNOverflowTrace = `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.000200: tracing_mark_write: B|200|JIT compiling Foo.bar
     worker-200 (100) [002] .... 5.001000: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.001100: tracing_mark_write: B|200|VerifyClass com.example.Foo
     worker-200 (100) [002] .... 5.002000: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.002100: tracing_mark_write: B|200|ShaderCompile pipeline warmup
     worker-200 (100) [002] .... 5.003000: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.003100: tracing_mark_write: B|200|Ark runtime compile module
     worker-200 (100) [002] .... 5.004000: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.004100: tracing_mark_write: B|200|Texture upload(7) 64x64
     worker-200 (100) [002] .... 5.005000: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.005100: tracing_mark_write: B|200|H:GC pause young
     worker-200 (100) [002] .... 5.006000: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.006500: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.007000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func TestSemLeadOnChainSemanticOutsideRootTopNStillMandatoryOptimizationMention(t *testing.T) {
	q := tracequery.Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.008, MaxDepth: 4,
		MinDurationMs: 0.05, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 3}
	obs := semLeadEngineObservationsFromTrace(t, "semlead_topn_overflow.systrace", semLeadTopNOverflowTrace, q)
	semanticClasses := map[string]bool{}
	rankedSemantic := 0
	for _, record := range obs {
		if record.Predicate == "trace_semantic_span" {
			semanticClasses[strings.TrimSpace(record.Object)] = true
		}
		if strings.HasPrefix(record.Predicate, "root_cause_") &&
			traceQueryRootCauseItemIsSemanticSpanWork(strings.TrimSpace(record.Object)) {
			rankedSemantic++
		}
	}
	if len(semanticClasses) != 6 {
		t.Fatalf("independent semantic channel must retain all six on-chain families: classes=%v observations=%+v", semanticClasses, obs)
	}
	if rankedSemantic >= len(semanticClasses) || rankedSemantic > 3 {
		t.Fatalf("fixture must force semantic families outside root TOP 3: ranked=%d independent=%d", rankedSemantic, len(semanticClasses))
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: obs})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %+v", set)
	}
	for _, node := range set.Projections[0].BackgroundCauses {
		if strings.TrimSpace(node.SemanticClass) != "" &&
			(strings.TrimSpace(node.ChainRelevance) == "on_chain" || strings.TrimSpace(node.Causality) == "on_wakeup_chain") {
			t.Fatalf("on-chain semantic work must never enter BackgroundCauses: %+v", node)
		}
	}
	md := audit730Render(t, audit730Bus(""), obs, "")
	if !strings.Contains(md, "确定性优化点") {
		t.Fatalf("mandatory semantic optimization block missing:\n%s", md)
	}
	for class := range semanticClasses {
		if !strings.Contains(md, class) {
			t.Fatalf("TOP-N-evicted semantic class %q must still be mentioned in the optimization block:\n%s", class, md)
		}
	}
}

func TestSemLeadOnChainSemanticNeverDemotesToBackgroundWhenTreeAttachmentMissing(t *testing.T) {
	base := types.TraceCausalProjectionNode{
		Subject: "unknown-thread", SemanticClass: "gc_pause", ChainDepth: 0,
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
	}
	byRelevance := base
	byRelevance.ChainRelevance = "on_chain"
	if runtimeTraceProjNodeDemotedToBackground(byRelevance, 3) {
		t.Fatalf("typed on_chain semantic row must keep an on-chain/depthless seat: %+v", byRelevance)
	}
	byCausality := base
	byCausality.Causality = "on_wakeup_chain"
	if runtimeTraceProjNodeDemotedToBackground(byCausality, 3) {
		t.Fatalf("typed on_wakeup_chain semantic row must keep an on-chain/depthless seat: %+v", byCausality)
	}
	offChain := base
	offChain.ChainRelevance = "background"
	offChain.Causality = "background"
	if !runtimeTraceProjNodeDemotedToBackground(offChain, 3) {
		t.Fatalf("unknown-subject off-chain row may still demote to background: %+v", offChain)
	}
}

// Fold precision guards (fail-open to the two-row render): value mismatch,
// member-count mismatch, relevance-lane mismatch, ambiguity, and cross-window
// pairs never fold. Unit-grain nodes (RNB
// TestRNBSameSegmentTwinFoldUnitGuards convention — the guards are pure
// typed-field comparisons).
func TestSemLeadSemanticRankTwinFoldUnitGuards(t *testing.T) {
	semantic := func() types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleSemanticSpan, Subject: "worker-200",
			Predicate: "trace_semantic_span", SemanticClass: "texture_upload",
			ChainRelevance: "on_chain", ImpactMS: 5.3, FamilyMemberCount: 2,
			LineStart: 3, LineEnd: 7, EvidenceID: "sem",
		}
	}
	rank := func() types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-200",
			Predicate: "root_cause_deterministic_optimization", SemanticClass: "texture_upload",
			ChainRelevance: "on_chain", ImpactMS: 5.3, EffectiveImpactMS: 5.3,
			CumulativeImpactMS: 5.3, FamilyMemberCount: 2, Rank: 1,
			Tier: "deterministic_optimization", LineStart: 3, LineEnd: 7, EvidenceID: "rank",
		}
	}
	fold := func(chainNodes, semantics []types.TraceCausalProjectionNode) (int, int, bool) {
		outChain, outSem, peers := runtimeTraceProjFoldSemanticRankLaneTwins(chainNodes, semantics)
		return len(outChain), outSem[0].Rank, len(peers) > 0
	}

	// Positive control: the engine-mirror pair folds and adopts the seat.
	if n, adopted, folded := fold([]types.TraceCausalProjectionNode{rank()},
		[]types.TraceCausalProjectionNode{semantic()}); n != 0 || adopted != 1 || !folded {
		t.Fatalf("the mirror pair must fold (n=%d rank=%d folded=%v)", n, adopted, folded)
	}
	// B9 positive control: an exact adjacent pair also folds, but the semantic
	// survivor keeps the background-board identity and never adopts an
	// on-chain root-cause ordinal.
	r := rank()
	r.ChainRelevance = "adjacent"
	r.Causality = "adjacent_to_wakeup_chain"
	r.BackgroundRank = 2
	s := semantic()
	s.ChainRelevance = "adjacent"
	s.Causality = "adjacent_to_wakeup_chain"
	outRank, outSem, peers := runtimeTraceProjFoldSemanticRankLaneTwins(
		[]types.TraceCausalProjectionNode{r}, []types.TraceCausalProjectionNode{s})
	if len(outRank) != 0 || len(outSem) != 1 || outSem[0].Rank != 0 ||
		outSem[0].BackgroundRank != 2 || len(peers) != 1 {
		t.Fatalf("an exact adjacent twin must fold onto one background-board semantic seat: rank=%+v sem=%+v peers=%+v",
			outRank, outSem, peers)
	}
	// Value mismatch — a different accounting never folds.
	r = rank()
	r.ImpactMS = 9.9
	if n, adopted, folded := fold([]types.TraceCausalProjectionNode{r},
		[]types.TraceCausalProjectionNode{semantic()}); n != 1 || adopted != 0 || folded {
		t.Fatalf("a value mismatch must fail open (n=%d rank=%d folded=%v)", n, adopted, folded)
	}
	// Member-count mismatch never folds.
	r = rank()
	r.FamilyMemberCount = 3
	if n, adopted, folded := fold([]types.TraceCausalProjectionNode{r},
		[]types.TraceCausalProjectionNode{semantic()}); n != 1 || adopted != 0 || folded {
		t.Fatalf("a member-count mismatch must fail open (n=%d rank=%d folded=%v)", n, adopted, folded)
	}
	// Different relevance lanes never fold: an adjacent rank arm cannot merge
	// into an on-chain semantic entity.
	r = rank()
	r.ChainRelevance = "adjacent"
	if n, adopted, folded := fold([]types.TraceCausalProjectionNode{r},
		[]types.TraceCausalProjectionNode{semantic()}); n != 1 || adopted != 0 || folded {
		t.Fatalf("a cross-lane rank twin must fail open (n=%d rank=%d folded=%v)", n, adopted, folded)
	}
	// Ambiguity (two rank arms under one key) never folds.
	if n, adopted, folded := fold([]types.TraceCausalProjectionNode{rank(), rank()},
		[]types.TraceCausalProjectionNode{semantic()}); n != 2 || adopted != 0 || folded {
		t.Fatalf("an ambiguous key must fail open (n=%d rank=%d folded=%v)", n, adopted, folded)
	}
	// Cross-window pairs never fold (SFD F1 mirror).
	r = rank()
	r.QueryWindowStartTs, r.QueryWindowEndTs = 5.0, 5.007
	s = semantic()
	s.QueryWindowStartTs, s.QueryWindowEndTs = 9.0, 9.007
	if n, adopted, folded := fold([]types.TraceCausalProjectionNode{r},
		[]types.TraceCausalProjectionNode{s}); n != 1 || adopted != 0 || folded {
		t.Fatalf("a cross-window pair must fail open (n=%d rank=%d folded=%v)", n, adopted, folded)
	}
	// Different line envelope = different entity — never folds.
	r = rank()
	r.LineStart, r.LineEnd = 10, 12
	if n, adopted, folded := fold([]types.TraceCausalProjectionNode{r},
		[]types.TraceCausalProjectionNode{semantic()}); n != 1 || adopted != 0 || folded {
		t.Fatalf("a different line envelope must fail open (n=%d rank=%d folded=%v)", n, adopted, folded)
	}
}

// TestB9CustTraceVC710OffChainClassVerificationTwinSingleSeat mirrors the
// production witness cust_trace_vc_710.txt: one off-chain VerifyClass family
// (ease.cloudmusic-63993 ×10) was published through trace_semantic_span and
// root-cause-rank channels. The report must keep exactly one adjacent seat,
// never mint a causal-tree `├─语义─` seat, retain the rank record as evidence,
// and independently retain the deterministic-optimization obligation.
func TestB9CustTraceVC710OffChainClassVerificationTwinSingleSeat(t *testing.T) {
	semantic := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "E27",
		Subject: "ease.cloudmusic-63993", Predicate: "trace_semantic_span",
		Object: "class_verification", SpanName: "VerifyClass java.lang.String",
		SemanticClass: "class_verification", ChainRelevance: "adjacent",
		Causality: "adjacent_to_wakeup_chain", BackgroundRank: 1,
		ImpactMS: 6.400, EffectiveImpactMS: 6.400, CumulativeImpactMS: 6.400,
		FamilyMemberCount: 10, FamilyMemberMinMS: 0.200, FamilyMemberMaxMS: 1.100,
		FamilyFoldCaliber: "sum_disjoint",
		FamilyMemberRoster: []string{
			"VerifyClass java.lang.String 1.100ms",
			"VerifyClass java.util.HashMap 0.900ms",
			"VerifyClass android.view.View 0.800ms",
		},
		LineStart: 2700, LineEnd: 2799, StartTs: 5.0002, EndTs: 5.0066,
		QueryWindowStartTs: 5.000, QueryWindowEndTs: 5.007,
		Confidence: 0.91,
	}
	rank := semantic
	rank.Role = types.TraceCausalRoleRootCauseContext
	rank.EvidenceID = "E32"
	rank.Predicate = "root_cause_deterministic_optimization"
	rank.SpanName = ""
	rank.Rank = 7
	rank.BackgroundRank = 2
	rank.Tier = "deterministic_optimization"

	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"render-worker-7", "app-100"},
		WindowStartTs: 5.000, WindowEndTs: 5.007,
		// The production projection carried both observation-channel copies in
		// the adjacent bucket; this is the exact double-seat precondition.
		AdjacentCauses: []types.TraceCausalProjectionNode{rank, semantic},
		SemanticSpans:  []types.TraceCausalProjectionNode{semantic},
	}
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(projection, evidence, true)

	isWitness := func(row runtimeTraceProjTreeRow) bool {
		return row.Node.Subject == semantic.Subject && row.Node.SemanticClass == semantic.SemanticClass
	}
	count := func(rows []runtimeTraceProjTreeRow) int {
		n := 0
		for _, row := range rows {
			if isWitness(row) {
				n++
			}
		}
		return n
	}
	if got := count(model.TreeRows); got != 0 {
		t.Fatalf("off-chain semantic work must never enter the causal main tree, got %d rows: %+v", got, model.TreeRows)
	}
	if got := count(model.Adjacent); got != 1 {
		t.Fatalf("the twin must keep exactly one adjacent publication seat, got %d: %+v", got, model.Adjacent)
	}
	if got := count(model.Background); got != 0 {
		t.Fatalf("the adjacent witness must not gain a second background seat, got %d: %+v", got, model.Background)
	}
	var survivor runtimeTraceProjTreeRow
	for _, row := range model.Adjacent {
		if isWitness(row) {
			survivor = row
			break
		}
	}
	if survivor.Node.Rank != 0 || survivor.Node.BackgroundRank != 2 {
		t.Fatalf("the off-chain survivor must retain only its background-board identity: %+v", survivor.Node)
	}
	if len(survivor.RankFoldPeers) != 1 || survivor.RankFoldPeers[0].Rank != 0 ||
		strings.TrimSpace(survivor.RankFoldPeers[0].EvidenceTag) == "" {
		t.Fatalf("the folded rank record must remain an evidence carrier without an on-chain ordinal: %+v", survivor.RankFoldPeers)
	}

	detailRows := runtimeTraceProjDetailRows(model)
	if got := count(detailRows); got != 1 {
		t.Fatalf("the lossless detail inventory must expose one physical family row, got %d: %+v", got, detailRows)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "├─语义─") && strings.Contains(fence, "VerifyClass") {
		t.Fatalf("off-chain VerifyClass must not render in the causal tree:\n%s", fence)
	}
	if !strings.Contains(fence, "邻近区段") || !strings.Contains(fence, "背景榜位#2") {
		t.Fatalf("the single adjacent seat must keep its background-board identity:\n%s", fence)
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "rank行[") || !strings.Contains(detail, "已并入本行,数值不重复计入") {
		t.Fatalf("the detail surface must disclose the folded rank record without a second value:\n%s", detail)
	}
	if len(evidence.order) != 2 {
		t.Fatalf("both observation records must remain in the evidence index, got %d: %+v", len(evidence.order), evidence.order)
	}

	_, optimizationRows := runtimeTraceSemanticOptimizationParts(projection,
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if len(optimizationRows) == 0 || !strings.Contains(strings.Join(optimizationRows[0].Cells, "|"), "class_verification") {
		t.Fatalf("off-chain folding must not remove the independent deterministic-optimization obligation: %+v", optimizationRows)
	}
}

// Non-chain control (§29.7-2 后半不变): a background semantic family keeps the
// background comprehensive board identity — no board seat, no ❶, no 主根因,
// 行2 speaks 背景榜位#N. Hand-shaped from the production record forms (the
// non-chain lane is bytes-untouched by this batch; the control pins that).
func TestSemLeadNonChainSemanticFamilyStaysOnBackgroundBoard(t *testing.T) {
	obs := []types.ObservationRecord{
		rnbAnchor(),
		rnbPath("worker-9 -> app-100"),
		projV3Obs("chain-root", "root_cause_primary", "root_cause_primary:worker",
			"worker-9", "d_state_or_io_wait", "12.000", 12.0, 100, 200,
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=1", "dominant_state=d_state_or_io_wait", "effective_impact_ms=12.000"),
		{
			ID: "sem-bg", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan,
			ClaimKey: "trace_semantic_span:shader_compile", Subject: "gpu-worker-7",
			Predicate: "trace_semantic_span", Object: "shader_compile",
			Value: "4.400", Unit: "ms",
			Span:    types.ObservationSpan{LineStart: 300, LineEnd: 320, StartTs: 100.01, EndTs: 100.05},
			Summary: "semantic trace span family class=shader_compile x2 same-thread span(s) totalled 4.400ms",
			RichNotes: []string{
				"span_name=shader_compile", "semantic_class=shader_compile",
				"chain_relevance=background", "causality=background",
				"background_rank=1", "member_count=2", "member_max_ms=3.000",
				"member_min_ms=1.400", "member_fold_caliber=sum_disjoint",
				"member_roster=shader_compile 3.000ms | shader_compile 1.400ms",
			},
		},
	}
	md := audit730Render(t, audit730Bus(""), obs, "")
	if !strings.Contains(md, "背景榜位#1") {
		t.Fatalf("the non-chain family keeps its background board seat:\n%s", md)
	}
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "shader_compile") && strings.Contains(line, "❶") {
			t.Fatalf("a non-chain semantic row must never wear ❶: %q", line)
		}
	}
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "**主根因:**") && strings.Contains(line, "shader_compile") {
			t.Fatalf("a non-chain semantic row must never crown 主根因: %q", line)
		}
	}
}

// --- 复核 P1-1 / P2-1 pins ------------------------------------------------------

// semLeadRealBelowPrimaryToolTrace mirrors the engine-half real<primary
// fixture (semantic_lead_semlead_test.go): D/IO 8.100ms > texture real total
// 5.300ms while the internal boost (11.130ms) would have flipped the order.
const semLeadRealBelowPrimaryToolTrace = `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.000400: tracing_mark_write: B|200|Texture upload(15573) 1140x1856
     worker-200 (100) [002] .... 5.002500: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.002600: tracing_mark_write: B|200|Texture upload(15563) 1140x1140
     worker-200 (100) [002] .... 5.005800: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.005900: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (100) [002] .... 5.006000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
      irq-2 (2) [002] .... 5.006100: sched_blocked_reason: pid=200 iowait=1 caller=f2fs_wait_on_block
      irq-2 (2) [002] .... 5.014100: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002
     worker-200 (100) [002] .... 5.014200: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (100) [002] .... 5.014300: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 5.014400: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.014800: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func semLeadEngineObservationsFromTrace(t *testing.T, name, content string, q tracequery.Query) []types.ObservationRecord {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	chain := tracequery.BuildWakeupChain(idx, q)
	rank := tracequery.BuildRootCauseRank(idx, q)
	stats := tracequery.ComputeWindowStats(idx, q)
	result := tracequery.Result{WakeupChain: &chain, RootCauseRank: &rank, WindowStats: &stats}
	return traceQueryTypedObservations(result, name, "payload", "raw", "",
		time.Unix(1751600000, 0).UTC())
}

// semLeadNodeBlockLines returns the fence row line containing marker plus its
// subordinate "·" continuation lines (the node's 行2..N block).
func semLeadNodeBlockLines(md, marker string) []string {
	lines := strings.Split(md, "\n")
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if !inFence || !strings.Contains(line, marker) {
			continue
		}
		block := []string{line}
		for j := i + 1; j < len(lines); j++ {
			trimmed := strings.TrimLeft(lines[j], "│ \t")
			if strings.HasPrefix(trimmed, "·") || strings.HasPrefix(trimmed, "└") && strings.Contains(trimmed, "·") {
				block = append(block, lines[j])
				continue
			}
			break
		}
		return block
	}
	return nil
}

// TestSemLeadBadgeOrdinalConsistencyRealBelowPrimary — 复核 P1-1 witness
// face: on the real<primary form the ❶ badge row and the 根因排序#1 ordinal
// are the SAME node (序数 ≡ 徽章 ≡ 图例; the review caught ❶ paired with
// 根因排序#2 on one page), and the semantic family wears its honest lower
// ordinal while staying published with its tier word (参赛+提及地板不回退).
func TestSemLeadBadgeOrdinalConsistencyRealBelowPrimary(t *testing.T) {
	obs := semLeadEngineObservationsFromTrace(t, "semlead_real_below_primary.systrace",
		semLeadRealBelowPrimaryToolTrace,
		tracequery.Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.015, MaxDepth: 4,
			MinDurationMs: 0.05, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12})
	md := audit730Render(t, audit730Bus(""), obs, "")
	badge := semLeadNodeBlockLines(md, "❶")
	if len(badge) == 0 {
		t.Fatalf("expected a ❶ badge row:\n%s", md)
	}
	badgeBlock := strings.Join(badge, "\n")
	if !strings.Contains(badgeBlock, "根因排序#1") {
		t.Fatalf("the ❶ row must be the 根因排序#1 node (序数≡徽章):\n%s\n---\n%s", badgeBlock, md)
	}
	if strings.Contains(badgeBlock, "Texture upload") {
		t.Fatalf("the semantic family (real 5.300 < 8.100) must not wear ❶ on this form:\n%s", badgeBlock)
	}
	tex := semLeadNodeBlockLines(md, "Texture upload ×2")
	if len(tex) == 0 {
		t.Fatalf("the semantic family row must stay published:\n%s", md)
	}
	texBlock := strings.Join(tex, "\n")
	if !strings.Contains(texBlock, "确定性优化候选") {
		t.Fatalf("the family keeps its tier word (提及地板 carrier):\n%s", texBlock)
	}
	m := regexp.MustCompile(`根因排序#(\d+)`).FindStringSubmatch(texBlock)
	if len(m) != 2 || m[1] == "1" {
		t.Fatalf("the family must wear its honest lower ordinal (got %v):\n%s", m, texBlock)
	}
	if strings.Contains(md, "11.130") {
		t.Fatalf("the boosted internal value must never surface:\n%s", md)
	}
}

// semLeadPureSemanticBoardTrace — 复核 P2-1: a window whose ONLY on-chain
// competitor is the semantic family. The worker has no sched intervals (marks
// + wakeup only) and the target's runnable slice sits under the 0.05ms floor,
// so the semantic family itself must mint the sole primary-tier row.
const semLeadPureSemanticBoardTrace = `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.000400: tracing_mark_write: B|200|Texture upload(15573) 1140x1856
     worker-200 (100) [002] .... 5.002500: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.002600: tracing_mark_write: B|200|Texture upload(15563) 1140x1140
     worker-200 (100) [002] .... 5.005800: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.006000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func TestSemLeadPureSemanticBoardMintsTypedPrimaryAndCrowns(t *testing.T) {
	q := tracequery.Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.007, MaxDepth: 4,
		MinDurationMs: 0.05, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	obs := semLeadEngineObservationsFromTrace(t, "semlead_pure_semantic.systrace",
		semLeadPureSemanticBoardTrace, q)
	primarySemantic := false
	for _, record := range obs {
		if strings.HasPrefix(strings.TrimSpace(record.Predicate), "root_cause_primary") &&
			strings.TrimSpace(record.Object) == "texture_upload" {
			primarySemantic = true
		}
	}
	if !primarySemantic {
		t.Fatalf("the on-chain semantic family must mint a typed primary record: %+v", obs)
	}
	md := audit730Render(t, audit730Bus(""), obs, "")
	leadLine := ""
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "**主根因:**") {
			leadLine = line
			break
		}
	}
	if leadLine == "" || !strings.Contains(leadLine, "Texture upload") || strings.Contains(leadLine, "未定位到链上主根因") {
		t.Fatalf("the pure-semantic typed primary must crown (§29.7-2 可登顶), got %q in:\n%s", leadLine, md)
	}
	badge := semLeadNodeBlockLines(md, "❶")
	if len(badge) == 0 || !strings.Contains(strings.Join(badge, "\n"), "Texture upload") {
		t.Fatalf("❶ must seat on the semantic family:\n%s", md)
	}
}

// TestSemLeadEmptyPrimaryArmControls — the crowning arm's negative controls:
// a NON-semantic board[0] on an empty primary bucket keeps the legacy
// no-conclusion nil (never mis-crowned), and a board emptied by symptom-tier
// rank=0 rows plus an unranked semantic row stays nil.
func TestSemLeadEmptyPrimaryArmControls(t *testing.T) {
	nonSemantic := types.TraceCausalProjection{
		WakeupPath: []string{"worker-9", "app-1"},
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, Subject: "worker-9",
			Predicate: "wakeup_causal_impact", Object: "d_state_or_io_wait",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			ImpactMS: 8.1, EffectiveImpactMS: 8.1, Rank: 1,
			LineStart: 10, LineEnd: 20, EvidenceID: "hop",
		}},
	}
	model := buildRuntimeTraceProjTreeModel(nonSemantic, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if lead, lane := runtimeTraceProjLeadSelect(nonSemantic, model); lead != nil || lane != runtimeTraceProjLeadLaneNone {
		t.Fatalf("a non-semantic board[0] must keep the legacy empty-primary nil, got %+v lane=%d", lead, lane)
	}
	symptomOnly := types.TraceCausalProjection{
		WakeupPath: []string{"worker-9", "app-1"},
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, Subject: "app-1",
			Predicate: "root_cause_target_self_state", Object: "sleep_wait",
			Tier: "target_self_state", ChainRelevance: "on_chain", ChainDepth: 0,
			ImpactMS: 6.0, EffectiveImpactMS: 6.0, Rank: 0,
			LineStart: 30, LineEnd: 40, EvidenceID: "self",
		}},
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, Subject: "worker-9",
			Predicate: "trace_semantic_span", SemanticClass: "texture_upload",
			ChainRelevance: "on_chain", ImpactMS: 5.3,
			LineStart: 3, LineEnd: 7, EvidenceID: "sem",
		}},
	}
	model = buildRuntimeTraceProjTreeModel(symptomOnly, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if lead, lane := runtimeTraceProjLeadSelect(symptomOnly, model); lead != nil || lane != runtimeTraceProjLeadLaneNone {
		t.Fatalf("symptom rank=0 rows + an unranked semantic row must stay nil, got %+v lane=%d", lead, lane)
	}
}

package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_projection_twodim_test.go — TWODIM-1 (§18 双维度审计处置,
// user ruling 2026-07-28): root causes have TWO dimensions — the rule-priced
// eliminable board AND raw time occupancy guiding NEW fix directions. These
// pins cover the G1 outlet (the 未计价占用 aux account for on-chain
// context-only genuine occupancy) and the teaching word faces.

func twodimProjectionWithUnpricedRunning() types.TraceCausalProjection {
	projection := elimBoardProjection()
	unpriced := elimChainNode("E-ctx", "worker-7777", "running", "running", 0, 41.500, 400)
	unpriced.Tier = types.TraceCausalTierContextOnly
	unpriced.Rank = 0
	unpriced.ChainRelevance = "on_chain"
	projection.OnChainCauses = append(projection.OnChainCauses, unpriced)
	return projection
}

// G1: an on-chain row whose raw occupancy is genuine but priced to zero
// (context_only) rides the 排除≠消失 aux account with the own-workload lever
// — it must never silently vanish from the ◎ guidance page again.
func TestTwoDimUnpricedOccupancyAuxRow(t *testing.T) {
	_, fence := elimRenderOverview(t, twodimProjectionWithUnpricedRunning(), true)
	if !strings.Contains(fence, "· 未计价占用") ||
		!strings.Contains(fence, "真实占时·杠杆=自身工作量(新方向)") {
		t.Fatalf("the unpriced-occupancy aux row must render with the own-workload lever:\n%s", fence)
	}
	if !strings.Contains(fence, "41.500ms") {
		t.Fatalf("the aux row must carry the largest raw value:\n%s", fence)
	}
	// Negative arm: no on-chain context-only valued rows → no aux row.
	_, clean := elimRenderOverview(t, elimBoardProjection(), true)
	if strings.Contains(clean, "未计价占用") {
		t.Fatalf("the aux row must be population-gated:\n%s", clean)
	}
	// EN face.
	_, en := elimRenderOverview(t, twodimProjectionWithUnpricedRunning(), false)
	if !strings.Contains(en, "unpriced occupancy") || !strings.Contains(en, "lever: own workload") {
		t.Fatalf("en face must carry the same account:\n%s", en)
	}
}

// G1b/G3③: both LLM word surfaces teach the two-dimension frame and the
// blocking_span pricing arm.
func TestTwoDimTeachingOnBothLLMFaces(t *testing.T) {
	tq := &TraceQuery{}
	for name, face := range map[string]string{
		"description": tq.Description(),
		"parameters":  string(tq.Parameters()),
	} {
		for _, want := range []string{
			"root causes have TWO dimensions",
			"raw time occupancy that guides NEW fix directions",
			"blocking_span rows price by their converged blocked wall clock",
			"未计价占用",
		} {
			if !strings.Contains(face, want) {
				t.Fatalf("%s must teach the two-dimension frame, missing %q", name, want)
			}
		}
	}
}

func twodimOccupancyMatrixProjection() types.TraceCausalProjection {
	projection := twodimProjectionWithUnpricedRunning()
	projection.OnChainCauses[len(projection.OnChainCauses)-1].EffectiveImpactMS = 0
	projection.OnChainCauses[len(projection.OnChainCauses)-1].EffectiveImpactPublished = true

	semantic := elimChainNode("E-semantic", "RenderThread-7788", "semantic_trace_span", "running", 0, 36, 510)
	semantic.Predicate = "semantic_trace_span"
	semantic.SpanName = "RenderTask"
	semantic.SemanticClass = "render_work"
	semantic.FamilyMemberCount = 3
	semantic.FamilyMemberMaxMS = 18
	semantic.EffectiveImpactMS = 0
	projection.SemanticSpans = []types.TraceCausalProjectionNode{semantic}

	projection.BusinessSpanMentions = []types.TraceCausalProjectionBusinessSpanMention{
		{
			Subject: "Worker-9001", Name: "one long span",
			Count: 1, TotalMS: 52, MaxMS: 52,
			StartLine: 600, EndLine: 610, Basis: "chain_member",
		},
		{
			Subject: "Worker-9002", Name: "many small spans",
			Count: 40, TotalMS: 80, MaxMS: 2,
			StartLine: 700, EndLine: 900, Basis: "self",
		},
	}
	projection.CPUOccupancyProcesses = []types.TraceCausalProjectionCPUOccupancyProcess{
		{
			Subject: "render_service-411", RunningCPUMS: 120, ThreadCount: 4,
			TopThread: "RSHardwareThre-1063", TopThreadMS: 70,
			CPUs: []int{2, 3}, CoreClasses: []string{"big"},
			WindowStart: projection.WindowStartTs, WindowEnd: projection.WindowEndTs,
			LineStart: 1000, LineEnd: 1200,
		},
	}
	projection.TargetStateAccount = &types.TraceCausalProjectionTargetStateAccount{
		Subject:   "target-42",
		RunningMS: 60, RunnableMS: 30, SleepMS: 80,
		SleepIOWaitMS: 8, DStateMS: 10, IOWaitMS: 20, TotalMS: 200,
		WindowStartTs: projection.WindowStartTs, WindowEndTs: projection.WindowEndTs,
	}
	return projection
}

// TWODIM-2 matrix: actual occupancy is a first-class decision surface while
// retaining separate wall-clock/cpu-time rulers and separate existing-rule
// pricing. The family names are arbitrary fixtures; no production branch
// reads them.
func TestTwoDimOccupancyDecisionSurfaceMatrix(t *testing.T) {
	projection := twodimOccupancyMatrixProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	block := runtimeTraceCausalProjectionOccupancyBlock(
		projection, model, true, runtimeTraceCausalProjectionBlockIDBase, "", nil, nil,
	)
	if block == nil {
		t.Fatal("typed occupancy matrix must publish the independent decision surface")
	}
	if block.Title != "主要时间占用 / 关键路径候选" {
		t.Fatalf("unexpected occupancy title: %q", block.Title)
	}
	joined := block.Text
	for _, item := range block.Items {
		joined += "\n" + strings.Join(item.Cells, " | ")
	}
	for _, want := range []string{
		"两轴独立，不能相加或互相替代",
		"本表自身不证明某个占用已经导致具体丢帧",
		"running 60.000ms、runnable 30.000ms、sleep 80.000ms",
		"非 IO D-state 10.000ms、io_wait 20.000ms",
		"不能直接当作可消除收益",
		"Worker-9001 / one long span",
		"52.000ms",
		"Worker-9002 / many small spans",
		"80.000ms",
		"2.000ms",
		"40",
		"RenderThread-7788 / RenderTask",
		"真实占时但现有规则未计价",
		"现规则可消 26.392ms 另见可消除榜",
		"render_service-411",
		"120.000cpu·ms",
		"多核并行可超过墙钟窗口",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("occupancy matrix missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "120.000ms") {
		t.Fatalf("cross-thread process CPU time must never be relabeled as wall clock:\n%s", joined)
	}

	// Producer order is not authority: the business family group must rank by
	// its own cumulative wall-clock ruler.
	many := strings.Index(joined, "many small spans")
	long := strings.Index(joined, "one long span")
	if many < 0 || long < 0 || many > long {
		t.Fatalf("business families must be ordered by cumulative wall clock:\n%s", joined)
	}

	// EN parity and the no-frame-proof boundary.
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	en := runtimeTraceCausalProjectionOccupancyBlock(
		projection, enModel, false, runtimeTraceCausalProjectionBlockIDBase, "", nil, nil,
	)
	if en == nil || !strings.Contains(en.Text, "The two axes are independent") ||
		!strings.Contains(en.Text, "does not prove that an occupancy caused a specific dropped frame") {
		t.Fatalf("english occupancy boundary missing: %+v", en)
	}
}

func TestTwoDimOccupancyDedupesPhysicalStateAcrossPublicationLanes(t *testing.T) {
	chain := types.TraceCausalProjectionNode{
		EvidenceID:               "chain-state-row",
		Subject:                  "app-100",
		Predicate:                "runnable",
		StateKind:                types.TraceStateKindRunnable,
		ImpactMS:                 0.8,
		EffectiveImpactMS:        0.8,
		EffectiveImpactPublished: true,
		StartTs:                  5.005,
		EndTs:                    5.0058,
		LineStart:                6,
		LineEnd:                  10,
	}
	self := chain
	self.EvidenceID = "ranked-target-self-row"
	self.Predicate = "runnable_wait"
	self.LineEnd = 9

	rows := runtimeTraceOccupancyPathCandidates(runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{{
			Node: chain, Kind: runtimeTraceProjTreeRowChain, HasData: true,
		}},
		SelfRows: []runtimeTraceProjTreeRow{{
			Node: self, Kind: runtimeTraceProjTreeRowSelf, HasData: true,
		}},
	}, true)
	if len(rows) != 1 {
		t.Fatalf("one physical runnable interval published through two lanes must render once, got %+v", rows)
	}
	if rows[0].subject != "app-100" || rows[0].totalMS != 0.8 || rows[0].location != "5.005000..5.005800；行 6–10" {
		t.Fatalf("deduped physical-state occupancy lost the first rich carrier: %+v", rows[0])
	}
}

func TestTwoDimOccupancyJoinsKeyedAndUnkeyedExactStatePublication(t *testing.T) {
	keyed := types.TraceCausalProjectionNode{
		EvidenceID: "wakeup-impact", Subject: "app-100", Predicate: "wakeup_causal_impact",
		StateKind: "s_sleep", StateAccountKey: "state_account:v2:exact-sleep",
		ImpactMS: 20, StartTs: 2, EndTs: 2.020, LineStart: 3, LineEnd: 15,
	}
	unkeyed := keyed
	unkeyed.EvidenceID = "state-drilldown"
	unkeyed.Predicate = "state_drilldown"
	unkeyed.StateAccountKey = ""
	unkeyed.LineEnd = 14

	rows := runtimeTraceOccupancyPathCandidates(runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{{Node: keyed, Kind: runtimeTraceProjTreeRowChain, HasData: true}},
		SelfRows: []runtimeTraceProjTreeRow{{Node: unkeyed, Kind: runtimeTraceProjTreeRowSelf, HasData: true}},
	}, true)
	if len(rows) != 1 || rows[0].totalMS != 20 || rows[0].location != "2.000000..2.020000；行 3–15" {
		t.Fatalf("one keyed/unkeyed publication of the exact sleep interval must render once: %+v", rows)
	}
}

func TestTwoDimOccupancyExactEnvelopeFailsOpenOnConflictingAccountsOrValue(t *testing.T) {
	base := types.TraceCausalProjectionNode{
		EvidenceID: "account-a", Subject: "app-100", Predicate: "wakeup_causal_impact",
		StateKind: "s_sleep", StateAccountKey: "state_account:v2:a",
		ImpactMS: 20, StartTs: 2, EndTs: 2.020,
	}
	conflict := base
	conflict.EvidenceID = "account-b"
	conflict.StateAccountKey = "state_account:v2:b"
	unkeyed := base
	unkeyed.EvidenceID = "state-drilldown"
	unkeyed.StateAccountKey = ""

	rows := runtimeTraceOccupancyPathCandidates(runtimeTraceProjTreeModel{TreeRows: []runtimeTraceProjTreeRow{
		{Node: base, Kind: runtimeTraceProjTreeRowChain, HasData: true},
		{Node: conflict, Kind: runtimeTraceProjTreeRowChain, HasData: true},
		{Node: unkeyed, Kind: runtimeTraceProjTreeRowChain, HasData: true},
	}}, true)
	if len(rows) != 3 {
		t.Fatalf("two conflicting producer accounts make the shared envelope ambiguous and must fail open: %+v", rows)
	}

	valueMismatch := unkeyed
	valueMismatch.EvidenceID = "different-value"
	valueMismatch.ImpactMS = 19
	rows = runtimeTraceOccupancyPathCandidates(runtimeTraceProjTreeModel{TreeRows: []runtimeTraceProjTreeRow{
		{Node: unkeyed, Kind: runtimeTraceProjTreeRowChain, HasData: true},
		{Node: valueMismatch, Kind: runtimeTraceProjTreeRowChain, HasData: true},
	}}, true)
	if len(rows) != 2 {
		t.Fatalf("same hull with a different physical value is not one exact interval: %+v", rows)
	}
}

func TestTwoDimOccupancyUsesExactStateAccountAcrossDifferentViewEnvelopes(t *testing.T) {
	const accountKey = "state_account:v2:exact-io-segments"
	rank := types.TraceCausalProjectionNode{
		EvidenceID: "ranked-io-row", Subject: "threadpool-400",
		Predicate: "root_cause_io_wait", StateKind: types.TraceStateKindIOWait,
		StateAccountKey: accountKey, ImpactMS: 11, StartTs: 2.003, EndTs: 2.014,
		LineStart: 6, LineEnd: 8,
	}
	impact := rank
	impact.EvidenceID = "wakeup-io-row"
	impact.Predicate = "wakeup_causal_impact"
	impact.StartTs = 2.002
	impact.EndTs = 2.016
	impact.LineEnd = 9

	rows := runtimeTraceOccupancyPathCandidates(runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{
			{Node: rank, Kind: runtimeTraceProjTreeRowCause, HasData: true},
			{Node: impact, Kind: runtimeTraceProjTreeRowChain, HasData: true},
		},
	}, true)
	if len(rows) != 1 || rows[0].totalMS != 11 {
		t.Fatalf("one exact IO account published through different view envelopes must render once: %+v", rows)
	}

	impact.StateAccountKey = "state_account:v2:different-io-segments"
	rows = runtimeTraceOccupancyPathCandidates(runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{
			{Node: rank, Kind: runtimeTraceProjTreeRowCause, HasData: true},
			{Node: impact, Kind: runtimeTraceProjTreeRowChain, HasData: true},
		},
	}, true)
	if len(rows) != 2 {
		t.Fatalf("different exact IO accounts must fail open even when scalars match: %+v", rows)
	}
}

func TestTwoDimOccupancyExcludesNonWallClockCaliberRows(t *testing.T) {
	pageCache := types.TraceCausalProjectionNode{
		EvidenceID:        "count-equivalent-page-cache",
		Subject:           "app-100",
		Predicate:         "root_cause_caliber_side",
		Object:            "page_cache_churn",
		TypeToken:         "page_cache_churn",
		Tier:              types.TraceCausalTierCaliberSide,
		ChainRelevance:    "self_caliber_side",
		ImpactMS:          81.616,
		MergedCount:       2,
		MergedMaxMS:       84.300,
		FamilyFoldCaliber: tracequery.RootCauseMemberFoldCaliberCountSum,
	}
	composite := types.TraceCausalProjectionNode{
		EvidenceID: "composite-io-score",
		Subject:    "app-100",
		Object:     "block_io_by_inode",
		TypeToken:  "block_io_by_inode",
		Unit:       types.TraceObservationUnitCompositeScore,
		ImpactMS:   2.694,
	}
	running := types.TraceCausalProjectionNode{
		EvidenceID: "real-running-wall-clock",
		Subject:    "app-100",
		StateKind:  types.TraceStateKindRunning,
		ImpactMS:   12.5,
		StartTs:    5,
		EndTs:      5.0125,
	}
	rows := runtimeTraceOccupancyPathCandidates(runtimeTraceProjTreeModel{
		SelfRows: []runtimeTraceProjTreeRow{
			{Node: pageCache, Kind: runtimeTraceProjTreeRowSelf, HasData: true},
			{Node: composite, Kind: runtimeTraceProjTreeRowSelf, HasData: true},
			{Node: running, Kind: runtimeTraceProjTreeRowSelf, HasData: true},
		},
	}, true)
	if len(rows) != 1 || rows[0].subject != "app-100" || rows[0].totalMS != 12.5 || rows[0].unit != "ms" {
		t.Fatalf("only genuine wall-clock state may enter occupancy rows: %+v", rows)
	}
}

// The occupancy block leads the existing causal/eliminable projection inside
// the cluster; it does not replace or mutate the priced board.
func TestTwoDimOccupancyLeadsUnchangedEliminableBoard(t *testing.T) {
	projection := twodimOccupancyMatrixProjection()
	cluster := runtimeTraceCausalProjectionClusterFor(
		projection, "zh-CN", runtimeTraceProjUserFocus{}, runtimeTraceCausalProjectionBlockIDBase, "",
	)
	if len(cluster) < 2 ||
		cluster[0].ID != runtimeTraceCausalProjectionBlockIDBase+runtimeTraceCausalProjectionOccupancySuffix ||
		cluster[1].ID != runtimeTraceCausalProjectionBlockIDBase {
		t.Fatalf("occupancy must lead the existing projection cluster: %+v", cluster)
	}
	if !strings.Contains(cluster[1].Text, "窗内可消除量") ||
		!strings.Contains(cluster[1].Text, "26.392ms") {
		t.Fatalf("existing eliminable board must remain present and numerically unchanged:\n%s", cluster[1].Text)
	}
	markRuntimeTraceSystemBlocks(cluster)
	model := types.AnswerBlock{ID: "model_answer", Kind: types.BlockSection, Title: "结论"}
	doc := &types.AnswerDocumentV2{Blocks: append([]types.AnswerBlock{model}, cluster...)}
	normalizeRuntimeTraceReportHierarchy(doc)
	if doc.Blocks[0].ID != "model_answer" ||
		doc.Blocks[1].ID != runtimeTraceCausalProjectionBlockIDBase+runtimeTraceCausalProjectionOccupancySuffix ||
		doc.Blocks[2].ID != runtimeTraceCausalProjectionBlockIDBase {
		t.Fatalf("model narrative → occupancy → priced projection order must hold: %+v", doc.Blocks[:3])
	}
}

// No span evidence must not mint a fake span family. Real on-chain state
// occupancy may still keep the independent surface alive.
func TestTwoDimOccupancyNoSpanNegative(t *testing.T) {
	projection := twodimProjectionWithUnpricedRunning()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	block := runtimeTraceCausalProjectionOccupancyBlock(
		projection, model, true,
		runtimeTraceCausalProjectionBlockIDBase, "", nil, nil,
	)
	if block == nil {
		t.Fatal("real on-chain occupancy should remain visible without span evidence")
	}
	for _, item := range block.Items {
		if strings.Contains(item.Cells[0], "span") {
			t.Fatalf("absence of span evidence must not fabricate a span row: %+v", item)
		}
	}
}

// Wire pin: the existing WindowStats CPU census publishes one strict cpu·ms
// side-channel record and the projection compiler carries it without adding a
// causal node, rank ordinal, or wall-clock conversion.
func TestTwoDimCPUOccupancyTypedSideChannel(t *testing.T) {
	stats := tracequery.WindowStats{
		Window: tracequery.TimeWindow{StartTs: 10, EndTs: 10.2, StartSet: true},
		CPUOccupancy: &tracequery.CPUOccupancyStats{
			TopProcesses: []tracequery.ProcessCPULoadSummary{
				{
					Process:     tracequery.ThreadRef{Comm: "render_service", PID: 411},
					ThreadCount: 4, RunningMs: 120,
					TopThread:   tracequery.ThreadRef{Comm: "RSHardwareThre", PID: 1063},
					TopThreadMs: 70, CPUs: []int{2, 3}, CoreClasses: []string{"big"},
					LineStart: 100, LineEnd: 200,
				},
			},
		},
	}
	records := traceQueryTypedWindowStatsObservations(
		stats, types.ObservationSourceRef{ArtifactID: "trace.systrace"}, "w1", "now",
	)
	var occupancy []types.ObservationRecord
	for _, record := range records {
		if record.Predicate == "cpu_occupancy_process" {
			occupancy = append(occupancy, record)
		}
	}
	if len(occupancy) != 1 || occupancy[0].Unit != "cpu·ms" || occupancy[0].Value != "120.000" {
		t.Fatalf("expected one strict cpu·ms side-channel record: %+v", occupancy)
	}
	root := types.ObservationRecord{
		ID:              "trace_query:w1#root_cause_primary:1",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		ClaimKey:        "root_cause_primary:target-1",
		Subject:         "target-1",
		Predicate:       "root_cause_primary",
		Object:          "runnable_wait",
		Value:           "1.000",
		Unit:            "ms",
		RichNotes: []string{
			"rank=1", "tier=primary", "chain_relevance=on_chain",
			"selected_window=10.000000..10.200000",
		},
	}
	projection := types.TraceCausalProjectionFromObservationRecords(append(occupancy, root))
	if len(projection.CPUOccupancyProcesses) != 1 {
		t.Fatalf("projection must carry the cpu occupancy side channel: record=%+v projection=%+v", occupancy[0], projection)
	}
	got := projection.CPUOccupancyProcesses[0]
	if got.RunningCPUMS != 120 || got.ThreadCount != 4 ||
		got.TopThread != "RSHardwareThre-1063" || got.TopThreadMS != 70 ||
		got.WindowStart != 10 || got.WindowEnd != 10.2 {
		t.Fatalf("typed cpu occupancy payload drifted: %+v", got)
	}
	if len(projection.PrimaryRootCauses) != 1 || projection.PrimaryRootCauses[0].EvidenceID != root.ID ||
		len(projection.OnChainCauses) != 1 || projection.OnChainCauses[0].EvidenceID != root.ID ||
		len(projection.AdjacentCauses) != 0 || len(projection.BackgroundCauses) != 0 {
		t.Fatalf("cpu resource context must never add a causal/rank seat beside the seed root: %+v", projection)
	}
}

package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// trace_board_summary_test.go — CR-1 件③ pins (§29.42.2① 单源摘要喂入,
// 2026-07-12): the typed board summary carries the seated rows in authority
// order with value + caliber + channel + confidence, excludes seatless rows,
// and stays silent on non-trace runs.

func traceBoardTestLedger() types.ObservationLedger {
	rankRecord := func(id, subject, typ string, notes []string, conf float64) types.ObservationRecord {
		return types.ObservationRecord{
			ID:         id,
			Origin:     types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:   "trace_query",
			Subject:    subject,
			Predicate:  "root_cause_primary",
			Object:     typ,
			RichNotes:  notes,
			Confidence: conf,
		}
	}
	return types.ObservationLedger{Records: []types.ObservationRecord{
		rankRecord("trace_query:t#root_cause_rank:1", "CompThread_0-2955", "d_state_or_io_wait",
			// CR-3 件③ P11: the typed process attribution rides the seat.
			[]string{"rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=36.757", "member_fold_caliber=sum_disjoint", "tgid=2916"}, 0.80),
		rankRecord("trace_query:t#root_cause_rank:2", "keva-1-17437", "sleep_wait",
			[]string{"rank=2", "tier=secondary", "chain_relevance=on_chain", "effective_impact_ms=3.399"}, 0.74),
		// Seatless rows never enter the board summary: the target's own
		// symptom row (rank=0) and a context-only pacing row.
		rankRecord("trace_query:t#root_cause_rank:3", ".ugc.aweme.lite-17267", "binder_wait",
			[]string{"tier=target_self_state", "effective_impact_ms=1.409"}, 0.92),
		rankRecord("trace_query:t#root_cause_rank:4", ".ugc.aweme.lite-17267", "pacing_idle",
			[]string{"tier=context_only", "effective_impact_ms=0.000"}, 0.85),
		// An adjacent-channel seat rides its own ordinal space.
		rankRecord("trace_query:t#root_cause_rank:5", "adj-5", "running",
			[]string{"rank=1", "tier=secondary", "chain_relevance=adjacent", "effective_impact_ms=4.596"}, 0.70),
	}}
}

func TestTraceRootCauseBoardSummaryAuthoritativeOrder(t *testing.T) {
	summary := formatTraceRootCauseBoardFromLedger(traceBoardTestLedger())
	if summary == "" {
		t.Fatalf("seated rank rows must render a board summary")
	}
	first := strings.Index(summary, "CompThread_0-2955")
	second := strings.Index(summary, "keva-1-17437")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("board summary must keep the engine seat order:\n%s", summary)
	}
	for _, want := range []string{
		"#1 root-cause seat — CompThread_0-2955 · d_state_or_io_wait",
		"36.757ms (effective attribution)",
		"fold=sum_disjoint",
		"channel=chain",
		"confidence=0.80",
		"cross_seat_aggregation_authority=forbidden",
		// CR-3 件③ P11 (冷读案8): the seat's process attribution (tgid) on
		// the LLM face — bare thread names stay traceable to their process.
		"tgid=2916",
		"Adjacent-impact seats",
		"#1 adjacent seat — adj-5 · running",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("board summary missing %q:\n%s", want, summary)
		}
	}
	// Seatless rows (symptom / context lanes) never take board lines.
	if strings.Contains(summary, "binder_wait") || strings.Contains(summary, "pacing_idle") {
		t.Fatalf("seatless rows must stay off the board summary:\n%s", summary)
	}
	// The teaching preamble states the disclosure duty, never a gate.
	if !strings.Contains(summary, "never reorder silently") || !strings.Contains(summary, "never sum rows together") {
		t.Fatalf("board summary preamble must carry the ordering/no-sum teaching:\n%s", summary)
	}
	if !strings.Contains(summary, "a distinct ordinal space") || strings.Contains(summary, "an independent ordinal space") {
		t.Fatalf("adjacent ordering must use domain vocabulary, not physical-independence wording:\n%s", summary)
	}
}

func TestTraceRootCauseBoardSummaryCollapsesExactCrossQuerySeat(t *testing.T) {
	ledger := traceBoardTestLedger()
	duplicate := ledger.Records[0]
	duplicate.ID = "trace_query:supplement#root_cause_rank:1"
	ledger.Records = append(ledger.Records, duplicate)
	summary := formatTraceRootCauseBoardFromLedger(ledger)
	if got := strings.Count(summary, "CompThread_0-2955 · d_state_or_io_wait"); got != 1 {
		t.Fatalf("one exact seat published by two query results must render once, got %d:\n%s", got, summary)
	}

	conflict := duplicate
	conflict.ID = "trace_query:conflict#root_cause_rank:1"
	conflict.RichNotes = append([]string(nil), conflict.RichNotes...)
	conflict.RichNotes[3] = "effective_impact_ms=36.758"
	ledger.Records = append(ledger.Records, conflict)
	summary = formatTraceRootCauseBoardFromLedger(ledger)
	if got := strings.Count(summary, "CompThread_0-2955 · d_state_or_io_wait"); got != 2 {
		t.Fatalf("typed value disagreement must remain visible, got %d:\n%s", got, summary)
	}
}

// TestTraceRootCauseBoardSummaryPreambleCaliberAware — DISPHYG-3 件4
// (FREQDIR-1 冷读 P3-1, 2026-07-20): the no-sum parenthetical defers to each
// row's own published caliber word instead of blanket-claiming every row a
// wall-clock measurement (a supply-fold CONVERTED seat — the 95946 #1 shape —
// is not one; the blanket claim mis-taught the model).
func TestTraceRootCauseBoardSummaryPreambleCaliberAware(t *testing.T) {
	summary := formatTraceRootCauseBoardFromLedger(traceBoardTestLedger())
	if !strings.Contains(summary, "per-thread measurements — wall-clock or converted, per each row's own published caliber word") {
		t.Fatalf("the preamble must speak the caliber-aware measurement wording:\n%s", summary)
	}
	if strings.Contains(summary, "they are per-thread wall-clock measurements") {
		t.Fatalf("the blanket wall-clock claim must be gone:\n%s", summary)
	}
}

// CR-2 组③ P7 / F-4 (冷读 F-4, 2026-07-12; witness tieba 20260712-135155
// prose: 「runnable_wait 窗口 — 34579.568118s–34579.572194s(25.847ms)」 — the
// whole-window total paired with ONE occurrence window). The board summary
// labels a row's representative window as one occurrence and teaches the
// value-window pairing rule (soft data-feeding lane, never a gate).
func TestTraceRootCauseBoardSummaryLabelsRepresentativeWindow(t *testing.T) {
	ledger := traceBoardTestLedger()
	ledger.Records[0].RichNotes = append(ledger.Records[0].RichNotes,
		"occurrence_windows=34579.568118..34579.572194;34579.579001..34579.581200")
	summary := formatTraceRootCauseBoardFromLedger(ledger)
	if !strings.Contains(summary, "representative_window=34579.568118..34579.572194") {
		t.Fatalf("a row with occurrence windows must label its representative window:\n%s", summary)
	}
	if !strings.Contains(summary, "ONE occurrence among several") {
		t.Fatalf("the preamble must teach the value-window pairing rule:\n%s", summary)
	}
	// Rows without occurrence windows stay byte-identical (no fabricated window).
	if strings.Contains(summary, "keva-1-17437 · sleep_wait · channel=chain · confidence=0.74 · representative_window") {
		t.Fatalf("windowless rows must not gain a representative window:\n%s", summary)
	}
}

// TestTraceRootCauseBoardSummary_FixDirectionWord — FREQDIR-1 件1 (§29.149,
// 2026-07-19; witness run 95946: the finalizer-visible #1 row spoke only the
// bare state word `running` and the answer's 修复方向 list dropped the #1
// direction 频率与热治理 58.320ms). The 95946 #1 seat shape now wears the
// typed direction word verbatim (single word-face source), a row without the
// note wears NO direction token (absence stays absent), an unregistered token
// synthesizes nothing, and the preamble teaches the repair-lane rule.
func TestTraceRootCauseBoardSummary_FixDirectionWord(t *testing.T) {
	ledger := traceBoardTestLedger()
	// The 95946 witness #1 row shape (log 20260719-123952-000-95946:1627).
	witness := types.ObservationRecord{
		ID:         "trace_query:w#root_cause_rank:1",
		Origin:     types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:   "trace_query",
		Subject:    ".ugc.aweme.lite-17267",
		Predicate:  "root_cause_primary",
		Object:     "running",
		Confidence: 0.86,
		RichNotes: []string{
			"rank=1", "tier=primary", "chain_relevance=on_chain",
			"effective_impact_ms=58.320", "tgid=17267",
			types.TraceNoteKeyFixDirection + "=frequency_thermal",
		},
	}
	ledger.Records = append([]types.ObservationRecord{witness}, ledger.Records...)
	summary := formatTraceRootCauseBoardFromLedger(ledger)
	t.Logf("FREQDIR-1 件1 witness board render:\n%s", summary)
	// The witness row carries the direction word verbatim, in-row. POOL2-1
	// 件⑤ EVOLUTION (§29.160⑤ user ruling 2026-07-20: EN 双面并列): the row
	// wears the zh face WITH its Table ⑦ EN face in parentheses — one pair,
	// both halves from tracefence.FixDirectionWord (零二表).
	want := "- #1 root-cause seat — .ugc.aweme.lite-17267 · running · channel=chain · confidence=0.86 · cross_seat_aggregation_authority=forbidden · tgid=17267 · 58.320ms (effective attribution) · tier=primary · 修向=频率与热治理 (frequency & thermal) · repair_lane_fold=max_on_chain_seat_not_sum"
	if !strings.Contains(summary, want) {
		t.Fatalf("the #1 seat must wear its typed direction word pair verbatim (件⑤ EN 双面), want\n%q\nin:\n%s", want, summary)
	}
	// Rows without a fix_direction note wear NO direction token: exactly one
	// in-row 修向 token on this board (the witness row's; the preamble's
	// teaching mention of 修向=X is not a row token).
	if got := strings.Count(summary, "· 修向="); got != 1 {
		t.Fatalf("only note-carrying rows may wear 修向= (want 1 row token, got %d):\n%s", got, summary)
	}
	// The preamble teaches the repair-lane rule alongside the token. 返工
	// P2-1 (双复核): the lane maximum is CHANNEL-QUALIFIED — adjacent rows
	// also wear 修向= and can exceed the chain maximum, but the ◎ section
	// heads fold on-chain members only, so an unqualified sentence would let
	// the model derive a lane maximum larger than the rendered board's.
	for _, teach := range []string{
		"seats sharing one 修向 form ONE repair lane",
		"LARGEST on-chain seat value — never the seats' sum",
		"adjacent rows are conditional upper bounds and never join the lane maximum",
		"A row without 修向 published no direction; never infer one",
	} {
		if !strings.Contains(summary, teach) {
			t.Fatalf("the preamble must carry the repair-lane rule (missing %q):\n%s", teach, summary)
		}
	}
	// An unregistered token synthesizes nothing (closed registry verbatim,
	// never a guessed word and never the raw token leaked).
	ledger.Records[0].RichNotes[5] = types.TraceNoteKeyFixDirection + "=bogus_direction"
	mutated := formatTraceRootCauseBoardFromLedger(ledger)
	if strings.Contains(mutated, "· 修向=") || strings.Contains(mutated, "bogus_direction") {
		t.Fatalf("an unregistered direction token must render nothing:\n%s", mutated)
	}
}

func TestTraceRootCauseBoardSummarySilentWithoutTraceObservations(t *testing.T) {
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{{
		ID:       "current",
		Origin:   types.AnswerEvidenceOriginCurrentSource,
		Producer: "read_file",
		Subject:  "someSymbol",
	}}}
	if got := formatTraceRootCauseBoardFromLedger(ledger); got != "" {
		t.Fatalf("non-trace runs must not render a board summary: %q", got)
	}
}

// B716: a user-requested window is the principal answer universe. Explorer
// drill-down queries remain useful evidence, but their independent #1/#2
// boards must not be co-presented as one authoritative ordering. The filter
// reads only the validated scope profile and typed selected_window notes.
func TestTraceRootCauseBoardSummaryPrefersExplicitRequestedWindow(t *testing.T) {
	start, end := 13762.791708, 13763.024898
	ledger := traceBoardTestLedger()
	ledger.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "13762.791708s 到 13763.024898s",
	}
	ledger.Records[0].RichNotes = append(ledger.Records[0].RichNotes,
		"selected_window=13762.791708..13763.024898")
	ledger.Records[1].RichNotes = append(ledger.Records[1].RichNotes,
		"selected_window=13762.930000..13763.024898")
	ledger.Records[4].RichNotes = append(ledger.Records[4].RichNotes,
		"selected_window=13762.791708..13763.024898")

	summary := formatTraceRootCauseBoardFromLedger(ledger)
	for _, want := range []string{
		"single authoritative ordering for the explicitly requested window 13762.791708..13763.024898",
		"CompThread_0-2955",
		"adj-5",
		"exploratory or narrower query windows remain available in the evidence ledger",
		"never add or compare their raw durations across windows",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("requested-window board missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "keva-1-17437") {
		t.Fatalf("narrower-window seat must stay out of the principal requested-window board:\n%s", summary)
	}
}

func TestTraceRootCauseBoardSummaryPreservesEvidenceWhenRequestedWindowWasNotMeasured(t *testing.T) {
	start, end := 10.0, 11.0
	ledger := traceBoardTestLedger()
	ledger.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "10.0s 到 11.0s",
	}
	for i := range ledger.Records {
		ledger.Records[i].RichNotes = append(ledger.Records[i].RichNotes,
			"selected_window=10.200000..10.800000")
	}

	summary := formatTraceRootCauseBoardFromLedger(ledger)
	if !strings.Contains(summary, "CompThread_0-2955") || !strings.Contains(summary, "keva-1-17437") {
		t.Fatalf("absence of an exact requested-window board must preserve bounded runtime evidence:\n%s", summary)
	}
	if strings.Contains(summary, "single authoritative ordering for the explicitly requested window") {
		t.Fatalf("an unmeasured requested window must not be claimed as measured authority:\n%s", summary)
	}
}

// This production-shaped pin holds the complete builder wiring: accepted
// AnalysisIR scope -> ObservationLedger -> AgentContext root-cause board.
// Deleting the scope carrier from either ledger adapter or board formatter
// must fail here even if the formatter-only tests still compile.
func TestBuildAgentContextTraceBoardUsesRequestedWindowAuthority(t *testing.T) {
	start, end := 20.0, 21.0
	record := func(id, subject, value, window string) types.ObservationRecord {
		return types.ObservationRecord{
			ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", Subject: subject,
			Predicate: "root_cause_primary", Object: "running", Confidence: 0.8,
			RichNotes: []string{
				"rank=1", "tier=primary", "chain_relevance=on_chain",
				"effective_impact_ms=" + value, "selected_window=" + window,
			},
		}
	}
	bus := &types.BusContext{
		RepoRoot: "/tmp/repo",
		Mutable:  types.NewMutableState("explicit-window trace question"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeStart:      &start,
				TimeEnd:        &end,
				SourceQuote:    "20.0s 到 21.0s",
			},
		}},
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query", Success: true,
			Observations: []types.ObservationRecord{
				record("trace_query:full#root_cause_rank:1", "requested-window-seat", "8.000", "20.000000..21.000000"),
				record("trace_query:narrow#root_cause_rank:1", "drilldown-window-seat", "5.000", "20.500000..21.000000"),
			},
		}},
	}

	ac := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	if !strings.Contains(ac.TraceRootCauseBoard, "requested-window-seat") ||
		strings.Contains(ac.TraceRootCauseBoard, "drilldown-window-seat") {
		t.Fatalf("builder must publish only the measured requested-window board:\n%s", ac.TraceRootCauseBoard)
	}
}

func TestBuildAgentContextFiniteRuntimeScopeDoesNotInjectRootCauseBoard(t *testing.T) {
	bus := &types.BusContext{
		RepoRoot: "/tmp/repo",
		Mutable:  types.NewMutableState("finite runtime effect question"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
				Scope: types.RuntimeQuestionScopeBoundedEffectVerdict,
				FactFamilies: []types.RuntimeQuestionFactFamily{
					types.RuntimeQuestionFactTargetSchedulerState,
					types.RuntimeQuestionFactCountOrDuration,
					types.RuntimeQuestionFactFrequencyResidency,
				},
			},
		}},
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query", Success: true,
			Observations: traceBoardTestLedger().Records,
		}},
	}

	ac := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	if ac.TraceRootCauseBoard != "" {
		t.Fatalf("finite typed scope inherited an exploration-time root-cause board:\n%s", ac.TraceRootCauseBoard)
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "finalize-answer"})
	if section := findSectionTitle(pc, SectionTraceRootCauseBoard); section != nil {
		t.Fatalf("finite typed scope emitted root-cause section: %+v", section)
	}
}

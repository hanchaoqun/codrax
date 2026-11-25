package agent

// HMC377 round 16 (batch-owner rulings R-377-16 ④, R-377-17 ③④, 2026-11-24):
// the finalizer first request states the restricted-ready relation the engine
// types between a CPU-restriction seat and the rows that hold its restricted
// ready instants, through the one decision (tool.TraceRestrictedReadyHolds),
// in the reader's words: the holder's line says how much of its time is
// restricted and under which constraint; the restriction seat's line (split
// arm) gives its account and that the part lies inside the holder's ordinal;
// an absorbed member (whole arm) has no line, so its holder's line carries it.
//
// The fixtures are the records the engine mints, read back through the
// finalizer's own entry (BuildInitialInstruction): Douyin reboot (dio w45,
// TID 76 restricted to CPU 7: chain seat #8 holds 3.913 ms of the seat's
// 6.606 ms restricted ready account, the seat keeps 2.693 ms) and the
// absorbed member of n03m (every restricted instant inside chain seat #11).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const hmc377R16FScope = "trace_query:publication-r16f"

// hmc377R16FRecord is one rank record of the board in the engine's shape:
// record id by board position, the row's typed notes, the member thread.
func hmc377R16FRecord(pos int, predicate, subject string, tid int, object, value string, notes ...string) types.ObservationRecord {
	base := []string{"type=" + object, "impact_ms=" + value, "cumulative_impact_ms=" + value, "effective_impact_ms=" + value,
		"dominant_state=runnable", "runnable=" + value, "selected_window=140.911304..141.835458",
		"rank_board_target=.ugc.aweme.lite-11105", "rank_board_params_fingerprint=dd60cf59", "fix_direction=scheduling_supply"}
	return types.ObservationRecord{
		ID:     fmt.Sprintf("%s#root_cause_rank:%d", hmc377R16FScope, pos),
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/captures/douyin-reboot.ftrace",
			ArtifactID: "trace_query", ArtifactKind: "trace", TimeDomain: "trace_seconds", CanonicalTimeDomain: "trace_seconds",
			QueryWindowKnown: true, QueryWindowStartTs: 140.911304, QueryWindowEndTs: 141.835458, QueryTargetPID: 11105, QueryTargetScope: "thread"},
		Span:     types.ObservationSpan{StartTs: 140.918435, EndTs: 141.833178, LineStart: 104275 + pos, LineEnd: 208169},
		ClaimKey: predicate, Predicate: predicate, Subject: subject, Object: object, Value: value, Unit: "ms",
		RichNotes:         append(notes, base...),
		TraceRankSubjects: &types.TraceRankSubjects{Target: types.TraceThreadCoordinate{TID: 11105, TGID: 11105}, Member: types.TraceThreadCoordinate{TID: tid, TGID: 2}},
	}
}

// hmc377R16FChain is an on-chain runnable seat of the board at position rank.
func hmc377R16FChain(rank int, subject string, tid int, value string, notes ...string) types.ObservationRecord {
	return hmc377R16FChainOf(rank, "runnable_wait", subject, tid, value, notes...)
}

// hmc377R16FChainOf is an on-chain seat of the given type. A thread's second
// holding row is a priority-inversion row (n17m TID 7155: one claim names the
// thread's two inversion rows #4 and #9): two same-type rows of one thread
// would be one row of the projection.
func hmc377R16FChainOf(rank int, object, subject string, tid int, value string, notes ...string) types.ObservationRecord {
	return hmc377R16FRecord(rank, "root_cause_tertiary", subject, tid, object, value,
		append([]string{fmt.Sprintf("rank=%d", rank), "tier=tertiary", "source=window_stats", "causality=on_wakeup_chain", "chain_relevance=on_chain"}, notes...)...)
}

// hmc377R16FSeat is a CPU-restriction seat at board position pos on the
// adjacent board (rank > 0), or, with rank 0, an engine-absorbed member.
func hmc377R16FSeat(pos, rank int, subject string, tid int, value string, notes ...string) types.ObservationRecord {
	predicate, lead := "root_cause_adjacent", []string{fmt.Sprintf("rank=%d", rank), "tier=tertiary"}
	if rank == 0 {
		predicate, lead = "root_cause_absorbed", []string{"tier=absorbed"}
	}
	return hmc377R16FRecord(pos, predicate, subject, tid, "cpu_affinity_or_cpuset", value,
		append(append(lead, "source=window_stats.cpu_constraints", "causality=adjacent_to_wakeup_chain", "chain_relevance=adjacent"), notes...)...)
}

// hmc377R16FConstraint is the engine's typed CPU-constraint judgment.
func hmc377R16FConstraint(allowed, excluded string, tier ...string) []string {
	return append([]string{"cpu_constraint_kind=sched_switch_next_info",
		"cpu_constraint_policy=next_info affinity=80 load=18 group=0 ices_boost=false sched_group=no_group smt_expel=expellee cgroup_name=SP_DEFAULT",
		"cpu_constraint_allowed_cpus=" + allowed, "cpu_constraint_excluded_cpus=" + excluded}, tier...)
}

// hmc377R16FSplit is a seat's split account and its claims.
func hmc377R16FSplit(account, elsewhere, here, claims string) []string {
	return []string{"restricted_ready_account_ms=" + account, "share_counted_elsewhere_ms=" + elsewhere, "share_counted_here_ms=" + here, "share_claims=" + claims}
}

func hmc377R16FRequest(t *testing.T, lang string, records ...types.ObservationRecord) string {
	t.Helper()
	ctx := answerDocCausalCeilingTestContext(false)
	ctx.Language = lang
	ctx.AnalysisIR.RequestModel.Language = lang
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 11105, Source: "user_explicit"}}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "root_cause_rank"}, Observations: records,
	}}})
	return (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
}

// hmc377R16FSubLines lists every sub-line of the request's rank section as
// "<its seat line> => <sub-line>", in order.
func hmc377R16FSubLines(t *testing.T, prompt string) []string {
	t.Helper()
	section := ""
	for _, head := range []string{"### 面向答案的 Trace 排名与算力供给口径\n", "### Reader-ready Trace ranking and compute-supply basis\n"} {
		if at := strings.Index(prompt, head); at >= 0 {
			section = prompt[at+len(head):]
			if end := strings.Index(section, "\n### "); end >= 0 {
				section = section[:end]
			}
		}
	}
	if section == "" {
		t.Fatal("the request has no rank section")
	}
	var out []string
	parent := ""
	for _, line := range strings.Split(section, "\n") {
		switch {
		case strings.HasPrefix(line, "      - "):
			out = append(out, parent+" => "+strings.TrimPrefix(line, "      - "))
		case strings.HasPrefix(line, "    - "):
			parent = strings.TrimPrefix(line, "    - ")
		}
	}
	return out
}

func hmc377R16FWantLines(t *testing.T, name string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("[%s] the rank section's sub-lines:\n got:\n  %s\nwant:\n  %s", name, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

const (
	hmc377R16FSeatZH  = "原始排序 #1 udk-irq-7-76：CPU亲和/cpuset限制；原始排序计量 2.693ms（仅支撑，不授予根因或优化收益）"
	hmc377R16FSeatEN  = "Published order #1 udk-irq-7-76: CPU affinity/cpuset restriction; published ranking measure 2.693ms (support only, not cause or optimization-benefit authority)"
	hmc377R16FDesc7ZH = "CPU约束描述:允许核 7 · 排除全域观测核 0-6 · 判定依据 sched_switch_next_info"
	hmc377R16FDesc7EN = "CPU-constraint description: allowed CPUs 7 · excludes observed CPUs 0-6 · basis sched_switch_next_info"
)

// The Douyin reboot board (dio w45). The holder's own runnable account here
// is 4.250 ms, so its value is never the restricted amount.
func hmc377R16FSplitBoard() []types.ObservationRecord {
	return []types.ObservationRecord{
		hmc377R16FChain(1, ".ugc.aweme.lite-11105", 11105, "92.026"),
		hmc377R16FChain(2, "udk-irq-7-76", 76, "4.250"),
		hmc377R16FSeat(3, 1, "udk-irq-7-76", 76, "2.693", append(hmc377R16FSplit("6.606", "3.913", "2.693", "anchored/clipped_seat/1/1/0/3.913/2"),
			hmc377R16FConstraint("7", "0,1,2,3,4,5,6")...)...),
	}
}

// R-377-17 ④, split arm: the holder's line states the seat's account and the
// part inside it, and the seat's constraint; the seat's line states its
// account and that the part lies inside the holder's ordinal (on the other
// list, named). The amount and the account are the one decision's.
func TestHMC377R16FinalizerSplitArmStatesTheAccountOnBothLines(t *testing.T) {
	records := hmc377R16FSplitBoard()
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("one projection expected, got %d", len(set.Projections))
	}
	holds := tool.TraceRestrictedReadyHolds(set.Projections[0])
	if len(holds) != 1 || fmt.Sprintf("%.3f/%.3f", holds[0].MS, holds[0].AccountMS) != "3.913/6.606" {
		t.Fatalf("fixture precondition: the decision holds 3.913 of 6.606 ms: %+v", holds)
	}
	zh := hmc377R16FRequest(t, "zh", records...)
	hmc377R16FWantLines(t, "zh", hmc377R16FSubLines(t, zh), []string{
		hmc377R16FSeatZH + " => CPU亲和/cpuset限制下合计6.606ms:其中3.913ms已含在排名清单 2 #2账内,不另计",
		"#2 udk-irq-7-76：runnable；估算优化潜力影响 4.250ms => CPU亲和/cpuset限制下合计6.606ms:其中3.913ms已含在本行账内,不另计",
		"#2 udk-irq-7-76：runnable；估算优化潜力影响 4.250ms => " + hmc377R16FDesc7ZH,
	})
	// The seat's own line already permits its label: once.
	if got := strings.Count(zh, `permitted_reader_cause_label="CPU亲和/cpuset限制"`); got != 1 {
		t.Errorf("the label section permits the restriction label %d times, want 1", got)
	}
	hmc377R16FWantLines(t, "en", hmc377R16FSubLines(t, hmc377R16FRequest(t, "en", records...)), []string{
		hmc377R16FSeatEN + " => 6.606ms under the CPU affinity/cpuset restriction in all: of which 3.913ms is already inside the account of ranking list 2 #2, never added again",
		"#2 udk-irq-7-76: runnable; 4.250ms modeled potential under existing rules => 6.606ms under the CPU affinity/cpuset restriction in all: of which 3.913ms is already inside this row's account, never added again",
		"#2 udk-irq-7-76: runnable; 4.250ms modeled potential under existing rules => " + hmc377R16FDesc7EN,
	})
}

// The n03m board: the restriction seat is an engine-absorbed member of chain
// seat #2's family (every restricted instant inside #2); it has no line of
// its own, so the holder's line carries the time and the constraint. The
// family's line places it (also where the claim names no holder); where no
// line wears the family key (the family is an unranked context row) the
// claim's holder places it.
func hmc377R16FWholeBoard(familyOnLine bool, claimHolders string) []types.ObservationRecord {
	key := "rank_pair:runnable_wait>cpu_affinity_or_cpuset|pid:76|on_chain|window:140.911304..141.835458|interval:141.000000..141.000111|lines:535671-535683"
	var lineKey, contextKey []string
	if familyOnLine {
		lineKey = []string{"rank_family_key=" + key}
	} else {
		contextKey = []string{"rank_family_key=" + key}
	}
	return []types.ObservationRecord{
		hmc377R16FChain(1, ".ugc.aweme.lite-11105", 11105, "92.026"),
		hmc377R16FChain(2, "udk-irq-7-76", 76, "0.020", lineKey...),
		hmc377R16FSeat(3, 0, "udk-irq-7-76", 76, "0.014", append([]string{"share_claims=anchored/clipped_seat/1/1/0/0.014/" + claimHolders,
			"absorbed_by_rank_family=true", "absorbed_into=" + key}, hmc377R16FConstraint("7", "0,1,2,3,4,5,6")...)...),
		hmc377R16FRecord(4, "root_cause_context_only", "udk-irq-7-76", 76, "running", "0.059",
			append([]string{"tier=context_only", "source=wakeup_chain.causal_impacts", "causality=on_wakeup_chain", "chain_relevance=on_chain"}, contextKey...)...),
	}
}

func TestHMC377R16FinalizerWholeArmHolderCarriesTheAbsorbedSeat(t *testing.T) {
	for _, tc := range []struct {
		familyOnLine bool
		holders      string
		placed       bool
	}{{true, "2", true}, {true, "", true}, {false, "2", true}, {false, "", false}} {
		name := fmt.Sprintf("family on a line=%v holders=%q", tc.familyOnLine, tc.holders)
		records := hmc377R16FWholeBoard(tc.familyOnLine, tc.holders)
		zh := hmc377R16FRequest(t, "zh", records...)
		var want, wantEN []string
		if tc.placed {
			want = []string{
				"#2 udk-irq-7-76：runnable；估算优化潜力影响 0.020ms => 其中CPU亲和/cpuset限制下0.014ms是本线程runnable时间里受限的那一部分,已含在本行账内,不另计",
				"#2 udk-irq-7-76：runnable；估算优化潜力影响 0.020ms => " + hmc377R16FDesc7ZH,
			}
			wantEN = []string{
				"#2 udk-irq-7-76: runnable; 0.020ms modeled potential under existing rules => of which 0.014ms under the CPU affinity/cpuset restriction is the restricted part of this thread's runnable time, already inside this row's account, never added again",
				"#2 udk-irq-7-76: runnable; 0.020ms modeled potential under existing rules => " + hmc377R16FDesc7EN,
			}
		}
		hmc377R16FWantLines(t, name+" zh", hmc377R16FSubLines(t, zh), want)
		if strings.Contains(zh, "：CPU亲和/cpuset限制；") {
			t.Errorf("[%s] an absorbed member has no line of its own in the rank section", name)
		}
		en := hmc377R16FRequest(t, "en", records...)
		hmc377R16FWantLines(t, name+" en", hmc377R16FSubLines(t, en), wantEN)
		// The reader-label section permits the label the holder's line speaks,
		// and only where it speaks it.
		permitted := 0
		if tc.placed {
			permitted = 1
		}
		for _, face := range []struct{ prompt, label string }{{zh, "CPU亲和/cpuset限制"}, {en, "CPU affinity/cpuset restriction"}} {
			if got := strings.Count(face.prompt, "permitted_reader_cause_label="+fmt.Sprintf("%q", face.label)); got != permitted {
				t.Errorf("[%s] the label section permits %q %d times, want %d", name, face.label, got, permitted)
			}
		}
	}
}

// Second review (2026-11-25), batch owner R16X-1: an absorbed member's claim
// naming two lines where no line wears its family key has no single holder's
// line — no line states it, the reader label is not permitted for it, and no
// co-holder words are written. Where a line wears the family key, that line
// holds it (the engine absorbed the member into that family).
func TestHMC377R16FinalizerWholeArmClaimOfTwoLinesStatesNothing(t *testing.T) {
	row2 := "#2 udk-irq-7-76：runnable；估算优化潜力影响 0.020ms"
	for _, tc := range []struct {
		familyOnLine bool
		want         []string
	}{
		{false, nil},
		{true, []string{
			row2 + " => 其中CPU亲和/cpuset限制下0.014ms是本线程runnable时间里受限的那一部分,已含在本行账内,不另计",
			row2 + " => " + hmc377R16FDesc7ZH,
		}},
	} {
		name := fmt.Sprintf("family on a line=%v", tc.familyOnLine)
		records := append(hmc377R16FWholeBoard(tc.familyOnLine, "2+5"),
			hmc377R16FChainOf(5, "priority_inversion_candidate", "udk-irq-7-76", 76, "0.030"))
		zh := hmc377R16FRequest(t, "zh", records...)
		if !strings.Contains(zh, "#5 udk-irq-7-76：优先级反转候选") {
			t.Fatalf("[%s] fixture precondition: the claim's second holder has a line of its own", name)
		}
		hmc377R16FWantLines(t, name, hmc377R16FSubLines(t, zh), tc.want)
		en := hmc377R16FRequest(t, "en", records...)
		if got := len(hmc377R16FSubLines(t, en)); got != len(tc.want) {
			t.Errorf("[%s] en: %d sub-lines, want %d", name, got, len(tc.want))
		}
		permitted := 0
		if tc.want != nil {
			permitted = 1
		}
		if got := strings.Count(zh, `permitted_reader_cause_label="CPU亲和/cpuset限制"`); got != permitted {
			t.Errorf("[%s] the label section permits the restriction label %d times, want %d", name, got, permitted)
		}
		hmc377R16FNoCoHolderWords(t, name, zh, en)
	}
}

// R-377-17 ③⑤: one description per thread and request, on the thread's first
// holding line in the section's order (the claims list the later line first),
// each from its own seat's typed judgment.
func TestHMC377R16FinalizerDescribesEachThreadOnceFromItsOwnSeat(t *testing.T) {
	records := []types.ObservationRecord{
		hmc377R16FChain(1, ".ugc.aweme.lite-11105", 11105, "92.026"),
		hmc377R16FChain(2, "udk-irq-7-76", 76, "4.000"),
		hmc377R16FChainOf(3, "priority_inversion_candidate", "udk-irq-7-76", 76, "1.500"),
		hmc377R16FChain(4, "udk-irq-6-75", 75, "2.000"),
		hmc377R16FSeat(5, 1, "udk-irq-7-76", 76, "1.000", append(hmc377R16FSplit("6.000", "5.000", "1.000",
			"unwalked/unwalked_seat/1/1/0/1.200/3;anchored/chain_seat/1/1/0/3.800/2"), hmc377R16FConstraint("7", "0,1,2,3,4,5,6")...)...),
		hmc377R16FSeat(6, 2, "udk-irq-6-75", 75, "0.500", append(hmc377R16FSplit("1.300", "0.800", "0.500", "anchored/residual_seat/1/1/0/0.800/4"),
			hmc377R16FConstraint("6", "0,1,2,3,4,5,7", "cpu_constraint_allowed_max_tier_khz=2188800", "cpu_constraint_global_max_tier_khz=2400000")...)...),
	}
	seat76 := "原始排序 #1 udk-irq-7-76：CPU亲和/cpuset限制；原始排序计量 1.000ms（仅支撑，不授予根因或优化收益）"
	seat75 := "原始排序 #2 udk-irq-6-75：CPU亲和/cpuset限制；原始排序计量 0.500ms（仅支撑，不授予根因或优化收益）"
	row2, row3, row4 := "#2 udk-irq-7-76：runnable；估算优化潜力影响 4.000ms", "#3 udk-irq-7-76：优先级反转候选；估算优化潜力影响 1.500ms", "#4 udk-irq-6-75：runnable；估算优化潜力影响 2.000ms"
	hmc377R16FWantLines(t, "zh", hmc377R16FSubLines(t, hmc377R16FRequest(t, "zh", records...)), []string{
		seat76 + " => CPU亲和/cpuset限制下合计6.000ms:其中1.200ms已含在排名清单 2 #3账内,不另计",
		seat76 + " => CPU亲和/cpuset限制下合计6.000ms:其中3.800ms已含在排名清单 2 #2账内,不另计",
		seat75 + " => CPU亲和/cpuset限制下合计1.300ms:其中0.800ms已含在排名清单 2 #4账内,不另计",
		row2 + " => CPU亲和/cpuset限制下合计6.000ms:其中3.800ms已含在本行账内,不另计",
		row2 + " => " + hmc377R16FDesc7ZH,
		row3 + " => CPU亲和/cpuset限制下合计6.000ms:其中1.200ms已含在本行账内,不另计",
		row4 + " => CPU亲和/cpuset限制下合计1.300ms:其中0.800ms已含在本行账内,不另计",
		row4 + " => CPU约束描述:允许核 6 · 排除全域观测核 0-5,7 · 绑核排除更大核档(允许核最高档 2.19GHz < 全域最大核档 2.40GHz) · 判定依据 sched_switch_next_info",
	})
}

// Case A' (the seat's chain-account claim stands beside a walked part: the
// chain seat keeps its own account), a claim naming two holders (the engine
// types no split between them: no holder's line states it, the seat's own
// line names both — second review 2026-11-25, batch owner R16X-1), and the
// merge of two parts of one seat inside one line's own account — only when
// both are of the same account form.
func TestHMC377R16FinalizerOwnAccountSharedClaimAndMerge(t *testing.T) {
	board := func(claims string) []types.ObservationRecord {
		return []types.ObservationRecord{
			hmc377R16FChain(1, ".ugc.aweme.lite-11105", 11105, "92.026"),
			hmc377R16FChain(2, "udk-irq-7-76", 76, "4.250"),
			hmc377R16FChain(3, ".ugc.aweme.lite-11105", 11105, "3.000"),
			hmc377R16FChainOf(4, "priority_inversion_candidate", "udk-irq-7-76", 76, "1.100"),
			hmc377R16FSeat(5, 1, "udk-irq-7-76", 76, "2.693", append(hmc377R16FSplit("6.606", "3.913", "2.693", claims), hmc377R16FConstraint("7", "0,1,2,3,4,5,6")...)...),
		}
	}
	row2 := "#2 udk-irq-7-76：runnable；估算优化潜力影响 4.250ms"
	row2en := "#2 udk-irq-7-76: runnable; 4.250ms modeled potential under existing rules"
	head := "CPU亲和/cpuset限制下合计6.606ms:其中"
	headEN := "6.606ms under the CPU affinity/cpuset restriction in all: of which "
	for _, tc := range []struct {
		name, claims string
		zh, en       []string
	}{
		{"case A'", "chain_account/chain_seat/1/1/0/3.000/2;anchored/chain_seat/1/1/0/0.272/2", []string{
			hmc377R16FSeatZH + " => " + head + "0.272ms落在链上席排名清单 2 #2时段内(链席另列自账)",
			row2 + " => " + head + "0.272ms落在本行时段内(本行另列自账)",
			row2 + " => " + hmc377R16FDesc7ZH,
		}, []string{
			hmc377R16FSeatEN + " => " + headEN + "0.272ms lies inside the time of the chain seat ranking list 2 #2 (the chain seat keeps its own separate account)",
			row2en + " => " + headEN + "0.272ms lies inside the time of this row (this row keeps its own separate account)",
			row2en + " => " + hmc377R16FDesc7EN,
		}},
		{"one claim, two holders", "anchored/chain_seat/1/1/0/2.365/4+2", []string{
			hmc377R16FSeatZH + " => " + head + "2.365ms已含在排名清单 2 #2、排名清单 2 #4账内,不另计",
		}, []string{
			hmc377R16FSeatEN + " => " + headEN + "2.365ms is already inside the accounts of ranking list 2 #2, ranking list 2 #4, never added again",
		}},
		{"two parts in one line's own account", "anchored/clipped_seat/1/1/0/1.000/2;unwalked/unwalked_seat/1/1/0/0.500/2", []string{
			hmc377R16FSeatZH + " => " + head + "1.000ms已含在排名清单 2 #2账内,不另计",
			hmc377R16FSeatZH + " => " + head + "0.500ms已含在排名清单 2 #2账内,不另计",
			row2 + " => " + head + "1.500ms已含在本行账内,不另计",
			row2 + " => " + hmc377R16FDesc7ZH,
		}, nil},
		{"two parts, one shared with another line", "anchored/clipped_seat/1/1/0/1.000/2;unwalked/unwalked_seat/1/1/0/0.500/2+4", []string{
			hmc377R16FSeatZH + " => " + head + "1.000ms已含在排名清单 2 #2账内,不另计",
			hmc377R16FSeatZH + " => " + head + "0.500ms已含在排名清单 2 #2、排名清单 2 #4账内,不另计",
			row2 + " => " + head + "1.000ms已含在本行账内,不另计",
			row2 + " => " + hmc377R16FDesc7ZH,
		}, nil},
		{"the shared part first", "unwalked/unwalked_seat/1/1/0/0.500/2+4;anchored/clipped_seat/1/1/0/1.000/2", []string{
			hmc377R16FSeatZH + " => " + head + "0.500ms已含在排名清单 2 #2、排名清单 2 #4账内,不另计",
			hmc377R16FSeatZH + " => " + head + "1.000ms已含在排名清单 2 #2账内,不另计",
			row2 + " => " + head + "1.000ms已含在本行账内,不另计",
			row2 + " => " + hmc377R16FDesc7ZH,
		}, nil},
		{"two parts of two account forms", "chain_account/chain_seat/1/1/0/3.000/2;anchored/chain_seat/1/1/0/1.000/2;unwalked/unwalked_seat/1/1/0/0.500/2", []string{
			hmc377R16FSeatZH + " => " + head + "1.000ms落在链上席排名清单 2 #2时段内(链席另列自账)",
			hmc377R16FSeatZH + " => " + head + "0.500ms已含在排名清单 2 #2账内,不另计",
			row2 + " => " + head + "1.000ms落在本行时段内(本行另列自账)",
			row2 + " => " + hmc377R16FDesc7ZH,
			row2 + " => " + head + "0.500ms已含在本行账内,不另计",
		}, nil},
	} {
		records := board(tc.claims)
		zh := hmc377R16FRequest(t, "zh", records...)
		hmc377R16FWantLines(t, tc.name+" zh", hmc377R16FSubLines(t, zh), tc.zh)
		en := hmc377R16FRequest(t, "en", records...)
		if tc.en != nil {
			hmc377R16FWantLines(t, tc.name+" en", hmc377R16FSubLines(t, en), tc.en)
		}
		hmc377R16FNoCoHolderWords(t, tc.name, zh, en)
	}
}

// hmc377R16FNoCoHolderWords: no line of the request names a holder beside
// its own account (R16X-1: no co-holder words in round 16).
func hmc377R16FNoCoHolderWords(t *testing.T, name, zh, en string) {
	t.Helper()
	for _, w := range []string{"本行与", "accounts of this row and"} {
		if strings.Contains(zh+en, w) {
			t.Errorf("[%s] the request names a co-holder beside its own account (%q)", name, w)
		}
	}
}

// A holder is named in the words of its own list: a support-list holder on
// the seat's list by its published order, one on another list with that
// list's name too — on the seat's line, for a claim of one holder and for a
// claim of two holders on two lists (stated on the seat's line only), in both
// languages.
func TestHMC377R16FinalizerNamesASupportListHolderInItsListsWords(t *testing.T) {
	records := []types.ObservationRecord{
		hmc377R16FChain(1, ".ugc.aweme.lite-11105", 11105, "92.026"),
		hmc377R16FSeat(2, 1, "udk-irq-7-76", 76, "2.693", append(hmc377R16FSplit("6.606", "3.913", "2.693", "anchored/residual_seat/1/1/0/3.000/3;unwalked/unwalked_seat/1/1/0/0.913/4+5"),
			hmc377R16FConstraint("7", "0,1,2,3,4,5,6")...)...),
		hmc377R16FRecord(3, "root_cause_adjacent", "udk-irq-7-76", 76, "runnable_wait", "3.000", "rank=2", "tier=tertiary", "source=window_stats", "causality=adjacent_to_wakeup_chain", "chain_relevance=adjacent"),
		hmc377R16FRecord(4, "root_cause_background", "udk-irq-7-76", 76, "runnable_wait", "0.913", "rank=1", "tier=background", "source=window_stats", "causality=background_pressure", "chain_relevance=background"),
		hmc377R16FRecord(5, "root_cause_tertiary", "udk-irq-7-76", 76, "priority_inversion_candidate", "1.200", "rank=2", "tier=tertiary", "source=window_stats", "causality=on_wakeup_chain", "chain_relevance=on_chain"),
	}
	seat := hmc377R16FSeatZH
	adjacent := "原始排序 #2 udk-irq-7-76：runnable；原始排序计量 3.000ms（仅支撑，不授予根因或优化收益）"
	hmc377R16FWantLines(t, "zh", hmc377R16FSubLines(t, hmc377R16FRequest(t, "zh", records...)), []string{
		seat + " => CPU亲和/cpuset限制下合计6.606ms:其中3.000ms已含在原始排序 #2账内,不另计",
		seat + " => CPU亲和/cpuset限制下合计6.606ms:其中0.913ms已含在支撑观测清单 2 原始排序 #1、排名清单 3 #2账内,不另计",
		adjacent + " => CPU亲和/cpuset限制下合计6.606ms:其中3.000ms已含在本行账内,不另计",
		adjacent + " => " + hmc377R16FDesc7ZH,
	})
	adjacentEN := "Published order #2 udk-irq-7-76: runnable; published ranking measure 3.000ms (support only, not cause or optimization-benefit authority)"
	headEN := "6.606ms under the CPU affinity/cpuset restriction in all: of which "
	hmc377R16FWantLines(t, "en", hmc377R16FSubLines(t, hmc377R16FRequest(t, "en", records...)), []string{
		hmc377R16FSeatEN + " => " + headEN + "3.000ms is already inside the account of published order #2, never added again",
		hmc377R16FSeatEN + " => " + headEN + "0.913ms is already inside the accounts of supporting observation list 2 published order #1, ranking list 3 #2, never added again",
		adjacentEN + " => " + headEN + "3.000ms is already inside this row's account, never added again",
		adjacentEN + " => " + hmc377R16FDesc7EN,
	})
}

// No typed hold, no line: a restriction seat whose account the engine did not
// split, a claim naming a record that is on no line of the request, a seat of
// another type carrying the same fields, and an absorbed marker naming no
// family present on the board (the engine's fold leaves it in place).
func TestHMC377R16FinalizerStatesNothingWithoutTheTypedHold(t *testing.T) {
	chain := []types.ObservationRecord{
		hmc377R16FChain(1, ".ugc.aweme.lite-11105", 11105, "92.026"),
		hmc377R16FChain(2, "udk-irq-7-76", 76, "4.250"),
	}
	for _, tc := range []struct {
		name string
		seat types.ObservationRecord
	}{
		{"unsplit seat", hmc377R16FSeat(3, 1, "udk-irq-7-76", 76, "6.606", hmc377R16FConstraint("7", "0,1,2,3,4,5,6")...)},
		{"holder on no line", hmc377R16FSeat(3, 1, "udk-irq-7-76", 76, "2.693", append(hmc377R16FSplit("6.606", "3.913", "2.693", "anchored/clipped_seat/1/1/0/3.913/2+7"),
			hmc377R16FConstraint("7", "0,1,2,3,4,5,6")...)...)},
		{"another type", hmc377R16FRecord(3, "root_cause_adjacent", "udk-irq-7-76", 76, "runnable_wait", "2.693", append([]string{"rank=1", "tier=tertiary", "chain_relevance=adjacent"},
			append(hmc377R16FSplit("6.606", "3.913", "2.693", "anchored/clipped_seat/1/1/0/3.913/2"), hmc377R16FConstraint("7", "0,1,2,3,4,5,6")...)...)...)},
		{"absorbed marker naming no present family", hmc377R16FSeat(3, 0, "udk-irq-7-76", 76, "0.014", append([]string{"share_claims=anchored/clipped_seat/1/1/0/0.014/",
			"absorbed_by_rank_family=true", "absorbed_into=rank_pair:no-such-family"}, hmc377R16FConstraint("7", "0,1,2,3,4,5,6")...)...)},
	} {
		for _, lang := range []string{"zh", "en"} {
			if got := hmc377R16FSubLines(t, hmc377R16FRequest(t, lang, append(append([]types.ObservationRecord(nil), chain...), tc.seat)...)); len(got) != 0 {
				t.Errorf("[%s %s] no typed hold, no line:\n%s", tc.name, lang, strings.Join(got, "\n"))
			}
		}
	}
}

// A holder named by a record merged into a line (the projection merges two
// same-type rows of one thread into one line that stands for both records)
// is placed on that line, once even where the claim names both of the line's
// records. The roster seat carries the merged ids from the node
// (BuildTraceRankRosterAuthorities).
func TestHMC377R16FinalizerPlacesAHolderMergedIntoALine(t *testing.T) {
	for _, holders := range []string{"3", "2+3"} {
		records := []types.ObservationRecord{
			hmc377R16FChain(1, ".ugc.aweme.lite-11105", 11105, "92.026"),
			hmc377R16FChain(2, "udk-irq-7-76", 76, "4.250"),
			hmc377R16FChain(3, "udk-irq-7-76", 76, "1.500"),
			hmc377R16FSeat(4, 1, "udk-irq-7-76", 76, "2.693", append(hmc377R16FSplit("6.606", "3.913", "2.693", "anchored/clipped_seat/1/1/0/3.913/"+holders),
				hmc377R16FConstraint("7", "0,1,2,3,4,5,6")...)...),
		}
		merged := false
		for _, projection := range types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records}).Projections {
			for _, node := range projection.RankedSeats {
				if node.EvidenceID == hmc377R16FScope+"#root_cause_rank:2" {
					merged = strings.Contains(strings.Join(node.MergedEvidenceIDs, ","), hmc377R16FScope+"#root_cause_rank:3")
				}
			}
		}
		if !merged {
			t.Fatal("fixture precondition: the projection merges row 3 into row 2")
		}
		hmc377R16FWantLines(t, "holders="+holders, hmc377R16FSubLines(t, hmc377R16FRequest(t, "zh", records...)), []string{
			hmc377R16FSeatZH + " => CPU亲和/cpuset限制下合计6.606ms:其中3.913ms已含在排名清单 2 #2账内,不另计",
			"#2 udk-irq-7-76：runnable；估算优化潜力影响 4.250ms => CPU亲和/cpuset限制下合计6.606ms:其中3.913ms已含在本行账内,不另计",
			"#2 udk-irq-7-76：runnable；估算优化潜力影响 4.250ms => " + hmc377R16FDesc7ZH,
		})
	}
}

// hmc377R16FOtherCapture is a second capture's board: the records of the
// split board under another capture path and record scope, its restriction
// seat on thread 74 (allowed CPU 5). dropParams leaves the board's query
// settings out, so the request lists its rows one by one (identity
// incomplete).
func hmc377R16FOtherCapture(scope string, dropParams bool) []types.ObservationRecord {
	records := hmc377R16FUnderPath([]types.ObservationRecord{
		hmc377R16FChain(1, ".ugc.aweme.lite-11105", 11105, "50.000"),
		hmc377R16FChain(2, "udk-irq-5-74", 74, "1.800"),
		hmc377R16FSeat(3, 1, "udk-irq-5-74", 74, "0.500", append(hmc377R16FSplit("2.000", "1.500", "0.500", "anchored/clipped_seat/1/1/0/1.500/2"),
			hmc377R16FConstraint("5", "0,1,2,3,4,6,7")...)...),
	}, scope, "/captures/yuanbao-reboot.ftrace")
	for i := range records {
		if dropParams {
			var notes []string
			for _, note := range records[i].RichNotes {
				if !strings.HasPrefix(note, "rank_board_params_fingerprint=") {
					notes = append(notes, note)
				}
			}
			records[i].RichNotes = notes
		}
	}
	return records
}

// hmc377R16FUnderPath is a board's records under another capture path and
// record scope (the same capture attached twice keeps its family keys).
func hmc377R16FUnderPath(records []types.ObservationRecord, scope, path string) []types.ObservationRecord {
	out := make([]types.ObservationRecord, len(records))
	for i, r := range records {
		r.ID = strings.Replace(r.ID, hmc377R16FScope, scope, 1)
		r.SourceRef.Path = path
		r.RichNotes = append([]string(nil), r.RichNotes...)
		out[i] = r
	}
	return out
}

// Two captures: the decision is read per capture against that capture's
// lines only, with each list numbered in the section — also where the second
// board's rows are listed one by one, and where the same capture is attached
// under a second path (its family keys repeat): each capture's thread is
// described on its own holding line.
func TestHMC377R16FinalizerReadsEachCapturesDecisionAgainstItsOwnLines(t *testing.T) {
	desc5 := "CPU约束描述:允许核 5 · 排除全域观测核 0-4,6-7 · 判定依据 sched_switch_next_info"
	seat74 := "原始排序 #1 udk-irq-5-74：CPU亲和/cpuset限制；原始排序计量 0.500ms（仅支撑，不授予根因或优化收益）"
	row74 := "#2 udk-irq-5-74：runnable；估算优化潜力影响 1.800ms"
	douyin := []string{
		hmc377R16FSeatZH + " => CPU亲和/cpuset限制下合计6.606ms:其中3.913ms已含在排名清单 2 #2账内,不另计",
		"#2 udk-irq-7-76：runnable；估算优化潜力影响 4.250ms => CPU亲和/cpuset限制下合计6.606ms:其中3.913ms已含在本行账内,不另计",
		"#2 udk-irq-7-76：runnable；估算优化潜力影响 4.250ms => " + hmc377R16FDesc7ZH,
	}
	for _, tc := range []struct {
		name       string
		dropParams bool
		list       string
	}{
		{"two boards", false, "排名清单 4 #2"},
		{"rows listed one by one", true, "排名清单 5 #2"},
	} {
		records := append(hmc377R16FSplitBoard(), hmc377R16FOtherCapture("trace_query:publication-r16g", tc.dropParams)...)
		hmc377R16FWantLines(t, tc.name, hmc377R16FSubLines(t, hmc377R16FRequest(t, "zh", records...)), append(append([]string(nil), douyin...),
			seat74+" => CPU亲和/cpuset限制下合计2.000ms:其中1.500ms已含在"+tc.list+"账内,不另计",
			row74+" => CPU亲和/cpuset限制下合计2.000ms:其中1.500ms已含在本行账内,不另计",
			row74+" => "+desc5,
		))
	}
	// The copy's holder row states 0.025 ms (the lines of the two captures
	// differ, so the listing tells them apart); its family key is the same.
	whole := hmc377R16FWholeBoard(true, "2")
	copied := hmc377R16FUnderPath(whole, "trace_query:publication-r16h", "/captures/douyin-reboot-copy.ftrace")
	copied[1].Value = "0.025"
	for i, note := range copied[1].RichNotes {
		copied[1].RichNotes[i] = strings.Replace(note, "=0.020", "=0.025", 1)
	}
	records := append(append([]types.ObservationRecord(nil), whole...), copied...)
	var want []string
	for _, row := range []string{"#2 udk-irq-7-76：runnable；估算优化潜力影响 0.020ms", "#2 udk-irq-7-76：runnable；估算优化潜力影响 0.025ms"} {
		want = append(want, row+" => 其中CPU亲和/cpuset限制下0.014ms是本线程runnable时间里受限的那一部分,已含在本行账内,不另计", row+" => "+hmc377R16FDesc7ZH)
	}
	hmc377R16FWantLines(t, "the same capture under two paths", hmc377R16FSubLines(t, hmc377R16FRequest(t, "zh", records...)), want)
}

// A seat without a typed CPU-constraint judgment: its time is stated, and no
// description line is written.
func TestHMC377R16FinalizerWritesNoDescriptionWithoutTheTypedJudgment(t *testing.T) {
	records := []types.ObservationRecord{
		hmc377R16FChain(1, ".ugc.aweme.lite-11105", 11105, "92.026"),
		hmc377R16FChain(2, "udk-irq-7-76", 76, "4.250"),
		hmc377R16FSeat(3, 1, "udk-irq-7-76", 76, "2.693", hmc377R16FSplit("6.606", "3.913", "2.693", "anchored/clipped_seat/1/1/0/3.913/2")...),
	}
	hmc377R16FWantLines(t, "zh", hmc377R16FSubLines(t, hmc377R16FRequest(t, "zh", records...)), []string{
		hmc377R16FSeatZH + " => CPU亲和/cpuset限制下合计6.606ms:其中3.913ms已含在排名清单 2 #2账内,不另计",
		"#2 udk-irq-7-76：runnable；估算优化潜力影响 4.250ms => CPU亲和/cpuset限制下合计6.606ms:其中3.913ms已含在本行账内,不另计",
	})
}

// One thread's restriction seat hands one part to a line of the support list
// (list 1, its second line) and one part to a line of the chain list (list 2,
// its first line). The thread's description goes on its first holding line in
// the section's order, so on the support list's line: the order compares the
// lists before the seats on them (separating shape of the merge verifier's
// mutant N13, 2026-11-25).
func TestHMC377R16FinalizerDescriptionFollowsTheSectionOrderAcrossLists(t *testing.T) {
	records := []types.ObservationRecord{
		hmc377R16FChain(1, "udk-irq-7-76", 76, "4.250"),
		hmc377R16FSeat(2, 1, "udk-irq-7-76", 76, "2.693", append(hmc377R16FSplit("6.606", "3.913", "2.693", "anchored/residual_seat/1/1/0/3.000/3;unwalked/unwalked_seat/1/1/0/0.913/1"),
			hmc377R16FConstraint("7", "0,1,2,3,4,5,6")...)...),
		hmc377R16FRecord(3, "root_cause_adjacent", "udk-irq-7-76", 76, "runnable_wait", "3.000", "rank=2", "tier=tertiary", "source=window_stats", "causality=adjacent_to_wakeup_chain", "chain_relevance=adjacent"),
	}
	got := hmc377R16FSubLines(t, hmc377R16FRequest(t, "zh", records...))
	support := "原始排序 #2 udk-irq-7-76：runnable；原始排序计量 3.000ms（仅支撑，不授予根因或优化收益）"
	chain := "#1 udk-irq-7-76：runnable；估算优化潜力影响 4.250ms"
	var onSupport, onChain int
	for _, l := range got {
		switch l {
		case support + " => " + hmc377R16FDesc7ZH:
			onSupport++
		case chain + " => " + hmc377R16FDesc7ZH:
			onChain++
		}
	}
	if onSupport != 1 || onChain != 0 {
		t.Errorf("the description sits once on the thread's first holding line in the section's order (the support list's line): support %d, chain %d\n  %s", onSupport, onChain, strings.Join(got, "\n  "))
	}
}

package orchestrator

// prose_fact_factrel_test.go — FACT-REL 件② pins (§29.55.4 F2 互斥状态包含
// 关系编造 + R2-F1 claim-of-absence, docs/design/real_trace_campaign_20260705.md,
// 2026-07-13). §29.53.2 discipline holds: PRESENCE trigger + typed fact
// listing only — the system never reads what relation the prose claimed, and
// the only verdict lane anywhere stays pure arithmetic.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestFactRel_PartitionFactOnMultiStateProse — arm a, account form (F2
// witness shape: prose pairs two state values for one thread in one unit).
// The appendix lists the typed FIVE-state account with the mutual-exclusion
// partition fact, Σ decomposed with real addends and the window length
// (件4: io_wait is the account's own fifth lane).
func TestFactRel_PartitionFactOnMultiStateProse(t *testing.T) {
	mut := psgTraceMutable(cr4FactRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("ugc.aweme.lite-17267 的 running 157.248ms 已包含在 sleep 70.338ms 之内，可整体相加。")

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var hit string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "五态为互斥分区") {
			hit = zh
		}
	}
	if hit == "" {
		t.Fatalf("the partition fact must render on multi-state prose, got %+v", facts)
	}
	for _, want := range []string{
		".ugc.aweme.lite-17267",
		"running 157.248/runnable 5.604/sleep 70.338/D-state 0.000/io_wait 0.000ms",
		"同一时刻仅居一态",
		"不存在包含关系",
		// 附注自证义务: the Σ equation prints the real addends and the real
		// computed sum, next to the window length.
		"Σ=157.248+5.604+70.338+0.000+0.000=233.190ms",
		"窗长 233.190ms",
	} {
		if !strings.Contains(hit, want) {
			t.Fatalf("partition fact missing %q: %s", want, hit)
		}
	}
	cr4BannedWordingCheck(t, []string{hit})
}

// factRelAccountRecord builds a target_window_states record WITH the io_wait
// lane (件4; p6AccountRecord predates the five-lane form).
func factRelAccountRecord(subject string, running, runnable, sleep, dstate, ioWait, total, windowMS float64) types.ObservationRecord {
	rec := p6AccountRecord(subject, running, runnable, sleep, dstate, 0, total, windowMS)
	rec.RichNotes = append(rec.RichNotes, fmt.Sprintf("%s=%.3f", types.TraceNoteKeyIOWait, ioWait))
	return rec
}

// TestFactRel_PartitionFactIOWaitLane — 件4 witness shape (donghu 常态
// io_wait>0): the account's io_wait lane joins the decomposition and the Σ
// identity claim holds only because all FIVE lanes are listed.
func TestFactRel_PartitionFactIOWaitLane(t *testing.T) {
	acct := factRelAccountRecord("com.baidu.tieba-59566", 7.738, 1.390, 74.552, 17.613, 13.647, 114.940, 114.940)
	mut := psgTraceMutable(acct)
	bus := psgBus(mut)
	doc := psgProseDoc("com.baidu.tieba-59566 的 running 7.738ms 已包含在 sleep 74.552ms 内。")

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var hit string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "五态为互斥分区") {
			hit = zh
		}
	}
	if hit == "" {
		t.Fatalf("the io_wait-lane partition fact must render, got %+v", facts)
	}
	for _, want := range []string{
		"io_wait 13.647ms",
		"Σ=7.738+1.390+74.552+17.613+13.647=114.940ms",
		"窗长 114.940ms",
	} {
		if !strings.Contains(hit, want) {
			t.Fatalf("io_wait-lane partition fact missing %q: %s", want, hit)
		}
	}
}

// TestFactRel_PartitionFactUnbalancedNoIdentityClaim — 件4 balance gate: an
// account whose five lanes do NOT sum to the window (clipped/partial shape)
// lists Σ and the window side by side but never claims Σ=窗长.
func TestFactRel_PartitionFactUnbalancedNoIdentityClaim(t *testing.T) {
	acct := factRelAccountRecord("com.baidu.tieba-59566", 7.738, 1.390, 74.552, 17.613, 0, 114.940, 114.940)
	mut := psgTraceMutable(acct)
	bus := psgBus(mut)
	doc := psgProseDoc("com.baidu.tieba-59566 的 running 7.738ms 已包含在 sleep 74.552ms 内。")

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var hit string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "五态为互斥分区") {
			hit = zh
		}
	}
	if hit == "" {
		t.Fatalf("the unbalanced partition fact must still list the values, got %+v", facts)
	}
	if !strings.Contains(hit, "Σ五态=101.293ms;窗长 114.940ms") {
		t.Fatalf("unbalanced form must list Σ and window side by side: %s", hit)
	}
	if strings.Contains(hit, "=114.940ms,窗长") {
		t.Fatalf("unbalanced form must not mint the Σ=窗长 identity claim: %s", hit)
	}
}

// TestFactRel_SeatWithoutEffectiveKeepsMemberCount — 件6 pin: a seat whose
// record published no effective magnitude still renders its member-count
// chip (the generic paren renderer no longer gates 成员共N on hasEff).
func TestFactRel_SeatWithoutEffectiveKeepsMemberCount(t *testing.T) {
	seat := psgTraceRecord("trace_query:t#root_cause_rank:1", "root_cause_rank_2", "",
		"rank=2", types.TraceNoteKeyMemberCount+"=7")
	seat.Subject = "ThreadPoolForeg-60555"
	seat.Object = "io_wait"
	seat.Value = ""
	mut := psgTraceMutable(seat)
	bus := psgBus(mut)
	doc := psgProseDoc("ThreadPoolForeg-60555 的 IO 等待是链源。")
	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var line string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "typed 席位=") {
			line = zh
		}
	}
	if !strings.Contains(line, "#2(成员共7)") {
		t.Fatalf("a no-effective seat must keep its member-count chip, got %q (%+v)", line, facts)
	}
}

// TestFactRel_NoAccountNoPartitionFact — 件3 (复核 P1-2): the rank-note
// state-dims FALLBACK is retired — rank state notes drift semantically per
// row kind, so a cross-row per-dim MAX is a chimera. No target_window_states
// account → NO partition fact (宁缺勿假), even on multi-state prose.
func TestFactRel_NoAccountNoPartitionFact(t *testing.T) {
	comp := psgTraceRecord("trace_query:t#root_cause_rank:1", "root_cause_primary", "36.757",
		"rank=1", "effective_impact_ms=36.757",
		types.TraceNoteKeyRunning+"=7.081",
		types.TraceNoteKeyRunnable+"=1.203",
		types.TraceNoteKeySleep+"=12.400",
		types.TraceNoteKeyDState+"=36.757")
	comp.Subject = "CompThread_0-2955"
	comp.Object = "d_state_or_io_wait"
	mut := psgTraceMutable(comp)
	bus := psgBus(mut)
	doc := psgProseDoc("CompThread_0-2955 的 running 7.081ms 已包含在 D-state 36.757ms 内。")

	for _, f := range proseFactJuxtapositionFindings(doc, bus, mut) {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "互斥分区") || strings.Contains(zh, "两两互斥") || strings.Contains(zh, "窗内状态账") {
			t.Fatalf("no-account threads must not draw a partition fact: %s", zh)
		}
	}
}

// TestFactRel_SingleStateProseSilent — arm a trigger discipline: one state
// token (or none) in the unit → no partition fact.
func TestFactRel_SingleStateProseSilent(t *testing.T) {
	mut := psgTraceMutable(cr4FactRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("ugc.aweme.lite-17267 在窗口内 running 157.248ms，主要在执行渲染。")

	for _, f := range proseFactJuxtapositionFindings(doc, bus, mut) {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "互斥分区") || strings.Contains(zh, "两两互斥") {
			t.Fatalf("single-state prose must not draw the partition fact: %s", zh)
		}
	}
}

// TestFactRel_SeatRosterCarriesUnprovenRemainder — arm b (R2-F1 tieba
// witness: prose closed with a coverage/absence claim while the report
// seated a 10.433ms cause-unproven remainder). The seat roster fact line
// itself is the counter-face: cause seats wear their typed 等待对象 word and
// the remainder seat wears 原因未证 — including past the roster capacity cap.
func TestFactRel_SeatRosterCarriesUnprovenRemainder(t *testing.T) {
	seat := func(id string, rank, eff string, notes ...string) types.ObservationRecord {
		rec := psgTraceRecord(id, "root_cause_rank_"+rank, eff,
			append([]string{"rank=" + rank, "effective_impact_ms=" + eff}, notes...)...)
		rec.Subject = "ThreadPoolForeg-60555"
		rec.Object = "io_wait"
		return rec
	}
	mut := psgTraceMutable(
		seat("trace_query:t#root_cause_rank:1", "1", "7.386",
			types.TraceNoteKeyMemberCount+"=17",
			types.TraceNoteKeyBlockedReasonCaller+"=fscache_page_wait_o"),
		seat("trace_query:t#root_cause_rank:2", "2", "0.171",
			types.TraceNoteKeyBlockedReasonCaller+"=hmfs_get_dnode"),
		seat("trace_query:t#root_cause_rank:3", "3", "0.145",
			types.TraceNoteKeyBlockedReasonCaller+"=hmfs_read"),
		seat("trace_query:t#root_cause_rank:4", "4", "0.100"),
		// The remainder seat arrives LAST with the roster already at the
		// capacity cap — it must still take its chip (claim-of-absence
		// counter-face never falls to a display cap).
		seat("trace_query:t#root_cause_rank:5", "5", "10.433",
			types.TraceNoteKeyDStateCauseUnprovenRemainder+"=true"),
	)
	bus := psgBus(mut)
	doc := psgProseDoc("ThreadPoolForeg-60555 的三类 IO 等待原因已覆盖全部 D-sleep 段，未证实的原因不存在。")

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var line string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "ThreadPoolForeg-60555") && strings.Contains(zh, "typed 席位=") {
			line = zh
		}
	}
	if line == "" {
		t.Fatalf("the seat roster fact line must render, got %+v", facts)
	}
	for _, want := range []string{
		"#1(有效归因 7.386ms,成员共17,等待对象=fscache_page_wait_o)",
		"#5(有效归因 10.433ms,原因未证)",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("seat roster missing %q: %s", want, line)
		}
	}
	cr4BannedWordingCheck(t, []string{line})
}

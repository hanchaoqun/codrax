package tracequery

// binder_attribution.go — P9 binder attribution write-off gate (CR-1 件①,
// §29.42 案1 BINDER-MISATTR, docs/design/real_trace_campaign_20260705.md,
// 2026-07-12; cold-read pass table §3.2 P9).
//
// Production witness (donghu.ftrace, 41006 report): the binder_wait
// classifier attributed the target's frame-pacing sleep segments to a
// synchronous binder transaction whose reply had ALREADY completed ~97ms
// earlier — the 100ms send-lookback window in findBinderWaitsForChain matched
// txn 12145963 (sent 13762.894465, reply 12145964 completed 13762.895412,
// ~1.0ms round trip) to a 15.758ms sleep segment that started at ~13762.99242
// right after a ONEWAY transaction, and that was actually ended by the
// surfaceflinger vsync-dispatch thread app-9511 (tgid 9432, not the blamed
// peer's 9743). Rank1 narrated "binder 同步等待约45ms" while the true
// synchronous round-trip total in the window was ≈3.5ms (13× inflation).
//
// Three arms, every input a PRECISE signal (timestamp comparison, integer
// tgid equality, verbatim span-name classification, single-meaning tolerance
// constant):
//
//	arm a (reply write-off): before a sleep segment is attributed to
//	  transaction T, look up T's reply completion — the first reply-flagged
//	  binder_transaction row directed at the sender thread at or after T's
//	  send. A completion timestamp strictly BEFORE the segment start means T
//	  was already finished and cannot explain the segment: the candidate edge
//	  is written off. No reply row in the trace keeps the current behavior
//	  (fail-open; the existing endpoint-only caveats already disclose it).
//	arm b (peer-process consistency): the segment-ending waker's tgid must
//	  match the attributed peer's tgid. On a mismatch the attribution is
//	  written off — UNLESS the reply verifiably arrived INSIDE the segment,
//	  in which case the binder wait stands and a typed caveat discloses the
//	  waker/peer process mismatch (如实披露, never silent). Unknown tgids on
//	  either side skip the comparison (precise signals only; absence never
//	  judges).
//	arm c (pacing rerouting): a segment whose binder candidates were all
//	  written off and whose length is within
//	  binderPacingFramePeriodToleranceMs of one plausible period is re-minted
//	  on an idle-cadence semantic lane instead — never a root-cause contender
//	  (rank rows carry RootCauseTierContextOnly via the dedicated type arm in
//	  assignRootCauseRanksAndTiers). The lane WORD forks on the segment-
//	  ending waker's frame-chain membership (复核 P2-1 勘正, 2026-07-12 —
//	  the pre-fork form let ANY 4-50ms periodic waker (timer/audio) wear the
//	  frame promise words):
//	    · pacing_idle (帧间空闲/waiting for the next frame): the waker
//	      provably emitted a frame-schedule span (verbatim span-name
//	      classification via classifyFramePhase: vsync / choreographer /
//	      doframe). Period evidence: the waker's VS-1 PeriodicSource
//	      aggregate (DetectedPeriodMs), or the waker's own consecutive
//	      wakeup cadence toward the sleeping thread bracketing the segment.
//	    · periodic_idle (周期空闲/waiting for the next periodic signal): the
//	      waker carries the typed VS-1 PeriodicSource aggregate but NO
//	      frame-schedule span — a generic periodic signal source. The
//	      aggregate is the ONLY admissible period evidence here (a bare
//	      two-wakeup cadence is not proof of periodicity).
//
// Only a positively decoded synchronous request may participate as "the
// waited-on transaction". Replies, oneway requests (flags bit 0x1) and
// malformed/unknown reply-or-flags argsets remain IPC inventory but fail
// closed at the wait-minting gate. Genuine synchronous waits (reply arrives
// inside the segment, waker process == peer process) are preserved
// byte-identically — pinned by the donghu-extracted positive fixture
// (binder_attribution_p9_test.go).

import (
	"fmt"
	"sort"
	"strings"
)

// binderPacingFramePeriodToleranceMs is the SINGLE-MEANING tolerance of the
// P9 arm-c pacing check: |sleep segment length − one frame period| must be
// below this constant for the segment to read as frame-pacing idle. The gap
// equals the thread's awake time inside the frame, so the constant is
// deliberately tight — a thread that worked >2ms before sleeping is not a
// pure pacing idle. Donghu witness: 16.738ms wakeup cadence vs 15.758ms
// segment → 0.980ms gap. Never borrow this constant for any other semantic
// (容差常量禁跨语义借用, §29.26 lesson).
const binderPacingFramePeriodToleranceMs = 2.0

// binderPacingMinFramePeriodMs / binderPacingMaxFramePeriodMs bound the
// PLAUSIBLE frame-period band for arm c: 4ms (240Hz) .. 50ms (20Hz). A
// waker cadence outside the band is not a display refresh cadence and never
// reroutes a segment to the pacing lane (donghu focused-replay witness: a
// 2.058ms RenderThread wake cadence briefly recast a 1.805ms sleep as
// "frame-pacing idle" — no display runs at ~486Hz). Single-meaning
// constants; never borrowed (容差常量禁跨语义借用).
const (
	binderPacingMinFramePeriodMs = 4.0
	binderPacingMaxFramePeriodMs = 50.0
)

// binderPacingPeriodSourceAggregate / binderPacingPeriodSourceCadence are the
// typed provenance tokens of the arm-c frame period.
const (
	binderPacingPeriodSourceAggregate = "waker_periodic_aggregate"
	binderPacingPeriodSourceCadence   = "waker_wakeup_cadence"
)

// binderIdleKindPacing / binderIdleKindPeriodic are the arm-c lane tokens
// (复核 P2-1, 2026-07-12): the frame form and the generic periodic form.
// Both are registered causal tokens demoting to RootCauseTierContextOnly.
const (
	binderIdleKindPacing   = "pacing_idle"
	binderIdleKindPeriodic = "periodic_idle"
)

// binderAttributionAudit carries the per-chain indexes the three P9 arms
// read. Built with ONE pass over idx.Events, restricted to exactly the
// identities the chain's sleep nodes need (reply destinations, terminating
// waker↔sleeper wakeup pairs, frame-schedule span emitters).
type binderAttributionAudit struct {
	// replyTsByDest: reply-flagged binder_transaction timestamps keyed by the
	// exact physical source plus dest_thread pid (the original sender the reply
	// returns to), sorted. Source scoping prevents one bundle child from
	// completing a wait in another child that reused the same TID.
	replyTsByDest map[binderReplyDestination][]float64
	// wakeupTsByPair: sched_wakeup timestamps keyed by (waker pid, wakee
	// pid), sorted — the arm-c cadence source.
	wakeupTsByPair map[[2]int][]float64
	// frameScheduleEmitters: pids that emitted a trace-mark span/instant
	// whose verbatim name classifies as frame_schedule (vsync /
	// choreographer / doframe) — the arm-c vsync-dispatch-chain membership
	// signal beside the VS-1 periodic aggregate.
	frameScheduleEmitters map[int]bool
	// wakeEdgeByNode: the segment-ending wakeup edge per chain-node index
	// (the LATEST in-window wakeup of the node's thread — the edge that ends
	// the sleep segment). Absent entry = no in-window wakeup edge.
	wakeEdgeByNode map[int]WakeupEdge
}

type binderReplyDestination struct {
	source string
	tid    int
}

// buildBinderAttributionAudit assembles the P9 indexes for one chain.
func buildBinderAttributionAudit(idx *Index, chain ChainResult) *binderAttributionAudit {
	audit := &binderAttributionAudit{
		replyTsByDest:         map[binderReplyDestination][]float64{},
		wakeupTsByPair:        map[[2]int][]float64{},
		frameScheduleEmitters: map[int]bool{},
		wakeEdgeByNode:        map[int]WakeupEdge{},
	}
	sleeperPIDs := map[int]bool{}
	wakerPIDs := map[int]bool{}
	pairs := map[[2]int]bool{}
	for i, node := range chain.Nodes {
		if node.Thread.PID == 0 {
			continue
		}
		if node.Dominant != StateSSleep && node.Dominant != StateDSleep && node.Dominant != StateIOWait {
			continue
		}
		sleeperPIDs[node.Thread.PID] = true
		if edge, ok := chainTerminatingWakeEdge(chain, node); ok {
			audit.wakeEdgeByNode[i] = edge
			if edge.Waker.PID > 0 {
				wakerPIDs[edge.Waker.PID] = true
				pairs[[2]int{edge.Waker.PID, node.Thread.PID}] = true
			}
		}
	}
	if len(sleeperPIDs) == 0 || idx == nil {
		return audit
	}
	for _, ev := range idx.Events {
		switch ev.Type {
		case EventBinderTransaction:
			bf := ev.BinderFields
			if bf == nil || !bf.binderTransactionKnown() || !bf.binderReplyKnown() || bf.Reply != 1 || !bf.binderDestinationKnown() || bf.DestThread <= 0 || !sleeperPIDs[bf.DestThread] {
				continue
			}
			source, sourceOK := tracePairingSourceIdentity(idx, ev)
			if !sourceOK {
				continue
			}
			key := binderReplyDestination{source: source, tid: bf.DestThread}
			audit.replyTsByDest[key] = append(audit.replyTsByDest[key], ev.Ts)
		case EventSchedWakeup:
			if ev.WakeePID <= 0 || ev.PID <= 0 {
				continue
			}
			key := [2]int{ev.PID, ev.WakeePID}
			if !pairs[key] {
				continue
			}
			audit.wakeupTsByPair[key] = append(audit.wakeupTsByPair[key], ev.Ts)
		case EventTraceMark:
			if ev.PID <= 0 || !wakerPIDs[ev.PID] || audit.frameScheduleEmitters[ev.PID] {
				continue
			}
			if ev.SpanAction != "B" && ev.SpanAction != "I" {
				continue
			}
			if strings.TrimSpace(ev.SpanName) == "" {
				continue
			}
			if classifyFramePhase(ev.SpanName) == "frame_schedule" {
				audit.frameScheduleEmitters[ev.PID] = true
			}
		}
	}
	for dest := range audit.replyTsByDest {
		sort.Float64s(audit.replyTsByDest[dest])
	}
	for key := range audit.wakeupTsByPair {
		sort.Float64s(audit.wakeupTsByPair[key])
	}
	return audit
}

// chainCausalImpactLinesForNode returns the evidence line span of the causal
// impact record measuring EXACTLY this node's segment (same thread, same
// branch window — float equality on the shared interval). ENG-2 追修
// (复核冷读 CP1-③ 第三折叠机, 2026-07-12): the idle-cadence rows mint with
// the SAME evidence coordinates as the segment's impact record, so the
// display compile's same-fact fold (subject + %.3f value + line span)
// engages BY CONSTRUCTION and the idle annotation rides the surviving seat
// through every downstream fold/cap — the pre-fix rows carried the raw
// sleep/wakeup line pair while the impact record published its evidence
// span, so the fold never fired and the standalone idle rows silently fell
// off the capped supporting-hop bucket.
func chainCausalImpactLinesForNode(chain ChainResult, node ChainNode) (int, int, bool) {
	for i := range chain.CausalImpacts {
		imp := &chain.CausalImpacts[i]
		if !sameThreadRef(imp.Thread, node.Thread) {
			continue
		}
		if imp.Window.StartTs != node.Window.StartTs || imp.Window.EndTs != node.Window.EndTs {
			continue
		}
		if imp.LineStart <= 0 || imp.LineEnd < imp.LineStart {
			continue
		}
		return imp.LineStart, imp.LineEnd, true
	}
	return 0, 0, false
}

// chainTerminatingWakeEdge returns the wakeup edge that ENDS the node's
// sleep segment: the latest in-window wakeup of the node's thread.
func chainTerminatingWakeEdge(chain ChainResult, node ChainNode) (WakeupEdge, bool) {
	best := WakeupEdge{}
	found := false
	for _, edge := range chain.Edges {
		if edge.Wakee.PID != node.Thread.PID {
			continue
		}
		if edge.WakeupTs < node.Window.StartTs || edge.WakeupTs > node.Window.EndTs {
			continue
		}
		if !found || edge.WakeupTs > best.WakeupTs {
			best = edge
			found = true
		}
	}
	return best, found
}

// firstReplyAtOrAfter returns the first reply completion directed at destPID
// at or after sendTs. A synchronous binder client blocks until its reply, so
// the first reply returning to the sender after a send IS that send's
// completion (stack discipline; nested server-side transactions preserve the
// same ordering on the client thread).
func (a *binderAttributionAudit) firstReplyAtOrAfter(source string, destPID int, sendTs float64) (float64, bool) {
	if a == nil || strings.TrimSpace(source) == "" || destPID <= 0 {
		return 0, false
	}
	replies := a.replyTsByDest[binderReplyDestination{source: source, tid: destPID}]
	i := sort.SearchFloat64s(replies, sendTs)
	if i >= len(replies) {
		return 0, false
	}
	return replies[i], true
}

// replyCompletedBeforeSegment is the arm-a verdict: T's reply completion
// exists and lies strictly before the segment start.
func (a *binderAttributionAudit) replyCompletedBeforeSegment(source string, destPID int, sendTs, segStartTs float64) bool {
	ts, ok := a.firstReplyAtOrAfter(source, destPID, sendTs)
	return ok && ts < segStartTs
}

// replyInsideSegment reports whether T's reply completion falls inside the
// segment — the direct positive witness of a genuine synchronous wait.
func (a *binderAttributionAudit) replyInsideSegment(source string, destPID int, sendTs, segStartTs, segEndTs float64) bool {
	ts, ok := a.firstReplyAtOrAfter(source, destPID, sendTs)
	return ok && ts >= segStartTs && ts <= segEndTs
}

// pacingVerdict is the arm-c check for one written-off sleep node. The
// segment reads as idle cadence when a typed period source exists, the
// period sits in the plausible band and the segment length matches it; the
// returned kind forks on the waker's frame-chain membership (复核 P2-1,
// 2026-07-12): frameScheduleEmitters ⇒ binderIdleKindPacing (帧间空闲 frame
// promise words), otherwise binderIdleKindPeriodic (generic 周期空闲 —
// aggregate-only period evidence; the frame words never leak to timer/audio
// style periodic wakers).
func (a *binderAttributionAudit) pacingVerdict(chain ChainResult, node ChainNode, wake WakeupEdge) (periodMs float64, source, kind string, ok bool) {
	if a == nil || wake.Waker.PID <= 0 || node.DurationMs <= 0 {
		return 0, "", "", false
	}
	frameChain := a.frameScheduleEmitters[wake.Waker.PID]
	// Period source 1 — the waker's typed VS-1 periodic-source aggregate
	// (admissible for BOTH lanes; the only admissible evidence for the
	// generic periodic lane).
	for _, agg := range chain.AggregatedImpacts {
		if agg.PeriodicSource && agg.DetectedPeriodMs > 0 && sameThreadRef(agg.Thread, wake.Waker) {
			periodMs, source = agg.DetectedPeriodMs, binderPacingPeriodSourceAggregate
			break
		}
	}
	// Period source 2 — frame-chain wakers only: the waker provably emits
	// frame-schedule spans (vsync / choreographer / doframe verbatim names)
	// and its consecutive wakeups of this thread bracket the segment — the
	// tick interval IS the frame period. A bare two-wakeup cadence is NOT
	// admissible periodicity evidence for a non-frame waker.
	if periodMs <= 0 && frameChain {
		ticks := a.wakeupTsByPair[[2]int{wake.Waker.PID, node.Thread.PID}]
		i := sort.SearchFloat64s(ticks, wake.WakeupTs)
		// Find the waker's latest wakeup of this thread strictly before the
		// segment-ending one.
		for i > 0 && ticks[i-1] >= wake.WakeupTs {
			i--
		}
		if i > 0 {
			if prev := ticks[i-1]; prev > 0 && prev < wake.WakeupTs {
				periodMs, source = (wake.WakeupTs-prev)*1000, binderPacingPeriodSourceCadence
			}
		}
	}
	if periodMs < binderPacingMinFramePeriodMs || periodMs > binderPacingMaxFramePeriodMs {
		// Zero, or outside the plausible display-refresh band (240Hz..20Hz):
		// not an idle cadence — the segment stays on its scheduler-state lane.
		return 0, "", "", false
	}
	diff := node.DurationMs - periodMs
	if diff < 0 {
		diff = -diff
	}
	if diff >= binderPacingFramePeriodToleranceMs {
		return 0, "", "", false
	}
	kind = binderIdleKindPeriodic
	if frameChain {
		kind = binderIdleKindPacing
	}
	return periodMs, source, kind, true
}

// renderPacingIdleSummary is the idle-cadence row's user-facing summary —
// wording forks on the typed Kind (复核 P2-1: frame promise words only for
// frame-chain wakers; generic periodic wakers get the periodic wording).
func renderPacingIdleSummary(p PacingIdleSummary) string {
	var summary string
	if p.Kind == binderIdleKindPeriodic {
		summary = fmt.Sprintf("%s spent %.3fms in periodic idle (waiting for the next periodic signal): the segment length matches the waker's %.3fms signal period and the segment-ending waker %s is a measured periodic signal source",
			threadLabel(p.Thread), p.DurationMs, p.FramePeriodMs, threadLabel(p.Waker))
	} else {
		summary = fmt.Sprintf("%s spent %.3fms in frame-pacing idle (waiting for the next frame): the segment length matches one %.3fms frame period and the segment-ending waker %s is on the frame-signal dispatch chain",
			threadLabel(p.Thread), p.DurationMs, p.FramePeriodMs, threadLabel(p.Waker))
	}
	if len(p.RejectedTransactionIDs) > 0 {
		ids := make([]string, 0, len(p.RejectedTransactionIDs))
		for _, id := range p.RejectedTransactionIDs {
			ids = append(ids, fmt.Sprintf("%d", id))
		}
		summary += fmt.Sprintf("; earlier synchronous binder transaction(s) %s had already completed before this segment started and do not explain it", strings.Join(ids, ", "))
	}
	summary += " — not a peer block; excluded from root-cause ranking"
	return summary
}

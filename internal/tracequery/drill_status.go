package tracequery

import (
	"fmt"
	"strings"
)

// drill_status.go — RCX① engine side (ledger §12.3 ruling 1, P0-E1): the
// typed drill-debt verdict for cross-thread blocking counterparts.
//
// DrillStatus classifies whether a row's contention/IPC counterpart was
// itself EXAMINED in this report's observation universe — i.e. whether any
// observation exists whose SUBJECT is that counterpart. Merely being NAMED as
// a cause/peer on someone else's row (peer= note, peer_state_* enrichment, a
// blocking_span rank row fronting the holder) is NOT examination: the
// universe below is built exclusively from rows that decompose a thread's own
// conduct. Counterpart lanes: binder_wait peers, io_latency completers
// (P3-5), and resolved lock-contention holders (blocking_span rows with a
// parsed BlockingKind). Rows without a counterpart lane carry no verdict
// (empty field, no claim).
//
// The verdicts are consumed by the frame-root-cause-bundle head disclosure
// ("largest impact undrilled", internal/tool/trace_query.go) and published as
// the drill_status observation note (NKR display tier). The
// projection/answer-face half (树头强制披露行 + 行2 限定词 + 明细对端行) is
// NOT wired today — it is known_gap OM-7 with host batch IC-A (§29.40,
// UXG-1 假注释勘正 2026-07-12; the former "deferred to the P0-A batch" note
// was an unbacked promissory pointer — P0-A shipped without it).
const (
	// DrillStatusDrilled — the counterpart is resolved AND a subject==peer
	// observation exists in this report.
	DrillStatusDrilled = "drilled"
	// DrillStatusUndrilledPeerKnown — the counterpart is resolved but was
	// never examined by any subject==peer observation (the q4 lock-holder
	// burial shape: the 112.223ms holder named on E1 yet never queried).
	DrillStatusUndrilledPeerKnown = "undrilled_peer_known"
	// DrillStatusPeerUnknown — the counterpart could not be resolved at all.
	DrillStatusPeerUnknown = "peer_unknown"
)

// Counterpart-source enum (P0-E2a, §10 A2 / §11 N8 / §12 Q4-C; LCK-2 §18.E).
// A resolved blocking counterpart (lock holder / binder peer) came from ONE of
// three deterministic origins; the typed source lets every downstream face say
// WHICH so a payload-direct resolution and a fallback-inferred one are never
// conflated. System-produced enum, never model prose.
const (
	// CounterpartSourceContentionPayload — the counterpart tid was parsed
	// straight out of the structured contention/binder payload AND that tid is
	// present in this trace (drillable). The unchanged, information-richest
	// origin.
	CounterpartSourceContentionPayload = "contention_payload"
	// CounterpartSourceNsSpanDerivation — LCK-2 rung ② (§18.E): the payload
	// tid is a container-namespace id this trace cannot resolve directly, and
	// the trace's own trace_mark emission pairs (SpanPID ↔ host row header)
	// mapped it to a host THREAD (②a self-reported ns-tid sample or ②b
	// main-thread special case; structural uniqueness enforced, comm never
	// consulted). Confidence 0.67 — between payload-direct (0.72) and the
	// wakeup-edge inference (0.62); 0.70 when the ②×③ identity unification
	// cross-corroborates.
	CounterpartSourceNsSpanDerivation = "ns_span_derivation"
	// CounterpartSourceWakeupEdge — the payload tid named no thread this trace
	// scheduled (cross-namespace phantom, or absent) and rung ② had no mapping
	// material, so the counterpart was recovered from the direct 1-hop wakeup
	// edge (the waker of the waiter). Lower confidence than a payload-direct
	// resolution.
	CounterpartSourceWakeupEdge = "wakeup_edge"
)

// counterpartSourceIsInferred reports whether a counterpart-source value names
// an INFERENCE lane (wakeup edge / ns-span derivation) as opposed to the
// payload-direct origin — the set-membership form of the old single-value
// `== CounterpartSourceWakeupEdge` comparison (LCK-2: a ns-span-derived holder
// is also an inference, so presumption inheritance and disclosure surfaces
// must treat it as one).
func counterpartSourceIsInferred(source string) bool {
	switch source {
	case CounterpartSourceWakeupEdge, CounterpartSourceNsSpanDerivation:
		return true
	default:
		return false
	}
}

// drillSubjectUniverse is the subject set of this report's own-conduct
// observations, keyed both by PID and by comm so a counterpart resolved as
// tid-only or name-only still matches precisely (exact keys, no substring
// heuristics).
type drillSubjectUniverse struct {
	pids  map[int]bool
	comms map[string]bool
}

func (u drillSubjectUniverse) add(t ThreadRef) {
	if t.PID > 0 {
		u.pids[t.PID] = true
	}
	if comm := strings.TrimSpace(t.Comm); comm != "" {
		u.comms[comm] = true
	}
}

// contains matches by PID when the counterpart carries one, otherwise by
// exact comm.
func (u drillSubjectUniverse) contains(t ThreadRef) bool {
	if t.PID > 0 {
		return u.pids[t.PID]
	}
	comm := strings.TrimSpace(t.Comm)
	return comm != "" && u.comms[comm]
}

// buildDrillSubjectUniverse collects every thread the report decomposes with
// its own subject==thread observation: chain nodes / causal impacts /
// aggregates / root evidence / target, plus EVERY window-stats surface that
// publishes an own-conduct rank/blocking row with a thread subject (state
// tops, running top, state churn, trace spans, IO-latency issuers,
// file/page-cache/block-IO summaries, IO bursts, runnable context, CPU
// constraints, workqueue / dma-fence / sched-stat accounting). P3-4: a
// counterpart whose only appearance is one of these resource surfaces is
// still EXAMINED — an under-counting universe would flip its rows to a false
// undrilled_peer_known.
func buildDrillSubjectUniverse(chain *ChainResult, stats *WindowStats) drillSubjectUniverse {
	u := drillSubjectUniverse{pids: map[int]bool{}, comms: map[string]bool{}}
	if chain != nil {
		u.add(chain.Target)
		for _, node := range chain.Nodes {
			u.add(node.Thread)
		}
		for _, impact := range chain.CausalImpacts {
			u.add(impact.Thread)
		}
		for _, aggregate := range chain.AggregatedImpacts {
			u.add(aggregate.Thread)
		}
		for _, root := range chain.RootEvidence {
			u.add(root.Thread)
		}
	}
	if stats != nil {
		for _, td := range stats.TopRunning {
			u.add(td.Thread)
		}
		for _, td := range stats.RunnableTop {
			u.add(td.Thread)
		}
		for _, td := range stats.SleepTop {
			u.add(td.Thread)
		}
		for _, td := range stats.DStateTop {
			u.add(td.Thread)
		}
		for _, td := range stats.IOWaitTop {
			u.add(td.Thread)
		}
		for _, churn := range stats.StateChurn {
			u.add(churn.Thread)
		}
		for _, span := range stats.TraceSpans {
			u.add(span.Thread)
		}
		allowedIOIssuers := map[int]bool{}
		if chain != nil {
			allowedIOIssuers = wakeupChainThreadSet(*chain)
		}
		for _, io := range ioLatencyCausalAccounting(*stats, allowedIOIssuers) {
			// The issuer's wait is the row's own conduct; the completer is
			// that row's COUNTERPART lane (P3-5), never auto-drilled here.
			u.add(io.IssueThread)
		}
		for _, file := range stats.FileIOByInode {
			u.add(file.Thread)
		}
		for _, cache := range stats.PageCacheByInode {
			u.add(cache.Thread)
		}
		for _, episode := range stats.IOBurstEpisodes {
			u.add(episode.Thread)
		}
		for _, inode := range stats.BlockIOByInode {
			u.add(inode.Thread)
		}
		for _, ctx := range stats.RunnableContext {
			u.add(ctx.Thread)
		}
		// NIAUTH-04: a constraint that minted a root seat from the full
		// accounting census must also count as drilled here even when it sits
		// beyond the public CPUConstraints display cap.
		for _, constraint := range cpuConstraintAccounting(*stats) {
			u.add(constraint.Thread)
		}
		for _, work := range stats.WorkqueueActivity {
			u.add(work.Thread)
		}
		for _, fence := range stats.DMAFenceActivity {
			u.add(fence.Thread)
		}
		for _, accounting := range stats.SchedStatAccounting {
			u.add(accounting.Thread)
		}
	}
	return u
}

// drillStatusForCounterpart is the single verdict function: precise typed
// inputs only (resolution of the counterpart ThreadRef + universe membership).
func drillStatusForCounterpart(peer ThreadRef, universe drillSubjectUniverse) string {
	if !threadRefResolved(peer) {
		return DrillStatusPeerUnknown
	}
	if universe.contains(peer) {
		return DrillStatusDrilled
	}
	return DrillStatusUndrilledPeerKnown
}

// drillStatusForCounterpartWithTimeline (P0-E2b, E2a correction ①) is the full
// production verdict: universe membership AND actual drillable timeline. A
// wakeup-edge counterpart can enter the observation universe merely by being
// NAMED (a chain node / waker with zero decomposition); if that tid has no
// sched_switch material, "drilled" would claim an examination that structurally
// cannot have decomposed anything — demote to undrilled_peer_known. Comm-only
// peers keep the pure verdict (their name already resolved through the trace's
// own thread catalog; there is no pid to check).
func drillStatusForCounterpartWithTimeline(idx *Index, peer ThreadRef, universe drillSubjectUniverse) string {
	verdict := drillStatusForCounterpart(peer, universe)
	if verdict == DrillStatusDrilled && peer.PID > 0 && !idx.tidHasScheduledTimeline(peer.PID) {
		return DrillStatusUndrilledPeerKnown
	}
	return verdict
}

// criticalBlockingDrillCounterpart returns the counterpart whose drill status
// a critical-blocking row carries, and whether the row HAS a counterpart lane:
// binder peers, IO-latency completers (P3-5; the builder publishes
// io.CompleteThread as the row Peer), and resolved-contention lock holders.
func criticalBlockingDrillCounterpart(item CriticalBlockingCandidate) (ThreadRef, bool) {
	switch item.Type {
	case "binder_wait", "io_latency":
		return item.Peer, true
	case "blocking_span":
		if item.BlockingKind != "" {
			return item.Peer, true
		}
	}
	return ThreadRef{}, false
}

// stampCriticalBlockingDrillStatus applies the RCX① verdict to every
// counterpart-lane row of a critical_blocking result (timeline-aware, E2a ①).
func stampCriticalBlockingDrillStatus(idx *Index, items []CriticalBlockingCandidate, universe drillSubjectUniverse) {
	for i := range items {
		peer, ok := criticalBlockingDrillCounterpart(items[i])
		if !ok {
			continue
		}
		items[i].DrillStatus = drillStatusForCounterpartWithTimeline(idx, peer, universe)
	}
}

// stampRootCauseRankDrillStatus applies the RCX① verdict to every
// counterpart-lane rank row (timeline-aware, E2a ①):
//   - blocking_span (Q4-A lock lane): the examined party is the HOLDER — the
//     row subject when resolved (BlockingPeer set), unknown otherwise;
//   - binder_wait / io_latency (§20 E-Gap⑥, 2026-07-07 — symmetry with the
//     critical_blocking face, criticalBlockingDrillCounterpart): the binder
//     peer / IO completer is the counterpart. Rank rows do not carry those
//     peers as typed fields, so the stamp re-joins the row to its backing
//     chain.BinderWaits / full stats IO-latency census record through the EXACT
//     construction-time values (thread PID + line span) — a deterministic
//     replay of the producer join, never a fuzzy match; rows without a
//     backing record keep the empty no-claim verdict.
func stampRootCauseRankDrillStatus(idx *Index, items []RootCauseRankItem, universe drillSubjectUniverse, chain *ChainResult, stats *WindowStats) {
	binderPeers := rankBinderWaitCounterparts(chain)
	ioCompleters := rankIOLatencyCounterparts(stats)
	for i := range items {
		switch items[i].Type {
		case "blocking_span":
			if items[i].BlockingKind == "" {
				continue
			}
			if !threadRefResolved(items[i].BlockingPeer) {
				// Unresolved holder: the subject stayed the waiter.
				items[i].DrillStatus = DrillStatusPeerUnknown
				continue
			}
			items[i].DrillStatus = drillStatusForCounterpartWithTimeline(idx, items[i].Thread, universe)
		case "binder_wait":
			if peer, ok := binderPeers[rankDrillJoinKey(items[i].Thread, items[i].LineStart, items[i].LineEnd)]; ok {
				items[i].DrillStatus = drillStatusForCounterpartWithTimeline(idx, peer, universe)
			}
		case "io_latency":
			if peer, ok := ioCompleters[rankDrillJoinKey(items[i].Thread, items[i].LineStart, items[i].LineEnd)]; ok {
				items[i].DrillStatus = drillStatusForCounterpartWithTimeline(idx, peer, universe)
			}
		}
	}
}

// rankDrillJoinKey is the deterministic construction-time identity of a
// counterpart-lane rank row: subject PID + the exact line span the producer
// stamped on the row. Exact keys only — no substring/window heuristics.
func rankDrillJoinKey(thread ThreadRef, lineStart, lineEnd int) string {
	return fmt.Sprintf("%d/%d/%d", thread.PID, lineStart, lineEnd)
}

// rankBinderWaitCounterparts indexes chain.BinderWaits by the SAME key shape
// the RootEvidence funnel used when it minted the binder_wait rank row
// (attachIPCGraphToChain: LineStart=firstPositive(SendLine, SleepLine),
// LineEnd=firstPositive(WakeupLine, ReceiveLine, SleepLine)). First record
// wins on a key collision — the funnel minted rows in the same order.
func rankBinderWaitCounterparts(chain *ChainResult) map[string]ThreadRef {
	if chain == nil || len(chain.BinderWaits) == 0 {
		return nil
	}
	out := map[string]ThreadRef{}
	for _, wait := range chain.BinderWaits {
		key := rankDrillJoinKey(wait.Thread, firstPositive(wait.SendLine, wait.SleepLine), firstPositive(wait.WakeupLine, wait.ReceiveLine, wait.SleepLine))
		if _, exists := out[key]; !exists {
			out[key] = wait.Peer
		}
	}
	return out
}

// rankIOLatencyCounterparts indexes the full IO-latency census by the SAME key shape
// the rank builder used (LineStart=IssueLine, LineEnd=CompleteLine); the
// counterpart is the completer (P3-5 — mirror of the critical_blocking face,
// which publishes io.CompleteThread as the row Peer).
func rankIOLatencyCounterparts(stats *WindowStats) map[string]ThreadRef {
	if stats == nil {
		return nil
	}
	latencies := stats.ioLatencyCensus
	if latencies == nil {
		latencies = stats.IOLatencies
	}
	if len(latencies) == 0 {
		return nil
	}
	out := map[string]ThreadRef{}
	for _, io := range latencies {
		key := rankDrillJoinKey(io.IssueThread, io.IssueLine, io.CompleteLine)
		if _, exists := out[key]; !exists {
			out[key] = io.CompleteThread
		}
	}
	return out
}

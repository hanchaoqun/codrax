package tracequery

// ns_span_derivation.go — LCK-2: rung ② of the lock-holder resolution ladder
// (ledger docs/design/real_trace_campaign_20260705.md §18.E + §18.E 增补 +
// §18.E.1, user rulings 2026-07-07) plus the ②×③ identity-unification
// declaration.
//
// A structured contention payload can print an owner tid drawn from a
// container pid namespace (`Lock contention on … (owner tid: 62020)` emitted
// as `B|60194|…` by host row com.baidu.tieba-59566 (59566): 62020 and 60194
// are container-ns ids, 59566 is the host tgid). Rung ① (payload direct,
// tid-presence) cannot resolve such an id; before falling to rung ③ (closing
// wakeup edge, 0.62) the trace itself often carries enough material to map
// the ns identity to a host identity:
//
//	Every trace_mark row is an EMISSION PAIR — the payload pid (Event.SpanPID,
//	the emitter's pid-namespace process id) rides next to the row header's
//	host tid/tgid. A whole-trace scan builds the ns→host maps; the hard-gate
//	discipline is STRUCTURAL UNIQUENESS (§18.E: 精确信号,不涉 comm) — a second
//	distinct host id for the same ns key marks the entry Ambiguous and rung ②
//	refuses it (falls to ③); comm NEVER disambiguates.
//
// Rung ② output forms (§18.E 增补 — NOT process-capped, thread level by shape):
//
//	②a self-reported ns-tid — a span whose TEXT carries a standalone `tid:`/
//	   `tid=` field is the emitting host thread reporting its own container
//	   tid: (ns-pid, ns-tid) → host tid, THREAD level. Payloads that parse as
//	   lock-contention are excluded wholesale (their tid fields name the
//	   OWNER — another thread), as is any `<word> tid:` compound key
//	   ("owner tid:", "to tid:") — a preceding word binds the field to a
//	   subject other than the emitter (fail-closed).
//	②b main-thread special case — pid-ns threads are 1:1, so owner
//	   ns-tid == ns-tgid ⇒ the owner is that process's main thread ⇒ host
//	   tid == host tgid. THREAD level, zero extra mapping material needed.
//	②c ns-tid → host tid inside the derived process — needs a thread-level
//	   mapping source, which today is exactly the ②a sample table; with no
//	   sample the result degrades to PROCESS level (host tgid only) with an
//	   EXPLICIT process-level disclosure, and rung ③ supplements the thread.
//
// ②×③ fusion (§18.E.1): the closing waker is by construction a host-level
// thread. When rung ② (thread level) and rung ③ independently name the SAME
// host tid, an identity-unification declaration is published (typed value,
// both lanes listed) — the "owner and holder are the same physical thread"
// prose claim becomes a system fact. A comm mismatch between the payload
// owner comm and the derived host comm NEVER vetoes the derivation (user
// ruling: comm 可动态改名, soft disclosure only). When ② could only reach
// PROCESS level, a closing waker INSIDE the derived process is the two-lane
// corroboration form: the waker (thread level, from ③) is published as the
// peer with the ns-span process identity riding as a display note (§19: the
// host tgid is NEVER stuffed into Peer.PID).
//
// Rung ① `(owner tid: N)` parsing is untouched (user pin: 匹配模式合理保留,
// 不泛化); rung ② fires only after rung ① failed, and only for a contention
// span whose emission is actually ns-divergent (SpanPID ≠ waiter host TGID —
// a precise two-integer gate; identity-namespace traces keep the pre-LCK-2
// rung-③ behavior byte-for-byte).

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// counterpartNsSpanDerivationConfidence is the visible confidence for a
// counterpart resolved via rung ② ns-span derivation (§19 LCK-2 pin): a typed
// whole-trace mapping is stronger than the 1-hop wakeup inference (0.62) but
// still weaker than a payload owner id directly present in the trace (0.72).
// Also the ceiling for a rung-③ waker corroborated at PROCESS level by rung ②
// (two agreeing lanes lift the bare wakeup inference to ns-span grade).
const counterpartNsSpanDerivationConfidence = 0.67

// counterpartNsSpanFusionConfidence is the ②×③ identity-unification ceiling
// (§18.E.1 置信升档): two INDEPENDENT lanes (ns-span mapping × closing wakeup)
// naming the same host THREAD outrank either single lane, but a cross-checked
// inference still never reaches payload-direct grade (0.72). Strict ordering:
// 0.72 direct > 0.70 unified > 0.67 ns-span > 0.62 wakeup edge.
const counterpartNsSpanFusionConfidence = 0.70

// Typed via-path labels for the thread-level rung-② forms (§18.E 增补).
const (
	nsSpanViaSelfReportedTid = "self_reported_ns_tid"
	nsSpanViaMainThread      = "ns_main_thread"
)

// nsSpanThreadKey identifies one container thread: the emitter's ns process
// id (SpanPID) plus the self-reported ns tid.
type nsSpanThreadKey struct {
	NsPID int
	NsTID int
}

// nsSpanProcessMapping is one ns-pid → host-tgid emission-pair entry.
// Ambiguous is set on the SECOND distinct host tgid ever observed for the
// same SpanPID — rung ② then refuses the whole ns-pid (hard reject, §19 pin:
// 第二 tgid→Ambiguous 硬拒落③, comm 不消歧).
type nsSpanProcessMapping struct {
	HostTGID int
	// HostMainComm is the host MAIN-thread comm (row tid == row tgid) observed
	// on this process's own emissions — display enrichment for the ②b
	// main-thread form only, never a matching key.
	HostMainComm string
	Ambiguous    bool
	Samples      int
}

// nsSpanThreadMapping is one (ns-pid, ns-tid) → host-tid self-report entry
// (②a). Ambiguous is set on the second distinct host tid claiming the same
// container identity (same hard-reject discipline as the process map).
type nsSpanThreadMapping struct {
	HostTID  int
	HostTGID int
	// HostComm is the LATEST row-header comm observed on the self-reporting
	// emissions — soft display material only (comm can be renamed at runtime).
	HostComm  string
	Ambiguous bool
	Samples   int
}

// nsSpanDerivation is the per-Index memo of both maps. Built lazily exactly
// once (sync.Once publishes an immutable value; Index is pointer-only
// throughout the package, same discipline as tidPresenceSet).
type nsSpanDerivation struct {
	process map[int]*nsSpanProcessMapping
	thread  map[nsSpanThreadKey]*nsSpanThreadMapping
}

func (idx *Index) nsSpanDerivedMaps() *nsSpanDerivation {
	if idx == nil {
		return nil
	}
	idx.nsSpanOnce.Do(func() {
		idx.nsSpanMaps = buildNsSpanDerivation(idx.Events)
	})
	return idx.nsSpanMaps
}

// buildNsSpanDerivation scans every trace_mark emission pair once. All inputs
// are typed parse products (SpanAction/SpanPID from parseTraceMark, row
// header PID/TGID/Comm) — never free substring guesses; the only text-derived
// input is the ②a standalone-tid field extraction, which is fail-closed (see
// nsSpanSelfReportedTid).
func buildNsSpanDerivation(events []Event) *nsSpanDerivation {
	d := &nsSpanDerivation{
		process: map[int]*nsSpanProcessMapping{},
		thread:  map[nsSpanThreadKey]*nsSpanThreadMapping{},
	}
	for i := range events {
		ev := &events[i]
		if ev.Type != EventTraceMark {
			continue
		}
		spanPID := ev.SpanPID
		switch ev.SpanAction {
		case "B", "E":
			// Closed action set: async S/F payload subjects do not establish a
			// process mapping.
		case "C":
			sample := parseTraceCounterSample(*ev)
			if !sample.identityOK || !sample.numericValid {
				continue
			}
			spanPID = sample.ownerPID
		default:
			continue
		}
		if spanPID <= 0 {
			continue
		}
		// Process map: every B/E/C emission pair with a known host tgid votes;
		// a second DISTINCT tgid poisons the entry (structural uniqueness).
		if ev.TGID > 0 {
			pm := d.process[spanPID]
			if pm == nil {
				pm = &nsSpanProcessMapping{HostTGID: ev.TGID}
				d.process[spanPID] = pm
			}
			if pm.HostTGID != ev.TGID {
				pm.Ambiguous = true
			}
			pm.Samples++
			if ev.PID == ev.TGID {
				if comm := strings.TrimSpace(ev.Comm); comm != "" && comm != "<...>" {
					pm.HostMainComm = comm
				}
			}
		}
		// ②a thread map: only named B rows from an actually ns-divergent
		// emitter (SpanPID ≠ host tgid) can self-report a container tid; an
		// identity-namespace self-report is inert (rung ① already resolves
		// those owners) and is not harvested.
		if ev.SpanAction != "B" || ev.PID <= 0 || ev.TGID <= 0 || spanPID == ev.TGID {
			continue
		}
		nsTid, ok := nsSpanSelfReportedTid(ev.SpanName)
		if !ok {
			continue
		}
		key := nsSpanThreadKey{NsPID: spanPID, NsTID: nsTid}
		tm := d.thread[key]
		if tm == nil {
			tm = &nsSpanThreadMapping{HostTID: ev.PID, HostTGID: ev.TGID}
			d.thread[key] = tm
		}
		if tm.HostTID != ev.PID {
			tm.Ambiguous = true
		} else if comm := strings.TrimSpace(ev.Comm); comm != "" && comm != "<...>" {
			tm.HostComm = comm
		}
		tm.Samples++
	}
	return d
}

// nsSpanSelfReportedTidRe matches a lowercase `tid:`/`tid=` key with an
// optional single space and a pid-plausible digit run (≤8 digits — Linux
// pid_max caps at 2^22; longer runs are data, same guard as the carved-mark
// restorer).
var nsSpanSelfReportedTidRe = regexp.MustCompile(`tid[:=] ?([0-9]{1,8})`)

// nsSpanSelfReportedTid extracts the emitter's SELF-reported container tid
// from a span name, fail-closed on every ambiguity:
//
//   - a payload that parses as a structured lock-contention print is excluded
//     wholesale — its tid fields (`owner tid: N`, the trailing `(tid)` of the
//     monitor form) name the OWNER, i.e. another thread;
//   - the `tid` key must be STANDALONE: walking back over spaces, the
//     preceding character must be the start of the name or a field delimiter
//     (| , ; : ( [ {). A preceding WORD ("owner tid:", "to tid:", "vtid:")
//     binds the field to some other subject and is rejected;
//   - the digit run must terminate the field (no trailing [A-Za-z0-9_]);
//   - two standalone fields claiming DIFFERENT values in one span name is a
//     self-contradiction → no extraction.
func nsSpanSelfReportedTid(name string) (int, bool) {
	if strings.TrimSpace(name) == "" {
		return 0, false
	}
	if _, isContention := parseLockContentionPayload(name); isContention {
		return 0, false
	}
	matches := nsSpanSelfReportedTidRe.FindAllStringSubmatchIndex(name, -1)
	value, found := 0, false
	for _, m := range matches {
		j := m[0] - 1
		for j >= 0 && name[j] == ' ' {
			j--
		}
		if j >= 0 {
			switch name[j] {
			case '|', ',', ';', ':', '(', '[', '{':
			default:
				continue
			}
		}
		if end := m[3]; end < len(name) {
			c := name[end]
			if c == '_' || ('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') {
				continue
			}
		}
		v, err := strconv.Atoi(name[m[2]:m[3]])
		if err != nil || v <= 0 {
			continue
		}
		if found && v != value {
			return 0, false
		}
		value, found = v, true
	}
	return value, found
}

// nsSpanOwnerResolution is the typed rung-② verdict for one payload owner tid.
type nsSpanOwnerResolution struct {
	// Host is the derived host THREAD (thread-level forms only).
	Host ThreadRef
	// HostTGID is the derived host process — always set when OK.
	HostTGID int
	// HostProcessComm is the derived process's main-thread comm when observed
	// (display enrichment only).
	HostProcessComm string
	// NsPID is the container ns process id the derivation keyed on.
	NsPID int
	// ThreadLevel is true for the ②a/②b forms; false = PROCESS level only
	// (②c thread mapping unavailable — the caller must disclose the downgrade
	// explicitly and keep the host tgid OUT of Peer.PID).
	ThreadLevel bool
	// Via names the thread-level path (nsSpanViaSelfReportedTid /
	// nsSpanViaMainThread); empty on the process-level form.
	Via string
	OK  bool
}

// resolveOwnerViaNsSpan runs rung ② for one contention row. Precise gates, in
// order:
//
//   - ns divergence: the contention span's own emission pair must be
//     cross-namespace (SpanPID > 0 ∧ waiter host TGID > 0 ∧ SpanPID ≠ TGID).
//     Identity-namespace and TGID-less traces return not-OK so the pre-LCK-2
//     rung-③ path stays byte-identical for them;
//   - structural uniqueness: an Ambiguous process or thread entry is refused
//     outright (comm never disambiguates — §18.E pin);
//   - ②a: (NsPID, owner ns-tid) self-report sample → host thread;
//   - ②b: owner ns-tid == NsPID → the owner is the container main thread ⇒
//     host tid == host tgid, provided that tid is actually present in this
//     trace;
//   - otherwise PROCESS level (host tgid only).
func resolveOwnerViaNsSpan(idx *Index, spanNsPID, waiterHostTGID, ownerNsTid int) nsSpanOwnerResolution {
	if idx == nil || spanNsPID <= 0 || ownerNsTid <= 0 {
		return nsSpanOwnerResolution{}
	}
	if waiterHostTGID <= 0 || spanNsPID == waiterHostTGID {
		return nsSpanOwnerResolution{}
	}
	maps := idx.nsSpanDerivedMaps()
	if maps == nil {
		return nsSpanOwnerResolution{}
	}
	pm := maps.process[spanNsPID]
	if pm == nil || pm.Ambiguous || pm.HostTGID <= 0 {
		return nsSpanOwnerResolution{}
	}
	// ②a — self-reported ns-tid sample (thread level).
	if tm := maps.thread[nsSpanThreadKey{NsPID: spanNsPID, NsTID: ownerNsTid}]; tm != nil && !tm.Ambiguous {
		return nsSpanOwnerResolution{
			Host:            ThreadRef{Comm: tm.HostComm, PID: tm.HostTID, TGID: firstPositive(tm.HostTGID, pm.HostTGID)},
			HostTGID:        firstPositive(tm.HostTGID, pm.HostTGID),
			HostProcessComm: pm.HostMainComm,
			NsPID:           spanNsPID,
			ThreadLevel:     true,
			Via:             nsSpanViaSelfReportedTid,
			OK:              true,
		}
	}
	// ②b — container main thread (ns-tid == ns-tgid ⇒ host tid == host tgid).
	if ownerNsTid == spanNsPID && idx.tidPresent(pm.HostTGID) {
		return nsSpanOwnerResolution{
			Host:            ThreadRef{Comm: pm.HostMainComm, PID: pm.HostTGID, TGID: pm.HostTGID},
			HostTGID:        pm.HostTGID,
			HostProcessComm: pm.HostMainComm,
			NsPID:           spanNsPID,
			ThreadLevel:     true,
			Via:             nsSpanViaMainThread,
			OK:              true,
		}
	}
	// ②c thread mapping unavailable — PROCESS level.
	return nsSpanOwnerResolution{
		HostTGID:        pm.HostTGID,
		HostProcessComm: pm.HostMainComm,
		NsPID:           spanNsPID,
		OK:              true,
	}
}

// applyNsSpanOwnerResolution stamps a successful rung-② verdict onto the
// folded blocking row and runs the ②×③ cross-check against the closing
// wakeup edge (§18.E 增补 / §18.E.1):
//
//	thread level: Peer = derived host thread, HolderSource=ns_span_derivation,
//	  confidence 0.67. Closing waker == derived thread ⇒ typed
//	  identity-unification declaration + fusion confidence 0.70; a different
//	  waker never overwrites the derivation — divergence is disclosed
//	  (possible intermediary wakeup).
//	process level: the host tgid NEVER enters Peer.PID (§19 typed-pair pin) —
//	  it rides the HolderHostProcess display note with an EXPLICIT
//	  process-level downgrade disclosure. A closing waker INSIDE the derived
//	  process is the ②×③ corroboration form (Peer=waker via wakeup_edge,
//	  confidence lifted to ns-span grade 0.67); outside (or unknown tgid) the
//	  plain rung-③ fallback fires at 0.62 with both identities disclosed.
//
// comm NEVER vetoes: a payload owner comm that differs from the derived host
// comm gets a soft "comm may have been renamed" disclosure and nothing else
// (user ruling 2026-07-07; the mismatch is EXPECTED under prctl renames).
func applyNsSpanOwnerResolution(idx *Index, cand *CriticalBlockingCandidate, ns nsSpanOwnerResolution, rawTid int) {
	payloadComm := strings.TrimSpace(cand.Peer.Comm)
	fb := resolveCounterpartViaWakeupEdge(idx, cand.Thread, cand.StartTs, cand.EndTs)
	cand.OwnerTidRaw = rawTid
	if ns.ThreadLevel {
		cand.Peer = ns.Host
		cand.HolderSource = CounterpartSourceNsSpanDerivation
		cand.Summary = appendRootCauseSummaryDetail(cand.Summary, nsSpanThreadLevelCaveat(rawTid, ns))
		if note, mismatch := nsSpanCommSoftDisclosure(payloadComm, ns.Host.Comm); mismatch {
			cand.Summary = appendRootCauseSummaryDetail(cand.Summary, note)
		}
		ceiling := counterpartNsSpanDerivationConfidence
		switch {
		case fb.OK && fb.Waker.PID == ns.Host.PID:
			// §18.E.1 identity unification: two independent lanes name the
			// same host thread.
			cand.HolderNsUnification = nsSpanUnificationValue(rawTid, ns.Host)
			cand.Summary = appendRootCauseSummaryDetail(cand.Summary, nsSpanUnificationCaveat(rawTid, ns.Host))
			if note, mismatch := nsSpanCommSoftDisclosure(payloadComm, fb.Waker.Comm); mismatch {
				cand.Summary = appendRootCauseSummaryDetail(cand.Summary, note)
			}
			ceiling = counterpartNsSpanFusionConfidence
		case fb.OK && fb.Waker.TGID > 0 && fb.Waker.TGID == ns.HostTGID:
			cand.Summary = appendRootCauseSummaryDetail(cand.Summary, nsSpanIntraProcessDivergenceCaveat(fb.Waker, ns.Host))
		case fb.OK:
			cand.Summary = appendRootCauseSummaryDetail(cand.Summary, nsSpanCrossProcessDivergenceCaveat(fb.Waker, ns.Host))
		}
		cand.Confidence = nsSpanCappedConfidence(cand.Confidence, ceiling)
		return
	}
	// PROCESS level.
	cand.HolderHostProcess = nsSpanHostProcessValue(ns)
	cand.Summary = appendRootCauseSummaryDetail(cand.Summary, nsSpanProcessLevelCaveat(rawTid, ns))
	if fb.OK {
		cand.Peer = fb.Waker
		cand.HolderSource = CounterpartSourceWakeupEdge
		if fb.Waker.TGID > 0 && fb.Waker.TGID == ns.HostTGID {
			// ②×③ corroboration: the closing waker lies inside the derived
			// host process — the two lanes agree, the waker is very likely the
			// holder thread itself (§18.E 增补).
			cand.Summary = appendRootCauseSummaryDetail(cand.Summary, nsSpanProcessCorroborationCaveat(fb.Waker, ns))
			if note, mismatch := nsSpanCommSoftDisclosure(payloadComm, fb.Waker.Comm); mismatch {
				cand.Summary = appendRootCauseSummaryDetail(cand.Summary, note)
			}
			cand.Confidence = nsSpanCappedConfidence(cand.Confidence, counterpartNsSpanDerivationConfidence)
			return
		}
		cand.Summary = appendRootCauseSummaryDetail(cand.Summary, counterpartWakeupEdgeCaveat(rawTid))
		if fb.Waker.TGID > 0 {
			cand.Summary = appendRootCauseSummaryDetail(cand.Summary, nsSpanProcessWakerOutsideCaveat(fb.Waker, ns))
		}
		cand.Confidence = nsSpanCappedConfidence(cand.Confidence, counterpartWakeupEdgeConfidence)
		return
	}
	// No usable wakeup edge: the peer stays UNRESOLVED (typed-pair gates read
	// false); the process-level identity survives on the display note only.
	cand.Peer = ThreadRef{}
}

// nsSpanCappedConfidence lowers a candidate's confidence to the given
// inference ceiling, never RAISING an already-lower value — same monotone
// no-gate demotion channel as counterpartDemotedConfidence.
func nsSpanCappedConfidence(current, ceiling float64) float64 {
	if current > 0 && current < ceiling {
		return current
	}
	return ceiling
}

// nsSpanCommSoftDisclosure builds the comm-mismatch SOFT disclosure. It never
// gates anything: comm is runtime-renameable (prctl), so a payload comm
// recorded at contention time may legitimately differ from the comm the
// scheduler face shows for the SAME physical thread — the mismatch is
// expected and disclosed, never a veto (user ruling 2026-07-07, §18.E.1).
func nsSpanCommSoftDisclosure(payloadComm, derivedComm string) (string, bool) {
	payloadComm = strings.TrimSpace(payloadComm)
	derivedComm = strings.TrimSpace(derivedComm)
	if payloadComm == "" || derivedComm == "" || payloadComm == derivedComm {
		return "", false
	}
	return fmt.Sprintf("payload owner comm %q differs from the derived host thread comm %q — comm can be renamed at runtime, so a name mismatch is expected and does NOT veto the derivation (soft disclosure only)", payloadComm, derivedComm), true
}

// nsSpanThreadLevelCaveat disclosures a thread-level rung-② resolution.
func nsSpanThreadLevelCaveat(rawTid int, ns nsSpanOwnerResolution) string {
	return fmt.Sprintf("holder derived thread-level via ns-span emission pairs (%s): payload owner tid %d is a container-namespace id (ns pid %d) not present in this trace; trace_mark pairs map it to host thread %s — a derivation, not a payload-confirmed holder", ns.Via, rawTid, ns.NsPID, threadLabel(ns.Host))
}

// nsSpanProcessLevelCaveat is the EXPLICIT process-level downgrade disclosure
// (§18.E 增补 ②c: 对不上 → 进程级 + 显式披露).
func nsSpanProcessLevelCaveat(rawTid int, ns nsSpanOwnerResolution) string {
	label := fmt.Sprintf("tgid=%d", ns.HostTGID)
	if ns.HostProcessComm != "" {
		label = fmt.Sprintf("%s-%d", ns.HostProcessComm, ns.HostTGID)
	}
	return fmt.Sprintf("holder narrowed PROCESS-LEVEL only via ns-span emission pairs: payload owner tid %d is a container-namespace id (ns pid %d) mapping to host process %s, but no thread-level mapping material exists in this trace — the specific holder thread stays underived at this rung", rawTid, ns.NsPID, label)
}

// nsSpanUnificationValue is the typed identity-unification declaration value
// (§18.E.1): the unified host thread plus the two independent lanes that
// derived it. System-produced, never model prose.
func nsSpanUnificationValue(rawTid int, host ThreadRef) string {
	return fmt.Sprintf("owner_ns_tid=%d host=%s lanes=%s+%s", rawTid, threadLabel(host), CounterpartSourceNsSpanDerivation, CounterpartSourceWakeupEdge)
}

// nsSpanUnificationCaveat is the prose face of the unification declaration.
func nsSpanUnificationCaveat(rawTid int, host ThreadRef) string {
	return fmt.Sprintf("identity unification: ns-span derivation and the closing wakeup edge INDEPENDENTLY name the same host thread %s for payload owner ns-tid %d — owner and releasing holder are one physical thread (two-lane cross-corroboration)", threadLabel(host), rawTid)
}

// nsSpanIntraProcessDivergenceCaveat: rung ③'s waker is a DIFFERENT thread of
// the same derived host process — possible intermediary wakeup; the ns-span
// thread-level identity is kept.
func nsSpanIntraProcessDivergenceCaveat(waker, host ThreadRef) string {
	return fmt.Sprintf("closing waker %s is a different thread of the same derived host process as holder %s — possible intermediary wakeup; holder identity kept from the ns-span derivation", threadLabel(waker), threadLabel(host))
}

// nsSpanCrossProcessDivergenceCaveat: rung ③'s waker does not even share the
// derived host process (or its tgid is unknown) — both identities disclosed,
// the derivation stands.
func nsSpanCrossProcessDivergenceCaveat(waker, host ThreadRef) string {
	return fmt.Sprintf("closing waker %s does not match the ns-span-derived holder %s (possible intermediary wakeup); the two signals diverge and are disclosed separately — holder identity kept from the ns-span derivation", threadLabel(waker), threadLabel(host))
}

// nsSpanProcessCorroborationCaveat: the ②×③ process-membership corroboration
// (waker ∈ derived process ⇒ 两级互证).
func nsSpanProcessCorroborationCaveat(waker ThreadRef, ns nsSpanOwnerResolution) string {
	return fmt.Sprintf("closing waker %s belongs to the ns-span-derived host process (tgid=%d) — the wakeup edge and the ns-span process derivation corroborate each other; the waker is very likely the holder thread itself", threadLabel(waker), ns.HostTGID)
}

// nsSpanProcessWakerOutsideCaveat: waker known to lie OUTSIDE the derived
// process — dual disclosure, no corroboration.
func nsSpanProcessWakerOutsideCaveat(waker ThreadRef, ns nsSpanOwnerResolution) string {
	return fmt.Sprintf("closing waker %s lies OUTSIDE the ns-span-derived host process (tgid=%d) — possible intermediary wakeup; the process-level identity and the waker are disclosed separately", threadLabel(waker), ns.HostTGID)
}

// nsSpanHostProcessValue is the typed process-level identity display value
// (§19: never a Peer.PID — display note only).
func nsSpanHostProcessValue(ns nsSpanOwnerResolution) string {
	v := fmt.Sprintf("tgid=%d ns_pid=%d level=process", ns.HostTGID, ns.NsPID)
	if ns.HostProcessComm != "" {
		v += " comm=" + ns.HostProcessComm
	}
	return v
}

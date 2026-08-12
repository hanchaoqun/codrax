package orchestrator

import (
	"fmt"
	"math/bits"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// prose_fact_juxtaposition.go — CR-4 修复轮方向改造 (用户裁定 2026-07-12):
// 系统事实对照 (fact juxtaposition).
//
// RULING: the system must NOT characterize the model's natural language by
// keyword-matching it (the accusatory verdict wordings are all retired —
// the CR-4 six arms' claim extraction with them; the banned wording grep
// lives in the pins). Instead:
//
//	presence trigger — a thread name / CPU id / numeric token APPEARS in
//	  the model prose (pure token presence, zero semantic understanding);
//	fact listing — the appendix lists that entity's system TYPED facts
//	  (「<线程>:typed 席位=… · typed 内核调用点=… · 窗内五态=…」), and the
//	  READER juxtaposes them against the prose. Over-selection is harmless:
//	  listing more true facts is never wrong.
//
// The ONLY verdict wording that remains is pure arithmetic (the false-
// equation arm below: 「文中等式 A+B=C:实际和为 D」 — mathematics, not NL
// understanding).
//
// Airtightness (零输出影响): this lane feeds ONLY the system cross-check
// appendix (collectSystemCrossCheckFindings). It never mints a violation,
// never enters a retry surface, never injects a caveat into the answer
// body. 全批软纪律 (§29.42.4/§29.47.1).
//
// Witness coverage by juxtaposition (previously "caught" by the retired
// accusatory arms, now covered by fact lines the reader compares):
//   - F-CR3-3: prose mentions CompThread + fscache_page_get_an → fact line
//     carries the thread's typed 内核调用点=dma_fence_default_w(12条记录);
//   - F-CR3-1: prose mentions ZeusThreadPo-61841 → fact line carries typed
//     锁角色=等待侧(锁主 tid=62020);
//   - F-CR3-2: prose mentions CPU0 + a frequency token → fact line carries
//     「CPU0:窗内 typed 频点=无观测记录」;
//   - SMR-1 P0: prose mentions the target thread → fact line carries the
//     full four-state account (running 157.248ms …);
//   - ghost seats (56249/91951/73346): prose mentions app-9511 /
//     DetectViewRect → fact lines carry 「typed 席位=无」 vs the seated
//     rows' 「typed 席位=#N(有效归因 …)」.
const (
	proseFactThreadCap = 6
	proseFactCPUCap    = 3
	// proseFactWakeupTopologyCap bounds the checkout-independent typed
	// wakeup placement rows published without consulting model prose. These
	// rows are especially important for separating cross-CPU dependency from
	// same-CPU competition, but large fan-out chains must not crowd every
	// other fact out of the appendix.
	proseFactWakeupTopologyCap = 3
	// proseFactPartitionCap bounds the FACT-REL arm-a four-state partition
	// facts (§29.55.4 F2, 2026-07-13).
	proseFactPartitionCap = 3
	proseFactEquationCap  = 4
	// proseFactEquationBaseTol: honest last-digit rounding of each printed
	// operand never trips the arithmetic check (per-numeral half-ulps sum,
	// floored here). Own named constant (容差常量禁跨语义借用).
	proseFactEquationBaseTolMS = 0.002
	// proseFactImplicitSubCap bounds the PROSE-RC-4 臂② implicit nested-
	// re-subtraction fact lines per report (one per prose unit at most).
	proseFactImplicitSubCap = 2
	// proseFactImplicitSubTermCap bounds the distinct proven-share operands
	// considered per prose unit (subset enumeration stays ≤ 2^6−1 = 63).
	proseFactImplicitSubTermCap = 6
	// proseFactImplicitSubBaseTolMS: honest last-digit rounding floor for
	// the implicit-subtraction identity |X−ΣY−Z|. Own named constant
	// (容差常量禁跨语义借用 — same value as the equation arm's is a
	// coincidence of discipline, never a shared constant).
	proseFactImplicitSubBaseTolMS = 0.002
)

// proseFactCPURE / proseFactFreqValueRE / proseFactFreqKVRE are token-level
// presence detectors (never claim parsers). The k=v form (C-3, tieba miss:
// 「CPU0(freq=1090000 kHz)」/「freq=1090000」) joins the unit form; a bare
// k=v value defaults to kHz.
var proseFactCPURE = regexp.MustCompile(`(?i)\bcpu\s*[-＝= ]?\s*([0-9]{1,2})\b`)
var proseFactFreqTokenRE = regexp.MustCompile(`(?i)[0-9]\s*(?:GHz|MHz|kHz)\b|freq(?:uency)?\s*[=＝:：]\s*[0-9]|频率|频点|限频|降频`)

// proseFactEquationRE captures an A+B(+…)=C numeric equation (strict = only
// — ≈/约 forms never match, 宁松勿严).
var proseFactEquationRE = regexp.MustCompile(`((?:[0-9]+(?:\.[0-9]+)?\s*(?:ms|毫秒)?\s*\+\s*)+[0-9]+(?:\.[0-9]+)?)\s*(?:ms|毫秒)?\s*[=＝]\s*([0-9]+(?:\.[0-9]+)?)`)

// proseFactWakeupDegradedRE reads the engine's typed degradation caveat off
// the tool-result banner (wakeup_target_cpu_degraded=true total=N).
var proseFactWakeupDegradedRE = regexp.MustCompile(`wakeup_target_cpu_degraded=true\s+total=([0-9]+)`)

// proseFactAnyNumeralRE harvests every numeral (integer or decimal) off an
// evidence surface for the implicit-subtraction arm's negative published-
// value gate. Over-inclusion is the SAFE direction here: a richer published
// set can only suppress findings, never mint one.
var proseFactAnyNumeralRE = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

// proseFactDecimalTokenRE finds prose-side decimal numeric tokens — the
// implicit-subtraction operands. Integer prose tokens (counts, line numbers,
// tids) stay out of this lane by design.
var proseFactDecimalTokenRE = regexp.MustCompile(`[0-9]+\.[0-9]+`)

// proseFactCensusShareRE pulls the per-caller Σms share magnitudes out of
// the engine's typed blocked_reason_census note ("sym×N(ΣXms)/…" — the same
// typed note format the wait-evidence summary consumes; format pinned by the
// census tests). Each Σ value is a published caller-named proven-share
// magnitude.
var proseFactCensusShareRE = regexp.MustCompile(`×[0-9]+\(Σ([0-9.]+)ms\)`)

// proseFactThreadFacts is one evidence-face entity's typed fact bundle.
type proseFactThreadFacts struct {
	subject string // evidence-face spelling
	tid     string

	seats       []proseFactSeat
	boardExists bool

	callers     []string
	callerCount int // blocked_reason window count when published (0 = unknown)

	lockWaiter bool
	lockHolder bool
	ownerTIDs  []string
	lockRows   int

	tgid string

	account *proseWallClockAccount

	confidences []float64
}

type proseFactSeat struct {
	rank        int
	effectiveMS float64
	hasEff      bool
	memberCount int
	// causeSymbol / unprovenRemainder (FACT-REL arm b, §29.55.4 R2-F1
	// claim-of-absence, 2026-07-13): the seat's typed cause word and the
	// §29.50.5 cause-unproven remainder marker ride the roster chip, so the
	// seat list itself is the counter-face a coverage/absence sentence can
	// be juxtaposed against (tieba witness: prose closed with 「未证实的原因
	// 不存在」 while the report seated a 10.433ms 原因未证 remainder).
	causeSymbol       string
	unprovenRemainder bool
}

func (f proseFactThreadFacts) richness() int {
	score := 0
	if f.account != nil {
		score += 3
	}
	if len(f.seats) > 0 {
		score += 2
	}
	if len(f.callers) > 0 {
		score += 2
	}
	if f.lockWaiter || f.lockHolder {
		score += 2
	}
	if f.tgid != "" {
		score++
	}
	return score
}

// proseFactJuxtapositionFindings is the appendix provider: typed fact lines
// for prose-present entities + arithmetic verdicts on prose equations.
func proseFactJuxtapositionFindings(doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState) []proseScalarBindingFinding {
	out := proseTypedFactJuxtapositionFindingsImpl(doc, bus, mut)
	// Offline diagnostic extension only. Production does not reach this
	// function (pinned by the ownership call-graph test), so prose-derived
	// arithmetic interpretations cannot escape through a false boolean branch.
	prose := collectModelProseUnits(doc)
	seen := map[string]bool{}
	for _, finding := range out {
		seen[finding.entryZH] = true
	}
	add := func(f proseScalarBindingFinding) {
		if f.entryZH == "" || seen[f.entryZH] {
			return
		}
		seen[f.entryZH] = true
		out = append(out, f)
	}
	for _, f := range proseFactEquationFindings(prose) {
		add(f)
	}
	if bus != nil && mut != nil {
		ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
		for _, f := range proseFactImplicitSubtractionFindings(prose, buildProseFactSubtractionInventory(ledger, bus, mut)) {
			add(f)
		}
	}
	return out
}

func proseFactWakeupCPUTopologyFindings(ledger types.ObservationLedger) []proseScalarBindingFinding {
	seen := map[string]bool{}
	out := make([]proseScalarBindingFinding, 0, proseFactWakeupTopologyCap)
	for _, record := range ledger.Records {
		if len(out) >= proseFactWakeupTopologyCap {
			break
		}
		if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			strings.TrimSpace(record.Predicate) != "wakeup_chain_edge" ||
			strings.TrimSpace(record.Subject) == "" || strings.TrimSpace(record.Object) == "" {
			continue
		}
		wakerCPU, wakerOK := proseFactNoteInt(record.RichNotes, types.TraceNoteKeyWakeupWakerCPU)
		wakeeCPU, wakeeOK := proseFactNoteInt(record.RichNotes, types.TraceNoteKeyWakeupWakeeTargetCPU)
		relation := strings.TrimSpace(proseWallClockNoteValue(record.RichNotes, types.TraceNoteKeyWakeupCPURelation))
		if !wakerOK || !wakeeOK || (relation != "same_cpu" && relation != "cross_cpu") {
			continue
		}
		waker := strings.TrimSpace(record.Subject)
		wakee := strings.TrimSpace(record.Object)
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%s", waker, wakee, wakerCPU, wakeeCPU, relation)
		if seen[key] {
			continue
		}
		seen[key] = true
		if relation == "cross_cpu" {
			out = append(out, proseScalarBindingFinding{
				entry: fmt.Sprintf("typed fact: wakeup topology %s -> %s — the wakeup event ran on CPU%d and targeted CPU%d (cross-CPU); this edge is not evidence of same-CPU occupancy, preemption, or direct competition",
					waker, wakee, wakerCPU, wakeeCPU),
				entryZH: fmt.Sprintf("typed 事实:唤醒拓扑 %s → %s — 唤醒事件发生在 CPU%d,目标投递 CPU%d(跨核);该边不构成同核占用、抢占或直接竞争证据",
					waker, wakee, wakerCPU, wakeeCPU),
			})
			continue
		}
		out = append(out, proseScalarBindingFinding{
			entry: fmt.Sprintf("typed fact: wakeup topology %s -> %s — the wakeup event ran on CPU%d and targeted CPU%d (same CPU); placement alone is not direct-competition evidence without a separate compatible running/runnable overlap",
				waker, wakee, wakerCPU, wakeeCPU),
			entryZH: fmt.Sprintf("typed 事实:唤醒拓扑 %s → %s — 唤醒事件发生在 CPU%d,目标投递 CPU%d(同核);仅凭放置相同仍不能证明直接竞争,还需独立的 running/runnable 重叠证据",
				waker, wakee, wakerCPU, wakeeCPU),
		})
	}
	return out
}

// proseTypedFactJuxtapositionFindings is the production appendix provider.
// It may use exact entity/token presence only to select rows, then renders
// values directly from the typed observation ledger. It never interprets a
// model-authored equation, subtraction, subject binding, cause, or omission.
// The broader provider above remains available to offline tests/audits but is
// not an answer-shipping authority.
func proseTypedFactJuxtapositionFindings(doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState) []proseScalarBindingFinding {
	return proseTypedFactJuxtapositionFindingsImpl(doc, bus, mut)
}

// proseTypedFactJuxtapositionFindingsImpl is the production-safe provider.
// It contains no prose-verdict branch and no call to an offline conclusion
// scanner; that structural separation is what makes wrapper reachability
// auditable.
func proseTypedFactJuxtapositionFindingsImpl(doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState) []proseScalarBindingFinding {
	if doc == nil || bus == nil || mut == nil {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return nil
	}
	statePrincipalAllowed := true
	if bus.AnalysisIR != nil {
		statePrincipalAllowed = types.RuntimeTraceTargetStateMaterializationAllowed(
			&bus.AnalysisIR.RequestModel,
			types.CompileTraceCausalProjectionSet(ledger),
		)
	}
	facts, freqByCPU, freqSeen := buildProseFactEvidence(ledger)
	prose := collectModelProseUnits(doc)

	var out []proseScalarBindingFinding
	seen := map[string]bool{}
	add := func(f proseScalarBindingFinding) {
		if f.entryZH == "" || seen[f.entryZH] {
			return
		}
		seen[f.entryZH] = true
		out = append(out, f)
	}

	// ── exact wakeup CPU topology (typed, prose-independent) ─────────────
	// A cross-CPU wakeup remains an on-chain dependency but is not same-CPU
	// occupancy/preemption/competition. Publish the exact placement fact in
	// the neutral appendix whether or not the model happened to repeat either
	// CPU token. This is an information carrier only: it never interprets,
	// rejects, or rewrites the model's conclusion.
	for _, finding := range proseFactWakeupCPUTopologyFindings(ledger) {
		add(finding)
	}

	// ── engine typed degradation fact (target_cpu 退化) ──────────────────
	if total, ok := proseFactWakeupDegradation(bus, mut); ok && proseFactMentionsCPU(prose) {
		add(proseScalarBindingFinding{
			entry: fmt.Sprintf("typed fact: this trace's in-window sched_wakeup target_cpu field is suspected degraded (%d events, all zero) — per-CPU accounting keyed on target_cpu is unreliable", total),
			entryZH: fmt.Sprintf("typed 事实:本 trace 窗内 sched_wakeup 的 target_cpu 字段疑退化(%d 条全 0),按 target_cpu 归账的 per-CPU 口径不可靠",
				total),
		})
	}

	// ── FACT-REL arm a (§29.55.4 F2, 2026-07-13): same-unit multi-state
	// presence → the thread's typed state account with the mutual-exclusion
	// partition fact (pure presence trigger + typed listing; the reader
	// juxtaposes — no relation reading of the prose).
	if statePrincipalAllowed {
		for _, f := range proseFactStatePartitionFindings(prose, facts) {
			add(f)
		}
	}

	// ── per-thread fact lines (presence-triggered) ───────────────────────
	if statePrincipalAllowed {
		present := proseFactPresentThreads(prose, facts)
		sort.SliceStable(present, func(i, j int) bool {
			if facts[present[i]].richness() != facts[present[j]].richness() {
				return facts[present[i]].richness() > facts[present[j]].richness()
			}
			return present[i] < present[j]
		})
		for i, tid := range present {
			if i >= proseFactThreadCap {
				break
			}
			if zh, en := proseFactThreadLine(facts[tid]); zh != "" {
				add(proseScalarBindingFinding{entry: en, entryZH: zh})
			}
		}
	}

	// ── per-CPU frequency facts (cpu token + any frequency token) ───────
	if len(freqSeen) > 0 {
		for _, cpuID := range proseFactPresentCPUs(prose) {
			points := freqByCPU[cpuID]
			if len(points) == 0 {
				add(proseScalarBindingFinding{
					entry:   fmt.Sprintf("typed fact: CPU%d carries no in-window frequency observation on this report's evidence face", cpuID),
					entryZH: fmt.Sprintf("typed 事实:CPU%d 在本报告证据面无窗内频率观测记录", cpuID),
				})
				continue
			}
			add(proseScalarBindingFinding{
				entry:   fmt.Sprintf("typed fact: CPU%d in-window typed frequency point(s) = %s", cpuID, proseFactFreqListLabel(points)),
				entryZH: fmt.Sprintf("typed 事实:CPU%d 窗内 typed 频点=%s", cpuID, proseFactFreqListLabel(points)),
			})
		}
	}
	return out
}

// buildProseFactEvidence assembles per-thread typed fact bundles plus the
// per-CPU frequency inventory from the observation ledger.
//
// F-A11 design note (落档): the frequency inventory admits every freq= note
// the accepted ledger carries; per-note WINDOW filtering is not possible
// today because the notes carry no timestamps — the records themselves are
// window-scoped by the run's queries, so the inventory is "this run's
// evidence face", not "all time". A per-note ts would need an engine-side
// emission change (future candidate).
func buildProseFactEvidence(ledger types.ObservationLedger) (map[string]*proseFactThreadFacts, map[int][]float64, map[int]bool) {
	facts := map[string]*proseFactThreadFacts{}
	freqByCPU := map[int][]float64{}
	freqSeen := map[int]bool{}
	get := func(subject string) *proseFactThreadFacts {
		tid := proseWallClockSubjectTID(subject)
		if tid == "" {
			return nil
		}
		if f, ok := facts[tid]; ok {
			if f.subject == "" {
				f.subject = subject
			}
			return f
		}
		f := &proseFactThreadFacts{subject: subject, tid: tid}
		facts[tid] = f
		return f
	}
	boardExists := false
	for _, record := range ledger.Records {
		if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
			continue
		}
		subject := strings.TrimSpace(record.Subject)
		notes := record.RichNotes
		f := get(subject)
		if f == nil {
			continue
		}
		// seats + effective + member count (typed board rows).
		if strings.Contains(record.ID, "#root_cause_rank:") {
			rank := 0
			if n, ok := proseFactNoteInt(notes, types.TraceNoteKeyRank); ok {
				rank = n
			}
			if rank > 0 {
				boardExists = true
				seat := proseFactSeat{rank: rank}
				// RANKDIS-M18 (§29.104.17 裁定② 2026-07-16, 复核件2 勘正):
				// composite-score rows publish the *_score twin instead of
				// the ms key. Under the current closed matrix they are always
				// rank=0 (io_pressure zero-effective → context_only;
				// block_io_by_inode → ⌗ caliber side), so this rank>0 arm is
				// a defensive reservation for a future seating ruling — that
				// ruling must also sync the board template's ms suit
				// (trace_board_summary.go). Zero behavior today.
				if v, ok := proseFactNoteFloat(notes, types.TraceNoteKeyEffectiveImpactMS); ok {
					seat.effectiveMS, seat.hasEff = v, true
				} else if v, ok := proseFactNoteFloat(notes, types.TraceNoteKeyEffectiveImpactScore); ok {
					seat.effectiveMS, seat.hasEff = v, true
				} else if v, err := strconv.ParseFloat(strings.TrimSpace(record.Value), 64); err == nil && v > 0 {
					seat.effectiveMS, seat.hasEff = v, true
				}
				if n, ok := proseFactNoteInt(notes, types.TraceNoteKeyMemberCount); ok && n > 1 {
					seat.memberCount = n
				}
				// FACT-REL arm b (§29.55.4, 2026-07-13): the seat's typed
				// cause word / cause-unproven remainder marker.
				seat.causeSymbol = strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyBlockedReasonCaller))
				seat.unprovenRemainder = strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyDStateCauseUnprovenRemainder)) == "true"
				// Identical republications collapse (the same board row may
				// reach the ledger through several result channels);
				// distinct-value twins (two query windows' boards) stay —
				// both are typed truth.
				dup := false
				for _, have := range f.seats {
					if have == seat {
						dup = true
						break
					}
				}
				// FACT-REL arm b: the cause-unproven remainder seat is the
				// claim-of-absence counter-face — it never falls to the
				// roster capacity cap.
				if !dup && (len(f.seats) < 4 || seat.unprovenRemainder) {
					f.seats = append(f.seats, seat)
				}
			}
			if record.Confidence > 0 {
				f.confidences = append(f.confidences, record.Confidence)
			}
			// 修复轮 件3 (复核 P1-2, 2026-07-13): the former arm-a FALLBACK
			// (rank-note state dims) is retired — rank state notes drift
			// semantically per row kind (per-CPU bucket / churn / impact-
			// segment Σ), so a cross-row per-dim MAX is a chimera wearing a
			// 「typed 事实」 label. The partition fact now renders ONLY from
			// the target_window_states account (宁缺勿假).
		}
		// blocked_reason waiting-object account.
		if v := proseWallClockNoteValue(notes, types.TraceNoteKeyBlockedReasonCaller); v != "" {
			f.callers = proseFactAppendUnique(f.callers, strings.TrimSpace(v))
		}
		if v := proseWallClockNoteValue(notes, types.TraceNoteKeyBlockedReasonWindowCaller); v != "" {
			for _, sym := range strings.Split(v, "/") {
				f.callers = proseFactAppendUnique(f.callers, strings.TrimSpace(sym))
			}
		}
		if n, ok := proseFactNoteInt(notes, types.TraceNoteKeyBlockedReasonWindowCount); ok && n > f.callerCount {
			f.callerCount = n
		}
		if strings.TrimSpace(record.Object) == "blocked_reason" {
			if sym := proseFactTokenAfter(record.Summary, "caller="); sym != "" {
				f.callers = proseFactAppendUnique(f.callers, sym)
			}
		}
		// lock roles.
		kind := strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyBlockingKind))
		if kind == "monitor_contention" || kind == "lock_contention" {
			f.lockRows++
			peer := strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyPeer))
			subjectHolds := proseWallClockNoteValue(notes, types.TraceNoteKeySubjectIsLockHolder) == "true"
			if subjectHolds {
				f.lockHolder = true
				if pf := get(peer); pf != nil {
					pf.lockWaiter = true
				}
			} else {
				f.lockWaiter = true
				if pf := get(peer); pf != nil {
					pf.lockHolder = true
				}
			}
			if raw := strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyOwnerTidRaw)); raw != "" {
				f.ownerTIDs = proseFactAppendUnique(f.ownerTIDs, raw)
			}
		}
		if tgid := strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyTGID)); tgid != "" && f.tgid == "" {
			f.tgid = tgid
		}
		// per-CPU frequency inventory.
		if cpuID, ok := proseFactNoteInt(notes, "cpu"); ok {
			freqSeen[cpuID] = true
			if khz, ok := proseFactNoteFloat(notes, "freq"); ok && khz > 0 {
				freqByCPU[cpuID] = append(freqByCPU[cpuID], khz)
			}
		}
	}
	for i := range facts {
		facts[i].boardExists = boardExists
	}
	// four-state accounts.
	accounts := proseWallClockAccountsFromLedger(ledger)
	for i := range accounts {
		if f := get(accounts[i].subject); f != nil {
			acc := accounts[i]
			f.account = &acc
		}
	}
	return facts, freqByCPU, freqSeen
}

// proseFactNameIndex maps evidence thread NAME parts to their tids (with the
// leading-'.' trimmed twin the prose extractor cannot start a token with:
// .ugc.aweme.lite-17267 → prose "ugc.aweme.lite-17267").
func proseFactNameIndex(facts map[string]*proseFactThreadFacts) map[string]map[string]bool {
	nameToTIDs := map[string]map[string]bool{}
	for tid, f := range facts {
		if dash := strings.LastIndexByte(f.subject, '-'); dash > 0 {
			name := f.subject[:dash]
			if nameToTIDs[name] == nil {
				nameToTIDs[name] = map[string]bool{}
			}
			nameToTIDs[name][tid] = true
			if trimmed := strings.TrimLeft(name, "."); trimmed != name && trimmed != "" {
				if nameToTIDs[trimmed] == nil {
					nameToTIDs[trimmed] = map[string]bool{}
				}
				nameToTIDs[trimmed][tid] = true
			}
		}
	}
	return nameToTIDs
}

// proseFactThreadsInText returns the evidence tids whose spelling (strict
// name-tid token, or a loose single-digit-tail name that resolves to exactly
// ONE evidence thread by name part) appears in ONE text unit. Pure token
// presence — no claim reading.
func proseFactThreadsInText(text string, facts map[string]*proseFactThreadFacts, nameToTIDs map[string]map[string]bool) map[string]bool {
	present := map[string]bool{}
	for _, tref := range extractProseScalarThreadRefs(text) {
		if _, ok := facts[tref.TID]; ok {
			present[tref.TID] = true
		}
	}
	// Loose names (single-digit tails: keva-3) resolve by name part when
	// unambiguous.
	for _, m := range proseScalarLooseThreadRE.FindAllString(text, -1) {
		for _, cand := range append([]string{m}, strings.Split(m, "/")...) {
			dash := strings.LastIndexByte(cand, '-')
			if dash <= 0 || len(cand)-dash-1 >= 2 {
				continue
			}
			if tids := nameToTIDs[cand]; len(tids) == 1 {
				for tid := range tids {
					present[tid] = true
				}
			}
		}
	}
	return present
}

// proseFactPresentThreads returns the tids of evidence-face threads whose
// spelling appears anywhere in the model prose.
func proseFactPresentThreads(prose []proseTextUnit, facts map[string]*proseFactThreadFacts) []string {
	nameToTIDs := proseFactNameIndex(facts)
	present := map[string]bool{}
	for _, unit := range prose {
		for tid := range proseFactThreadsInText(unit.text, facts, nameToTIDs) {
			present[tid] = true
		}
	}
	out := make([]string, 0, len(present))
	for tid := range present {
		out = append(out, tid)
	}
	sort.Strings(out)
	return out
}

// proseFactPresentCPUs returns the CPU ids the prose names, when the prose
// carries any frequency-family token at all (over-listing stays harmless
// but a report that never talks frequencies keeps a quiet appendix).
func proseFactPresentCPUs(prose []proseTextUnit) []int {
	anyFreq := false
	for _, unit := range prose {
		if proseFactFreqTokenRE.MatchString(unit.text) {
			anyFreq = true
			break
		}
	}
	if !anyFreq {
		return nil
	}
	seen := map[int]bool{}
	var out []int
	for _, unit := range prose {
		for _, m := range proseFactCPURE.FindAllStringSubmatch(unit.text, -1) {
			n, err := strconv.Atoi(m[1])
			if err != nil || seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, n)
			if len(out) >= proseFactCPUCap {
				return out
			}
		}
	}
	return out
}

func proseFactMentionsCPU(prose []proseTextUnit) bool {
	for _, unit := range prose {
		if proseFactCPURE.MatchString(unit.text) {
			return true
		}
	}
	return false
}

// proseFactThreadLine renders one entity's typed fact line (zh, en).
func proseFactThreadLine(f *proseFactThreadFacts) (string, string) {
	if f == nil || f.subject == "" {
		return "", ""
	}
	var zh, en []string
	if len(f.seats) > 0 {
		var pz, pe []string
		for _, seat := range f.seats {
			z := fmt.Sprintf("#%d", seat.rank)
			e := fmt.Sprintf("#%d", seat.rank)
			var dz, de []string
			if seat.hasEff {
				dz = append(dz, fmt.Sprintf("有效归因 %.3fms", seat.effectiveMS))
				de = append(de, fmt.Sprintf("effective %.3fms", seat.effectiveMS))
			}
			if seat.memberCount > 1 {
				dz = append(dz, fmt.Sprintf("成员共%d", seat.memberCount))
				de = append(de, fmt.Sprintf("%d members", seat.memberCount))
			}
			// FACT-REL arm b (§29.55.4 R2-F1, 2026-07-13): the seat's typed
			// blocked_reason caller and the honest cause-unproven remainder
			// marker ride the roster chip. The caller is a kernel wait call-site,
			// never a resource object or holder identity.
			if seat.causeSymbol != "" {
				dz = append(dz, "内核调用点="+seat.causeSymbol)
				de = append(de, "kernel wait call-site="+seat.causeSymbol)
			}
			if seat.unprovenRemainder {
				dz = append(dz, "原因未证")
				de = append(de, "cause unproven")
			}
			if len(dz) > 0 {
				z += "(" + strings.Join(dz, ",") + ")"
				e += " (" + strings.Join(de, ", ") + ")"
			}
			pz = append(pz, z)
			pe = append(pe, e)
		}
		zh = append(zh, "typed 席位="+strings.Join(pz, "/"))
		en = append(en, "typed seat(s)="+strings.Join(pe, "/"))
	} else if f.boardExists {
		zh = append(zh, "typed 席位=无")
		en = append(en, "typed seat=none")
	}
	if len(f.callers) > 0 {
		count := ""
		countEN := ""
		if f.callerCount > 0 {
			count = fmt.Sprintf("(%d条记录)", f.callerCount)
			countEN = fmt.Sprintf(" (%d record(s))", f.callerCount)
		}
		zh = append(zh, "typed 内核调用点="+strings.Join(f.callers, "/")+count)
		en = append(en, "typed kernel wait call-site="+strings.Join(f.callers, "/")+countEN)
	}
	if f.lockWaiter || f.lockHolder {
		role, roleEN := "等待侧", "waiter side"
		if f.lockHolder && !f.lockWaiter {
			role, roleEN = "持有侧", "holder side"
		} else if f.lockHolder && f.lockWaiter {
			role, roleEN = "两侧均有记录", "both sides recorded"
		}
		owner, ownerEN := "", ""
		if len(f.ownerTIDs) > 0 {
			owner = fmt.Sprintf("(锁主 tid=%s)", strings.Join(f.ownerTIDs, "/"))
			ownerEN = fmt.Sprintf(" (owner tid=%s)", strings.Join(f.ownerTIDs, "/"))
		}
		zh = append(zh, "typed 锁角色="+role+owner)
		en = append(en, "typed lock role="+roleEN+ownerEN)
	}
	if f.account != nil {
		// 件D (2026-07-13): ONE five-lane wording family — the thread fact
		// line and the partition fact speak the same 五态 form (io_wait
		// listed unconditionally, 0.000 stays honest zero); the old 四态/
		// 五态 twin wordings were a third-wording-family seed.
		zh = append(zh, fmt.Sprintf("窗内五态 running %.3f/runnable %.3f/sleep %.3f/非IO D-state %.3f/io_wait %.3fms",
			f.account.dims[proseWallClockDimRunning], f.account.dims[proseWallClockDimRunnable],
			f.account.dims[proseWallClockDimSleep], f.account.dims[proseWallClockDimDState], f.account.ioWait))
		en = append(en, fmt.Sprintf("in-window five-state running %.3f/runnable %.3f/sleep %.3f/non-IO D-state %.3f/io_wait %.3fms",
			f.account.dims[proseWallClockDimRunning], f.account.dims[proseWallClockDimRunnable],
			f.account.dims[proseWallClockDimSleep], f.account.dims[proseWallClockDimDState], f.account.ioWait))
	}
	if f.tgid != "" {
		zh = append(zh, "tgid="+f.tgid)
		en = append(en, "tgid="+f.tgid)
	}
	if len(f.seats) == 0 && len(f.confidences) > 0 {
		zh = append(zh, fmt.Sprintf("typed confidence=%.2f", f.confidences[0]))
		en = append(en, fmt.Sprintf("typed confidence=%.2f", f.confidences[0]))
	}
	if len(zh) == 0 {
		return "", ""
	}
	return fmt.Sprintf("typed 事实:%s — %s", f.subject, strings.Join(zh, " · ")),
		fmt.Sprintf("typed fact: %s — %s", f.subject, strings.Join(en, " · "))
}

// proseFactEquationFindings — C-2 假等式臂: the only verdict lane, pure
// arithmetic. 「文中等式 A+B=C:实际和为 D」. When the arithmetic HOLDS, the
// operands' provenance stays with the existing membership lanes.
func proseFactEquationFindings(prose []proseTextUnit) []proseScalarBindingFinding {
	var out []proseScalarBindingFinding
	seen := map[string]bool{}
	for _, unit := range prose {
		for _, m := range proseFactEquationRE.FindAllStringSubmatch(unit.text, -1) {
			if len(out) >= proseFactEquationCap {
				return out
			}
			lhs, rhsRaw := m[1], m[2]
			rhs, err := strconv.ParseFloat(rhsRaw, 64)
			if err != nil {
				continue
			}
			sum, tol := 0.0, proseFactEquationBaseTolMS
			var opsRaw []string
			bad := false
			for _, part := range strings.Split(lhs, "+") {
				numeral := strings.TrimSpace(part)
				numeral = strings.TrimSuffix(numeral, "毫秒")
				numeral = strings.TrimSuffix(strings.TrimSpace(numeral), "ms")
				numeral = strings.TrimSpace(numeral)
				v, err := strconv.ParseFloat(numeral, 64)
				if err != nil {
					bad = true
					break
				}
				sum += v
				tol += 0.5 * proseScalarUlp(numeral)
				opsRaw = append(opsRaw, numeral)
			}
			if bad || len(opsRaw) < 2 {
				continue
			}
			tol += 0.5 * proseScalarUlp(rhsRaw)
			diff := sum - rhs
			if diff < 0 {
				diff = -diff
			}
			if diff <= tol {
				continue
			}
			eq := strings.Join(opsRaw, "+") + "=" + rhsRaw
			if seen[eq] {
				continue
			}
			seen[eq] = true
			out = append(out, proseScalarBindingFinding{
				entry:   fmt.Sprintf("equation in the body %s (block %q): the actual sum is %.3f", eq, unit.blockID, sum),
				entryZH: fmt.Sprintf("文中等式 %s(块 %s):实际和为 %.3f", eq, unit.blockID, sum),
			})
		}
	}
	return out
}

// ── PROSE-RC-4 臂② (§29.78, 2026-07-14): implicit nested-re-subtraction ──
//
// The explicit equation arm above needs a literal "=", and the §29.78
// witness carried none: the prose framed the cause-unproven remainder seat
// (10.433) as a TOTAL containing the hmfs caller shares (0.145, 0.171) and
// re-subtracted them to mint 10.117 as the "real" unproven amount. This arm
// detects that three-value co-occurrence with ZERO natural-language reading —
// four precise signals must hold at once:
//
//	X — a prose decimal token VERBATIM-equal to a published cause-unproven
//	    remainder magnitude (typed DStateCauseUnprovenRemainder lane);
//	Y — prose decimal tokens VERBATIM-equal to published caller-named
//	    proven-share magnitudes (typed caller / census Σ lanes);
//	Z — a prose decimal token matching NO published numeral, by exact
//	    string AND by half-ulp value against every numeral any evidence
//	    surface of this run published (ledger values/notes/summaries plus
//	    tool banners — over-inclusive on purpose: over-inclusion only
//	    suppresses);
//	arithmetic — |X − ΣY − Z| ≤ 0.002 + Σ half-ulps (pure math, own
//	    tolerance constant).
//
// The finding wording stays fact-only (the remainder's net-of account
// property + the arithmetic identity + the set-membership fact); it never
// characterizes the prose, and it feeds only the cross-check appendix
// (零输出影响 lane discipline above).

// proseFactSubtractionInventory carries the typed magnitude sets the
// implicit-subtraction arm matches prose tokens against, verbatim.
type proseFactSubtractionInventory struct {
	remainder     map[string]bool // published cause-unproven remainder magnitudes
	proven        map[string]bool // published caller-named proven-share magnitudes
	published     map[string]bool // every numeral string any evidence surface published
	publishedVals []float64       // sorted numeric forms (half-ulp value gate)
}

// buildProseFactSubtractionInventory assembles the typed magnitude sets. The
// positive sets (remainder / proven) admit ONLY deterministic-query typed
// lanes; the negative published set sweeps every evidence surface.
func buildProseFactSubtractionInventory(ledger types.ObservationLedger, bus *types.BusContext, mut *types.MutableState) proseFactSubtractionInventory {
	inv := proseFactSubtractionInventory{
		remainder: map[string]bool{},
		proven:    map[string]bool{},
		published: map[string]bool{},
	}
	addMagnitude := func(set map[string]bool, raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || !strings.Contains(raw, ".") {
			return // prose-side tokens are decimal-only; integers cannot match
		}
		if v, err := strconv.ParseFloat(raw, 64); err != nil || v <= 0 {
			return
		}
		set[raw] = true
	}
	for _, record := range ledger.Records {
		if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
			continue
		}
		notes := record.RichNotes
		unproven := strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyDStateCauseUnprovenRemainder)) == "true"
		caller := strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyBlockedReasonCaller))
		if caller == "unknown" {
			caller = ""
		}
		if unproven || caller != "" {
			set := inv.proven
			if unproven {
				set = inv.remainder
			}
			// The same published-magnitude lanes the wait-evidence summary
			// feeds from (effective impact / duration note / record value).
			addMagnitude(set, proseWallClockNoteValue(notes, types.TraceNoteKeyEffectiveImpactMS))
			addMagnitude(set, proseWallClockNoteValue(notes, "duration"))
			addMagnitude(set, record.Value)
		}
		if raw := proseWallClockNoteValue(notes, types.TraceNoteKeyBlockedReasonCensus); raw != "" {
			for _, m := range proseFactCensusShareRE.FindAllStringSubmatch(raw, -1) {
				addMagnitude(inv.proven, m[1])
			}
		}
	}
	// A magnitude published on BOTH lanes teaches nothing about direction —
	// keep it as the remainder (X) it names and never as a subtrahend.
	for raw := range inv.remainder {
		delete(inv.proven, raw)
	}
	if len(inv.remainder) == 0 || len(inv.proven) == 0 {
		return inv // structurally inert: no scan, no published sweep
	}
	addPublished := func(text string) {
		for _, m := range proseFactAnyNumeralRE.FindAllString(text, -1) {
			if inv.published[m] {
				continue
			}
			inv.published[m] = true
			if v, err := strconv.ParseFloat(m, 64); err == nil {
				inv.publishedVals = append(inv.publishedVals, v)
			}
		}
	}
	for _, record := range ledger.Records {
		addPublished(record.Value)
		addPublished(record.Summary)
		for _, note := range record.RichNotes {
			addPublished(note)
		}
	}
	if bus != nil {
		for _, tr := range bus.ToolResults {
			addPublished(tr.Summary)
		}
	}
	if mut != nil {
		if ta := mut.TurnAArtifacts(); ta != nil {
			for _, tr := range ta.ToolResults {
				addPublished(tr.Summary)
			}
		}
	}
	sort.Float64s(inv.publishedVals)
	return inv
}

// publishedValue reports whether a prose token restates a published numeral:
// exact string, or within half an ulp of the token's own last written digit
// of ANY published value (an honest rounded re-quote is still published).
func (inv proseFactSubtractionInventory) publishedValue(tok proseFactNumToken) bool {
	if inv.published[tok.raw] {
		return true
	}
	tol := 0.5 * proseScalarUlp(tok.raw)
	i := sort.SearchFloat64s(inv.publishedVals, tok.val-tol)
	return i < len(inv.publishedVals) && inv.publishedVals[i] <= tok.val+tol
}

// proseFactNumToken is one prose-side decimal numeric token.
type proseFactNumToken struct {
	raw string
	val float64
}

// proseFactUnitDecimalTokens extracts the unit's boundary-clean decimal
// tokens (token-level presence only: percent forms and digit/dot-adjacent
// fragments stay out; a leading minus stays IN — subtrahends follow one).
func proseFactUnitDecimalTokens(text string) []proseFactNumToken {
	var out []proseFactNumToken
	for _, loc := range proseFactDecimalTokenRE.FindAllStringIndex(text, -1) {
		start, end := loc[0], loc[1]
		if start > 0 {
			if prev := text[start-1]; prev == '.' || (prev >= '0' && prev <= '9') {
				continue
			}
		}
		if end < len(text) {
			next := text[end]
			if next == '.' || next == '%' || (next >= '0' && next <= '9') {
				continue
			}
			if strings.HasPrefix(text[end:], "％") {
				continue
			}
		}
		raw := text[start:end]
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		out = append(out, proseFactNumToken{raw: raw, val: v})
	}
	return out
}

// proseFactImplicitSubtractionFindings runs the four-signal detection over
// the prose units; at most one finding per unit, capped per report.
func proseFactImplicitSubtractionFindings(prose []proseTextUnit, inv proseFactSubtractionInventory) []proseScalarBindingFinding {
	if len(inv.remainder) == 0 || len(inv.proven) == 0 {
		return nil
	}
	var out []proseScalarBindingFinding
	for _, unit := range prose {
		if len(out) >= proseFactImplicitSubCap {
			break
		}
		var xs, ys, zs []proseFactNumToken
		seenX, seenY := map[string]bool{}, map[string]bool{}
		for _, tok := range proseFactUnitDecimalTokens(unit.text) {
			switch {
			case inv.remainder[tok.raw]:
				if !seenX[tok.raw] {
					seenX[tok.raw] = true
					xs = append(xs, tok)
				}
			case inv.proven[tok.raw]:
				if !seenY[tok.raw] && len(ys) < proseFactImplicitSubTermCap {
					seenY[tok.raw] = true
					ys = append(ys, tok)
				}
			case !inv.publishedValue(tok):
				zs = append(zs, tok)
			}
		}
		if len(xs) == 0 || len(ys) == 0 || len(zs) == 0 {
			continue
		}
		if f, ok := proseFactImplicitSubtractionMatch(unit, xs, ys, zs); ok {
			out = append(out, f)
		}
	}
	return out
}

// proseFactImplicitSubtractionMatch enumerates the proven-share subsets
// (fullest decomposition first, then ascending mask — deterministic) against
// the residual candidates in text order and renders the first arithmetic hit
// as a fact-only line.
func proseFactImplicitSubtractionMatch(unit proseTextUnit, xs, ys, zs []proseFactNumToken) (proseScalarBindingFinding, bool) {
	masks := make([]int, 0, 1<<len(ys)-1)
	for m := 1; m < 1<<len(ys); m++ {
		masks = append(masks, m)
	}
	sort.SliceStable(masks, func(i, j int) bool {
		if pi, pj := bits.OnesCount(uint(masks[i])), bits.OnesCount(uint(masks[j])); pi != pj {
			return pi > pj
		}
		return masks[i] < masks[j]
	})
	for _, x := range xs {
		for _, mask := range masks {
			sum, tol := 0.0, proseFactImplicitSubBaseTolMS+0.5*proseScalarUlp(x.raw)
			var terms []string
			for i, y := range ys {
				if mask&(1<<i) == 0 {
					continue
				}
				sum += y.val
				tol += 0.5 * proseScalarUlp(y.raw)
				terms = append(terms, y.raw)
			}
			w := x.val - sum
			if w <= 0 {
				continue
			}
			for _, z := range zs {
				diff := w - z.val
				if diff < 0 {
					diff = -diff
				}
				if diff > tol+0.5*proseScalarUlp(z.raw) {
					continue
				}
				expr := x.raw + "-" + strings.Join(terms, "-")
				return proseScalarBindingFinding{
					entry: fmt.Sprintf("fact juxtaposition (block %q): %s is a published cause-unproven remainder value — already net of every caller-named proven share, which all lie outside it; the co-occurring numbers satisfy %s≈%s, and %s matches no typed published value of this run",
						unit.blockID, x.raw, expr, z.raw, z.raw),
					entryZH: fmt.Sprintf("系统事实对照(块 %s):%s 为已扣除全部已证原因份额后的净值(原因未证余数发布值,各已证份额均在其外);文中共现数值满足 %s≈%s,而 %s 非本次运行的任何 typed 发布值",
						unit.blockID, x.raw, expr, z.raw, z.raw),
				}, true
			}
		}
	}
	return proseScalarBindingFinding{}, false
}

// proseFactWakeupDegradation reads the engine's typed target_cpu degradation
// marker off the run's deterministic tool banners.
func proseFactWakeupDegradation(bus *types.BusContext, mut *types.MutableState) (int, bool) {
	scan := func(text string) (int, bool) {
		if m := proseFactWakeupDegradedRE.FindStringSubmatch(text); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				return n, true
			}
		}
		return 0, false
	}
	for _, tr := range bus.ToolResults {
		if n, ok := scan(tr.Summary); ok {
			return n, true
		}
	}
	if ta := mut.TurnAArtifacts(); ta != nil {
		for _, tr := range ta.ToolResults {
			if n, ok := scan(tr.Summary); ok {
				return n, true
			}
		}
	}
	return 0, false
}

// proseFactStatePartitionFindings — FACT-REL arm a (§29.55.4 F2 互斥状态
// 包含关系编造 + R2-F1, 2026-07-13). PRESENCE trigger only: one prose unit
// carries a thread token AND ≥2 distinct state-valued tokens (a scheduler-
// state keyword within the wall-clock keyword gap before an ms numeral —
// the same battle-tested token resolver as the P6 lane, zero relation
// reading). The appendix then lists that thread's typed state account with
// the mutual-exclusion partition fact, decomposed with the actual values
// (附注自证义务: the Σ equation prints real addends and the real computed
// sum). §29.53.2 discipline: the system never reads WHAT relation the prose
// claimed — listing the typed partition is harmless under correct prose.
func proseFactStatePartitionFindings(prose []proseTextUnit, facts map[string]*proseFactThreadFacts) []proseScalarBindingFinding {
	var out []proseScalarBindingFinding
	nameToTIDs := proseFactNameIndex(facts)
	emitted := map[string]bool{}
	for _, unit := range prose {
		if len(out) >= proseFactPartitionCap {
			break
		}
		if len(proseFactUnitStateValueDims(unit)) < 2 {
			continue
		}
		tids := make([]string, 0, 4)
		for tid := range proseFactThreadsInText(unit.text, facts, nameToTIDs) {
			tids = append(tids, tid)
		}
		sort.Strings(tids)
		for _, tid := range tids {
			if emitted[tid] || len(out) >= proseFactPartitionCap {
				continue
			}
			zh, en := proseFactPartitionFact(facts[tid])
			if zh == "" {
				continue
			}
			emitted[tid] = true
			out = append(out, proseScalarBindingFinding{entry: en, entryZH: zh})
		}
	}
	return out
}

// proseFactUnitStateValueDims returns the DISTINCT scheduler-state
// dimensions of the unit's state-valued tokens (state keyword within the
// bounded gap before a positive ms numeral — token-level presence only).
func proseFactUnitStateValueDims(unit proseTextUnit) map[proseWallClockDimension]bool {
	toks := extractProseScalarTokens(unit.blockID, unit.text)
	if len(toks) == 0 {
		return nil
	}
	cpuContext := proseWallClockSentenceHasCPUContext(unit.text)
	dims := map[proseWallClockDimension]bool{}
	for _, tok := range toks {
		if tok.percent() || tok.Value <= 0 {
			continue
		}
		if dim, ok := proseWallClockClaimDimension(unit.text, tok.Pos, cpuContext); ok {
			dims[dim] = true
		}
	}
	return dims
}

// proseFactPartitionBalanceTolMS / proseFactPartitionBalanceTolRel — 件4
// (2026-07-13) balance gate for the partition fact's Σ=窗长 identity claim.
// Own named constants (容差常量禁跨语义借用): last-digit rounding of five
// printed %.3f lanes plus honest window-edge clipping stay inside; anything
// beyond drops the identity claim (回退措辞), never the value listing.
const (
	proseFactPartitionBalanceTolMS  = 0.01
	proseFactPartitionBalanceTolRel = 0.001
)

// proseFactPartitionFact renders one thread's typed state account with the
// mutual-exclusion partition fact (zh, en). 件3 (复核 P1-2, 2026-07-13):
// ONLY the target_window_states account form remains — rank-note state dims
// drift semantically per row kind, so the former fallback minted chimera
// accounts (宁缺勿假: no account → no partition fact). 件4: the account is a
// FIVE-state partition (io_wait is its own lane); the Σ decomposition lists
// all five actual values, and the Σ=窗长 identity claim is gated on the
// balance actually holding (unbalanced → same listing, no identity claim).
func proseFactPartitionFact(f *proseFactThreadFacts) (string, string) {
	if f == nil || f.subject == "" || f.account == nil {
		return "", ""
	}
	a := f.account
	r := a.dims[proseWallClockDimRunning]
	q := a.dims[proseWallClockDimRunnable]
	s := a.dims[proseWallClockDimSleep]
	d := a.dims[proseWallClockDimDState]
	io := a.ioWait
	sum := r + q + s + d + io
	head := fmt.Sprintf("typed 事实:%s — 窗内五态账 running %.3f/runnable %.3f/sleep %.3f/非IO D-state %.3f/io_wait %.3fms · 五态为互斥分区,同一时刻仅居一态,不存在包含关系",
		f.subject, r, q, s, d, io)
	headEN := fmt.Sprintf("typed fact: %s — in-window five-state account running %.3f/runnable %.3f/sleep %.3f/non-IO D-state %.3f/io_wait %.3fms · the five states are a mutually exclusive partition — one state at any instant, none contains another",
		f.subject, r, q, s, d, io)
	diff := sum - a.windowMS
	if diff < 0 {
		diff = -diff
	}
	tol := proseFactPartitionBalanceTolMS + a.windowMS*proseFactPartitionBalanceTolRel
	if a.windowMS > 0 && diff <= tol {
		// Balanced: the Σ=窗长 identity claim ships with its decomposed
		// actual addends (附注自证义务).
		zh := head + fmt.Sprintf("(Σ=%.3f+%.3f+%.3f+%.3f+%.3f=%.3fms,窗长 %.3fms)",
			r, q, s, d, io, sum, a.windowMS)
		en := headEN + fmt.Sprintf(" (Σ=%.3f+%.3f+%.3f+%.3f+%.3f=%.3fms, window %.3fms)",
			r, q, s, d, io, sum, a.windowMS)
		return zh, en
	}
	// Unbalanced (or windowless): list the actual Σ and the window side by
	// side WITHOUT the identity claim (回退措辞 — the reader sees both).
	zh := head + fmt.Sprintf("(Σ五态=%.3fms;窗长 %.3fms)", sum, a.windowMS)
	en := headEN + fmt.Sprintf(" (Σ five states=%.3fms; window %.3fms)", sum, a.windowMS)
	return zh, en
}

// --- small helpers -------------------------------------------------------------

func proseFactAppendUnique(list []string, v string) []string {
	if v == "" || v == "unknown" {
		return list
	}
	for _, have := range list {
		if have == v {
			return list
		}
	}
	return append(list, v)
}

func proseFactTokenAfter(text, marker string) string {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return ""
	}
	rest := text[idx+len(marker):]
	if end := strings.IndexAny(rest, " \t,;)）"); end >= 0 {
		rest = rest[:end]
	}
	if plus := strings.IndexByte(rest, '+'); plus > 0 {
		rest = rest[:plus]
	}
	return strings.TrimSpace(rest)
}

func proseFactNoteInt(notes []string, key string) (int, bool) {
	v := strings.TrimSpace(proseWallClockNoteValue(notes, key))
	if v == "" {
		return 0, false
	}
	v = strings.TrimRightFunc(v, func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	})
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return n, true
}

func proseFactNoteFloat(notes []string, key string) (float64, bool) {
	v := strings.TrimSpace(proseWallClockNoteValue(notes, key))
	if v == "" {
		return 0, false
	}
	// Banner-parsed notes keep their unit suffix ("freq=1160000kHz").
	v = strings.TrimRightFunc(v, func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	})
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func proseFactFreqListLabel(points []float64) string {
	var distinct []float64
	for _, p := range points {
		dup := false
		for _, have := range distinct {
			d := have - p
			if d < 0 {
				d = -d
			}
			if d <= 500 {
				dup = true
				break
			}
		}
		if !dup {
			distinct = append(distinct, p)
		}
	}
	sort.Float64s(distinct)
	var parts []string
	for i, p := range distinct {
		if i >= 4 {
			parts = append(parts, fmt.Sprintf("(+%d)", len(distinct)-i))
			break
		}
		parts = append(parts, fmt.Sprintf("%.0fMHz", p/1e3))
	}
	return strings.Join(parts, "/")
}

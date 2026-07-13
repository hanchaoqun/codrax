package orchestrator

import (
	"fmt"
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
//	  (「<线程>:typed 席位=… · typed 等待对象=… · 窗内四态=…」), and the
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
//     carries the thread's typed 等待对象=dma_fence_default_w(12条记录);
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
	proseFactThreadCap   = 6
	proseFactCPUCap      = 3
	proseFactEquationCap = 4
	// proseFactEquationBaseTol: honest last-digit rounding of each printed
	// operand never trips the arithmetic check (per-numeral half-ulps sum,
	// floored here). Own named constant (容差常量禁跨语义借用).
	proseFactEquationBaseTolMS = 0.002
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
	if doc == nil || bus == nil || mut == nil {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return nil
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

	// ── arithmetic verdicts first (the only verdict wording, math-only) ──
	for _, f := range proseFactEquationFindings(prose) {
		add(f)
	}

	// ── engine typed degradation fact (target_cpu 退化) ──────────────────
	if total, ok := proseFactWakeupDegradation(bus, mut); ok && proseFactMentionsCPU(prose) {
		add(proseScalarBindingFinding{
			entry: fmt.Sprintf("typed fact: this trace's in-window sched_wakeup target_cpu field is suspected degraded (%d events, all zero) — per-CPU accounting keyed on target_cpu is unreliable", total),
			entryZH: fmt.Sprintf("typed 事实:本 trace 窗内 sched_wakeup 的 target_cpu 字段疑退化(%d 条全 0),按 target_cpu 归账的 per-CPU 口径不可靠",
				total),
		})
	}

	// ── per-thread fact lines (presence-triggered) ───────────────────────
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
				if v, ok := proseFactNoteFloat(notes, types.TraceNoteKeyEffectiveImpactMS); ok {
					seat.effectiveMS, seat.hasEff = v, true
				} else if v, err := strconv.ParseFloat(strings.TrimSpace(record.Value), 64); err == nil && v > 0 {
					seat.effectiveMS, seat.hasEff = v, true
				}
				if n, ok := proseFactNoteInt(notes, types.TraceNoteKeyMemberCount); ok && n > 1 {
					seat.memberCount = n
				}
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
				if !dup && len(f.seats) < 4 {
					f.seats = append(f.seats, seat)
				}
			}
			if record.Confidence > 0 {
				f.confidences = append(f.confidences, record.Confidence)
			}
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

// proseFactPresentThreads returns the tids of evidence-face threads whose
// spelling (strict name-tid token, or a loose single-digit-tail name that
// resolves to exactly ONE evidence thread by name part) appears in the
// model prose. Pure token presence — no claim reading.
func proseFactPresentThreads(prose []proseTextUnit, facts map[string]*proseFactThreadFacts) []string {
	nameToTIDs := map[string]map[string]bool{}
	for tid, f := range facts {
		if dash := strings.LastIndexByte(f.subject, '-'); dash > 0 {
			name := f.subject[:dash]
			if nameToTIDs[name] == nil {
				nameToTIDs[name] = map[string]bool{}
			}
			nameToTIDs[name][tid] = true
			// The evidence spelling may carry a leading '.' the prose
			// extractor cannot start a token with (.ugc.aweme.lite-17267 →
			// prose "ugc.aweme.lite-17267").
			if trimmed := strings.TrimLeft(name, "."); trimmed != name && trimmed != "" {
				if nameToTIDs[trimmed] == nil {
					nameToTIDs[trimmed] = map[string]bool{}
				}
				nameToTIDs[trimmed][tid] = true
			}
		}
	}
	present := map[string]bool{}
	for _, unit := range prose {
		for _, tref := range extractProseScalarThreadRefs(unit.text) {
			if _, ok := facts[tref.TID]; ok {
				present[tref.TID] = true
			}
		}
		// Loose names (single-digit tails: keva-3) resolve by name part when
		// unambiguous.
		for _, m := range proseScalarLooseThreadRE.FindAllString(unit.text, -1) {
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
			if seat.hasEff {
				z += fmt.Sprintf("(有效归因 %.3fms", seat.effectiveMS)
				e += fmt.Sprintf(" (effective %.3fms", seat.effectiveMS)
				if seat.memberCount > 1 {
					z += fmt.Sprintf(",成员共%d", seat.memberCount)
					e += fmt.Sprintf(", %d members", seat.memberCount)
				}
				z += ")"
				e += ")"
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
		zh = append(zh, "typed 等待对象="+strings.Join(f.callers, "/")+count)
		en = append(en, "typed waiting object="+strings.Join(f.callers, "/")+countEN)
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
		zh = append(zh, fmt.Sprintf("窗内四态 running %.3f/runnable %.3f/sleep %.3f/D-state %.3fms",
			f.account.dims[proseWallClockDimRunning], f.account.dims[proseWallClockDimRunnable],
			f.account.dims[proseWallClockDimSleep], f.account.dims[proseWallClockDimDState]))
		en = append(en, fmt.Sprintf("in-window four-state running %.3f/runnable %.3f/sleep %.3f/D-state %.3fms",
			f.account.dims[proseWallClockDimRunning], f.account.dims[proseWallClockDimRunnable],
			f.account.dims[proseWallClockDimSleep], f.account.dims[proseWallClockDimDState]))
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

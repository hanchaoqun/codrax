package context

// trace_wait_evidence_summary.go — EVID-BR 件① (§29.55.4 F1/R2-F3,
// docs/design/real_trace_campaign_20260705.md, 2026-07-13).
//
// Witnesses: donghu 四跑四答案 — the question asked for the KERNEL-recorded
// wait object; the typed appendix carried dma_fence_default_w every run while
// the model prose invented four different answers, because the blocked_reason
// typed observations never reached the model's evidence face. And the waker
// inversion (R2-F3): the model listed the D-entry switch's next_comm as the
// "waker" 11/11 times while the real sched_wakeup source was
// gpu-token-id4-2931 — the measured wakeup edges never reached the model
// face either.
//
// CR-1 precedent (trace_board_summary.go): feed the typed data at the
// investigation and answer-rendering dispatches — INPUT, never a gate
// (§29.42.4 喂好数据、教好方法、信任模型). Same shape here: the kernel's own
// sched_blocked_reason wait-object account (per-caller symbol × count × Σms,
// verbatim published values) and the measured sched_wakeup edges (waker /
// wakee / timestamp — the data itself distinguishes a waker from a switch
// next_comm; no teaching about other lanes). Sources are typed notes on the
// observation ledger only; volume is deliberately restrained.
//
// 修复轮 件1 (census 根修, 2026-07-13): the per-caller census now consumes
// the ENGINE's typed blocked_reason_census note (full-accumulator fold,
// per-caller 符号×count×Σms) — the banner-parse arm is demoted to a
// FALLBACK that engages only when no census note reached the ledger at all
// (复核实锤: the banner rows are a top-8 display truncation with
// per-offset bucket splits, and blob previews can silently drop banner
// middles). 件5: the analysis target / anchor threads never fall to the
// thread cap, every cap discloses its overflow, and the wakeup-edge cap
// samples head+tail with anchor-wakee priority (答案常在窗尾).
//
// PROSE-RC (§29.57 残余立案, 2026-07-13): every exportable quantity is fed
// as a NAMED fact so the model quotes instead of re-deriving — the fed
// values were all correct while the prose re-computed them wrong (tieba
// opening "18次" against a fed ×17; "2.731ms" minted by subtracting the
// three cause seats from the cause-unproven remainder 10.433 the same
// report published; a waker summary "8×" self-counted against its own
// 12-row list). Three named-fact arms: ① per-waker observed wakeup-edge
// counts aggregated from the minted edge records (count desc, deterministic
// ties, capped with a named remainder of unlisted edges — the counts are
// labelled with the observed-edge caliber ONLY, never a whole-window
// caliber, because the minted edge inventory is itself capped upstream);
// ② the cause-unproven remainder becomes a standalone named fact line
// (verbatim seat value, zero recompute) carrying the typed partition
// property (cause seats = proven share only, disjoint from the remainder,
// never subtracted against each other — a mathematical property of the
// account, not a characterization of any prose, §29.53.2 边界内); ③ the
// census lead carries the engine's own total record count verbatim
// (record value of the census observation) as a directly quotable total.

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	// traceWaitEvidenceThreadCap / traceWaitEvidenceCallerCap /
	// traceWaitEvidenceWakeupEdgeCap bound the summary (体积预算克制 —
	// never a full observation dump). Anchor threads bypass the thread cap
	// (件5: the question's own thread must never sink below the cap).
	traceWaitEvidenceThreadCap     = 6
	traceWaitEvidenceCallerCap     = 5
	traceWaitEvidenceWakeupEdgeCap = 12
	// traceWaitEvidenceWakerCountCap (PROSE-RC ①) bounds the per-waker
	// observed-edge count fact lines; edges left uncovered by the listed
	// count lines are disclosed as a named remainder (帽外具名余数).
	traceWaitEvidenceWakerCountCap = 8
)

// traceWaitCallerFact is one thread's published wait-object fact: the
// kernel blocked_reason caller symbol (or the honest cause-unproven
// remainder) with the record's own published magnitude, verbatim.
type traceWaitCallerFact struct {
	caller   string // semantic caller symbol; "" on the unproven remainder
	state    string // the record's state/type token (record.Object)
	value    string // published magnitude, verbatim note/record value (ms)
	members  string // member_count note, verbatim ("" when unpublished)
	unproven bool   // §29.50.5 cause-unproven remainder marker
}

// traceWaitCensusEntry is one caller symbol's census account for a thread:
// 符号×count×Σms (Σms verbatim from the engine note; "" when the engine
// withheld it — partial delay coverage).
type traceWaitCensusEntry struct {
	count int
	sigMS string
}

type traceWaitThreadFacts struct {
	subject string
	facts   []traceWaitCallerFact
	// windowCount / windowCallers: the unconsumed in-window blocked_reason
	// markers (CR-3 件② P10 lane) — count + symbol list, verbatim.
	windowCount   int
	windowCallers []string
	// census: the engine's pid-keyed per-caller blocked_reason census
	// (件1 根修: primary source = the typed blocked_reason_census note off
	// the FULL accumulator; the banner parse is a fallback only). Keyed by
	// semantic symbol; per-symbol MAX across republications, never summed.
	census         map[string]traceWaitCensusEntry
	censusOrder    []string
	censusOverflow int // distinct caller symbols beyond the engine's cap
	// censusTotal (PROSE-RC ③): the census observation's own published
	// total record count, verbatim (the engine's Value field on the
	// blocked_reason_census record — full-accumulator in-window total).
	// MAX across republications like the per-symbol counts, never summed.
	censusTotal      string
	censusTotalValue int
	// anchor (件5): the run's analysis-target / anchor thread — carries a
	// target_window_states account or a tier=target_self_state row. Anchor
	// threads never fall to the thread cap (the tieba symptom thread rides
	// the P10 marker lane with orderMS=0 and was first to be evicted).
	anchor  bool
	orderMS float64 // largest published magnitude (ordering only)
}

// traceWaitCensusBannerRE reads the engine's deterministic window_stats
// banner row for the pid-keyed blocked_reason census (FALLBACK lane only —
// the typed note is the primary source):
//
//	- blocked_reason <thread> iowait=<n> count=<n> line=<n> caller=<reason>
//
// Thread labels may contain spaces (lazy match up to the typed iowait=).
var traceWaitCensusBannerRE = regexp.MustCompile(`(?m)^- blocked_reason (.+?) iowait=[0-9]+ count=([0-9]+) line=[0-9]+ caller=(\S+)`)

// traceWaitCensusEntryRE parses ONE engine census-note entry:
// "sym×12(Σ38.983ms)" or "sym×12".
var traceWaitCensusEntryRE = regexp.MustCompile(`^(.+?)×([0-9]+)(?:\(Σ([0-9.]+)ms\))?$`)

// formatTraceWaitWakeEvidenceFromLedger renders the typed kernel
// wait-object + wakeup-source evidence summary, or "" when the run carries
// no such typed observations (zero-emission anti-noise). toolResults may be
// nil; it feeds only the banner-parse census FALLBACK.
func formatTraceWaitWakeEvidenceFromLedger(ledger types.ObservationLedger, toolResults []types.ToolResult) string {
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return ""
	}
	threads := map[string]*traceWaitThreadFacts{}
	var threadOrder []string
	get := func(subject string) *traceWaitThreadFacts {
		if f, ok := threads[subject]; ok {
			return f
		}
		f := &traceWaitThreadFacts{subject: subject}
		threads[subject] = f
		threadOrder = append(threadOrder, subject)
		return f
	}
	addCensus := func(f *traceWaitThreadFacts, symbol string, entry traceWaitCensusEntry) {
		if symbol == "" || entry.count <= 0 {
			return
		}
		if f.census == nil {
			f.census = map[string]traceWaitCensusEntry{}
		}
		have, seen := f.census[symbol]
		if !seen {
			f.censusOrder = append(f.censusOrder, symbol)
		}
		// Per-symbol MAX across republications — never summed.
		if !seen || entry.count > have.count {
			f.census[symbol] = entry
		}
	}
	type wakeupEdge struct {
		waker, wakee, ts, latency string
		tsValue                   float64
	}
	var edges []wakeupEdge
	seenEdges := map[string]bool{}
	censusFromNotes := false
	for _, record := range ledger.Records {
		if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
			continue
		}
		subject := strings.TrimSpace(record.Subject)
		notes := traceBoardNoteMap(record.RichNotes)
		// ── measured wakeup edges (sched_wakeup source facts) ──────────────
		if strings.TrimSpace(record.Predicate) == "wakeup_chain_edge" {
			wakee := strings.TrimSpace(record.Object)
			ts := strings.TrimSpace(notes["wakeup_ts"])
			if subject == "" || wakee == "" {
				continue
			}
			key := subject + "->" + wakee + "@" + ts
			if seenEdges[key] {
				continue // identical republications collapse
			}
			seenEdges[key] = true
			edge := wakeupEdge{waker: subject, wakee: wakee, ts: ts, latency: strings.TrimSpace(notes["latency"])}
			edge.tsValue, _ = strconv.ParseFloat(ts, 64)
			edges = append(edges, edge)
			continue
		}
		if subject == "" {
			continue
		}
		// ── anchor threads (件5): the analysis target's own records ────────
		if strings.TrimSpace(record.Predicate) == "target_window_states" ||
			strings.TrimSpace(notes[types.TraceNoteKeyTier]) == "target_self_state" {
			get(subject).anchor = true
		}
		// ── the engine's typed per-pid census note (件1 primary source) ────
		if raw := strings.TrimSpace(notes[types.TraceNoteKeyBlockedReasonCensus]); raw != "" {
			f := get(subject)
			for _, part := range strings.Split(raw, "/") {
				m := traceWaitCensusEntryRE.FindStringSubmatch(strings.TrimSpace(part))
				if m == nil {
					continue
				}
				count, err := strconv.Atoi(m[2])
				if err != nil || count <= 0 {
					continue
				}
				addCensus(f, strings.TrimSpace(m[1]), traceWaitCensusEntry{count: count, sigMS: m[3]})
				censusFromNotes = true
			}
			if raw := strings.TrimSpace(notes[types.TraceNoteKeyBlockedReasonCensusOverflow]); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n > f.censusOverflow {
					f.censusOverflow = n
				}
			}
			// PROSE-RC ③: the census record's own Value is the engine's
			// full-accumulator in-window total record count — carry it
			// verbatim as a directly quotable named total (MAX across
			// republications, matching the per-symbol count discipline).
			if strings.TrimSpace(record.Predicate) == "blocked_reason_census" {
				if raw := strings.TrimSpace(record.Value); raw != "" {
					if n, err := strconv.Atoi(raw); err == nil && n > f.censusTotalValue {
						f.censusTotalValue = n
						f.censusTotal = raw
					}
				}
			}
		}
		// ── kernel wait-object facts (blocked_reason typed notes) ──────────
		caller := strings.TrimSpace(notes[types.TraceNoteKeyBlockedReasonCaller])
		if caller == "unknown" {
			// The engine's not-a-symbol sentinel teaches nothing here (same
			// filter as the appendix fact lane).
			caller = ""
		}
		unproven := strings.TrimSpace(notes[types.TraceNoteKeyDStateCauseUnprovenRemainder]) == "true"
		if caller != "" || unproven {
			fact := traceWaitCallerFact{
				caller:   caller,
				state:    strings.TrimSpace(record.Object),
				members:  strings.TrimSpace(notes[types.TraceNoteKeyMemberCount]),
				unproven: unproven,
			}
			// Published magnitude, strongest lane first — verbatim strings,
			// never recomputed (the CR-1 rule).
			for _, v := range []string{
				notes[types.TraceNoteKeyEffectiveImpactMS],
				notes["duration"],
				strings.TrimSpace(record.Value),
			} {
				if strings.TrimSpace(v) != "" {
					fact.value = strings.TrimSpace(v)
					break
				}
			}
			f := get(subject)
			dup := false
			for _, have := range f.facts {
				if have == fact {
					dup = true
					break
				}
			}
			if !dup {
				f.facts = append(f.facts, fact)
				if ms, err := strconv.ParseFloat(fact.value, 64); err == nil && ms > f.orderMS {
					f.orderMS = ms
				}
			}
		}
		// Unconsumed in-window markers (kept even when a caller fact exists:
		// both are typed truth on their own lanes).
		if raw := strings.TrimSpace(notes[types.TraceNoteKeyBlockedReasonWindowCount]); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				f := get(subject)
				if n > f.windowCount {
					f.windowCount = n
				}
				for _, sym := range strings.Split(notes[types.TraceNoteKeyBlockedReasonWindowCaller], "/") {
					sym = strings.TrimSpace(sym)
					if sym == "" {
						continue
					}
					dup := false
					for _, have := range f.windowCallers {
						if have == sym {
							dup = true
							break
						}
					}
					if !dup {
						f.windowCallers = append(f.windowCallers, sym)
					}
				}
			}
		}
	}
	// ── banner-parse census FALLBACK (件1: engages only when NO typed
	// census note reached the ledger — the banner is a top-8 display view
	// with per-offset bucket splits and preview-truncation exposure) ───────
	if !censusFromNotes {
		for _, tr := range toolResults {
			if !tr.Success {
				continue
			}
			for _, m := range traceWaitCensusBannerRE.FindAllStringSubmatch(tr.Summary, -1) {
				subject := strings.TrimSpace(m[1])
				count, err := strconv.Atoi(m[2])
				if err != nil || count <= 0 || subject == "" {
					continue
				}
				// Semantic symbol head (the engine's caller symbol lane trims
				// the +offset[module] tail the raw banner keeps).
				symbol := strings.TrimSpace(m[3])
				if plus := strings.IndexByte(symbol, '+'); plus > 0 {
					symbol = symbol[:plus]
				}
				if symbol == "" || symbol == "unknown" {
					continue
				}
				addCensus(get(subject), symbol, traceWaitCensusEntry{count: count})
			}
		}
	}

	// ── thread selection (件5: anchors never fall to the cap) ─────────────
	var subjects []string
	for _, s := range threadOrder {
		f := threads[s]
		if len(f.facts) > 0 || f.windowCount > 0 || len(f.census) > 0 {
			subjects = append(subjects, s)
		}
	}
	if len(subjects) == 0 && len(edges) == 0 {
		return ""
	}
	sort.SliceStable(subjects, func(i, j int) bool {
		if threads[subjects[i]].anchor != threads[subjects[j]].anchor {
			return threads[subjects[i]].anchor
		}
		if threads[subjects[i]].orderMS != threads[subjects[j]].orderMS {
			return threads[subjects[i]].orderMS > threads[subjects[j]].orderMS
		}
		return subjects[i] < subjects[j]
	})
	selectedSubjects := subjects
	subjectOverflow := 0
	if len(subjects) > traceWaitEvidenceThreadCap {
		// Anchors sort first, so the cap can only evict non-anchor threads;
		// when anchors alone exceed the cap they ALL stay.
		cut := traceWaitEvidenceThreadCap
		for cut < len(subjects) && threads[subjects[cut]].anchor {
			cut++
		}
		subjectOverflow = len(subjects) - cut
		selectedSubjects = subjects[:cut]
	}

	// ── wakeup-edge selection (件5: anchor-wakee priority + head/tail
	// sampling — 答案常在窗尾, the old earliest-12 cut dropped the window
	// tail wholesale) ──────────────────────────────────────────────────────
	sort.SliceStable(edges, func(i, j int) bool { return edges[i].tsValue < edges[j].tsValue })
	selectedEdges := edges
	edgeOverflow := 0
	if len(edges) > traceWaitEvidenceWakeupEdgeCap {
		anchorWakee := func(e wakeupEdge) bool {
			f, ok := threads[e.wakee]
			return ok && f.anchor
		}
		var pool, rest []wakeupEdge
		for _, e := range edges {
			if anchorWakee(e) {
				pool = append(pool, e)
			} else {
				rest = append(rest, e)
			}
		}
		pick := func(list []wakeupEdge, n int) []wakeupEdge {
			if n <= 0 {
				return nil
			}
			if len(list) <= n {
				return list
			}
			head := (n + 1) / 2
			tail := n - head
			out := append([]wakeupEdge(nil), list[:head]...)
			return append(out, list[len(list)-tail:]...)
		}
		selectedEdges = pick(pool, traceWaitEvidenceWakeupEdgeCap)
		selectedEdges = append(selectedEdges, pick(rest, traceWaitEvidenceWakeupEdgeCap-len(selectedEdges))...)
		sort.SliceStable(selectedEdges, func(i, j int) bool { return selectedEdges[i].tsValue < selectedEdges[j].tsValue })
		edgeOverflow = len(edges) - len(selectedEdges)
	}

	var b strings.Builder
	b.WriteString("Measured kernel wait-object and wakeup-source evidence for this run (verbatim values). When the question asks WHAT a thread was waiting on in the kernel (uninterruptible / D-state / IO wait), the kernel's own record is the blocked_reason caller symbol below — answer with these symbols verbatim; a share marked cause-unproven has NO kernel-recorded wait object, so never invent one for it. Per-thread blocked_reason record counts are keyed by the waiting thread itself, not by the trace line a record happens to print on; a per-caller Σ value is the records' own self-reported delay= field and may include pre-window accumulation. When the question asks WHO woke a thread (and when), answer with the sched_wakeup edge below: its waker thread and its wakeup timestamp. Use the named counts, totals, and remainder shares below verbatim — never re-count listed rows yourself, and never derive a share by adding or subtracting other published values (the published value already IS the share it names).\n")
	if len(selectedSubjects) > 0 {
		b.WriteString("Kernel-recorded wait objects (sched_blocked_reason):\n")
		for _, subject := range selectedSubjects {
			f := threads[subject]
			var parts []string
			for j, fact := range f.facts {
				if j >= traceWaitEvidenceCallerCap {
					parts = append(parts, fmt.Sprintf("(+%d more)", len(f.facts)-traceWaitEvidenceCallerCap))
					break
				}
				var seg string
				if fact.unproven {
					seg = "cause-unproven remainder (no blocked_reason record backs this share)"
				} else {
					seg = "caller=" + fact.caller
				}
				if fact.state != "" {
					seg += " · " + fact.state
				}
				if fact.value != "" {
					seg += " " + fact.value + "ms"
				}
				if fact.members != "" {
					seg += " · members=" + fact.members
				}
				parts = append(parts, seg)
			}
			if f.windowCount > 0 {
				seg := fmt.Sprintf("window holds %d blocked_reason record(s)", f.windowCount)
				if len(f.windowCallers) > 0 {
					seg += " (caller=" + strings.Join(f.windowCallers, "/") + ")"
				}
				parts = append(parts, seg)
			}
			if len(f.censusOrder) > 0 {
				// 符号×count×Σms, ordered by count (desc) then first appearance.
				symbols := append([]string(nil), f.censusOrder...)
				sort.SliceStable(symbols, func(i, j int) bool {
					return f.census[symbols[i]].count > f.census[symbols[j]].count
				})
				var segs []string
				for _, sym := range symbols {
					entry := f.census[sym]
					seg := fmt.Sprintf("%s ×%d", sym, entry.count)
					if entry.sigMS != "" {
						seg += "(Σ" + entry.sigMS + "ms)"
					}
					segs = append(segs, seg)
				}
				if f.censusOverflow > 0 {
					segs = append(segs, fmt.Sprintf("(+%d more caller symbol(s))", f.censusOverflow))
				}
				lead := "kernel blocked_reason record census for THIS thread"
				if f.censusTotal != "" {
					// PROSE-RC ③: the engine's own published total, verbatim —
					// a directly quotable count (tieba 开篇 +1 漂移 witness).
					lead += fmt.Sprintf(" — total %s blocked_reason record(s) in its selected window, use this total verbatim", f.censusTotal)
				}
				parts = append(parts, lead+": "+strings.Join(segs, " / "))
			}
			b.WriteString("- " + subject + " — " + strings.Join(parts, "; ") + "\n")
			// PROSE-RC ②: the cause-unproven remainder as a standalone NAMED
			// fact (verbatim seat value, zero recompute) with the typed
			// partition property. Iterates ALL facts — a remainder share
			// never falls to the caller cap. The witness: prose minted
			// "2.731ms" by subtracting the cause seats (7.702) from the
			// remainder 10.433 the same report had already published.
			for _, fact := range f.facts {
				if !fact.unproven || fact.value == "" {
					continue
				}
				stateWord := ""
				if fact.state != "" {
					stateWord = fact.state + " "
				}
				memberSeg := ""
				if fact.members != "" {
					// PROSE-RC 复放新形 (tieba 052947): the prose re-scoped
					// "原因未证" onto ONE member segment (1.899) of the 10.433
					// remainder — the membership property is typed
					// (member_count), so state it: all members are jointly
					// unproven, no single member alone is the unproven part.
					memberSeg = fmt.Sprintf(" Its %s member segment(s) are ALL inside the unproven share together — no single member segment alone is the unproven part.", fact.members)
				}
				// PROSE-RC 收尾件3 (冷读姊妹形, 054419/052947): with the
				// subtraction lane closed, the re-derivation urge re-routed
				// into BINDING — the prose moved the whole remainder seat
				// (value + fold attributes) under the fscache caller's name.
				// The sister partition property closes that direction: the
				// remainder has NO kernel-recorded caller and never belongs
				// under any caller-named proven cause.
				b.WriteString(fmt.Sprintf("  cause-unproven remainder fact for %s: the %scause-unproven share is %sms, and this published value already IS the entire unproven share — use it verbatim. It has NO kernel-recorded caller and must never be attributed to any caller-named proven cause. The caller-named shares cover only the cause-proven part and are disjoint from this remainder (one account split into non-overlapping shares), so never subtract caller-share values from this remainder, and never subtract this remainder from a caller share, to derive a new unproven amount.%s\n", subject, stateWord, fact.value, memberSeg))
			}
		}
		if subjectOverflow > 0 {
			b.WriteString(fmt.Sprintf("- (+%d more threads with blocked_reason evidence; see the measured observations)\n", subjectOverflow))
		}
	}
	if len(selectedEdges) > 0 {
		b.WriteString("Measured wakeup edges (sched_wakeup; waker → wakee at timestamp):\n")
		for _, edge := range selectedEdges {
			line := "- " + edge.waker + " → " + edge.wakee
			if edge.ts != "" {
				line += " at " + edge.ts
			}
			if edge.latency != "" {
				line += " (wakeup latency " + edge.latency + "ms)"
			}
			b.WriteString(line + "\n")
		}
		if edgeOverflow > 0 {
			b.WriteString(fmt.Sprintf("- (+%d more wakeup edges; head and tail of the window are sampled above)\n", edgeOverflow))
		}
		// ── PROSE-RC ①: per-waker observed-edge count facts ────────────────
		// Aggregated over the FULL deduplicated edge inventory (including
		// edges past the row cap above — the witness summary self-counted
		// "8×" against its own 12-row list). Caliber discipline: these are
		// counts of OBSERVED wakeup-edge records only — the inventory the
		// counts describe is itself capped at the measurement side, so the
		// label never claims a whole-window/whole-trace caliber.
		type wakerPairCount struct {
			waker, wakee string
			count        int
		}
		pairCounts := map[string]*wakerPairCount{}
		var pairOrder []string
		for _, e := range edges {
			key := e.waker + "\x00" + e.wakee
			if pc, ok := pairCounts[key]; ok {
				pc.count++
				continue
			}
			pairCounts[key] = &wakerPairCount{waker: e.waker, wakee: e.wakee, count: 1}
			pairOrder = append(pairOrder, key)
		}
		sort.SliceStable(pairOrder, func(i, j int) bool {
			a, c := pairCounts[pairOrder[i]], pairCounts[pairOrder[j]]
			if a.count != c.count {
				return a.count > c.count
			}
			if a.waker != c.waker {
				return a.waker < c.waker
			}
			return a.wakee < c.wakee
		})
		listedPairs := pairOrder
		if len(pairOrder) > traceWaitEvidenceWakerCountCap {
			listedPairs = pairOrder[:traceWaitEvidenceWakerCountCap]
		}
		covered := 0
		for _, key := range listedPairs {
			covered += pairCounts[key].count
		}
		b.WriteString("Observed wakeup-edge counts per waker (each count tallies ALL measured wakeup-edge records of this run for that waker → wakee pair, including edges past the row cap above — use these counts verbatim instead of re-counting rows; they count observed wakeup edges only, so the trace itself may hold more wakeups than were measured here):\n")
		for _, key := range listedPairs {
			pc := pairCounts[key]
			b.WriteString(fmt.Sprintf("- %s → %s ×%d observed wakeup edge(s)\n", pc.waker, pc.wakee, pc.count))
		}
		if rem := len(edges) - covered; rem > 0 {
			b.WriteString(fmt.Sprintf("- (+%d more observed wakeup edge(s) across %d more waker → wakee pair(s) beyond the counts above)\n", rem, len(pairOrder)-len(listedPairs)))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

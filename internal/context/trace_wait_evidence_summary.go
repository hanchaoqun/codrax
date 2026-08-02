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
//
// WAKE-CENSUS (§29.58 立案, 2026-07-13): the per-waker count lane's PRIMARY
// source is now the engine's typed wakeup_edge_census records — per-pair
// counts folded over the FULL pre-cap edge set of each wakeup_chain result
// (count = record Value, first/last ts + overflow via typed notes; per-pair
// MAX across republications, blocked_reason census 同款纪律). The old
// PROSE-RC ① aggregation over the minted edge records stays as the FALLBACK
// lane ONLY (no census record reached the ledger — an older result shape),
// keeping its observed-edges-only caliber label verbatim (降级自认). With
// census the caliber upgrades to the whole-inventory form, the pair
// direction is pinned to the sched_wakeup source truth, and a complete
// enumeration (zero overflow) states the absence property outright — the
// PRC-F1 witness invented「OS_IPC_14_34911 ×4」for a pair whose only raw
// edge ran the OPPOSITE direction and was never fed.
//
// 修复轮 (复核 SHIP-WITH-FIXES, 2026-07-13): 件2 — the absence sentence's
// descriptive half claims exactly the census caliber ("never measured with
// a per-pair count here"), never a whole-run zero-edges claim (bundle runs
// hold engine-measured edges that publish no per-pair count), plus the
// WC-F1 scope label (an absence is data coverage, never kernel behavior);
// 件3 — overflow disclosures group by record-ID scope: MAX within one
// result scope, and a multi-scope union DE-NUMBERIZES its remainder line
// instead of minting a definite-looking unsound sum; 件4 — the per-pair MAX
// pin is order-adversarial (the stale lower count arrives first).
//
// INV-SUPPLY 件② (§29.61.11/.11a 用户裁定+确认, 2026-07-14): supply-gap-
// dominant inversion seats (the typed criterion types.TraceSupplyGapDominant
// over the SAME two published notes the display compound word judges) feed
// their seat composition as a named fact — 席位构成(➊ …优先级反转候选·供给
// 缺口主导): 反转等待(全额) X + running 折算 Y(供给缺口 Z 下界为主,热限压
// fmax)——两因并提,引用勿推导. Witness 090607: 行3 carried the full split
// while the prose compressed to the single type word and dropped the
// frequency component — the decomposition is a display face the model never
// sees, same disease family as the 等待对象四跑四答案 witness above.
//
// PROSE-RC 续批 (§29.74 R4 witness, 2026-07-14): the remainder fact's
// property-sentence family gains its third sister sentence — the explicit
// MEMBER-level prohibition (zh/en). The re-derivation urge, blocked from
// subtracting shares (052947) and from binding the whole seat under a
// caller's name (054419), re-routed into re-allocating the remainder's
// member segments: one member (1.899) bound to the fscache proven cause
// plus a minted derived unproven amount 8.534 = 10.433 − 1.899. The new
// sentence anchors the true-partition property at member granularity: the
// members are one indivisible unproven whole, no single member rebinds to
// any proven cause, and no member subtraction mints a new unproven amount.
//
// PROSE-RC-4 臂① (§29.78 第四改道向, 2026-07-14): the FOURTH sister sentence.
// With the subtraction, whole-seat binding and member re-allocation lanes
// closed, the re-derivation urge re-routed into NESTING — the prose framed
// the remainder seat as a TOTAL containing the hmfs caller shares
// (0.145+0.171, self-summed wrong as 0.296) and re-subtracted them to mint
// 10.117 = 10.433 − 0.145 − 0.171 as the "real" unproven amount; no explicit
// "=" equation, and the disjoint sentence was bypassed by the total framing.
// The new sentence states the OUTSIDE/net property with the account's own
// caller symbols named: every caller-named share lies outside this value,
// which is already net of every proven cause — never nested inside it. The
// orchestrator side grows the matching implicit-subtraction arithmetic arm
// (prose_fact_juxtaposition.go, 臂②).
//
// QH2-B caliber-word binding (§29.79 观察续档, 2026-07-15): a report
// paraphrased the seat line's 全额 into 满额 — value zero-loss, so every
// value-membership audit stayed silent while the caliber word (the account
// the number belongs to) silently changed meaning. The seat-composition
// lead therefore binds the caliber words THEMSELVES into the named fact
// (INV-SUPPLY 反压缩导语 discipline, bilingual): the word and its value are
// one quotable unit — 「反转等待(全额) X」/「running 折算 Y」/「…下界」 —
// never re-worded, never swapped for an unpublished near-synonym. The
// published words stay literals (the semantic wording lane lint tracks
// them); the never-published example word and the answer-side caliber audit
// (internal/orchestrator/prose_scalar_caliber_check.go) read the tracefence
// Table ③c single source, and the test-side containment pins tie the two
// faces together.

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
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
	// traceWaitEvidenceCensusPairCap (WAKE-CENSUS §29.58) bounds the census
	// count lines. It matches the engine's own census pair cap, so a single
	// wakeup_chain result always lists whole; only multi-query unions can
	// trim, and the trim folds into the named remainder (never silent).
	traceWaitEvidenceCensusPairCap = 16
	// traceWaitEvidenceSeatCompositionCap (INV-SUPPLY 件② §29.61.11a) bounds
	// the per-seat composition fact lines (帽外具名余数,照 feed 惯例).
	traceWaitEvidenceSeatCompositionCap = 4
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

// traceSeatCompositionFact is one INV-SUPPLY 件② (§29.61.11a 用户确认必需)
// seat-composition named fact: a seated inversion row whose published
// supply-fold deficit dominates its published effective attribution (the
// SAME typed criterion as the display compound word —
// types.TraceSupplyGapDominant over the same two note values, so the feed
// and the report face can never fork). All magnitudes are VERBATIM note
// strings (CR-1 rule, zero recompute); only the thermal cap converts its
// unit (kHz → GHz) via the display layer's own formula.
type traceSeatCompositionFact struct {
	rank             int
	subject          string
	runnable         string // gated_runnable note, verbatim ("" = no runnable component)
	running          string // gated_running_deficit note, verbatim
	deficit          string // supply_fold_deficit_ms note, verbatim
	thermalKHz       int
	thermalWitnessed bool
}

// traceSupplyDeficitFact is one FREQDIR-1 件2 (§29.149 修向②, 2026-07-19)
// supply-fold deficit named fact: a chain-seated rank row whose supply fold
// ran and published a positive deficit, and which the 席位构成 arm above did
// NOT already feed (the composition arm keeps its own inversion+split gates;
// witness 95946: the #1 non-inversion running seat — the owner of the
// 58.320ms deficit — published NO thermal/frequency named fact while the
// inversion seats ➋➌#8 did, and the model absorbed the seat into the
// inversion narrative). The fact publishes ONLY what the seat actually has —
// the deficit and the thermal-cap facts — never a fabricated gated split
// (禁伪造拆分). Magnitudes are VERBATIM note strings (CR-1 rule); only the
// thermal cap converts kHz → GHz via the display layer's own formula.
type traceSupplyDeficitFact struct {
	rank             int
	subject          string
	typeToken        string // the row's own published type token (record.Object)
	deficit          string // supply_fold_deficit_ms note, verbatim
	thermalKHz       int
	thermalWitnessed bool
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
//   - blocked_reason <thread> iowait=<n> count=<n> line=<n> caller=<reason>
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
	// WAKE-CENSUS (§29.58): per-(waker → wakee) whole-inventory counts from
	// the engine's typed wakeup_edge_census records. Per-pair MAX across
	// republications — the whole entry (count + first/last ts) travels
	// together from ONE record, never stitched across publications.
	type wakerCensusFact struct {
		waker, wakee, first, last string
		count                     int
		// WAKE-CENSUS-D 2A (§29.58.4): the typed exit-state split (sleep / D /
		// other-or-unclassified) — partitions count exactly when present;
		// legacy records without the split keep -1 (absence never invents).
		sleepExit, dExit, otherExit int
		// window is the pair's typed selected_window note value; "" when the
		// record carried none or when republications DISAGREE on it — the
		// per-wakee TOTAL lead only sums pairs of one wakee measured over ONE
		// window (cross-window sums are unsound; same-window republication
		// across result scopes is idempotent and stays additive).
		window string
		// targetWakee (修复轮 件2): every publication of this pair carried the
		// per-result target_wakee marker (the wakee IS that result's own
		// analysis target, whose pair set is cap-immune). AND across
		// republications — one unmarked publication drops the completeness
		// authority.
		targetWakee bool
	}
	wakeCensus := map[string]*wakerCensusFact{}
	var wakeCensusOrder []string
	// 修复轮 件3 (2026-07-13): overflow disclosures are per-RESULT facts — a
	// whole-ledger MAX across DIFFERENT results minted a definite-looking but
	// unsound union number. Group by the record-ID scope (the ID prefix
	// before '#', one per published result): MAX within a scope collapses
	// republication; across scopes the numbers are not soundly combinable,
	// so a multi-scope union de-numberizes the remainder line instead.
	wakeCensusOverflowPairsByScope := map[string]int{}
	wakeCensusOverflowEdgesByScope := map[string]int{}
	wakeCensusScopes := map[string]bool{}
	// wakeCensusWindowByScope: each result scope's census window (one window
	// per result by construction) — the TOTAL lead's completeness witness.
	wakeCensusWindowByScope := map[string]string{}
	// seatComps (INV-SUPPLY 件② §29.61.11a): the compound-word seats' typed
	// composition facts, in board seat order.
	var seatComps []traceSeatCompositionFact
	// supplyDeficits (FREQDIR-1 件2 §29.149): the remaining chain seats'
	// supply-fold deficit facts, in board seat order.
	var supplyDeficits []traceSupplyDeficitFact
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
		// ── WAKE-CENSUS (§29.58): per-pair whole-inventory counts ──────────
		if strings.TrimSpace(record.Predicate) == "wakeup_edge_census" {
			wakee := strings.TrimSpace(record.Object)
			count, err := strconv.Atoi(strings.TrimSpace(record.Value))
			if subject == "" || wakee == "" || err != nil || count <= 0 {
				continue
			}
			key := subject + "\x00" + wakee
			// WAKE-CENSUS-D 2A: the split travels WITH its count (whole-entry
			// replacement, never stitched across publications); -1 = the
			// record carried no split note.
			exitSplit := func(noteKey string) int {
				raw := strings.TrimSpace(notes[noteKey])
				if raw == "" {
					return -1
				}
				n, err := strconv.Atoi(raw)
				if err != nil || n < 0 {
					return -1
				}
				return n
			}
			scope := record.ID
			if cut := strings.Index(scope, "#"); cut >= 0 {
				scope = scope[:cut]
			}
			window := strings.TrimSpace(notes[types.TraceNoteKeySelectedWindow])
			targetWakee := strings.TrimSpace(notes[types.TraceNoteKeyWakeupEdgeCensusTargetWakee]) == "true"
			entry, ok := wakeCensus[key]
			if !ok {
				wakeCensus[key] = &wakerCensusFact{
					waker: subject, wakee: wakee, count: count,
					first:       strings.TrimSpace(notes[types.TraceNoteKeyWakeupEdgeCensusFirstTs]),
					last:        strings.TrimSpace(notes[types.TraceNoteKeyWakeupEdgeCensusLastTs]),
					sleepExit:   exitSplit(types.TraceNoteKeyWakeupEdgeCensusSleepExit),
					dExit:       exitSplit(types.TraceNoteKeyWakeupEdgeCensusDExit),
					otherExit:   exitSplit(types.TraceNoteKeyWakeupEdgeCensusOtherExit),
					window:      window,
					targetWakee: targetWakee,
				}
				wakeCensusOrder = append(wakeCensusOrder, key)
			} else {
				if count > entry.count {
					// MAX across republications — replace the WHOLE entry.
					entry.count = count
					entry.first = strings.TrimSpace(notes[types.TraceNoteKeyWakeupEdgeCensusFirstTs])
					entry.last = strings.TrimSpace(notes[types.TraceNoteKeyWakeupEdgeCensusLastTs])
					entry.sleepExit = exitSplit(types.TraceNoteKeyWakeupEdgeCensusSleepExit)
					entry.dExit = exitSplit(types.TraceNoteKeyWakeupEdgeCensusDExit)
					entry.otherExit = exitSplit(types.TraceNoteKeyWakeupEdgeCensusOtherExit)
				}
				if entry.window != window {
					// Republications disagreeing on the measured window: the
					// TOTAL lead may not sum it (cross-window mixture); the
					// pair line itself keeps the MAX union form.
					entry.window = ""
				}
				// 件2: one unmarked publication drops the target authority.
				entry.targetWakee = entry.targetWakee && targetWakee
			}
			// Overflow disclosures ride every census record of a result; MAX
			// within the record's result scope (absence ⇔ 0 ⇔ complete
			// enumeration for that result) — 件3: never MAX-folded across
			// different results.
			wakeCensusScopes[scope] = true
			if window != "" {
				if prev, ok := wakeCensusWindowByScope[scope]; !ok || prev == window {
					wakeCensusWindowByScope[scope] = window
				} else {
					// One result publishes ONE census window; a disagreement is
					// a malformed replay — never let it prove completeness.
					wakeCensusWindowByScope[scope] = "\x00conflict"
				}
			}
			if raw := strings.TrimSpace(notes[types.TraceNoteKeyWakeupEdgeCensusOverflowPairs]); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n > wakeCensusOverflowPairsByScope[scope] {
					wakeCensusOverflowPairsByScope[scope] = n
				}
			}
			if raw := strings.TrimSpace(notes[types.TraceNoteKeyWakeupEdgeCensusOverflowEdges]); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n > wakeCensusOverflowEdgesByScope[scope] {
					wakeCensusOverflowEdgesByScope[scope] = n
				}
			}
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
		// ── INV-SUPPLY 件② (§29.61.11a): seat-composition named facts ──────
		// Seated rank rows only (the board-summary identity: one seat per
		// published result), gated by the SAME typed dominance criterion as
		// the display compound word. Values stay verbatim note strings.
		//
		// 收尾件5 (P3-1, ACCEPTED asymmetry 2026-07-14): the feed additionally
		// requires the gated_running_deficit note (the template's running 折算
		// term cannot render without it), while the display compound word does
		// not — a fold-bearing inversion seat whose gated split never
		// published would wear the word with the feed silent. Both failure
		// directions are honest (the display never claims components it lacks;
		// the feed never fabricates a term), no such seat has been witnessed
		// (the engine mints the gated split wherever it mints the fold on an
		// inversion row), and collapsing the asymmetry would mean fabricating
		// a one-term "composition" — rejected.
		if strings.Contains(record.ID, "#root_cause_rank:") {
			rank, _ := strconv.Atoi(strings.TrimSpace(notes[types.TraceNoteKeyRank]))
			inversion := strings.TrimSpace(notes[types.TraceNoteKeyPriorityInversionCandidate]) == "true" ||
				strings.TrimSpace(record.Object) == "priority_inversion_candidate"
			deficitRaw := strings.TrimSpace(notes[types.TraceNoteKeySupplyFoldDeficitMS])
			effRaw := strings.TrimSpace(notes[types.TraceNoteKeyEffectiveImpactMS])
			foldRan := strings.TrimSpace(notes[types.TraceNoteKeyFoldBasis]) != ""
			runningRaw := strings.TrimSpace(notes[types.TraceNoteKeyGatedRunningDeficit])
			if rank > 0 && inversion && foldRan && runningRaw != "" {
				deficitMS, errD := strconv.ParseFloat(deficitRaw, 64)
				effMS, errE := strconv.ParseFloat(effRaw, 64)
				if errD == nil && errE == nil && types.TraceSupplyGapDominant(deficitMS, effMS) {
					fact := traceSeatCompositionFact{
						rank:     rank,
						subject:  subject,
						runnable: strings.TrimSpace(notes[types.TraceNoteKeyGatedRunnable]),
						running:  runningRaw,
						deficit:  deficitRaw,
					}
					if raw := strings.TrimSpace(notes[types.TraceNoteKeyThermalCapKHz]); raw != "" {
						if khz, err := strconv.Atoi(raw); err == nil && khz > 0 {
							fact.thermalKHz = khz
							fact.thermalWitnessed = strings.TrimSpace(notes[types.TraceNoteKeyThermalCapWitnessed]) == "true"
						}
					}
					dup := false
					for _, have := range seatComps {
						if have == fact {
							dup = true // identical republications collapse
							break
						}
					}
					if !dup {
						seatComps = append(seatComps, fact)
					}
				}
			}
			// ── FREQDIR-1 件2 (§29.149 修向②, 2026-07-19): supply-fold deficit
			// facts for NON-INVERSION chain seats. Witness 95946: the
			// inversion==true gate handed 热限压/缺口 named facts to seats
			// ➋➌#8 while the #1 non-inversion running seat — the owner of the
			// 58.320ms deficit — got nothing, so its supply nature survived
			// only as a buried English summary attribute and the model
			// absorbed the seat into the inversion narrative. The new arm
			// publishes ONLY the facts the seat actually has (deficit +
			// thermal cap), never a fabricated gated split (禁伪造拆分), and
			// keeps silence when the seat published no positive deficit
			// (absence stays absent). Chain seats only — the adjacent channel
			// and the background/⌗ caliber-side lanes never enter (链上 rank
			// 席; PRECISE typed reads: rank int, relevance token, note
			// presence, one float parse).
			//
			// 返工 P1 (双复核 2026-07-19): INVERSION seats are excluded
			// wholesale (!inversion) — their supply narrative belongs to the
			// 席位构成 arm above EXCLUSIVELY. On an inversion row the deficit
			// IS the counted running component of the seat's effective
			// attribution (同源同值, §29.88.12 R5 — which RETIRED the
			// 「独立口径」 word face for that family), so this arm's
			// 「独立折算口径,不与墙钟(全额)值相加」 face would re-mint the
			// retired lie on any inversion seat that merely fails the
			// dominance gate (deficit < 0.5×eff) or lacks the gated-split
			// note. A sub-dominant inversion seat staying silent is the
			// honest outcome, not a gap.
			relevance := strings.TrimSpace(notes[types.TraceNoteKeyChainRelevance])
			chainSeat := rank > 0 && relevance != "adjacent" &&
				relevance != "background" && relevance != "self_caliber_side"
			if chainSeat && !inversion && foldRan && deficitRaw != "" {
				if deficitMS, err := strconv.ParseFloat(deficitRaw, 64); err == nil && deficitMS > 0 {
					fact := traceSupplyDeficitFact{
						rank:      rank,
						subject:   subject,
						typeToken: strings.TrimSpace(record.Object),
						deficit:   deficitRaw,
					}
					if raw := strings.TrimSpace(notes[types.TraceNoteKeyThermalCapKHz]); raw != "" {
						if khz, err := strconv.Atoi(raw); err == nil && khz > 0 {
							fact.thermalKHz = khz
							fact.thermalWitnessed = strings.TrimSpace(notes[types.TraceNoteKeyThermalCapWitnessed]) == "true"
						}
					}
					dup := false
					for _, have := range supplyDeficits {
						if have == fact {
							dup = true // identical republications collapse
							break
						}
					}
					if !dup {
						supplyDeficits = append(supplyDeficits, fact)
					}
				}
			}
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
	if len(subjects) == 0 && len(edges) == 0 && len(wakeCensusOrder) == 0 &&
		len(seatComps) == 0 && len(supplyDeficits) == 0 {
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
	b.WriteString("Measured kernel wait-call-site and wakeup-source evidence for this run (verbatim values). A sched_blocked_reason caller names the kernel call-site/symbol where the scheduler wait was recorded; it does NOT by itself identify the resource, lock object, owner, or holder that the thread waited for. Report the caller symbol verbatim as a wait call-site. Name a waited-on object or holder only when a separate typed relation provides that identity. A share marked cause-unproven has NO kernel-recorded wait call-site, so never invent one for it. Per-thread blocked_reason record counts are keyed by the waiting thread itself, not by the trace line a record happens to print on; a per-caller Σ value is the records' own self-reported delay= field and may include pre-window accumulation. When the question asks WHO woke a thread (and when), answer with the sched_wakeup edge below: its waker thread and its wakeup timestamp. Use the named counts, totals, and remainder shares below verbatim — never re-count listed rows yourself, and never derive a share by adding or subtracting other published values (the published value already IS the share it names).\n")
	if len(selectedSubjects) > 0 {
		b.WriteString("Kernel-recorded wait call-sites (sched_blocked_reason):\n")
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
					seg = "cause-unproven remainder (no blocked_reason call-site backs this share)"
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
					//
					// PROSE-RC 续批 (§29.74 R4 witness, 2026-07-14): with the
					// subtraction lane AND the whole-seat binding lane closed,
					// the re-derivation urge re-routed into MEMBER re-allocation
					// — the prose bound one member segment (1.899) to the
					// fscache proven cause and minted a derived unproven amount
					// 8.534 = 10.433 − 1.899 (the softer "no single member alone
					// is the unproven part" wording was present and bypassed).
					// The membership property therefore rises to an EXPLICIT
					// member-level prohibition in the same imperative family:
					// the members are one indivisible unproven whole — never
					// rebind any single member to a proven cause, never derive a
					// new unproven amount by subtracting member values. A
					// partition/account property statement, never a
					// characterization of any prose (§29.53.2 边界内); bilingual
					// so the quoted answer language cannot lose it.
					memberSeg = fmt.Sprintf(" Its %s member segment(s) are ALL inside the unproven share together — no single member segment alone is the unproven part. These member segments are one indivisible unproven whole: never rebind any single member segment to a caller-named proven cause, and never derive a new unproven amount by subtracting member-segment values from this share or from each other. 这些成员段是不可拆分的未证整体——禁止把任一成员段单独重绑到任何已证原因名下,也禁止用本份额减去成员段值、或成员段之间互减,铸造新的未证量。", fact.members)
				}
				// PROSE-RC-4 臂① (§29.78 第四改道向, 2026-07-14): with the
				// subtraction, whole-seat binding and member re-allocation
				// lanes closed, the re-derivation urge re-routed into NESTING
				// — the prose framed the remainder seat as a TOTAL containing
				// the hmfs caller shares (0.145+0.171) and re-subtracted them
				// to mint "10.117" as the real unproven amount (no explicit
				// equation, so the arithmetic arm stayed dark; the disjoint
				// sentence was bypassed by the total framing). The fourth
				// sister states the OUTSIDE/net account property with the
				// account's own caller symbols named: every caller-named
				// share lies outside this value, which is already net of
				// every proven cause. Account property only (§29.53.2 边界内),
				// bilingual so the quoted answer language cannot lose it.
				var callerNames []string
				seenName := map[string]bool{}
				addCallerName := func(sym string) {
					sym = strings.TrimSpace(sym)
					if sym == "" || sym == "unknown" || seenName[sym] {
						return
					}
					seenName[sym] = true
					callerNames = append(callerNames, sym)
				}
				for _, pf := range f.facts {
					addCallerName(pf.caller)
				}
				for _, sym := range f.censusOrder {
					addCallerName(sym)
				}
				for _, sym := range f.windowCallers {
					addCallerName(sym)
				}
				nameSeg := ""
				if len(callerNames) > 0 {
					shown := callerNames
					if len(shown) > traceWaitEvidenceCallerCap {
						shown = append(append([]string(nil), shown[:traceWaitEvidenceCallerCap]...), "…")
					}
					nameSeg = "(" + strings.Join(shown, ", ") + ")"
				}
				outsideSegEN := " The caller-named shares"
				outsideSegZH := "各已证原因份额"
				if nameSeg != "" {
					outsideSegEN += " " + nameSeg
					outsideSegZH += nameSeg
				}
				outsideSeg := outsideSegEN + " are OUTSIDE this unproven share, never nested inside it — this value is already net of every proven cause, so never treat any caller-named share as contained in this share, and never subtract caller-named share values from this value again to derive a smaller unproven amount. " + outsideSegZH + "均在本未证份额之外、绝非嵌套其中——本值已扣除全部已证原因、本身即净值;禁止把任何已证份额视作包含在本份额之内,也禁止再用本值减去已证份额值,铸造更小的新未证量。"
				// PROSE-RC 收尾件3 (冷读姊妹形, 054419/052947): with the
				// subtraction lane closed, the re-derivation urge re-routed
				// into BINDING — the prose moved the whole remainder seat
				// (value + fold attributes) under the fscache caller's name.
				// The sister partition property closes that direction: the
				// remainder has NO kernel-recorded caller and never belongs
				// under any caller-named proven cause.
				b.WriteString(fmt.Sprintf("  cause-unproven remainder fact for %s: the %scause-unproven share is %sms, and this published value already IS the entire unproven share — use it verbatim. It has NO kernel-recorded caller and must never be attributed to any caller-named proven cause. The caller-named shares cover only the cause-proven part and are disjoint from this remainder (one account split into non-overlapping shares), so never subtract caller-share values from this remainder, and never subtract this remainder from a caller share, to derive a new unproven amount.%s%s\n", subject, stateWord, fact.value, outsideSeg, memberSeg))
			}
		}
		if subjectOverflow > 0 {
			b.WriteString(fmt.Sprintf("- (+%d more threads with blocked_reason evidence; see the measured observations)\n", subjectOverflow))
		}
	}
	if len(seatComps) > 0 {
		// ── INV-SUPPLY 件② (§29.61.11a 用户裁定: 让模型看到构成): each
		// supply-gap-dominant inversion seat's OWN published split as a
		// quotable named fact — the 行3 decomposition is a display face the
		// model never sees (it writes prose BEFORE rendering; 等待对象四跑四
		// 答案 同构病根), so the composition rides the CR-1 dual-stage feed.
		// Verbatim value chain (note strings, zero recompute); only the
		// thermal cap converts kHz → GHz via the display layer's own formula
		// (supplyfold.go THERM appendix, %.2f of khz/1e6 — a unit conversion
		// of ONE typed value, never a derivation).
		sort.SliceStable(seatComps, func(i, j int) bool {
			if seatComps[i].rank != seatComps[j].rank {
				return seatComps[i].rank < seatComps[j].rank
			}
			return seatComps[i].subject < seatComps[j].subject
		})
		// 复放轮强化 (2026-07-14, run-1 witness: the fact reached the finalize
		// dispatch yet the zh prose still compressed to the single type word) —
		// the imperative is BILINGUAL with an explicit anti-compression clause,
		// the EVID-1/PROSE-RC sister-sentence discipline ("bilingual so the
		// quoted answer language cannot lose it").
		// QH2-B (§29.79 观察续档): the caliber words are bound INTO the named
		// fact — the witnessed paraphrase swapped 全额→满额 with the value
		// intact, so the imperative names the word+value pair as the quote
		// unit and the unpublished near-synonym as the concrete wrong form
		// (bilingual, sister-sentence discipline). The published words stay
		// literals so the semantic wording lane lint keeps seeing them
		// (internal/tool/semantic_wording_lint_test.go); only the
		// never-published example interpolates from the tracefence Table ③c
		// single source, and the cross-face containment pins in
		// trace_wait_evidence_summary_test.go tie the literals to the same
		// table.
		neverWord := tracefence.CaliberWordNeverPublishedZH()[0]
		b.WriteString(fmt.Sprintf("Seat composition facts (typed, per-seat published split): each line below is that seat's OWN published composition — when naming that seat's cause, state BOTH factors together (the inversion wait AND the supply-gap/thermal-frequency component); quote each value verbatim and never re-derive, sum across seats, or drop either factor. The caliber word attached to each value is PART of the fact: quote the word and its value together exactly as printed (反转等待(全额) X / running 折算 Y / …下界), and never replace a caliber word with a near-synonym this report does not publish (e.g. %[1]s — the published word is 全额). 叙述下列席位的根因时必须两因并提:优先级反转等待 + 供给缺口/热限压(频点跑慢)成分——按行内构成值逐字引用;禁止把该席压缩为只提「优先级反转」的单因词形。口径词与数值同为具名事实:引用时连词带值整体照抄(「反转等待(全额) X」「running 折算 Y」「…下界」);禁止改写口径词,或以「%[1]s」等未发布近义词替换「全额」等发布词。\n",
			neverWord))
		badges := tracefence.BadgeGlyphs()
		compoundWord := tracefence.InversionCandidateWordZH + "·" + tracefence.SupplyGapDominantWordZH
		for i, fact := range seatComps {
			if i >= traceWaitEvidenceSeatCompositionCap {
				b.WriteString(fmt.Sprintf("- (+%d more supply-gap-dominant seat(s); see the measured observations)\n", len(seatComps)-traceWaitEvidenceSeatCompositionCap))
				break
			}
			seat := fmt.Sprintf("#%d", fact.rank)
			if fact.rank >= 1 && fact.rank <= len(badges) {
				seat = badges[fact.rank-1]
			}
			var terms []string
			if fact.runnable != "" {
				terms = append(terms, "反转等待(全额) "+fact.runnable+"ms")
			}
			terms = append(terms, "running 折算 "+fact.running+"ms")
			paren := "供给缺口 " + fact.deficit + "ms 下界为主"
			if fact.thermalKHz > 0 {
				if fact.thermalWitnessed {
					paren += fmt.Sprintf(",热限压 %.2fGHz", float64(fact.thermalKHz)/1e6)
				} else {
					paren += fmt.Sprintf(",窗内运行于 %.2fGHz(限压原因未见证)", float64(fact.thermalKHz)/1e6)
				}
			}
			b.WriteString(fmt.Sprintf("- 席位构成(%s %s %s): %s(%s)——两因并提,引用勿推导\n",
				seat, fact.subject, compoundWord, strings.Join(terms, " + "), paren))
		}
	}
	// ── FREQDIR-1 件2 (§29.149 修向②, 2026-07-19): the non-inversion chain
	// seats' supply-fold deficit named facts. The 席位构成 coverage filter
	// below (rank+subject) guards the malformed-replay shape only — the same
	// seat republished once WITH and once WITHOUT the inversion marker must
	// not carry both narratives (返工 P1: the collection predicate already
	// excludes every inversion-marked record). The fact carries the deficit
	// with its caliber words embedded IN the string (口径词嵌串防加和: the
	// verbatim-quote discipline then makes the model carry the words with the
	// number) plus the thermal facts the seat actually published — never a
	// fabricated gated split.
	if len(supplyDeficits) > 0 {
		filtered := supplyDeficits[:0]
		for _, fact := range supplyDeficits {
			covered := false
			for _, have := range seatComps {
				if have.rank == fact.rank && have.subject == fact.subject {
					covered = true
					break
				}
			}
			if !covered {
				filtered = append(filtered, fact)
			}
		}
		supplyDeficits = filtered
	}
	if len(supplyDeficits) > 0 {
		sort.SliceStable(supplyDeficits, func(i, j int) bool {
			if supplyDeficits[i].rank != supplyDeficits[j].rank {
				return supplyDeficits[i].rank < supplyDeficits[j].rank
			}
			return supplyDeficits[i].subject < supplyDeficits[j].subject
		})
		b.WriteString("Supply-fold deficit facts (typed, per-seat): each line below is that seat's OWN published compute-supply deficit — a DISCOUNTED (折算) caliber value. Quote the value together with its caliber words exactly as printed; the discounted value never adds to any wall-clock (全额) value, never enters a four-state or cross-seat total, and its repair benefit never sums with other seats' wall-clock benefits. A seat without a line here published no deficit — never derive one. 下列各席的「供给折算缺口」为折算口径具名事实:连口径词与数值整体照抄;折算值不与任何墙钟(全额)值相加、不计入四态合计;未列出的席位即未发布缺口,勿代算。\n")
		badges := tracefence.BadgeGlyphs()
		for i, fact := range supplyDeficits {
			if i >= traceWaitEvidenceSeatCompositionCap {
				b.WriteString(fmt.Sprintf("- (+%d more seat(s) with a published supply-fold deficit; see the measured observations)\n", len(supplyDeficits)-traceWaitEvidenceSeatCompositionCap))
				break
			}
			seat := fmt.Sprintf("#%d", fact.rank)
			if fact.rank >= 1 && fact.rank <= len(badges) {
				seat = badges[fact.rank-1]
			}
			paren := "运行频点非最高"
			if fact.thermalKHz > 0 {
				if fact.thermalWitnessed {
					paren += fmt.Sprintf(",热限压 %.2fGHz", float64(fact.thermalKHz)/1e6)
				} else {
					paren += fmt.Sprintf(",窗内运行于 %.2fGHz(限压原因未见证)", float64(fact.thermalKHz)/1e6)
				}
			}
			head := seat + " " + fact.subject
			if fact.typeToken != "" {
				head += " " + fact.typeToken
			}
			b.WriteString(fmt.Sprintf("- 供给折算(%s): 供给折算缺口 %sms(%s)——独立折算口径,不与墙钟(全额)值相加、不计入四态合计;连口径词与数值整体照抄,勿推导\n",
				head, fact.deficit, paren))
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
	}
	if len(wakeCensusOrder) > 0 {
		// ── WAKE-CENSUS (§29.58) count lane: the engine's whole-inventory
		// per-pair census. Counts fold over each wakeup_chain result's FULL
		// edge set BEFORE any publication cap, so they are exact totals —
		// never lower bounds — and a complete enumeration (zero overflow)
		// carries the absence property outright (PRC-F1: never invent a
		// count for a pair that was never measured).
		entries := make([]*wakerCensusFact, 0, len(wakeCensusOrder))
		for _, key := range wakeCensusOrder {
			entries = append(entries, wakeCensus[key])
		}
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].count != entries[j].count {
				return entries[i].count > entries[j].count
			}
			if entries[i].waker != entries[j].waker {
				return entries[i].waker < entries[j].waker
			}
			return entries[i].wakee < entries[j].wakee
		})
		listed := entries
		// 件3: overflow numbers are exact only within ONE result scope (MAX
		// collapses republication). A multi-scope union keeps the remainder
		// but drops the arithmetic — no definite-looking unsound number.
		singleScope := len(wakeCensusScopes) <= 1
		unlistedPairs, unlistedEdges := 0, 0
		for scope := range wakeCensusScopes {
			unlistedPairs += wakeCensusOverflowPairsByScope[scope]
			unlistedEdges += wakeCensusOverflowEdgesByScope[scope]
		}
		if len(entries) > traceWaitEvidenceCensusPairCap {
			for _, entry := range entries[traceWaitEvidenceCensusPairCap:] {
				unlistedPairs++
				unlistedEdges += entry.count
			}
			listed = entries[:traceWaitEvidenceCensusPairCap]
		}
		// WAKE-CENSUS-D 2A (§29.58.4): the window-total caliber wording (and
		// its STRONGER absence property) engages only on 2A-provenanced
		// entries — every listed pair carrying the typed exit split (count>0
		// partitions into the three buckets, so at least one split note is
		// always present on a 2A record). Legacy/archived census records keep
		// the first-batch observed-edges wording byte-identically: claiming
		// window-total for an edge-fold count would be an over-claim, while
		// the legacy wording under a 2A count only under-claims (fail-open
		// direction — 宁弱勿假).
		windowTotal := true
		for _, entry := range listed {
			if entry.sleepExit < 0 && entry.dExit < 0 && entry.otherExit < 0 {
				windowTotal = false
				break
			}
		}
		if windowTotal {
			// The SCOPE sentence stays: only the chain-thread wakees were
			// counted (范围句保留 — wakees outside the set remain unmeasured).
			b.WriteString("Measured wakeup counts per waker (window-total census: each count below is the total number of raw sched_wakeup rows waking that wakee across its query's whole analysis window — counted directly from the raw event inventory, independently of the causal-chain expansion, not just the edge rows listed above; quote these counts verbatim and never re-count rows yourself. Each arrow keeps the sched_wakeup record's own waker → wakee direction — never reverse a pair's direction. The counted wakee set is the chain's threads (analysis target and chain nodes) — wakees outside that set were not counted):\n")
		} else {
			b.WriteString("Measured wakeup-edge counts per waker (full-inventory census: each count below is the measured total of wakeup edges for that waker → wakee pair across its query's whole analysis window, counted over every measured edge — not just the rows listed above; quote these counts verbatim and never re-count rows yourself. Each arrow keeps the sched_wakeup record's own waker → wakee direction — never reverse a pair's direction. These counts cover measured wakeup edges only, so the raw trace may still hold wakeups outside the measured set):\n")
		}
		for _, entry := range listed {
			var line string
			if windowTotal {
				nonNeg := func(n int) int {
					if n < 0 {
						return 0
					}
					return n
				}
				// WAKE-CENSUS-D 2A: the typed exit split (zero-dropped notes
				// read back as 0 — the three columns partition the count).
				line = fmt.Sprintf("- %s → %s ×%d raw wakeup(s) in the analysis window [exits: sleep=%d, D-state/IO=%d, other/unclassified=%d — measurement facts about which state the wakee left, never causal attribution]",
					entry.waker, entry.wakee, entry.count, nonNeg(entry.sleepExit), nonNeg(entry.dExit), nonNeg(entry.otherExit))
			} else {
				line = fmt.Sprintf("- %s → %s ×%d measured wakeup edge(s)", entry.waker, entry.wakee, entry.count)
			}
			if entry.first != "" && entry.last != "" {
				line += fmt.Sprintf(" (first at %s, last at %s)", entry.first, entry.last)
			}
			b.WriteString(line + "\n")
		}
		// WAKE-CENSUS-D 2A 总数导语 (RANK-U Stage 1 复放实锤 2026-07-13): three
		// replay runs consumed the per-pair counts verbatim yet FABRICATED a
		// derived total (「其余共 12 次」against a true 17 / 「累计 121 次」/
		// 「总计 30 次」against a true 29) — the lane published no quotable
		// total, so the model re-derived one. Counts of ONE wakee's pairs
		// measured by ONE result are additive (count additivity, one wakee,
		// one window), so a complete same-scope enumeration publishes the
		// per-wakee total as a directly quotable line (噪音从源头消除). Gated
		// PRECISELY per wakee: window-total provenance ∧ every pair of the
		// wakee from ONE result scope ∧ that scope complete (zero overflow) ∧
		// no listing-cap trim — a trimmed, mixed-scope or cross-window union
		// never mints a definite-looking partial total.
		if windowTotal && len(listed) == len(entries) {
			// completeWindows: a window is proven COMPLETE when at least one
			// result scope measured exactly that window with ZERO pair-cap
			// overflow (same-window republication is idempotent: counts are
			// deterministic per pair, MAX dedup keeps one copy, and the
			// complete scope's pair set subsumes any overflowed sibling's).
			completeWindows := map[string]bool{}
			for scope, window := range wakeCensusWindowByScope {
				if window != "" && !strings.HasPrefix(window, "\x00") &&
					wakeCensusOverflowPairsByScope[scope] == 0 {
					completeWindows[window] = true
				}
			}
			type wakeeTotal struct {
				wakee, window               string
				pairs, count                int
				sleepExit, dExit, otherExit int
				sound, targetWakee          bool
			}
			totalsByWakee := map[string]*wakeeTotal{}
			var wakeeOrder []string
			nonNeg := func(n int) int {
				if n < 0 {
					return 0
				}
				return n
			}
			for _, entry := range listed {
				acc, ok := totalsByWakee[entry.wakee]
				if !ok {
					acc = &wakeeTotal{wakee: entry.wakee, window: entry.window, sound: entry.window != "", targetWakee: true}
					totalsByWakee[entry.wakee] = acc
					wakeeOrder = append(wakeeOrder, entry.wakee)
				}
				if entry.window == "" || entry.window != acc.window {
					acc.sound = false
				}
				acc.targetWakee = acc.targetWakee && entry.targetWakee
				acc.pairs++
				acc.count += entry.count
				acc.sleepExit += nonNeg(entry.sleepExit)
				acc.dExit += nonNeg(entry.dExit)
				acc.otherExit += nonNeg(entry.otherExit)
			}
			for _, wakee := range wakeeOrder {
				acc := totalsByWakee[wakee]
				if !acc.sound {
					continue
				}
				// Per-wakee completeness: EITHER the window's enumeration is
				// complete (a zero-overflow result measured it), OR every
				// publication of every pair of this wakee carried the
				// per-RESULT target_wakee marker — that wakee's pair set is
				// pair-cap IMMUNE on the engine AND tool faces by construction
				// (件5), so its enumeration is complete even when the scope's
				// non-target pairs overflowed (donghu shape: 83 pairs, 67
				// beyond the cap, all 11 CompThread pairs listed).
				//
				// EVOLUTION RECORD (修复轮 件2, 复核 F1 2026-07-13): the first
				// cut read the SESSION-global anchor flag (threads[].anchor) —
				// a T1 anchor could vouch for a T2 result's TRIMMED pair set
				// and mint a definite-looking partial TOTAL. The authority is
				// now the typed per-result marker only.
				if !completeWindows[acc.window] && !acc.targetWakee {
					continue
				}
				b.WriteString(fmt.Sprintf("- TOTAL for wakee %s in window %s: %d raw wakeup(s) across the %d listed waker pair(s) [exits: sleep=%d, D-state/IO=%d, other/unclassified=%d] — quote this total verbatim; never sum or subtract pair counts yourself.\n",
					acc.wakee, acc.window, acc.count, acc.pairs, acc.sleepExit, acc.dExit, acc.otherExit))
			}
		}
		if windowTotal {
			// 件1 (修复轮, 冷读 RU-F1 2026-07-13): the population PROPERTY
			// sentence — run6 witness: the prose extrapolated a census fact
			// into「整个窗口内所有配对中唯一大于 0 的 d_exit」while the raw
			// window held 38 D-exit pairs BETWEEN out-of-population threads.
			// The window-total caliber is per-COUNTED-WAKEE only; the census
			// says nothing (not even zero) about pairs among uncounted
			// threads. Unconditional in the window-total arm (the overflow
			// shape never reaches the absence sentence below — donghu: 67
			// overflow pairs, and exactly that run over-claimed). Bilingual so
			// the quoted answer language cannot lose the qualifier in
			// translation.
			b.WriteString("- Census population property: this census counts wakeups of the chain-thread wakee set ONLY (the analysis target and the chain-node threads). Pairs BETWEEN threads outside that set were never measured here, so never turn a census fact into a whole-window / all-pairs claim — e.g. never call a listed count \"the only non-zero D-exit pair in the window\", and never claim zero (or uniqueness) for any out-of-population pair: the raw window may hold many wakeup/D-exit pairs between uncounted threads. 本 census 种群=分析目标线程∪链节点线程;种群外线程之间的配对未测量——禁止据此作全窗/全部配对宣称(包括「窗口内唯一」「种群外为零」类)。\n")
		}
		switch {
		case unlistedPairs > 0 && singleScope:
			b.WriteString(fmt.Sprintf("- (+%d more waker → wakee pair(s) carrying %d more measured wakeup edge(s) are not listed here — their per-pair counts are unpublished, so never guess or invent a count for an unlisted pair)\n", unlistedPairs, unlistedEdges))
		case unlistedPairs > 0:
			// Multi-result union: the per-result remainders do not add into
			// one sound number — state the remainder without arithmetic.
			b.WriteString("- (additional measured waker → wakee pairs beyond those listed exist across the combined analyses — their per-pair counts are unpublished, so never guess or invent a count for an unlisted pair)\n")
		case windowTotal:
			// WAKE-CENSUS-D 2A (§29.58.4): with the window-total source the
			// absence property strengthens — a pair absent from a complete
			// enumeration has ZERO raw sched_wakeup rows waking that counted
			// wakee inside that analysis window (no longer merely "never
			// measured with a per-pair count"). The SCOPE sentence stays:
			// wakees outside the chain-thread set were not counted, so no
			// claim exists for them. WC-F1 label extended with the D-causality
			// pointer (双重归因防护: counts say who woke whom how many times,
			// never WHY a D wait ended — blocked_reason owns that lane).
			b.WriteString("- These pairs are the COMPLETE list of counted waker → wakee pairs for the chain-thread wakee set: a pair absent from this list has ZERO raw sched_wakeup rows waking that counted wakee inside its analysis window (window-total caliber), so never report a wakeup count for an absent pair. Wakees OUTSIDE the chain-thread set were not counted — never claim any count, including zero, for them. An absence here is a data-coverage fact about the measured set's scope — it is not a kernel scheduling behavior and needs no mechanism explanation. Counts are measurement-set facts (who woke whom, how many times), never causal attribution — for WHY a D-state/uninterruptible wait happened, read the sched_blocked_reason evidence, not this census.\n")
		default:
			// 件2 (P2-1): the descriptive half claims exactly the census
			// caliber — a bundle run can hold engine-measured edges that
			// published no per-pair count, so "ZERO measured wakeup edges in
			// this run" over-claimed. The normative half is unchanged.
			// WC-F1: absence is a data-coverage fact about the measured set's
			// scope — the closing label blocks the invented-kernel-mechanism
			// explanation lane (the census never states kernel behavior).
			b.WriteString("- These pairs are the COMPLETE list of per-pair counted waker → wakee pairs from the wakeup-chain analyses above: a pair absent from this list was never measured with a per-pair count here, so never report a wakeup count for an absent pair. An absence here reflects only the measured set's scope — it is not a kernel scheduling behavior and needs no mechanism explanation.\n")
		}
	} else if len(edges) > 0 {
		// ── PROSE-RC ①: per-waker observed-edge count facts — FALLBACK lane
		// only (no wakeup_edge_census record reached the ledger; 降级自认).
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

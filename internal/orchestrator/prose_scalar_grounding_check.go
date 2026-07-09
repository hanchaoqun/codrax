package orchestrator

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// prose_scalar_grounding_check.go — PSG batch (§25 ruling b,
// docs/design/real_trace_campaign_20260705.md, 2026-07-08) + PSG-2 second
// arm (§24.14, 2026-07-08).
//
// Background (huadong_01 §22 C-P1): on a runtime-trace run whose typed
// boundary excludes source analysis, the answer carries ZERO citations, so
// the whole citation-quote guard family is structurally bypassed and
// model-authored prose numerals have NO gate at all. The audited specimen
// carried a 46.821ms engine truth that no rendered evidence surface could
// verify, a 1.59ms value with no attributable evidence row, and a doubled
// "8 occurrences in each window" count.
//
// This validator deterministically scans model-authored prose blocks for
// ms / percentage scalars and checks membership against the report's
// evidence-face scalar set:
//
//	accepted aggregate facts (labels, values, dimensions, members)
//	∪ observation-ledger records (values, k=v note numerals, summaries)
//	∪ system-injected runtime projection / facts blocks in the document
//	∪ citation quotes
//
// PSG-2 second arm (§24.14 B-3/D-2, cmp_78_01): membership alone lets a
// value that IS on the evidence face ship under the WRONG window or thread
// — the audited flagship sentence bound a 2250ms-window measurement
// ("sleep 63.6% (1430ms)") under an 1800ms window's name, and a ~50ms wait
// published for one binder thread was stated for a different thread. After
// a token passes membership, this arm audits the SENTENCE BINDING: when
// the same prose unit names a window (endpoint span / length) or a thread
// (name-tid token) next to the scalar, the evidence rows that actually
// carry that scalar must agree with the named window / thread. The precise
// signal is triple co-occurrence ON ONE EVIDENCE ROW (scalar + window
// tokens + thread tokens published together); the sentence-side token
// extraction is noisy, so a mismatch only ever feeds the same soft
// violation kind — and the arm asserts a misbinding ONLY when every
// evidence row carrying the scalar is positively bound to a different
// window / thread. A single carrier row with no window/thread tokens, or
// one carrier row that agrees with the sentence, keeps the token silent
// (宁松勿严). Units with no window/thread tokens are never audited —
// membership stays sufficient there.
//
// The percent-recompute exemption gets the matching extension: a
// percentage grounded ONLY by the a/b×100 recompute arm, in a sentence
// that names a window length, keeps its exemption only if some
// reproducing denominator is either NOT a published window length (an
// ordinary ratio — stays loose) or matches a window the sentence names.
// A percent that recomputes exclusively against a DIFFERENT window's
// length than the one the sentence names is the §24.14 B-3 shape.
//
// PSG-2H (§29.10-2 ruling, docs/design/real_trace_campaign_20260705.md,
// 2026-07-10) — the G13/G14 residual-witness hardening (79+792 双位数
// witness 全表), three layers, none of which is a hard reject:
//
//	① detection gains the THREAD-ENTITY face: prose name-tid tokens are
//	  matched LITERALLY against the evidence-face thread spelling set
//	  (same extractor both sides); a token matching no spelling AND no
//	  published tid is a fabrication finding (witness: dh-irq-bind-4-93,
//	  one character from the real dh-irq-bind-0-89). tid membership
//	  keeps truncated/aliased spellings of a REAL thread silent. The
//	  pairwise-sum grounding arm gains the matching binding extension
//	  (witness: a 139.615-class cross-thread sum stated for one thread).
//	② the single raise stays a FORCED one-round rewrite whose hint names
//	  every violating token individually with a delete-or-replace
//	  directive (upgrade of the §25(b) advisory wording).
//	③ when the forced round is consumed and the SHIPPED document still
//	  carries unlocatable tokens, a deterministic system-channel caveat
//	  lists them verbatim (appendProseScalarResidualCaveatToAnswer at
//	  the ship chokepoint) — the body is never rewritten, the loop
//	  never re-enters. The FRCAP total-round hard cap (§29.12) always
//	  wins conflicts: a cap-preempted raise ships through the residual-
//	  concerns caveat instead, and this lane stays silent (no latch).
//
// Red-line compliance (precise-signals-for-hard-gates): the regex
// extraction is a NOISY signal, so it drives only SOFT guidance — a
// retry hint listing the unmatched / misbound numerals, raised AT MOST
// ONCE per run (see the one-round latch below), never a hard emit-time
// reject. The membership tolerance and the exemption arms are deliberately
// LOOSE (宁松勿严): mis-flagging one legitimate number costs more than
// letting one ungrounded number ship, because the gate's failure mode is
// only a single bounded retry.
//
// One-round latch (anti-livelock, one-shot pattern), SHARED by both arms —
// membership misses and binding mismatches merge into ONE violation of the
// same kind, so the run-level round budget stays exactly one:
//
//	part 1 — when the CURRENT dispatch's retry surface already lists
//	         this kind, the hint has been delivered; mark the sticky
//	         run-level latch and stay silent forever after.
//	part 2 — the sticky latch survives later retry rounds that rebuild
//	         the retry surface without this kind, so the gate can never
//	         raise a second time in one run.
//
// Within ONE finalize attempt the check is repeatable (the auto-repair
// recheck sees the same verdict), because the latch is only set when a
// dispatched retry surface carries the kind — never at raise time.

// proseScalarScanTokenCap bounds how many prose scalar tokens one pass
// examines; proseScalarEvidenceValueCap bounds the numeric evidence pool
// consumed by the tolerance / recompute arms. Both are generous for real
// answers and exist only to bound worst-case work.
const (
	proseScalarScanTokenCap     = 200
	proseScalarEvidenceValueCap = 8192
	proseScalarDetailListCap    = 8

	// PSG-2 binding-arm work bounds and tolerances.
	proseScalarBindingRowCap   = 2048 // evidence rows kept for binding audit
	proseScalarRowValueCap     = 64   // numerals parsed per evidence row
	proseScalarWindowRefCap    = 8    // window identities per text unit
	proseScalarThreadRefCap    = 16   // thread identities per text unit
	proseScalarDenominatorCap  = 32   // recompute denominators collected
	proseScalarWindowSpanTolS  = 0.01 // endpoint tolerance, seconds
	proseScalarWindowMaxSpanMS = 600000

	// PSG-2H thread-entity arm work bounds (§29.10-2, 2026-07-10).
	proseScalarThreadSpellingCap = 8192 // evidence thread spelling set size
	proseScalarThreadScanCap     = 200  // prose thread tokens audited per pass
	proseScalarSumPairCap        = 16   // reproducing (a, b) pairs audited
)

// proseScalarTokenRE captures a decimal numeral immediately followed by an
// ms / percent unit (half- or full-width). Only these two unit families are
// in scope (§25 ruling b); second-denominated timestamps, line numbers,
// tid/pid handles, dates and E# indexes never match because they carry no
// such unit — the exemption set is structural before it is arithmetic.
var proseScalarTokenRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*(ms|毫秒|%|％)`)

// proseScalarNumeralRE extracts every bare numeral from an evidence-side
// text surface (unit-agnostic on purpose — the evidence pool is loose).
var proseScalarNumeralRE = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

// proseScalarWindowSpanRE captures a "start–end s" second-denominated time
// span — the window-identity form the runtime evidence rows publish
// ("查询窗 3680.569–3682.819s") and comparison prose repeats
// ("窗口（3680.819~3682.619s，1800ms）"). The second endpoint MUST carry the
// seconds unit so ms ranges ("1.107–97.342ms") and line ranges never parse
// as windows.
var proseScalarWindowSpanRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*(?:s|秒)?\s*(?:[–—~～－]|\.\.|-)\s*([0-9]+(?:\.[0-9]+)?)\s*(?:s|秒)`)

// proseScalarWindowLengthREs capture an explicit window LENGTH in ms next
// to a window noun. Each pattern's single capture group is the numeral.
// Deliberately conservative: a free-standing "1800ms" with no window noun
// is a measurement, not a window identity.
var proseScalarWindowLengthREs = []*regexp.Regexp{
	// parenthesized window descriptor: 窗口（3680.819~3682.619s，1800ms）
	regexp.MustCompile(`窗口?\s*[（(][^（()）]{0,80}?([0-9]+(?:\.[0-9]+)?)\s*(?:ms|毫秒)\s*[）)]`),
	// genitive pre-noun form: 1800ms 的分析窗 / 2250ms 的窗口
	regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*(?:ms|毫秒)\s*的(?:分析窗|查询窗|时间窗|窗口|窗)`),
	// post-noun form: 分析窗 1645ms / 窗长 1800ms / 窗口: 1800ms
	regexp.MustCompile(`(?:分析窗|查询窗|时间窗|窗长|窗口长度|窗口|窗)\s*[:：=]?\s*(?:约|仅)?\s*([0-9]+(?:\.[0-9]+)?)\s*(?:ms|毫秒)`),
	// english forms: 1800ms window / window of 1800ms / window=1800ms
	regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*ms\s+(?:analysis\s+|query\s+)?window`),
	regexp.MustCompile(`(?i)window(?:\s+(?:of|length))?\s*[:=]\s*([0-9]+(?:\.[0-9]+)?)\s*ms`),
}

// proseScalarThreadTokenRE captures a thread identity token in the
// name-tid form the trace surfaces publish (com.xs.fm.lite-6565,
// binder:8815_1-6581, wk:1/1/0/8-6537, <...>-21674). Boundary and
// name-shape guards live in extractProseScalarThreadRefs.
var proseScalarThreadTokenRE = regexp.MustCompile(`[A-Za-z_<][A-Za-z0-9_.:/<>@\-]*-[0-9]{2,7}`)

// proseScalarToken is one extracted prose scalar occurrence.
type proseScalarToken struct {
	Raw     string // numeral as written, e.g. "46.821"
	Unit    string // "ms" / "毫秒" / "%" / "％"
	BlockID string
	Pos     int // byte offset of the numeral in its text unit
	Value   float64
	Ulp     float64 // one unit in the last written digit
}

func (t proseScalarToken) percent() bool {
	return t.Unit == "%" || t.Unit == "％"
}

func (t proseScalarToken) label() string {
	return t.Raw + t.Unit
}

// proseScalarWindowRef is one window identity parsed from a text surface:
// a second-denominated endpoint span and/or a length in ms.
type proseScalarWindowRef struct {
	StartS   float64
	EndS     float64
	HasSpan  bool
	LengthMS float64 // >0 when known (explicit, or derived from the span)
	Raw      string  // as written, for the violation detail
	Pos      int
}

func (w proseScalarWindowRef) displayLabel() string {
	if w.HasSpan && w.LengthMS > 0 {
		return fmt.Sprintf("%s (≈%.0fms)", w.Raw, w.LengthMS)
	}
	return w.Raw
}

// proseScalarThreadRef is one thread identity token (name-tid).
type proseScalarThreadRef struct {
	Raw string
	TID string
	Pos int
}

// proseScalarEvidenceRow is one row-granular evidence unit for the PSG-2
// binding arm: the numerals, window identities and thread identities that
// were PUBLISHED TOGETHER (one observation record, one line of a system
// evidence block, one table row, one citation quote).
type proseScalarEvidenceRow struct {
	values  []float64
	windows []proseScalarWindowRef
	threads []proseScalarThreadRef
}

// proseScalarEvidenceSet is the numeric membership pool plus the
// row-granular binding index.
type proseScalarEvidenceSet struct {
	exact  map[string]bool
	values []float64 // sorted ascending
	// PSG-2 binding index.
	rows          []proseScalarEvidenceRow
	windowLengths []float64 // every published window length, ms
	// PSG-2H thread-entity face (§29.10-2): every thread identity
	// spelling (verbatim name-tid token) and every tid the evidence
	// surfaces publish. Extracted with the SAME extractor the prose
	// scan uses, so the two faces cannot diverge in shape.
	threadSpellings map[string]bool
	threadTIDs      map[string]bool
}

// proseScalarBindingFinding is one second-arm mismatch, preformatted for
// the violation detail.
type proseScalarBindingFinding struct {
	entry string
}

// proseScalarThreadFinding is one PSG-2H fabricated-thread finding: a
// prose name-tid token that matches no evidence-face thread spelling
// and no evidence-face tid.
type proseScalarThreadFinding struct {
	Raw     string
	BlockID string
}

func (f proseScalarThreadFinding) label() string { return f.Raw }

// runProseScalarGroundingCheck is the PSG §25(b) answer-side gate. It
// returns at most ONE violation naming every unmatched prose scalar
// (membership arm) and every value the prose binds to a different window /
// thread than its evidence rows published it under (PSG-2 binding arm);
// the bus-scoped strict arm (isStrictViolationForBus) makes that single
// raise retry-eligible for exactly one round.
func runProseScalarGroundingCheck(doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState) []types.Violation {
	if doc == nil || bus == nil || mut == nil {
		return nil
	}
	// One-round latch part 1: the current dispatch was born carrying the
	// prose-scalar hint — its output is final for this lane.
	if rs := mut.RetryState(); rs != nil && retryStateListsProseScalarHint(rs) {
		mut.MarkProseScalarGroundingHintDelivered()
		return nil
	}
	// One-round latch part 2: the hint was consumed on an earlier round.
	if mut.ProseScalarGroundingHintDelivered() {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	// Scope gate (typed, precise): only runs whose accepted ledger carries a
	// deterministic runtime-query observation (trace_query family). Ordinary
	// source-code answers keep their numbers under the existing citation /
	// scalar-block guards; this lane exists for the trace evidence plane.
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return nil
	}
	evidence := buildProseScalarEvidenceSet(doc, mut, ledger)
	unmatched, misbound, fabricated := scanProseScalarFindings(doc, evidence)
	if len(unmatched) == 0 && len(misbound) == 0 && len(fabricated) == 0 {
		return nil
	}
	var parts []string
	if len(unmatched) > 0 {
		listed := make([]string, 0, len(unmatched))
		for i, tok := range unmatched {
			if i >= proseScalarDetailListCap {
				listed = append(listed, fmt.Sprintf("(+%d more)", len(unmatched)-proseScalarDetailListCap))
				break
			}
			listed = append(listed, fmt.Sprintf("%s (block %q)", tok.label(), tok.BlockID))
		}
		parts = append(parts, fmt.Sprintf(
			"answer prose states %d numeric value(s) that match nothing in this report's evidence surfaces (measured observation records, structured facts, projection tables, or quoted lines): %s",
			len(unmatched), strings.Join(listed, ", ")))
	}
	if len(misbound) > 0 {
		listed := make([]string, 0, len(misbound))
		for i, f := range misbound {
			if i >= proseScalarDetailListCap {
				listed = append(listed, fmt.Sprintf("(+%d more)", len(misbound)-proseScalarDetailListCap))
				break
			}
			listed = append(listed, f.entry)
		}
		parts = append(parts, fmt.Sprintf(
			"answer prose binds %d numeric value(s) to a different time window or thread than the one their evidence rows published them under: %s",
			len(misbound), strings.Join(listed, "; ")))
	}
	if len(fabricated) > 0 {
		listed := make([]string, 0, len(fabricated))
		for i, f := range fabricated {
			if i >= proseScalarDetailListCap {
				listed = append(listed, fmt.Sprintf("(+%d more)", len(fabricated)-proseScalarDetailListCap))
				break
			}
			listed = append(listed, fmt.Sprintf("%s (block %q)", f.label(), f.BlockID))
		}
		parts = append(parts, fmt.Sprintf(
			"answer prose names %d thread identity token(s) that no evidence surface of this report publishes under that spelling or thread id: %s",
			len(fabricated), strings.Join(listed, ", ")))
	}
	return []types.Violation{{
		Kind:   types.ViolProseScalarUngrounded,
		Detail: strings.Join(parts, "; and "),
		Repair: "go through the listed tokens ONE BY ONE — for EACH listed number, either delete it from the prose or replace it with a value exactly as an evidence surface of this report publishes it (never invent a replacement; when a value is one you derived yourself, name the published values it was derived from); for EACH listed thread identity, either delete it or replace it with a thread spelling exactly as an evidence surface publishes it — never assemble or adjust a thread name or id. When a sentence names the time window or the thread a number belongs to, use the window and thread that number's evidence row was published under — restate the number under its own window and thread, or re-read the value for the window you name; when comparing two windows, normalize each side by its own window length before reading them against each other.",
		Stage:  string(types.StageFinalize),
		ClusterKey: types.IdentityClusterKey("prose_scalar_ungrounded",
			"answer_prose_scalars"),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_document.blocks.prose",
			Reason:     "prose ms/% scalar or thread identity with no evidence-face member, or a value bound to the wrong window/thread",
			Confidence: 0.6,
		},
		RepairLocusOverride: types.LocusFinalizer,
	}}
}

// proseScalarResidualFindingLabels re-runs the PSG scan against a
// document and returns the display labels of everything still
// unlocatable — the PSG-2H (§29.10-2) deterministic caveat backstop
// input. Consumed at answer-ship time, and ONLY on runs where the one
// forced rewrite round was actually delivered (the MutableState latch
// is the typed activation signal), so clean runs pay nothing.
func proseScalarResidualFindingLabels(doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState) []string {
	if doc == nil || bus == nil || mut == nil {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return nil
	}
	evidence := buildProseScalarEvidenceSet(doc, mut, ledger)
	unmatched, misbound, fabricated := scanProseScalarFindings(doc, evidence)
	seen := map[string]bool{}
	var out []string
	add := func(label string) {
		label = strings.TrimSpace(label)
		if label == "" || seen[label] {
			return
		}
		seen[label] = true
		out = append(out, label)
	}
	for _, tok := range unmatched {
		add(tok.label())
	}
	for _, f := range misbound {
		add(f.entry)
	}
	for _, f := range fabricated {
		add(f.label())
	}
	return out
}

// proseScalarResidualCaveatMessage renders the PSG-2H deterministic
// caveat (§29.10-2 layer ③): the run consumed its single forced
// rewrite round and the SHIPPED document still carries unlocatable
// values / thread identities — disclose them verbatim through the
// system channel and never loop again. The body is never rewritten by
// the system. Wording audited against the caveat red lines (no
// internal pipeline vocabulary; CN branch stays CN).
func proseScalarResidualCaveatMessage(lang string, findings []string) string {
	if len(findings) == 0 {
		return ""
	}
	listed := make([]string, 0, len(findings))
	for i, f := range findings {
		if i >= proseScalarDetailListCap {
			break
		}
		listed = append(listed, f)
	}
	extra := len(findings) - len(listed)
	if isChineseLang(lang) {
		msg := "以下数值/实体未能定位于本报告的证据面（观测记录、结构化指标、投影表或引用行），请谨慎采信：" + strings.Join(listed, "；")
		if extra > 0 {
			msg += fmt.Sprintf("；另有 %d 项", extra)
		}
		return msg + "。"
	}
	msg := "The following values/entities in the answer could not be located on this report's evidence surfaces (observation records, structured facts, projection tables, or quoted lines); treat them with caution: " + strings.Join(listed, "; ")
	if extra > 0 {
		msg += fmt.Sprintf(" (+%d more)", extra)
	}
	return msg + "."
}

// appendProseScalarResidualCaveatToAnswer is the ship-time chokepoint
// for the PSG-2H caveat backstop. Activation reads PRECISE typed
// signals only: the run-level latch says the single forced rewrite
// round was delivered, and the re-scan of the SHIPPED document (the
// one in MutableState — every accept, auto-repair, recovery and
// best-draft path writes the shipping doc back there) still finds
// unlocatable tokens. Runs where the gate never fired return the
// answer unchanged, byte-for-byte (正常报告零开销零 caveat).
//
// Livelock impossibility is structural: this helper only APPENDS a
// disclosure at the final assembly point — it raises no violation,
// requests no retry, and the one-round latch already guarantees the
// gate itself can never raise a second time in one run.
//
// Dedup with the cap-preempt lane below: both route through the CAVSTR
// replay register keyed by the verbatim caveat text, so a run where
// the P6 cap branch already disclosed (e.g. a best-draft swap back to
// the PSG-violating round while the latch is ALSO set) never appends
// the disclosure twice.
func (o *Orchestrator) appendProseScalarResidualCaveatToAnswer(answer string) string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return answer
	}
	if !o.busCtx.Mutable.ProseScalarGroundingHintDelivered() {
		return answer
	}
	return o.appendRegisteredAnswerCaveatBullet(answer, o.proseScalarResidualCaveatTextForShippedDoc())
}

// appendProseScalarCapPreemptCaveatToAnswer is the P6-cap-preempt
// disclosure lane (adversarial review P1, 2026-07-10). When the FRCAP
// total-round cap fires BEFORE the PSG forced rewrite round was ever
// dispatched, the latch is cold and the ship-exit lane above stays
// silent BY DESIGN — but the shipping violation set still carries the
// PSG kind, whose registry entry has no caveat family, so the generic
// residual-concerns materializer produced ZERO user-visible words for
// it (fabricated thread names / numerals shipped undisclosed — the
// exact inverse of the §29.10-2 layer-③ promise). This lane is
// latch-INDEPENDENT: the P6 cap branch calls it whenever the selected
// draft's violations contain the PSG kind, re-scanning the SHIPPED
// document with PSG-2H's own generator. Same register key as the
// ship-exit lane → structurally no double disclosure.
func (o *Orchestrator) appendProseScalarCapPreemptCaveatToAnswer(answer string) string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return answer
	}
	return o.appendRegisteredAnswerCaveatBullet(answer, o.proseScalarResidualCaveatTextForShippedDoc())
}

// proseScalarResidualCaveatTextForShippedDoc renders the PSG-2H
// disclosure for the document currently in MutableState (the shipped
// draft on every path, including after an FRCAP best-draft restore).
// Empty when the shipped doc grounds everything.
func (o *Orchestrator) proseScalarResidualCaveatTextForShippedDoc() string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return ""
	}
	mut := o.busCtx.Mutable
	findings := proseScalarResidualFindingLabels(mut.AnswerDocumentV2(), o.busCtx, mut)
	if len(findings) == 0 {
		return ""
	}
	return proseScalarResidualCaveatMessage(o.busCtx.Language, findings)
}

// violationsContainProseScalarKind reports whether the typed violation
// set carries the PSG kind — the precise activation signal for the
// cap-preempt disclosure lane.
func violationsContainProseScalarKind(violations []types.Violation) bool {
	for _, v := range violations {
		if v.Kind == types.ViolProseScalarUngrounded {
			return true
		}
	}
	return false
}

// retryStateListsProseScalarHint reports whether the typed retry surface
// carries the prose-scalar kind — i.e. the dispatch consuming that surface
// received the hint.
func retryStateListsProseScalarHint(rs *types.RetryState) bool {
	if rs == nil {
		return false
	}
	for _, v := range rs.ActiveViolations {
		if v.Kind == types.ViolProseScalarUngrounded {
			return true
		}
	}
	return false
}

// proseScalarGroundingStrictViolation is the bus-scoped strict arm consumed
// by isStrictViolationForBus: the single validator-side raise is
// retry-eligible (the one-round latch guarantees it cannot recur), so the
// kind can stay SoftByDefault at the registry layer per the commercial
// post-emit policy.
func proseScalarGroundingStrictViolation(v types.Violation) bool {
	return v.Kind == types.ViolProseScalarUngrounded
}

// proseScalarSystemEvidenceBlock reports whether a document block is a
// system-injected runtime evidence surface: the EXACT block-id spellings the
// runtime-trace materializers construct (causal projection family, facts /
// metric snapshot / semantic optimizations / perf quality / numbered
// next-step rows). Their numerals feed the evidence pool and their text is
// never scanned as model prose.
//
// Adversarial-review fix (PSG 收尾①(a), 2026-07-08): the first cut used a
// loose HasPrefix("runtime_trace_") — a model-authored lookalike id sharing
// the prefix would have laundered fabricated numbers into the evidence set.
// The gate now reads tool.RuntimeTraceSystemBlockID, the same exact-spelling
// discipline the projection idempotence guard uses. The evidence-FEED face
// must be tight even though the exemption arms are loose: 宁松勿严 governs
// the exemption arms only, never the "model-authored text is not evidence"
// axiom.
func proseScalarSystemEvidenceBlock(blk types.AnswerBlock) bool {
	return tool.RuntimeTraceSystemBlockID(strings.TrimSpace(blk.ID))
}

// proseScalarScanExemptBlock reports whether a block's text stays out of the
// prose scan. Superset of the system evidence surfaces: the bare
// "next_steps" / "next_step" id is ALSO scan-exempt (that id names the
// next-step lane whether system-merged or model-authored — guidance rows,
// not measurement prose; model-authored next_steps blocks are preserved
// content) but, being model-authorable, it is NOT an evidence surface (PSG
// 收尾①(b)): its numerals never ground other prose. The SKIP face may stay
// loose; the FEED face must not.
func proseScalarScanExemptBlock(blk types.AnswerBlock) bool {
	if proseScalarSystemEvidenceBlock(blk) {
		return true
	}
	id := strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(blk.ID)))
	return id == "next_step" || id == "next_steps"
}

// buildProseScalarEvidenceSet assembles the loose numeric membership pool
// and the PSG-2 row-granular binding index. Both consume EXACTLY the same
// evidence faces (§25 enumeration; the feed face stays tight, PSG 收尾①):
// accepted aggregate facts, observation-ledger records, system-injected
// runtime blocks, citation quotes. Window lengths derived from published
// endpoint spans join the pool too, so an honest "the 2250ms window"
// phrasing and a window-share recomputation both stay grounded.
func buildProseScalarEvidenceSet(doc *types.AnswerDocumentV2, mut *types.MutableState, ledger types.ObservationLedger) proseScalarEvidenceSet {
	set := proseScalarEvidenceSet{
		exact:           map[string]bool{},
		threadSpellings: map[string]bool{},
		threadTIDs:      map[string]bool{},
	}
	addText := func(text string) {
		if text == "" || len(set.values) >= proseScalarEvidenceValueCap {
			return
		}
		for _, numeral := range proseScalarNumeralRE.FindAllString(text, -1) {
			if len(set.values) >= proseScalarEvidenceValueCap {
				return
			}
			if set.exact[numeral] {
				continue
			}
			set.exact[numeral] = true
			if f, err := strconv.ParseFloat(numeral, 64); err == nil {
				set.values = append(set.values, f)
			}
		}
	}
	addRow := func(texts ...string) {
		if len(set.rows) >= proseScalarBindingRowCap {
			return
		}
		row := proseScalarEvidenceRow{}
		for _, text := range texts {
			if text == "" {
				continue
			}
			for _, numeral := range proseScalarNumeralRE.FindAllString(text, -1) {
				if len(row.values) >= proseScalarRowValueCap {
					break
				}
				if f, err := strconv.ParseFloat(numeral, 64); err == nil {
					row.values = append(row.values, f)
				}
			}
			row.windows = append(row.windows, extractProseScalarWindowRefs(text)...)
			row.threads = append(row.threads, extractProseScalarThreadRefs(text)...)
		}
		// PSG-2H thread-entity face: every extracted spelling and tid
		// joins the run-level sets BEFORE the per-row cap truncation, so
		// a thread published on a wide row still grounds the prose.
		for _, tref := range row.threads {
			if len(set.threadSpellings) >= proseScalarThreadSpellingCap {
				break
			}
			set.threadSpellings[tref.Raw] = true
			set.threadTIDs[tref.TID] = true
		}
		if len(row.windows) > proseScalarWindowRefCap {
			row.windows = row.windows[:proseScalarWindowRefCap]
		}
		if len(row.threads) > proseScalarThreadRefCap {
			row.threads = row.threads[:proseScalarThreadRefCap]
		}
		if len(row.values) == 0 && len(row.windows) == 0 && len(row.threads) == 0 {
			return
		}
		for _, w := range row.windows {
			if w.LengthMS <= 0 {
				continue
			}
			set.windowLengths = append(set.windowLengths, w.LengthMS)
			// Derived window lengths are pool members too: the length of a
			// published window is published arithmetic, not a model invention.
			if len(set.values) < proseScalarEvidenceValueCap {
				set.values = append(set.values, w.LengthMS)
			}
			if len(row.values) < proseScalarRowValueCap {
				row.values = append(row.values, w.LengthMS)
			}
		}
		set.rows = append(set.rows, row)
	}
	addFacts := func(facts []types.AnswerAggregateFact) {
		for _, fact := range facts {
			addText(fact.Label)
			addText(fact.Value)
			addText(fact.Provenance)
			for _, dim := range fact.Dimensions {
				addText(dim.Value)
			}
			for _, m := range fact.Members {
				addText(m)
			}
			for _, n := range fact.MemberNotes {
				addText(n)
			}
			texts := []string{fact.Label, fact.Value, fact.Provenance}
			for _, dim := range fact.Dimensions {
				texts = append(texts, dim.Value)
			}
			texts = append(texts, fact.Members...)
			texts = append(texts, fact.MemberNotes...)
			addRow(texts...)
		}
	}
	// P-d① lane: the aggregate-facts channel IS an evidence surface — a
	// scalar the investigation carried through emit_investigation_complete
	// (e.g. the huadong 46.821 actual-family value) is a grounded member.
	addFacts(mut.StableInvestigationAggregateFacts())
	if ta := mut.TurnAArtifacts(); ta != nil {
		addFacts(ta.AcceptedAggregateFacts)
	}
	for _, record := range ledger.Records {
		addText(record.Value)
		addText(record.Subject)
		addText(record.Object)
		addText(record.Summary)
		addText(record.RawExcerpt)
		for _, note := range record.RichNotes {
			addText(note)
		}
		for _, term := range record.SurfaceTerms {
			addText(term)
		}
		texts := []string{record.Value, record.Subject, record.Object, record.Summary, record.RawExcerpt}
		texts = append(texts, record.RichNotes...)
		texts = append(texts, record.SurfaceTerms...)
		addRow(texts...)
	}
	if doc != nil {
		for _, blk := range doc.Blocks {
			if !proseScalarSystemEvidenceBlock(blk) {
				continue
			}
			addText(blk.Title)
			addText(blk.Text)
			for _, col := range blk.Columns {
				addText(col)
			}
			for _, it := range blk.Items {
				addText(it.Label)
				addText(it.Text)
				for _, cell := range it.Cells {
					addText(cell)
				}
			}
			// Binding rows at row granularity: the block title (with its
			// column headers) is one row, every text LINE is one row, and
			// every item (label + text + cells published together) is one row.
			addRow(append([]string{blk.Title}, blk.Columns...)...)
			for _, line := range strings.Split(blk.Text, "\n") {
				addRow(line)
			}
			for _, it := range blk.Items {
				texts := []string{it.Label, it.Text}
				texts = append(texts, it.Cells...)
				addRow(texts...)
			}
		}
		for _, cite := range doc.Citations {
			addText(cite.Quote)
			addRow(cite.Quote)
		}
	}
	// Deliberately NOT an evidence surface (PSG 收尾①(c), 2026-07-08):
	// recovered display attachments. Their producer is the rejected-draft
	// recovery path — model-authored text that failed structured
	// validation — so feeding them here would let a rejected draft launder
	// its own fabricated numbers into the very set that grounds the retry.
	// They are also outside the §25 ruling's enumerated evidence faces.
	sort.Float64s(set.values)
	return set
}

// scanProseScalarFindings walks the model-authored prose surfaces and
// returns every ms/% token the evidence pool cannot account for (membership
// arm), every grounded token whose same-unit window / thread naming
// contradicts the evidence rows that published it (PSG-2 binding arm), and
// every prose thread identity token that matches no evidence-face thread
// spelling and no evidence-face tid (PSG-2H entity arm, §29.10-2 — the
// dh-irq-bind-4-93 fabrication shape, one character away from the real
// dh-irq-bind-0-89).
func scanProseScalarFindings(doc *types.AnswerDocumentV2, evidence proseScalarEvidenceSet) (unmatched []proseScalarToken, misbound []proseScalarBindingFinding, fabricated []proseScalarThreadFinding) {
	scanned := 0
	threadScanned := 0
	seenFabricated := map[string]bool{}
	scan := func(blockID, text string) {
		if text == "" {
			return
		}
		if scanned >= proseScalarScanTokenCap && threadScanned >= proseScalarThreadScanCap {
			return
		}
		toks := extractProseScalarTokens(blockID, text)
		var sentWindows []proseScalarWindowRef
		var sentThreads []proseScalarThreadRef
		if len(toks) > 0 || len(evidence.threadSpellings) > 0 {
			sentWindows = extractProseScalarWindowRefs(text)
			sentThreads = extractProseScalarThreadRefs(text)
		}
		// PSG-2H entity arm: audited independently of scalars — the
		// fabricated-thread witness sentence needs no number nearby.
		// Activation requires the evidence face to publish at least one
		// thread token (an inventory to be absent FROM); tid membership
		// keeps truncated / aliased spellings of a REAL thread silent
		// (same tid-equality agreement the binding arm uses).
		if len(evidence.threadSpellings) > 0 {
			for _, tref := range sentThreads {
				if threadScanned >= proseScalarThreadScanCap {
					break
				}
				threadScanned++
				if evidence.threadSpellings[tref.Raw] || evidence.threadTIDs[tref.TID] {
					continue
				}
				if !proseScalarThreadNameAuditable(tref.Raw) {
					continue
				}
				if seenFabricated[tref.Raw] {
					continue
				}
				seenFabricated[tref.Raw] = true
				fabricated = append(fabricated, proseScalarThreadFinding{Raw: tref.Raw, BlockID: blockID})
			}
		}
		if len(toks) == 0 {
			return
		}
		for _, tok := range toks {
			if scanned >= proseScalarScanTokenCap {
				return
			}
			scanned++
			verdict := proseScalarTokenGrounding(tok, evidence)
			if !verdict.grounded {
				unmatched = append(unmatched, tok)
				continue
			}
			// PSG-2 binding arm: only audited when the unit positively
			// names a window or a thread (句内无窗/主体 token 时不核 —
			// membership stays sufficient for unbound sentences).
			if len(sentWindows) == 0 && len(sentThreads) == 0 {
				continue
			}
			if tok.Value == 0 {
				continue // zero values carry no measurement claim
			}
			carriers := proseScalarCarrierRows(evidence.rows, tok)
			if len(carriers) > 0 {
				if len(sentWindows) > 0 {
					if published, bad := proseScalarWindowBindingMismatch(carriers, sentWindows); bad {
						misbound = append(misbound, proseScalarBindingFinding{entry: fmt.Sprintf(
							"%s (block %q) is published under window %s but the prose binds it to window %s",
							tok.label(), tok.BlockID, published,
							proseScalarNearestWindowLabel(sentWindows, tok))})
					}
				}
				if len(sentThreads) > 0 {
					if published, bad := proseScalarThreadBindingMismatch(carriers, sentThreads); bad {
						misbound = append(misbound, proseScalarBindingFinding{entry: fmt.Sprintf(
							"%s (block %q) is published for thread %s but the prose binds it to thread %s",
							tok.label(), tok.BlockID, published,
							proseScalarNearestThreadLabel(sentThreads, tok))})
					}
				}
				continue
			}
			// Percent-recompute extension (§24.14 B-3): a self-derived
			// percentage in a window-naming sentence must recompute against
			// a window the sentence names (or against a non-window pair).
			if tok.percent() && verdict.recomputeOnly && len(sentWindows) > 0 {
				if published, bad := proseScalarRecomputeWindowMismatch(verdict.denominatorsMS, evidence.windowLengths, sentWindows); bad {
					misbound = append(misbound, proseScalarBindingFinding{entry: fmt.Sprintf(
						"%s (block %q) recomputes only against window %s but the prose states it for window %s",
						tok.label(), tok.BlockID, published,
						proseScalarNearestWindowLabel(sentWindows, tok))})
				}
			}
			// PSG-2H sum extension (§29.8 witness: a 139.615-class
			// cross-thread SUM stated for one named thread). A token
			// grounded ONLY by the pairwise-sum arm, in a thread-naming
			// sentence, is misbound when EVERY reproducing pair is
			// positively published for other threads.
			if !tok.percent() && verdict.sumOnly && len(sentThreads) > 0 {
				if published, bad := proseScalarSumThreadMismatch(evidence.rows, verdict.sumPairs, sentThreads, proseScalarTokenTol(tok)); bad {
					misbound = append(misbound, proseScalarBindingFinding{entry: fmt.Sprintf(
						"%s (block %q) is a sum of values published for thread(s) %s but the prose states it for thread %s",
						tok.label(), tok.BlockID, published,
						proseScalarNearestThreadLabel(sentThreads, tok))})
				}
			}
		}
	}
	for _, blk := range doc.Blocks {
		if proseScalarScanExemptBlock(blk) {
			continue
		}
		// Caveat blocks are honesty markers and diagram bodies have their
		// own validators — both stay out of this lane.
		if blk.Kind == types.BlockCaveat || blk.Kind == types.BlockDiagram {
			continue
		}
		scan(blk.ID, blk.Title)
		scan(blk.ID, blk.Text)
		for _, it := range blk.Items {
			scan(blk.ID, it.Label)
			scan(blk.ID, it.Text)
			for _, cell := range it.Cells {
				scan(blk.ID, cell)
			}
		}
	}
	return unmatched, misbound, fabricated
}

// proseScalarThreadNameAuditable is the entity-arm noise dampener: the
// name part must either carry a thread-namespace separator (: / . _ <
// > @) or be at least 4 characters, so short hyphenated technical
// terms ("x86-64", "sha-256", "arm-64") never enter the audit. The
// dampener LOOSENS detection only (a missed short fabrication is a
// sanctioned cost); it never tightens it.
func proseScalarThreadNameAuditable(raw string) bool {
	dash := strings.LastIndexByte(raw, '-')
	if dash <= 0 {
		return false
	}
	name := raw[:dash]
	if strings.ContainsAny(name, ":/._<>@") {
		return true
	}
	return len(name) >= 4
}

// proseScalarSumThreadMismatch audits the sum-only grounding of a token
// in a thread-naming sentence. It asserts a misbinding ONLY in the
// precise all-pairs form: every reproducing (a, b) pair has carrier
// rows on BOTH sides, every one of those rows names at least one
// thread, and no row tid intersects the sentence tids. Any silent side
// (a summand with no carrier row, e.g. a derived window length), any
// thread-silent row, any tid agreement, or a pair set that overflowed
// its collection cap keeps the token silent (宁松勿严).
func proseScalarSumThreadMismatch(rows []proseScalarEvidenceRow, pairs [][2]float64, sentThreads []proseScalarThreadRef, tol float64) (string, bool) {
	if len(pairs) == 0 || len(pairs) > proseScalarSumPairCap {
		return "", false
	}
	var publishing []proseScalarThreadRef
	for pi, pair := range pairs {
		for _, side := range pair {
			carriers := proseScalarCarrierRowsForValue(rows, side, tol)
			if len(carriers) == 0 {
				return "", false
			}
			for _, row := range carriers {
				if len(row.threads) == 0 {
					return "", false
				}
				for _, rt := range row.threads {
					for _, st := range sentThreads {
						if rt.TID == st.TID {
							return "", false
						}
					}
				}
				if pi == 0 {
					publishing = append(publishing, row.threads...)
				}
			}
		}
	}
	published := proseScalarThreadListLabel(publishing)
	return published, published != ""
}

// proseScalarCarrierRowsForValue is the value-keyed variant of
// proseScalarCarrierRows used by the sum extension.
func proseScalarCarrierRowsForValue(rows []proseScalarEvidenceRow, value, tol float64) []proseScalarEvidenceRow {
	var out []proseScalarEvidenceRow
	for _, row := range rows {
		for _, v := range row.values {
			if math.Abs(v-value) <= tol+1e-9 {
				out = append(out, row)
				break
			}
		}
	}
	return out
}

// extractProseScalarTokens applies the unit-scoped regex with manual
// word-boundary checks (a numeral glued to a preceding letter/digit/dot is
// an identifier fragment, and "ms" glued to a following letter is a word).
func extractProseScalarTokens(blockID, text string) []proseScalarToken {
	var out []proseScalarToken
	for _, m := range proseScalarTokenRE.FindAllStringSubmatchIndex(text, -1) {
		start := m[0]
		if start > 0 {
			prev := text[start-1]
			// ',' guards thousands-separated forms ("1,234.5ms"): the
			// numeral regex has no comma arm, so extracting the post-comma
			// fragment ("234.5") would misread the number — skipping the
			// token entirely is the loose, safe disposition (PSG 收尾③).
			if prev == '.' || prev == '-' || prev == '_' || prev == ',' ||
				(prev >= '0' && prev <= '9') ||
				(prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') {
				continue
			}
		}
		unit := text[m[4]:m[5]]
		if unit == "ms" {
			end := m[5]
			if end < len(text) {
				next := text[end]
				if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') ||
					next == '_' {
					continue
				}
			}
		}
		raw := text[m[2]:m[3]]
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		out = append(out, proseScalarToken{
			Raw:     raw,
			Unit:    unit,
			BlockID: blockID,
			Pos:     m[2],
			Value:   value,
			Ulp:     proseScalarUlp(raw),
		})
	}
	return out
}

// extractProseScalarWindowRefs parses the window identities a text unit
// names: endpoint spans (with the derived length) and explicit ms lengths
// next to a window noun. Noisy by design — window refs only ever drive the
// SOFT binding audit, never membership.
func extractProseScalarWindowRefs(text string) []proseScalarWindowRef {
	if text == "" {
		return nil
	}
	var out []proseScalarWindowRef
	for _, m := range proseScalarWindowSpanRE.FindAllStringSubmatchIndex(text, -1) {
		if len(out) >= proseScalarWindowRefCap {
			return out
		}
		if m[0] > 0 {
			prev := text[m[0]-1]
			if prev == '.' || prev == '-' || (prev >= '0' && prev <= '9') {
				continue
			}
		}
		if m[1] < len(text) {
			next := text[m[1]]
			if (next >= '0' && next <= '9') || (next >= 'a' && next <= 'z') ||
				(next >= 'A' && next <= 'Z') {
				continue
			}
		}
		start, err1 := strconv.ParseFloat(text[m[2]:m[3]], 64)
		end, err2 := strconv.ParseFloat(text[m[4]:m[5]], 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		lengthMS := (end - start) * 1000
		if lengthMS <= 0 || lengthMS > proseScalarWindowMaxSpanMS {
			continue
		}
		out = append(out, proseScalarWindowRef{
			StartS: start, EndS: end, HasSpan: true, LengthMS: lengthMS,
			Raw: text[m[0]:m[1]], Pos: m[0],
		})
	}
	for _, re := range proseScalarWindowLengthREs {
		for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
			if len(out) >= proseScalarWindowRefCap {
				return out
			}
			v, err := strconv.ParseFloat(text[m[2]:m[3]], 64)
			if err != nil || v <= 0 || v > proseScalarWindowMaxSpanMS {
				continue
			}
			out = append(out, proseScalarWindowRef{
				LengthMS: v,
				Raw:      text[m[2]:m[3]] + "ms",
				Pos:      m[2],
			})
		}
	}
	return out
}

// extractProseScalarThreadRefs parses name-tid thread identity tokens with
// boundary guards and a name-shape guard (the name part must look like a
// thread name — at least one lowercase letter or a namespace separator —
// so "SHA-256"-style all-caps acronyms never parse as threads).
func extractProseScalarThreadRefs(text string) []proseScalarThreadRef {
	if text == "" {
		return nil
	}
	var out []proseScalarThreadRef
	for _, m := range proseScalarThreadTokenRE.FindAllStringIndex(text, -1) {
		if len(out) >= proseScalarThreadRefCap {
			return out
		}
		if m[0] > 0 {
			prev := text[m[0]-1]
			if prev == '-' || prev == '.' || prev == '_' || prev == ':' || prev == '/' ||
				(prev >= '0' && prev <= '9') ||
				(prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') {
				continue
			}
		}
		if m[1] < len(text) {
			next := text[m[1]]
			if next == '_' || (next >= '0' && next <= '9') ||
				(next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') {
				continue
			}
		}
		tok := text[m[0]:m[1]]
		dash := strings.LastIndexByte(tok, '-')
		if dash <= 0 {
			continue
		}
		name, tid := tok[:dash], tok[dash+1:]
		if !strings.ContainsAny(name, "abcdefghijklmnopqrstuvwxyz_.:/<>@") {
			continue
		}
		out = append(out, proseScalarThreadRef{Raw: tok, TID: tid, Pos: m[0]})
	}
	return out
}

// proseScalarTokenTol is the shared membership / carrier tolerance: one
// unit in the token's last written digit, floored at the round-3 band so
// "46.821" still accepts an evidence "46.8213".
func proseScalarTokenTol(tok proseScalarToken) float64 {
	tol := tok.Ulp
	if tol < 0.0005 {
		tol = 0.0005
	}
	return tol
}

// proseScalarCarrierRows returns the evidence rows that carry the token's
// value (within the same tolerance the membership arm uses). This is the
// PRECISE half of the binding audit: a scalar and a window/thread token
// co-occurring on one published row is the provenance signal.
func proseScalarCarrierRows(rows []proseScalarEvidenceRow, tok proseScalarToken) []proseScalarEvidenceRow {
	tol := proseScalarTokenTol(tok)
	var out []proseScalarEvidenceRow
	for _, row := range rows {
		for _, v := range row.values {
			if math.Abs(v-tok.Value) <= tol+1e-9 {
				out = append(out, row)
				break
			}
		}
	}
	return out
}

// proseScalarWindowBindingMismatch asserts a window misbinding ONLY in the
// precise all-rows form: every carrier row names at least one window and
// none of those windows agrees with any window the sentence names. Any
// window-silent carrier row, or any single agreement, keeps the token
// silent (宁松勿严).
func proseScalarWindowBindingMismatch(carriers []proseScalarEvidenceRow, sentWindows []proseScalarWindowRef) (string, bool) {
	published := ""
	for _, row := range carriers {
		if len(row.windows) == 0 {
			return "", false
		}
		for _, rw := range row.windows {
			for _, sw := range sentWindows {
				if proseScalarWindowsMatch(rw, sw) {
					return "", false
				}
			}
		}
		if published == "" {
			published = proseScalarWindowListLabel(row.windows)
		}
	}
	return published, published != ""
}

// proseScalarThreadBindingMismatch is the thread-facet analog: every
// carrier row names at least one thread and no row tid intersects the
// sentence tids (tid equality, so a truncated or aliased thread NAME for
// the same tid still counts as agreement).
func proseScalarThreadBindingMismatch(carriers []proseScalarEvidenceRow, sentThreads []proseScalarThreadRef) (string, bool) {
	published := ""
	for _, row := range carriers {
		if len(row.threads) == 0 {
			return "", false
		}
		for _, rt := range row.threads {
			for _, st := range sentThreads {
				if rt.TID == st.TID {
					return "", false
				}
			}
		}
		if published == "" {
			published = proseScalarThreadListLabel(row.threads)
		}
	}
	return published, published != ""
}

// proseScalarRecomputeWindowMismatch implements the §24.14 percent
// extension: when EVERY reproducing denominator is itself a published
// window length and NONE matches a window length the sentence names, the
// recompute exemption is a cross-window disguise. A single denominator
// that is not a known window length (an ordinary ratio) keeps the
// exemption — coincidence pairs stay a sanctioned loose cost (§25.1).
func proseScalarRecomputeWindowMismatch(denominatorsMS, windowLengths []float64, sentWindows []proseScalarWindowRef) (string, bool) {
	if len(denominatorsMS) == 0 || len(windowLengths) == 0 {
		return "", false
	}
	var sentLens []float64
	for _, sw := range sentWindows {
		if sw.LengthMS > 0 {
			sentLens = append(sentLens, sw.LengthMS)
		}
	}
	if len(sentLens) == 0 {
		return "", false
	}
	published := ""
	for _, d := range denominatorsMS {
		known := false
		for _, w := range windowLengths {
			if math.Abs(d-w) <= proseScalarWindowLengthTol(w) {
				known = true
				break
			}
		}
		if !known {
			return "", false
		}
		for _, s := range sentLens {
			if math.Abs(d-s) <= proseScalarWindowLengthTol(math.Max(d, s)) {
				return "", false
			}
		}
		if published == "" {
			published = fmt.Sprintf("≈%.0fms", d)
		}
	}
	return published, published != ""
}

// proseScalarWindowsMatch reports whether a row-published window agrees
// with a sentence-named window: matching endpoints, a row occurrence span
// contained inside the stated window, or matching lengths. Every arm errs
// toward agreement.
func proseScalarWindowsMatch(rw, sw proseScalarWindowRef) bool {
	if rw.HasSpan && sw.HasSpan {
		if math.Abs(rw.StartS-sw.StartS) <= proseScalarWindowSpanTolS &&
			math.Abs(rw.EndS-sw.EndS) <= proseScalarWindowSpanTolS {
			return true
		}
		// A measurement published for a sub-interval may be stated inside
		// the larger window that contains it.
		if rw.StartS >= sw.StartS-proseScalarWindowSpanTolS &&
			rw.EndS <= sw.EndS+proseScalarWindowSpanTolS {
			return true
		}
	}
	if rw.LengthMS > 0 && sw.LengthMS > 0 &&
		math.Abs(rw.LengthMS-sw.LengthMS) <= proseScalarWindowLengthTol(math.Max(rw.LengthMS, sw.LengthMS)) {
		return true
	}
	return false
}

// proseScalarWindowLengthTol is the loose length tolerance: 1% of the
// window, floored at 1ms (so a stated 1645 accepts the derived 1644.940).
func proseScalarWindowLengthTol(lengthMS float64) float64 {
	tol := 0.01 * lengthMS
	if tol < 1 {
		tol = 1
	}
	return tol
}

func proseScalarWindowListLabel(windows []proseScalarWindowRef) string {
	labels := make([]string, 0, 2)
	for _, w := range windows {
		if len(labels) >= 2 {
			break
		}
		labels = append(labels, w.displayLabel())
	}
	return strings.Join(labels, " / ")
}

func proseScalarThreadListLabel(threads []proseScalarThreadRef) string {
	labels := make([]string, 0, 2)
	for _, t := range threads {
		if len(labels) >= 2 {
			break
		}
		labels = append(labels, t.Raw)
	}
	return strings.Join(labels, " / ")
}

// proseScalarNearestWindowLabel lists the sentence's window identities
// nearest to the token (up to two), so the detail names the binding the
// reader actually sees next to the number.
func proseScalarNearestWindowLabel(sentWindows []proseScalarWindowRef, tok proseScalarToken) string {
	ordered := make([]proseScalarWindowRef, len(sentWindows))
	copy(ordered, sentWindows)
	sort.SliceStable(ordered, func(i, j int) bool {
		return proseScalarPosDistance(ordered[i].Pos, tok.Pos) < proseScalarPosDistance(ordered[j].Pos, tok.Pos)
	})
	return proseScalarWindowListLabel(ordered)
}

func proseScalarNearestThreadLabel(sentThreads []proseScalarThreadRef, tok proseScalarToken) string {
	if len(sentThreads) == 0 {
		return ""
	}
	nearest := sentThreads[0]
	for _, t := range sentThreads[1:] {
		if proseScalarPosDistance(t.Pos, tok.Pos) < proseScalarPosDistance(nearest.Pos, tok.Pos) {
			nearest = t
		}
	}
	return nearest.Raw
}

func proseScalarPosDistance(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// proseScalarUlp returns one unit in the last written digit: "46.821" →
// 0.001, "60.8" → 0.1, "300" → 1.
func proseScalarUlp(raw string) float64 {
	if dot := strings.IndexByte(raw, '.'); dot >= 0 {
		return math.Pow(10, -float64(len(raw)-dot-1))
	}
	return 1
}

// proseScalarGroundingVerdict reports how a token was grounded.
// recomputeOnly marks tokens grounded EXCLUSIVELY by the a/b×100 recompute
// arm; denominatorsMS carries the reproducing denominators (ms scale) the
// §24.14 percent extension audits. sumOnly marks ms tokens grounded
// EXCLUSIVELY by the pairwise-sum arm; sumPairs carries the reproducing
// (a, b) pairs the PSG-2H cross-thread sum extension audits.
type proseScalarGroundingVerdict struct {
	grounded       bool
	recomputeOnly  bool
	denominatorsMS []float64
	sumOnly        bool
	sumPairs       [][2]float64
}

// proseScalarTokenGrounding runs the tolerance and exemption arms, loosest
// first. Every arm errs toward acceptance (宁松勿严).
func proseScalarTokenGrounding(tok proseScalarToken, evidence proseScalarEvidenceSet) proseScalarGroundingVerdict {
	// Exemption: zero values carry no measurement claim.
	if tok.Value == 0 {
		return proseScalarGroundingVerdict{grounded: true}
	}
	// Arm 1 — verbatim numeral string appears on an evidence surface.
	if evidence.exact[tok.Raw] {
		return proseScalarGroundingVerdict{grounded: true}
	}
	tol := proseScalarTokenTol(tok)
	// Arm 2 — direct member within tolerance.
	if proseScalarNear(evidence.values, tok.Value, tol) {
		return proseScalarGroundingVerdict{grounded: true}
	}
	if tok.percent() {
		// Arm 3 — the evidence carries the same percentage as a 0-1 ratio.
		if proseScalarNear(evidence.values, tok.Value/100, tol/100) {
			return proseScalarGroundingVerdict{grounded: true}
		}
		// Arm 4 — recomputable percentage: some evidence pair (a, b)
		// reproduces the value as a/b at percent scale. k=100 covers a
		// same-unit pair; k=0.1 covers an ms numerator over a
		// seconds-denominated window (denominators rescaled to ms).
		var denoms []float64
		denoms = proseScalarRatioDenominatorsMS(evidence.values, tok.Value, 100, tol, 1, denoms)
		denoms = proseScalarRatioDenominatorsMS(evidence.values, tok.Value, 0.1, tol, 1000, denoms)
		if len(denoms) > 0 {
			return proseScalarGroundingVerdict{grounded: true, recomputeOnly: true, denominatorsMS: denoms}
		}
		return proseScalarGroundingVerdict{}
	}
	// Arm 5 (ms only) — pairwise sum of two evidence values: an honest
	// self-derived total of two published figures is not a fabrication.
	// The reproducing pairs feed the PSG-2H cross-thread sum extension
	// (a capped overflow keeps the extension silent, 宁松勿严).
	if pairs := proseScalarSumPairs(evidence.values, tok.Value, tol); len(pairs) > 0 {
		return proseScalarGroundingVerdict{grounded: true, sumOnly: true, sumPairs: pairs}
	}
	return proseScalarGroundingVerdict{}
}

// proseScalarNear reports whether sorted values contains a member within
// tol of target.
func proseScalarNear(values []float64, target, tol float64) bool {
	idx := sort.SearchFloat64s(values, target-tol)
	return idx < len(values) && values[idx] <= target+tol+1e-9
}

// proseScalarSumPairs collects the reproducing (a, b) pairs (a ≤ b,
// deduplicated on a) for the pairwise-sum arm. Collection stops at
// proseScalarSumPairCap+1 — the consumer treats an overflowed set as
// unauditable and stays silent, so capping can only LOOSEN the sum
// extension, never produce a false positive.
func proseScalarSumPairs(values []float64, target, tol float64) [][2]float64 {
	var out [][2]float64
	prev := math.Inf(-1)
	for _, v := range values {
		if v >= target+tol {
			break
		}
		if v == prev {
			continue
		}
		prev = v
		if v > target-v+tol {
			break // beyond the midpoint every pair is a mirror of an earlier one
		}
		if proseScalarNear(values, target-v, tol) {
			out = append(out, [2]float64{v, target - v})
			if len(out) > proseScalarSumPairCap {
				return out
			}
		}
	}
	return out
}

// proseScalarRatioDenominatorsMS collects every denominator b (rescaled by
// msScale) for which some pair (a, b) of sorted values with b > 0 satisfies
// |a/b*k - target| <= tol. A non-empty result grounds the percent (the old
// boolean recompute arm); the collected denominators feed the §24.14
// percent extension.
func proseScalarRatioDenominatorsMS(values []float64, target, k, tol, msScale float64, out []float64) []float64 {
	if target <= 0 || len(out) >= proseScalarDenominatorCap {
		return out
	}
	for _, a := range values {
		if a <= 0 {
			continue
		}
		// b must land in [a*k/(target+tol), a*k/(target-tol)].
		lo := a * k / (target + tol)
		hi := math.Inf(1)
		if target-tol > 0 {
			hi = a * k / (target - tol)
		}
		for idx := sort.SearchFloat64s(values, lo-1e-9); idx < len(values) && values[idx] <= hi+1e-9; idx++ {
			b := values[idx]
			if b <= 0 {
				continue
			}
			scaled := b * msScale
			dup := false
			for _, have := range out {
				if have == scaled {
					dup = true
					break
				}
			}
			if !dup {
				out = append(out, scaled)
				if len(out) >= proseScalarDenominatorCap {
					return out
				}
			}
		}
	}
	return out
}

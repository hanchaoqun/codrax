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
// docs/design/real_trace_campaign_20260705.md, 2026-07-08).
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
// Red-line compliance (precise-signals-for-hard-gates): the regex
// extraction is a NOISY signal, so it drives only SOFT guidance — a
// retry hint listing the unmatched numerals, raised AT MOST ONCE per run
// (see the one-round latch below), never a hard emit-time reject. The
// membership tolerance and the exemption arms are deliberately LOOSE
// (宁松勿严): mis-flagging one legitimate number costs more than letting
// one ungrounded number ship, because the gate's failure mode is only a
// single bounded retry.
//
// One-round latch (anti-livelock, one-shot pattern):
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

// proseScalarToken is one extracted prose scalar occurrence.
type proseScalarToken struct {
	Raw     string // numeral as written, e.g. "46.821"
	Unit    string // "ms" / "毫秒" / "%" / "％"
	BlockID string
	Value   float64
	Ulp     float64 // one unit in the last written digit
}

func (t proseScalarToken) percent() bool {
	return t.Unit == "%" || t.Unit == "％"
}

// proseScalarEvidenceSet is the numeric membership pool.
type proseScalarEvidenceSet struct {
	exact  map[string]bool
	values []float64 // sorted ascending, deduped
}

// runProseScalarGroundingCheck is the PSG §25(b) answer-side gate. It
// returns at most ONE violation naming every unmatched prose scalar; the
// bus-scoped strict arm (isStrictViolationForBus) makes that single raise
// retry-eligible for exactly one round.
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
	unmatched := scanUnmatchedProseScalars(doc, evidence)
	if len(unmatched) == 0 {
		return nil
	}
	listed := make([]string, 0, len(unmatched))
	for i, tok := range unmatched {
		if i >= proseScalarDetailListCap {
			listed = append(listed, fmt.Sprintf("(+%d more)", len(unmatched)-proseScalarDetailListCap))
			break
		}
		listed = append(listed, fmt.Sprintf("%s%s (block %q)", tok.Raw, tok.Unit, tok.BlockID))
	}
	return []types.Violation{{
		Kind: types.ViolProseScalarUngrounded,
		Detail: fmt.Sprintf(
			"answer prose states %d numeric value(s) that match nothing in this report's evidence surfaces (measured observation records, structured facts, projection tables, or quoted lines): %s",
			len(unmatched), strings.Join(listed, ", ")),
		Repair: "for each listed number, either state next to it the exact source view and time window it was read from — quoting the value as that view published it — or remove the number from the prose. Do not invent a replacement number; when a value is one you derived yourself, name the published values it was derived from.",
		Stage:  string(types.StageFinalize),
		ClusterKey: types.IdentityClusterKey("prose_scalar_ungrounded",
			"answer_prose_scalars"),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_document.blocks.prose",
			Reason:     "prose ms/% scalar with no evidence-face member",
			Confidence: 0.6,
		},
		RepairLocusOverride: types.LocusFinalizer,
	}}
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

// buildProseScalarEvidenceSet assembles the loose numeric membership pool.
func buildProseScalarEvidenceSet(doc *types.AnswerDocumentV2, mut *types.MutableState, ledger types.ObservationLedger) proseScalarEvidenceSet {
	set := proseScalarEvidenceSet{exact: map[string]bool{}}
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
		}
		for _, cite := range doc.Citations {
			addText(cite.Quote)
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

// scanUnmatchedProseScalars walks the model-authored prose surfaces and
// returns every ms/% token the evidence pool cannot account for.
func scanUnmatchedProseScalars(doc *types.AnswerDocumentV2, evidence proseScalarEvidenceSet) []proseScalarToken {
	var unmatched []proseScalarToken
	scanned := 0
	scan := func(blockID, text string) {
		if text == "" || scanned >= proseScalarScanTokenCap {
			return
		}
		for _, tok := range extractProseScalarTokens(blockID, text) {
			if scanned >= proseScalarScanTokenCap {
				return
			}
			scanned++
			if proseScalarTokenGrounded(tok, evidence) {
				continue
			}
			unmatched = append(unmatched, tok)
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
	return unmatched
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
			Value:   value,
			Ulp:     proseScalarUlp(raw),
		})
	}
	return out
}

// proseScalarUlp returns one unit in the last written digit: "46.821" →
// 0.001, "60.8" → 0.1, "300" → 1.
func proseScalarUlp(raw string) float64 {
	if dot := strings.IndexByte(raw, '.'); dot >= 0 {
		return math.Pow(10, -float64(len(raw)-dot-1))
	}
	return 1
}

// proseScalarTokenGrounded runs the tolerance and exemption arms, loosest
// first. Every arm errs toward acceptance (宁松勿严).
func proseScalarTokenGrounded(tok proseScalarToken, evidence proseScalarEvidenceSet) bool {
	// Exemption: zero values carry no measurement claim.
	if tok.Value == 0 {
		return true
	}
	// Arm 1 — verbatim numeral string appears on an evidence surface.
	if evidence.exact[tok.Raw] {
		return true
	}
	// Tolerance = one unit in the token's last written digit, floored at
	// the round-3 band so "46.821" still accepts an evidence "46.8213".
	tol := tok.Ulp
	if tol < 0.0005 {
		tol = 0.0005
	}
	// Arm 2 — direct member within tolerance.
	if proseScalarNear(evidence.values, tok.Value, tol) {
		return true
	}
	if tok.percent() {
		// Arm 3 — the evidence carries the same percentage as a 0-1 ratio.
		if proseScalarNear(evidence.values, tok.Value/100, tol/100) {
			return true
		}
		// Arm 4 — recomputable percentage: some evidence pair (a, b)
		// reproduces the value as a/b at percent scale. k=100 covers a
		// same-unit pair; k=0.1 covers an ms numerator over a
		// seconds-denominated window.
		for _, k := range []float64{100, 0.1} {
			if proseScalarRatioMatch(evidence.values, tok.Value, k, tol) {
				return true
			}
		}
		return false
	}
	// Arm 5 (ms only) — pairwise sum of two evidence values: an honest
	// self-derived total of two published figures is not a fabrication.
	return proseScalarPairSumMatch(evidence.values, tok.Value, tol)
}

// proseScalarNear reports whether sorted values contains a member within
// tol of target.
func proseScalarNear(values []float64, target, tol float64) bool {
	idx := sort.SearchFloat64s(values, target-tol)
	return idx < len(values) && values[idx] <= target+tol+1e-9
}

// proseScalarPairSumMatch reports whether two members of sorted values
// (repetition allowed) sum to target within tol.
func proseScalarPairSumMatch(values []float64, target, tol float64) bool {
	for _, v := range values {
		if v >= target+tol {
			break
		}
		if proseScalarNear(values, target-v, tol) {
			return true
		}
	}
	return false
}

// proseScalarRatioMatch reports whether some pair (a, b) of sorted values
// with b > 0 satisfies |a/b*k - target| <= tol.
func proseScalarRatioMatch(values []float64, target, k, tol float64) bool {
	if target <= 0 {
		return false
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
		idx := sort.SearchFloat64s(values, lo-1e-9)
		if idx < len(values) && values[idx] <= hi+1e-9 && values[idx] > 0 {
			return true
		}
	}
	return false
}

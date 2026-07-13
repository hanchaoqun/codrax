package orchestrator

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// prose_lexicon_board_check.go — CR-1 件② P2 vocabulary arm + 件⑤ P3a/P3b
// board-consistency arms (§29.42 / §29.42.3 / §29.42.4 user rulings,
// docs/design/real_trace_campaign_20260705.md, 2026-07-12; cold-read pass
// table §3.2 P2/P3).
//
// Witnesses (41006 cold read):
//   - 案22: prose presented `same_priority_dependency` /
//     `lower_priority_dependency` / `sync_buffer_read` in the engine's output
//     voice — none of the three exists on any evidence surface (the engine's
//     published type vocabulary was binder_wait / d_state_or_io_wait /
//     io_latency / runnable_wait / s_sleep).
//   - 案17: one report carried TWO first root causes (prose Rank1=binder
//     15.758 vs the typed board's #1 D-state 36.757) and silently violated
//     its own declared ordering key.
//
// §29.42.4 final ruling (verbatim duty — 全线无硬拦), superseded on the
// dispatch side by S3' ① (§29.47.1, 2026-07-12 — witnesses 2779/76278):
// every arm here is INFORMATION-only. The raise is never strict for the
// bus, dispatches ZERO repair rounds, and surfaces on the system
// cross-check appendix at ship time (systemCrossCheckAppendix); the latch
// below survives as a defensive relic for retry surfaces built by OTHER
// kinds. This lane never hard-rejects, never loops, and never injects a
// caveat into the answer (系统不往答案塞话). 答案出厂权属于模型. Hard
// gates stay reserved for criteria that hold no matter who is right;
// none live here.
//
// The P3b deviation arm implements the §29.20 conscious-flip precedent: a
// deviation from the typed board's #1 seat is fine WHEN DISCLOSED — only the
// silent form draws the one hint. The typed board summary fed into the
// same dispatch (§29.42.2① 单源摘要喂入) is this design's precondition:
// freedom is granted alongside the best available information.

// proseLexiconTokenRE extracts engine-styled snake_case tokens from prose:
// at least two lowercase segments joined by underscores. Deliberately narrow
// (identifiers with dots/slashes or capitals never match) — the NOISY
// extraction only ever feeds the soft lane.
var proseLexiconTokenRE = regexp.MustCompile(`[a-z][a-z0-9]*(?:_[a-z0-9]+)+`)

// prosePrimaryClaimRE finds superlative primary-cause claims (zh + en).
var prosePrimaryClaimRE = regexp.MustCompile(`主根因|首要根因|第一根因|最主要的?(?:根因|原因|瓶颈)|(?i:primary root cause|top root cause|#1 root cause)`)

// proseBoardHeadRE finds a prose-authored root-cause board opening (FEED-1②,
// 2026-07-12 — the 84618 replay's misfire shape: the model's own board read
// 「根因排序:① app-9511 …」, which no superlative pattern matched, so P3b
// never saw the claim). The first thread token AFTER the board head binds as
// a primary-cause claim.
var proseBoardHeadRE = regexp.MustCompile(`根因排序|根因清单|(?i:root[- ]cause ranking)`)

// proseDeclaredSortKeyRE finds a self-declared sort key (P3-1, §29.42.3 P3a
// second sub-arm — 案17 键自违半边: the prose declares 「按 X 排序/降序」 and
// then lists values that do not follow it). CR-2 组④ F-2① (2026-07-12):
// 排列 joins the verb set — the 133933 witness head read 「按
// effective_attribution 排列」 and the arm never engaged.
var proseDeclaredSortKeyRE = regexp.MustCompile(`按[^,。;:\n]{1,24}?(?:排序|排列|降序|从大到小)|(?i:sorted by|in descending order)`)

// proseOrdinalMarkRE finds the ordered-list heads the declared-key monotonic
// check walks (①②…/Rank N). CR-2 组④ F-2① (2026-07-12): the markdown
// numbered-list heads (line-anchored 「N. 」) and the prose seat chips
// (「#N」) join the mark set — the 133933 witness board (「3. **#3 app-9511
// … 21.153 ms**」) used both and the monotone walk saw neither. Adjacent
// duplicate marks are harmless: value extraction skips empty segments.
// CR-4 臂6 (2026-07-12, 91951 witness): the 「R3:」 seat-chip form joins
// (the SMR-1 round's ghost board used R1..R5 heads and neither regex had
// the form).
var proseOrdinalMarkRE = regexp.MustCompile(`[①②③④⑤⑥⑦⑧⑨]|#[0-9]{1,2}|\bR[0-9]{1,2}\b|(?m:^\s{0,8}[0-9]{1,2}\.\s)|(?i:rank\s*[0-9])`)

// proseOrdinalValueRE captures the first ms value after one ordinal head.
var proseOrdinalValueRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*(?:ms|毫秒)`)

// proseBoardDeviationDisclosureRE recognizes deviation-disclosure wording —
// ANY match anywhere in the model prose passes the P3b arm (宁松勿严; a
// disclosed deviation always ships without a hint).
var proseBoardDeviationDisclosureRE = regexp.MustCompile(`综合判断|综合考虑|综合权衡|偏离|排序上|与.{0,12}(?:榜|排序)不同|(?i:combined judgment|deviat|differs? from the measured|overall assessment)`)

const (
	proseLexiconTokenScanCap  = 120
	proseLexiconFindingCap    = 8
	prosePrimaryClaimScanCap  = 24
	proseLexiconCorpusTextCap = 1 << 20 // corpus text budget guard (bytes)
)

// runProseLexiconBoardCheck is the CR-1 件②/件⑤ soft gate. It returns at
// most ONE violation naming every out-of-vocabulary snake_case token (P2),
// an internal double-primary contradiction (P3a), and a silent deviation of
// the prose primary from the typed board's #1 seat (P3b). One shared
// one-round latch; never a hard reject; advisory log fallback.
func runProseLexiconBoardCheck(doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState) []types.Violation {
	if doc == nil || bus == nil || mut == nil {
		return nil
	}
	latched := mut.ProseLexiconBoardHintDelivered()
	// One-round latch part 1: the current dispatch was born carrying this
	// lane's hint — its output is final for the lane.
	if rs := mut.RetryState(); rs != nil && retryStateListsProseLexiconBoardHint(rs) {
		mut.MarkProseLexiconBoardHintDelivered()
		latched = true
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	// Scope gate (typed, precise): trace evidence plane only — the same gate
	// the prose-scalar lane uses.
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return nil
	}
	findings := proseLexiconBoardInternalStrings(scanProseLexiconBoardFindings(doc, bus, mut, ledger))
	if len(findings) == 0 {
		return nil
	}
	if latched {
		// §29.42.4 ②: the round is spent — ship exactly as written; the
		// fallback is ONE advisory log line (never a caveat, never a loop).
		logging.Info("orchestrator: prose lexicon/board advisory (soft lane, hint round spent, answer ships as written): %s", strings.Join(findings, "; "))
		return nil
	}
	listed := findings
	if len(listed) > proseLexiconFindingCap {
		listed = append(append([]string(nil), listed[:proseLexiconFindingCap]...), fmt.Sprintf("(+%d more)", len(findings)-proseLexiconFindingCap))
	}
	return []types.Violation{{
		Kind:   types.ViolProseLexiconBoardInconsistent,
		Detail: strings.Join(listed, "; "),
		Repair: "go through the listed items ONE BY ONE — for each listed technical token, either remove it or replace it with a term exactly as the measured evidence of this report publishes it (never invent an evidence-styled token); state exactly ONE primary root cause across the whole answer; and when the primary root cause you name differs from the measured root-cause board's #1 row, keep your conclusion but say explicitly that it differs from the measured ordering and what your judgment is based on — never reorder silently. If after review you stand by your wording, keep it — this reminder is delivered once. Apply the fix with the SMALLEST possible edit: change ONLY the blocks named in the finding (by their block id) and keep every other block byte-for-byte identical — prefer emit_answer_document_patch, listing the untouched block ids as unchanged and replacing only the named blocks, over rewriting the whole answer.",
		Stage:  string(types.StageFinalize),
		ClusterKey: types.IdentityClusterKey("prose_lexicon_board_inconsistent",
			"answer_prose_lexicon_board"),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_document.blocks.prose",
			Reason:     "prose uses an unpublished engine-styled token, claims two primary root causes, or silently deviates from the measured board's #1 seat",
			Confidence: 0.5,
		},
		RepairLocusOverride: types.LocusFinalizer,
	}}
}

// proseLexiconBoardFinding is one mechanical finding with two rendering
// faces: internal (the English detail for violations and advisory logs —
// byte-compatible with the pre-S3' strings) and user-readable (the S3' ②
// system cross-check appendix wording, §29.47.1 — plain language, never
// internal machinery vocabulary).
type proseLexiconBoardFinding struct {
	internal string
	userZH   string
	userEN   string
}

func (f proseLexiconBoardFinding) userReadable(lang string) string {
	if isChineseLang(lang) {
		return f.userZH
	}
	return f.userEN
}

func proseLexiconBoardInternalStrings(findings []proseLexiconBoardFinding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.internal)
	}
	return out
}

// proseLexiconBoardResidualFindings re-runs the lexicon/board scan against a
// document (latch-independent) — the S3' appendix's raw verdict input,
// mirroring proseScalarResidualFindingLabels. Empty on non-trace runs.
func proseLexiconBoardResidualFindings(doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState) []proseLexiconBoardFinding {
	if doc == nil || bus == nil || mut == nil {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return nil
	}
	return scanProseLexiconBoardFindings(doc, bus, mut, ledger)
}

// scanProseLexiconBoardFindings runs the three arms and returns typed
// findings (empty = clean).
//
// CR-4 臂6 unit-granularity root fix (2026-07-12; witnesses 133933 §29.49
// F-2 + 56249/91951 两轮幽灵席复发): the board-family arms (primary claims,
// declared-key monotone walk, seat identity) scan RENDER-SHAPE units — one
// fused unit per block (Title + numbered "Label — Text" item lines,
// mirroring renderV2BlockItem) — because the production board is a list
// block whose head, chip, subject and confidence are split across Title /
// Label / Text fields, and per-field units structurally hid every claim
// from every arm across two witness rounds. The vocabulary arm keeps
// per-field units (finer block-id attribution; token dedup makes the
// granularity immaterial there).
func scanProseLexiconBoardFindings(doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState, ledger types.ObservationLedger) []proseLexiconBoardFinding {
	var findings []proseLexiconBoardFinding
	vocabulary := buildProseLexiconVocabulary(doc, bus, mut, ledger)
	prose := collectModelProseUnits(doc)
	boardProse := collectModelProseBoardUnits(doc)

	// ── P2 vocabulary arm ────────────────────────────────────────────────
	scanned := 0
	seen := map[string]bool{}
	var unknown, unknownZH []string
	for _, unit := range prose {
		for _, token := range proseLexiconTokenRE.FindAllString(unit.text, -1) {
			if scanned >= proseLexiconTokenScanCap {
				break
			}
			scanned++
			if seen[token] || vocabulary[token] {
				continue
			}
			seen[token] = true
			unknown = append(unknown, fmt.Sprintf("%s (block %q)", token, unit.blockID))
			unknownZH = append(unknownZH, fmt.Sprintf("%s（块 %s）", token, unit.blockID))
		}
	}
	if len(unknown) > 0 {
		findings = append(findings, proseLexiconBoardFinding{
			internal: fmt.Sprintf("answer prose uses %d engine-styled token(s) that no evidence surface of this report publishes: %s", len(unknown), strings.Join(capStrings(unknown, proseLexiconFindingCap), ", ")),
			userZH:   fmt.Sprintf("正文中的术语 %s 未在本报告证据面出现，可能为改写或笔误", strings.Join(capStrings(unknownZH, proseLexiconFindingCap), "、")),
			userEN:   fmt.Sprintf("the body's term(s) %s do not appear on this report's evidence surfaces and may be paraphrases or typos", strings.Join(capStrings(unknown, proseLexiconFindingCap), ", ")),
		})
	}

	// ── P3a internal-contradiction arm + P3b silent-deviation arm ───────
	claims := collectProsePrimaryClaims(boardProse)
	distinct := map[string]string{} // tid → raw spelling
	for _, claim := range claims {
		if claim.tid != "" {
			if _, ok := distinct[claim.tid]; !ok {
				distinct[claim.tid] = claim.raw
			}
		}
	}
	if len(distinct) >= 2 {
		var names []string
		for _, raw := range distinct {
			names = append(names, raw)
		}
		sort.Strings(names)
		findings = append(findings, proseLexiconBoardFinding{
			internal: fmt.Sprintf("answer prose claims %d different entities as THE primary root cause in one document: %s — state exactly one primary", len(distinct), strings.Join(capStrings(names, 4), " vs ")),
			userZH:   fmt.Sprintf("正文在不同位置分别将 %s 称为首要根因，表述不一致", strings.Join(capStrings(names, 4), " 与 ")),
			userEN:   fmt.Sprintf("the body names %s each as the primary root cause in different places — the statements are inconsistent", strings.Join(capStrings(names, 4), " and ")),
		})
	} else if len(distinct) == 1 {
		boardSubject, boardTID := typedBoardPrimarySeat(ledger)
		if boardTID != "" {
			if _, claimed := distinct[boardTID]; !claimed && !proseCarriesDeviationDisclosure(prose) {
				for _, raw := range distinct {
					findings = append(findings, proseLexiconBoardFinding{
						internal: fmt.Sprintf("answer prose names %s as the primary root cause while the measured board's #1 seat is %s, with no wording disclosing the deviation — keep your conclusion but disclose the difference and its basis", raw, boardSubject),
						userZH:   fmt.Sprintf("正文首因（%s）与实测榜 #1（%s）不同，未见披露句", raw, boardSubject),
						userEN:   fmt.Sprintf("the body's primary cause (%s) differs from the measured board's #1 (%s), with no sentence disclosing the deviation", raw, boardSubject),
					})
					break
				}
			}
		}
	}

	// ── P3-1 declared-sort-key monotonic sub-arm (§29.42.3 P3a 第二子臂) ──
	if finding, ok := proseDeclaredKeyNonMonotonic(boardProse); ok {
		findings = append(findings, finding)
	}

	// ── CR-2 组④ F-2② board-member identity gate (2026-07-12) ────────────
	findings = append(findings, proseBoardSeatIdentityFindings(boardProse, ledger)...)
	return findings
}

// proseBoardSeatCap bounds the F-2② findings (one per unseated subject).
const proseBoardSeatCap = 4

// proseBoardSeatChipRE finds prose seat chips (#N) for the identity gate.
// EVOLUTION RECORD (CR-4 修复轮方向改造, 用户裁定 2026-07-12): the CR-4
// R-chip / circled-digit chip extensions are RETIRED with the accusatory
// annotation lane — the system no longer characterizes the model's prose
// (「正文自排名次」-class judgments); ghost-seat juxtaposition now rides the
// fact lane's 「typed 席位=…」 field (prose_fact_juxtaposition.go), which
// needs no chip grammar at all. The original CR-2 F-2② #N gate keeps its
// engine-chip-grammar scope unchanged.
var proseBoardSeatChipRE = regexp.MustCompile(`#([0-9]{1,2})`)

// collectModelProseBoardUnits builds RENDER-SHAPE prose units (CR-4 臂6
// root fix): one unit per block — Title, block text, then each item as the
// renderer's "N. Label — Text" line — so board heads, seat chips, subjects
// and confidences that production splits across Title/Label/Text land in
// ONE scannable unit (and one sentence where the renderer joins them with
// an em-dash). Same block population as collectModelProseUnits.
func collectModelProseBoardUnits(doc *types.AnswerDocumentV2) []proseTextUnit {
	var out []proseTextUnit
	if doc == nil {
		return out
	}
	for _, blk := range doc.Blocks {
		if proseScalarScanExemptBlock(blk) {
			continue
		}
		if blk.Kind == types.BlockCaveat || blk.Kind == types.BlockDiagram {
			continue
		}
		var b strings.Builder
		if strings.TrimSpace(blk.Title) != "" {
			b.WriteString(blk.Title)
			b.WriteString("\n")
		}
		if strings.TrimSpace(blk.Text) != "" {
			b.WriteString(blk.Text)
			b.WriteString("\n")
		}
		for i, it := range blk.Items {
			line := strings.TrimSpace(it.Label)
			if txt := strings.TrimSpace(it.Text); txt != "" {
				if line != "" {
					line += " — " + txt
				} else {
					line = txt
				}
			}
			for _, cell := range it.Cells {
				if cell = strings.TrimSpace(cell); cell != "" {
					line += " " + cell
				}
			}
			if line == "" {
				continue
			}
			fmt.Fprintf(&b, "%d. %s\n", i+1, line)
		}
		if text := strings.TrimSpace(b.String()); text != "" {
			out = append(out, proseTextUnit{blockID: blk.ID, text: text})
		}
	}
	return out
}

// proseBoardSeatIdentityFindings is the CR-2 组④ F-2② board-member identity
// gate (ledger §29.49; witness 133933: prose seated #3 app-9511 / #4
// DetectViewRect-17679 while the engine board held no such seats — the
// self-made board rode the engine's own chip grammar and no arm noticed).
// For every prose seat chip (#N) inside a board-form unit (a 根因排序 head or
// ≥2 chips), the subject bound to the chip must hold SOME typed board seat;
// an unseated subject draws one information finding (per subject, capped).
// 宁松勿严: chip without a nearby thread token, tid-less bindings and
// seatless typed boards all stay silent.
func proseBoardSeatIdentityFindings(prose []proseTextUnit, ledger types.ObservationLedger) []proseLexiconBoardFinding {
	seated := typedBoardSeatRanks(ledger)
	if len(seated) == 0 {
		return nil
	}
	var findings []proseLexiconBoardFinding
	flagged := map[string]bool{}
	for _, unit := range prose {
		chips := proseBoardSeatChipRE.FindAllStringSubmatchIndex(unit.text, -1)
		if len(chips) == 0 {
			continue
		}
		if len(chips) < 2 && !proseBoardHeadRE.MatchString(unit.text) {
			continue // not a board-form unit
		}
		threads := extractProseScalarThreadRefs(unit.text)
		if len(threads) == 0 {
			continue
		}
		for _, chip := range chips {
			if len(findings) >= proseBoardSeatCap {
				return findings
			}
			ordinal := unit.text[chip[2]:chip[3]]
			// The chip's subject: the first thread token starting at/after the
			// chip, within a short leash (the witness grammar 「#3 app-9511」).
			var bound *proseScalarThreadRef
			for i := range threads {
				if threads[i].Pos >= chip[1] && threads[i].Pos-chip[1] <= 60 {
					bound = &threads[i]
					break
				}
			}
			if bound == nil || bound.TID == "" || flagged[bound.TID] {
				continue
			}
			if len(seated[bound.TID]) > 0 {
				continue
			}
			flagged[bound.TID] = true
			findings = append(findings, proseLexiconBoardFinding{
				internal: fmt.Sprintf("prose board seats #%s onto %s, which holds no seat on the measured board — present measured seats as measured, and mark self-derived rankings as your own analysis", ordinal, bound.Raw),
				userZH:   fmt.Sprintf("正文榜第%s位（%s）在实测根因榜上无对应席位，或为正文自排名次", ordinal, bound.Raw),
				userEN:   fmt.Sprintf("the body's board seats #%s onto %s, which holds no seat on the measured board — possibly a self-derived ranking", ordinal, bound.Raw),
			})
		}
	}
	return findings
}

// typedBoardSeatRanks collects, per tid, every SEATED typed board rank
// (rank > 0, chain and adjacent channels alike). The identity gate asks
// membership; the F-CR3-10(c) ordinal arm asks WHICH seat.
func typedBoardSeatRanks(ledger types.ObservationLedger) map[string]map[int]bool {
	seated := map[string]map[int]bool{}
	for _, record := range ledger.Records {
		if record.Producer != "trace_query" || !strings.Contains(record.ID, "#root_cause_rank:") {
			continue
		}
		rank := 0
		for _, note := range record.RichNotes {
			if v, ok := strings.CutPrefix(note, types.TraceNoteKeyRank+"="); ok {
				rank, _ = strconv.Atoi(strings.TrimSpace(v))
				break
			}
		}
		if rank <= 0 {
			continue
		}
		for _, tref := range extractProseScalarThreadRefs(record.Subject) {
			if tref.TID != "" {
				if seated[tref.TID] == nil {
					seated[tref.TID] = map[int]bool{}
				}
				seated[tref.TID][rank] = true
			}
		}
	}
	return seated
}

// proseTextUnit is one model-authored prose surface.
type proseTextUnit struct {
	blockID string
	text    string
}

// collectModelProseUnits walks the model-authored prose surfaces — the same
// block population the prose-scalar scan audits (system evidence blocks,
// next-step lanes, caveats and diagrams stay out).
func collectModelProseUnits(doc *types.AnswerDocumentV2) []proseTextUnit {
	var out []proseTextUnit
	if doc == nil {
		return out
	}
	add := func(blockID, text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		out = append(out, proseTextUnit{blockID: blockID, text: text})
	}
	for _, blk := range doc.Blocks {
		if proseScalarScanExemptBlock(blk) {
			continue
		}
		if blk.Kind == types.BlockCaveat || blk.Kind == types.BlockDiagram {
			continue
		}
		add(blk.ID, blk.Title)
		add(blk.ID, blk.Text)
		for _, it := range blk.Items {
			add(blk.ID, it.Label)
			add(blk.ID, it.Text)
			for _, cell := range it.Cells {
				add(blk.ID, cell)
			}
		}
	}
	return out
}

// buildProseLexiconVocabulary assembles the run's published snake_case
// vocabulary: every snake_case token any evidence surface of THIS run
// published (the same faces the prose-scalar evidence set reads), plus the
// engine's registered causal-token universe, plus tokens the user's own
// request carried (quoting the user is never a fabrication).
func buildProseLexiconVocabulary(doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState, ledger types.ObservationLedger) map[string]bool {
	vocabulary := map[string]bool{}
	budget := proseLexiconCorpusTextCap
	addText := func(text string) {
		if text == "" || budget <= 0 {
			return
		}
		if len(text) > budget {
			text = text[:budget]
		}
		budget -= len(text)
		for _, token := range proseLexiconTokenRE.FindAllString(text, -1) {
			vocabulary[token] = true
		}
	}
	for _, token := range tracequery.CausalTokenUniverse() {
		vocabulary[token] = true
	}
	// S1b (STAB-1, 2026-07-12): standard kernel/ftrace event names are a
	// closed-set dictionary — the 2779 witness flagged block_rq_issue /
	// irq_handler_entry / irq_handler_exit, all real kernel event names.
	for _, name := range tracequery.StandardTraceEventNameUniverse() {
		vocabulary[name] = true
	}
	// S1a (STAB-1, 2026-07-12): every snake_case token greppable in the
	// attached artifact text is a legal quote (block_rq_issue is IN the
	// trace). Extracted ONCE per run into a deduped set (the MutableState
	// cache — 禁全文扫描每次重复).
	for token := range proseLexiconAttachedArtifactTokens(bus, mut) {
		vocabulary[token] = true
	}
	if rm := mut.RequestModel(); rm != nil {
		addText(rm.RawRequest)
	}
	// 复核 P4-3 (2026-07-12): the CURRENT run's raw tool outputs are engine
	// publications — every snake_case token a deterministic tool printed is
	// quotable (冷读实锤: ohos_rt-class tokens come from tool output, not
	// model fabrication; flagging them would be a false soft hint).
	for _, tr := range bus.ToolResults {
		addText(tr.Summary)
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
	addFacts(mut.StableInvestigationAggregateFacts())
	if ta := mut.TurnAArtifacts(); ta != nil {
		addFacts(ta.AcceptedAggregateFacts)
		// Raw tool outputs are part of what the engine published to this
		// run: every token a deterministic tool printed is quotable.
		for _, tr := range ta.ToolResults {
			addText(tr.Summary)
		}
	}
	for _, record := range ledger.Records {
		addText(record.Value)
		addText(record.Subject)
		addText(record.Object)
		addText(record.Predicate)
		addText(record.ClaimKey)
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
	return vocabulary
}

// proseLexiconAttachedArtifact caps bound the S1a extraction: token set
// size and scanned bytes (the witness trace is 3.5MB — one bounded pass).
const (
	proseLexiconAttachedArtifactTokenCap = 20000
	proseLexiconAttachedArtifactByteCap  = 8 << 20
)

// proseLexiconAttachedArtifactTokens returns the attached artifacts'
// snake_case token set (S1a, STAB-1 2026-07-12), computed AT MOST ONCE per
// run and cached on MutableState. Nil-safe; empty attachments yield an
// empty (but cached) set.
func proseLexiconAttachedArtifactTokens(bus *types.BusContext, mut *types.MutableState) map[string]bool {
	if bus == nil || mut == nil {
		return nil
	}
	if cached := mut.AttachedArtifactLexicon(); cached != nil {
		return cached
	}
	lexicon := map[string]bool{}
	scan := func(text string) {
		if text == "" || len(lexicon) >= proseLexiconAttachedArtifactTokenCap {
			return
		}
		if len(text) > proseLexiconAttachedArtifactByteCap {
			// Tail-integrity guard (P3-3): the byte cap may cut a token
			// mid-spelling — trim the partial tail so a truncated spelling
			// never enters the quotable lexicon.
			text = strings.TrimRightFunc(text[:proseLexiconAttachedArtifactByteCap], func(r rune) bool {
				return r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
			})
		}
		for _, token := range proseLexiconTokenRE.FindAllString(text, -1) {
			if len(lexicon) >= proseLexiconAttachedArtifactTokenCap {
				return
			}
			lexicon[token] = true
		}
	}
	scan(bus.AttachedHitrace)
	scan(bus.AttachedLog)
	mut.SetAttachedArtifactLexicon(lexicon)
	return lexicon
}

// prosePrimaryClaim is one superlative primary-cause claim with its nearest
// thread identity (tid "" = no thread token in the claim's text unit).
type prosePrimaryClaim struct {
	raw string
	tid string
}

// collectProsePrimaryClaims finds primary-cause claims and binds each to a
// thread token (noisy by design — soft lane only). Two claim forms:
//   - superlative wording (主根因/primary root cause …): nearest thread token
//     in the unit;
//   - a prose-authored BOARD opening (FEED-1②, 2026-07-12: 「根因排序:①
//     app-9511 …」): the FIRST thread token at/after the board head — the
//     84618 replay misfire shape, where the model listed its own board with
//     no superlative word and P3b never saw the claim.
func collectProsePrimaryClaims(prose []proseTextUnit) []prosePrimaryClaim {
	var out []prosePrimaryClaim
	for _, unit := range prose {
		threads := extractProseScalarThreadRefs(unit.text)
		if len(threads) == 0 {
			continue
		}
		for _, m := range prosePrimaryClaimRE.FindAllStringIndex(unit.text, -1) {
			if len(out) >= prosePrimaryClaimScanCap {
				return out
			}
			nearest := threads[0]
			for _, tref := range threads[1:] {
				if proseScalarPosDistance(tref.Pos, m[0]) < proseScalarPosDistance(nearest.Pos, m[0]) {
					nearest = tref
				}
			}
			out = append(out, prosePrimaryClaim{raw: nearest.Raw, tid: nearest.TID})
		}
		if m := proseBoardHeadRE.FindStringIndex(unit.text); m != nil {
			if len(out) >= prosePrimaryClaimScanCap {
				return out
			}
			// The board's first-listed entity: the first thread token at or
			// after the head (falls back to none when the board names no
			// thread — absence never claims).
			for _, tref := range threads {
				if tref.Pos >= m[0] {
					out = append(out, prosePrimaryClaim{raw: tref.Raw, tid: tref.TID})
					break
				}
			}
		}
	}
	return out
}

// proseDeclaredKeyNonMonotonic is the P3-1 (§29.42.3 P3a second sub-arm)
// internal-contradiction check: a prose unit that DECLARES a sort key (按 X
// 排序/降序/from largest) and then lists ordinal entries whose own ms values
// INCREASE somewhere down the list contradicts itself — 与谁对无关 (案17:
// 「按 effective_impact_ms 排序」 with #1 15.758 < the same list's later
// values). Returns (finding, true) on a contradiction. Noisy extraction —
// information lane only.
func proseDeclaredKeyNonMonotonic(prose []proseTextUnit) (proseLexiconBoardFinding, bool) {
	for _, unit := range prose {
		if !proseDeclaredSortKeyRE.MatchString(unit.text) {
			continue
		}
		marks := proseOrdinalMarkRE.FindAllStringIndex(unit.text, -1)
		if len(marks) < 2 {
			continue
		}
		var values []float64
		for i, mark := range marks {
			segmentEnd := len(unit.text)
			if i+1 < len(marks) {
				segmentEnd = marks[i+1][0]
			}
			m := proseOrdinalValueRE.FindStringSubmatch(unit.text[mark[1]:segmentEnd])
			if m == nil {
				continue
			}
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				values = append(values, v)
			}
		}
		for i := 1; i < len(values); i++ {
			if values[i] > values[i-1]+1e-9 {
				return proseLexiconBoardFinding{
					internal: fmt.Sprintf("answer prose declares a sort key but its own listed values do not follow it (%.3fms is listed before %.3fms in block %q) — either fix the order or restate the key", values[i-1], values[i], unit.blockID),
					userZH:   fmt.Sprintf("正文声明了排序，但列表中 %.3fms 排在 %.3fms 之前（块 %s），与所声明的顺序不符", values[i-1], values[i], unit.blockID),
					userEN:   fmt.Sprintf("the body declares a sort order, yet %.3fms is listed before %.3fms (in %q), which does not follow the declared order", values[i-1], values[i], unit.blockID),
				}, true
			}
		}
	}
	return proseLexiconBoardFinding{}, false
}

// proseCarriesDeviationDisclosure reports whether ANY model prose unit
// carries deviation-disclosure wording (a disclosed deviation always passes).
func proseCarriesDeviationDisclosure(prose []proseTextUnit) bool {
	for _, unit := range prose {
		if proseBoardDeviationDisclosureRE.MatchString(unit.text) {
			return true
		}
	}
	return false
}

// typedBoardPrimarySeat returns the typed board's #1 chain seat (subject
// label + tid), or "" when the run published no seated chain rows.
func typedBoardPrimarySeat(ledger types.ObservationLedger) (subject, tid string) {
	for _, record := range ledger.Records {
		if record.Producer != "trace_query" || !strings.Contains(record.ID, "#root_cause_rank:") {
			continue
		}
		rank, relevance := 0, ""
		for _, note := range record.RichNotes {
			if v, ok := strings.CutPrefix(note, types.TraceNoteKeyRank+"="); ok {
				rank, _ = strconv.Atoi(strings.TrimSpace(v))
			}
			if v, ok := strings.CutPrefix(note, types.TraceNoteKeyChainRelevance+"="); ok {
				relevance = strings.TrimSpace(v)
			}
		}
		if rank != 1 || relevance == "adjacent" {
			continue
		}
		subject = strings.TrimSpace(record.Subject)
		for _, tref := range extractProseScalarThreadRefs(subject) {
			tid = tref.TID
			break
		}
		return subject, tid
	}
	return "", ""
}

// retryStateListsProseLexiconBoardHint reports whether the typed retry
// surface carries this lane's kind — i.e. the dispatch consuming that
// surface received the hint.
func retryStateListsProseLexiconBoardHint(rs *types.RetryState) bool {
	if rs == nil {
		return false
	}
	for _, v := range rs.ActiveViolations {
		if v.Kind == types.ViolProseLexiconBoardInconsistent {
			return true
		}
	}
	return false
}

func capStrings(in []string, limit int) []string {
	if len(in) <= limit {
		return in
	}
	out := append([]string(nil), in[:limit]...)
	return append(out, fmt.Sprintf("(+%d more)", len(in)-limit))
}

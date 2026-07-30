package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EVALFIX-2E (CLASS 5) — degradation-disclosure footer pins.

func degradationFooterTestDoc() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "答案主体。",
		}},
	}
}

// seededDegradationMutable builds the decisive-e2e state through the
// REAL channels the writers use: 17 citation rewrites via
// AppendDegradation (the lane the tool-layer call sites book) plus one
// facet softening on the EXISTING richness-telemetry channel (the
// one-way projection arm — no ledger writer exists for that lane).
func seededDegradationMutable() *types.MutableState {
	mu := types.NewMutableState("q")
	mu.AppendDegradation(types.DegradeLaneCitationQuoteRewrite, 17)
	mu.AppendRichnessTelemetry(types.RichnessTelemetrySignal{Kind: "facet_softened", FacetID: "f1", Reason: "no acceptable form"})
	return mu
}

// TestDegradationFooterRendersExactlyOneGroupedLine is the decisive
// positive pin: a run with one facet softened + 17 citation rewrites
// renders EXACTLY ONE grouped footer line with both counts, wording per
// session language, order = registry declaration order.
func TestDegradationFooterRendersExactlyOneGroupedLine(t *testing.T) {
	ctxZH := &types.AgentContext{Mutable: seededDegradationMutable()}
	gotZH := RenderAnswerDocumentWithLastMileSupplements(ctxZH, degradationFooterTestDoc(), nil, "zh")
	wantZH := "系统降级披露：引用摘录回填 ×17、必答面硬转软 ×1（完整明细见运行日志）"
	if n := strings.Count(gotZH, "系统降级披露："); n != 1 {
		t.Fatalf("footer prefix must appear EXACTLY once, got %d:\n%s", n, gotZH)
	}
	if !strings.Contains(gotZH, wantZH) {
		t.Fatalf("zh footer line mismatch.\nwant substring: %s\ngot:\n%s", wantZH, gotZH)
	}

	ctxEN := &types.AgentContext{Mutable: seededDegradationMutable()}
	gotEN := RenderAnswerDocumentWithLastMileSupplements(ctxEN, degradationFooterTestDoc(), nil, "en")
	wantEN := "System degradation disclosure: citation quote backfill ×17, required-facet softened ×1 (full detail in run logs)"
	if n := strings.Count(gotEN, "System degradation disclosure:"); n != 1 {
		t.Fatalf("EN footer prefix must appear EXACTLY once, got %d:\n%s", n, gotEN)
	}
	if !strings.Contains(gotEN, wantEN) {
		t.Fatalf("en footer line mismatch.\nwant substring: %s\ngot:\n%s", wantEN, gotEN)
	}

	// The footer is one single line — no multi-line expansion, never
	// per-event rows.
	if line := renderDegradationDisclosureFooter(ctxZH, degradationFooterTestDoc(), "zh"); strings.Contains(line, "\n") {
		t.Fatalf("footer must be a single line, got %q", line)
	}
}

// TestDegradationFooterCleanRunZeroBytes pins the healthy path: a clean
// run (empty ledger) renders byte-identically with the footer feature
// present — zero extra bytes, no prefix.
func TestDegradationFooterCleanRunZeroBytes(t *testing.T) {
	clean := &types.AgentContext{Mutable: types.NewMutableState("q")}
	got := RenderAnswerDocumentWithLastMileSupplements(clean, degradationFooterTestDoc(), nil, "zh")
	if strings.Contains(got, "系统降级披露") || strings.Contains(got, "System degradation disclosure") {
		t.Fatalf("clean run must not carry a degradation footer:\n%s", got)
	}
	// And byte-identity: with the knob off (footer arm fully inert) the
	// clean render is the same bytes — the footer contributed nothing.
	SetDegradationFooterEnabled(false)
	defer SetDegradationFooterEnabled(true)
	off := RenderAnswerDocumentWithLastMileSupplements(
		&types.AgentContext{Mutable: types.NewMutableState("q")}, degradationFooterTestDoc(), nil, "zh")
	if got != off {
		t.Fatalf("clean-run render must be byte-identical with footer enabled vs disabled.\nenabled:\n%s\ndisabled:\n%s", got, off)
	}
}

// TestDegradationFooterKnobOffRestoresByteIdenticalOutput pins the
// pipeline_degradation_footer escape: with entries present, knob-off
// output equals knob-on output minus exactly the appended footer line —
// i.e. the pre-change bytes. Recording is NOT stopped by the knob.
func TestDegradationFooterKnobOffRestoresByteIdenticalOutput(t *testing.T) {
	mu := seededDegradationMutable()
	ctx := &types.AgentContext{Mutable: mu}
	on := RenderAnswerDocumentWithLastMileSupplements(ctx, degradationFooterTestDoc(), nil, "zh")
	footer := renderDegradationDisclosureFooter(ctx, degradationFooterTestDoc(), "zh")
	if footer == "" {
		t.Fatalf("precondition: footer must render for seeded state")
	}
	SetDegradationFooterEnabled(false)
	defer SetDegradationFooterEnabled(true)
	off := RenderAnswerDocumentWithLastMileSupplements(ctx, degradationFooterTestDoc(), nil, "zh")
	if strings.Contains(off, "系统降级披露") {
		t.Fatalf("knob off must remove the footer:\n%s", off)
	}
	// Byte-level relation pinned exactly: on == trimRight(off) + "\n\n" + footer + "\n"
	if want := strings.TrimRight(off, "\n") + "\n\n" + footer + "\n"; on != want {
		t.Fatalf("knob-off must restore byte-identical pre-footer output.\non:\n%q\nreconstructed:\n%q", on, want)
	}
	// The ledger keeps recording while the footer is off (telemetry is
	// always on; the knob gates rendering only).
	if entries := types.BuildDegradationLedgerView(mu); len(entries) == 0 {
		t.Fatalf("knob off must not clear or stop the ledger")
	}
}

// TestDegradationFooterPlumbingLanesNeverDisclose is the permanent
// negative pin: plumbing-class lanes are registered (classification is
// explicit) but NEVER ride the user surface, even if a future writer
// appends them — log-only forever.
func TestDegradationFooterPlumbingLanesNeverDisclose(t *testing.T) {
	mu := types.NewMutableState("q")
	for lane, spec := range types.DegradationLaneRegistry {
		if spec.Class == types.ClassPlumbing {
			mu.AppendDegradation(lane, 5)
		}
	}
	ctx := &types.AgentContext{Mutable: mu}
	if footer := renderDegradationDisclosureFooter(ctx, degradationFooterTestDoc(), "zh"); footer != "" {
		t.Fatalf("plumbing-only ledger must render ZERO footer bytes, got %q", footer)
	}
	got := RenderAnswerDocumentWithLastMileSupplements(ctx, degradationFooterTestDoc(), nil, "zh")
	if strings.Contains(got, "系统降级披露") {
		t.Fatalf("plumbing lanes must never disclose on the user surface:\n%s", got)
	}
	for lane, spec := range types.DegradationLaneRegistry {
		if spec.Class != types.ClassPlumbing {
			continue
		}
		if strings.Contains(got, spec.ZH) || strings.Contains(got, spec.EN) || strings.Contains(got, string(lane)) {
			t.Fatalf("plumbing lane %q wording leaked onto the user surface:\n%s", lane, got)
		}
	}
}

// TestDegradationFooterUnregisteredLaneFailsOpenToGenericWord pins the
// defensive arm: an unregistered lane keeps its count but renders the
// generic word — the system never invents a specific name and the
// internal token never rides the user surface.
func TestDegradationFooterUnregisteredLaneFailsOpenToGenericWord(t *testing.T) {
	mu := types.NewMutableState("q")
	mu.AppendDegradation(types.DegradationLaneID("mystery_future_lane"), 3)
	ctx := &types.AgentContext{Mutable: mu}
	zh := renderDegradationDisclosureFooter(ctx, degradationFooterTestDoc(), "zh")
	if !strings.Contains(zh, "确定性降级处理 ×3") {
		t.Fatalf("unregistered lane must fail open to the generic zh word with count kept, got %q", zh)
	}
	if strings.Contains(zh, "mystery_future_lane") {
		t.Fatalf("internal token rode the user surface: %q", zh)
	}
	en := renderDegradationDisclosureFooter(ctx, degradationFooterTestDoc(), "en")
	if !strings.Contains(en, "deterministic degradation ×3") {
		t.Fatalf("unregistered lane must fail open to the generic en word, got %q", en)
	}
}

// TestDegradationFooterInternalTokensNeverRideUserSurface is the word-
// discipline pin (degradedSectionDisplayNames 同款纪律): with every
// registered lane active, no internal snake_case lane token appears in
// the rendered answer.
func TestDegradationFooterInternalTokensNeverRideUserSurface(t *testing.T) {
	mu := types.NewMutableState("q")
	for _, lane := range types.DegradationLaneRegistryOrder {
		mu.AppendDegradation(lane, 1)
	}
	ctx := &types.AgentContext{Mutable: mu}
	for _, lang := range []string{"zh", "en"} {
		got := RenderAnswerDocumentWithLastMileSupplements(ctx, degradationFooterTestDoc(), nil, lang)
		for _, lane := range types.DegradationLaneRegistryOrder {
			if strings.Contains(got, string(lane)) {
				t.Fatalf("internal lane token %q must never ride the user surface (lang=%s):\n%s", lane, lang, got)
			}
		}
	}
}

// TestDegradationFooterConsumptionSetEqualsAnswerSemanticsSet is the
// bidirectional equality tripwire between the registry classification
// and the footer's consumption set: seeding EVERY registered lane at
// count 1 must render exactly the answer-semantics labels — each
// answer-semantics label present, each plumbing label absent, and the
// group count equal to the answer-semantics lane count.
func TestDegradationFooterConsumptionSetEqualsAnswerSemanticsSet(t *testing.T) {
	mu := types.NewMutableState("q")
	for _, lane := range types.DegradationLaneRegistryOrder {
		mu.AppendDegradation(lane, 1)
	}
	ctx := &types.AgentContext{Mutable: mu}
	footer := renderDegradationDisclosureFooter(ctx, degradationFooterTestDoc(), "zh")
	if footer == "" {
		t.Fatalf("footer must render when answer-semantics lanes are active")
	}
	answerSemantics := 0
	for lane, spec := range types.DegradationLaneRegistry {
		switch spec.Class {
		case types.ClassAnswerSemantics:
			answerSemantics++
			if !strings.Contains(footer, spec.ZH+" ×1") {
				t.Errorf("answer-semantics lane %q (%s) missing from footer: %q", lane, spec.ZH, footer)
			}
		case types.ClassPlumbing:
			if strings.Contains(footer, spec.ZH) {
				t.Errorf("plumbing lane %q (%s) leaked into footer: %q", lane, spec.ZH, footer)
			}
		}
	}
	if groups := strings.Count(footer, "×"); groups != answerSemantics {
		t.Fatalf("footer group count = %d, want %d (== |ClassAnswerSemantics| — consumption set equality, both directions): %q",
			groups, answerSemantics, footer)
	}
}

// TestDegradationFooterSelfDisclosedLaneSuppressed — R6-5 semantics
// under the R7-1 doc-derived mechanism: when the document being rendered
// already discloses a lane's work through its own degraded caveat (whose
// citation-quote-backfill section word is IDENTICAL to the lane's
// registry word 引用摘录回填 — identity asserted here, it is what the
// suppression predicate keys on), the footer must not disclose that lane
// again — one answer never carries the same lane's disclosure twice.
// Other lanes keep rendering, and the ledger/operator account is
// untouched.
func TestDegradationFooterSelfDisclosedLaneSuppressed(t *testing.T) {
	// The word identity the suppression rides on: section display pair ==
	// lane registry pair for the backfill token. If either side is
	// renamed without the other, suppression silently dies — pin it.
	pair, ok := degradedSectionDisplayNames[tool.DegradedSectionCitationQuoteBackfill]
	if !ok {
		t.Fatalf("backfill token missing from degradedSectionDisplayNames")
	}
	spec := types.DegradationLaneRegistry[types.DegradeLaneCitationQuoteRewrite]
	if pair[0] != spec.ZH || pair[1] != spec.EN {
		t.Fatalf("section display words %v must equal the citation lane registry words {%s,%s} — the doc-derived suppression keys on this identity", pair, spec.ZH, spec.EN)
	}

	mu := seededDegradationMutable() // citation ×17 + facet softened ×1
	ctx := &types.AgentContext{Mutable: mu}
	// The degraded doc carries the self-disclosing caveat exactly as the
	// production recovery lanes append it (same builder, same tokens).
	degradedDoc := degradationFooterTestDoc()
	degradedDoc.Caveats = append(degradedDoc.Caveats,
		degradedDeterministicSectionsCaveat("zh", []string{"observation_board", tool.DegradedSectionCitationQuoteBackfill}))
	footer := renderDegradationDisclosureFooter(ctx, degradedDoc, "zh")
	if strings.Contains(footer, "引用摘录回填") {
		t.Fatalf("self-disclosed citation lane must not disclose again on the footer: %q", footer)
	}
	if !strings.Contains(footer, "必答面硬转软 ×1") {
		t.Fatalf("other lanes must keep rendering: %q", footer)
	}
	// End-to-end on the degraded doc: the caveat wording appears exactly
	// once (the caveat itself), never a second time via the footer.
	rendered := RenderAnswerDocumentWithLastMileSupplements(ctx, degradedDoc, nil, "zh")
	if n := strings.Count(rendered, "引用摘录回填"); n != 1 {
		t.Fatalf("degraded doc must carry the backfill word exactly once (caveat only, no footer repeat), got %d:\n%s", n, rendered)
	}
	// The EN caveat face suppresses too (predicate checks both faces).
	degradedEN := degradationFooterTestDoc()
	degradedEN.Caveats = append(degradedEN.Caveats,
		degradedDeterministicSectionsCaveat("en", []string{tool.DegradedSectionCitationQuoteBackfill}))
	if footer := renderDegradationDisclosureFooter(ctx, degradedEN, "en"); strings.Contains(footer, "citation quote backfill") {
		t.Fatalf("EN degraded caveat must suppress the EN footer entry too: %q", footer)
	}
	// Telemetry untouched: the ledger still carries the full account for
	// the operator "[degrade] ledger:" line.
	entries := types.BuildDegradationLedgerView(mu)
	found := false
	for _, entry := range entries {
		if entry.Lane == types.DegradeLaneCitationQuoteRewrite && entry.Count == 17 {
			found = true
		}
	}
	if !found {
		t.Fatalf("suppression is footer-only — the ledger count must survive: %+v", entries)
	}
	// A degraded caveat WITHOUT the backfill section suppresses nothing:
	// the footer stays the sole disclosure (per-Run rewrite events
	// semantic, design ledger CLASS 5 §10.7).
	noBackfill := degradationFooterTestDoc()
	noBackfill.Caveats = append(noBackfill.Caveats,
		degradedDeterministicSectionsCaveat("zh", []string{"observation_board"}))
	if footer := renderDegradationDisclosureFooter(&types.AgentContext{Mutable: seededDegradationMutable()}, noBackfill, "zh"); !strings.Contains(footer, "引用摘录回填 ×17") {
		t.Fatalf("without the self-disclosing caveat the footer stays the disclosure of record: %q", footer)
	}
}

// TestDegradationFooterQuotingCaveatDoesNotSuppress — R8-1 (round-8
// sweep): doc.Caveats is an LLM-WRITABLE free-text channel, and the
// R7-1 predicate did strings.Contains over it — a caveat merely QUOTING
// the degraded-caveat leader plus a lane word (e.g. prose explaining
// that an upstream run had shipped degraded) falsely suppressed a
// legitimate footer: disclosure loss on a non-degraded answer
// (noisy-signal-for-hard-behavior class). The system materializer
// always constructs the leader at position 0 of its caveat entry
// (degradedDeterministicSectionsCaveat returns prefix+details), so
// suppression now anchors on the entry START (strings.HasPrefix); an
// LLM quote embeds the leader mid-text and must never suppress.
func TestDegradationFooterQuotingCaveatDoesNotSuppress(t *testing.T) {
	ctx := &types.AgentContext{Mutable: seededDegradationMutable()} // citation ×17 + facet ×1
	quoting := degradationFooterTestDoc()
	quoting.Caveats = append(quoting.Caveats,
		"注意：上游批次日志中曾出现「降级出厂说明：本稿未通过最终结构化校验。」字样并触发引用摘录回填，本稿不受其影响。")
	footer := renderDegradationDisclosureFooter(ctx, quoting, "zh")
	if !strings.Contains(footer, "引用摘录回填 ×17") {
		t.Fatalf("a caveat QUOTING the degraded leader mid-text must not suppress the footer disclosure: %q", footer)
	}
	// EN face: same shape, quoted mid-text.
	quotingEN := degradationFooterTestDoc()
	quotingEN.Caveats = append(quotingEN.Caveats,
		`Note: an upstream batch printed "Degraded delivery: this draft did NOT pass final structured validation." and ran citation quote backfill; this answer is unaffected.`)
	if footer := renderDegradationDisclosureFooter(ctx, quotingEN, "en"); !strings.Contains(footer, "citation quote backfill ×17") {
		t.Fatalf("EN quoting caveat must not suppress the footer disclosure: %q", footer)
	}
	// Discriminator: the genuine system caveat (leader at position 0 of
	// the entry) still suppresses — kills an over-broad fix that stops
	// suppressing altogether.
	genuine := degradationFooterTestDoc()
	genuine.Caveats = append(genuine.Caveats,
		degradedDeterministicSectionsCaveat("zh", []string{tool.DegradedSectionCitationQuoteBackfill}))
	if footer := renderDegradationDisclosureFooter(ctx, genuine, "zh"); strings.Contains(footer, "引用摘录回填") {
		t.Fatalf("the genuine degraded caveat must still suppress the lane: %q", footer)
	}
}

// TestDegradationFooterRejectedRecoveryThenCleanRetryDiscloses — R7-1
// (round-7 sweep): a recovery attempt builds a degraded draft whose own
// caveat self-discloses the citation lane, but that draft is REJECTED
// and a later attempt re-emits a CLEAN answer document. The clean answer
// carries no self-disclosing degraded caveat, so the footer MUST
// disclose the lane — suppression must never outlive the rejected draft
// (R6-5's marker did: it was set at MATERIALIZE time on run state, so
// the clean retry rendered footer-suppressed with no caveat either —
// total disclosure loss, red-first against that shape). Doc-derived
// suppression is stateless by construction; this pin kills any future
// reintroduction of run-level suppression state.
func TestDegradationFooterRejectedRecoveryThenCleanRetryDiscloses(t *testing.T) {
	mu := seededDegradationMutable() // citation ×17 + facet softened ×1
	ctx := &types.AgentContext{Mutable: mu}
	// Recovery attempt: the degraded draft renders (its caveat suppresses
	// the citation lane on ITS OWN render) — then it is rejected and never
	// ships.
	rejected := degradationFooterTestDoc()
	rejected.Caveats = append(rejected.Caveats,
		degradedDeterministicSectionsCaveat("zh", []string{"observation_board", tool.DegradedSectionCitationQuoteBackfill}))
	draft := RenderAnswerDocumentWithLastMileSupplements(ctx, rejected, nil, "zh")
	if n := strings.Count(draft, "引用摘录回填"); n != 1 {
		t.Fatalf("degraded draft must self-disclose exactly once (no footer repeat), got %d:\n%s", n, draft)
	}
	// Later attempt on the SAME run state: a clean re-emitted answer
	// document renders. Nothing in THIS document self-discloses, so the
	// footer is the disclosure of record and must carry the citation lane.
	got := RenderAnswerDocumentWithLastMileSupplements(ctx, degradationFooterTestDoc(), nil, "zh")
	if !strings.Contains(got, "引用摘录回填 ×17") {
		t.Fatalf("clean retry after a rejected recovery draft must disclose the citation lane on the footer:\n%s", got)
	}
}

// TestDegradedMaterializeCallSitesAppendSelfDisclosingCaveat —
// structural pairing pin for R6-5, repointed by R7-1: every production
// call of tool.MaterializeDeterministicAnswerSectionsForDegradedDoc in
// this package must be followed (within a few lines) by
// degradedDeterministicSectionsCaveat on the same materialized list —
// the caveat appended to the outgoing doc is now BOTH the user-facing
// self-disclosure AND the footer's suppression carrier
// (degradationLaneSelfDisclosedInDoc), so a call site that materializes
// without appending it would reintroduce the co-surface double
// disclosure.
func TestDegradedMaterializeCallSitesAppendSelfDisclosingCaveat(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	const materializeToken = "MaterializeDeterministicAnswerSectionsForDegradedDoc("
	const caveatToken = "degradedDeterministicSectionsCaveat("
	callSites := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		lines := strings.Split(string(raw), "\n")
		for i, line := range lines {
			if !strings.Contains(line, materializeToken) {
				continue
			}
			callSites++
			paired := false
			for j := i; j < len(lines) && j <= i+12; j++ {
				if strings.Contains(lines[j], caveatToken) {
					paired = true
					break
				}
			}
			if !paired {
				t.Errorf("%s:%d calls %s without a %s pairing within 12 lines — the outgoing doc would lack the self-disclosing caveat and the degraded surface and footer would double-disclose the citation lane",
					name, i+1, materializeToken, caveatToken)
			}
		}
	}
	if callSites == 0 {
		t.Fatal("no degraded materializer call sites found — pin is vacuous (token drifted?)")
	}
}

// TestTRUNCDegradationFooterChokepointStructural extends the TRUNC
// family: the footer builder is called from EXACTLY ONE production
// site — inside renderAnswerDocumentWithLastMileSupplements — so no
// alternate render path can double-print it and no path can drop it
// (the chokepoint itself is already pinned by
// TestTRUNCFinalAnswerRerenderChokepointStructural).
func TestTRUNCDegradationFooterChokepointStructural(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	const callToken = "renderDegradationDisclosureFooter("
	callSites := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := string(raw)
		occurrences := strings.Count(src, callToken)
		if name == "degradation_disclosure_footer.go" {
			// Definition file: the func declaration only, no self-calls.
			if occurrences != 1 {
				t.Fatalf("definition file must contain exactly the declaration, got %d occurrences", occurrences)
			}
			continue
		}
		if occurrences == 0 {
			continue
		}
		if name != "answer_document_evaluator.go" || occurrences != 1 {
			t.Fatalf("%s calls renderDegradationDisclosureFooter %d time(s) — the ONLY production call site is the last-mile chokepoint in answer_document_evaluator.go (TRUNC discipline: bypass render paths lose system lines)", name, occurrences)
		}
		// And that single call must sit inside the chokepoint function
		// body, not some sibling helper.
		start := strings.Index(src, "func (e *answerDocumentEvaluator) renderAnswerDocumentWithLastMileSupplements(")
		if start < 0 {
			t.Fatalf("chokepoint function not found in answer_document_evaluator.go")
		}
		body := src[start:]
		if end := strings.Index(body[1:], "\nfunc "); end >= 0 {
			body = body[:end+1]
		}
		if !strings.Contains(body, callToken) {
			t.Fatalf("the footer call in answer_document_evaluator.go is OUTSIDE renderAnswerDocumentWithLastMileSupplements — it must live inside the chokepoint body")
		}
		callSites++
	}
	if callSites != 1 {
		t.Fatalf("expected exactly 1 production call site inside the chokepoint, found %d", callSites)
	}
}

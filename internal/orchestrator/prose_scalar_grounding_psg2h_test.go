package orchestrator

// PSG-2H pins (§29.10-2 ruling + §29.8 792-witness table,
// docs/design/real_trace_campaign_20260705.md, 2026-07-10). Covers:
//
//	entity arm — fabricated thread name caught by literal spelling+tid
//	  matching (dh-irq-bind-4-93 vs the real dh-irq-bind-0-89);
//	  MUTATION KILL: relaxing detection to substring containment lets a
//	  boundary-truncated fabrication (dh-irq-bind-4-93 against
//	  dh-irq-bind-4-931) ship — the pin turns red;
//	  loose dispositions — same-tid alias, no thread inventory, short
//	  hyphenated technical terms — all stay silent.
//	sum extension — a 139.615-class cross-thread sum stated for one
//	  named thread raises a binding mismatch; a summand on the named
//	  thread keeps it silent.
//	membership witnesses — VSync 50.756×8 (unlocatable single-period
//	  claim) and the 113.067-vs-112.175 same-report contradiction.
//	caveat backstop — second round still violating → deterministic
//	  system-channel caveat, pinned VERBATIM (zh + en); repaired final
//	  doc → no caveat; clean run → answer byte-identical (zero cost);
//	  MUTATION KILL: deleting the caveat backstop in favour of
//	  re-raising (infinite retry) breaks the structural round bound —
//	  the gate can never raise more than once per run.

import (
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func psg2hThreadRecord(id, subject, value string, notes ...string) types.ObservationRecord {
	rec := psgTraceRecord(id, "state_drilldown:"+subject+":s_sleep", value, notes...)
	rec.Subject = subject
	return rec
}

// TestPSG2H_FabricatedThreadNameWitness — §29.8 huadong witness: the
// prose names dh-irq-bind-4-93 while the only real spelling on the
// evidence face is dh-irq-bind-0-89 (one character apart, different
// tid). The entity arm must catch it and the hint must name the token.
func TestPSG2H_FabricatedThreadNameWitness(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "dh-irq-bind-0-89", "38.996"))
	bus := psgBus(mut)
	doc := psgProseDoc("中断绑定线程 dh-irq-bind-4-93 在窗口内被反复抢占。")

	got := runProseScalarGroundingCheck(doc, bus, mut)
	if len(got) != 1 {
		t.Fatalf("fabricated thread name must raise exactly one violation, got %d: %+v", len(got), got)
	}
	v := got[0]
	if v.Kind != types.ViolProseScalarUngrounded {
		t.Fatalf("kind = %q", v.Kind)
	}
	if !strings.Contains(v.Detail, "dh-irq-bind-4-93") ||
		!strings.Contains(v.Detail, "thread identity token") {
		t.Fatalf("Detail must name the fabricated thread token:\n%s", v.Detail)
	}
	if !strings.Contains(v.Repair, "never assemble or adjust a thread name or id") {
		t.Fatalf("Repair must carry the per-token thread directive:\n%s", v.Repair)
	}
	if isStrictViolationForBus(v, bus) {
		t.Fatalf("S3' ①: the PSG-2H raise must never be strict for the bus")
	}
}

// TestPSG2H_SubstringRelaxationMutationKill — mutation pin (§29.10-2
// 突变形①): the evidence face publishes dh-irq-bind-4-931 (tid 931);
// the prose token dh-irq-bind-4-93 is a strict SUBSTRING of that
// spelling but matches neither the spelling nor any published tid. A
// detection relaxed to substring containment would ground it — this
// pin must stay red under that mutant.
func TestPSG2H_SubstringRelaxationMutationKill(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "dh-irq-bind-4-931", "38.996"))
	bus := psgBus(mut)
	doc := psgProseDoc("线程 dh-irq-bind-4-93 的等待段占据窗口大半。")

	got := runProseScalarGroundingCheck(doc, bus, mut)
	if len(got) != 1 || !strings.Contains(got[0].Detail, "dh-irq-bind-4-93") {
		t.Fatalf("a substring of a real spelling with an unpublished tid is still a fabrication, got %+v", got)
	}
}

// TestPSG2H_SameTidAliasStaysSilent — loose disposition: trace surfaces
// publish the same thread under truncated / aliased names, so a prose
// spelling with a PUBLISHED tid never raises (tid-equality agreement,
// the same rule the PSG-2 binding arm uses).
func TestPSG2H_SameTidAliasStaysSilent(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "com.xs.fm.lite-6565", "38.996"))
	bus := psgBus(mut)
	doc := psgProseDoc("主线程 fm.lite-6565 长期处于睡眠状态。")

	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("a published tid grounds an aliased spelling, got %+v", got)
	}
}

// TestPSG2H_NoThreadInventoryStaysSilent — loose disposition: when the
// evidence face publishes NO thread token at all, absence cannot be
// asserted and the entity arm stays inert.
func TestPSG2H_NoThreadInventoryStaysSilent(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "opendir_total", "38.996"))
	bus := psgBus(mut)
	doc := psgProseDoc("线程 ghost.thread-777 多次进入等待。")

	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("no thread inventory on the face → entity arm inert, got %+v", got)
	}
}

// TestPSG2H_ShortTechnicalTermsExempt — noise dampener pin: short
// hyphenated technical terms (x86-64, arm-64, sha-256) never enter the
// audit even when the face has a thread inventory.
func TestPSG2H_ShortTechnicalTermsExempt(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "app.main-42591", "38.996"))
	bus := psgBus(mut)
	doc := psgProseDoc("该库在 x86-64 与 arm-64 平台均出现，校验和为 sha-256。")

	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("short hyphenated technical terms are exempt, got %+v", got)
	}
}

// TestPSG2H_CrossThreadSumWitness — §29.8 witness: 139.615ms is the sum
// of two values published for two OTHER threads (69.200 + 70.415), and
// the prose states it for a named third thread — the pair-sum grounding
// no longer silences the misbinding. The named thread itself is real
// (published elsewhere), isolating the SUM misbinding from the entity
// arm.
func TestPSG2H_CrossThreadSumWitness(t *testing.T) {
	mut := psgTraceMutable(
		psg2hThreadRecord("r1", "kworker.io-200", "69.200"),
		psg2hThreadRecord("r2", "hmfs.writer-300", "70.415"),
		psg2hThreadRecord("r3", "target.main-100", "5.000"),
	)
	bus := psgBus(mut)
	doc := psgProseDoc("线程 target.main-100 的跨窗等待合计 139.615ms。")

	got := runProseScalarGroundingCheck(doc, bus, mut)
	if len(got) != 1 {
		t.Fatalf("cross-thread sum must raise exactly one violation, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Detail, "139.615ms") ||
		!strings.Contains(got[0].Detail, "is a sum of values published for thread(s)") ||
		!strings.Contains(got[0].Detail, "target.main-100") {
		t.Fatalf("Detail must expose the cross-thread sum shape:\n%s", got[0].Detail)
	}

	// Control: one summand published for the NAMED thread → agreement,
	// silent (宁松勿严).
	mut2 := psgTraceMutable(
		psg2hThreadRecord("r1", "kworker.io-200", "69.200"),
		psg2hThreadRecord("r2", "target.main-100", "70.415"),
	)
	doc2 := psgProseDoc("线程 target.main-100 的跨窗等待合计 139.615ms。")
	if got := runProseScalarGroundingCheck(doc2, psgBus(mut2), mut2); len(got) != 0 {
		t.Fatalf("a summand on the named thread keeps the sum silent, got %+v", got)
	}
}

// TestPSG2H_VSyncUnlocatableScalarWitness — §29.8 witness: the prose
// claims a per-period "50.756ms ×8" while the face publishes only the
// 406.048 total — the single-period figure is unlocatable and must be
// listed by the membership arm.
func TestPSG2H_VSyncUnlocatableScalarWitness(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "dh-VSyncGenerator-2270", "406.048"))
	bus := psgBus(mut)
	doc := psgProseDoc("VSync 周期性等待约 50.756ms ×8 次，覆盖整个窗口。")

	got := runProseScalarGroundingCheck(doc, bus, mut)
	if len(got) != 1 || !strings.Contains(got[0].Detail, "50.756ms") {
		t.Fatalf("the unlocatable per-period figure must be listed, got %+v", got)
	}
}

// TestPSG2H_SameReportContradictionWitness — §29.8 opendir witness: the
// prose totals 113.067ms while the face publishes 112.175ms — the
// unpublished side of a same-report contradiction is caught by
// membership.
func TestPSG2H_SameReportContradictionWitness(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "opendir.worker-4411", "112.175"))
	bus := psgBus(mut)
	doc := psgProseDoc("opendir 自身区间合计 113.067ms，而明细行合计 112.175ms。")

	got := runProseScalarGroundingCheck(doc, bus, mut)
	if len(got) != 1 || !strings.Contains(got[0].Detail, "113.067ms") {
		t.Fatalf("the unpublished side of the contradiction must be listed, got %+v", got)
	}
	if strings.Contains(got[0].Detail, "112.175ms (block") {
		t.Fatalf("the published side must NOT be listed:\n%s", got[0].Detail)
	}
}

// psg2hDeliverHint simulates the orchestrator dispatching the forced
// rewrite round: the typed retry surface carries the PSG kind, and the
// next gate pass marks the sticky run latch.
func psg2hDeliverHint(t *testing.T, mut *types.MutableState, bus *types.BusContext, doc *types.AnswerDocumentV2) {
	t.Helper()
	mut.SetRetryState(&types.RetryState{Attempt: 1, ActiveViolations: []types.ScoredViolation{{
		Kind: types.ViolProseScalarUngrounded, Severity: types.SeverityMedium, Layer: "answer_oracle",
	}}})
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("the hint round is final, got %+v", got)
	}
	if !mut.ProseScalarGroundingHintDelivered() {
		t.Fatalf("hint delivery must set the sticky run latch")
	}
}

// TestPSG2H_SecondRoundResidualCaveatVerbatim — §29.10-2 layer ③: the
// forced round was consumed and the shipped document STILL carries the
// fabricated tokens → the ship chokepoint appends the deterministic
// system-channel caveat listing them, VERBATIM, and the answer body
// above it is untouched.
func TestPSG2H_SecondRoundResidualCaveatVerbatim(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "dh-irq-bind-0-89", "38.996"))
	bus := psgBus(mut)
	doc := psgProseDoc("聚合影响达 46.821ms，另见线程 dh-irq-bind-4-93。")

	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 1 {
		t.Fatalf("first pass must raise, got %+v", got)
	}
	psg2hDeliverHint(t, mut, bus, doc)

	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	o := &Orchestrator{busCtx: bus}
	answer := o.appendProseScalarResidualCaveatToAnswer("正文结论。")
	if !strings.HasPrefix(answer, "正文结论。") {
		t.Fatalf("the body must never be rewritten:\n%s", answer)
	}
	wantCaveat := "- 以下数值/实体未能定位于本报告的证据面（观测记录、结构化指标、投影表或引用行），请谨慎采信：46.821ms；dh-irq-bind-4-93。"
	if !strings.Contains(answer, "**补充说明：**") || !strings.Contains(answer, wantCaveat) {
		t.Fatalf("residual caveat must be appended verbatim, got:\n%s", answer)
	}

	// EN branch verbatim pin.
	en := proseScalarResidualCaveatMessage("en", []string{"46.821ms", "dh-irq-bind-4-93"})
	wantEN := "The following values/entities in the answer could not be located on this report's evidence surfaces (observation records, structured facts, projection tables, or quoted lines); treat them with caution: 46.821ms; dh-irq-bind-4-93."
	if en != wantEN {
		t.Fatalf("EN caveat drifted:\nwant %q\ngot  %q", wantEN, en)
	}
}

// TestPSG2H_RepairedFinalDocNoCaveat — the caveat reads the SHIPPED
// document: when the forced rewrite actually fixed the tokens, the
// ship chokepoint appends nothing even though the latch is set.
func TestPSG2H_RepairedFinalDocNoCaveat(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "dh-irq-bind-0-89", "38.996"))
	bus := psgBus(mut)
	bad := psgProseDoc("聚合影响达 46.821ms，另见线程 dh-irq-bind-4-93。")
	if got := runProseScalarGroundingCheck(bad, bus, mut); len(got) != 1 {
		t.Fatalf("first pass must raise, got %+v", got)
	}
	psg2hDeliverHint(t, mut, bus, bad)

	repaired := psgProseDoc("线程 dh-irq-bind-0-89 睡眠 38.996ms，为窗口主导状态。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, repaired)
	o := &Orchestrator{busCtx: bus}
	if answer := o.appendProseScalarResidualCaveatToAnswer("正文结论。"); answer != "正文结论。" {
		t.Fatalf("a repaired final doc must ship with no caveat, got:\n%s", answer)
	}
}

// TestPSG2H_CleanRunZeroCostZeroCaveat — 正常报告零开销零 caveat: a run
// where the gate never fired ships its answer byte-identical through
// the caveat chokepoint (the typed latch is the activation signal).
func TestPSG2H_CleanRunZeroCostZeroCaveat(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "dh-irq-bind-0-89", "38.996"))
	bus := psgBus(mut)
	doc := psgProseDoc("线程 dh-irq-bind-0-89 睡眠 38.996ms。")
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("clean report must raise nothing, got %+v", got)
	}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	o := &Orchestrator{busCtx: bus}
	const body = "正文结论。"
	if answer := o.appendProseScalarResidualCaveatToAnswer(body); answer != body {
		t.Fatalf("clean run must ship byte-identical, got:\n%s", answer)
	}
}

// TestPSG2H_StructuralRoundBoundNoLivelock — mutation pin (§29.10-2
// 突变形②): deleting the caveat backstop in favour of letting the gate
// re-raise turns the one forced round into an unbounded loop. The
// structural bound: across ANY number of post-delivery rounds with the
// violations still present, the gate raises ZERO further violations —
// total raises per run ≤ 1 (plus same-attempt rechecks before
// delivery, which repeat the SAME single raise deterministically).
func TestPSG2H_StructuralRoundBoundNoLivelock(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "dh-irq-bind-0-89", "38.996"))
	bus := psgBus(mut)
	doc := psgProseDoc("聚合影响达 46.821ms，另见线程 dh-irq-bind-4-93。")

	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 1 {
		t.Fatalf("first pass must raise, got %+v", got)
	}
	psg2hDeliverHint(t, mut, bus, doc)

	// Rounds 2..6: violations persist, retry surfaces rotate through
	// other kinds — the gate must stay silent on every one of them.
	for round := 2; round <= 6; round++ {
		mut.SetRetryState(&types.RetryState{Attempt: round, ActiveViolations: []types.ScoredViolation{{
			Kind: types.ViolBlockCoverageMissing, Severity: types.SeverityHigh, Layer: "v2_oracle",
		}}})
		if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
			t.Fatalf("round %d: the gate re-raised after delivery — livelock shape, got %+v", round, got)
		}
	}
	// The residual findings remain available for the ship-time caveat.
	if labels := proseScalarResidualFindingLabels(doc, bus, mut); len(labels) == 0 {
		t.Fatalf("post-delivery residuals must surface through the caveat lane, not retries")
	}
}

// TestPSG2H_CapPreemptDisclosure — adversarial review P1 (2026-07-10):
// when the FRCAP total-round cap fires BEFORE the PSG forced rewrite
// was ever dispatched (latch cold — 792 witness shape: two pre-finalize
// scheduler retries + default cap=2 preempt the PSG round), the PSG
// kind has no caveat family and the generic residual-concerns
// materializer renders NOTHING for it. The cap-preempt lane must
// disclose through PSG-2H's own generator, latch-independent.
// MUTATION KILL: deleting the cap-branch disclosure call turns this red.
func TestPSG2H_CapPreemptDisclosure(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "dh-irq-bind-0-89", "38.996"))
	bus := psgBus(mut)
	o := &Orchestrator{busCtx: bus}
	doc := psgProseDoc("聚合影响达 46.821ms，另见线程 dh-irq-bind-4-93。")

	capViolations := runProseScalarGroundingCheck(doc, bus, mut)
	if len(capViolations) != 1 {
		t.Fatalf("premise: PSG raise expected, got %+v", capViolations)
	}
	if !violationsContainProseScalarKind(capViolations) {
		t.Fatalf("gate helper must detect the PSG kind")
	}
	if mut.ProseScalarGroundingHintDelivered() {
		t.Fatalf("premise: cap-preempt means the latch was never set")
	}
	// The generic residual-concerns materializer produces zero
	// user-visible words for a PSG-only set — the review's P1 finding.
	if bullets := MaterializeResidualConcernDetails(capViolations, "zh"); len(bullets) != 0 {
		t.Fatalf("premise drift: the generic materializer now renders PSG concerns (%v) — re-audit the cap-preempt lane's necessity", bullets)
	}

	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	answer := "正文结论。"
	if violationsContainProseScalarKind(capViolations) {
		answer = o.appendProseScalarCapPreemptCaveatToAnswer(answer)
	}
	if !strings.Contains(answer, "以下数值/实体未能定位于本报告的证据面") ||
		!strings.Contains(answer, "46.821ms") ||
		!strings.Contains(answer, "dh-irq-bind-4-93") {
		t.Fatalf("cap-preempt lane must disclose the PSG tokens, latch-independent:\n%s", answer)
	}
	if mut.ProseScalarGroundingHintDelivered() {
		t.Fatalf("the disclosure lane must not flip the retry latch")
	}

	// Dedup with the ship-exit lane (best-draft swap-back shape: latch
	// set AND the cap branch already disclosed): no second copy.
	mut.MarkProseScalarGroundingHintDelivered()
	answer = o.appendProseScalarResidualCaveatToAnswer(answer)
	if got := strings.Count(answer, "以下数值/实体未能定位于本报告的证据面"); got != 1 {
		t.Fatalf("cap-preempt + ship-exit must disclose exactly once, got %d:\n%s", got, answer)
	}
}

// TestPSG2H_CapPreemptCleanDocNoCaveat — the cap-preempt lane re-scans
// the SHIPPED document: when the FRCAP best-draft restore swapped to a
// draft that grounds everything, the lane appends nothing even though
// the violation set names the PSG kind (recorded from another round).
func TestPSG2H_CapPreemptCleanDocNoCaveat(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "dh-irq-bind-0-89", "38.996"))
	bus := psgBus(mut)
	o := &Orchestrator{busCtx: bus}
	clean := psgProseDoc("线程 dh-irq-bind-0-89 睡眠 38.996ms。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, clean)
	const body = "正文结论。"
	if answer := o.appendProseScalarCapPreemptCaveatToAnswer(body); answer != body {
		t.Fatalf("a grounded shipped doc must ship with no cap-preempt caveat, got:\n%s", answer)
	}
}

// TestPSG2HCapPreemptDisclosureWiredStructural — wiring pin (mutation
// form: "cap-branch disclosure call deleted"). The behavioral pins
// above exercise the LANE; this pin holds the P6 hard-cap branch
// WIRING: between the hard-cap gate and the W2.6 cross-scope gate,
// orchestrator.go must call the cap-preempt lane guarded by the typed
// PSG-kind check. Same grep-pin discipline as
// TestTRUNCFinalAnswerRerenderChokepointStructural.
func TestPSG2HCapPreemptDisclosureWiredStructural(t *testing.T) {
	src, err := os.ReadFile("orchestrator.go")
	if err != nil {
		t.Fatalf("read orchestrator.go: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "o.finalizeRepairHardCapValue(); hardCap > 0")
	if start < 0 {
		t.Fatalf("P6 hard-cap gate not found")
	}
	end := strings.Index(body[start:], "W2.6")
	if end < 0 {
		t.Fatalf("W2.6 cross-scope marker not found after the hard-cap gate")
	}
	branch := body[start : start+end]
	if !strings.Contains(branch, "violationsContainProseScalarKind(capViolations)") ||
		!strings.Contains(branch, "o.appendProseScalarCapPreemptCaveatToAnswer(out.FinalAnswer)") {
		t.Fatalf("the P6 hard-cap branch must disclose PSG residuals through the cap-preempt lane (guarded by the typed kind check); branch body:\n%s", branch)
	}
}

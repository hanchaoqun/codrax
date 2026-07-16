package agent

// XGAP-FIX ②/③/④ pins (§29.104.7/.8, witness 20260715-202022.323-89609).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func xgapMemberSetRejectObs(fingerprint string, success bool) LoopObservation {
	tr := &types.ToolResult{ToolName: "emit_answer_document_patch", Success: success}
	if !success {
		tr.Repair = &types.ToolRepair{
			Code: "answer_doc_pre_emit_contract",
			Metadata: map[string]string{
				types.ToolRepairMetaMemberSetMissingFingerprint: fingerprint,
			},
		}
	}
	return LoopObservation{LastToolResult: tr}
}

func TestMemberSetCoverageRejectBreaker_AlternatingFingerprintsAccumulate(t *testing.T) {
	// The XGAP witness alternated two contradictory obligation sets
	// A/B/A/B — a consecutive-identical streak (F7 shape) never trips on
	// that. Strikes must accumulate per fingerprint across interleaving.
	e := &answerDocumentEvaluator{}
	seq := []string{"fpA", "fpB", "fpA", "fpB", "fpA", "fpB", "fpA"}
	for i, fp := range seq[:6] {
		if sig := e.memberSetCoverageRejectBreakerSignal(xgapMemberSetRejectObs(fp, false)); sig.StopRequested {
			t.Fatalf("breaker must not trip before the strike budget (reject %d, fp=%s)", i+1, fp)
		}
	}
	// 7th reject = 4th strike of fpA → exceeds the default budget (3).
	sig := e.memberSetCoverageRejectBreakerSignal(xgapMemberSetRejectObs(seq[6], false))
	if !sig.StopRequested {
		t.Fatal("4th identical missing-obligation strike must trip the breaker")
	}
	if !strings.Contains(sig.StopReason, "member-set coverage reject breaker") {
		t.Fatalf("unexpected stop reason: %q", sig.StopReason)
	}
}

func TestMemberSetCoverageRejectBreaker_SuccessResetsOtherCausesDoNot(t *testing.T) {
	e := &answerDocumentEvaluator{}
	for i := 0; i < 3; i++ {
		if sig := e.memberSetCoverageRejectBreakerSignal(xgapMemberSetRejectObs("fpA", false)); sig.StopRequested {
			t.Fatalf("strike %d must still be paid", i+1)
		}
	}
	// A reject WITHOUT the fingerprint (different cause) neither counts
	// nor clears (the alternation lesson).
	other := LoopObservation{LastToolResult: &types.ToolResult{
		ToolName: "emit_answer_document",
		Success:  false,
		Repair:   &types.ToolRepair{Code: "answer_doc_blocks_required"},
	}}
	if sig := e.memberSetCoverageRejectBreakerSignal(other); sig.StopRequested {
		t.Fatal("different-cause reject must not trip this breaker")
	}
	if sig := e.memberSetCoverageRejectBreakerSignal(xgapMemberSetRejectObs("fpA", false)); !sig.StopRequested {
		t.Fatal("strikes must survive interleaved different-cause rejects")
	}

	// A successful emit clears everything.
	e2 := &answerDocumentEvaluator{}
	for i := 0; i < 3; i++ {
		e2.memberSetCoverageRejectBreakerSignal(xgapMemberSetRejectObs("fpA", false))
	}
	e2.memberSetCoverageRejectBreakerSignal(xgapMemberSetRejectObs("", true))
	if sig := e2.memberSetCoverageRejectBreakerSignal(xgapMemberSetRejectObs("fpA", false)); sig.StopRequested {
		t.Fatal("a successful emit must reset accumulated strikes")
	}
}

func TestSetFinalizerMemberSetBreakerMaxStrikes_NonPositiveFallsBack(t *testing.T) {
	t.Cleanup(func() { SetFinalizerMemberSetBreakerMaxStrikes(0) })
	SetFinalizerMemberSetBreakerMaxStrikes(5)
	if memberSetCoverageBreakerMaxStrikes != 5 {
		t.Fatalf("knob must apply, got %d", memberSetCoverageBreakerMaxStrikes)
	}
	SetFinalizerMemberSetBreakerMaxStrikes(-1)
	if memberSetCoverageBreakerMaxStrikes != memberSetCoverageBreakerMaxStrikesDefault {
		t.Fatalf("non-positive must fall back to default, got %d", memberSetCoverageBreakerMaxStrikes)
	}
}

func TestR10FinalizerBudgetDefaults(t *testing.T) {
	// R10 (§29.104.7): validation retry budget default 3 → 4, which lifts
	// the derived reject-hint budget max(n×4, 8) from 12 to 16.
	settings := types.DefaultAgentSettings()
	if settings.FinalizerMaxCorrectionRetries != 4 {
		t.Fatalf("R10: FinalizerMaxCorrectionRetries default must be 4, got %d", settings.FinalizerMaxCorrectionRetries)
	}
	e := &answerDocumentEvaluator{maxRetries: settings.FinalizerMaxCorrectionRetries}
	if got := e.rejectHintBudget(); got != 16 {
		t.Fatalf("R10: derived reject-hint budget must be 16, got %d", got)
	}
}

func TestFinalizerLoopPolicyIdenticalErrorStreakRaised(t *testing.T) {
	// R10: the same-error-CLASS force-stop (the gate that actually killed
	// the XGAP witness loop) rises to 4 for the finalizer ONLY.
	deps := &Dependencies{AgentSettings: types.DefaultAgentSettings()}
	ag, ok := NewFinalizerAgent(deps).(*BaseAgent)
	if !ok {
		t.Fatal("finalizer is expected to be a BaseAgent")
	}
	if finalizerIdenticalErrorStreak != 4 {
		t.Fatalf("R10 pins finalizerIdenticalErrorStreak at 4, got %d", finalizerIdenticalErrorStreak)
	}
	if got := ag.deps.LoopPolicy.IdenticalErrorStreak; got != finalizerIdenticalErrorStreak {
		t.Fatalf("finalizer IdenticalErrorStreak must be %d, got %d", finalizerIdenticalErrorStreak, got)
	}
	// Other knobs keep the default policy values.
	if ag.deps.LoopPolicy.MaxContinuations != DefaultLoopPolicy().MaxContinuations {
		t.Fatalf("finalizer policy must inherit the default beyond the raised streak, got %+v", ag.deps.LoopPolicy)
	}
	// Every other agent keeps the generic default (3).
	if DefaultLoopPolicy().IdenticalErrorStreak != 3 {
		t.Fatalf("generic IdenticalErrorStreak must stay 3, got %d", DefaultLoopPolicy().IdenticalErrorStreak)
	}
}

func TestDegradedDeterministicSectionsCaveat_UserReadableSections(t *testing.T) {
	// 修补轮 件C pin (2026-07-16): the degraded footer must speak
	// user-readable section names — internal snake_case tokens
	// (observation_board, artifact_quote_check, …) never ride the user
	// surface; unknown tokens fail open to the generic word instead of an
	// invented name.
	tokens := []string{"observation_board", "artifact_quote_check", "zz_unknown_probe"}

	zh := degradedDeterministicSectionsCaveat("zh", tokens)
	for _, want := range []string{"运行时观测板", "引用核对", "确定性板块"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh footer must carry %q, got %q", want, zh)
		}
	}
	en := degradedDeterministicSectionsCaveat("en", tokens)
	for _, want := range []string{"runtime observation board", "citation quote check", "deterministic section"} {
		if !strings.Contains(en, want) {
			t.Fatalf("EN footer must carry %q, got %q", want, en)
		}
	}
	for name, caveat := range map[string]string{"zh": zh, "en": en} {
		for _, token := range tokens {
			if strings.Contains(caveat, token) {
				t.Fatalf("%s footer leaks internal token %q: %q", name, token, caveat)
			}
		}
		if strings.Contains(caveat, "_") {
			t.Fatalf("%s footer must carry no snake_case remnants: %q", name, caveat)
		}
	}

	// Two unknown tokens collapse into ONE generic word (no stutter).
	zhDup := degradedDeterministicSectionsCaveat("zh", []string{"zz_a", "zz_b"})
	if strings.Count(zhDup, "确定性板块") != 1 {
		t.Fatalf("unknown tokens must dedupe to one generic word, got %q", zhDup)
	}

	// The empty-list wording is unchanged.
	if !strings.Contains(degradedDeterministicSectionsCaveat("zh", nil), "本次无适用板块") {
		t.Fatal("zh empty-list wording regressed")
	}
	if !strings.Contains(degradedDeterministicSectionsCaveat("en", nil), "none applicable to this run") {
		t.Fatal("EN empty-list wording regressed")
	}

	// Every token the degraded export can actually produce is mapped — a
	// materializer rename must consciously visit the display table.
	for _, token := range []string{
		"causal_projection", "semantic_optimization", "metric_snapshot",
		"next_steps", "perf_quality", "observation_board",
		"supplement_disclosure", "report_hierarchy",
		"citation_quote_backfill", "artifact_quote_check",
	} {
		if _, ok := degradedSectionDisplayNames[token]; !ok {
			t.Fatalf("materializer token %q missing from the display-name table", token)
		}
	}
}

func TestParseOutput_RetryStateRecoveryLane_TypedDegradedAndFooter(t *testing.T) {
	// ④ pin: the witness lane ("rendered previous retry-state document")
	// must now ship typed-degraded, carry the deterministic-sections
	// footer sentence, and record the shipped draft on the degraded
	// carrier for the ⑤ prose defenses.
	mu := types.NewMutableState("xgap degraded lane")
	mu.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID: "sum", Kind: types.BlockSummary, Text: "上一版结构化草稿正文。",
		}},
	})
	ctx := &types.AgentContext{AgentName: types.AgentFinalizer, Mutable: mu}
	e := &answerDocumentEvaluator{mu: mu}
	out, err := e.ParseOutput(ctx, []llm.Message{{Role: "assistant", Content: "最后一轮原文散文。"}}, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if !out.AnswerDegraded || !out.SkipAnswerChecks {
		t.Fatalf("retry-state recovery must ship typed-degraded, got degraded=%v skip=%v", out.AnswerDegraded, out.SkipAnswerChecks)
	}
	if out.DegradeReason != "answer_document_retry_state_recovered" {
		t.Fatalf("unexpected degrade reason %q", out.DegradeReason)
	}
	if !strings.Contains(out.FinalAnswer, "降级出厂说明") {
		t.Fatalf("degraded footer sentence missing from answer:\n%s", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "上一版结构化草稿正文。") {
		t.Fatalf("recovered draft content missing:\n%s", out.FinalAnswer)
	}
	if mu.DegradedRecoveredAnswerDocumentV2() == nil {
		t.Fatal("shipped degraded draft must land on the degraded carrier for the prose defenses")
	}
	if mu.AnswerDocumentV2() != nil {
		t.Fatal("recovery must never promote the draft to the validated carrier")
	}
}

func TestParseOutput_HealthyLaneStaysUndegraded(t *testing.T) {
	mu := types.NewMutableState("healthy lane")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID: "sum", Kind: types.BlockSummary, Text: "validated answer body.",
		}},
	})
	ctx := &types.AgentContext{AgentName: types.AgentFinalizer, Mutable: mu}
	e := &answerDocumentEvaluator{mu: mu}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnswerDegraded || out.SkipAnswerChecks || out.DegradeReason != "" {
		t.Fatalf("healthy lane must stay undegraded, got %+v", out)
	}
	if strings.Contains(out.FinalAnswer, "降级出厂说明") {
		t.Fatalf("healthy lane must not carry the degraded footer:\n%s", out.FinalAnswer)
	}
}

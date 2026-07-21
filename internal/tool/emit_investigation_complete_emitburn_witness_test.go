package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EMITBURN-1 weak-model witness (§29.173): replays the customer's four-round
// serial-reject burn as ONE emit call carrying the exact three-error shape —
// a negative_observation missing its origin dimension, the absent-thing
// dimension misnamed "evidence", and a second entry carrying an unknown
// field. The old pipeline surfaced these one retry round at a time
// (round5=origin arm, round6=target arm, round7-8=decode/shape); the new
// single reject must name ALL of them at once, carry the closed near-miss
// rename hint, and teach fix-over-delete.
func TestEmitInvestigationComplete_EMITBURN1WitnessSingleRejectNamesEveryViolation(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"customer replay: three distinct payload defects in one emit",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{"kind":"negative_observation","label":"no binder reply in window","value":"0",
			 "dimensions":[{"name":"evidence","value":"binder transaction reply"}]},
			{"kind":"scalar_value","label":"main-thread blocked span","value":"281.9","unit":"ms","confidence_note":"measured"}
		]
	}`)
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Fatalf("witness payload must be rejected, got success: %s", res.Summary)
	}
	for _, want := range []string{
		// decode-layer violation named (entry 1 unknown field)…
		`unknown field "confidence_note"`,
		// …with the valid entry-field list reflected from the tool schema (件2)
		"aggregate_facts entries accept only these fields: kind, label, value, role",
		// every entry named at once with per-entry indexes (件1)
		"aggregate_facts[0]:",
		"aggregate_facts[1]:",
		// both dimension arms of entry 0 in the SAME reject (the customer
		// burned one round per arm on these two)
		"requires a non-repo origin dimension",
		"requires dimension target, query, pattern, or predicate",
		// closed near-miss rename hint (件3)
		`rename dimension "evidence" to "target"`,
		// complete minimal example rides the arms (件2)
		"minimal valid example",
		// single-reject accumulation framing (件1)
		"fix ALL of them in this one re-emit",
		// anti-deletion teaching (件4)
		"name the dropped entry in reason",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("single reject must contain %q, got: %s", want, res.Summary)
		}
	}
}

// The same witness minus the unknown-field entry lands on the normalize lane:
// the reject headline stays the serial gate's first error (zero
// verdict-semantics change) while the accumulated listing names every failing
// arm of the fact — origin, target, and scope — in one message.
func TestEmitInvestigationComplete_EMITBURN1NormalizeLaneAccumulatesAllArms(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"customer replay round two: remaining dimension-arm defects",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{"kind":"negative_observation","label":"no binder reply in window","value":"0",
			 "dimensions":[{"name":"evidence","value":"binder transaction reply"}]},
			{"kind":"scalar_value","label":"main-thread blocked span","value":"281.9","unit":"ms"}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if res.Success {
		t.Fatalf("payload with missing hard dimensions must be rejected, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "emit_investigation_complete rejected: aggregate_facts[0]:") {
		t.Fatalf("headline must stay the serial gate's first error, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "The payload has 3 violations") {
		t.Fatalf("accumulated listing must count all three arms, got: %s", res.Summary)
	}
	for _, want := range []string{
		"[1] aggregate_facts[0]:",
		"[2] aggregate_facts[0]:",
		"[3] aggregate_facts[0]:",
		"requires a non-repo origin dimension",
		"requires dimension target, query, pattern, or predicate",
		"requires dimension scope=<bounded observed surface>",
		`rename dimension "evidence" to "target"`,
		"minimal valid example",
		"fix ALL of them in this one re-emit",
		"name the dropped entry in reason",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("accumulated reject must contain %q, got: %s", want, res.Summary)
		}
	}
}

// The accumulate walker mints message content only: its first element must be
// byte-identical to the serial gate's reject error, pinning that EMITBURN-1
// changed zero validation semantics.
func TestEmitInvestigationComplete_EMITBURN1CollectFirstElementMatchesSerialGate(t *testing.T) {
	facts := []types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateNegativeObservation,
			Label: "no binder reply in window",
			Value: "0",
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "evidence", Value: "binder transaction reply"},
			},
		},
		{Kind: types.AnswerAggregateTotalCount, Label: "call sites", Value: "not-a-number"},
	}
	_, serialErr := types.NormalizeAnswerAggregateFacts(facts)
	if serialErr == nil {
		t.Fatalf("serial gate must reject the witness facts")
	}
	violations := types.CollectAnswerAggregateFactsViolations(facts)
	if len(violations) < 2 {
		t.Fatalf("walker must report violations from more than the first failing arm, got %d: %v", len(violations), violations)
	}
	if violations[0] != serialErr.Error() {
		t.Fatalf("walker first element must match the serial gate error:\n serial: %s\n walker: %s", serialErr.Error(), violations[0])
	}
}

// EMITBURN-1 件5 pins: the negative_observation origin auto-fill widens to a
// MIXED repo+artifact run ONLY when the fact itself anchors to the artifact
// (artifact_id / trace_window). Run-level attachment alone must NOT stamp
// origin — a bare negative_observation in a mixed run stays ambiguous between
// a repo-search absence and an artifact observation.
func TestEmitInvestigationComplete_EMITBURN1MixedRunOriginInjectionPins(t *testing.T) {
	originOf := func(fact types.AnswerAggregateFact) string {
		for _, dim := range fact.Dimensions {
			if strings.EqualFold(strings.TrimSpace(dim.Name), "origin") {
				return dim.Value
			}
		}
		return ""
	}
	anchored := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateNegativeObservation,
		Label: "no wakeup in window",
		Value: "0",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "target", Value: "wakeup"},
			{Name: "trace_window", Value: "10.0..10.2"},
		},
	}
	bare := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateNegativeObservation,
		Label: "no wakeup anywhere",
		Value: "0",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "target", Value: "wakeup"},
		},
	}

	mixed := &types.BusContext{Mutable: types.NewMutableState("q"), AttachedHitrace: "systrace excerpt"}
	out, _ := normalizeCompletionNegativeObservationFacts(mixed, []types.AnswerAggregateFact{anchored, bare})
	if got := originOf(out[0]); got != string(types.AnswerEvidenceOriginRuntimeArtifact) {
		t.Fatalf("artifact-anchored fact in a mixed run must get origin=runtime_artifact, got %q", got)
	}
	if got := originOf(out[1]); got != "" {
		t.Fatalf("bare fact in a mixed run must NOT get an injected origin, got %q", got)
	}

	noArtifact := &types.BusContext{Mutable: types.NewMutableState("q")}
	out, _ = normalizeCompletionNegativeObservationFacts(noArtifact, []types.AnswerAggregateFact{anchored})
	if got := originOf(out[0]); got != "" {
		t.Fatalf("without an attached runtime artifact no origin may be injected, got %q", got)
	}
}

// RUN2AUDIT-1 nine-attempt replay (§29.174, /Users/han/opt/customlogs/
// runnable_2.txt): the customer session spent NINE emit_investigation_complete
// calls — four probe-window rejects (L30/L33/L36/L40: origin arm, then the
// "evidence"-vs-target arm, then remaining shape errors one at a time), three
// member_set contract rejects (L50 evidence subsection, L69/L72 validate
// subsection: count-only member_set), and two accepts (L56/L76). This fixture
// replays each attempt shape against one MutableState in a trace-attached
// context and asserts (a) every reject now teaches the complete remaining
// lesson in ONE message, (b) member_set rejects carry the by-name minimal
// example, and (c) the two historically accepted shapes STILL accept — zero
// validation-semantics change.
func TestEmitInvestigationComplete_EMITBURN1RunnableTwoNineAttemptReplay(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut, AttachedHitrace: "systrace excerpt"}
	tool := &EmitInvestigationComplete{}
	emit := func(t *testing.T, aggregates string) types.ToolResult {
		t.Helper()
		res, _ := tool.Execute(bus, json.RawMessage(`{
			"reason":"replayed customer attempt",
			"confidence":"high",
			"result_kind":"resolved",
			"aggregate_facts":`+aggregates+`}`))
		return res
	}
	requireReject := func(t *testing.T, res types.ToolResult, wants ...string) {
		t.Helper()
		if res.Success {
			t.Fatalf("attempt must be rejected, got success: %s", res.Summary)
		}
		for _, want := range wants {
			if !strings.Contains(res.Summary, want) {
				t.Fatalf("reject must contain %q, got: %s", want, res.Summary)
			}
		}
	}

	// Attempt 1 (L30): origin missing, absent thing under dimension
	// "evidence", second entry carries an unknown field. One reject names
	// every violation.
	res := emit(t, `[
		{"kind":"negative_observation","label":"no binder reply in window","value":"0",
		 "dimensions":[{"name":"evidence","value":"binder transaction reply"}]},
		{"kind":"scalar_value","label":"main-thread blocked span","value":"281.9","unit":"ms","confidence_note":"measured"}]`)
	requireReject(t, res,
		`unknown field "confidence_note"`,
		"requires a non-repo origin dimension",
		"requires dimension target, query, pattern, or predicate",
		`rename dimension "evidence" to "target"`,
		"name the dropped entry in reason")

	// Attempt 2 (L33, customer fixed only origin): the remaining target arm
	// plus the unknown-field entry are still both named at once.
	res = emit(t, `[
		{"kind":"negative_observation","label":"no binder reply in window","value":"0",
		 "dimensions":[{"name":"origin","value":"runtime_artifact"},{"name":"evidence","value":"binder transaction reply"}]},
		{"kind":"scalar_value","label":"main-thread blocked span","value":"281.9","unit":"ms","confidence_note":"measured"}]`)
	requireReject(t, res,
		`unknown field "confidence_note"`,
		"requires dimension target, query, pattern, or predicate",
		`rename dimension "evidence" to "target"`)

	// Attempt 3 (L36, customer fixed target): only the unknown-field entry
	// remains; the reject names it with the valid entry-field list.
	res = emit(t, `[
		{"kind":"negative_observation","label":"no binder reply in window","value":"0",
		 "dimensions":[{"name":"origin","value":"runtime_artifact"},{"name":"target","value":"binder transaction reply"}]},
		{"kind":"scalar_value","label":"main-thread blocked span","value":"281.9","unit":"ms","confidence_note":"measured"}]`)
	requireReject(t, res,
		`unknown field "confidence_note"`,
		"aggregate_facts entries accept only these fields: kind, label, value, role")

	// Attempt 4 (L40, "remove or fix"): a remaining count fact carries a
	// unit-suffixed measurement value. AUTOREPAIR-1 (§29.175
	// T2-UNIT-SUFFIX-SPLIT ruling) flipped this attempt from reject to a
	// disclosed Tier2 repair: the exact lexical split lands kind→scalar_value
	// with value/unit separated, so the round no longer burns — the flip IS
	// the ruled behavior, asserted here against the matrix wording (this is
	// not an acceptance-test loosening: the IsCountQuestion carve-out and the
	// unit-conflict arm keep the hard reject, pinned in
	// emit_investigation_complete_autorepair_test.go).
	res = emit(t, `[
		{"kind":"total_count","label":"vsync 到 doFrame 延迟","value":"119.320ms"}]`)
	if !res.Success {
		t.Fatalf("unit-suffixed measurement in a non-count-question run must repair, not reject (§29.175 T2-UNIT-SUFFIX-SPLIT), got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, `kind normalized total_count→scalar_value;值 "119.320ms" 拆为 value=119.320 unit=ms(计数类须整数)`) {
		t.Fatalf("repair must disclose the split in the accepted summary, got: %s", res.Summary)
	}

	// Attempt 5 (L50, evidence subsection): count-only member_set — the
	// reject demands members by name and shows the complete minimal example.
	res = emit(t, `[
		{"kind":"member_set","label":"blocking chain principal threads","value":"4","role":"principal_answer"}]`)
	requireReject(t, res,
		"requires exact members listed by name in members[]",
		`"kind":"member_set"`,
		"minimal valid example",
		"Fix the failing entries in place rather than deleting them")

	// Attempt 6 (L56): the historically accepted shape still accepts.
	res = emit(t, `[
		{"kind":"member_set","label":"blocking chain principal threads","value":"4","role":"principal_answer",
		 "members":["ThreadPoolForeg-60555","NetworkService-60595","CookieMonsterCl-59843","com.baidu.tieba-59566"]}]`)
	if !res.Success {
		t.Fatalf("historically accepted member_set handoff must still accept (zero semantics change), got: %s", res.Summary)
	}

	// Attempt 7 (L69, validate subsection): count-only member_set again.
	res = emit(t, `[
		{"kind":"member_set","label":"根因链路成员","value":"5","role":"principal_answer"}]`)
	requireReject(t, res,
		"requires exact members listed by name in members[]",
		"minimal valid example")

	// Attempt 8 (L72): still count-only (empty members array this time).
	res = emit(t, `[
		{"kind":"member_set","label":"根因链路成员","value":"5","role":"principal_answer","members":[]}]`)
	requireReject(t, res,
		"requires exact members listed by name in members[]",
		"minimal valid example")

	// Attempt 9 (L76): the second historically accepted shape still accepts.
	res = emit(t, `[
		{"kind":"member_set","label":"根因链路成员","value":"5","role":"principal_answer",
		 "members":["com.baidu.tieba-59566","CookieMonsterCl-59843","NetworkService-60595","ThreadPoolForeg-60555","Chrome_IOThread-60560"]}]`)
	if !res.Success {
		t.Fatalf("final accepted handoff must still accept (zero semantics change), got: %s", res.Summary)
	}
}

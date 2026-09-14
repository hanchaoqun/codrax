package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These are typed producer fixtures, not live model runs. Every assertion below
// crosses both public context assembly and the finalizer instruction entrypoint.
func b1670BlockingScopeRecords(capture, subject string, start float64, independent string) []types.ObservationRecord {
	end := start + .020
	window := fmt.Sprintf("selected_window=%.6f..%.6f", start, end)
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: capture, ArtifactKind: "trace", Path: "/captures/" + capture + ".ftrace"}
	row := func(id, predicate, who, object, value string, lo, hi float64, notes ...string) types.ObservationRecord {
		return types.ObservationRecord{
			ID: capture + "-" + id, Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
			Subject: who, Predicate: predicate, ClaimKey: predicate + ":" + who, Object: object, Value: value, Unit: "ms",
			Span:      types.ObservationSpan{StartTs: lo, EndTs: hi, LineStart: 1, LineEnd: 20},
			RichNotes: append([]string{window}, notes...),
		}
	}
	rows := []types.ObservationRecord{
		row("state", "target_window_states", subject, "state_partition", "20.000", start, end,
			"running=5.000", "runnable=1.000", "sleep=14.000", "d_state=0.000", "io_wait=0.000", "sleep_io_wait=0.000", "total=20.000"),
		row("work", "root_cause_primary", "worker-200", "running", "5.000", start+.001, start+.006,
			"rank=1", "tier=primary", "chain_relevance=on_chain", "chain_depth=1", "impact_ms=5.000", "effective_impact_ms=2.000", "fix_direction=self_workload",
			types.TraceNoteKeyRankBoardTarget+"="+subject, types.TraceNoteKeyRankBoardParams+"=b1670-board"),
	}
	if independent == "binder" || independent == "both" {
		inventory := row("inventory", "target_binder_wait_inventory", subject, "indexed_target_verified_closed_waits", "2.000", start, end)
		inventory.Summary = "Verified closed Binder waits: 1, union=2.000ms; unresolved=0, unassociated=0. Index scan complete; not all waits or roots."
		rows = append(rows, inventory, row("binder", "critical_blocking", subject, "binder_wait", "2.000", start+.006, start+.008, "type=binder_wait", "capacity_truncated=false"))
	}
	if independent == "io" || independent == "both" {
		rows = append(rows, row("io", "io_latency", subject, "block_io", "1.000", start+.012, start+.013,
			"completion_woke_issuer=true", "causal_wait_caliber=completion_closed_issuer_blocked", "issuer_blocked_state=s_sleep", "issuer_blocked=1.000",
			fmt.Sprintf("issuer_blocked_start=%.6f", start+.012), fmt.Sprintf("issuer_blocked_end=%.6f", start+.013), "capacity_truncated=false"))
	}
	return rows
}

func b1670BlockingScopePublicContext(records []types.ObservationRecord, lang string) *types.AgentContext {
	start, end := 10.0, 10.020
	mutable := types.NewMutableState("Explain the response delay in the selected capture and window")
	mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: records}}})
	mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model-summary", Kind: types.BlockSummary, Text: "The model owns this conclusion and any remaining uncertainty."}}})
	bus := &types.BusContext{Language: lang, Mutable: mutable, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Language: lang, Intent: types.IntentRootCause,
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "client-100", Source: "user_explicit"}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10.000000..10.020000"},
	}}}
	return promptcontext.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
}

func b1670ScopeLines(prompt, prefix string) string {
	var lines []string
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func b1670ScopeSnapshot(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	ledger := answerDocObservationLedger(ctx)
	value, err := json.Marshal([]any{ctx.Mutable.TurnAArtifacts(), ctx.Mutable.AnswerDocumentV2(), ctx.AnalysisIR.RequestModel,
		types.CompileTraceCausalProjectionSet(ledger), types.BuildTraceBlockingWallClockAuthorities(ledger, &ctx.AnalysisIR.RequestModel)})
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}

func TestB1670PublicFinalizerScopesProjectionAbsenceWithoutNegatingIndependentWaits(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		for _, independent := range []string{"none", "binder", "io", "both"} {
			t.Run(lang+"/"+independent, func(t *testing.T) {
				ctx := b1670BlockingScopePublicContext(b1670BlockingScopeRecords("capture-A", "client-100", 10, independent), lang)
				before := b1670ScopeSnapshot(t, ctx)
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				compact := b1670ScopeLines(prompt, "- compact_authority")
				mechanism := b1670ScopeLines(prompt, "- final_answer_mechanism_scope")
				// Premises are positive checks on the actual emitted prompt: no
				// vacuous RED caused by a fixture that never reaches either tail.
				for _, want := range []string{"target=`client-100`", "target_direct_blocking_authority=`not_provided_by_projection`", "direct_blocking_decision=`not_established`", "wakeup_path_blocking_authority=`not_implied`"} {
					if !strings.Contains(compact, want) {
						t.Fatalf("missing compact fixture premise %q: %s", want, compact)
					}
				}
				if !strings.Contains(mechanism, "subject=`worker-200`; target=`client-100`") {
					t.Fatalf("missing leader fixture premise: %s", mechanism)
				}
				if independent == "binder" || independent == "both" {
					if !strings.Contains(prompt, "verified_wait_union=2.000ms") {
						t.Fatal("independent Binder evidence was not delivered")
					}
				}
				if independent == "io" || independent == "both" {
					accounts := types.BuildTraceBlockingWallClockAuthorities(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
					found := false
					for _, a := range accounts {
						if a.Type == "block_io_completion_closed_issuer_wait" && a.ObservedMS > .999 && a.ObservedMS < 1.001 {
							found = true
						}
					}
					if !found || !strings.Contains(prompt, "proven_blocking_wall_clock=1.000ms") {
						t.Fatal("independent completion-closed IO evidence was not delivered")
					}
				}
				for _, text := range []string{compact, mechanism} {
					if !strings.Contains(text, "same capture, target, and window") || !strings.Contains(text, "independently proved waits, including Binder or completion-closed IO") {
						t.Errorf("projection-local absence must not erase independent scoped wait evidence: %s", text)
					}
					if !strings.Contains(text, "do not by themselves establish a holder relation or root-cause eligibility") {
						t.Errorf("independent waits must not automatically grant relation/root authority: %s", text)
					}
				}
				if !strings.Contains(compact, "this projection provides no typed waiter/holder relation for this target") {
					t.Errorf("compact absence overstates its inspected domain: %s", compact)
				}
				if !strings.Contains(mechanism, "From this row and the projection's waiter/holder surface alone") {
					t.Errorf("leader ceiling overstates its inspected domain: %s", mechanism)
				}
				for _, forbidden := range []string{"say that no typed direct blocker was established for this target", "No typed target-blocking relation establishes that the target waited for this work"} {
					if strings.Contains(prompt, forbidden) {
						t.Errorf("global absence instruction contradicts independent wait evidence: %q", forbidden)
					}
				}
				for _, want := range []string{"Model-Owned Conclusion", "actual", "effective", "20.000ms", "wakeup peer, IRQ peer, kernel caller, adjacent row, or another thread's blocking interval"} {
					if !strings.Contains(prompt, want) {
						t.Errorf("existing ownership/two-axis/window/relation guidance missing %q", want)
					}
				}
				if before != b1670ScopeSnapshot(t, ctx) {
					t.Fatal("prompt rendering changed observations, request, projection or wait accounts")
				}
			})
		}
	}
}

func TestB1670PublicFinalizerRetainsTypedWaiterHolderIdentityGates(t *testing.T) {
	for _, lane := range []string{"same-target", "unknown-holder", "foreign-target", "foreign-capture", "missing-target", "out-of-window"} {
		t.Run(lane, func(t *testing.T) {
			rows := b1670BlockingScopeRecords("capture-A", "client-100", 10, "both")
			blocker := rows[1]
			blocker.ID, blocker.Subject, blocker.Predicate, blocker.Object = "blocker", "client-100", "critical_blocking", "monitor_contention"
			blocker.ClaimKey = "critical_blocking:client-100"
			blocker.RichNotes = []string{"selected_window=10.000000..10.020000", "blocking_kind=monitor_contention", "peer=holder-300", "chain_relevance=on_chain"}
			switch lane {
			case "unknown-holder":
				blocker.RichNotes[2] = "peer=unknown-thread"
			case "foreign-target":
				blocker.Subject = "other-900"
			case "foreign-capture":
				blocker.SourceRef.ArtifactID, blocker.SourceRef.Path = "capture-B", "/captures/capture-B.ftrace"
			case "missing-target":
				rows = rows[1:]
			case "out-of-window":
				blocker.Span.StartTs, blocker.Span.EndTs = 9, 9.005
			}
			rows = append(rows, blocker)
			ctx := b1670BlockingScopePublicContext(rows, "en")
			before := b1670ScopeSnapshot(t, ctx)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			compact := b1670ScopeLines(prompt, "- compact_authority")
			positive := lane == "same-target" || lane == "unknown-holder"
			if strings.Contains(compact, "target_direct_blocking_authority=`typed_waiter_holder`") != positive {
				t.Fatalf("typed relation identity gate changed in %s: %s", lane, compact)
			}
			if positive {
				holder := "holder-300"
				if lane == "unknown-holder" {
					holder = "unresolved"
				}
				for _, want := range []string{"waiter=`client-100`", "holder=`" + holder + "`", "blocking_kind=`monitor_contention`", "direct_blocking_decision=`established_by_typed_relation`"} {
					if !strings.Contains(compact, want) {
						t.Errorf("positive relation lost %q: %s", want, compact)
					}
				}
				if b1670ScopeLines(prompt, "- final_answer_mechanism_scope") != "" {
					t.Fatal("negative leader reminder overwrote positive relation")
				}
			} else if lane == "missing-target" {
				if !strings.Contains(compact, "unavailable_without_typed_target") {
					t.Fatal("missing target was guessed from independent waits")
				}
				for _, want := range []string{"This projection provides no typed target for its waiter/holder relation check", "Independently proved waits retain their own capture, target, and window"} {
					if !strings.Contains(compact, want) {
						t.Errorf("missing projection target was generalized to unrelated evidence: %s", compact)
					}
				}
				if !strings.Contains(prompt, "verified_wait_union=2.000ms") || !strings.Contains(prompt, "proven_blocking_wall_clock=1.000ms") {
					t.Fatal("missing projection target erased independently scoped waits")
				}
			} else if !strings.Contains(compact, "not_provided_by_projection") {
				t.Fatal("negative relation premise missing")
			}
			if before != b1670ScopeSnapshot(t, ctx) {
				t.Fatal("identity test mutated typed inputs")
			}
		})
	}
}

func TestB1670PublicFinalizerKeepsTwoCaptureRelationScopesIndependent(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			rows := b1670BlockingScopeRecords("capture-A", "client-100", 10, "both")
			other := b1670BlockingScopeRecords("capture-B", "client-100", 20, "both")
			blocker := other[1]
			blocker.ID, blocker.Subject, blocker.ClaimKey, blocker.Predicate, blocker.Object = "capture-B-blocker", "client-100", "critical_blocking:client-100", "critical_blocking", "monitor_contention"
			blocker.RichNotes = []string{"selected_window=20.000000..20.020000", "blocking_kind=monitor_contention", "peer=holder-300", "chain_relevance=on_chain"}
			rows = append(rows, append(other, blocker)...)
			if reverse {
				for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
					rows[i], rows[j] = rows[j], rows[i]
				}
			}
			ctx := b1670BlockingScopePublicContext(rows, "en")
			before := b1670ScopeSnapshot(t, ctx)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			a := b1670ScopeLines(prompt, "- compact_authority artifact=`capture-A.ftrace`: target=")
			b := b1670ScopeLines(prompt, "- compact_authority artifact=`capture-B.ftrace`: target=")
			if !strings.Contains(a, "not_provided_by_projection") || strings.Contains(a, "typed_waiter_holder") {
				t.Fatalf("capture-A borrowed capture-B relation: %s", a)
			}
			if !strings.Contains(b, "typed_waiter_holder") || !strings.Contains(b, "holder=`holder-300`") {
				t.Fatalf("capture-B lost its own typed relation: %s", b)
			}
			mechanism := b1670ScopeLines(prompt, "- final_answer_mechanism_scope")
			if !strings.Contains(mechanism, "artifact=`capture-A.ftrace`") || strings.Contains(mechanism, "artifact=`capture-B.ftrace`") {
				t.Fatalf("negative leader reminder crossed capture/window identity: %s", mechanism)
			}
			if before != b1670ScopeSnapshot(t, ctx) {
				t.Fatal("multi-capture prompt changed facts, scopes or model answer")
			}
		})
	}
}

package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Equal duration, issuer, completer and interval do not identify a request.
// This exercises native parsing and publication before the actual public emit
// and finalizer paths, including the renderer's duplicate safety net.
func TestHMC233PublicIORequestIdentityAndRulerAcrossSurfaces(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, renamed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/renamed=%t", lang, renamed), func(t *testing.T) {
				issuer, peer := "issuer", "irq"
				if renamed {
					issuer, peer = "storage-client", "completion-service"
				}
				body := fmt.Sprintf("%s-41 (41) [001] .... 5.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [%s]\n"+
					"%s-41 (41) [001] .... 5.000000: block_rq_issue: 8,0 R 4096 () 456 + 8 [%s]\n"+
					"%s-41 (41) [001] .... 5.000000: block_bio_queue: 8,0 R 789 + 8 [%s]\n"+
					"%s-2 (2) [001] .... 5.010000: block_rq_complete: 8,0 R () 123 + 8 [0]\n"+
					"%s-2 (2) [001] .... 5.010000: block_rq_complete: 8,0 R () 456 + 8 [0]\n"+
					"%s-2 (2) [001] .... 5.010000: block_bio_complete: 8,0 R 789 + 8 [0]\n", issuer, issuer, issuer, issuer, issuer, issuer, peer, peer, peer)
				path := filepath.Join(t.TempDir(), "independent-requests.ftrace")
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("Explain the observed storage requests"), Language: lang}
				raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "critical_blocking_calls", "time_start": 4.999, "time_end": 5.011})
				result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
				if err != nil || !result.Success {
					t.Fatalf("query: %v %+v", err, result)
				}
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
				ledger := answerDocObservationLedger(ctx)
				before := b1645Snapshot(t, ledger)
				native := 0
				keys, foldKeys := map[string]bool{}, map[string]bool{}
				for _, r := range ledger.Records {
					if r.Predicate == "critical_blocking" && traceQueryObservationSupplementNoteValue(r, types.TraceNoteKeyType) == "io_latency" {
						native++
						if key, handled := types.TraceObservationIOPublicationKey(r); !handled || key == "" {
							t.Errorf("request receipt lost across ledger: id=%s endpoints=%+v", r.ID, r.IORequestEndpoints)
						}
						keys[traceQueryObservationSupplementKey(r)] = true
						foldKeys[traceQueryObservationSameValueFoldKey(r, filepath.Base(path), "", lang == "zh")] = true
						if r.Value != "10.000" {
							t.Fatalf("equal-duration premise: %+v", r)
						}
					}
				}
				if native != 3 {
					t.Fatalf("native parser must publish three independent requests, got %d", native)
				}
				if len(keys) != 3 || len(foldKeys) != 3 {
					t.Errorf("independent requests collide in supplement identity: exact=%d same-value=%d", len(keys), len(foldKeys))
				}
				set := types.CompileTraceCausalProjectionSet(ledger)
				projected := 0
				for _, p := range set.Projections {
					for _, n := range p.BackgroundCauses {
						if n.TypeToken == "io_latency" {
							projected++
							if n.ImpactMS != 10 || n.DuplicatePublications > 1 || n.MergedCount > 1 || n.ResourceCompletionClosure {
								t.Errorf("independent request changed quantity or acquired issuer-wait proof: id=%s impact=%g duplicates=%d merged=%d closure=%t", n.EvidenceID, n.ImpactMS, n.DuplicatePublications, n.MergedCount, n.ResourceCompletionClosure)
							}
						}
					}
				}
				if projected != 3 {
					t.Errorf("projection merged independent requests: got %d, want 3", projected)
				}
				raw = []byte(`{"document_model":"v2","blocks":[{"id":"summary","kind":"summary","text":"Model-owned storage explanation."}]}`)
				emitted, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
				if err != nil || !emitted.Success {
					t.Fatalf("emit: %v %+v", err, emitted)
				}
				doc := ctx.Mutable.AnswerDocumentV2()
				for _, id := range []string{"runtime_trace_causal_projection", "runtime_trace_causal_projection_detail_full"} {
					found := false
					for _, b := range doc.Blocks {
						if b.ID != id {
							continue
						}
						found = true
						text := types.AnswerBlockVisibleSurface(b)
						for caliber, want := range map[string]int{types.TraceIOValueCaliberRQResidence: 2, types.TraceIOValueCaliberBIOResidence: 1} {
							label := types.TraceIOValueCaliberLabel(caliber, lang == "zh")
							if got := strings.Count(text, label); got < want {
								t.Errorf("%s lost request-local ruler %q: got %d want >= %d\n%s", id, label, got, want, text)
							}
						}
					}
					if !found {
						t.Errorf("missing public native surface %s", id)
					}
				}
				out, err := (&answerDocumentEvaluator{language: lang}).ParseOutput(ctx, nil, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				for _, caliber := range []string{types.TraceIOValueCaliberRQResidence, types.TraceIOValueCaliberBIOResidence} {
					if !strings.Contains(out.FinalAnswer, types.TraceIOValueCaliberLabel(caliber, lang == "zh")) {
						t.Errorf("finalizer lost native ruler %s", caliber)
					}
				}
				if !strings.Contains(out.FinalAnswer, "Model-owned storage explanation.") || before != b1645Snapshot(t, answerDocObservationLedger(ctx)) {
					t.Fatal("native rendering changed model wording or source observations")
				}
			})
		}
	}
}

// One request may have two equal-valued but different measurements. The
// request endpoints, scheduler wait and downstream wakeup must stay distinct.
func TestHMC233PublicSameRequestDifferentRulers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closed-request.ftrace")
	body := "idle-0 (0) [001] .... 4.999990: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20\n" +
		"target-41 (41) [001] .... 5.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [target]\n" +
		"target-41 (41) [001] .... 5.000010: sched_switch: prev_comm=target prev_pid=41 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n" +
		"irq-2 (2) [001] .... 5.010000: block_rq_complete: 8,0 R () 123 + 8 [0]\n" +
		"irq-2 (2) [001] .... 5.010010: sched_wakeup: comm=target pid=41 prio=20 target_cpu=001\n" +
		"idle-0 (0) [001] .... 5.010030: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "en", Mutable: types.NewMutableState("Explain the observed request and target delay")}
	var results []types.ToolResult
	for _, view := range []string{"root_cause_rank", "critical_blocking_calls"} {
		params := map[string]any{"source": "path", "path": path, "view": view, "time_start": 4.99999, "time_end": 5.02, "min_duration_ms": 0.001}
		if view == "critical_blocking_calls" {
			params["pid"] = 41
		}
		raw, _ := json.Marshal(params)
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !result.Success {
			t.Fatalf("native %s failed: %v %s", view, err, result.Summary)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ledger := answerDocObservationLedger(ctx)
	keys := map[string]bool{}
	residence, blocked := false, false
	for _, record := range ledger.Records {
		if record.Unit != "ms" || record.IORequestEndpoints == nil {
			continue
		}
		if !strings.HasPrefix(record.Predicate, "root_cause_") && record.Predicate != "critical_blocking" {
			continue
		}
		if record.Value != "10.000" {
			t.Fatalf("equal-value premise failed: %s=%s", record.Predicate, record.Value)
		}
		pair := record.IORequestEndpoints
		if pair.IssueLine != 2 || pair.CompleteLine != 4 || pair.IssueTs != 5 || pair.CompleteTs != 5.01 {
			t.Fatalf("request identity borrowed wait interval: %+v", pair)
		}
		key, handled := types.TraceObservationIOPublicationKey(record)
		if !handled || key == "" {
			t.Fatalf("native ruler lost authenticated identity: %s", record.ID)
		}
		keys[key] = true
		if strings.HasPrefix(record.Predicate, "root_cause_") {
			residence = true
		} else {
			blocked = true
			if record.Span.LineStart != 3 || record.Span.LineEnd != 5 || traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyIOValueCaliber) != types.TraceIOValueCaliberIssuerBlocked {
				t.Fatal("issuer wait lost its own endpoints or ruler")
			}
		}
	}
	if !residence || !blocked || len(keys) != 2 {
		t.Fatalf("two measurements must remain separate: residence=%t blocked=%t keys=%d", residence, blocked, len(keys))
	}
	_, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), []byte(`{"document_model":"v2","blocks":[{"id":"summary","kind":"summary","text":"Two independently measured intervals."}]}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&answerDocumentEvaluator{language: "en"}).ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, caliber := range []string{types.TraceIOValueCaliberRQResidence, types.TraceIOValueCaliberIssuerBlocked} {
		if !strings.Contains(out.FinalAnswer, types.TraceIOValueCaliberLabel(caliber, false)) {
			t.Errorf("final answer lost ruler %s\n%s", caliber, out.FinalAnswer)
		}
	}
}

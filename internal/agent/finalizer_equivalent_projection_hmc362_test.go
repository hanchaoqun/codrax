package agent

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc362RosterContext(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx := traceRootCauseTestContext("Explain the observed stalls in the two selected trace windows")
	ctx.Stage, ctx.AgentName, ctx.Language = types.StageFinalize, types.AgentFinalizer, "en"
	set := types.TraceCausalProjectionSet{}
	for i, label := range []string{"first.ftrace", "second.ftrace"} {
		projection := types.TraceCausalProjection{ArtifactLabel: label}
		for j := 0; j < 9; j++ {
			projection.RankedSeats = append(projection.RankedSeats, types.TraceCausalProjectionNode{
				EvidenceID: fmt.Sprintf("exact-evidence-%d-%02d", i, j), Subject: fmt.Sprintf("worker-%d", j),
				Rank: j + 1, TypeToken: "scheduler_latency", ChainRelevance: "on_chain", ImpactMS: float64(10 - j),
			})
		}
		set.Projections = append(set.Projections, projection)
	}
	contract, err := tracefinding.CompileCandidateContract(types.ObservationLedger{}, set, tracefinding.SeatFrameCausalityAuthority{})
	if err != nil || len(contract.Candidates) != 18 {
		t.Fatalf("typed roster fixture: %v / %+v", err, contract)
	}
	for i := range contract.Candidates {
		if i == 17 {
			continue // legacy unknown metadata stays absent, not inherited
		}
		start, end := 103.012345, 103.678901
		if i == 8 {
			end = 103.7 // differently scoped row, never another row's requested ruler
		}
		scope := types.TraceQueryWindowScope{RequestedWindowKnown: true, RequestedWindowStartTs: start, RequestedWindowEndTs: 103.678901}.ForWindow(start, end)
		if i == 9 || i == 10 {
			scope = types.TraceQueryWindowScope{} // present but has no note
		}
		d := &contract.Candidates[i].Decision
		if d.EvidenceFacts == nil {
			d.EvidenceFacts = &types.TraceCauseEvidenceFacts{}
		}
		d.EvidenceFacts.WindowScope = &scope
	}
	contract.RootCauseReportEnabled = true
	ctx.Mutable.SetTraceFindingContract(contract)
	return ctx
}

func hmc362CaptureFinalizer(t *testing.T, ctx *types.AgentContext, window int) (*hmc326BudgetCapture, *finalizerPromptBudget, error) {
	t.Helper()
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	capture := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("stop at actual adapter input")}, window: window}
	agent := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1,
		AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}}).(*BaseAgent)
	_, err := agent.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	return capture, agent.eval.(*answerDocumentEvaluator).promptBudget, err
}

func TestHMC362FinalizerPublicLosslessCausalRosterUnderPressure(t *testing.T) {
	ctx := hmc362RosterContext(t)
	original, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
	wide, _, err := hmc362CaptureFinalizer(t, ctx, 2_000_000)
	if wide.calls != 1 || !errors.Is(err, wide.stop) {
		t.Fatalf("roomy baseline did not send: calls=%d %v", wide.calls, err)
	}
	wideText := hmc326DynamicRequestMessage(t, wide)
	full := llm.EstimateRequestBytes(wide.messages, wide.tools)
	// Same evidence and actual tool schemas, at a capacity where the expanded
	// prompt cannot possibly fit but an equivalent compact roster can.
	tight, budget, err := hmc362CaptureFinalizer(t, ctx, (full-1500)*2/7)
	if tight.calls != 1 || !errors.Is(err, tight.stop) {
		t.Fatalf("complete equivalent context should have reached the adapter; full=%d budget=%+v calls=%d err=%v", full, budget, tight.calls, err)
	}
	if got := llm.EstimateRequestBytes(tight.messages, tight.tools); got > budget.RequestAllocationBytes || got >= full-1500 {
		t.Fatalf("bad wire budget: %d / %d (expanded %d)", got, budget.RequestAllocationBytes, full)
	}
	if !reflect.DeepEqual(wide.tools, tight.tools) {
		t.Fatal("compacting display changed the offered candidate enum or tool contract")
	}
	tightText := hmc326DynamicRequestMessage(t, tight)
	wideRows, wideShared := hmc362RosterRows(t, wideText, 2)
	tightRows, tightShared := hmc362RosterRows(t, tightText, 2)
	if len(wideRows) != 2 || wideShared != 0 || tightShared != 2 || !reflect.DeepEqual(wideRows, tightRows) {
		t.Fatal("compact groups lost ordered candidates, evidence, scope, or separate artifact membership")
	}
	for _, wanted := range []string{"The full answer is the primary deliverable", "Background and adjacent observations", "first.ftrace", "second.ftrace"} {
		if !strings.Contains(tightText, wanted) {
			t.Errorf("lost the existing authority boundary %q", wanted)
		}
	}
	after, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
	if string(after) != string(original) || ctx.Mutable.AnswerDocumentV2() != nil || ctx.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("display planning mutated the candidate contract or authored a conclusion")
	}
	blocked, noFit, err := hmc362CaptureFinalizer(t, ctx, (full-18000)*2/7)
	var allocation *AnswerContextByteBudgetError
	if blocked.calls != 0 || !errors.As(err, &allocation) || allocation.ActualBytes <= allocation.AllocatedBytes || noFit.EquivalentGroups != 1 {
		t.Fatalf("equivalent but still oversized evidence must not dispatch: calls=%d budget=%+v err=%v", blocked.calls, noFit, err)
	}
}

// Test-only decoding of rendered JSON. Verify equality of every ordered
// candidate, not just a matching subset of scalars or display words.
func hmc362RosterRows(t *testing.T, prompt string, groupCount int) ([][]map[string]json.RawMessage, int) {
	t.Helper()
	_, tail, ok := strings.Cut(prompt, "## Optional Trace Root Cause JSON")
	if !ok {
		t.Fatal("missing existing sidecar roster heading")
	}
	var groups [][]map[string]json.RawMessage
	shared := 0
	for len(groups) < groupCount {
		_, tail, ok = strings.Cut(tail, "```json\n")
		body, next, closed := strings.Cut(tail, "\n```")
		if !ok || !closed {
			t.Fatal("missing original complete JSON roster")
		}
		tail = next
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(body), &rows); err != nil {
			var compact struct {
				WindowContexts map[string]map[string]json.RawMessage `json:"window_contexts"`
				Candidates     []map[string]json.RawMessage          `json:"candidates"`
			}
			if err := json.Unmarshal([]byte(body), &compact); err != nil {
				t.Fatal(err)
			}
			rows = compact.Candidates
			uses := make(map[string]int)
			for _, row := range rows {
				if ref := row["window_context_ref"]; len(ref) > 0 {
					var id string
					if err := json.Unmarshal(ref, &id); err != nil || compact.WindowContexts[id]["window_scope"] == nil || row["window_scope"] != nil || row["window_scope_note"] != nil {
						t.Fatal("dangling or competing scope ownership")
					}
					uses[id]++
					for key, value := range compact.WindowContexts[id] {
						row[key] = value
					}
					delete(row, "window_context_ref")
				}
			}
			for id := range compact.WindowContexts {
				if uses[id] < 2 {
					t.Fatal("only repeated local scope definitions should be shared")
				}
			}
			shared++
		}
		// Whitespace is not a measurement. UseNumber preserves exact values.
		for _, row := range rows {
			for key, value := range row {
				var datum any
				dec := json.NewDecoder(strings.NewReader(string(value)))
				dec.UseNumber()
				if err := dec.Decode(&datum); err != nil {
					t.Fatal(err)
				}
				row[key], _ = json.Marshal(datum)
			}
		}
		groups = append(groups, rows)
	}
	return groups, shared
}

func TestHMC362EquivalentRangesSurviveDeferredInsertion(t *testing.T) {
	// The deferred section and the original suffix each own their existing
	// separator. A budget decision must not rewrite either boundary.
	const expected = "第一组 small\n\n保留中间证据\n\n\n\n最后一组"
	var b strings.Builder
	trace := newAnswerDocDynamicTrace(nil)
	trace.promptBudget = &finalizerPromptBudget{Policy: types.FinalizerRequestByteEnforce, DynamicAllocatedBytes: len(expected)}
	trace.appendEquivalentSection(&b, "first", func(bool) finalizerTextProjection {
		return finalizerTextProjection{expanded: "第一组 small", compact: "第一组"}
	})
	trace.appendSection(&b, "observation_ledger", func() string { return "保留中间证据" })
	trace.appendEquivalentSection(&b, "last", func(bool) finalizerTextProjection {
		return finalizerTextProjection{expanded: "最后一组 substantial repeated content", compact: "最后一组"}
	})
	got := trace.finishBudgeted(b.String())
	if got != expected || trace.promptBudget.EquivalentGroups != 1 || trace.promptBudget.DynamicPublishedBytes != len(got) {
		t.Fatalf("range relocation or minimum replacement failed: %q %+v", got, trace.promptBudget)
	}
}

// Replay only the published byte-policy failure, not the model or a live
// request. The immutable prior logger frame is supplied by the operator.
func TestHMC362ArchivedRequestExactBudgetReplay(t *testing.T) {
	path := os.Getenv("CODRAX_HMC362_ARCHIVED_REQUEST")
	if path == "" {
		t.Skip("requires sealed §360 finalizer input")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const archivedSHA = "24af6e7a8264d63ef37a2c54e8e9637fb54f6b78974273213c25800de482b28b"
	const allocation, static, schema, tail, role = 700000, 160367, 119506, 62, 4
	const dynamicLimit = allocation - static - schema - tail - role
	full, framed := strings.CutSuffix(string(raw), "\n---\n")
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != archivedSHA || !framed || len(full) != 423812 {
		t.Fatal("archive hash or exact dynamic message framing changed")
	}
	const heading = "## Optional Trace Root Cause JSON"
	start := strings.Index(full, heading)
	if start < 0 {
		t.Fatal("archive missing typed producer section")
	}
	header, fenced, ok := strings.Cut(full[start:], "```json\n")
	if !ok {
		t.Fatal("missing original JSON fence")
	}
	body, _, ok := strings.Cut(fenced, "\n```")
	var rows []traceRootCauseRosterRow
	if !ok || json.Unmarshal([]byte(body), &rows) != nil || len(rows) != 16 {
		t.Fatal("wrong original roster population")
	}
	end := start + len(header) + len("```json\n") + len(body) + len("\n```")
	projection := renderTraceRootCauseRosters(header, []traceRootCauseRosterGroup{{json: []byte(body), compact: compactTraceRootCauseRoster(rows)}}, false, true)
	got, groups, saved := compactEquivalentFinalizerSections(full, []finalizerEquivalentSection{{start: start, end: end, compact: projection}}, dynamicLimit)
	before, _ := hmc362RosterRows(t, full, 1)
	after, shared := hmc362RosterRows(t, got, 1)
	if groups != 1 || shared != 1 || len(got) > dynamicLimit || saved != len(full)-len(got) ||
		!strings.HasPrefix(got, full[:start]) || !strings.HasSuffix(got, full[end:]) || !reflect.DeepEqual(before, after) {
		t.Fatalf("lossless replay did not meet original allocation: published=%d limit=%d saved=%d shared=%d", len(got), dynamicLimit, saved, shared)
	}
	t.Logf("archive_sha256=%s rows=%d groups=%d full=%d->%d request_allocation=%d dynamic=%d->%d dynamic_allocation=%d saved=%d", archivedSHA, len(rows), groups, len(full)+static+schema+tail+role, len(got)+static+schema+tail+role, allocation, len(full), len(got), dynamicLimit, saved)
}

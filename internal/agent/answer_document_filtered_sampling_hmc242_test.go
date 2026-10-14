package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func filteredSamplingContextHMC242(t *testing.T, direct bool, patterns ...string) (*types.AgentContext, types.ToolResult) {
	t.Helper()
	ctx, prepared := samplingBundleContextHMC241(t)
	var path string
	for _, r := range prepared.Observations {
		if p, ok := tool.DecodeTracePerfSamplingTree(r); ok {
			path = p.SourcePath
			if direct {
				path = p.Cohorts[0].SourcePath
			}
			break
		}
	}
	pattern := "symbol=work"
	if len(patterns) == 1 {
		pattern = patterns[0]
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 11, "time_start": 1, "time_end": 2, "pattern": pattern})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("public filtered query: %v %s", err, result.Summary)
	}
	return ctx, result
}

func TestFilteredSamplingExactDSOFinalizerHMC242(t *testing.T) {
	ctx, result := filteredSamplingContextHMC242(t, false, "dso=/app/A.so")
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	before, _ := json.Marshal(result.Observations)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	part := filteredSamplingNativeSectionHMC242(prompt)
	for _, want := range []string{`"samples=2"`, `"sample_weight=15"`, `"sample_weight=100"`, `"sample_weight=7"`, `"weight":15`, `"weight":100`, `"weight":7`} {
		// JSON notes are themselves JSON strings inside this native envelope.
		if !strings.Contains(part, want) && !strings.Contains(part, strings.ReplaceAll(want, `"`, `\"`)) {
			t.Errorf("native filtered display lost %s", want)
		}
	}
	for _, bad := range []string{"/app/B.so", `"sample_weight=900"`, `"sample_weight=20"`} {
		if strings.Contains(part, bad) {
			t.Errorf("native filtered display leaked %s", bad)
		}
	}
	after, _ := json.Marshal(result.Observations)
	if string(before) != string(after) {
		t.Fatal("display mutated original observations")
	}
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
		t.Fatal("filtered display gained causal authority")
	}
}

func TestFilteredSamplingPreflightDecorationHMC242(t *testing.T) {
	ctx, result := filteredSamplingContextHMC242(t, false, "dso=/app/A.so")
	var original types.ObservationRecord
	for _, r := range result.Observations {
		if _, valid, _ := tool.TraceQueryFilteredPerfMemberSelection(r); valid {
			original = r
			break
		}
	}
	ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: original.SourceRef.Path, Carrier: "attachment"}}}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ledger := answerDocObservationLedger(ctx)
	decorated := false
	for _, r := range ledger.Records {
		if r.ID == original.ID {
			decorated = r.SourceRef.ToolCallID != "" && r.SourceRef.CaptureIdentityPath != "" && r.SourceRef.ArtifactID != original.SourceRef.ArtifactID
			if _, valid, _ := tool.TraceQueryFilteredPerfMemberSelection(r); !valid {
				t.Fatal("precise preflight decoration invalidated native selection")
			}
		}
	}
	if !decorated {
		t.Fatal("fixture missed actual ledger/preflight source decoration")
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(filteredSamplingNativeSectionHMC242(prompt), `"sample_weight=15"`) {
		t.Fatal("preflight dropped independently selected native fields")
	}
}

func TestFilteredSamplingBudgetPublicationHMC242(t *testing.T) {
	ctx, first := filteredSamplingContextHMC242(t, false)
	var path string
	for _, r := range first.Observations {
		if _, valid, _ := tool.TraceQueryFilteredPerfMemberSelection(r); valid {
			path = r.SourceRef.Path
			break
		}
	}
	results := []types.ToolResult{first}
	for i := 1; i < 12; i++ {
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 11, "time_start": 1, "time_end": 2, "pattern": "symbol=work", "line_end": 20 + i})
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !r.Success {
			t.Fatalf("query %d: %v %s", i, err, r.Summary)
		}
		results = append(results, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ledger := answerDocObservationLedger(ctx)
	selected, _ := answerDocFinalizerObservationRecords(ctx, ledger.Records)
	promptLedger := ledger
	promptLedger.Records = selected
	publication := &answerDocNativePresentationReceipt{}
	text := renderAnswerDocScopedPerfMemberFacts(ctx, ledger, promptLedger, publication)
	if text == "" || len(text) > 32<<10 {
		t.Fatalf("native field budget not exercised: %d bytes", len(text))
	}
	blocked := false
	for key, complete := range publication.queries {
		if strings.HasPrefix(key, "perf_flat_members:") && !complete {
			blocked = true
		}
	}
	if !blocked || publication.completeFor(ctx) {
		t.Fatal("budget-omitted members acquired full publication or removed narrative")
	}
}

func filteredSamplingNativeSectionHMC242(prompt string) string {
	_, part, found := strings.Cut(prompt, "### Recorded sampling hotspot fields")
	if !found {
		return ""
	}
	if next := strings.Index(part, "\n### "); next >= 0 {
		part = part[:next]
	}
	return part
}

func TestFilteredSamplingFinalizerRequestBoundariesHMC242(t *testing.T) {
	for _, name := range []string{"other_thread", "process_not_thread", "other_window", "missing_receipt", "wrong_source", "wrong_window", "mixed_member"} {
		t.Run(name, func(t *testing.T) {
			ctx, result := filteredSamplingContextHMC242(t, false)
			switch name {
			case "other_thread":
				ctx.AnalysisIR.RequestModel.RuntimeTargets[0].PID = 12
			case "process_not_thread":
				ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Kind = types.RuntimeTargetKindProcess
			case "other_window":
				a, b := 3., 4.
				p := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
				p.TimeStart, p.TimeEnd = &a, &b
				p.TimeWindows = nil
			default:
				changed := false
				for i := range result.Observations {
					r := &result.Observations[i]
					if _, ok, _ := tool.TraceQueryFilteredPerfMemberSelection(*r); !ok {
						continue
					}
					switch name {
					case "missing_receipt":
						var notes []string
						for _, n := range r.RichNotes {
							if !strings.HasPrefix(n, types.TraceNoteKeyPerfFilteredMember+"=") {
								notes = append(notes, n)
							}
						}
						r.RichNotes = notes
					case "wrong_source":
						r.SourceRef.Path += ".other"
					case "wrong_window":
						r.SourceRef.QueryWindowEndTs = 4
					case "mixed_member":
						if !changed {
							r.Value = "424242"
							changed = true
						}
					}
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			part := filteredSamplingNativeSectionHMC242(prompt)
			if name == "mixed_member" {
				if part == "" || strings.Contains(part, "424242") {
					t.Fatal("valid sibling qualified an altered member or lost all valid members")
				}
				return
			}
			if part != "" {
				t.Fatalf("unrequested/unbound filtered facts bypassed shared projection: %s", name)
			}
		})
	}
}

func TestFilteredSamplingActualFinalizerHMC242(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepared_member", true: "direct_file"}[direct], func(t *testing.T) {
			ctx, result := filteredSamplingContextHMC242(t, direct)
			unavailable, members := false, 0
			for _, r := range result.Observations {
				if p, ok := tool.DecodeTracePerfSamplingTree(r); ok {
					unavailable = p.Status == "unavailable" && p.Reason == "requires_time_and_thread_selectors_only"
				}
				if r.Predicate == "perf_sample_top_symbol" {
					members++
				}
			}
			if !unavailable || members != 8 {
				t.Fatalf("fixture tree unavailable=%t native flat members=%d", unavailable, members)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{"### Recorded sampling hotspot fields", `"samples=2"`, `"sample_weight=15"`, `"sample_weight=20"`, "requires_time_and_thread_selectors_only"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("actual finalizer lost %q", want)
				}
			}
		})
	}
}

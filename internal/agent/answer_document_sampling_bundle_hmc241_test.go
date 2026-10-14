package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func samplingBundleContextHMC241(t *testing.T) (*types.AgentContext, types.ToolResult) {
	t.Helper()
	ctx := samplingSelectionAnalysisContext(t)
	dir := t.TempDir()
	child := filepath.Join(dir, "physical-samples.perftrace")
	body := []byte(strings.ReplaceAll(ctx.AttachedHitrace, "tid=11 ", "tid=11 thread_identity_known=true "))
	if err := os.WriteFile(child, body, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	sha := hex.EncodeToString(digest[:])
	capture, err := tracebundle.CaptureID([]tracebundle.CaptureMember{{Type: "perftrace", Path: filepath.Base(child), Bytes: int64(len(body)), SHA256: sha}})
	if err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(map[string]any{"schema": tracebundle.SchemaV2, "capture_id": capture, "artifacts": []any{map[string]any{"type": "perftrace", "path": filepath.Base(child), "bytes": len(body), "sha256": sha, "perf_capability": map[string]any{"time_domain": "trace_seconds", "thread_identity": "sample_pid_tid_thread_comm", "cpu_identity": "sample_cpu", "trace_query_ready": true}}}})
	path := filepath.Join(dir, "prepared.tracebundle.json")
	if err := os.WriteFile(path, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 11, "time_start": 1, "time_end": 2})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("public query: %v %s", err, result.Summary)
	}
	return ctx, result
}

func TestSamplingBundleFiniteFinalizerHMC241(t *testing.T) {
	ctx, result := samplingBundleContextHMC241(t)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal(ledger)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"### 采样前缀树", "total_weight=35", "total_weight=100", "total_weight=7", "### Recorded sampling hotspot fields", `"samples=2"`, `"sample_weight=15"`, `"samples=1"`, `"sample_weight=20"`} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual bounded named-target finalizer lost %q", want)
		}
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("display source bridge changed the full audit ledger")
	}
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
		t.Fatal("sampling member display enabled a causal report")
	}
}

func TestSamplingBundleFiniteFinalizerBoundariesHMC241(t *testing.T) {
	for _, name := range []string{"source", "query", "generation", "member", "window", "target", "process_target", "missing_receipt"} {
		t.Run(name, func(t *testing.T) {
			ctx, result := samplingBundleContextHMC241(t)
			for i := range result.Observations {
				r := &result.Observations[i]
				if r.Predicate != tool.TracePerfSamplingTreePredicate {
					continue
				}
				p, ok := tool.DecodeTracePerfSamplingTree(*r)
				if !ok {
					t.Fatal("invalid native test baseline")
				}
				switch name {
				case "source":
					r.SourceRef.Path += ".other"
					p.SourcePath = r.SourceRef.Path
				case "query":
					r.SourceRef.QueryScopeID += ".other"
				case "generation":
					for j := range p.Cohorts {
						p.Cohorts[j].SourceGeneration += ".other"
					}
				case "member":
					for j := range p.Cohorts {
						p.Cohorts[j].SourcePath += ".other"
					}
				case "window":
					r.SourceRef.QueryWindowEndTs = 3
					r.Span.EndTs = 3
					p.Window.EndTs = 3
				case "target":
					ctx.AnalysisIR.RequestModel.RuntimeTargets[0].PID = 12
				case "process_target":
					ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Kind = types.RuntimeTargetKindProcess
				case "missing_receipt":
					filtered := make([]string, 0, len(r.RichNotes))
					for _, n := range r.RichNotes {
						if !strings.HasPrefix(n, "perf_sampling_source_binding=") {
							filtered = append(filtered, n)
						}
					}
					r.RichNotes = filtered
				}
				replaceSamplingBundleTreeHMC241(t, r, p)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if strings.Contains(prompt, "### 采样前缀树") || strings.Contains(prompt, "### Recorded sampling hotspot fields") {
				t.Fatal("unbound bundle tree laundered flat publication into actual finalizer")
			}
		})
	}
}

func replaceSamplingBundleTreeHMC241(t *testing.T, r *types.ObservationRecord, p tracequery.PerfSamplingTreeResult) {
	t.Helper()
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range r.RichNotes {
		if strings.HasPrefix(n, "perf_sampling_tree=") {
			r.RichNotes[i] = "perf_sampling_tree=" + string(body)
		}
	}
}

package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Public regression: member selection must come from a completed native query,
// not whichever material preparation happened to cache first.
func TestHMC264SuccessfulZIPMemberFeedsSystemSupplement(t *testing.T) {
	hmc176DisableExternalProviders(t)
	suppCoreSetConfig(t, true, 8<<20, 20*time.Second, 120)
	bus, _, conversions := hmc17NamedPathContext(t)
	path := filepath.Join(t.TempDir(), "two-captures.zip")
	original := hmc176Zip(t, map[string][]byte{"a.data": hmc176PerfData(true), "b.data": hmc17NamedBinaryTailFixture()})
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 1.0105, 1.012
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:                      types.IntentRootCause,
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 424242, Thread: "tail-target", Source: "user_explicit", Confidence: 1}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.0105..1.012"},
		AnalyzerHints:               types.AnalyzerHints{ExactTargets: []string{path}},
	}}
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": "b.data", "view": "event_search", "pid": 424242, "time_start": start, "time_end": end})
	payload := hmc17NamedPayload(t, result)
	bus.ToolResults = append(bus.ToolResults, result)
	before, _ := json.Marshal(bus.ToolResults)
	out := RunTraceQuerySystemSupplement(bus)
	if len(out.Executed) == 0 || out.SkipReason != "" {
		t.Fatalf("successful B did not reach ordinary system supplement: %+v", out)
	}
	found := false
	for _, supplemented := range bus.Mutable.SystemTraceSupplementResults() {
		got := hmc17NamedPayload(t, supplemented)
		if got.SourcePath != payload.SourcePath || got.TimeStart != start || got.TimeEnd != end {
			t.Fatalf("supplement changed member/window: source=%s time=%g..%g", got.SourcePath, got.TimeStart, got.TimeEnd)
		}
		if got.RootCauseRank != nil {
			found = got.RootCauseRank.Target.PID == 424242 && got.EventCount > 0
			for _, item := range got.RootCauseRank.Items {
				if item.ChainRelevance == "on_chain" && item.ResourceCompletionClosure {
					t.Fatal("wakeups without closed waits acquired a resource root")
				}
			}
		}
	}
	if !found || conversions.Load() != 1 {
		t.Fatalf("missing exact B target or duplicate preparation: found=%t conversions=%d", found, conversions.Load())
	}
	after, _ := json.Marshal(bus.ToolResults)
	if string(before) != string(after) {
		t.Fatal("system supplement rewrote model exploration")
	}
	hmc176AssertOriginal(t, path, original)
}

func hmc264SelectionContext(t *testing.T) (*types.BusContext, string) {
	t.Helper()
	bus, _, _ := hmc17NamedPathContext(t)
	path := filepath.Join(t.TempDir(), "two-captures.zip")
	body := hmc176Zip(t, map[string][]byte{"a.data": hmc176PerfData(true), "b.data": hmc17NamedBinaryTailFixture(), "same.data": hmc17NamedBinaryTailFixture()})
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:                      types.IntentRootCause,
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 424242, Thread: "tail-target", Source: "user_explicit", Confidence: 1}},
		RuntimeArtifactScopeProfile: requestWindowTestProfile(1.0105, 1.012),
		AnalyzerHints:               types.AnalyzerHints{ExactTargets: []string{path}},
	}}
	return bus, path
}

func hmc264QueryMember(t *testing.T, bus *types.BusContext, path, member string) types.ToolResult {
	t.Helper()
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": member, "view": "event_search", "time_start": 1.0, "time_end": 1.02, "limit": 100})
	_ = hmc17NamedPayload(t, result)
	bus.ToolResults = append(bus.ToolResults, result)
	return result
}

func TestHMC264ZIPSupplementSelectionAuthority(t *testing.T) {
	hmc176DisableExternalProviders(t)
	suppCoreSetConfig(t, true, 8<<20, 20*time.Second, 120)
	for _, kind := range []string{"prepare_only", "failed_query", "canceled_query", "repeated_member", "prepared_other_member", "alias_same_member", "success_then_failed", "two_members_ab", "two_members_ba", "same_bytes_members", "two_members_other_source", "history_json", "new_turn", "late_fork", "original_replaced", "manifest_rewritten", "original_replaced_other_source", "manifest_rewritten_other_source", "no_current_named_source", "unrelated_named_source", "multiple_named_sources", "comparison"} {
		t.Run(kind, func(t *testing.T) {
			bus, path := hmc264SelectionContext(t)
			preparer := bus.TraceInputPreparer.(types.TraceInputSelectionPreparer)
			prepare := func(member string) *attachment.TraceMaterial {
				material, err := preparer.PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: member})
				if err != nil {
					t.Fatal(err)
				}
				return material
			}
			want := kind == "repeated_member" || kind == "prepared_other_member" || kind == "alias_same_member" || kind == "success_then_failed"
			switch kind {
			case "prepare_only":
				prepare("b.data")
			case "failed_query":
				out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": "missing.data", "view": "event_search"})
				if out.Success {
					t.Fatal("missing member succeeded")
				}
			case "canceled_query":
				prepare("b.data")
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				bus.Ctx = ctx
				out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": "b.data", "view": "event_search"})
				if out.Success {
					t.Fatal("canceled query succeeded")
				}
				bus.Ctx = nil
			case "prepared_other_member":
				prepare("a.data")
				hmc264QueryMember(t, bus, path, "b.data")
			case "two_members_ab", "two_members_ba", "same_bytes_members", "two_members_other_source":
				first, last := "a.data", "b.data"
				if kind == "two_members_ba" {
					first, last = last, first
				}
				if kind == "same_bytes_members" {
					first = "same.data"
				}
				hmc264QueryMember(t, bus, path, first)
				for range 4 {
					hmc264QueryMember(t, bus, path, last)
				}
				bus.ToolResults = bus.ToolResults[len(bus.ToolResults)-1:] // trimming cannot make a choice unique
			case "late_fork":
				fork := bus.ShallowClone()
				fork.Mutable = bus.Mutable.ForkForExploreDispatch()
				fork.WorkDir = t.TempDir()
				bus.Mutable.ResetTurnAArtifacts()
				hmc264QueryMember(t, fork, path, "b.data")
				bus.Mutable.MergeExploreFork(fork.Mutable)
			default:
				result := hmc264QueryMember(t, bus, path, "b.data")
				switch kind {
				case "repeated_member":
					hmc264QueryMember(t, bus, path, "b.data")
					derived := hmc17NamedPayload(t, result).SourcePath
					out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": derived, "view": "event_search"})
					_ = hmc17NamedPayload(t, out)
				case "alias_same_member":
					alias := filepath.Join(t.TempDir(), "alias.zip")
					if err := os.Symlink(path, alias); err != nil {
						t.Fatal(err)
					}
					hmc264QueryMember(t, bus, alias, "b.data")
					bus.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{path, alias}
				case "success_then_failed":
					out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": "missing.data", "view": "event_search"})
					if out.Success {
						t.Fatal("missing member succeeded")
					}
				case "history_json":
					raw, _ := json.Marshal(result)
					var historical types.ToolResult
					if err := json.Unmarshal(raw, &historical); err != nil {
						t.Fatal(err)
					}
					bus.Mutable = types.NewMutableState("new run")
					bus.Mutable.AppendDispatchToolResult(historical)
					bus.ToolResults = []types.ToolResult{historical}
				case "new_turn":
					bus.Mutable.ResetTurnAArtifacts()
				case "original_replaced", "original_replaced_other_source":
					body, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path+".new", body, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(path+".new", path); err != nil {
						t.Fatal(err)
					}
				case "manifest_rewritten", "manifest_rewritten_other_source":
					material := prepare("b.data")
					body, err := os.ReadFile(material.QueryPath())
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(material.QueryPath(), append(body, '\n'), 0600); err != nil {
						t.Fatal(err)
					}
				case "no_current_named_source":
					bus.AnalysisIR.RequestModel.AnalyzerHints = types.AnalyzerHints{}
				case "unrelated_named_source", "multiple_named_sources", "comparison":
					other := filepath.Join(t.TempDir(), "other.htrace")
					if err := os.WriteFile(other, hmc176PerfData(true), 0600); err != nil {
						t.Fatal(err)
					}
					if kind == "unrelated_named_source" {
						bus.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{other}
					}
					if kind == "multiple_named_sources" {
						bus.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{path, other}
						// Control: the legacy resolver sees only the recognized
						// suffix and would drop the selected generic archive.
						legacyPath, label, reject := resolveTraceQuerySource(bus, traceQueryParams{})
						if reject != nil || label != "path" || legacyPath != other {
							t.Fatalf("legacy fallback control changed: %q %q %v", legacyPath, label, reject)
						}
					}
					if kind == "comparison" {
						bus.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = hmc224SourceOwnedScope(path, other, false)
					}
				}
			}
			if kind == "two_members_other_source" || kind == "original_replaced_other_source" || kind == "manifest_rewritten_other_source" {
				other := filepath.Join(t.TempDir(), "other.htrace")
				if err := os.WriteFile(other, hmc17NamedBinaryTailFixture(), 0600); err != nil {
					t.Fatal(err)
				}
				bus.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{path, other}
				legacyPath, label, reject := resolveTraceQuerySource(bus, traceQueryParams{})
				if reject != nil || label != "path" || legacyPath != other {
					t.Fatalf("fallback control: %q %q %v", legacyPath, label, reject)
				}
			}
			out := RunTraceQuerySystemSupplement(bus)
			results := bus.Mutable.SystemTraceSupplementResults()
			if want {
				if len(out.Executed) == 0 || len(results) == 0 || out.SkipReason != "" {
					t.Fatalf("legitimate selection not supplemented: %+v", out)
				}
				for _, result := range results {
					got := hmc17NamedPayload(t, result)
					if got.TimeStart != 1.0105 || got.TimeEnd != 1.012 || got.EventCount != 12 {
						t.Fatalf("wrong source/window population: %+v", got)
					}
				}
			} else if len(out.Executed) != 0 || len(results) != 0 {
				t.Fatalf("%s supplied unauthorized supplement: %+v", kind, out)
			}
			if kind == "repeated_member" {
				out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search"})
				if out.Success {
					t.Fatal("successful supplement made bare multi-member ZIP query valid")
				}
			}
		})
	}
}

func TestHMC264PreparedSelectionRequiresSuccessfulNativeStamp(t *testing.T) {
	hmc176DisableExternalProviders(t)
	for _, kind := range []string{"native", "failed", "memo", "wrong_tool", "invalid_params", "canceled", "old_epoch", "system_lane"} {
		t.Run(kind, func(t *testing.T) {
			bus, path := hmc264SelectionContext(t)
			material, err := bus.TraceInputPreparer.(types.TraceInputSelectionPreparer).PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: "b.data"})
			if err != nil {
				t.Fatal(err)
			}
			ref := bus.Mutable.PrepareTraceQuerySourceRead(material.QueryPath())
			out := types.ToolResult{ToolName: "trace_query", Success: true}
			params := json.RawMessage(`{"view":"event_search"}`)
			ctx := t.Context()
			switch kind {
			case "failed":
				out.Success = false
			case "memo":
				out.ReusedFromRunMemo = true
			case "wrong_tool":
				out.ToolName = "read_file"
			case "invalid_params":
				params = json.RawMessage(`[]`)
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "old_epoch":
				bus.Mutable.ResetTurnAArtifacts()
			case "system_lane":
				bus.Mutable.BeginSystemTraceSupplementExecution()
				defer bus.Mutable.EndSystemTraceSupplementExecution()
			}
			bus.Mutable.StampTraceIntervalNavigation(ctx, ref, &out, params, material)
			selected, state := bus.Mutable.SuccessfulPreparedArchiveSelection(t.Context())
			valid := state == types.TracePreparedSelectionUnique
			if valid != (kind == "native") {
				t.Fatalf("%s selection=%d", kind, state)
			}
			if valid && selected != material {
				t.Fatal("producer selection changed material")
			}
		})
	}
}

func TestHMC264ZIPSupplementConcurrentSelections(t *testing.T) {
	hmc176DisableExternalProviders(t)
	suppCoreSetConfig(t, true, 8<<20, 20*time.Second, 120)
	for _, different := range []bool{false, true} {
		t.Run(map[bool]string{false: "same_member", true: "different_members"}[different], func(t *testing.T) {
			bus, path := hmc264SelectionContext(t)
			type outcome struct {
				result types.ToolResult
				err    error
			}
			done := make(chan outcome, 4)
			for i := range 4 {
				fork := bus.ShallowClone()
				fork.Mutable = bus.Mutable.ForkForExploreDispatch()
				fork.WorkDir = t.TempDir()
				member := "b.data"
				if different && i%2 == 0 {
					member = "a.data"
				}
				raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "archive_member": member, "view": "event_search"})
				go func() { result, err := (&TraceQuery{}).Execute(fork, raw); done <- outcome{result, err} }()
			}
			for range 4 {
				out := <-done
				if out.err != nil {
					t.Fatal(out.err)
				}
				_ = hmc17NamedPayload(t, out.result)
			}
			out := RunTraceQuerySystemSupplement(bus)
			if different {
				if len(out.Executed) != 0 {
					t.Fatalf("concurrency elected one member: %+v", out)
				}
			} else {
				if len(out.Executed) == 0 {
					t.Fatalf("same physical selection lost: %+v", out)
				}
				for _, result := range bus.Mutable.SystemTraceSupplementResults() {
					got := hmc17NamedPayload(t, result)
					if got.EventCount != 12 || got.TimeStart != 1.0105 || got.TimeEnd != 1.012 {
						t.Fatal("concurrent supplement changed facts")
					}
				}
			}
		})
	}
}

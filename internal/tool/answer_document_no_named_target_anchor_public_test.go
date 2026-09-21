package tool

import (
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the same native query -> ledger -> projection -> context-focus
// renderer seam as final answers. Entity order is deliberately unrelated to
// the query target; neither a discovered business instance nor a navigation
// cursor turns an artifact-wide request into a user-named thread request.
func noNamedAnchorPublicFixture(t *testing.T, renamed bool) (*types.BusContext, types.TraceBusinessSpanRef, []string) {
	t.Helper()
	ctx, ref := supplementBusinessFocusFixture(t, "OpenDocument")
	names := []string{"storage-irq-80", "document-worker-200", "app-main-100"}
	if renamed {
		data, err := os.ReadFile(ref.Data().Path)
		if err != nil {
			t.Fatal(err)
		}
		replacer := strings.NewReplacer("storage-irq", "device-completion", "document-worker", "index-loader", "app-main", "reader-main")
		if err := os.WriteFile(ref.Data().Path, []byte(replacer.Replace(string(data))), 0600); err != nil {
			t.Fatal(err)
		}
		// A replaced physical source must publish and accept its own fresh ref.
		result := businessRefTestQuery(t, ctx, map[string]any{"path": ref.Data().Path, "view": "span_window", "span_name": "OpenDocument", "time_start": .999, "time_end": 1.05})
		if !result.Success || len(result.TraceBusinessSpanRefs) != 1 {
			t.Fatalf("renamed native source did not publish one instance: %+v", result)
		}
		ref = result.TraceBusinessSpanRefs[0]
		names = []string{"device-completion-80", "index-loader-200", "reader-main-100"}
	}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeFullArtifact}
	ctx.AnalysisIR.RequestModel.RuntimeTargetProfile = &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNoNamedTarget, Confidence: 1}
	supplementAcceptBusinessFocus(t, ctx, &ref)
	return ctx, ref, names
}

func noNamedAnchorPublicCompile(t *testing.T, ctx *types.BusContext, ref types.TraceBusinessSpanRef) (types.ObservationLedger, types.TraceCausalProjection) {
	t.Helper()
	before, err := os.ReadFile(ref.Data().Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "critical_blocking_calls"} {
		result := businessRefTestQuery(t, ctx, map[string]any{"path": ref.Data().Path, "view": view, "pid": 100, "time_start": 1.0, "time_end": 1.051})
		if !result.Success || len(result.Observations) == 0 {
			t.Fatalf("native %s failed to publish observations: %s", view, result.Summary)
		}
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		t.Fatalf("one physical source must produce one projection: %+v", set)
	}
	after, err := os.ReadFile(ref.Data().Path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("native queries/compilation changed the physical trace: %v", err)
	}
	return ledger, set.Projections[0]
}

func noNamedAnchorPublicAssertMeasurements(t *testing.T, ledger types.ObservationLedger, p types.TraceCausalProjection, worker, target string) {
	t.Helper()
	a := p.TargetStateAccount
	if a == nil || a.Subject != target || a.WindowStartTs != 1 || a.WindowEndTs != 1.051 || math.Abs(a.TotalMS-51) > 1e-6 || math.Abs(a.RunningMS-6) > 1e-6 || math.Abs(a.RunnableMS-1) > 1e-6 || math.Abs(a.SleepMS-44) > 1e-6 {
		t.Errorf("full query target account changed or was relabeled as its upstream worker: %+v", a)
	}
	var business, request, blocking, background bool
	for _, row := range ledger.Records {
		if row.Predicate == "io_latency" && row.Subject == worker && row.Value == "35.000" {
			request = true
			if traceSupplementRichNoteValue(row.RichNotes, types.TraceNoteKeyIOIssuerBlocked) != "31.000" || traceSupplementRichNoteValue(row.RichNotes, types.TraceNoteKeyIOIssuerBlockedState) != "s_sleep" || traceSupplementRichNoteValue(row.RichNotes, types.TraceNoteKeyIONonAdditiveWithBlocked) != "true" {
				t.Errorf("request and actual sleeping issuer wait lost their independent rulers: %+v", row)
			}
		}
		if row.Predicate == types.TraceBusinessSpanPredicate && row.Subject == target && row.Object == "OpenDocument" && row.Value == "50.000" && row.Span.StartTs == 1 && row.Span.EndTs == 1.05 {
			business = true
		}
	}
	for _, node := range append(append([]types.TraceCausalProjectionNode(nil), p.PrimaryRootCauses...), p.OnChainCauses...) {
		if strings.Contains(node.Subject, "backup-900") {
			t.Error("unrelated background IO became an on-chain cause")
		}
		if node.Subject == worker && node.Object == "io_latency" && math.Abs(node.ImpactMS-31) < 1e-6 {
			blocking = true
		}
	}
	for _, node := range p.BackgroundCauses {
		if node.Subject == "backup-900" && math.Abs(node.ImpactMS-47) < 1e-6 {
			background = true
		}
	}
	if !business || !request || !blocking || !background {
		t.Errorf("independent measured evidence missing: business50=%t request35=%t blocking31=%t background47=%t", business, request, blocking, background)
	}
}

func TestNoNamedTargetAnchorPublicPreservesNativeChain(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		label := "original"
		if renamed {
			label = "renamed"
		}
		for _, entities := range []string{"worker_first", "target_first", "exact_worker", "empty", "unspecified"} {
			t.Run(label+"/"+entities, func(t *testing.T) {
				ctx, ref, names := noNamedAnchorPublicFixture(t, renamed)
				hints := &ctx.AnalysisIR.RequestModel.AnalyzerHints
				switch entities {
				case "worker_first":
					hints.Entities = []string{names[1], names[2], "OpenDocument"}
				case "target_first":
					hints.Entities = []string{names[2], names[1], "OpenDocument"}
				case "exact_worker":
					hints.ExactTargets = []string{names[1]}
					hints.Entities = []string{names[2], names[1]}
				case "unspecified":
					ctx.AnalysisIR.RequestModel.RuntimeTargetProfile.Declaration = types.RuntimeTargetDeclarationUnspecified
					hints.Entities = []string{names[1], names[2]}
				}
				ledger, p := noNamedAnchorPublicCompile(t, ctx, ref)
				if !reflect.DeepEqual(p.WakeupPath, names) || p.WakeupPathUserElected || len(p.WakeupPathUserEntityHits) != 0 {
					t.Errorf("profile %s must preserve the native chain without user election: path=%v elected=%t hits=%v; want %v", ctx.AnalysisIR.RequestModel.RuntimeTargetProfile.Declaration, p.WakeupPath, p.WakeupPathUserElected, p.WakeupPathUserEntityHits, names)
				}
				noNamedAnchorPublicAssertMeasurements(t, ledger, p, names[1], names[2])
				focus := runtimeTraceProjUserFocusFromBusContext(ctx)
				for _, lang := range []string{"zh", "en"} {
					blocks := runtimeTraceCausalProjectionCluster(p, lang, focus)
					corpus := suppFullCapCorpus(blocks)
					for _, unauthorizedLabel := range []string{"用户关注线程", "user-focus thread", "user-focused thread", "IS your specified", "即你指定的"} {
						if strings.Contains(corpus, unauthorizedLabel) {
							t.Errorf("%s renderer claims a user-named target under explicit absence (%q), native path=%v", lang, unauthorizedLabel, p.WakeupPath)
						}
					}
					model := buildRuntimeTraceProjTreeModel(p, nil, lang == "zh")
					runtimeTraceProjApplyUserFocus(&model, focus)
					if model.Target != names[2] {
						t.Errorf("upstream worker must not become the self/root thread: target=%q, want %q", model.Target, names[2])
					}
					for _, row := range model.SelfRows {
						if row.Node.Subject == names[1] {
							t.Errorf("upstream worker was relabeled as target-self: %+v", row.Node)
						}
					}
				}
			})
		}
	}
}

func TestNoNamedTargetAnchorPublicKeepsAuthorizedAndLegacyFocus(t *testing.T) {
	for _, variant := range []string{"named_target", "named_worker", "same_tid_alias", "named_outside_chain", "cursor_only", "nil_profile_worker"} {
		t.Run(variant, func(t *testing.T) {
			ctx, ref, names := noNamedAnchorPublicFixture(t, false)
			rm := &ctx.AnalysisIR.RequestModel
			wantPath, wantElected := names, true
			switch variant {
			case "named_target", "named_worker", "same_tid_alias", "named_outside_chain":
				pid, name := 100, names[2]
				if variant == "named_worker" {
					pid, name, wantPath = 200, names[1], names[:2]
				}
				if variant == "same_tid_alias" {
					name = "reader-alias-100"
				}
				if variant == "named_outside_chain" {
					pid, name, wantElected = 999, "another-reader-999", false
				}
				rm.RuntimeTargetProfile = &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: name, Confidence: 1}
				rm.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: pid, Thread: name, Source: "user_explicit"}}
				rm.AnalyzerHints.ExactTargets = []string{name}
				rm.AnalyzerHints.Entities = []string{names[1], names[2]}
			case "cursor_only":
				rm.RuntimeTargetProfile = nil
				rm.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 200, Thread: names[1], Source: types.RuntimeTargetSourceExplicitToolCall}}
				wantElected = false
			case "nil_profile_worker":
				rm.RuntimeTargetProfile = nil
				rm.AnalyzerHints.Entities = []string{names[1]}
				wantPath = names[:2]
			}
			ledger, p := noNamedAnchorPublicCompile(t, ctx, ref)
			if !reflect.DeepEqual(p.WakeupPath, wantPath) || p.WakeupPathUserElected != wantElected {
				t.Fatalf("authorized/legacy focus changed: path=%v elected=%t, want %v elected=%t", p.WakeupPath, p.WakeupPathUserElected, wantPath, wantElected)
			}
			focus := runtimeTraceProjUserFocusFromBusContext(ctx)
			model := buildRuntimeTraceProjTreeModel(p, nil, true)
			runtimeTraceProjApplyUserFocus(&model, focus)
			wantAnchorOnly := variant == "named_outside_chain"
			if model.Target != wantPath[len(wantPath)-1] || model.RootFocusAnchorOnly != wantAnchorOnly {
				t.Errorf("existing named/legacy root label changed: target=%q anchorOnly=%t", model.Target, model.RootFocusAnchorOnly)
			}
			if variant == "named_outside_chain" {
				noNamedAnchorPublicAssertMeasurements(t, ledger, p, names[1], names[2])
				if len(p.WakeupPathUserEntityHits) != 0 {
					t.Errorf("generic entities supplied a fallback user hit for an unrelated named target: %v", p.WakeupPathUserEntityHits)
				}
			}
			corpus := suppFullCapCorpus(runtimeTraceCausalProjectionCluster(p, "zh", focus))
			if variant == "same_tid_alias" && (!strings.Contains(corpus, "reader-alias-100") || !strings.Contains(corpus, names[2])) {
				t.Error("same-tid aliases must retain both identities in the reader-facing projection")
			}
		})
	}
}

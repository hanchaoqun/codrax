package types

import (
	"context"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

// Public lifecycle entry points distinguish a legacy query-output read path
// from current source evidence, navigation tickets, catalogs and completion.
// Genuine native-fact retention remains covered by the unchanged HMC326
// public TraceQuery replacement tests in internal/agent.
func TestHMC327LateForkMergesOnlyLegacyQueryOutputPaths(t *testing.T) {
	for _, kind := range []string{"replacement", "task_reset"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			capture := writeTraceReadTestFile(t, root, "capture.ftrace")
			parent := NewMutableState("late query output")
			fork := parent.ForkForExploreDispatch()
			result := nativeTraceSourceReadResult(capture)
			result.RawRef = artifactNavigationRootA
			fork.StampTraceQuerySourceRead(fork.PrepareTraceQuerySourceRead(capture), &result)
			fork.AppendDispatchToolResult(result)
			fork.AppendDispatchToolResult(artifactNavigationResult(t, fork, artifactNavigationRootA, artifactNavigationDerived))
			catalog, err := tracecatalog.Discover(context.Background(), root, tracecatalog.DiscoverOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = fork.InstallTraceCatalog(catalog); err != nil {
				t.Fatal(err)
			}
			fork.SetTurnAArtifacts(TurnAArtifacts{ToolResults: []ToolResult{result}, InvestigationNotes: []string{"old note"}})
			fork.SetInvestigationComplete("old completion")
			if _, ok := fork.ResolveTraceQuerySourceRead(capture); !ok || len(fork.TraceCatalogs()) != 1 {
				t.Fatal("fixture did not publish current source permission and catalog")
			}
			if _, ok := parent.ResolveTraceQueryBlobRef(artifactNavigationRootA); ok {
				t.Fatal("fork-local query path leaked before merge")
			}
			if kind == "task_reset" {
				parent.ResetTurnAArtifacts()
			} else {
				parent.SetTurnAArtifacts(TurnAArtifacts{UserQuestion: "current replacement"})
			}
			before, nativeBefore := parent.TurnAArtifacts(), parent.NativeFactRetentionStatus()
			parent.MergeExploreFork(fork)
			if got, ok := parent.ResolveTraceQueryBlobRef(artifactNavigationRootA); !ok || got != artifactNavigationRootA {
				t.Fatal("late fork lost its already-published legacy query-output path")
			}
			if _, ok := parent.ResolveTraceQueryBlobRef(artifactNavigationDerived); ok {
				t.Fatal("derived navigation became a query-output read permission")
			}
			for _, path := range []string{artifactNavigationRootA, artifactNavigationDerived} {
				if _, ok := parent.PrepareArtifactReadNavigation(path); ok {
					t.Fatalf("legacy read path minted current navigation: %s", path)
				}
			}
			if _, ok := parent.ResolveTraceQuerySourceRead(capture); ok {
				t.Fatal("historical query-output access revived physical-source permission")
			}
			if !reflect.DeepEqual(parent.TurnAArtifacts(), before) || parent.NativeFactRetentionStatus() != nativeBefore ||
				len(parent.DispatchToolResults()) != 0 || len(parent.TraceCatalogs()) != 0 ||
				parent.IsInvestigationComplete() || parent.StableInvestigationCompleteReason() != "" ||
				parent.TraceQueryRuntimeObservationCount() != 0 {
				t.Fatal("late fork imported evidence, tools, catalog or completion alongside a legacy path")
			}
			parent.MergeExploreFork(fork) // Idempotent path union, not a new publication.
			if refs := parent.TraceQueryBlobRefs(); !reflect.DeepEqual(refs, []string{artifactNavigationRootA}) {
				t.Fatalf("repeated late merge changed the allowed path set: %v", refs)
			}
			// A real current publication still creates a current navigation
			// origin, but does not revive an old derived I/O ticket.
			artifactNavigationPublishRoot(parent, artifactNavigationRootA)
			if _, ok := parent.PrepareArtifactReadNavigation(artifactNavigationRootA); !ok {
				t.Fatal("legacy registry presence blocked current-generation publication")
			}
			if _, ok := parent.PrepareArtifactReadNavigation(artifactNavigationDerived); ok {
				t.Fatal("current root publication resurrected the stale derived ticket")
			}
		})
	}
}

func TestHMC327CurrentForkStillMergesToolsAndNavigation(t *testing.T) {
	parent := NewMutableState("current query output")
	fork := parent.ForkForExploreDispatch()
	artifactNavigationPublishRoot(fork, artifactNavigationRootA)
	derived := artifactNavigationResult(t, fork, artifactNavigationRootA, artifactNavigationDerived)
	fork.AppendDispatchToolResult(derived)
	fork.SetTurnAArtifacts(TurnAArtifacts{ToolResults: fork.DispatchToolResults(), InvestigationNotes: []string{"current note"}})
	fork.SetInvestigationComplete("current completion")
	parent.MergeExploreFork(fork)
	if _, ok := parent.ResolveTraceQueryBlobRef(artifactNavigationRootA); !ok {
		t.Fatal("current merge lost the legacy path")
	}
	if nav, ok := parent.PrepareArtifactReadNavigation(artifactNavigationDerived); !ok || nav.OriginQueryRef != artifactNavigationRootA {
		t.Fatal("current merge lost exact original-to-derived navigation")
	}
	got := parent.TurnAArtifacts()
	if got == nil || len(got.ToolResults) != 2 || !parent.IsInvestigationComplete() || parent.StableInvestigationCompleteReason() != "current completion" {
		t.Fatal("legacy path compatibility blocked current tools or completion")
	}
}

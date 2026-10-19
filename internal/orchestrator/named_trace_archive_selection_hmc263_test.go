package orchestrator

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc263NamedArchive(t *testing.T, dir, name string, members ...string) string {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for _, member := range members {
		f, err := w.Create(member)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("not yet decoded; namespace-only candidate")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHMC263NamedArchiveSelectionReachesInvestigationWithoutReadyFacts(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	// The public run receives a typed trace-shaped carrier; format remains ZIP
	// by its bytes. No new generic .zip or raw-prose intent inference is needed.
	path := hmc263NamedArchive(t, repo, "captures.htrace", "a.sys", "b.sys")
	analyzerCalls, otherCalls := 0, 0
	var events []render.Event
	o := newTypedNamedTraceAdmissionTestOrchestrator([]string{path}, &analyzerCalls, &otherCalls, &events)
	o.SetTraceRuntimeAnchor(filepath.Join(t.TempDir(), "runtime"))
	bus, err := o.Run("只分析 `"+path+"` 中指定的采集，不分析源码", repo, "main")
	var archive *hitraceconv.TraceArchiveError
	if errors.As(err, &archive) || bus == nil || analyzerCalls != 1 || !hasTraceAdmissionEventKind(events, render.EventAnalysisReady) {
		t.Fatalf("recoverable selection stopped before investigation: analyzer=%d other=%d bus=%v err=%v", analyzerCalls, otherCalls, bus != nil, err)
	}
	if len(bus.TraceInputPreparer.PreparedMaterials()) != 0 || bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" {
		t.Fatal("unselected archive published ready material")
	}
	if !bus.RuntimeArtifactPreflight.HasTraceArtifact() || !strings.Contains(bus.RuntimeArtifactPreflight.Artifacts[0].Detail, "archive_member") {
		t.Fatalf("pending selection lost navigation: %+v", bus.RuntimeArtifactPreflight)
	}
	if _, err := bus.TraceInputPreparer.Prepare(context.Background(), path); !errors.As(err, &archive) || archive.Code != "multiple_trace_members" {
		t.Fatalf("navigation changed actual unselected admission: %v", err)
	}
}

func TestHMC263NamedArchiveSelectionNeverDefersOtherIntakeFailures(t *testing.T) {
	for _, shape := range []string{"unsafe_member", "malformed", "unknown", "pending_then_malformed", "malformed_then_pending"} {
		t.Run(shape, func(t *testing.T) {
			dir := t.TempDir()
			pending := hmc263NamedArchive(t, dir, "pending.htrace", "a.sys", "b.sys")
			bad := filepath.Join(dir, "bad.htrace")
			if shape == "unsafe_member" {
				bad = hmc263NamedArchive(t, dir, "bad.htrace", "a.sys", "../escape.sys")
			} else if shape == "unknown" {
				bad = hmc263NamedArchive(t, dir, "bad.htrace", "unknown.data")
			} else if err := os.WriteFile(bad, []byte("PK\x03\x04broken"), 0600); err != nil {
				t.Fatal(err)
			}
			paths := []string{bad}
			if shape == "pending_then_malformed" {
				paths = []string{pending, bad}
			}
			if shape == "malformed_then_pending" {
				paths = []string{bad, pending}
			}
			bus := hmc263NamedAdmissionBus(t, dir, paths)
			if err := validateTypedNamedTraceInputsBeforeExploration(t.Context(), bus, "typed trace investigation"); err == nil {
				t.Fatal("non-selection failure was deferred")
			}
			if len(bus.TraceInputPreparer.PreparedMaterials()) != 0 {
				t.Fatal("failed intake published facts")
			}
		})
	}
}

func TestHMC263TypedZIPPendingProfileIsNavigationOnly(t *testing.T) {
	dir := t.TempDir()
	path := hmc263NamedArchive(t, dir, "captures.zip", "a.sys", "b.sys")
	bus := hmc263NamedAdmissionBus(t, dir, []string{path})
	if err := validateTypedNamedTraceInputsBeforeExploration(t.Context(), bus, "typed trace investigation"); err != nil {
		t.Fatal(err)
	}
	if len(bus.TraceInputPreparer.PreparedMaterials()) != 0 || !bus.RuntimeArtifactPreflight.HasTraceArtifact() {
		t.Fatal("pending profile acquired authority or lost navigation")
	}
	// Supplying a real member remains subject to decoder admission, not the
	// earlier directory's namespace-only candidate classification.
	_, err := bus.TraceInputPreparer.(types.TraceInputSelectionPreparer).PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: "a.sys"})
	if err == nil {
		t.Fatal("namespace candidate bypassed complete decoding")
	}
}

func hmc263NamedAdmissionBus(t *testing.T, dir string, paths []string) *types.BusContext {
	t.Helper()
	bus := &types.BusContext{RepoRoot: dir, Mutable: types.NewMutableState("typed trace investigation"),
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, "runtime")}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Scenario: types.ScenarioPerformanceBottleneck,
			ExternalObservationPolicy: &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ExclusionKind: types.ExternalObservationSourceExclusionExplicitUserBoundary, ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly, SourceQuotes: []string{"only trace"}}}}}
	for _, path := range paths {
		bus.RuntimeArtifactPreflight.Artifacts = append(bus.RuntimeArtifactPreflight.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Carrier: "request_path", Source: path})
	}
	return bus
}

func TestHMC263NamedArchivePendingRequiresSoleProducerVerdict(t *testing.T) {
	pending := &hitraceconv.TraceArchiveError{Code: "multiple_trace_members", Cause: errors.New("two candidates")}
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{"typed", pending, true},
		{"single_wrapped", fmt.Errorf("prepare: %w", errors.Join(pending)), true},
		{"raw_text", errors.New(pending.Error()), false},
		{"other_code", &hitraceconv.TraceArchiveError{Code: "member_integrity_failed"}, false},
		{"joined_cancel", errors.Join(pending, context.Canceled), false},
		{"joined_source_or_close_failure", errors.Join(pending, errors.New("source changed or close failed")), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := namedTraceArchiveSelectionPending(test.err); got != test.want {
				t.Fatalf("pending=%v want %v", got, test.want)
			}
		})
	}
}

func TestHMC263NamedArchivePendingPreservesIndependentPairFailureBoundary(t *testing.T) {
	for _, test := range []struct {
		name, other string
		wantFailure bool
		wantReady   int
	}{
		{"two_pending", "pending", false, 0},
		{"pending_and_fatal", "fatal", true, 0},
		{"pending_and_ready", "ready", false, 1},
		{"ready_and_fatal", "fatal", false, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			first := hmc263NamedArchive(t, dir, "first.htrace", "a.sys", "b.sys")
			if test.name == "ready_and_fatal" {
				if err := os.WriteFile(first, []byte("app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			second := filepath.Join(dir, "second.htrace")
			switch test.other {
			case "pending":
				second = hmc263NamedArchive(t, dir, "second.htrace", "a.sys", "b.sys")
			case "fatal":
				if err := os.WriteFile(second, []byte("PK\x03\x04broken"), 0600); err != nil {
					t.Fatal(err)
				}
			case "ready":
				if err := os.WriteFile(second, []byte("app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			bus := hmc263NamedAdmissionBus(t, dir, []string{first, second})
			bus.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = hmc224ComparisonScope([2]string{first, second})
			err := validateTypedNamedTraceInputsBeforeExploration(t.Context(), bus, "typed comparison")
			if (err != nil) != test.wantFailure {
				t.Fatalf("failure=%v want %v", err, test.wantFailure)
			}
			if got := len(bus.TraceInputPreparer.PreparedMaterials()); got != test.wantReady {
				t.Fatalf("ready=%d want %d", got, test.wantReady)
			}
			if len(bus.Mutable.TraceCatalogs()) != 1 {
				t.Fatal("independent navigation lost")
			}
		})
	}
}

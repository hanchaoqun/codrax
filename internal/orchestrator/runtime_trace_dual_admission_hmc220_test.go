package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This pins the existing admission boundary, not HMC-10.1 completion. A pair
// tool may isolate query failures, but the natural named-path entry still
// rejects a failed typed input before reaching that tool. Do not weaken the
// old source safety gate merely to make the new pair tool appear reachable.
func TestHMC220NamedTraceAdmissionExistingBoundary(t *testing.T) {
	const healthy = "app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n"
	for _, tc := range []struct {
		name, suffix string
		body         []byte
		blocked      bool
	}{
		{"unsupported_binary", ".htrace", []byte{'P', 'K', 3, 4, 0, 1}, true},
		{"missing_typed_trace", ".trace", nil, false},
		{"missing_generic_data", ".data", nil, false},
		{"both_healthy", ".htrace", []byte(healthy), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			first, second := filepath.Join(repo, "baseline.trace"), filepath.Join(repo, "current"+tc.suffix)
			if err := os.WriteFile(first, []byte(healthy), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.body != nil {
				if err := os.WriteFile(second, tc.body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			canonicalSecond, _ := filepath.EvalSymlinks(second)
			analyzer, other := 0, 0
			var events []render.Event
			o := newTypedNamedTraceAdmissionTestOrchestrator([]string{first, second}, &analyzer, &other, &events)
			o.SetTraceRuntimeAnchor(t.TempDir())
			bus, err := o.Run("Compare the measurements in "+first+" and "+second, repo, "main")
			ready := hasTraceAdmissionEventKind(events, render.EventAnalysisReady)
			if bus == nil || analyzer != 1 || ready == tc.blocked {
				t.Fatalf("admission boundary changed: bus=%t analyzer=%d other=%d ready=%t blocked=%t err=%v", bus != nil, analyzer, other, ready, tc.blocked, err)
			}
			if tc.blocked && (err == nil || other != 0) {
				t.Fatalf("failed typed source crossed admission: other=%d err=%v", other, err)
			}
			var code string
			var admission *traceinput.Error
			if errors.As(err, &admission) {
				code = admission.Code
			}
			profileSources := []string{}
			for _, artifact := range bus.RuntimeArtifactPreflight.Artifacts {
				profileSources = append(profileSources, artifact.Source)
			}
			paths, pathErr := typedNamedTraceAdmissionPaths(bus.RuntimeArtifactPreflight, bus.AnalysisIR.RequestModel, "", repo)
			if pathErr != nil {
				t.Fatal(pathErr)
			}
			foundSecond := false
			for _, path := range paths {
				foundSecond = foundSecond || path == second || path == canonicalSecond
			}
			if foundSecond != (tc.body != nil) {
				t.Fatalf("typed candidate scope changed: %v", paths)
			}
			t.Logf("ready=%t other_calls=%d preparation_code=%q preflight_sources=%v admitted_candidates=%v error=%v", ready, other, code, profileSources, paths, err)
		})
	}
}

func TestHMC220NamedTraceAdmissionRejectsCachedGenerationChange(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "baseline.trace"), filepath.Join(dir, "current.trace")
	const healthy = "app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n"
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte(healthy), 0600); err != nil {
			t.Fatal(err)
		}
	}
	coordinator := traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()})
	for _, path := range []string{first, second} {
		if _, err := coordinator.Prepare(context.Background(), path); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(second, []byte(healthy+healthy), 0600); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: dir, TraceInputPreparer: coordinator,
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{
			{Kind: "trace", Source: first, Carrier: "request_path"}, {Kind: "trace", Source: second, Carrier: "request_path"},
		}},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{ExternalObservationPolicy: &types.ExternalObservationPolicy{
			CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
		}}},
	}
	err := validateTypedNamedTraceInputsBeforeExploration(context.Background(), bus, "")
	if err == nil || !strings.Contains(err.Error(), "current.trace") {
		t.Fatalf("cached generation was silently replaced: %v", err)
	}
	if _, err := coordinator.Prepare(context.Background(), first); err != nil {
		t.Fatal("healthy generation was corrupted by sibling rejection", err)
	}
	t.Logf("cached generation stays fail-closed before pair exploration: %v", err)
}

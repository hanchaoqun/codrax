package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreparedTraceMaterialRunEntryRejectsDriftAndPlainSetterClears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.systrace")
	if err := os.WriteFile(path, []byte("# trace text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{}
	o.SetAttachedHitrace(m.Preview())
	o.SetAttachedTraceMaterial(m)
	if err := o.validateAttachedTraceInputs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# source replaced\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := o.validateAttachedTraceInputs(context.Background()); err == nil {
		t.Fatal("stale source admitted")
	}
	o.SetAttachedHitrace("# new inline trace\n")
	if o.AttachedTraceMaterial() != nil {
		t.Fatal("plain setter inherited old receipt")
	}
	if err := o.validateAttachedTraceInputs(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedTraceMaterialNamedOriginalReusesOnlyExactPreparedSource(t *testing.T) {
	dir := t.TempDir()
	source, query := filepath.Join(dir, "original.sys"), filepath.Join(dir, "query.systrace")
	if err := os.WriteFile(source, []byte("PERFILE2\x00binary fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(query, []byte("# converted text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bindings := make(map[string]filegeneration.Identity)
	for _, p := range []string{source, query} {
		id, err := filegeneration.FromPath(p)
		if err != nil {
			t.Fatal(err)
		}
		bindings[p] = id
	}
	preview := "# codrax-source: " + query + "\n# preview\n"
	m, err := attachment.BindTraceMaterial(source, query, preview, bindings)
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: dir, AttachedHitrace: preview, AttachedTraceMaterial: m, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Scenario:                  types.ScenarioPerformanceBottleneck,
		ExternalObservationPolicy: &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ExclusionKind: types.ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"only analyze the trace"}},
		AnalyzerHints:             types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{Path: source, Confidence: 1}}},
	}}}
	if err := validateTypedNamedTraceInputsBeforeExploration(context.Background(), bus, ""); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "another.sys")
	if err := os.WriteFile(other, []byte("PERFILE2\x00other binary"), 0600); err != nil {
		t.Fatal(err)
	}
	bus.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints[0].Path = other
	if err := validateTypedNamedTraceInputsBeforeExploration(context.Background(), bus, ""); err == nil {
		t.Fatal("unprepared named binary inherited another file's conversion")
	}
}

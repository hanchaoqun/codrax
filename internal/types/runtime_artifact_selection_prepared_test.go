package types

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestRuntimeArtifactSelectionPreparedSourceAndQueryAreOneCapture(t *testing.T) {
	ctx, original, query := preparedSelectionFixture(t)
	ctx.AnalysisIR = &AnalysisIR{RequestModel: RequestModel{
		ExternalObservationPolicy: traceOnlyExternalObservationPolicy(),
		AnalyzerHints:             AnalyzerHints{RequiredFileHints: []RequiredFileHint{{Path: original, Confidence: 1}}},
	}}
	ctx.PerfTrace = &PerfBundle{}
	ctx.AnalysisIR.RequestModel.PerfTrace = ctx.PerfTrace
	view := RuntimeArtifactSelectionViewFromAgentContext(ctx)
	if view.TraceCount != 1 || view.Policy.Kind != RuntimeArtifactAnalysisPolicyTraceOnlyExactArtifact || view.Items[0].Source != query {
		t.Fatalf("one prepared capture became multiple artifacts: %+v", view)
	}
	for _, carrier := range []string{"request_path", "attachment", "attached_trace", "required_file_hint", "perf_trace", "request_model_perf_trace"} {
		if !runtimeArtifactSelectionTestHasCarrier(view.Items[0], carrier) {
			t.Fatalf("alias collapse dropped %s provenance: %+v", carrier, view)
		}
	}
}

func TestRuntimeArtifactSelectionPreparedNeverCollapsesAnotherCapture(t *testing.T) {
	ctx, _, query := preparedSelectionFixture(t)
	other := filepath.Join(t.TempDir(), filepath.Base(query))
	if err := os.WriteFile(other, []byte("# different physical capture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, RuntimeArtifactPreflightArtifact{Kind: "trace", Source: other, Carrier: "request_path"})
	view := RuntimeArtifactSelectionViewFromAgentContext(ctx)
	if view.TraceCount != 2 {
		t.Fatalf("different capture collapsed: %+v", view)
	}
}

func TestRuntimeArtifactSelectionPreparedStaleOrWrongPreviewCannotAlias(t *testing.T) {
	for _, stale := range []bool{false, true} {
		ctx, source, _ := preparedSelectionFixture(t)
		if stale {
			if err := os.WriteFile(source, []byte("replacement source"), 0o600); err != nil {
				t.Fatal(err)
			}
		} else {
			ctx.AttachedHitrace += "# different preview\n"
		}
		if view := RuntimeArtifactSelectionViewFromAgentContext(ctx); view.TraceCount != 2 {
			t.Fatalf("invalid prepared receipt collapsed physical roles: %+v", view)
		}
	}
}

func TestRuntimeArtifactSelectionPreparedCanonicalSourceAlias(t *testing.T) {
	ctx, source, _ := preparedSelectionFixture(t)
	canonical, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	ctx.RuntimeArtifactPreflight.Artifacts[0].Source = canonical
	if view := RuntimeArtifactSelectionViewFromAgentContext(ctx); view.TraceCount != 1 {
		t.Fatalf("canonical source spellings created duplicate capture: %+v", view)
	}
}

func preparedSelectionFixture(t *testing.T) (*AgentContext, string, string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "original.htrace")
	query := filepath.Join(dir, "derived.systrace")
	for path, body := range map[string]string{source: "OHOSPROF\x00original", query: "# converted trace\n"} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bindings := map[string]filegeneration.Identity{}
	for _, path := range []string{source, query} {
		id, err := filegeneration.FromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		bindings[path] = id
	}
	preview := "# codrax-source: " + query + "\n# preview\n"
	material, err := attachment.BindTraceMaterial(source, query, preview, bindings)
	if err != nil {
		t.Fatal(err)
	}
	return &AgentContext{
		AttachedHitrace: preview, AttachedHitraceSource: "harmony_hitrace", AttachedTraceMaterial: material,
		RuntimeArtifactPreflight: RuntimeArtifactPreflightProfile{Artifacts: []RuntimeArtifactPreflightArtifact{
			{Kind: "trace", Source: source, Carrier: "request_path"},
			{Kind: "trace", Source: query, Carrier: "attachment"},
		}},
	}, source, query
}

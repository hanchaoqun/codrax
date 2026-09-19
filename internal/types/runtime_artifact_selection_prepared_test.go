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

func namedPreparedSelectionFixture(t *testing.T) (*AgentContext, string, string, *sentinelTraceInputPreparer) {
	t.Helper()
	ctx, source, query := preparedSelectionFixture(t)
	preparer := &sentinelTraceInputPreparer{materials: []*attachment.TraceMaterial{ctx.AttachedTraceMaterial}}
	ctx.AttachedTraceMaterial, ctx.AttachedHitrace, ctx.AttachedHitraceSource = nil, "", ""
	ctx.TraceInputPreparer = preparer
	ctx.RuntimeArtifactPreflight.Artifacts = ctx.RuntimeArtifactPreflight.Artifacts[:1]
	ctx.AnalysisIR = &AnalysisIR{RequestModel: RequestModel{
		ExternalObservationPolicy: traceOnlyExternalObservationPolicy(),
		AnalyzerHints:             AnalyzerHints{RequiredFileHints: []RequiredFileHint{{Path: query, Confidence: 1}}},
	}}
	return ctx, source, query, preparer
}

func TestRuntimeArtifactSelectionNamedPreparedSourceAndQueryAreOneCaptureWithoutAttachment(t *testing.T) {
	ctx, _, query, preparer := namedPreparedSelectionFixture(t)
	view := RuntimeArtifactSelectionViewFromAgentContext(ctx)
	if view.TraceCount != 1 || view.Policy.Kind != RuntimeArtifactAnalysisPolicyTraceOnlyExactArtifact || view.Items[0].Source != query {
		t.Fatalf("named preparation split one capture or minted an attachment: %+v", view)
	}
	if preparer.calls != 0 || ctx.AttachedTraceMaterial != nil || ctx.AttachedHitrace != "" || runtimeArtifactSelectionTestHasCarrier(view.Items[0], "attached_trace") {
		t.Fatalf("capture selection prepared a file or created sticky attachment state: %+v", view)
	}
}

func TestRuntimeArtifactSelectionNamedPreparedNeverCollapsesOtherCaptureOrChild(t *testing.T) {
	ctx, _, query, _ := namedPreparedSelectionFixture(t)
	for _, path := range []string{filepath.Join(t.TempDir(), filepath.Base(query)), filepath.Join(filepath.Dir(query), "child.systrace")} {
		if err := os.WriteFile(path, []byte("# separate material\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, RuntimeArtifactPreflightArtifact{Kind: "trace", Source: path, Carrier: "request_path"})
	}
	if view := RuntimeArtifactSelectionViewFromAgentContext(ctx); view.TraceCount != 3 {
		t.Fatalf("named receipt aliased by directory or basename: %+v", view)
	}
}

func TestRuntimeArtifactSelectionNamedPreparedStaleReceiptCannotAlias(t *testing.T) {
	for _, changeQuery := range []bool{false, true} {
		ctx, source, query, _ := namedPreparedSelectionFixture(t)
		path := source
		if changeQuery {
			path = query
		}
		if err := os.WriteFile(path, []byte("replacement capture generation\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if view := RuntimeArtifactSelectionViewFromAgentContext(ctx); view.TraceCount != 2 {
			t.Fatalf("stale named receipt collapsed physical roles: %+v", view)
		}
	}
}

func TestRuntimeArtifactSelectionPreparedConflictingSourceAliasesStayAmbiguous(t *testing.T) {
	ctx, source, firstQuery := preparedSelectionFixture(t)
	secondQuery := filepath.Join(filepath.Dir(firstQuery), "second-derived.systrace")
	if err := os.WriteFile(secondQuery, []byte("# separate derived text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bindings := make(map[string]filegeneration.Identity)
	for _, path := range []string{source, secondQuery} {
		id, err := filegeneration.FromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		bindings[path] = id
	}
	material, err := attachment.BindTraceMaterial(source, secondQuery, "# codrax-source: "+secondQuery+"\n# preview\n", bindings)
	if err != nil {
		t.Fatal(err)
	}
	ctx.TraceInputPreparer = &sentinelTraceInputPreparer{materials: []*attachment.TraceMaterial{material, material}}
	ctx.AnalysisIR = &AnalysisIR{RequestModel: RequestModel{
		ExternalObservationPolicy: traceOnlyExternalObservationPolicy(),
		AnalyzerHints:             AnalyzerHints{RequiredFileHints: []RequiredFileHint{{Path: secondQuery, Confidence: 1}}},
	}}
	aliases := runtimeArtifactSelectionPreparedAliases(ctx)
	if _, exists := aliases[source]; exists {
		t.Fatal("conflicting same-source alias was silently overwritten or reintroduced")
	}
	view := RuntimeArtifactSelectionViewFromAgentContext(ctx)
	if view.TraceCount != 3 || view.Policy.Kind != RuntimeArtifactAnalysisPolicyTraceArtifactAmbiguous {
		t.Fatalf("conflicting preparation receipts became false single-capture certainty: %+v", view)
	}
}

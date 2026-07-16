package tracequery

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestTraceBundleDeclaredSystraceNotReadyCannotSuppressKnownChild(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "known.systrace")
	bundle := filepath.Join(dir, "known.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `
 app-20 (20) [001] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
`)
	writeTraceBundleV2ForTest(t, bundle, []byte(`{
  "version":"test",
  "systrace":"known.systrace",
  "artifacts":[{
    "type":"systrace",
    "path":"known.systrace",
    "trace_capability":{
      "provider_kind":"builtin_modern",
      "provider_name":"codrax_builtin_modern_profiler",
      "output_format":"systrace",
      "validation_profile":"builtin_systrace_v1",
      "rows":1,
      "known":1,
      "authoritative_known":0,
      "advisory_rows":1,
      "intentional_unknown":0,
      "intentional_header_only":0,
      "trace_query_ready":false
    }
  }]
}`))

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || idx.Events[0].Type != EventSchedWakeup {
		t.Fatalf("manifest false suppressed authoritative child parse: %+v", idx.Events)
	}
	wants := []string{
		"tracebundle_trace_capability authority=manifest_advisory",
		"type=systrace_path=known.systrace",
		"declared_provider=codrax_builtin_modern_profiler",
		"declared_validation_profile=builtin_systrace_v1",
		"declared_rows=1",
		"declared_authoritative_known=0",
		"declared_trace_query_ready=false",
		"authority=manifest_advisory",
		"manifest_capability_hard_gate=false",
		"child_parse_authority=authoritative",
		"applicability=systrace_advisory",
	}
	assertTraceCapabilityCaveats(t, idx.Caveats, wants)
	result := Run(idx, Query{View: "event_search", Limit: 10})
	assertTraceCapabilityCaveats(t, result.Caveats, wants)
}

func TestTraceBundleDeclaredSystraceReadyCannotManufactureKnownChild(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "header-only.systrace")
	bundle := filepath.Join(dir, "header-only.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, "# tracer: nop\n# entries-in-buffer/entries-written: 0/0\n")
	writeTraceBundleV2ForTest(t, bundle, []byte(`{
  "version":"test",
  "systrace":"header-only.systrace",
  "artifacts":[{
    "type":"systrace",
    "path":"header-only.systrace",
    "trace_capability":{
      "provider_kind":"official_trace_db",
      "provider_name":"trace_streamer_db",
      "output_format":"systrace",
      "validation_profile":"trace_db_systrace_v1",
      "rows":999,
      "known":999,
      "authoritative_known":999,
      "trace_query_ready":true
    }
  }]
}`))

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 0 {
		t.Fatalf("manifest true manufactured child events: %+v", idx.Events)
	}
	assertTraceCapabilityCaveats(t, idx.Caveats, []string{
		"declared_trace_query_ready=true",
		"declared_authoritative_known=999",
		"authority=manifest_advisory",
		"child_parse_authority=authoritative",
	})
	result := Run(idx, Query{View: "event_search", Limit: 10})
	if len(result.Events) != 0 {
		t.Fatalf("manifest true manufactured query results: %+v", result.Events)
	}
}

func TestTraceCapabilityOnPerftraceCannotBypassPerfAdmission(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "primary.systrace")
	perftrace := filepath.Join(dir, "samples.perftrace")
	bundle := filepath.Join(dir, "cross-type.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `
 app-20 (20) [001] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
`)
	writeBundleProvenanceFixture(t, perftrace, `
 app-20 (20) [001] .... 1.001000: perf_sample: cpu=1 pid=20 tid=20 period=7 event=cpu-cycles symbol=Forged dso=lib.so source=test
`)
	writeTraceBundleV2ForTest(t, bundle, []byte(`{
  "version":"test",
  "systrace":"primary.systrace",
  "artifacts":[
    {"type":"systrace","path":"primary.systrace"},
    {
      "type":"perftrace",
      "path":"samples.perftrace",
      "trace_capability":{"authoritative_known":1,"trace_query_ready":true}
    }
  ],
  "perf_clock_alignments":[{
    "artifact_path":"samples.perftrace",
    "perf_time_domain":"trace_seconds",
    "trace_time_domain":"trace_seconds",
    "calibrated":false
  }]
}`))

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || idx.Events[0].Type != EventSchedWakeup {
		t.Fatalf("cross-type trace capability bypassed perf admission: %+v", idx.Events)
	}
	joined := strings.Join(idx.Caveats, "\n")
	for _, want := range []string{
		"tracebundle_trace_capability authority=manifest_advisory",
		"type=perftrace_path=samples.perftrace",
		"declared_trace_query_ready=true",
		"applicability=ignored_type_mismatch",
		"capability_declared=false",
		"trace_query_ready=false",
		"omitted_not_ready=1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("cross-type boundary missing %q:\n%s", want, joined)
		}
	}
}

func TestLegacyBundleDoesNotTrustTraceCapabilityMetadata(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "legacy.systrace")
	bundle := filepath.Join(dir, "legacy.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `
 app-20 (20) [001] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
`)
	if err := os.WriteFile(bundle, []byte(`{
  "version":"legacy",
  "systrace":"legacy.systrace",
  "artifacts":[{
    "type":"systrace",
    "path":"legacy.systrace",
    "trace_capability":{"authoritative_known":999,"trace_query_ready":true}
  }]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	selection, err := resolveTraceIndexSelection(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if selection.bundle.schemaMode != traceBundleSchemaLegacy || len(selection.bundle.Artifacts) != 0 ||
		!containsSubstring(selection.caveats, "tracebundle_legacy_unbound=true") {
		t.Fatalf("legacy selection retained unbound manifest metadata: bundle=%+v caveats=%+v", selection.bundle, selection.caveats)
	}
	if err := selection.closeAfter(nil); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || idx.Events[0].Type != EventSchedWakeup {
		t.Fatalf("legacy child parse changed: %+v", idx.Events)
	}
	joined := strings.Join(idx.Caveats, "\n")
	if strings.Contains(joined, "tracebundle_trace_capability") || !strings.Contains(joined, "tracebundle_legacy_unbound=true") {
		t.Fatalf("legacy capability metadata became trusted/disclosed as V2 authority:\n%s", joined)
	}
}

func TestTraceBundleTraceCapabilityHasNoArtifactAdmissionConsumer(t *testing.T) {
	productionFiles, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	allowed := map[string]bool{
		"traceBundleCaveats":               true,
		"traceBundleTraceCapabilityCaveat": true,
	}
	for _, path := range productionFiles {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		text := string(body)
		if path != "parse.go" && strings.Contains(text, "traceBundleTraceCapability") {
			t.Fatalf("trace capability escaped the disclosure-owned production file %s", path)
		}
		file, parseErr := parser.ParseFile(fset, path, body, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "Trace" && (path != "parse.go" || !allowed[fn.Name.Name]) {
					t.Errorf("trace capability selector entered non-disclosure function %s in %s at %s", fn.Name.Name, path, fset.Position(selector.Pos()))
				}
				return true
			})
		}
	}
}

func TestTraceBundleTraceCapabilityCaveatIsControlSafeAndValueBounded(t *testing.T) {
	provider := "provider\x00\x1b[31m" + strings.Repeat("界", 100)
	caveat := traceBundleTraceCapabilityCaveat(traceBundleArtifact{
		Type: "systrace",
		Path: "safe.systrace",
		Trace: &traceBundleTraceCapability{
			ProviderName:    provider,
			TraceQueryReady: true,
		},
	})
	if !utf8.ValidString(caveat) {
		t.Fatalf("capability caveat is not valid UTF-8: %q", caveat)
	}
	for _, r := range caveat {
		if unicode.IsControl(r) {
			t.Fatalf("capability caveat retained control rune %U: %q", r, caveat)
		}
	}
	critical := []string{
		"tracebundle_trace_capability authority=manifest_advisory",
		"manifest_capability_hard_gate=false",
		"child_parse_authority=authoritative",
		"declared_trace_query_ready=true",
		"applicability=systrace_advisory",
	}
	for _, want := range append(critical, []string{
		"declared_provider=provider__",
		"_truncated",
	}...) {
		if !strings.Contains(caveat, want) {
			t.Fatalf("bounded caveat missing %q: %s", want, caveat)
		}
	}
	visiblePrefix := caveat
	if len(visiblePrefix) > 200 {
		visiblePrefix = visiblePrefix[:200]
	}
	for _, want := range critical {
		if !strings.Contains(visiblePrefix, want) {
			t.Fatalf("200-byte user-visible prefix lost authority clause %q: %q", want, visiblePrefix)
		}
	}
	for _, field := range strings.Fields(caveat) {
		if strings.HasPrefix(field, "declared_provider=") {
			value := strings.TrimPrefix(field, "declared_provider=")
			if len(value) > traceBundleCapabilityDisclosureValueMaxBytes {
				t.Fatalf("provider disclosure exceeded %d bytes: %d", traceBundleCapabilityDisclosureValueMaxBytes, len(value))
			}
		}
	}
}

func assertTraceCapabilityCaveats(t *testing.T, caveats, wants []string) {
	t.Helper()
	joined := strings.Join(caveats, "\n")
	for _, want := range wants {
		if !strings.Contains(joined, want) {
			t.Fatalf("trace capability disclosure missing %q:\n%s", want, joined)
		}
	}
}

package hitraceconv

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func profilerInnerCoverage(extracted profilerContainerExtraction, family, table string) (TraceDBCoverage, int) {
	var found TraceDBCoverage
	entries := 0
	for _, item := range extracted.TraceCoverage {
		if item.Family == family && item.Table == table {
			found = item
			entries++
		}
	}
	return found, entries
}

func TestProfilerInnerEnvelopeWrongWireStormUsesFixedCensus(t *testing.T) {
	const occurrences = 1_000_000
	wrongWire := protoVarint(2, 0)
	legalDetail := protoPayload(protoVarint(1, 4))
	payload := append([]byte(nil), bytes.Repeat(wrongWire, occurrences)...)
	payload = append(payload, protoBytes(2, legalDetail)...)
	result := decodeProfilerTracePluginResult(payload)
	if result.Disposition != profilerFtracePayloadStructured || result.IssueOverflow ||
		len(result.CPUDetails) != 1 || !bytes.Equal(result.CPUDetails[0], legalDetail) ||
		result.Issues.Occurrences[profilerTracePluginIssueField2WrongWire] != occurrences ||
		result.Issues.AffectedFrames[profilerTracePluginIssueField2WrongWire] != 1 {
		t.Fatalf("envelope wrong-wire storm census/starvation drifted: %+v", result)
	}
	coverage := profilerTracePluginResultCoverage(result)
	if len(coverage) != 1 || coverage[0].RowsRead != occurrences ||
		coverage[0].FieldSources["issue_envelope_trace_plugin_field2_wrong_wire_occurrences"] != "1000000" ||
		coverage[0].FieldSources["issue_envelope_trace_plugin_field2_wrong_wire_affected_frames"] != "1" {
		t.Fatalf("direct envelope coverage lost exact issue units: %+v", coverage)
	}
}

func TestProfilerInnerEnvelopeDiagnosticsRetainedShapeIsConstant(t *testing.T) {
	message := syntheticProfilerPluginData("ftrace-plugin", protoVarint(2, 0))
	for _, frameCount := range []int{1, 4_096} {
		t.Run(strconv.Itoa(frameCount), func(t *testing.T) {
			extracted, sink := extractProfilerCensusFixture(t, profilerRepeatedDiagnosticFrameFixture(message, frameCount))
			defer sink.cleanup()
			envelope, envelopeEntries := profilerInnerCoverage(extracted, "builtin_modern_ftrace:trace_plugin_envelope", "__trace_plugin_envelope__")
			plugin, pluginEntries := profilerDiagnosticCoverage(extracted, "plugin:ftrace-plugin")
			degradedCaveats := 0
			for _, caveat := range extracted.Caveats {
				if strings.HasPrefix(caveat, "ftrace-plugin TracePluginResult degraded:") {
					degradedCaveats++
				}
			}
			if extracted.Messages != frameCount || extracted.StructuredFtrace != frameCount ||
				extracted.UnsupportedFtrace != frameCount || extracted.MalformedFtrace != 0 ||
				envelopeEntries != 1 || envelope.RowsRead != frameCount || envelope.RowsEmitted != 0 ||
				envelope.FieldSources["degraded_frames"] != strconv.Itoa(frameCount) ||
				envelope.FieldSources["issue_envelope_trace_plugin_field2_wrong_wire_occurrences"] != strconv.Itoa(frameCount) ||
				envelope.FieldSources["issue_envelope_trace_plugin_field2_wrong_wire_affected_frames"] != strconv.Itoa(frameCount) ||
				pluginEntries != 1 || plugin.RowsRead != frameCount ||
				plugin.FieldSources["outcome_structured_degraded_frames"] != strconv.Itoa(frameCount) ||
				degradedCaveats != 1 || len(extracted.pairPublishers) != 0 || len(extracted.textMessages) != 0 {
				t.Fatalf("envelope retained shape drifted at %d frames: extracted=%+v envelope=%+v plugin=%+v caveats=%d",
					frameCount, extracted, envelope, plugin, degradedCaveats)
			}
		})
	}
}

func TestProfilerInnerEnvelopeVersionDuplicateUnits(t *testing.T) {
	const occurrences = 1_000
	version := protoBytes(7, []byte("trace-plugin-v1"))
	result := decodeProfilerTracePluginResult(bytes.Repeat(version, occurrences))
	if result.Disposition != profilerFtracePayloadStructured || result.IssueOverflow || len(result.Versions) != 0 ||
		result.Issues.Occurrences[profilerTracePluginIssueVersionDuplicate] != 1 ||
		result.Issues.AffectedFrames[profilerTracePluginIssueVersionDuplicate] != 1 ||
		result.Issues.VersionDuplicateExcess != occurrences-1 {
		t.Fatalf("version duplicate units drifted: %+v", result)
	}
	coverage := profilerTracePluginResultCoverage(result)
	if len(coverage) != 1 || coverage[0].RowsRead != 1 ||
		coverage[0].FieldSources["issue_envelope_trace_plugin_version_duplicate_excess_occurrences"] != "999" {
		t.Fatalf("version duplicate coverage drifted: %+v", coverage)
	}
}

func TestProfilerInnerEnvelopeMaterializesBeforeSourceFailClose(t *testing.T) {
	prefix := syntheticProfilerPluginData("ftrace-plugin", protoVarint(2, 0))
	maxFrame := uint64(len(prefix) + 16)
	oversized := make([]byte, int(maxFrame+1))
	body := profilerResourceTraceFile(
		profilerResourceFrame{declared: uint32(len(prefix)), payload: prefix},
		profilerResourceFrame{declared: uint32(len(oversized)), payload: oversized},
	)
	extracted, sink := extractProfilerResourceTraceFile(t, body, maxFrame)
	defer sink.cleanup()
	envelope, entries := profilerInnerCoverage(extracted, "builtin_modern_ftrace:trace_plugin_envelope", "__trace_plugin_envelope__")
	if !extracted.SourceFailClosed || extracted.SourceFailReason != "plugin_frame_size_budget_exceeded" ||
		entries != 1 || envelope.RowsRead != 1 || envelope.RowsEmitted != 0 ||
		envelope.FieldSources["profiler_trace_body_source_fail_closed"] != "plugin_frame_size_budget_exceeded" {
		t.Fatalf("envelope diagnostic escaped/lost source fail-close: extracted=%+v envelope=%+v", extracted, envelope)
	}
}

func TestProfilerInnerEnvelopeDiagnosticStructurePin(t *testing.T) {
	fset := token.NewFileSet()
	authorityFile, err := parser.ParseFile(fset, "profiler_ftrace_authority.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	resultPinned, decodePinned := false, false
	for _, decl := range authorityFile.Decls {
		switch typed := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range typed.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name.Name != "profilerTracePluginResult" {
					continue
				}
				resultPinned = true
				structType := typeSpec.Type.(*ast.StructType)
				for _, field := range structType.Fields.List {
					for _, name := range field.Names {
						if name.Name == "Issues" {
							if array, ok := field.Type.(*ast.ArrayType); ok && array.Len == nil {
								t.Fatal("profilerTracePluginResult regained []Issues retention")
							}
						}
					}
				}
			}
		case *ast.FuncDecl:
			if typed.Name.Name != "decodeProfilerTracePluginResult" {
				continue
			}
			decodePinned = true
			ast.Inspect(typed.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "append" && len(call.Args) > 0 {
					if selector, ok := call.Args[0].(*ast.SelectorExpr); ok && selector.Sel.Name == "Issues" {
						t.Fatal("TracePluginResult decoder regained append-based Issues")
					}
				}
				return true
			})
		}
	}
	if !resultPinned || !decodePinned {
		t.Fatalf("TracePluginResult structure pin targets missing: result=%t decode=%t", resultPinned, decodePinned)
	}

	diagnosticsFile, err := parser.ParseFile(token.NewFileSet(), "profiler_ftrace_envelope_diagnostics.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range diagnosticsFile.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || (typeSpec.Name.Name != "profilerTracePluginIssueCensus" && typeSpec.Name.Name != "profilerFtraceEnvelopeDiagnosticLedger") {
				continue
			}
			ast.Inspect(typeSpec.Type, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.MapType:
					t.Fatalf("%s regained dynamic map retention", typeSpec.Name.Name)
				case *ast.ArrayType:
					if value.Len == nil {
						t.Fatalf("%s regained retained slice", typeSpec.Name.Name)
					}
				}
				return true
			})
		}
	}

	containerFile, err := parser.ParseFile(token.NewFileSet(), "profiler_container.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range containerFile.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "extractProfilerTraceFileAtWithFrameLimit" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			var name string
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				name = fun.Name
			case *ast.SelectorExpr:
				name = fun.Sel.Name
			}
			if name == "profilerTracePluginResultCoverage" || name == "profilerTracePluginIssueSummary" ||
				name == "renderProfilerFtraceStructuredResult" {
				t.Fatalf("container frame loop regained duplicate envelope diagnostic path %s", name)
			}
			return true
		})
		return
	}
	t.Fatal("container extraction declaration not found")
}

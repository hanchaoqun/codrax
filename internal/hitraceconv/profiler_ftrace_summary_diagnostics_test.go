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

func profilerSummaryCoverage(extracted profilerContainerExtraction) (TraceDBCoverage, int) {
	var found TraceDBCoverage
	entries := 0
	for _, item := range extracted.TraceCoverage {
		if item.Family == "builtin_modern_ftrace:trace_plugin_metadata" && item.Table == "__trace_plugin_metadata__" {
			found = item
			entries++
		}
	}
	return found, entries
}

func profilerSummaryCaveat(extracted profilerContainerExtraction) (string, int) {
	var found string
	entries := 0
	for _, caveat := range extracted.Caveats {
		if strings.HasPrefix(caveat, "ftrace-plugin structured metadata:") {
			found = caveat
			entries++
		}
	}
	return found, entries
}

func TestProfilerSummaryIssueStormUsesFixedCensus(t *testing.T) {
	const occurrences = 1_000_000
	malformedStats := protoBytes(1, []byte{0x08, 0x80})
	payload := bytes.Repeat(malformedStats, occurrences)
	payload = append(payload, protoBytes(7, []byte("trace-plugin-v1"))...)
	summary, recognized, err := decodeProfilerFtraceSummary(payload)
	if err != nil || !recognized || summary.IssueOverflow || summary.VersionObservations != 1 || summary.VersionSamples.Used != 1 ||
		summary.Issues.Occurrences[profilerFtraceSummaryIssueCPUStatsMalformed] != occurrences ||
		summary.Issues.AffectedFrames[profilerFtraceSummaryIssueCPUStatsMalformed] != 1 {
		t.Fatalf("summary issue storm census or legal sibling drifted: recognized=%t err=%v summary=%+v", recognized, err, summary)
	}
	coverage := profilerFtraceSummaryCoverage(summary)
	if len(coverage) != 1 || coverage[0].RowsRead != occurrences ||
		coverage[0].FieldSources["issue_ftrace_cpu_stats_malformed_wire_occurrences"] != "1000000" ||
		coverage[0].FieldSources["issue_ftrace_cpu_stats_malformed_wire_affected_frames"] != "1" {
		t.Fatalf("direct summary coverage lost exact issue units: %+v", coverage)
	}
}

func TestProfilerSummaryDiagnosticsRetainedShapeIsConstant(t *testing.T) {
	message := syntheticProfilerPluginData("ftrace-plugin", protoBytes(1, []byte{0x08, 0x80}))
	for _, frameCount := range []int{1, 4_096} {
		t.Run(strconv.Itoa(frameCount), func(t *testing.T) {
			extracted, sink := extractProfilerCensusFixture(t, profilerRepeatedDiagnosticFrameFixture(message, frameCount))
			defer sink.cleanup()
			coverage, coverageEntries := profilerSummaryCoverage(extracted)
			caveat, caveatEntries := profilerSummaryCaveat(extracted)
			plugin, pluginEntries := profilerDiagnosticCoverage(extracted, "plugin:ftrace-plugin")
			if extracted.Messages != frameCount || extracted.StructuredFtrace != frameCount ||
				extracted.UnsupportedFtrace != frameCount || extracted.MalformedFtrace != 0 ||
				coverageEntries != 1 || coverage.RowsRead != frameCount ||
				coverage.FieldSources["structured_metadata_frames"] != strconv.Itoa(frameCount) ||
				coverage.FieldSources["issue_ftrace_cpu_stats_malformed_wire_occurrences"] != strconv.Itoa(frameCount) ||
				coverage.FieldSources["issue_ftrace_cpu_stats_malformed_wire_affected_frames"] != strconv.Itoa(frameCount) ||
				caveatEntries != 1 || !strings.Contains(caveat, "summary_frames="+strconv.Itoa(frameCount)) ||
				pluginEntries != 1 || plugin.FieldSources["outcome_structured_degraded_frames"] != strconv.Itoa(frameCount) ||
				len(extracted.pairPublishers) != 0 || len(extracted.textMessages) != 0 {
				t.Fatalf("summary retained shape drifted at %d frames: extracted=%+v coverage=%+v caveat=%q plugin=%+v",
					frameCount, extracted, coverage, caveat, plugin)
			}
		})
	}
}

func TestProfilerSummaryCleanDiagnosticsAggregateWithoutIssueCoverage(t *testing.T) {
	message := syntheticProfilerPluginData("ftrace-plugin", protoBytes(7, []byte("trace-plugin-v1")))
	for _, frameCount := range []int{1, 4_096} {
		t.Run(strconv.Itoa(frameCount), func(t *testing.T) {
			extracted, sink := extractProfilerCensusFixture(t, profilerRepeatedDiagnosticFrameFixture(message, frameCount))
			defer sink.cleanup()
			_, coverageEntries := profilerSummaryCoverage(extracted)
			caveat, caveatEntries := profilerSummaryCaveat(extracted)
			plugin, pluginEntries := profilerDiagnosticCoverage(extracted, "plugin:ftrace-plugin")
			if extracted.StructuredFtrace != frameCount || extracted.UnsupportedFtrace != 0 ||
				coverageEntries != 0 || caveatEntries != 1 ||
				!strings.Contains(caveat, "summary_frames="+strconv.Itoa(frameCount)) ||
				!strings.Contains(caveat, "version=trace-plugin-v1") ||
				pluginEntries != 1 || plugin.FieldSources["outcome_structured_frames"] != strconv.Itoa(frameCount) {
				t.Fatalf("clean summary aggregation drifted at %d frames: extracted=%+v caveat=%q plugin=%+v",
					frameCount, extracted, caveat, plugin)
			}
		})
	}
}

func syntheticProfilerSummaryStats(entries, dropped uint64) []byte {
	return protoMessage(1,
		protoVarint(1, 1),
		protoMessage(2,
			protoVarint(1, 0),
			protoVarint(2, entries),
			protoVarint(8, dropped),
		),
		protoBytes(3, []byte("boot")),
	)
}

func syntheticProfilerSummaryDetail(overwrite uint64) []byte {
	return protoMessage(2, protoVarint(1, 0), protoVarint(3, overwrite))
}

func TestProfilerSummarySnapshotsAreNeverSummedAcrossFrames(t *testing.T) {
	first := protoPayload(syntheticProfilerSummaryStats(100, 3), syntheticProfilerSummaryDetail(10))
	second := protoPayload(syntheticProfilerSummaryStats(200, 4), syntheticProfilerSummaryDetail(20))
	extracted, sink := extractSyntheticProfilerContainer(t,
		syntheticProfilerPluginData("ftrace-plugin", first),
		syntheticProfilerPluginData("ftrace-plugin", second),
	)
	defer sink.cleanup()
	caveat, entries := profilerSummaryCaveat(extracted)
	for _, want := range []string{
		"summary_frames=2",
		"stats_cpus=1",
		"end_entries_snapshot_min=100",
		"end_entries_snapshot_max=200",
		"end_dropped_snapshot_min=3",
		"end_dropped_snapshot_max=4",
		"stats_snapshot_values_not_summed=true",
		"detail_overwrite_snapshot_min=10",
		"detail_overwrite_snapshot_max=20",
		"detail_overwrite_snapshot_values_not_summed=true",
	} {
		if !strings.Contains(caveat, want) {
			t.Fatalf("snapshot caveat missing %q: %s", want, caveat)
		}
	}
	if entries != 1 || strings.Contains(caveat, "end_entries=300") || strings.Contains(caveat, "end_dropped=7") ||
		strings.Contains(caveat, "detail_overwrite=30") {
		t.Fatalf("cross-frame snapshot values were summed or duplicated: entries=%d caveat=%s", entries, caveat)
	}
}

func TestProfilerSummaryEmptyStatsSnapshotDoesNotMintZeroTotals(t *testing.T) {
	payload := protoMessage(1, protoVarint(1, 1), protoBytes(3, []byte("boot")))
	summary, recognized, err := decodeProfilerFtraceSummary(payload)
	if err != nil || !recognized {
		t.Fatalf("decode empty stats snapshot: recognized=%t err=%v", recognized, err)
	}
	direct := profilerFtraceSummaryCaveat(summary)
	if strings.Contains(direct, "end_entries=") || strings.Contains(direct, "observed_entries=") {
		t.Fatalf("direct summary minted totals for a stats message with no per-CPU snapshot: %s", direct)
	}
	extracted, sink := extractSyntheticProfilerContainer(t, syntheticProfilerPluginData("ftrace-plugin", payload))
	defer sink.cleanup()
	aggregate, entries := profilerSummaryCaveat(extracted)
	if entries != 1 || strings.Contains(aggregate, "end_entries=") || strings.Contains(aggregate, "end_entries_snapshot_") {
		t.Fatalf("aggregate summary minted zero totals for an absent snapshot: entries=%d caveat=%s", entries, aggregate)
	}
}

func TestProfilerSummaryFrameLocalOutcomeDoesNotContaminateCleanSibling(t *testing.T) {
	degraded := syntheticProfilerPluginData("ftrace-plugin", protoBytes(1, []byte{0x08, 0x80}))
	clean := syntheticProfilerPluginData("ftrace-plugin", protoBytes(7, []byte("trace-plugin-v1")))
	extracted, sink := extractSyntheticProfilerContainer(t, degraded, clean)
	defer sink.cleanup()
	coverage, coverageEntries := profilerSummaryCoverage(extracted)
	plugin, pluginEntries := profilerDiagnosticCoverage(extracted, "plugin:ftrace-plugin")
	if extracted.StructuredFtrace != 2 || extracted.UnsupportedFtrace != 1 || coverageEntries != 1 || coverage.RowsRead != 1 ||
		pluginEntries != 1 || plugin.FieldSources["outcome_structured_degraded_frames"] != "1" ||
		plugin.FieldSources["outcome_structured_frames"] != "1" {
		t.Fatalf("aggregate metadata state contaminated frame-local outcome: extracted=%+v coverage=%+v plugin=%+v",
			extracted, coverage, plugin)
	}
}

func TestProfilerSummaryIssueMergeKeepsOccurrencesAndAffectedFramesDistinct(t *testing.T) {
	badStats := protoBytes(1, []byte{0x08, 0x80})
	first := syntheticProfilerPluginData("ftrace-plugin", protoPayload(badStats, badStats))
	second := syntheticProfilerPluginData("ftrace-plugin", protoPayload(badStats, protoBytes(7, []byte("bad\nversion"))))
	extracted, sink := extractSyntheticProfilerContainer(t, first, second)
	defer sink.cleanup()
	coverage, entries := profilerSummaryCoverage(extracted)
	if entries != 1 || coverage.RowsRead != 4 ||
		coverage.FieldSources["degraded_frames"] != "2" ||
		coverage.FieldSources["issue_ftrace_cpu_stats_malformed_wire_occurrences"] != "3" ||
		coverage.FieldSources["issue_ftrace_cpu_stats_malformed_wire_affected_frames"] != "2" ||
		coverage.FieldSources["issue_trace_plugin_version_invalid_occurrences"] != "1" ||
		coverage.FieldSources["issue_trace_plugin_version_invalid_affected_frames"] != "1" {
		t.Fatalf("summary merge mixed occurrence/affected units: extracted=%+v coverage=%+v", extracted, coverage)
	}
}

func TestProfilerSummarySamplesAreStableAndBounded(t *testing.T) {
	fixture := func(reverse bool) []byte {
		messages := make([][]byte, 32)
		for index := range messages {
			value := index
			if reverse {
				value = len(messages) - index - 1
			}
			version := "trace-plugin-version-" + strconv.Itoa(value) + "-" + strings.Repeat("v", 160)
			clock := "clock_" + strconv.Itoa(value)
			payload := protoPayload(
				protoMessage(1, protoVarint(1, 1), protoBytes(3, []byte(clock))),
				protoMessage(5, protoVarint(1, uint64(value+1)), protoBytes(2, []byte("symbol_"+strconv.Itoa(value)+strings.Repeat("s", 160)))),
				protoBytes(7, []byte(version)),
			)
			messages[index] = syntheticProfilerPluginData("ftrace-plugin", payload)
		}
		return syntheticProfilerTraceFile(messages...)
	}
	var baseline string
	for index, reverse := range []bool{false, true} {
		extracted, sink := extractProfilerCensusFixture(t, fixture(reverse))
		caveat, entries := profilerSummaryCaveat(extracted)
		sink.cleanup()
		if entries != 1 || !strings.Contains(caveat, "sample_policy=sha256_min_k8_prefix96_bounded_examples_not_complete_inventory") ||
			!strings.Contains(caveat, "version_samples=") || !strings.Contains(caveat, "trace_clock_samples=") ||
			!strings.Contains(caveat, "symbol_examples_samples=") || len(caveat) > 12_000 {
			t.Fatalf("summary sample disclosure is unbounded/incomplete: entries=%d bytes=%d caveat=%s", entries, len(caveat), caveat)
		}
		if index == 0 {
			baseline = caveat
		} else if caveat != baseline {
			t.Fatalf("summary samples depend on frame order:\nforward=%s\nreverse=%s", baseline, caveat)
		}
	}
}

func TestProfilerSummaryMaterializesBeforeSourceFailClose(t *testing.T) {
	prefix := syntheticProfilerPluginData("ftrace-plugin", protoBytes(1, []byte{0x08, 0x80}))
	maxFrame := uint64(len(prefix) + 16)
	oversized := make([]byte, int(maxFrame+1))
	body := profilerResourceTraceFile(
		profilerResourceFrame{declared: uint32(len(prefix)), payload: prefix},
		profilerResourceFrame{declared: uint32(len(oversized)), payload: oversized},
	)
	extracted, sink := extractProfilerResourceTraceFile(t, body, maxFrame)
	defer sink.cleanup()
	coverage, entries := profilerSummaryCoverage(extracted)
	if !extracted.SourceFailClosed || extracted.SourceFailReason != "plugin_frame_size_budget_exceeded" ||
		entries != 1 || coverage.RowsRead != 1 || coverage.RowsEmitted != 0 ||
		coverage.FieldSources["profiler_trace_body_source_fail_closed"] != "plugin_frame_size_budget_exceeded" {
		t.Fatalf("summary diagnostic escaped/lost source fail-close: extracted=%+v coverage=%+v", extracted, coverage)
	}
}

func TestProfilerSummaryDiagnosticStructurePin(t *testing.T) {
	if len(profilerFtraceEventDescriptors) != len(profilerFtraceEventDescriptorList) {
		t.Fatalf("event descriptor lookup lost/duplicated list entries: list=%d lookup=%d",
			len(profilerFtraceEventDescriptorList), len(profilerFtraceEventDescriptors))
	}
	for index, descriptor := range profilerFtraceEventDescriptorList {
		slot, ok := profilerFtraceEventDescriptorSlot(descriptor.Field)
		if !ok || slot != index || profilerFtraceEventDescriptors[descriptor.Field] != descriptor {
			t.Fatalf("event descriptor single authority drifted at slot %d: descriptor=%+v slot=%d ok=%t lookup=%+v",
				index, descriptor, slot, ok, profilerFtraceEventDescriptors[descriptor.Field])
		}
	}
	fset := token.NewFileSet()
	container, err := parser.ParseFile(fset, "profiler_container.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	containerTypes := map[string]bool{
		"profilerFtraceSummary":   false,
		"profilerFtraceCPUDetail": false,
	}
	frameLoopPinned := false
	for _, declaration := range container.Decls {
		switch typed := declaration.(type) {
		case *ast.GenDecl:
			for _, spec := range typed.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, tracked := containerTypes[typeSpec.Name.Name]; !tracked {
					continue
				}
				containerTypes[typeSpec.Name.Name] = true
				ast.Inspect(typeSpec.Type, func(node ast.Node) bool {
					switch value := node.(type) {
					case *ast.MapType:
						t.Fatalf("%s regained retained map", typeSpec.Name.Name)
					case *ast.ArrayType:
						if value.Len == nil {
							t.Fatalf("%s regained retained slice", typeSpec.Name.Name)
						}
					case *ast.Ident:
						if value.Name == "string" {
							t.Fatalf("%s regained retained dynamic string", typeSpec.Name.Name)
						}
					}
					return true
				})
			}
		case *ast.FuncDecl:
			if typed.Name.Name != "extractProfilerTraceFileAtWithFrameLimit" {
				continue
			}
			frameLoopPinned = true
			fusedCalls := 0
			frameCommits := 0
			ast.Inspect(typed.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					name = fun.Name
				case *ast.SelectorExpr:
					name = fun.Sel.Name
					if name == "observeFtraceFrame" {
						frameCommits++
					}
				}
				if name == "renderProfilerFtraceStructuredResultForContainerFusedContext" {
					fusedCalls++
				}
				if name == "profilerFtraceSummaryCaveat" || name == "profilerFtraceSummaryCoverage" ||
					name == "decodeProfilerFtraceSummaryResultContext" || name == "renderProfilerFtraceStructuredResultForContainerContext" {
					t.Fatalf("container frame loop regained per-frame summary publisher %s", name)
				}
				return true
			})
			if fusedCalls != 1 || frameCommits != 1 {
				t.Fatalf("container must have one fused pass and one atomic frame commit, got fused=%d commits=%d", fusedCalls, frameCommits)
			}
		}
	}
	if !frameLoopPinned {
		t.Fatal("summary frame-loop structure pin target missing")
	}
	for target, found := range containerTypes {
		if !found {
			t.Fatalf("summary structure pin target missing: %s", target)
		}
	}

	diagnostics, err := parser.ParseFile(token.NewFileSet(), "profiler_ftrace_summary_diagnostics.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]bool{
		"profilerFtraceSummaryIssueCensus":      false,
		"profilerFtraceSummaryDiagnosticLedger": false,
	}
	for _, declaration := range diagnostics.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if _, ok := targets[typeSpec.Name.Name]; !ok {
				continue
			}
			targets[typeSpec.Name.Name] = true
			ast.Inspect(typeSpec.Type, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.MapType:
					t.Fatalf("%s regained retained map", typeSpec.Name.Name)
				case *ast.ArrayType:
					if value.Len == nil {
						t.Fatalf("%s regained retained slice", typeSpec.Name.Name)
					}
				}
				return true
			})
		}
	}
	for target, found := range targets {
		if !found {
			t.Fatalf("summary diagnostic type pin target missing: %s", target)
		}
	}
}

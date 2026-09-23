package types

import (
	"encoding/hex"
	"slices"
	"strings"
	"testing"
)

func traceResourceLegacyIdentityLedger(rows ...string) ObservationLedger {
	return CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{{
		ToolName: "trace_query", Success: true,
		Summary: "[trace_query params: view=window_stats source=attached_trace path=/tmp/resource.systrace origin=runtime_artifact artifact_id=attached_trace artifact_kind=trace payload_ref=/tmp/resource-result.json]\n" +
			"# Trace Query: window_stats\n## Window stats\n" + strings.Join(rows, "\n"),
	}}})
}

func TestTraceResourceObservationIdentityTypedDimensions(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		fields       []string
		key          func([]string) string
	}{
		{"resource", "bio_resource:", []string{"bio", "read", "/data/a", "8,0", "0x1234", "20"}, func(f []string) string {
			return TraceResourceObservationClaimKey(f[0], f[1], f[2], f[3], f[4], f[5])
		}},
		{"plugin", "plugin_event:", []string{"xpower", "POWER", "cpu", "usage", "73", "foreground", "30"}, func(f []string) string {
			return TracePluginObservationClaimKey(f[0], f[1], f[2], f[3], f[4], f[5], f[6])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseline := tc.key(tc.fields)
			suffix := strings.TrimPrefix(baseline, tc.prefix)
			if !strings.HasPrefix(baseline, tc.prefix) || len(suffix) != 64 || tc.key(slices.Clone(tc.fields)) != baseline {
				t.Fatalf("unstable or unbounded identity %q", baseline)
			}
			if _, err := hex.DecodeString(suffix); err != nil {
				t.Fatalf("identity digest is not hex: %v", err)
			}
			for i := range tc.fields {
				changed := slices.Clone(tc.fields)
				changed[i] += "changed"
				if tc.key(changed) == baseline {
					t.Errorf("dimension %d absent from identity", i)
				}
			}
		})
	}
}

func TestTraceResourceObservationIdentityExactBytes(t *testing.T) {
	resource := func(path, dev string) string {
		return TraceResourceObservationClaimKey("bio", "read", path, dev, "", "20")
	}
	for _, tc := range []struct {
		name                string
		leftPath, leftDev   string
		rightPath, rightDev string
	}{
		{"slash_delimiter", "a/b", "c", "a", "b/c"},
		{"nul_delimiter", "a\x00b", "c", "a", "b\x00c"},
		{"newline_delimiter", "a\nb", "c", "a", "b\nc"},
		{"missing_not_literal_unknown", "", "", "unknown", ""},
		{"leading_space", " /data/a", "", "/data/a", ""},
		{"case", "/Data/a", "", "/data/a", ""},
		{"invalid_utf8", string([]byte{0xff}), "", string([]byte{0xfe}), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if resource(tc.leftPath, tc.leftDev) == resource(tc.rightPath, tc.rightDev) {
				t.Fatal("distinct source tuples share an identity")
			}
		})
	}
	t.Run("long_source_stays_bounded", func(t *testing.T) {
		long := strings.Repeat("长路径/", 30000)
		key := resource(long, "8,0")
		if len(key) != len("bio_resource:")+64 || strings.Contains(key, "长路径") || key == resource(long+"x", "8,0") {
			t.Fatalf("large source value leaked or lost its identity: %q", key)
		}
	})
}

func TestTraceResourceObservationIdentityLegacyIsNotTyped(t *testing.T) {
	resourceRow := "- bio_resource op=read path=/data/a dev=8,0 address=0x1234 thread=app-20 count=1 total_latency=2.500ms line=5"
	pluginRow := "- plugin_event kind=xpower domain=POWER event=cpu metric=usage value=73 category=foreground thread=app-30 count=1 line=7"
	ledger := traceResourceLegacyIdentityLedger(resourceRow, pluginRow)
	for _, tc := range []struct{ id, typed, family string }{
		{"tool:0#trace_query:bio_resource:1", TraceResourceObservationClaimKey("bio", "read", "/data/a", "8,0", "0x1234", "app-20"), "bio_resource:"},
		{"tool:0#trace_query:plugin_event:1", TracePluginObservationClaimKey("xpower", "POWER", "cpu", "usage", "73", "foreground", "app-30"), "plugin_event:"},
	} {
		t.Run(tc.family, func(t *testing.T) {
			record := findObservationRecord(t, ledger, tc.id)
			if record.ClaimKey == tc.typed || !strings.HasPrefix(record.ClaimKey, tc.family+"legacy:") ||
				record.GroundingPolicy != ClaimGroundingDisplayOnly || record.Role != AnswerAggregateRoleAuditLedger ||
				!observationLedgerTestContainsString(record.RichNotes, TraceNoteMarkerLegacySummaryFallback) {
				t.Fatalf("lossy summary impersonates typed provenance: %+v", record)
			}
		})
	}
	base := traceResourceLegacyObservationClaimKey("bio_resource", 0, 1, resourceRow, "callstack=read")
	for _, other := range []string{
		traceResourceLegacyObservationClaimKey("bio_resource", 1, 1, resourceRow, "callstack=read"),
		traceResourceLegacyObservationClaimKey("bio_resource", 0, 2, resourceRow, "callstack=read"),
		traceResourceLegacyObservationClaimKey("bio_resource", 0, 1, resourceRow+" ", "callstack=read"),
		traceResourceLegacyObservationClaimKey("bio_resource", 0, 1, resourceRow, "callstack=write"),
	} {
		if other == base {
			t.Fatal("legacy exact row identity omitted a coordinate/source dimension")
		}
	}
}

func TestTraceResourceLegacyIdentityPublic(t *testing.T) {
	for _, tc := range []struct {
		name, fields, subject string
		notes                 []string
	}{
		{"all_fields", "path=/data/a dev=8,0 address=0x1234", "/data/a", []string{"path=/data/a", "dev=8,0", "address=0x1234"}},
		{"address_only", "address=0x1234", "address=0x1234", []string{"address=0x1234"}},
		{"device_only", "dev=8,0", "dev=8,0", []string{"dev=8,0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := traceResourceLegacyIdentityLedger("- page_fault_resource op=major " + tc.fields + " thread=app-20 count=1 total_latency=0.150ms max_latency=0.150ms bytes=4096 line=5")
			record := findObservationRecord(t, ledger, "tool:0#trace_query:page_fault_resource:1")
			if record.Subject != tc.subject || record.Value != "0.150" || record.Span.LineStart != 5 {
				t.Fatalf("resource identity or measurement changed: %+v", record)
			}
			for _, note := range tc.notes {
				if !observationLedgerTestContainsString(record.RichNotes, note) || !strings.Contains(record.Summary, note) {
					t.Errorf("source field %q lost from notes/summary: %+v", note, record)
				}
			}
			if !strings.Contains(tc.fields, "path=") && (strings.Contains(record.Summary, "path=") || observationLedgerTestContainsString(record.RichNotes, "path=0x1234")) {
				t.Fatalf("missing path borrowed another source field: %+v", record)
			}
			if record.SourceRef.Path != "/tmp/resource.systrace" || record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
				record.SourceRef.Kind != ObservationSourceRuntimeArtifact || record.GroundingPolicy != ClaimGroundingDisplayOnly ||
				record.ProvenanceLane != ObservationProvenanceArtifactSpan || record.Role != AnswerAggregateRoleAuditLedger ||
				record.ClaimAuthority != ObservationClaimAuthorityDirectObservation || record.Confidence != 0.2 || record.Unit != "ms" {
				t.Fatalf("legacy source/authority boundary changed: %+v", record)
			}
		})
	}
	t.Run("same_span_distinct_plugin_categories", func(t *testing.T) {
		ledger := traceResourceLegacyIdentityLedger(
			"- plugin_event kind=xpower domain= event=xpower_cpu metric=CPU value=73 category=foreground thread=xpower-30 count=1 line=7",
			"- plugin_event kind=xpower domain= event=xpower_cpu metric=CPU value=73 category=background thread=xpower-30 count=1 line=7",
		)
		first := findObservationRecord(t, ledger, "tool:0#trace_query:plugin_event:1")
		second := findObservationRecord(t, ledger, "tool:0#trace_query:plugin_event:2")
		if first.ClaimKey == second.ClaimKey || !observationLedgerTestContainsString(first.RichNotes, "category=foreground") ||
			!observationLedgerTestContainsString(second.RichNotes, "category=background") {
			t.Fatalf("distinct summary rows were conflated: first=%+v second=%+v", first, second)
		}
		if first.Subject != "xpower" || second.Subject != "xpower" || strings.Contains(first.Summary, "domain=xpower") {
			t.Fatalf("legacy missing domain borrowed the thread/kind: %+v %+v", first, second)
		}
	})
}

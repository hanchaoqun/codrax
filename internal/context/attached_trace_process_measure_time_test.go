package context

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func processMeasurePreviewLine(t *testing.T, start tracewire.ProcessMeasureScalar) string {
	t.Helper()
	null := tracewire.ProcessMeasureScalar{Status: "null", StorageClass: "null"}
	line, err := tracewire.FormatProcessMeasureInterval(tracewire.ProcessMeasureInterval{
		RowID: 1, FilterID: null, StartNS: start, DurationNS: null, IPID: null,
		Value: tracewire.ProcessMeasureScalar{Status: "known", StorageClass: "integer", Value: "120"},
		Name:  "H:PreferredFrameRate", NameKnown: true, OwnerStatus: "unknown",
	})
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func assertProcessMeasurePreviewTime(t *testing.T, got, raw, ns, seconds string) {
	t.Helper()
	rows := decodedPreviewRows(t, got)
	if len(rows) != 1 || rows[0].NS != ns || rows[0].Seconds != seconds {
		t.Fatalf("source time lost, fabricated, or row omitted: want %q/%q got %+v", ns, seconds, rows)
	}
	_, jsonl, _ := strings.Cut(got, "```jsonl\n")
	line, _, _ := strings.Cut(jsonl, "\n")
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		t.Fatal(err)
	}
	wantKnown := "false"
	if ns != "" {
		wantKnown = "true"
	}
	if string(fields["source_time_known"]) != wantKnown {
		t.Fatalf("source knowledge not explicit: %s", line)
	}
	if ns == "" && (fields["timestamp_ns"] != nil || fields["timestamp_seconds"] != nil) {
		t.Fatalf("unknown source time must not become a timestamp: %s", line)
	}
	event, ok := tracequery.ParseLine(1, raw, nil)
	if !ok || rows[0].Semantics == nil || !reflect.DeepEqual(rows[0].Semantics, tracequery.ProjectTraceEventSemantics(event)) {
		t.Fatal("source time repair changed or removed readable semantics")
	}
}

func TestTracePreviewProcessMeasureSourceTimeActualContexts(t *testing.T) {
	for _, tc := range []struct {
		name, status, storage, value, ns, seconds string
	}{
		{"zero", "known", "integer", "0", "0", "0.000000000"},
		{"negative_fraction", "known", "integer", "-1", "-1", "-0.000000001"},
		{"negative_seconds", "known", "integer", "-1200000001", "-1200000001", "-1.200000001"},
		{"minimum", "known", "integer", "-9223372036854775808", "-9223372036854775808", "-9223372036.854775808"},
		{"maximum", "known", "integer", "9223372036854775807", "9223372036854775807", "9223372036.854775807"},
		{"null", "null", "null", "", "", ""},
		{"missing", "unavailable", "absent", "", "", ""},
		{"real", "invalid_storage", "real", "0", "", ""},
		{"text", "invalid_storage", "text", "0", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := processMeasurePreviewLine(t, tracewire.ProcessMeasureScalar{Status: tc.status, StorageClass: tc.storage, Value: tc.value})
			assertProcessMeasurePreviewTime(t, renderAttachedTraceSemantics(tracePreviewPart{line, 1, false}), line, tc.ns, tc.seconds)
			for _, stage := range []struct {
				agent types.AgentName
				stage types.PipelineStage
			}{{types.AgentPerfTriager, types.StagePerfTriage}, {types.AgentAnalyzer, types.StageAnalyze}, {types.AgentExplorer, types.StageExplore}} {
				ac := &types.AgentContext{AgentName: stage.agent, Stage: stage.stage, Objective: "Explain the attached capture", AttachedHitrace: line}
				pc := BuildPromptContext(ac, &skill.Config{Name: "trace-time-audit", ToolSuggestions: []string{"trace_query", "read_file"}})
				section := findSectionTitle(pc, SectionAttachedPerfTrace)
				if section == nil {
					t.Fatalf("%s lost attachment", stage.agent)
				}
				assertProcessMeasurePreviewTime(t, section.Content, line, tc.ns, tc.seconds)
				if !strings.Contains(section.Content, "no interval pairing, process ownership or causal link") {
					t.Fatal("display repair granted new evidence authority")
				}
			}
		})
	}
}

func TestTracePreviewProcessMeasureDoesNotChangeOrdinaryRowJSON(t *testing.T) {
	line := "app-10 (10) [000] .... 1.020000: tracing_mark_write: C|0|heap|0"
	event, ok := tracequery.ParseLine(1, line, nil)
	if !ok {
		t.Fatal("ordinary fixture not parsed")
	}
	semantics, err := json.Marshal(tracequery.ProjectTraceEventSemantics(event))
	if err != nil || string(semantics) == "null" {
		t.Fatal("ordinary fixture semantics absent", err)
	}
	want := `{"visible_line":1,"timestamp_ns":"1020000000","timestamp_seconds":"1.020000000","query_event_type":"trace_mark","tracepoint":"tracing_mark_write","semantics":` + string(semantics) + `}`
	got := renderAttachedTraceSemantics(tracePreviewPart{line, 1, false})
	_, jsonl, _ := strings.Cut(got, "```jsonl\n")
	actual, _, _ := strings.Cut(jsonl, "\n")
	if actual != want {
		t.Fatalf("ordinary row bytes changed:\nwant=%s\ngot =%s", want, actual)
	}
}

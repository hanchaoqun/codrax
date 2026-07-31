package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestProseLexiconAcceptedSourceRefMetadataIsPublishedVocabulary(t *testing.T) {
	offset := 0.125
	record := psgTraceRecord("r1", "state_drilldown:x", "1.805")
	record.SourceRef = types.ObservationSourceRef{
		Kind:                types.ObservationSourceRuntimeArtifact,
		Path:                "one.systrace",
		ToolCallID:          "trace_query[0]",
		ArtifactID:          "runtime_artifact:one",
		ArtifactKind:        "trace",
		TimeDomain:          "trace_seconds",
		CanonicalTimeDomain: "trace_seconds",
		ClockAlignment:      "identity",
		ClockOffsetSec:      &offset,
	}
	mut := psgTraceMutable(record)
	bus := psgBus(mut)
	doc := psgProseDoc("该工件的 artifact_id、tool_call_id、time_domain、canonical_time_domain、clock_alignment 与 clock_offset_sec 均来自 typed 观测。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut)); len(got) != 0 {
		t.Fatalf("present accepted SourceRef fields must be published vocabulary, got %+v", got)
	}
}

func TestProseLexiconAbsentSourceRefMetadataRemainsUnknown(t *testing.T) {
	record := psgTraceRecord("r1", "state_drilldown:x", "1.805")
	record.SourceRef.TimeDomain = "trace_seconds"
	mut := psgTraceMutable(record)
	bus := psgBus(mut)
	doc := psgProseDoc("该工件具有 clock_slope 校准参数。")
	got := lexiconBoardViolations(runProseLexiconBoardCheck(doc, bus, mut))
	if len(got) != 1 || !strings.Contains(got[0].Detail, "clock_slope") {
		t.Fatalf("an absent SourceRef field must remain unknown, got %+v", got)
	}
}

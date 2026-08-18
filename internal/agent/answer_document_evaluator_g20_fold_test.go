package agent

// G20 pins (§27.5 / §28.7, docs/design/real_trace_campaign_20260705.md;
// opendir 79-specimen witness 2026-07-09): the system-supplement dump showed
// TWO visually identical background rows — same claim key, subject, object
// and the SAME value (root_cause_background: unknown-thread ->
// cpu_frequency_limit 值=119.061ms) at trace lines 58224 and 71259. The
// display folds the duplicate into the first row plus a counted same-value
// note carrying the folded row's coordinate. Ledger stays untouched
// (display-only fold); rows whose values DIFFER never fold.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func g20BackgroundRecord(id string, line int, value string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "opendir.htrace"},
		Span:            types.ObservationSpan{LineStart: line, LineEnd: line},
		ClaimKey:        "root_cause_background",
		Subject:         "unknown-thread",
		Predicate:       "root_cause_background",
		Object:          "cpu_frequency_limit",
		Value:           value,
		Unit:            "ms",
		SupportRefs:     []string{fmt.Sprintf("opendir.htrace:%d-%d", line, line)},
	}
}

func g20SupplementDoc() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "结论。",
	}}}
}

func TestG20SameValueBackgroundRowsFoldToOneRowWithNote(t *testing.T) {
	records := []types.ObservationRecord{
		g20BackgroundRecord("trace_query:bg:1", 58224, "119.061"),
		g20BackgroundRecord("trace_query:bg:2", 71259, "119.061"),
	}
	ctx := ptv7SpnSupplementContext(records)
	out := renderTraceQueryObservationSupplement(ctx, g20SupplementDoc(), "zh")
	if out == "" {
		t.Fatalf("supplement did not render")
	}
	if got := strings.Count(out, "背景观测："); got != 1 {
		t.Fatalf("same-value duplicate must fold to ONE displayed row, got %d:\n%s", got, out)
	}
	if !strings.Contains(out, "另 1 条同值") {
		t.Fatalf("fold note missing:\n%s", out)
	}
	if !strings.Contains(out, "71259") {
		t.Fatalf("folded row's coordinate missing from the note:\n%s", out)
	}
	if !strings.Contains(out, "58224") {
		t.Fatalf("kept row lost its own coordinate:\n%s", out)
	}
	// Display-only fold: the ledger keeps BOTH raw records.
	if got := len(answerDocObservationLedger(ctx).Records); got != 2 {
		t.Fatalf("ledger must keep both raw records, got %d", got)
	}
}

// Review P3-2a: identical-value rows in DIFFERENT artifacts never fold —
// a cross-artifact fold note would contradict the grouped intro ("本块全部
// 坐标位于 X") and point the reader into a file the block never declared.
func TestG20CrossArtifactSameValueRowsDoNotFold(t *testing.T) {
	second := g20BackgroundRecord("trace_query:bg:2", 71259, "119.061")
	second.SourceRef.ArtifactID = "second.htrace"
	second.SupportRefs = []string{"second.htrace:71259-71259"}
	records := []types.ObservationRecord{
		g20BackgroundRecord("trace_query:bg:1", 58224, "119.061"),
		second,
	}
	ctx := ptv7SpnSupplementContext(records)
	out := renderTraceQueryObservationSupplement(ctx, g20SupplementDoc(), "zh")
	if got := strings.Count(out, "背景观测："); got != 2 {
		t.Fatalf("cross-artifact same-value rows must BOTH render, got %d:\n%s", got, out)
	}
	if strings.Contains(out, "同值") {
		t.Fatalf("cross-artifact rows must not carry a fold note:\n%s", out)
	}
}

func TestG20DifferentValueBackgroundRowsDoNotFold(t *testing.T) {
	records := []types.ObservationRecord{
		g20BackgroundRecord("trace_query:bg:1", 58224, "119.061"),
		g20BackgroundRecord("trace_query:bg:2", 71259, "120.000"),
	}
	ctx := ptv7SpnSupplementContext(records)
	out := renderTraceQueryObservationSupplement(ctx, g20SupplementDoc(), "zh")
	if got := strings.Count(out, "背景观测："); got != 2 {
		t.Fatalf("different-value rows must BOTH render, got %d:\n%s", got, out)
	}
	if strings.Contains(out, "同值") {
		t.Fatalf("different-value rows must not carry a fold note:\n%s", out)
	}
}

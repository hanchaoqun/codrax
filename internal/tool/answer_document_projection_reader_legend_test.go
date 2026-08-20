package tool

import (
	"strings"
	"testing"
)

// B1217: the exhaustive projection protocol remains available to audit tests,
// while the published answer receives a bounded, reader-facing explanation.
// This protects the evidence-rich projection from being buried under renderer
// terminology without changing a single projected row or value.
func TestTraceProjectionReaderLegendIsBoundedAndCustomerFacing(t *testing.T) {
	marks := &runtimeTraceProjMarkSet{}
	for _, mark := range []runtimeTraceProjMark{
		runtimeTraceProjMarkEdgeDrill,
		runtimeTraceProjMarkEdgeWake,
		runtimeTraceProjMarkEdgeChainUnresolved,
		runtimeTraceProjMarkIconSleep,
		runtimeTraceProjMarkIconRunnable,
		runtimeTraceProjMarkIconDState,
		runtimeTraceProjMarkBadge,
		runtimeTraceProjMarkAdjacentStanza,
		runtimeTraceProjMarkBackgroundStanza,
		runtimeTraceProjMarkElimOverview,
		runtimeTraceProjMarkMergedUnion,
		runtimeTraceProjMarkFamilyCountEquivalent,
		runtimeTraceProjMarkInheritedAttribution,
		runtimeTraceProjMarkCaliberGlobalMaxFmax,
		runtimeTraceProjMarkSemanticSpan,
		runtimeTraceProjMarkPeriodicSource,
	} {
		marks.mark(mark)
	}
	lines := runtimeTraceProjReaderLegendLines(marks, true, true)
	if len(lines) > 14 {
		t.Fatalf("reader legend grew beyond its bounded customer surface: %d lines\n%s", len(lines), strings.Join(lines, "\n"))
	}
	text := strings.Join(lines, "\n")
	for _, want := range []string{
		"主根因专指已证链上项目中单项可消除量最大的项目",
		"后二者都不能替代链上根因",
		"不同修复方向的收益不能直接相加",
		"不是墙钟时长",
		"不据此补造唤醒边",
		"帧因果尚未证明",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("reader legend lost required boundary %q:\n%s", want, text)
		}
	}
	for _, internal := range []string{"typed", "registry", "发射", "佩章", "席位", "板内", "判词文法"} {
		if strings.Contains(text, internal) {
			t.Fatalf("reader legend leaked internal term %q:\n%s", internal, text)
		}
	}
}

func TestTraceProjectionAuditCatalogRemainsLossless(t *testing.T) {
	marks := &runtimeTraceProjMarkSet{}
	marks.mark(runtimeTraceProjMarkElimDirectionSection)
	reader := strings.Join(runtimeTraceProjReaderLegendLines(marks, true, false), "\n")
	audit := strings.Join(runtimeTraceProjLegendGroupLines(marks, true), "\n")
	if strings.Contains(reader, "registry 属性轴") {
		t.Fatalf("reader face must hide implementation vocabulary:\n%s", reader)
	}
	if !strings.Contains(audit, "registry 属性轴") {
		t.Fatalf("audit catalog must remain lossless for structural verification:\n%s", audit)
	}
}

func TestTraceProjectionClusterPublishesReaderLegendNotAuditCatalog(t *testing.T) {
	blocks := runtimeTraceCausalProjectionCluster(revisit76IOProjection(), "zh", runtimeTraceProjUserFocus{})
	var lead string
	for _, block := range blocks {
		if block.ID == runtimeTraceCausalProjectionBlockIDBase {
			lead = block.Text
			break
		}
	}
	if lead == "" {
		t.Fatal("fixture must publish the trace causal projection lead")
	}
	if !strings.Contains(lead, "`唤醒`的子行唤醒父行") {
		t.Fatalf("production lead did not use the bounded reader legend:\n%s", lead)
	}
	for _, auditOnly := range []string{"registry 属性轴", "佩章行", "◎ 判词文法", "typed 时间包络"} {
		if strings.Contains(lead, auditOnly) {
			t.Fatalf("production lead leaked audit-catalog term %q:\n%s", auditOnly, lead)
		}
	}
}

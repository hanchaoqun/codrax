package preview

import (
	"strings"
	"testing"
)

func TestRenderMarkdownHTMLWrapsGeneratedTraceAuditChaptersLosslessly(t *testing.T) {
	markdown := []byte(`# 报告

## 因果投影明细(逐节点完整属性) — trace-a

导语 <anonymous>

### E1 VerifyClass

- 属性 A
- 属性 B

## 证据索引 — trace-a

- E1: trace-a:10-20

## 其他章节

不应在审计包装内
`)

	got, err := RenderMarkdownHTML(markdown)
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	if strings.Count(got, `<section class="trace-projection-detail">`) != 1 ||
		strings.Count(got, `<section class="trace-projection-evidence">`) != 1 {
		t.Fatalf("missing exact audit wrappers:\n%s", got)
	}
	for _, preserved := range []string{
		"导语 &lt;anonymous&gt;", "E1 VerifyClass", "属性 A", "trace-a:10-20", "不应在审计包装内",
	} {
		if !strings.Contains(got, preserved) {
			t.Fatalf("content %q lost during wrapping:\n%s", preserved, got)
		}
	}
	detailAt := strings.Index(got, `<section class="trace-projection-detail">`)
	detailEnd := detailAt + strings.Index(got[detailAt:], "</section>")
	evidenceAt := strings.Index(got, `<section class="trace-projection-evidence">`)
	evidenceEnd := evidenceAt + strings.Index(got[evidenceAt:], "</section>")
	otherAt := strings.Index(got, "其他章节")
	if !(detailAt >= 0 && detailEnd < evidenceAt && evidenceAt < evidenceEnd && evidenceEnd < otherAt) {
		t.Fatalf("wrapper boundaries reordered or captured the next chapter:\n%s", got)
	}
}

func TestRenderMarkdownHTMLTraceAuditHeadingGateIsExact(t *testing.T) {
	markdown := []byte(`## 因果投影明细

不是精确标题

## 证据索引补充

不是精确标题

## Evidence Index —

缺少 artifact
`)
	got, err := RenderMarkdownHTML(markdown)
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	if strings.Contains(got, "trace-projection-detail") || strings.Contains(got, "trace-projection-evidence") {
		t.Fatalf("near-match heading must not trigger compact layout:\n%s", got)
	}
}

func TestStandaloneTraceAuditCSSIsCompactResponsiveAndPrintable(t *testing.T) {
	page, err := RenderStandaloneMarkdownHTML("trace", []byte("## 因果投影明细(逐节点完整属性)\n\nnode\n"))
	if err != nil {
		t.Fatalf("RenderStandaloneMarkdownHTML: %v", err)
	}
	for _, want := range []string{
		"section.trace-projection-detail", "section.trace-projection-evidence",
		"column-count: 2", "font-size: .86rem", "column-count: 1", "font-size: 8pt",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("standalone report missing compact audit CSS %q", want)
		}
	}
}

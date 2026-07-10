package preview

import (
	"strings"
	"testing"
)

func TestRenderMarkdownHTMLWrapsGeneratedTraceAuditChaptersLosslessly(t *testing.T) {
	markdown := []byte(`# 报告

## 确定性优化点

可直接落地的优化项。

| 优化点 | 耗时 |
|---|---|
| VerifyClass | 4.6ms |

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
	if strings.Count(got, `<section class="trace-action-optimization">`) != 1 ||
		strings.Count(got, `<section class="trace-projection-detail">`) != 1 ||
		strings.Count(got, `<section class="trace-projection-evidence">`) != 1 {
		t.Fatalf("missing exact audit wrappers:\n%s", got)
	}
	for _, preserved := range []string{
		"可直接落地的优化项", "VerifyClass", "导语 &lt;anonymous&gt;", "E1 VerifyClass", "属性 A", "trace-a:10-20", "不应在审计包装内",
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

// 审计 #56 (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10): the evidence
// index is the report's LAST H2 in production — the wrapper must terminate at
// the index's OWN generated body (intro paragraph + bullet list) and never
// swallow the trailing disclosure surfaces: `**说明**：`/`**引用**：` bold
// paragraphs, `> ` caveat blockquotes, and the §29.9 阅读参考 appendix (aux
// custom node appended by the priority-500 auxFold transformer before this
// one runs at 600).
func TestRenderMarkdownHTMLEvidenceWrapperDoesNotSwallowTrailingDisclosures(t *testing.T) {
	markdown := []byte(`# 报告

树读法:

- 🎯 = 分析目标

## 证据索引 — trace-a

正文用 E1、E2 等编号引用证据;本索引给出位置与审计字段。

- E1: trace-a:10-20
- E2: trace-a:30-40

**说明**：部分查询结果按容量截断。

**引用**：trace-a 原始记录。

> 低覆盖披露:本窗证据覆盖不足。
`)
	got, err := RenderMarkdownHTML(markdown)
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	evidenceAt := strings.Index(got, `<section class="trace-projection-evidence">`)
	if evidenceAt < 0 {
		t.Fatalf("missing evidence wrapper:\n%s", got)
	}
	evidenceEnd := evidenceAt + strings.Index(got[evidenceAt:], "</section>")
	inner := got[evidenceAt:evidenceEnd]
	// The index's OWN body stays inside.
	for _, want := range []string{"证据索引", "本索引给出位置与审计字段", "trace-a:10-20", "trace-a:30-40"} {
		if !strings.Contains(inner, want) {
			t.Fatalf("evidence body %q must stay inside the wrapper:\n%s", want, inner)
		}
	}
	// Trailing disclosure surfaces stay DOCUMENT-LEVEL (never in the box).
	for _, disclosure := range []string{"部分查询结果按容量截断", "trace-a 原始记录", "低覆盖披露", "阅读参考"} {
		if !strings.Contains(got, disclosure) {
			t.Fatalf("disclosure %q missing from the render:\n%s", disclosure, got)
		}
		if strings.Contains(inner, disclosure) {
			t.Fatalf("#56: disclosure %q swallowed into the evidence audit box:\n%s", disclosure, inner)
		}
	}
}

// 复核 R2 (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10): when the
// evidence-index block is absent the DETAIL chapter is the report's last H2 —
// the wrapper must keep admitting its own unbounded stanza shape (bold
// heading paragraph + roster list pairs) while the trailing disclosure
// surfaces (caveat blockquote, `**说明**：`/`**引用**：` closed-set headings
// and their lists) stay document-level.
func TestRenderMarkdownHTMLDetailWrapperDoesNotSwallowTrailingDisclosures(t *testing.T) {
	markdown := []byte(`# 报告

## 因果投影明细(逐节点完整属性) — trace-a

每个节点一块,给出树和指标表中省略或压缩的全部属性。

**[E1] worker-200 · JIT compiling Foo**

- 层级: 链上L1
- 类型: jit_compile

**[E2] app-100 · sleep**

- 层级: 目标
- 类型: sleep_wait

**说明**：

- 部分查询结果按容量截断。

**引用**：

- trace-a 原始记录。

> 低覆盖披露:本窗证据覆盖不足。
`)
	got, err := RenderMarkdownHTML(markdown)
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	detailAt := strings.Index(got, `<section class="trace-projection-detail">`)
	if detailAt < 0 {
		t.Fatalf("missing detail wrapper:\n%s", got)
	}
	detailEnd := detailAt + strings.Index(got[detailAt:], "</section>")
	inner := got[detailAt:detailEnd]
	// The generator's own stanza pairs stay inside.
	for _, want := range []string{"每个节点一块", "[E1] worker-200 · JIT compiling Foo", "层级: 链上L1", "[E2] app-100 · sleep"} {
		if !strings.Contains(inner, want) {
			t.Fatalf("detail stanza %q must stay inside the wrapper:\n%s", want, inner)
		}
	}
	// The trailing disclosure surfaces stay DOCUMENT-LEVEL.
	for _, disclosure := range []string{"部分查询结果按容量截断", "trace-a 原始记录", "低覆盖披露"} {
		if !strings.Contains(got, disclosure) {
			t.Fatalf("disclosure %q missing from the render:\n%s", disclosure, got)
		}
		if strings.Contains(inner, disclosure) {
			t.Fatalf("R2: disclosure %q swallowed into the detail audit box:\n%s", disclosure, inner)
		}
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
		"section.trace-action-optimization", "section.trace-projection-detail", "section.trace-projection-evidence",
		"column-count: 2", "font-size: .86rem", "column-count: 1", "font-size: 8pt",
		"border-left-width: 4px", "color: var(--action-fg)",
		"section.trace-projection-detail > h3 { break-after: avoid-column; break-inside: avoid-column; }",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("standalone report missing compact audit CSS %q", want)
		}
	}
}

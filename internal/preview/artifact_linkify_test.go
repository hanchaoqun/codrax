package preview

import (
	"strings"
	"testing"
)

const artifactLinkifyCustomerName = "Other_trace_20260722222426@69326-2310.sys.systrace"

func TestRenderMarkdownHTMLKeepsTraceArtifactOutOfMailto(t *testing.T) {
	windowsPath := `D:\temp\` + artifactLinkifyCustomerName
	unixPath := "/tmp/" + artifactLinkifyCustomerName
	body := "转换工件 " + artifactLinkifyCustomerName + "。Windows " + windowsPath +
		"；Unix " + unixPath + "。普通邮箱 user@example.com，网址 https://example.com/report 保持链接。"
	html, err := RenderMarkdownHTML([]byte(body))
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	for _, artifact := range []string{artifactLinkifyCustomerName, windowsPath, unixPath} {
		if !strings.Contains(html, "<code>"+artifact+"</code>") {
			t.Fatalf("trace artifact %q was not rendered as literal inline code:\n%s", artifact, html)
		}
		if strings.Contains(html, "mailto:"+artifact) {
			t.Fatalf("trace artifact %q was still rendered as mailto:\n%s", artifact, html)
		}
	}
	if !strings.Contains(html, `href="mailto:user@example.com"`) {
		t.Fatalf("ordinary email linkify regressed:\n%s", html)
	}
	if !strings.Contains(html, `href="https://example.com/report"`) {
		t.Fatalf("ordinary URL linkify regressed:\n%s", html)
	}
}

func TestRenderMarkdownHTMLKeepsTraceArtifactOutOfMailtoInsideCJKPunctuation(t *testing.T) {
	// 与终端面同一客户形(no_window.txt):全角括号/引号/书名号直接包裹
	// artifact 名,名后紧跟全角闭合标点。
	for _, wrap := range []struct{ open, close string }{
		{"（", "）"},
		{"“", "”"},
		{"《", "》"},
		{"「", "」"},
	} {
		body := "分析结论基于 attached trace 运行时观测" + wrap.open +
			artifactLinkifyCustomerName + wrap.close + "，普通邮箱 user@example.com 保持链接。"
		html, err := RenderMarkdownHTML([]byte(body))
		if err != nil {
			t.Fatalf("RenderMarkdownHTML: %v", err)
		}
		if strings.Contains(html, "mailto:") && !strings.Contains(html, `href="mailto:user@example.com"`) {
			t.Fatalf("wrap %s…%s: unexpected mailto form:\n%s", wrap.open, wrap.close, html)
		}
		if strings.Contains(html, "mailto:trace_") {
			t.Fatalf("wrap %s…%s: trace artifact tail was rendered as mailto:\n%s", wrap.open, wrap.close, html)
		}
		if !strings.Contains(html, `href="mailto:user@example.com"`) {
			t.Fatalf("wrap %s…%s: ordinary email linkify regressed:\n%s", wrap.open, wrap.close, html)
		}
	}
}

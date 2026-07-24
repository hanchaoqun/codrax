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

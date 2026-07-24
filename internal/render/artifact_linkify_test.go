package render

import (
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

const terminalArtifactLinkifyCustomerName = "Other_trace_20260722222426@69326-2310.sys.systrace"

func TestTerminalMarkdownKeepsTraceArtifactOutOfMailto(t *testing.T) {
	windowsPath := `D:\temp\` + terminalArtifactLinkifyCustomerName
	unixPath := "/tmp/" + terminalArtifactLinkifyCustomerName
	artifacts := []string{terminalArtifactLinkifyCustomerName, windowsPath, unixPath}
	source := []byte("转换工件 " + artifacts[0] + "。Windows " + artifacts[1] +
		"；Unix " + artifacts[2] + "。普通邮箱 user@example.com 保持链接。")
	r := New(nil, true)
	doc := r.markdown.md.Parser().Parse(text.NewReader(source))
	codeArtifacts := map[string]bool{}
	linkArtifacts := map[string]bool{}
	var ordinaryEmailLink bool
	if err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.CodeSpan:
			label := string(n.Text(source))
			for _, artifact := range artifacts {
				if label == artifact {
					codeArtifacts[artifact] = true
				}
			}
		case *ast.AutoLink:
			label := string(n.Label(source))
			for _, artifact := range artifacts {
				if strings.Contains(label, artifact) {
					linkArtifacts[artifact] = true
				}
			}
			if label == "user@example.com" {
				ordinaryEmailLink = true
			}
		}
		return ast.WalkContinue, nil
	}); err != nil {
		t.Fatalf("walk terminal markdown AST: %v", err)
	}
	for _, artifact := range artifacts {
		if !codeArtifacts[artifact] || linkArtifacts[artifact] {
			t.Fatalf("artifact %q AST authority: code=%t autolink=%t", artifact, codeArtifacts[artifact], linkArtifacts[artifact])
		}
	}
	if !ordinaryEmailLink {
		t.Fatal("ordinary email no longer reaches terminal Linkify")
	}
	plain := stripAnsiEscapes(r.RenderMarkdown(string(source)))
	for _, artifact := range artifacts {
		if !strings.Contains(plain, artifact) {
			t.Fatalf("terminal output lost artifact %q: %q", artifact, plain)
		}
	}
}

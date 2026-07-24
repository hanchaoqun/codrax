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

func TestTerminalMarkdownKeepsTraceArtifactOutOfMailtoInsideCJKPunctuation(t *testing.T) {
	// no_window.txt 客户形:artifact 名被全角括号直接包裹(名前无空格,
	// Linkify 从 Other_ 的 _ 触发),名后紧跟 ）——修前 ）不在 separator/
	// trailing 表,token 识别失败,Linkify 铸 mailto:trace_...。
	for _, wrap := range []struct{ open, close string }{
		{"（", "）"},
		{"“", "”"},
		{"《", "》"},
		{"「", "」"},
	} {
		source := []byte("分析结论基于 attached trace 运行时观测" + wrap.open +
			terminalArtifactLinkifyCustomerName + wrap.close + "，普通邮箱 user@example.com 保持链接。")
		r := New(nil, true)
		doc := r.markdown.md.Parser().Parse(text.NewReader(source))
		var artifactMailto bool
		var artifactTailCode bool
		var ordinaryEmailLink bool
		if err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			switch n := node.(type) {
			case *ast.CodeSpan:
				if strings.HasSuffix(string(n.Text(source)), ".sys.systrace") {
					artifactTailCode = true
				}
			case *ast.AutoLink:
				label := string(n.Label(source))
				if strings.Contains(label, ".sys.systrace") {
					artifactMailto = true
				}
				if label == "user@example.com" {
					ordinaryEmailLink = true
				}
			}
			return ast.WalkContinue, nil
		}); err != nil {
			t.Fatalf("walk terminal markdown AST: %v", err)
		}
		if artifactMailto || !artifactTailCode {
			t.Fatalf("wrap %s…%s: artifact AST authority mailto=%t code=%t", wrap.open, wrap.close, artifactMailto, artifactTailCode)
		}
		if !ordinaryEmailLink {
			t.Fatalf("wrap %s…%s: ordinary email no longer reaches terminal Linkify", wrap.open, wrap.close)
		}
		// 显示形:CJK 标点紧贴时 Linkify/本 parser 都从 Other_ 的 _ 触发,
		// 尾段 trace_...systrace 成 code span、Other_ 前缀留 text(终端 code
		// span 两侧有排版空格,全名不再连续)——接受该外观,语义要求是
		// 零 mailto 且两段字符都逐字在场。
		plain := stripAnsiEscapes(r.RenderMarkdown(string(source)))
		if strings.Contains(plain, "mailto:trace_") {
			t.Fatalf("wrap %s…%s: terminal output still carries artifact mailto: %q", wrap.open, wrap.close, plain)
		}
		if !strings.Contains(plain, "trace_20260722222426@69326-2310.sys.systrace") ||
			!strings.Contains(plain, "Other_") {
			t.Fatalf("wrap %s…%s: terminal output lost artifact characters: %q", wrap.open, wrap.close, plain)
		}
	}
}

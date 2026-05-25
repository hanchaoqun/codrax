package outputdump

import (
	"strings"
	"testing"
)

func TestBuildBodyNormalizesAnswerMermaidFences(t *testing.T) {
	body := BuildBody(Args{
		Request: "show a diagram",
		Answer: strings.Join([]string{
			"```mermaid",
			"flowchart TD",
			`    ../A.md -->|success (measurement==true)| B[preStages\n(Conditional)]`,
			"```",
		}, "\n"),
	})
	if !strings.Contains(body, `codraxNode1["../A.md"] -->|"success (measurement==true)"| B["preStages\n(Conditional)"]`) {
		t.Fatalf("answer Mermaid fence was not normalized in output dump:\n%s", body)
	}
}

func TestBuildBodyPreservesQuestionMermaidSource(t *testing.T) {
	request := strings.Join([]string{
		"why does this fail?",
		"```mermaid",
		"flowchart TD",
		`    ../A.md -->|success (measurement==true)| B`,
		"```",
	}, "\n")
	body := BuildBody(Args{
		Request: request,
		Answer:  "plain answer",
	})
	if !strings.Contains(body, request) {
		t.Fatalf("request Mermaid source should remain verbatim:\n%s", body)
	}
}

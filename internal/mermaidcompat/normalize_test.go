package mermaidcompat

import (
	"strings"
	"testing"
)

func TestNormalizeSequenceStops_RewritesBareStop(t *testing.T) {
	in := strings.Join([]string{
		"sequenceDiagram",
		"    participant Explorer as Explorer",
		"    participant Runtime as Runtime",
		"    alt failed",
		"        Runtime-->>Explorer: Original Output",
		"        stop",
		"    end",
	}, "\n")
	got := NormalizeSequenceStops(in)
	if strings.Contains(got, "\n        stop\n") {
		t.Fatalf("bare stop survived:\n%s", got)
	}
	if !strings.Contains(got, "Note over Explorer,Runtime: stop") {
		t.Fatalf("stop note missing:\n%s", got)
	}
}

func TestNormalizeSequenceStops_LeavesNonSequenceBodiesAlone(t *testing.T) {
	in := "flowchart TD\n  stop[stop] --> B"
	if got := NormalizeSequenceStops(in); got != in {
		t.Fatalf("flowchart body changed:\n%s", got)
	}
}

func TestNormalizeSequenceParticipantMessagePrefixes_RewritesMessageLine(t *testing.T) {
	in := strings.Join([]string{
		"sequenceDiagram",
		"    participant build as buildAnalysisIR",
		"    participant normalizer->>resolver: resolver",
		"    actor resolver-->>build: done",
	}, "\n")
	got := NormalizeSequenceParticipantMessagePrefixes(in)
	if strings.Contains(got, "participant normalizer->>resolver") || strings.Contains(got, "actor resolver-->>build") {
		t.Fatalf("message prefix survived:\n%s", got)
	}
	if !strings.Contains(got, "    normalizer->>resolver: resolver") || !strings.Contains(got, "    resolver-->>build: done") {
		t.Fatalf("message lines not preserved:\n%s", got)
	}
	if !strings.Contains(got, "participant build as buildAnalysisIR") {
		t.Fatalf("valid participant declaration changed:\n%s", got)
	}
}

func TestNormalizeSequenceParticipantMessagePrefixes_LeavesDeclarationsAlone(t *testing.T) {
	in := strings.Join([]string{
		"sequenceDiagram",
		"    participant A as \"A->>B label\"",
		"    A->>B: call",
	}, "\n")
	if got := NormalizeSequenceParticipantMessagePrefixes(in); got != in {
		t.Fatalf("valid declaration changed:\n%s", got)
	}
}

func TestNormalizeFlowchartPipeLabels_QuotesParserSensitiveLabels(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    cmd["execCommand.Execute"] --> payload{"execCommandPayloadWithTypedOrigins"}`,
		`    payload --> store["StoreBlob"]`,
		`    store --> result["ToolResult (Summary + RawRef)"]`,
		`    result --> originLine{"execCommandTypedOriginLine"}`,
		`    originLine --> proof{"DeterministicCountProofInteger"}`,
		`    proof -->|success (measurement==true)| kv["kvBanner: origin=command_measurement, count"]`,
	}, "\n")
	got := NormalizeFlowchartPipeLabels(in)
	if !strings.Contains(got, `proof -->|"success (measurement==true)"| kv`) {
		t.Fatalf("edge label with parentheses was not quoted:\n%s", got)
	}
	if !strings.Contains(got, `payload{"execCommandPayloadWithTypedOrigins"}`) {
		t.Fatalf("node shape/label should be preserved:\n%s", got)
	}
}

func TestNormalizeFlowchartPipeLabels_LeavesSafeAndAlreadyQuotedLabelsAlone(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    A -->|safe label| B`,
		`    B -->|"already (quoted)"| C`,
	}, "\n")
	if got := NormalizeFlowchartPipeLabels(in); got != in {
		t.Fatalf("safe labels should be byte-preserved:\n%s", got)
	}
}

func TestNormalizeFlowchartUnsafeNodeIDs_AliasesPathLikeEndpoints(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		"    ../A.md --> packages/core/src/B.ts",
		`    packages/core/src/B.ts --> fileLine["pkg/c.go:12"]`,
	}, "\n")
	got := NormalizeFlowchartUnsafeNodeIDs(in)
	for _, want := range []string{
		`codraxNode1["../A.md"] --> codraxNode2["packages/core/src/B.ts"]`,
		`codraxNode2["packages/core/src/B.ts"] --> fileLine["pkg/c.go:12"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("path-like endpoint was not safely aliased; missing %q in:\n%s", want, got)
		}
	}
}

func TestNormalizeFlowchartUnsafeNodeIDs_PreservesExistingLabelsAndEdgeLabels(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    ../A.md["Readable A"] -->|success (measurement==true)| ./B.md{"Decision B"}`,
	}, "\n")
	got := NormalizeFlowchartUnsafeNodeIDs(in)
	if !strings.Contains(got, `codraxNode1["Readable A"] -->|success (measurement==true)| codraxNode2{"Decision B"}`) {
		t.Fatalf("unsafe IDs should alias while existing node/edge labels survive:\n%s", got)
	}
}

func TestNormalizeFlowchartUnsafeNodeIDs_RewritesDirectiveRefsWithoutLabels(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		"    ../A.md --> B",
		"    click ../A.md callback",
		"    style ../A.md fill:#fff",
	}, "\n")
	got := NormalizeFlowchartUnsafeNodeIDs(in)
	for _, want := range []string{
		`codraxNode1["../A.md"] --> B`,
		"click codraxNode1 callback",
		"style codraxNode1 fill:#fff",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("directive reference did not follow aliased unsafe node; missing %q in:\n%s", want, got)
		}
	}
}

func TestNormalizeFlowchartUnsafeNodeIDs_DoesNotTreatGraphPrefixNodeAsHeader(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		"    graph.node --> B",
	}, "\n")
	got := NormalizeFlowchartUnsafeNodeIDs(in)
	if !strings.Contains(got, `codraxNode1["graph.node"] --> B`) {
		t.Fatalf("graph-prefixed node ID should still be normalized:\n%s", got)
	}
}

func TestNormalizeSourceForMarkdown_NormalizesFlowchartSubgraphAndEdgeLabels(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		"  subgraph Explorer System",
		"    ../A.md -->|ok (verified)| B",
		"  end",
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if !strings.Contains(got, "subgraph Explorer_System_2 [Explorer System]") {
		t.Fatalf("bare subgraph title was not normalized:\n%s", got)
	}
	if !strings.Contains(got, `codraxNode1["../A.md"] -->|"ok (verified)"| B`) {
		t.Fatalf("parser-sensitive edge label was not normalized:\n%s", got)
	}
}

func TestMermaidKeywordRegistryCoversPreviewAndTerminalForms(t *testing.T) {
	if got := FirstKeywordIn("flowchart TD"); got != "flowchart" {
		t.Fatalf("flowchart keyword = %q", got)
	}
	if got := FirstKeywordIn("classDiagram"); got != "classDiagram" {
		t.Fatalf("classDiagram keyword = %q", got)
	}
	if IsSupportedKeyword("classDiagram") {
		t.Fatal("classDiagram is valid Mermaid but must remain outside terminal supported subset")
	}
	if !IsSupportedKeyword("sequenceDiagram") {
		t.Fatal("sequenceDiagram should remain terminal-renderable")
	}
	if !InfoLineStartsWithKeyword("classDiagram") {
		t.Fatal("direct classDiagram info string should be recognized")
	}
	directive, kw := InfoLineDirective("mermaid flowchart LR")
	if directive != "flowchart LR" || kw != "flowchart" {
		t.Fatalf("mermaid info directive = (%q,%q)", directive, kw)
	}
	if !LooksLikeBody("\n  flowchart LR\n    A --> B\n") {
		t.Fatal("body whose first non-empty line is flowchart should be recognized")
	}
	if LooksLikeBody("A --> B") {
		t.Fatal("raw arrows without a Mermaid directive must not be classified as Mermaid")
	}
}

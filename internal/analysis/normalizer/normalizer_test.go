package normalizer

import (
	"fmt"
	"strings"
	"testing"
)

type evalCase struct {
	lang      string
	request   string
	expected  []string
	notWanted []string
}

func TestBuildTermGraphMultilingualRules(t *testing.T) {
	cases := []evalCase{
		{
			lang:    "zh",
			request: "请解释 AgentAnalyzer 如何注册到 orchestrator.stage_map，并检查 `go test ./internal/agent/...`",
			expected: []string{
				"AgentAnalyzer", "orchestrator.stage_map", "go test ./internal/agent/...", "注册",
			},
			notWanted: []string{"the", "and"},
		},
		{
			lang:    "en",
			request: "Find how BaseAgent executes SubExplorer and map pipeline.max_steps in config/codrax.yaml",
			expected: []string{
				"BaseAgent", "SubExplorer", "pipeline.max_steps", "find",
			},
			notWanted: []string{"the", "with"},
		},
		{
			lang:    "ja",
			request: "Analyzer の Alias Graph を作成し、`make test` の結果を確認して設定キー pipeline.enable_verify を説明して",
			expected: []string{
				"Analyzer", "make test", "pipeline.enable_verify", "設定キー",
			},
			notWanted: []string{"request"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			graph := BuildTermGraph(tc.request)
			if len(graph.Nodes) == 0 {
				t.Fatalf("expected extracted nodes")
			}
			tokens := RetrievalTokens(graph)
			joined := strings.Join(tokens, "\n")
			for _, want := range tc.expected {
				if !strings.Contains(joined, strings.ToLower(want)) && !strings.Contains(joined, want) {
					t.Fatalf("expected token %q in %v", want, tokens)
				}
			}
			for _, bad := range tc.notWanted {
				for _, got := range tokens {
					if strings.EqualFold(got, bad) {
						t.Fatalf("unexpected noisy token %q in %v", bad, tokens)
					}
				}
			}
		})
	}
}

func TestTermPrecisionRecallReport(t *testing.T) {
	cases := []evalCase{
		{lang: "zh", request: "统计 RegisterDefaults 中 SubExplorer 的注册入口", expected: []string{"RegisterDefaults", "SubExplorer", "注册"}},
		{lang: "en", request: "Trace ExecCommand and explain pipeline.max_steps", expected: []string{"ExecCommand", "pipeline.max_steps", "trace"}},
		{lang: "ja", request: "Alias Graph で Analyzer と pipeline.enable_verify の関係を確認", expected: []string{"Analyzer", "pipeline.enable_verify", "関係"}},
	}

	type score struct{ p, r float64 }
	scores := map[string]score{}
	for _, tc := range cases {
		graph := BuildTermGraph(tc.request)
		set := map[string]bool{}
		for _, token := range RetrievalTokens(graph) {
			set[strings.ToLower(token)] = true
		}
		tp := 0
		for _, want := range tc.expected {
			if set[strings.ToLower(want)] {
				tp++
			}
		}
		precision := float64(tp) / float64(len(set))
		recall := float64(tp) / float64(len(tc.expected))
		scores[tc.lang] = score{p: precision, r: recall}
		if recall < 0.66 {
			t.Fatalf("%s recall too low: %.2f", tc.lang, recall)
		}
	}

	for lang, s := range scores {
		t.Logf("term precision/recall [%s]: precision=%.2f recall=%.2f", lang, s.p, s.r)
	}
	if len(scores) != 3 {
		t.Fatal(fmt.Errorf("expected 3 language reports"))
	}
}

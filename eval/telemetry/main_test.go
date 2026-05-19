package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectParsesAnalyzerAndFinalizerTelemetry(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "codrax.log")
	body := strings.Join([]string{
		`2026-05-19T10:00:00.000 INFO [analyzer] entity provenance summary: total=4 scope=1 symbol=1 file=1 prescan_anchor=0 inferred_concept=1 use_for_search=3 use_for_shape=3 mean_noise=0.17`,
		`2026-05-19T10:00:00.010 INFO [emit_analysis] blocklist_shadow: dropped=2 would_search=0 would_shape=0 inferred_concept=2 sample=Agent, Handler`,
		`2026-05-19T10:00:00.020 ERROR [analyzer-v3] quality gate HARD failure: subtopic_coherence: unresolved asymmetric sub-topic`,
		`2026-05-19T10:00:00.030 INFO [render]   ⟳ 1/4 模型响应出错,正在重新理解问题`,
		`2026-05-19T10:00:00.040 DEBUG [diag analyzer] iter=0 phase=toolresult TOOLRESULT emit_analysis ok=false len=32: emit_analysis rejected: keywords below hard floor got=1 want≥3`,
		`2026-05-19T10:00:00.050 DEBUG [diag finalizer] iter=0 phase=toolresult TOOLRESULT emit_answer_document ok=false len=99: bad citation`,
		`2026-05-19T10:00:00.055 DEBUG [diag finalizer] phase=answer_contract_check section=v2_block_oracles done elapsed=2ms violations=3`,
		`2026-05-19T10:00:00.060 INFO [render]   ⟳ 4/4 答案待完善，正在重写`,
		`2026-05-19T10:00:00.070 INFO [orchestrator] repair_plan: primary=finalizer clusters=1 kinds=[diagram_edge_endpoint_hallucinated citation_ref_missing] target=finalizer_only`,
		`2026-05-19T10:00:00.080 WARN [diag] sanitized invalid tool-call arguments for history tool=emit_answer_document id=x len=10`,
		`2026-05-19T10:00:00.090 WARN [agent] tool "emit_answer_document" params auto-repaired (LLM-corrupted JSON: structural repair)`,
		`2026-05-19T10:00:00.100 ERROR [llm] chat failed: context deadline exceeded`,
		``,
	}, "\n")
	if err := os.WriteFile(logPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	rep, err := collect([]string{dir})
	if err != nil {
		t.Fatalf("collect returned error: %v", err)
	}
	if rep.Files != 1 || rep.Lines == 0 {
		t.Fatalf("file/line counters wrong: %+v", rep)
	}
	if rep.Analyzer.ProvenanceEvents != 1 || rep.Analyzer.Provenance.Total != 4 {
		t.Fatalf("provenance not parsed: %+v", rep.Analyzer.Provenance)
	}
	if rep.Analyzer.BlocklistEvents != 1 || rep.Analyzer.BlocklistShadow.Dropped != 2 {
		t.Fatalf("blocklist shadow not parsed: %+v", rep.Analyzer.BlocklistShadow)
	}
	if rep.Analyzer.BlocklistShadow.WouldSearch != 0 || rep.Analyzer.BlocklistShadow.WouldShape != 0 {
		t.Fatalf("shadow would-search/shape should remain zero: %+v", rep.Analyzer.BlocklistShadow)
	}
	if rep.Analyzer.RetryRenders != 1 || rep.Analyzer.ToolRejects != 1 || rep.Analyzer.HardGateFailures != 1 {
		t.Fatalf("analyzer retry/reject counters wrong: %+v", rep.Analyzer)
	}
	if rep.Finalizer.DocumentRejects != 1 || rep.Finalizer.RewriteRenders != 1 || rep.Finalizer.RepairPlans != 1 {
		t.Fatalf("finalizer counters wrong: %+v", rep.Finalizer)
	}
	if rep.Finalizer.ContractViolations != 3 || rep.Finalizer.ContractViolationBySection["v2_block_oracles"] != 3 {
		t.Fatalf("contract violations not parsed: %+v", rep.Finalizer)
	}
	if rep.Finalizer.RepairKinds["diagram_edge_endpoint_hallucinated"] != 1 ||
		rep.Finalizer.RepairKinds["citation_ref_missing"] != 1 {
		t.Fatalf("repair kinds not parsed: %+v", rep.Finalizer.RepairKinds)
	}
	if rep.ToolParamCompat.SanitizedInvalidArgs != 1 ||
		rep.ToolParamCompat.AutoRepairedParams != 1 ||
		rep.ToolParamCompat.Tools["emit_answer_document"] != 1 {
		t.Fatalf("tool param compat counters wrong: %+v", rep.ToolParamCompat)
	}
	if rep.LLM.Errors != 2 || rep.LLM.Timeouts != 1 {
		t.Fatalf("llm counters wrong: %+v", rep.LLM)
	}
	if len(rep.FilesByRetryScore) != 1 || rep.FilesByRetryScore[0].BlocklistDropped != 2 {
		t.Fatalf("hot-log snapshot wrong: %+v", rep.FilesByRetryScore)
	}
}

func TestWriteMarkdownIncludesDecisionSignals(t *testing.T) {
	rep := report{
		Files: 1,
		Lines: 12,
		Analyzer: analyzerSummary{
			ProvenanceEvents: 1,
			Provenance: provenanceSum{
				Total:           3,
				InferredConcept: 1,
				UseForSearch:    2,
				UseForShape:     2,
				MeanNoise:       0.23,
			},
			BlocklistEvents: 1,
			BlocklistShadow: blocklistSum{
				Dropped:         1,
				InferredConcept: 1,
				Samples:         map[string]int{"Agent": 1},
			},
		},
		Finalizer: finalizerSummary{
			ToolRejects:                1,
			ContractViolations:         2,
			ContractViolationBySection: map[string]int{"support_plan": 2},
			RepairKinds:                map[string]int{"diagram_edge_endpoint_hallucinated": 1},
		},
	}
	var b bytes.Buffer
	writeMarkdown(&b, rep, 5)
	out := b.String()
	for _, want := range []string{
		"# Codrax Telemetry Report",
		"blocklist shadow: events=1 dropped=1 would_search=0 would_shape=0",
		"diagram_edge_endpoint_hallucinated",
		"support_plan",
		"Agent",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown missing %q:\n%s", want, out)
		}
	}
}

func TestCollectAcceptsDirectTxtFragments(t *testing.T) {
	dir := t.TempDir()
	fragment := filepath.Join(dir, "customer.txt")
	if err := os.WriteFile(fragment, []byte(`[emit_analysis] blocklist_shadow: dropped=1 would_search=0 would_shape=0 inferred_concept=1 sample=Config`), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := collect([]string{fragment})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 1 || rep.Analyzer.BlocklistShadow.Dropped != 1 {
		t.Fatalf("direct txt fragment not collected: %+v", rep)
	}
}

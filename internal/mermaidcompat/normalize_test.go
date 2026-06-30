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

func TestSourceRepairHash_UsesBeforeAndAfter(t *testing.T) {
	a := sourceRepairHash("before", "after")
	if a == "" {
		t.Fatal("hash is empty")
	}
	if got := sourceRepairHash("before", "after"); got != a {
		t.Fatalf("hash must be deterministic: got %q want %q", got, a)
	}
	if got := sourceRepairHash("before", "after!"); got == a {
		t.Fatalf("hash must include after source")
	}
	if got := sourceRepairHash("before!", "after"); got == a {
		t.Fatalf("hash must include before source")
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

func TestNormalizeSourceForMarkdown_MergesSplitPipeEdgeLabelsBeforeAliasing(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    bpf_map_update_value -->|calls @ :261|:297| ops_dispatch`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if strings.Contains(got, "codraxNode") {
		t.Fatalf("split edge label fragment was aliased as a node:\n%s", got)
	}
	if !strings.Contains(got, `bpf_map_update_value -->|"calls @ :261 / :297"| ops_dispatch`) {
		t.Fatalf("split edge label was not merged into one quoted label:\n%s", got)
	}
}

func TestNormalizeSourceForMarkdown_DropsStandaloneHiddenMarkerBeforeAliasing(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    syscall_entry["SYSCALL_DEFINE3(bpf, …)"]`,
		`    syscall_entry @[hidden]`,
		`    syscall_entry --> __sys_bpf`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if strings.Contains(got, "codraxNode") || strings.Contains(got, "@[hidden]") {
		t.Fatalf("hidden marker leaked after normalization:\n%s", got)
	}
	if !strings.Contains(got, `syscall_entry --> __sys_bpf`) {
		t.Fatalf("edge should be preserved:\n%s", got)
	}
}

func TestNormalizeSourceForMarkdown_DropsGeneratedHiddenMarkerLine(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    A["visible"]`,
		`    A codraxNode1[hidden]`,
		`    A --> B["hidden label is visible text"]`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if strings.Contains(got, "codraxNode1[hidden]") {
		t.Fatalf("generated hidden marker line survived:\n%s", got)
	}
	if !strings.Contains(got, `B["hidden label is visible text"]`) {
		t.Fatalf("legitimate hidden label text should be preserved:\n%s", got)
	}
}

func TestNormalizeSourceForMarkdown_DoesNotMergeChainedPipeLabels(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    A -->|first| B -->|second| C`,
		`    A -->|safe| B["label|with pipe"]`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	for _, want := range []string{
		`A -->|first| B -->|second| C`,
		`A -->|safe| B["label|with pipe"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("valid flowchart line changed; missing %q in:\n%s", want, got)
		}
	}
}

func TestNormalizeFlowchartNodeLabels_QuotesParserSensitiveUnquotedLabels(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    SB1["StageLogTriage → AgentLogTriager"]`,
		`    StageBindings -->|绑定| SB1`,
		`    PS[preStages: LogTriage, PerfTriage\n(Conditional)]`,
		`    PT[pipelineTopology: Analyze → Explore → Extract → Finalize]`,
		`    A1["AgentAnalyzer\n(analysis-skill)"]`,
		`    D{branch (verified)}`,
		`    C[(Database (cache))]`,
		`    S[[subroutine (portable)]]`,
		`    H{{hexagon (portable)}}`,
	}, "\n")
	got := NormalizeFlowchartNodeLabels(in)
	for _, want := range []string{
		`SB1["StageLogTriage → AgentLogTriager"]`,
		`StageBindings -->|绑定| SB1`,
		`PS["preStages: LogTriage, PerfTriage<br/>(Conditional)"]`,
		`PT[pipelineTopology: Analyze → Explore → Extract → Finalize]`,
		`A1["AgentAnalyzer<br/>(analysis-skill)"]`,
		`D{"branch (verified)"}`,
		`C[("Database (cache)")]`,
		`S[["subroutine (portable)"]]`,
		`H{{"hexagon (portable)"}}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("node label normalization missing %q in:\n%s", want, got)
		}
	}
}

func TestNormalizeSourceForMarkdown_RepairsBracketedTraceNodeLabels(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    A[com.tencent.mm-36379<br/>2942.124416-260.210<br/>135.8ms] -->|wakeup 94μs| B[[GT]codraxNode1>prio=20]`,
		`    A -->|wakeup 135.8ms| C[[GT]codraxNode2>prio=20]`,
		`    B -->|wakeup| D[[GT]codraxNode3>prio=10]`,
		`    C -->|wakeup| E[wc_srvinit_7-37014<br/>prio=22]`,
		`    D -->|wakeup| F[[GT]codraxNode4>prio=5]`,
		`    G[阻塞原因: fscache_page_wait_o<br/>I/O等待+锁竞争] -.-> A`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	for _, bad := range []string{
		`B[[GT]codraxNode1>prio=20]`,
		`C[[GT]codraxNode2>prio=20]`,
		`D[[GT]codraxNode3>prio=10]`,
		`F[[GT]codraxNode4>prio=5]`,
	} {
		if strings.Contains(got, bad) {
			t.Fatalf("malformed bracketed trace node label survived %q in:\n%s", bad, got)
		}
	}
	for _, want := range []string{
		`B["[GT]codraxNode1>prio=20"]`,
		`C["[GT]codraxNode2>prio=20"]`,
		`D["[GT]codraxNode3>prio=10"]`,
		`F["[GT]codraxNode4>prio=5"]`,
		`A[com.tencent.mm-36379<br/>2942.124416-260.210<br/>135.8ms] -->|wakeup 94μs| B`,
		`G[阻塞原因: fscache_page_wait_o<br/>I/O等待+锁竞争] -.-> A`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("repaired trace diagram missing %q in:\n%s", want, got)
		}
	}
}

func TestNormalizeSourceForMarkdown_PreservesValidSubroutineLabels(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    A --> S[[valid subroutine]]`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if !strings.Contains(got, `S[[valid subroutine]]`) {
		t.Fatalf("valid subroutine label should remain a subroutine shape:\n%s", got)
	}
}

func TestNormalizeSourceForMarkdown_UsesUnifiedLabelQuotingPolicy(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    subgraph Runtime [GT] Layer`,
		`      A[stage|slot] -->|ready (queue[0])| B{ok?}`,
		`      B --> C["already (quoted)"]`,
		`    end`,
		`    subgraph AlreadyValid[Already Valid]`,
		`      X --> Y`,
		`    end`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	for _, want := range []string{
		`subgraph Runtime_GT_Layer_2 ["Runtime [GT] Layer"]`,
		`A["stage|slot"] -->|"ready (queue[0])"| B{ok?}`,
		`B --> C["already (quoted)"]`,
		`subgraph AlreadyValid[Already Valid]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("unified label quoting missing %q in:\n%s", want, got)
		}
	}
}

func TestNormalizeSourceForMarkdown_FlowchartQuotedLabelNewlinesStayInsideNode(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    io_issue_defs["io_issue_defs[]`,
		`io_uring/opdef.c:54"] --> io_send["io_send`,
		`io_uring/net.c:646"]`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	for _, bad := range []string{"codraxNode"} {
		if strings.Contains(got, bad) {
			t.Fatalf("quoted multiline labels should not be split into alias nodes; found %q in:\n%s", bad, got)
		}
	}
	for _, want := range []string{
		`io_issue_defs["io_issue_defs[]<br/>io_uring/opdef.c:54"] --> io_send["io_send<br/>io_uring/net.c:646"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("quoted label newline repair missing %q in:\n%s", want, got)
		}
	}
}

func TestNormalizeSourceForMarkdown_SplitsQuotedFlowchartEdgeFragments(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    keymaster["keymaster@3.0-s-8595<br/>(prio=20/CFS)"] -->|"sched_wakeup<br/>5569.394008 / 36.562ms"| binder85_2["binder:85_2-8593<br/>(prio=20/CFS)"]codraxNode1>    "binder85_2 -->|&quot;sched_wakeup<br/>5569.395xxx / Coordinator&quot;| CookieMonsterBa[&quot;CookieMonsterBa-56265<br/>(prio=20/CFS)&quot;"]`,
		`    CookieMonsterBa -->|"sched_wakeup<br/>延迟累积 / Coordinator"| CookieMonsterCl["CookieMonsterCl-56264<br/>(prio=20/CFS)"]codraxNode1>    "CookieMonsterCl -->|&quot;sched_wakeup<br/>延迟累积 / 15ms&quot;| mainRT[&quot;android.haitong-56023<br/>(prio=53/ohos_rt)&quot;"]`,
		`    DefaultDispatch["DefaultDispatch-56273<br/>(prio=20/CFS)"] -->|"&quot;sched_wakeup<br/>5569.507993<br/>延迟43.726ms"| codraxNode2>    "Thread10[&quot;Thread-10-56284<br/>(prio=20/CFS)&quot;"] -->|"&quot;sched_wakeup<br/>5570.040054<br/>延迟15.137ms"| mainRT`,
		`    style mainRT fill:#ffcccc`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	for _, bad := range []string{"codraxNode1>", "codraxNode2>", "&quot;", `|""`} {
		if strings.Contains(got, bad) {
			t.Fatalf("quoted edge fragment repair leaked %q in:\n%s", bad, got)
		}
	}
	for _, want := range []string{
		`keymaster["keymaster@3.0-s-8595<br/>(prio=20/CFS)"] -->|"sched_wakeup<br/>5569.394008 / 36.562ms"| binder85_2["binder:85_2-8593<br/>(prio=20/CFS)"]`,
		`binder85_2 -->|"sched_wakeup<br/>5569.395xxx / Coordinator"| CookieMonsterBa["CookieMonsterBa-56265<br/>(prio=20/CFS)"]`,
		`CookieMonsterCl -->|"sched_wakeup<br/>延迟累积 / 15ms"| mainRT["android.haitong-56023<br/>(prio=53/ohos_rt)"]`,
		`DefaultDispatch["DefaultDispatch-56273<br/>(prio=20/CFS)"] -->|"sched_wakeup<br/>5569.507993<br/>延迟43.726ms"| Thread10["Thread-10-56284<br/>(prio=20/CFS)"]`,
		`Thread10["Thread-10-56284<br/>(prio=20/CFS)"] -->|"sched_wakeup<br/>5570.040054<br/>延迟15.137ms"| mainRT`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("quoted edge fragment repair missing %q in:\n%s", want, got)
		}
	}
}

func TestNormalizeSourceForMarkdown_DoesNotSplitQuotedArrowInsideLabels(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    A["text says B --> C"] -->|"quoted label with X --> Y"| B["done"]`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if !strings.Contains(got, `A["text says B --> C"] -->|"quoted label with X --> Y"| B["done"]`) {
		t.Fatalf("quoted arrow inside labels should remain one edge:\n%s", got)
	}
}

func TestNormalizeSourceForMarkdown_MergesSplitUnquotedNodeLabelsBeforeAliasing(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    A[read pipeline`,
		`stage boundary] --> B`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if strings.Contains(got, "codraxNode") {
		t.Fatalf("split node label continuation was aliased as a node:\n%s", got)
	}
	if !strings.Contains(got, `A[read pipeline<br/>stage boundary] --> B`) {
		t.Fatalf("split node label was not folded into one node statement:\n%s", got)
	}
}

func TestNormalizeSourceForMarkdown_MergesGeneratedAliasContinuationIntoOpenShape(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    Registry[(Registry`,
		`codraxNode1["&quot;管理所有Agent&quot;])"]`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if strings.Contains(got, "codraxNode") {
		t.Fatalf("generated alias continuation leaked into normalized source:\n%s", got)
	}
	if strings.Contains(got, "Registry[(Registry\n") {
		t.Fatalf("open shape label remained split across physical lines:\n%s", got)
	}
	if !strings.Contains(got, `Registry[(Registry<br/>&quot;管理所有Agent&quot;)]`) {
		t.Fatalf("generated alias label was not folded back into the open cylinder shape:\n%s", got)
	}
}

func TestNormalizeSourceForMarkdown_ConvertsLiteralFlowchartLabelNewlineEscapes(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    A["line one\nline two"] --> B[plain\nlabel]`,
		`    P["C:\new\file.txt"] --> Q["C:\\new\\escaped.txt"]`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	for _, want := range []string{
		`A["line one<br/>line two"]`,
		`B["plain<br/>label"]`,
		`P["C:\new\file.txt"]`,
		`Q["C:\\new\\escaped.txt"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("literal label newline escape was not converted to browser-stable break %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `line one\n`) || strings.Contains(got, `plain\n`) {
		t.Fatalf("visible flowchart label newline escape should not leak as literal text:\n%s", got)
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

func TestNormalizeFlowchartDanglingPunctuation_RemovesTrailingComma(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    A["log_triager (log_triage)"] --> B["perf_triager (perf_triage)"]`,
		`    B --> C["analyzer (analyze)"],`,
		`    C["label, with comma"]`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if strings.Contains(got, `B --> C["analyzer (analyze)"],`) {
		t.Fatalf("dangling edge comma survived:\n%s", got)
	}
	if !strings.Contains(got, `B --> C["analyzer (analyze)"]`) {
		t.Fatalf("edge did not survive comma repair:\n%s", got)
	}
	if !strings.Contains(got, `C["label, with comma"]`) {
		t.Fatalf("label comma should be preserved:\n%s", got)
	}
}

func TestNormalizeMarkdownMermaidFences_NormalizesDanglingComma(t *testing.T) {
	in := strings.Join([]string{
		"before",
		"```mermaid",
		"flowchart TD",
		`    I["coder (apply)"] --> J["verifier (verify)"],`,
		"```",
		"after",
	}, "\n")
	got := NormalizeMarkdownMermaidFences(in)
	if strings.Contains(got, `J["verifier (verify)"],`) {
		t.Fatalf("persisted markdown retained dangling Mermaid comma:\n%s", got)
	}
	if !strings.Contains(got, `I["coder (apply)"] --> J["verifier (verify)"]`) {
		t.Fatalf("persisted markdown lost repaired edge:\n%s", got)
	}
}

func TestNormalizeSourceForMarkdown_NormalizesFlowchartSubgraphAndEdgeLabels(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		"  subgraph Explorer System (Read Mode)",
		"    ../A.md -->|ok (verified)| B",
		`    B[preStages: LogTriage, PerfTriage\n(Conditional)]`,
		"  end",
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if !strings.Contains(got, `subgraph Explorer_System_Read_Mode_2 ["Explorer System (Read Mode)"]`) {
		t.Fatalf("bare subgraph title was not normalized:\n%s", got)
	}
	if !strings.Contains(got, `codraxNode1["../A.md"] -->|"ok (verified)"| B`) {
		t.Fatalf("parser-sensitive edge label was not normalized:\n%s", got)
	}
	if !strings.Contains(got, `B["preStages: LogTriage, PerfTriage<br/>(Conditional)"]`) {
		t.Fatalf("parser-sensitive node label was not normalized:\n%s", got)
	}
}

func TestNormalizeSourceForMarkdown_RepairsMismatchedDecisionClosersBeforeAliasing(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    F1["xfs_trans_alloc_icreate()"] --> G1{"分配事务")`,
		`    G1 --> H1{"ENOSPC?"]`,
		`    H1 -->|是| I1["xfs_flush_inodes(mp)"]`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	for _, bad := range []string{
		`G1{"分配事务")`,
		`H1{"ENOSPC?"]`,
		`G1{codraxNode`,
		`H1{codraxNode`,
	} {
		if strings.Contains(got, bad) {
			t.Fatalf("mismatched shape closer or aliasing artifact survived %q in:\n%s", bad, got)
		}
	}
	for _, want := range []string{
		`G1{"分配事务"}`,
		`H1{"ENOSPC?"}`,
		`H1 -->|是| I1["xfs_flush_inodes(mp)"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("repaired Mermaid source missing %q in:\n%s", want, got)
		}
	}
}

func TestNormalizeMarkdownMermaidFences_RepairsMismatchedDecisionClosers(t *testing.T) {
	in := strings.Join([]string{
		"before",
		"```mermaid",
		"flowchart TD",
		`    G1 --> H1{"ENOSPC?"]`,
		"```",
		"after",
	}, "\n")
	got := NormalizeMarkdownMermaidFences(in)
	if strings.Contains(got, `H1{"ENOSPC?"]`) || strings.Contains(got, "codraxNode") {
		t.Fatalf("persisted markdown retained malformed decision node:\n%s", got)
	}
	if !strings.Contains(got, `H1{"ENOSPC?"}`) {
		t.Fatalf("persisted markdown missing repaired decision node:\n%s", got)
	}
}

func TestNormalizeMarkdownMermaidFences_NormalizesPersistedMarkdownSource(t *testing.T) {
	in := strings.Join([]string{
		"before",
		"```mermaid",
		"flowchart TD",
		`    ../A.md -->|success (measurement==true)| B[preStages\n(Conditional)]`,
		"```",
		"after",
	}, "\n")
	got := NormalizeMarkdownMermaidFences(in)
	for _, want := range []string{
		"```mermaid",
		`codraxNode1["../A.md"] -->|"success (measurement==true)"| B["preStages<br/>(Conditional)"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("persisted markdown Mermaid normalization missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "../A.md -->|success (measurement==true)|") {
		t.Fatalf("unsafe raw Mermaid source survived:\n%s", got)
	}
}

func TestNormalizeMarkdownMermaidFences_ConvertsDirectInfoDirective(t *testing.T) {
	in := strings.Join([]string{
		"```flowchart TD",
		"A --> B",
		"```",
	}, "\n")
	got := NormalizeMarkdownMermaidFences(in)
	if !strings.Contains(got, "```mermaid\nflowchart TD\nA --> B\n```") {
		t.Fatalf("direct Mermaid info directive not converted:\n%s", got)
	}
}

func TestNormalizeMarkdownMermaidFences_LeavesNonMermaidCodeAlone(t *testing.T) {
	in := "```go\nfunc main() {}\n```"
	if got := NormalizeMarkdownMermaidFences(in); got != in {
		t.Fatalf("non-Mermaid fence changed:\n%s", got)
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

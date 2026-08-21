package mermaidcompat

import (
	"reflect"
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

func TestParseEdges_FlowchartPreservesColonQualifiedCallableLabels(t *testing.T) {
	tests := []struct {
		language string
		caller   string
		callee   string
	}{
		{"rust", "service::handle", "repo::count"},
		{"cpp", "Service::handle", "Repository::count"},
		{"ruby", "Service::handle", "Repository::count"},
		{"cangjie", "clinic::VisitService::schedule", "clinic::VisitRepository::countOpenVisits"},
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			body := "flowchart TD\n  A[\"" + tc.caller + "\"] --> B[\"" + tc.callee + "\"]\n"
			edges := ParseEdges(body)
			if len(edges) != 1 || edges[0].From != "A" || edges[0].To != "B" ||
				edges[0].FromLabel != tc.caller || edges[0].ToLabel != tc.callee {
				t.Fatalf("%s qualified call edge was truncated at colon: %+v", tc.language, edges)
			}
		})
	}
}

func TestParseSubgraphsPreservesParentAndDirectMembers(t *testing.T) {
	body := `flowchart TD
  subgraph owner_group["BusContext"]
    state["Mutable<br/>MutableState"]
    subgraph nested["Nested"]
      leaf["Leaf"]
    end
  end
  outside["Outside"]`
	got := ParseSubgraphs(body)
	if len(got) != 2 {
		t.Fatalf("subgraphs=%+v", got)
	}
	if got[0].Ident != "owner_group" || got[0].Label != "BusContext" || got[0].ParentIndex != -1 ||
		len(got[0].Nodes) != 1 || got[0].Nodes[0].Ident != "state" {
		t.Fatalf("owner group=%+v", got[0])
	}
	if got[1].Ident != "nested" || got[1].Label != "Nested" || got[1].ParentIndex != 0 ||
		len(got[1].Nodes) != 1 || got[1].Nodes[0].Ident != "leaf" {
		t.Fatalf("nested group=%+v", got[1])
	}
}

func TestNodeDeclarationsAllRecognizesStandaloneBareFlowNodeWithoutMintingStatements(t *testing.T) {
	for _, line := range []string{"analyzer", "  Explorer_2  ", "finalizer;"} {
		got := NodeDeclarationsAll(line)
		if len(got) != 1 || got[0].Ident == "" || got[0].Label != got[0].Ident {
			t.Fatalf("line=%q declarations=%+v, want one bare node", line, got)
		}
	}
	for _, line := range []string{
		"flowchart", "graph", "sequenceDiagram", "end", "subgraph", "direction",
		"classDef", "class", "style", "click", "linkStyle", "A --> B", "A B", "../A.md",
	} {
		if got := NodeDeclarationsAll(line); len(got) != 0 {
			t.Fatalf("statement/nonportable line=%q minted node declarations=%+v", line, got)
		}
	}
}

func TestParseEdges_FlowchartDoesNotInventEdgesFromCodeInsideNodeLabels(t *testing.T) {
	body := strings.Join([]string{
		"flowchart TD",
		`  W["sink_->write<br/>(src/logger.cpp:36)"]`,
		`  F["sink_->flush\n(src/logger.cpp:38)"]`,
		`  L["Logger::log"] --> W`,
	}, "\n")
	edges := ParseEdges(body)
	if len(edges) != 1 || edges[0].From != "L" || edges[0].To != "W" {
		t.Fatalf("code syntax inside node labels minted a diagram edge: %+v", edges)
	}
}

func TestParseEdges_FlowchartIgnoresArrowBytesInsideEndpointLabels(t *testing.T) {
	body := `flowchart TD
  A["holder->run"] --> B["target::run"]`
	edges := ParseEdges(body)
	if len(edges) != 1 || edges[0].From != "A" || edges[0].To != "B" ||
		edges[0].FromLabel != "holder->run" || edges[0].ToLabel != "target::run" {
		t.Fatalf("endpoint label arrow bytes replaced the actual diagram edge: %+v", edges)
	}
}

func TestParseEdges_FlowchartPipeLabelDoesNotRewriteEndpointLabels(t *testing.T) {
	body := `flowchart TD
  A["left|right"] -->|dispatch holder->run| B["target|value"]`
	edges := ParseEdges(body)
	if len(edges) != 1 || edges[0].From != "A" || edges[0].To != "B" ||
		edges[0].FromLabel != "left|right" || edges[0].ToLabel != "target|value" ||
		edges[0].Label != "dispatch holder->run" {
		t.Fatalf("pipe edge-label parsing corrupted endpoint labels: %+v", edges)
	}
}

func TestParseEdges_FlowchartChainPreservesEveryVisibleHop(t *testing.T) {
	body := `flowchart TD
  A["StageAnalyze"] --> E["StageExplore"] --> X["StageExtract"] --> F["StageFinalize"]`
	edges := ParseEdges(body)
	if len(edges) != 3 {
		t.Fatalf("edge count=%d, want 3: %+v", len(edges), edges)
	}
	want := [][2]string{{"A", "E"}, {"E", "X"}, {"X", "F"}}
	for i := range want {
		if edges[i].From != want[i][0] || edges[i].To != want[i][1] {
			t.Fatalf("edge[%d]=%s->%s, want %s->%s: %+v", i, edges[i].From, edges[i].To, want[i][0], want[i][1], edges)
		}
	}
}

func TestParseEdges_FlowchartChainKeepsPerHopLabelsAndProtectedArrowBytes(t *testing.T) {
	body := `flowchart LR
  A["holder->run; still label"] -->|first -> display| B["target::run"] -.->|second| C["done"]`
	edges := ParseEdges(body)
	if len(edges) != 2 {
		t.Fatalf("edge count=%d, want 2: %+v", len(edges), edges)
	}
	if edges[0].From != "A" || edges[0].To != "B" || edges[0].Label != "first -> display" || edges[0].Operator != "-->" {
		t.Fatalf("first chained edge corrupted: %+v", edges[0])
	}
	if edges[1].From != "B" || edges[1].To != "C" || edges[1].Label != "second" || edges[1].Operator != "-.->" {
		t.Fatalf("second chained edge corrupted: %+v", edges[1])
	}
}

func TestNormalizeSourceForMarkdown_RepairsRepeatedDotEdgesBeforeUnsafeAliasing(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		"  PreStages -..-|conditional| MainPipeline",
		"  MainPipeline -..->|optional| Finalizer",
		`  Literal["keep -..- inside label"] --> Done`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	for _, want := range []string{
		"PreStages -.-|conditional| MainPipeline",
		"MainPipeline -.->|optional| Finalizer",
		`Literal["keep -..- inside label"] --> Done`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("repeated-dot Mermaid edge was not safely normalized; missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "codraxNode") {
		t.Fatalf("malformed operator must not be aliased as a synthetic node:\n%s", got)
	}
	edges := ParseEdges(got)
	if len(edges) != 3 || edges[0].Operator != "-.-" || edges[1].Operator != "-.->" {
		t.Fatalf("normalized dotted edges were not parsed consistently: %+v", edges)
	}
	if again := NormalizeSourceForMarkdown(got); again != got {
		t.Fatalf("normalization is not idempotent:\nfirst:\n%s\nsecond:\n%s", got, again)
	}
}

func TestParseEdges_FlowchartSemicolonDoesNotBridgeIndependentStatements(t *testing.T) {
	edges := ParseEdges("flowchart TD\n  A --> B; C --> D")
	if len(edges) != 1 || edges[0].From != "A" || edges[0].To != "B" {
		t.Fatalf("semicolon-separated statement was treated as one chain: %+v", edges)
	}
}

func TestParseEdges_ClassDiagramCanonicalizesDirectedRelationEndpoints(t *testing.T) {
	body := strings.Join([]string{
		"classDiagram",
		"  Base <|-- Child",
		"  API <|.. Impl",
		"  OtherChild --|> OtherBase",
		"  OtherImpl ..|> OtherAPI",
		"  Whole *-- Part : owns",
		"  Leaf --* Root",
		"  Aggregate o-- Item",
		"  Entry --o Collection",
		"  Caller --> Callee",
		"  Parent <-- Sender",
		"  Source ..> Target",
		"  Destination <.. Origin",
	}, "\n")
	edges := ParseEdges(body)
	want := []struct {
		from, to, operator, label string
	}{
		{"Child", "Base", "<|--", ""},
		{"Impl", "API", "<|..", ""},
		{"OtherChild", "OtherBase", "--|>", ""},
		{"OtherImpl", "OtherAPI", "..|>", ""},
		{"Whole", "Part", "*--", "owns"},
		{"Root", "Leaf", "--*", ""},
		{"Aggregate", "Item", "o--", ""},
		{"Collection", "Entry", "--o", ""},
		{"Caller", "Callee", "-->", ""},
		{"Sender", "Parent", "<--", ""},
		{"Source", "Target", "..>", ""},
		{"Origin", "Destination", "<..", ""},
	}
	if len(edges) != len(want) {
		t.Fatalf("class relation edge count=%d, want %d: %+v", len(edges), len(want), edges)
	}
	for i := range want {
		if edges[i].From != want[i].from || edges[i].To != want[i].to ||
			edges[i].Operator != want[i].operator || edges[i].Label != want[i].label {
			t.Fatalf("edge[%d]=%+v, want %+v", i, edges[i], want[i])
		}
	}
}

func TestParseEdges_ClassDiagramPreservesCardinalityAndQualifiedEndpointIdentity(t *testing.T) {
	body := strings.Join([]string{
		"classDiagram",
		`  shop::Customer "1" --> "*" shop::Ticket : places`,
		`  class Literal["A <|.. B"]`,
	}, "\n")
	edges := ParseEdges(body)
	if len(edges) != 1 || edges[0].From != "shop::Customer" || edges[0].To != "shop::Ticket" ||
		edges[0].Operator != "-->" || edges[0].Label != "places" {
		t.Fatalf("class cardinality/qualified endpoint parsing changed identity: %+v", edges)
	}
}

func TestParseEdges_ClassDiagramLeavesUndirectedRelationsOutsideDirectedAuthority(t *testing.T) {
	edges := ParseEdges("classDiagram\n  A -- B\n  C .. D\n")
	if len(edges) != 0 {
		t.Fatalf("undirected class links must not mint directed semantic edges: %+v", edges)
	}
}

func TestParseEdges_SequenceMessageColonStillParses(t *testing.T) {
	body := "sequenceDiagram\n  participant A as service::handle\n  participant B as repo::count\n  A->>B: count(key: value)\n"
	edges := ParseEdges(body)
	if len(edges) != 1 || edges[0].From != "A" || edges[0].To != "B" || edges[0].Label != "count(key: value)" {
		t.Fatalf("sequence target/message split regressed: %+v", edges)
	}
}

func TestParseEdges_SequenceMessageArrowBytesStayInMessage(t *testing.T) {
	body := "sequenceDiagram\n  participant A\n  participant B\n  A->>B: compare C-->>D then E->>F\n"
	edges := ParseEdges(body)
	if len(edges) != 1 {
		t.Fatalf("edge count=%d, want 1: %+v", len(edges), edges)
	}
	if edges[0].From != "A" || edges[0].To != "B" || edges[0].Operator != "->>" ||
		edges[0].Label != "compare C-->>D then E->>F" {
		t.Fatalf("message arrow bytes replaced the actor edge: %+v", edges[0])
	}
}

func TestParseEdges_SequenceParticipantLabelArrowBytesAreNotEdges(t *testing.T) {
	body := strings.Join([]string{
		"sequenceDiagram",
		"  participant SW as sink_->write",
		"  actor CS as ConsoleSink::write",
		"  SW->>CS: write(message)",
	}, "\n")
	edges := ParseEdges(body)
	if len(edges) != 1 {
		t.Fatalf("participant presentation bytes minted extra edges: %+v", edges)
	}
	if edges[0].From != "SW" || edges[0].To != "CS" || edges[0].Label != "write(message)" {
		t.Fatalf("real sequence message changed: %+v", edges[0])
	}
}

func TestParseEdges_SequenceNonMessageDirectiveArrowBytesAreNotEdges(t *testing.T) {
	body := strings.Join([]string{
		"sequenceDiagram",
		"  participant A",
		"  participant B",
		"  Note over A,B: source path A -> B is illustrative",
		"  loop while A -> B remains pending",
		"    A->>B: dispatch C -> D",
		"  end",
		"  alt A -> B is available",
		"  else C -> D is unavailable",
		"  end",
	}, "\n")
	edges := ParseEdges(body)
	if len(edges) != 1 {
		t.Fatalf("sequence directives minted semantic edges: %+v", edges)
	}
	if edges[0].From != "A" || edges[0].To != "B" || edges[0].Label != "dispatch C -> D" {
		t.Fatalf("real sequence message changed: %+v", edges[0])
	}
}

func TestParseEdges_SequenceDirectiveNamedParticipantMessageIsPreserved(t *testing.T) {
	body := strings.Join([]string{
		"sequenceDiagram",
		"  participant Note",
		"  participant A",
		"  Note->>A: call()",
	}, "\n")
	edges := ParseEdges(body)
	if len(edges) != 1 || edges[0].From != "Note" || edges[0].To != "A" || edges[0].Label != "call()" {
		t.Fatalf("directive-named participant message was suppressed: %+v", edges)
	}
}

func TestParseEdges_SequenceOperatorMatrixMatchesSharedRendererTable(t *testing.T) {
	operators := []string{
		"-->>+", "-->>-", "->>+", "->>-",
		"--)+", "--)-", "-)+", "-)-",
		"--x+", "--x-", "-x+", "-x-",
		"-->+", "-->-", "->+", "->-",
		"-->>", "->>", "-->", "->", "--x", "-x", "--)", "-)",
	}
	for _, operator := range operators {
		t.Run(strings.NewReplacer(">", "gt", "-", "dash", "+", "plus", ")", "paren").Replace(operator), func(t *testing.T) {
			line := "A" + operator + "B: message"
			at, shared := FindSequenceArrow(line)
			if at != 1 || shared != operator {
				t.Fatalf("shared arrow lookup=%d,%q want 1,%q", at, shared, operator)
			}
			edges := ParseEdges("sequenceDiagram\n  " + line + "\n")
			if len(edges) != 1 || edges[0].From != "A" || edges[0].To != "B" || edges[0].Operator != operator || edges[0].Label != "message" {
				t.Fatalf("operator %q not preserved by semantic parser: %+v", operator, edges)
			}
		})
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

func TestNormalizeSourceForMarkdown_RewritesClassGeneralizationInsideFlowchart(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    LoopController["LoopController"]`,
		`    analyzerEvaluator["analyzerEvaluator"]`,
		`    LoopController <|-- analyzerEvaluator`,
		`    Worker --|> BaseWorker : extends`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	for _, want := range []string{
		`analyzerEvaluator -->|generalization| LoopController`,
		`Worker -->|"generalization: extends"| BaseWorker`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mixed class/flowchart edge was not normalized; missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<|--") || strings.Contains(got, "--|>") ||
		strings.Contains(got, "codraxNode") {
		t.Fatalf("class-only operator leaked into normalized flowchart:\n%s", got)
	}
}

func TestNormalizeFlowchartClassRelationEdges_MatchesClassDiagramSemanticDirection(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{name: "head on left", in: "flowchart TD\n  Interface <|-- Impl", want: "Impl -->|generalization| Interface"},
		{name: "head on right", in: "flowchart TD\n  Impl --|> Interface", want: "Impl -->|generalization| Interface"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeFlowchartClassRelationEdges(tc.in)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("semantic UML direction was not preserved; want %q:\n%s", tc.want, got)
			}
			edges := ParseEdges(got)
			if len(edges) != 1 || edges[0].From != "Impl" || edges[0].To != "Interface" {
				t.Fatalf("normalized visible edge direction = %+v", edges)
			}
		})
	}
}

func TestNormalizeFlowchartClassRelationEdges_LeavesQuotedLabelsAndClassDiagramAlone(t *testing.T) {
	flow := "flowchart TD\n  A[\"literal <|-- label\"] --> B"
	if got := NormalizeFlowchartClassRelationEdges(flow); got != flow {
		t.Fatalf("quoted label must remain byte-preserved:\n%s", got)
	}
	class := "classDiagram\n  Base <|-- Child"
	if got := NormalizeFlowchartClassRelationEdges(class); got != class {
		t.Fatalf("valid classDiagram relation must remain byte-preserved:\n%s", got)
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

func TestNormalizeSequenceParticipantDisplayLabels_QuotesCrossRendererSensitiveAliases(t *testing.T) {
	in := strings.Join([]string{
		"sequenceDiagram",
		"    participant Run as Orchestrator.Run",
		"    participant AnalyzePhase as Orchestrator.runAnalyzePhase",
		"    participant analyze as analyze",
		"    participant BusCtx as o.busCtx.AnalysisIR",
		"    participant ir as ir",
		"    Run->>AnalyzePhase: runAnalyzePhase",
		"    BusCtx-->>ir: ir := o.busCtx.AnalysisIR",
	}, "\n")

	got := NormalizeSourceForMarkdown(in)
	for _, want := range []string{
		`participant Run as "Orchestrator.Run"`,
		`participant AnalyzePhase as "Orchestrator.runAnalyzePhase"`,
		`participant analyze as analyze`,
		`participant BusCtx as "o.busCtx.AnalysisIR"`,
		`participant ir as ir`,
		`Run->>AnalyzePhase: runAnalyzePhase`,
		`BusCtx-->>ir: ir := o.busCtx.AnalysisIR`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("portable sequence normalization missing %q in:\n%s", want, got)
		}
	}
	if again := NormalizeSourceForMarkdown(got); again != got {
		t.Fatalf("participant display-label normalization must be idempotent:\nfirst:\n%s\nsecond:\n%s", got, again)
	}
	beforeEdges := ParseEdges(in)
	afterEdges := ParseEdges(got)
	if !reflect.DeepEqual(beforeEdges, afterEdges) {
		t.Fatalf("display-label normalization changed message topology:\nbefore=%+v\nafter=%+v", beforeEdges, afterEdges)
	}
}

func TestNormalizeSequenceParticipantDisplayLabels_PreservesQuotedLabelsAndInlineCommentUnknowns(t *testing.T) {
	in := strings.Join([]string{
		"sequenceDiagram",
		`    participant A as "Already.Quoted"`,
		"    actor B as Customer %% presentation note",
		"    A->>B: call",
	}, "\n")
	if got := NormalizeSequenceParticipantDisplayLabels(in); got != in {
		t.Fatalf("already-safe or unsupported comment-bearing declaration changed:\n%s", got)
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

func TestNormalizeSourceForMarkdown_QuotedPipeInsideEdgeLabelIsNotDelimiter(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    NewFinalizerAgent -->|"返回 BaseAgent|注册为 Finalizer"| FinalizerAgent`,
		`    FinalizerAgent -->|"完整输出|增量修补"| EmitAnswerDocument`,
		`    EmitAnswerDocument -->|don't split apostrophes| Result`,
	}, "\n")

	got := NormalizeSourceForMarkdown(in)
	if got != in {
		t.Fatalf("quoted label pipes are display text, not edge-label delimiters:\nwant:\n%s\ngot:\n%s", in, got)
	}
	edges := ParseEdges(got)
	if len(edges) != 3 ||
		edges[0].From != "NewFinalizerAgent" || edges[0].To != "FinalizerAgent" ||
		edges[1].From != "FinalizerAgent" || edges[1].To != "EmitAnswerDocument" ||
		edges[2].From != "EmitAnswerDocument" || edges[2].To != "Result" {
		t.Fatalf("quoted label pipes changed visible topology: %+v", edges)
	}
}

func TestNormalizeSourceForMarkdown_CanonicalizesInlineEdgeLabelsWithoutChangingTopology(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`  analyzerEval["analyzerEvaluator"] -.implements.-> LoopController["LoopController"]`,
		`  plannerEval -- implements --> LoopController`,
		`  hotPath == dominates ==> target`,
		`  first --> second --continues--> third -.observes.-> fourth`,
	}, "\n")
	want := strings.Join([]string{
		"flowchart TD",
		`  analyzerEval["analyzerEvaluator"] -.->|implements| LoopController["LoopController"]`,
		`  plannerEval -->|implements| LoopController`,
		`  hotPath ==>|dominates| target`,
		`  first --> second -->|continues| third -.->|observes| fourth`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if got != want {
		t.Fatalf("inline edge-label repair mismatch:\nwant:\n%s\ngot:\n%s", want, got)
	}
	if strings.Contains(got, "codraxNode") {
		t.Fatalf("inline relation label must not be aliased as a node:\n%s", got)
	}
	edges := ParseEdges(got)
	if len(edges) != 6 ||
		edges[0].From != "analyzerEval" || edges[0].To != "LoopController" || edges[0].Label != "implements" ||
		edges[1].From != "plannerEval" || edges[1].To != "LoopController" || edges[1].Label != "implements" ||
		edges[2].From != "hotPath" || edges[2].To != "target" || edges[2].Label != "dominates" ||
		edges[3].From != "first" || edges[3].To != "second" ||
		edges[4].From != "second" || edges[4].To != "third" || edges[4].Label != "continues" ||
		edges[5].From != "third" || edges[5].To != "fourth" || edges[5].Label != "observes" {
		t.Fatalf("inline edge-label repair changed semantic topology: %+v", edges)
	}
	if again := NormalizeSourceForMarkdown(got); again != got {
		t.Fatalf("inline edge-label repair must be idempotent:\nfirst:\n%s\nsecond:\n%s", got, again)
	}
}

func TestNormalizeSourceForMarkdown_DoesNotTreatInlineOperatorBytesInsideLabelsAsEdges(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`  A["literal --implements--> text"] --> B["literal -.calls.-> text"]`,
		`  C -->|already portable| D`,
	}, "\n")
	if got := NormalizeSourceForMarkdown(in); got != in {
		t.Fatalf("display-label operator bytes must remain untouched:\nwant:\n%s\ngot:\n%s", in, got)
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

func TestNormalizeSourceForMarkdown_RepairsJSONEscapedQuotesInsideQuotedLabels(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		`    make_sink["make_sink<br/>(kind=\"console\")"] --> guard{"kind == \"console\"?"}`,
		`    guard -->|"kind=\"console\""| console[ConsoleSink]`,
	}, "\n")
	want := strings.Join([]string{
		"flowchart TD",
		`    make_sink["make_sink<br/>(kind=&quot;console&quot;)"] --> guard{"kind == &quot;console&quot;?"}`,
		`    guard -->|"kind=&quot;console&quot;"| console[ConsoleSink]`,
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if got != want {
		t.Fatalf("quoted-label quote repair mismatch:\nwant:\n%s\ngot:\n%s", want, got)
	}
	if again := NormalizeSourceForMarkdown(got); again != got {
		t.Fatalf("quoted-label quote repair must be idempotent:\nfirst:\n%s\nsecond:\n%s", got, again)
	}
	edges := ParseEdges(got)
	if len(edges) != 2 || edges[0].From != "make_sink" || edges[0].To != "guard" ||
		edges[1].From != "guard" || edges[1].To != "console" {
		t.Fatalf("quote repair changed topology: %+v", edges)
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

func TestNormalizeClassDiagramToFlowchart_PreservesDirectedTypeRelations(t *testing.T) {
	in := strings.Join([]string{
		"classDiagram",
		"    class LoopController {",
		"        <<interface>>",
		"        +Observe(ctx, obs) LoopSignal",
		"    }",
		"    class analyzerEvaluator",
		"    analyzerEvaluator ..|> LoopController",
	}, "\n")
	got, ok := NormalizeClassDiagramToFlowchart(in)
	if !ok {
		t.Fatal("portable classDiagram should convert")
	}
	for _, want := range []string{
		"flowchart TD",
		`LoopController["LoopController<br/>&lt;&lt;interface&gt;&gt;<br/>+Observe(ctx, obs) LoopSignal"]`,
		`analyzerEvaluator["analyzerEvaluator"]`,
		`analyzerEvaluator -->|"implements"| LoopController`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("converted class diagram missing %q:\n%s", want, got)
		}
	}
	if again, ok := NormalizeClassDiagramToFlowchart(got); ok || again != got {
		t.Fatalf("converter must be idempotent outside classDiagram: ok=%t\n%s", ok, again)
	}
}

func TestNormalizeClassDiagramToFlowchart_FailsOpenWhenCardinalityWouldBeLost(t *testing.T) {
	in := "classDiagram\n  Customer \"1\" --> \"many\" Order : places"
	if got, ok := NormalizeClassDiagramToFlowchart(in); ok || got != in {
		t.Fatalf("cardinality-bearing diagram must stay byte-identical: ok=%t got=%q", ok, got)
	}
}

// MMD-1 (2026-07-13, witness .codrax/output/20260713-181931.791-19240.html):
// Mermaid lexes bare subgraph titles with the restricted statement-level
// token stream (NODE_STRING + UNICODE_TEXT letters), so a single-token CJK
// title carrying an em-dash killed the whole browser render. The repair
// quotes exactly the empirically unlexable titles and leaves every form the
// lexer accepts byte-identical.
func TestNormalizeSourceForMarkdown_QuotesUnlexableSubgraphTitles(t *testing.T) {
	in := strings.Join([]string{
		"flowchart TD",
		"    subgraph 次因—优先级反转候选",
		`        K1["x"]`,
		"    end",
	}, "\n")
	got := NormalizeSourceForMarkdown(in)
	if strings.Contains(got, "subgraph 次因—优先级反转候选") {
		t.Fatalf("unlexable em-dash subgraph title survived unquoted:\n%s", got)
	}
	if !strings.Contains(got, `subgraph subgraph_2 ["次因—优先级反转候选"]`) {
		t.Fatalf("em-dash subgraph title was not rewritten to explicit quoted form:\n%s", got)
	}
}

func TestNormalizeSourceForMarkdown_QuotesUnlexableSubgraphTitleRunes(t *testing.T) {
	// Every title below fails Mermaid 11.12.0's statement-level lexer when
	// left bare (verified against the embedded mermaid.min.js).
	for _, title := range []string{
		"次因–候选",     // en dash
		"次因×候选",     // multiplication sign
		"次因·候选",     // middle dot
		"次因、候选",     // ideographic comma
		"次因：候选",     // fullwidth colon
		"次因（候选）",    // fullwidth parentheses
		"次因…候选",     // ellipsis
		"次因１候选",     // fullwidth digit
		"次因①候选",     // circled digit
		"stage,two", // ASCII comma
		"stage@two", // ASCII at
		"stage=two", // ASCII equals
		"a->b",      // link token
		"a--b",      // link token
		"a==b",      // link token
	} {
		in := strings.Join([]string{
			"flowchart TD",
			"    subgraph " + title,
			`        K1["x"]`,
			"    end",
		}, "\n")
		got := NormalizeSourceForMarkdown(in)
		if strings.Contains(got, "subgraph "+title+"\n") {
			t.Fatalf("unlexable subgraph title %q survived unquoted:\n%s", title, got)
		}
		if !strings.Contains(got, `["`+title+`"]`) {
			t.Fatalf("unlexable subgraph title %q lost its quoted visible label:\n%s", title, got)
		}
	}
}

func TestNormalizeSourceForMarkdown_KeepsLexableSubgraphTitlesByteIdentical(t *testing.T) {
	// Every title below parses bare in Mermaid 11.12.0 (verified against the
	// embedded mermaid.min.js), so the shim must not touch a byte.
	for _, title := range []string{
		"目标线程",              // plain CJK letters
		"根因层",               // plain CJK letters
		"Ｓｔａｇｅカナー",          // fullwidth letters + kana + prolonged sound mark
		"étape_greek_αβ",    // accented Latin + Greek
		"stage.two",         // ASCII dot
		"stage:two",         // ASCII colon
		"stage/two",         // ASCII slash
		"stage&two",         // ASCII ampersand
		"stage;two",         // ASCII semicolon
		"stage+two",         // ASCII plus
		"a-b",               // single dash
		"a..b",              // dots
		`"次因—优先级反转候选"`,      // already fully quoted: legal STR title
		`sg1["次因—优先级反转候选"]`, // already explicit id["title"] form
	} {
		in := strings.Join([]string{
			"flowchart TD",
			"    subgraph " + title,
			`        K1["x"]`,
			"    end",
		}, "\n")
		if got := NormalizeSourceForMarkdown(in); got != in {
			t.Fatalf("lexable subgraph title %q was perturbed:\nin:\n%s\ngot:\n%s", title, in, got)
		}
	}
}

func TestRemovableNodeDeclarationsAndExactRemoval(t *testing.T) {
	t.Run("sequence explicit only", func(t *testing.T) {
		body := "sequenceDiagram\n  participant A as API\n  actor B\n  A->>B: call\n  Note over A: keep\n"
		got := RemovableNodeDeclarations(body)
		if len(got) != 2 || got[0].Ident != "A" || got[0].Label != "API" || got[1].Ident != "B" {
			t.Fatalf("removable sequence declarations=%+v", got)
		}
		after, count := RemoveRemovableNodeDeclaration(body, "A")
		if count != 1 || strings.Contains(after, "participant A as API") || !strings.Contains(after, "A->>B: call") || !strings.HasSuffix(after, "\n") {
			t.Fatalf("sequence removal count=%d body=%q", count, after)
		}
	})

	t.Run("flow standalone only", func(t *testing.T) {
		body := "flowchart LR\n  A[API]\n  B\n  A --> C[Worker]\n  subgraph SG[Group]\n    D[Member]\n  end\n"
		got := RemovableNodeDeclarations(body)
		if len(got) != 3 || got[0].Ident != "A" || got[1].Ident != "B" || got[2].Ident != "D" {
			t.Fatalf("removable flow declarations=%+v", got)
		}
		after, count := RemoveRemovableNodeDeclaration(body, "B")
		if count != 1 || strings.Contains(after, "\n  B\n") || !strings.Contains(after, "A --> C[Worker]") {
			t.Fatalf("flow removal count=%d body=%q", count, after)
		}
	})

	t.Run("duplicate stays ambiguous", func(t *testing.T) {
		body := "sequenceDiagram\n participant A\n participant A as Again\n"
		if after, count := RemoveRemovableNodeDeclaration(body, "A"); count != 2 || after != body {
			t.Fatalf("duplicate declaration must remain untouched: count=%d after=%q", count, after)
		}
	})
}

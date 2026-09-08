package agent

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1627ConcreteReturnPreservesWholeExpressionWithoutBindingGuess(t *testing.T) {
	for _, lang := range repotypes.SupportedReadLanguages() {
		if lang == repotypes.LangProto {
			continue // Proto has declarations, not executable return statements.
		}
		t.Run(lang, func(t *testing.T) {
			for _, expression := range []string{
				"delayMs + Math.floor(Math.random() * this.spreadMs)",
				"baseValue + Arithmetic.round(Sampler.next() * spreadValue)",
				"factory()", "NewHandler(deps)", `"worker"`, "false", "17",
			} {
				entries := extractConcreteValues("return "+expression, lang)
				found := false
				for _, entry := range entries {
					if isBindsKind(entry.kind) {
						t.Errorf("return expression became a guessed binding: %s: %+v", expression, entry)
					}
					if entry.kind == "returns" && entry.value == expression {
						found = true
						if entry.candidate {
							t.Errorf("complete source expression became an incomplete/derived claim: %+v", entry)
						}
					}
				}
				if !found {
					t.Errorf("whole return expression missing: %s: %+v", expression, entries)
				}
			}
		})
	}
}

func TestB1627ConcreteConstructorCandidatesAcrossLanguageMotherTable(t *testing.T) {
	sources := map[string]string{
		repotypes.LangGo:         "registry.Register(NewHandler(deps))",
		repotypes.LangPython:     "registry.register(Handler())",
		repotypes.LangJavaScript: "registry.register(new Handler());",
		repotypes.LangTypeScript: "registry.register(new Handler());",
		repotypes.LangJava:       "registry.register(new Handler());",
		repotypes.LangKotlin:     "registry.register(Handler())",
		repotypes.LangRust:       "registry.register(Handler::new());",
		repotypes.LangC:          "register_handler(NewHandler());",
		repotypes.LangCpp:        "registry.Register(Handler::Create());",
		repotypes.LangArkTS:      "registry.register(new Handler());",
		repotypes.LangCangjie:    "registry.register(Handler())",
		repotypes.LangRuby:       "registry.register(Handler.new())",
		repotypes.LangSwift:      "registry.register(Handler())",
		repotypes.LangLua:        "registry:register(Handler.new())",
	}
	for _, lang := range repotypes.SupportedReadLanguages() {
		t.Run(lang, func(t *testing.T) {
			if lang == repotypes.LangProto {
				entries := extractDeclarationConcreteValues("rpc Fetch (Request) returns (Response);", lang)
				found := false
				for _, entry := range entries {
					if isBindsKind(entry.kind) || entry.candidate {
						t.Errorf("declaration contract became a constructor guess: %+v", entry)
					}
					if entry.kind == "returns" && strings.Contains(entry.value, "Response") {
						found = true
					}
				}
				if !found {
					t.Fatalf("Proto declaration contract was removed: %+v", entries)
				}
				return
			}
			source, ok := sources[lang]
			if !ok {
				t.Fatalf("supported language lacks explicit source shape: %s", lang)
			}
			entries := extractConcreteValues(source, lang)
			found := false
			for _, entry := range entries {
				if isBindsKind(entry.kind) {
					found = true
					if entry.kind != "binds" || !entry.candidate || entry.lineOffset != 0 {
						t.Errorf("constructor-shaped argument must stay a source-located candidate, without exclusivity: %+v", entry)
					}
				}
			}
			if !found {
				t.Fatalf("real constructor-shaped call was discarded: %+v", entries)
			}
		})
	}
}

func TestB1627ConcreteReturnBoundaryKeepsDelimitersAndIncompleteLimits(t *testing.T) {
	for _, tc := range []struct {
		source, want string
		candidate    bool
	}{
		{`func f() { return &Handler{} }`, "&Handler{}", false},
		{`return Handler{Nested: Pair{A: 1}};`, "Handler{Nested: Pair{A: 1}}", false},
		{`return "braces }; and // stay"; }`, `"braces }; and // stay"`, false},
		{`return sourceValue + tail; } else { other(); }`, "sourceValue + tail", false},
		{`return factory(`, "factory(", true},
		{`return Handler{`, "Handler{", true},
		{`return "unterminated`, `"unterminated`, true},
	} {
		t.Run(tc.source, func(t *testing.T) {
			entries := extractConcreteValues(tc.source, repotypes.LangGo)
			for _, entry := range entries {
				if entry.kind == "returns" {
					if entry.value != tc.want || entry.candidate != tc.candidate {
						t.Fatalf("incorrect source expression boundary: %+v", entry)
					}
					return
				}
			}
			t.Fatalf("source clue was lost: %+v", entries)
		})
	}
}

func TestB1627ConcreteCompletionCountersRequireIndependentFacts(t *testing.T) {
	item := types.EvidenceItem{
		ID: "exact", Kind: types.EvidenceRegistration, Scope: types.ScopeLine,
		Subject: "Register", Predicate: "registers", Object: "Handler",
		Source: "fixture.go", LineStart: 2, LineEnd: 2, GroundingStatus: types.GroundingGrounded,
	}
	candidate := item
	candidate.ID, candidate.DerivationCandidate = "candidate", true
	for _, pool := range [][]types.EvidenceItem{{candidate}, {candidate, item}, {item, candidate}} {
		eval := &explorerEvaluator{
			structuredEvidence: pool,
			ermRequirements:    []EvidenceRequirement{{Kind: types.ReqRegistration}},
		}
		before := append([]types.EvidenceItem(nil), pool...)
		want := len(pool) - 1
		if got := eval.groundedRequirementCarrierCount(); got != want {
			t.Errorf("grounded completion counted candidate: got %d want %d", got, want)
		}
		got := eval.completionReadinessWithCoverage(nil, 0, false, false, nil, nil)
		if got.DirectCount != want {
			t.Errorf("direct completion counted candidate: got %d want %d", got.DirectCount, want)
		}
		if !reflect.DeepEqual(before, eval.structuredEvidence) {
			t.Fatal("completion counter changed model/source evidence")
		}
	}
}

func b1627ParsedGraph(t *testing.T, files map[string]string) (*repomap.Graph, string) {
	t.Helper()
	root := t.TempDir()
	for file, source := range files {
		path := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := repomap.ScanFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	return repomap.BuildGraph(root, repomap.ParseFiles(entries, root)), root
}

func TestB1627ConcreteProductionKeepsCandidatesAndIndependentLiteral(t *testing.T) {
	graph, root := b1627ParsedGraph(t, map[string]string{
		"registry.go": `package sample
type Registry struct{}
func (r *Registry) Register(value any) {}
func RegisterHandlers(r *Registry) {
    r.Register(NewHandler())
}
`,
		"handler.go": `package sample
type Handler struct{}
func NewHandler() *Handler { return &Handler{} }
func (h *Handler) Name() string { return "worker" }
`,
	})
	eval := &explorerEvaluator{searchResult: &keywordSearchResult{Graph: graph}}
	result := eval.buildConcreteValuesSection(context.Background(), root, map[string]bool{"registry.go": true, "handler.go": true}, nil)
	foundBinding, foundChain, foundLiteral := false, false, false
	for _, item := range result.evidence {
		if item.Producer == "concrete_values" && isBindsKind(item.Predicate) {
			foundBinding = true
			if !types.EvidenceIsDerivationCandidate(item) || item.Predicate != "binds" || item.Source != "registry.go" || item.LineStart != 5 {
				t.Errorf("binding guess lost its limit or exact source: %+v", item)
			}
		}
		if item.Predicate == "resolution_chain" {
			foundChain = true
			if !types.EvidenceIsDerivationCandidate(item) {
				t.Errorf("joined candidate became a proved chain: %+v", item)
			}
		}
		if item.Predicate == "returns" && item.Object == `"worker"` {
			foundLiteral = true
			if types.EvidenceIsDerivationCandidate(item) || item.Source != "handler.go" || item.LineStart != 4 {
				t.Errorf("independently extracted source literal was downgraded or relocated: %+v", item)
			}
		}
	}
	if !foundBinding || !foundChain || !foundLiteral {
		t.Fatalf("capabilities disappeared: binding=%t chain=%t literal=%t evidence=%+v", foundBinding, foundChain, foundLiteral, result.evidence)
	}
	if !strings.Contains(result.markdown, "Candidate derivation") || strings.Contains(result.markdown, "binds ONLY") || strings.Contains(result.markdown, "ground truth, not summaries") {
		t.Fatalf("producer markdown still teaches candidates as exact/exclusive: %s", result.markdown)
	}
}

func TestB1627ConcreteProductionRetainsTSReturnExpression(t *testing.T) {
	const expression = "delayMs + Math.floor(Math.random() * this.spreadMs)"
	graph, root := b1627ParsedGraph(t, map[string]string{"retry.ts": `export class JitterHelper {
  constructor(private spreadMs: number) {}
  apply(delayMs: number): number {
    return ` + expression + `;
  }
}
`})
	eval := &explorerEvaluator{
		searchResult: &keywordSearchResult{Graph: graph},
		structuredEvidence: []types.EvidenceItem{{
			ID: "definition", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
			Source: "retry.ts", LineStart: 3, LineEnd: 3, AnchorKind: types.AnchorDefinition,
			AnchorSymbol: "apply", Subject: "JitterHelper.apply", GroundingStatus: types.GroundingGrounded,
		}},
	}
	result := eval.buildConcreteValuesSection(context.Background(), root, map[string]bool{"retry.ts": true}, nil)
	found := false
	for _, item := range result.evidence {
		if item.Producer != "concrete_values" || item.Subject != "JitterHelper.apply" {
			continue
		}
		if isBindsKind(item.Predicate) {
			t.Errorf("return was recast as a constructor binding: %+v", item)
		}
		if item.Predicate == "returns" && item.Object == expression {
			found = true
			if item.LineStart != 4 || types.EvidenceIsDerivationCandidate(item) {
				t.Errorf("complete source expression lost its source-only semantics: %+v", item)
			}
		}
	}
	if !found {
		t.Fatalf("production parser/enrichment dropped exact return: %+v", result.evidence)
	}
}

func TestB1627BridgeJoinsDoNotUpgradeTheirLiteralDependency(t *testing.T) {
	graph, root := b1627ParsedGraph(t, map[string]string{"fixture.go": `package sample
type Handler struct{}
type Registry struct{}
func (r *Registry) Register(value any) {}
func NewHandler() *Handler { return &Handler{} }
func (h *Handler) Name() string { return "worker" }
func RegisterHandlers(r *Registry) {
    r.Register(NewHandler())
}
`})
	extraction := extractBridgeLiteralEvidence(graph, root, []concreteValue{{
		file: "consumer.go", method: "Dispatch", kind: "assigns", value: "target := services.Handlers.Get(key)", line: 9,
	}})
	seen := map[string]bool{}
	for _, item := range extraction.chains {
		seen[item.Producer] = true
		if !types.EvidenceIsDerivationCandidate(item) {
			t.Errorf("heuristic join inherited proof from its literal: %+v", item)
		}
	}
	if !seen["bridge_literal"] || !seen["consumer_gate"] || len(extraction.terminalReturns) == 0 {
		t.Fatalf("independent literal / bridge / consumer capability lost: %+v", extraction)
	}
	for _, item := range extraction.terminalReturns {
		if types.EvidenceIsDerivationCandidate(item) || item.Object != `"worker"` || item.Source != "fixture.go" || item.LineStart != 6 {
			t.Errorf("real independent literal no longer usable: %+v", item)
		}
	}
	provider := types.EvidenceRelationCandidateSource{Items: append(append([]types.EvidenceItem(nil), extraction.chains...), extraction.terminalReturns...)}
	query := types.TypedRelationQuery{Sources: []string{"RegisterHandlers"}, Kinds: []types.TypedRelationKind{types.TypedRelationRegisters}, Purpose: types.TypedRelationPurposePromptHint}
	rows := provider.TypedRelationCandidates(query)
	if len(rows) == 0 {
		t.Fatal("real bridge no longer provides registration navigation")
	}
	for _, row := range rows {
		if row.CoverageGateEligible() || row.Member.Name != "worker" || row.Member.File != "fixture.go" || row.SourceLine != 8 {
			t.Errorf("real producer bridge must retain source/member navigation without coverage authority: %+v", row)
		}
	}
	query.Purpose = types.TypedRelationPurposeCoverageGate
	if got := provider.TypedRelationCandidates(query); len(got) != 0 {
		t.Errorf("real producer candidate was promoted by relation provider: %+v", got)
	}
}

func TestB1627BridgeFactoryJoinAndNonLiteralReturnBoundary(t *testing.T) {
	graph, root := b1627ParsedGraph(t, map[string]string{"fixture.go": `package sample
type Handler struct { Name string }
type Registry struct{}
func (r *Registry) Register(value any) {}
func BuildHandler() *Handler {
    return &Handler{
        Name: "worker",
    }
}
func RegisterHandlers(r *Registry) {
    r.Register(BuildHandler())
}
`})
	extraction := extractBridgeLiteralEvidence(graph, root, nil)
	if len(extraction.chains) == 0 {
		t.Fatal("factory source-expression clue was removed")
	}
	for _, item := range extraction.chains {
		if !types.EvidenceIsDerivationCandidate(item) {
			t.Errorf("factory/name-field join became proof: %+v", item)
		}
	}
	for _, raw := range []string{`"worker" + "suffix"`, `"worker".upper()`, `"worker`} {
		if concreteValueIsQuotedLiteral(raw) {
			t.Errorf("expression or fragment became one independent literal: %s", raw)
		}
	}
	for _, raw := range []string{`"worker"`, `'worker'`, `"worker\"suffix"`} {
		if !concreteValueIsQuotedLiteral(raw) {
			t.Errorf("whole source literal was lost: %s", raw)
		}
	}
}

func TestB1627ConcretePreviewAndConflictTeachingRemainAdvisory(t *testing.T) {
	graph, root := b1627ParsedGraph(t, map[string]string{"registry.go": `package sample
func RegisterHandlers(r *Registry) {
    r.Register(NewHandler())
}
`})
	eval := &explorerEvaluator{
		phase: 1, repoRoot: root, searchResult: &keywordSearchResult{Graph: graph},
		exactAnchorFiles: []string{"registry.go"}, investigationNotes: []string{"examining the registration", "checking the source relation"},
	}
	signal := eval.observeSoftStop(LoopObservation{ContinuationsUsed: 1, IdleStreak: 1})
	if !strings.Contains(signal.Hint, "Programmatic Evidence Preview") || !strings.Contains(signal.Hint, "Candidate derivation") {
		t.Fatalf("actual soft-stop preview route not exercised: %+v", signal)
	}
	for _, bad := range []string{"do NOT need to re-investigate", "provided as ground truth"} {
		if strings.Contains(signal.Hint, bad) {
			t.Errorf("soft preview negates the candidate boundary: %s", bad)
		}
	}
	conflict := crossValidateEvidence([]string{"- [REGISTRATION] `RegisterHandlers()` line 3: binds OtherHandler"},
		"| registry.go:3 | `RegisterHandlers()` | Candidate derivation: binds NewHandler() |")
	if conflict == "" || !strings.Contains(conflict, "RegisterHandlers") {
		t.Fatal("existing soft discrepancy navigation was removed")
	}
	for _, bad := range []string{"CONTRADICT", "ground truth", "source code shows"} {
		if strings.Contains(conflict, bad) {
			t.Errorf("noisy comparison became source proof: %s", conflict)
		}
	}
}

func TestB1627ConcreteReturnLanguageBoundariesDoNotTruncateUnknownSyntax(t *testing.T) {
	for _, tc := range []struct {
		lang, expression string
		candidate        bool
	}{
		{repotypes.LangPython, "total // count", false},
		{repotypes.LangLua, "total // count", false},
		{repotypes.LangJavaScript, "/[;}]/.test(value); }", true},
		{repotypes.LangTypeScript, "`outer ${format(`inner ${value}`)}`; }", true},
		{repotypes.LangArkTS, "left / right; }", true},
		{repotypes.LangRust, "borrow::<'a, 'b>(value); }", true},
		{repotypes.LangGo, "value // pending comment }", true},
		{repotypes.LangGo, `"url://inside"`, false},
	} {
		t.Run(tc.lang+"/"+tc.expression, func(t *testing.T) {
			for _, entry := range extractConcreteValues("return "+tc.expression, tc.lang) {
				if entry.kind != "returns" {
					continue
				}
				if entry.value != tc.expression || entry.candidate != tc.candidate {
					t.Fatalf("language-ambiguous source was truncated or upgraded: %+v", entry)
				}
				return
			}
			t.Fatal("source clue was dropped")
		})
	}
}

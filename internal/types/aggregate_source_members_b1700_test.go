package types

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func b1700SourceMemberFixture() (AnswerAggregateFact, RequestModel, ObservationLedgerInput) {
	fact := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Role: AnswerAggregateRolePrincipalAnswer,
		Label: "selected members", Value: "1", Members: []string{"Worker"}, SupportRefs: []string{"src/worker.go:12"},
		MemberNotes: []string{"model-owned explanation remains unchanged"}}
	rm := RequestModel{Intent: IntentEnumerate}
	source := ObservationLedgerInput{EvidenceItems: []EvidenceItem{{ID: "worker", Kind: EvidenceDirect,
		Scope: ScopeLine, Source: "src/worker.go", LineStart: 12, AnchorKind: AnchorDefinition,
		Subject: "Worker", GroundingStatus: GroundingGrounded}}}
	return fact, rm, source
}

func TestB1700SourceMemberQualificationRequiresObservedIdentity(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*AnswerAggregateFact, *ObservationLedgerInput)
		want bool
	}{
		{"exact-definition", func(*AnswerAggregateFact, *ObservationLedgerInput) {}, true},
		{"recovered-definition", func(_ *AnswerAggregateFact, s *ObservationLedgerInput) {
			s.EvidenceItems[0].GroundingStatus = GroundingRecovered
		}, true},
		{"numeric-symbol", func(f *AnswerAggregateFact, s *ObservationLedgerInput) {
			f.Members[0] = "Worker50"
			s.EvidenceItems[0].Subject = "Worker50"
		}, true},
		{"unobserved-label", func(f *AnswerAggregateFact, _ *ObservationLedgerInput) { f.Members[0] = "unobserved runtime result" }, false},
		{"unobserved-number", func(f *AnswerAggregateFact, _ *ObservationLedgerInput) { f.Members[0] = "86.111ms > 50ms" }, false},
		{"source-prose", func(f *AnswerAggregateFact, s *ObservationLedgerInput) {
			f.Members[0] = "unobserved"
			s.EvidenceItems[0].Summary = "unobserved"
			s.EvidenceItems[0].Snippet = "unobserved"
			s.EvidenceItems[0].SurfaceTerms = []string{"unobserved"}
		}, false},
		{"empty-status", func(_ *AnswerAggregateFact, s *ObservationLedgerInput) { s.EvidenceItems[0].GroundingStatus = "" }, false},
		{"ungrounded", func(_ *AnswerAggregateFact, s *ObservationLedgerInput) {
			s.EvidenceItems[0].GroundingStatus = GroundingUngrounded
		}, false},
		{"unknown-claim", func(_ *AnswerAggregateFact, s *ObservationLedgerInput) { s.EvidenceItems[0].AnchorKind = "" }, false},
		{"text-reference-subject", func(_ *AnswerAggregateFact, s *ObservationLedgerInput) {
			s.EvidenceItems[0].AnchorKind = AnchorTextReference
		}, false},
		{"definition-spoofed-subject", func(_ *AnswerAggregateFact, s *ObservationLedgerInput) { s.EvidenceItems[0].AnchorSymbol = "Other" }, false},
		{"definition-spoofed-object", func(f *AnswerAggregateFact, s *ObservationLedgerInput) {
			f.Members[0] = "Other"
			s.EvidenceItems[0].Object = "Other"
		}, false},
		{"different-file", func(_ *AnswerAggregateFact, s *ObservationLedgerInput) { s.EvidenceItems[0].Source = "other/worker.go" }, false},
		{"different-line", func(_ *AnswerAggregateFact, s *ObservationLedgerInput) { s.EvidenceItems[0].LineStart = 13 }, false},
		{"one-false-member", func(f *AnswerAggregateFact, _ *ObservationLedgerInput) {
			f.Members = append(f.Members, "Other")
			f.Value = "2"
		}, false},
		{"one-false-ref", func(f *AnswerAggregateFact, _ *ObservationLedgerInput) {
			f.SupportRefs = append(f.SupportRefs, "other.go:1")
		}, false},
		{"source-location", func(f *AnswerAggregateFact, _ *ObservationLedgerInput) { f.Members[0] = "src/worker.go:12" }, true},
		{"source-file", func(f *AnswerAggregateFact, _ *ObservationLedgerInput) { f.Members[0] = "src/worker.go" }, true},
		{"inline-location", func(f *AnswerAggregateFact, _ *ObservationLedgerInput) { f.Members[0] = "Worker @ src/worker.go:12" }, true},
		{"label-spoof-at-location", func(f *AnswerAggregateFact, _ *ObservationLedgerInput) { f.Members[0] = "Other @ src/worker.go:12" }, false},
		{"return-value", func(f *AnswerAggregateFact, s *ObservationLedgerInput) {
			f.Members[0] = "worker"
			s.EvidenceItems[0].AnchorKind = AnchorReturn
			s.EvidenceItems[0].Object = "worker"
		}, true},
		{"assignment-value", func(f *AnswerAggregateFact, s *ObservationLedgerInput) {
			f.Members[0] = "50"
			s.EvidenceItems[0].AnchorKind = AnchorAssignment
			s.EvidenceItems[0].Object = "50"
		}, true},
		{"external-evidence", func(_ *AnswerAggregateFact, s *ObservationLedgerInput) { s.EvidenceItems[0].Origin = ClaimOriginPerf }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fact, rm, source := b1700SourceMemberFixture()
			test.edit(&fact, &source)
			before, _ := json.Marshal(fact)
			if got := AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &rm, source); got != test.want {
				t.Fatalf("principal qualification=%v want=%v", got, test.want)
			}
			if got := aggregateFactHasIndependentTypedAuthorityWithSourceContext(fact, &rm, source); got != test.want {
				t.Fatalf("independent source qualification=%v want=%v", got, test.want)
			}
			set := EnumerationDisplaySet{Rows: []EnumerationDisplayRow{{HasCitation: true, Source: "src/worker.go", LineStart: 12}}}
			if got := EnumerationDisplaySetAuthorizesPrincipalContractWithSourceContext(&rm, fact, set, source); got != test.want {
				t.Fatalf("display qualification=%v want=%v", got, test.want)
			}
			after, _ := json.Marshal(fact)
			if string(before) != string(after) {
				t.Fatal("source admission rewrote the model fact")
			}
		})
	}
}

func TestB1700SourceMemberContextCannotComeFromSerializationOrSummary(t *testing.T) {
	fact, rm, source := b1700SourceMemberFixture()
	encoded, err := json.Marshal(fact)
	if err != nil {
		t.Fatal(err)
	}
	var replay AnswerAggregateFact
	if err := json.Unmarshal(encoded, &replay); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fact, replay) {
		t.Fatal("fact JSON compatibility changed")
	}
	for _, context := range []ObservationLedgerInput{
		{},
		{ToolResults: []ToolResult{{ToolName: "read_file", Success: true, Summary: "Worker src/worker.go:12 grounded"}}},
	} {
		if AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(replay, &rm, context) {
			t.Fatal("replayed facts/summary minted observed membership")
		}
	}
	if !AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(replay, &rm, source) {
		t.Fatal("replayed claim with actual current evidence lost eligibility")
	}
	// Compatibility classification is intentionally not a production proof API.
	if !AnswerAggregateFactAuthorizesPrincipalContract(replay, &rm) {
		t.Fatal("context-free API compatibility changed")
	}
}

func TestB1700ExplicitExternalLaneDoesNotCertifySourceMember(t *testing.T) {
	for _, ref := range []string{"trace_query:window_stats:E7", "git_log[0]", "mcp_resource: mcp://fixture/report#L7"} {
		fact, rm, source := b1700SourceMemberFixture()
		fact.Members[0] = "unobserved external member"
		fact.SupportRefs = append(fact.SupportRefs, ref)
		if !AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &rm, source) {
			t.Fatalf("external lane lost: %s", ref)
		}
		if aggregateFactHasIndependentTypedAuthorityWithSourceContext(fact, &rm, source) {
			t.Fatalf("external ref certified arbitrary source member: %s", ref)
		}
		set := EnumerationDisplaySet{EvidenceOrigins: []AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource, AnswerEvidenceOriginRuntimeArtifact},
			Rows: []EnumerationDisplayRow{{HasCitation: true, Source: "src/worker.go", LineStart: 12}}}
		if EnumerationDisplaySetAuthorizesPrincipalContractWithSourceContext(&rm, fact, set, source) ||
			AnswerAggregateFactAuthorizesSourceMemberCarrier(fact, &rm, source) {
			t.Fatalf("external principal permission minted an unobserved source member table: %s", ref)
		}
	}
}

func TestB1700FullSourceContextSurvivesDisplayLimitAndPlanSelection(t *testing.T) {
	fact, rm, source := b1700SourceMemberFixture()
	mu := NewMutableState("list members")
	var evidence []EvidenceItem
	for i := 0; i < 70; i++ {
		evidence = append(evidence, EvidenceItem{ID: fmt.Sprintf("context-%d", i), Kind: EvidenceDirect, Scope: ScopeLine,
			Source: "context.go", LineStart: i + 1, AnchorKind: AnchorDefinition, Subject: fmt.Sprintf("Context%d", i), GroundingStatus: GroundingGrounded})
	}
	evidence = append(evidence, source.EvidenceItems...)
	mu.AppendEvidence(evidence)
	mu.AppendDispatchToolResult(ToolResult{ToolName: "read_file", Success: true,
		ReadCoverage: &ToolReadCoverage{Path: "src/worker.go", LineStart: 1, LineEnd: 20, TotalLines: 20}})
	mu.SetInvestigationAggregateFacts([]AnswerAggregateFact{fact})
	mu.RetainInvestigationAggregateFacts()
	ir := &AnalysisIR{RequestModel: rm}
	bus := &BusContext{AnalysisIR: ir, Mutable: mu, EvidenceItems: evidence}
	agent := &AgentContext{AnalysisIR: ir, Mutable: mu, EvidenceItems: evidence[:1]}
	for name, input := range map[string]ObservationLedgerInput{
		"bus-limited":   ObservationLedgerInputFromBusContext(bus, 64),
		"agent-limited": ObservationLedgerInputFromAgentContext(agent, 64),
		"tool-full":     AnswerAggregateSourceContextFromBusContext(bus),
	} {
		t.Run(name, func(t *testing.T) {
			if !AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &rm, input) ||
				!aggregateFactHasIndependentTypedAuthorityWithSourceContext(fact, &rm, input) {
				t.Fatal("display-limited consumer lost full observed source membership")
			}
			ledger := CompileObservationLedger(input)
			record := findObservationRecord(t, ledger, "aggregate:0#current_source")
			if record.ClaimAuthority != ObservationClaimAuthorityIndependentlyProven {
				t.Fatalf("aggregate ledger qualification diverged: %+v", record)
			}
		})
	}
	for _, plan := range []*AnswerSurfacePlan{BuildAnswerSurfacePlanForBusContext(bus), BuildAnswerSurfacePlanForAgentContext(agent)} {
		plan.SurfaceEvidence = nil // A display/facet selection is not a proof-pool edit.
		sets := CompileEnumerationDisplaySets(&rm, plan)
		if len(sets) != 1 || !enumerationDisplaySetAuthorizesPrincipalContract(&rm, fact, sets[0], plan.aggregateSourceClaims) {
			t.Fatal("display selection removed observed member qualification")
		}
		clone := cloneAnswerSurfacePlan(plan)
		clone.aggregateSourceClaims.items[0].Subject = "changed clone"
		if plan.aggregateSourceClaims.items[0].Subject == "changed clone" {
			t.Fatal("plan clone shared mutable proof context")
		}
	}
}

func TestB1700ReadCoverageSupportsCoordinatesNotInventedLabels(t *testing.T) {
	fact, rm, _ := b1700SourceMemberFixture()
	input := ObservationLedgerInput{ToolResults: []ToolResult{{ToolName: "read_file", Success: true,
		ReadCoverage: &ToolReadCoverage{Path: "src/worker.go", LineStart: 1, LineEnd: 20, TotalLines: 20}}}}
	if AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &rm, input) {
		t.Fatal("read coverage alone declared a source member label")
	}
	fact.Members[0] = "src/worker.go:12"
	if !AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &rm, input) {
		t.Fatal("actual source coordinate member lost read coverage proof")
	}
	input.ToolResults[0].Success = false
	if AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &rm, input) {
		t.Fatal("failed read minted coordinate membership")
	}
}

func TestB1700BranchEffectKeepsOriginalClaimFormInSourceSnapshot(t *testing.T) {
	fact, rm, input := b1700SourceMemberFixture()
	item := &input.EvidenceItems[0]
	item.Kind, item.AnchorKind = EvidenceControlFlow, AnchorCall
	item.Producer, item.Predicate = EvidenceProducerDataflowLowererPrefix+"go", ControlFlowPredicateConsequence
	item.Subject, item.Object, item.LineEnd = "if ready", "Dispatch", item.LineStart
	fact.Members[0] = item.Subject
	if ClaimFormOf(*item) != ClaimBranchEffect {
		t.Fatal("fixture must carry a parser-owned branch effect")
	}
	if AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &rm, input) ||
		aggregateFactHasIndependentTypedAuthorityWithSourceContext(fact, &rm, input) {
		t.Fatal("source snapshot changed branch-effect ownership into a call-member claim")
	}
}

func TestB1700NativeInventoryNamesAreIndependentMemberWitnesses(t *testing.T) {
	for _, test := range []struct {
		name   string
		active bool
		state  SourceInventoryCoverageState
		member string
		want   bool
		edit   func(*AnswerAggregateFact, *SourceInventoryObservation)
	}{
		{"observed-partial", true, SourceInventoryCoverageObserved, "Worker", true, nil},
		{"inactive", false, SourceInventoryCoverageObserved, "Worker", false, nil},
		{"ambiguous", true, SourceInventoryCoverageAmbiguous, "Worker", false, nil},
		{"unobserved", true, "", "Worker", false, nil},
		{"pending-read", true, SourceInventoryCoverageNeedsRead, "Worker", false, nil},
		{"unrelated-claim", true, SourceInventoryCoverageObserved, "Worker owns runtime outcome", false, nil},
		{"wrong-file-same-name", true, SourceInventoryCoverageObserved, "Worker", false, func(_ *AnswerAggregateFact, i *SourceInventoryObservation) {
			i.Sets[0].Members[0].File = "other/worker.go"
		}},
		{"wrong-line", true, SourceInventoryCoverageObserved, "Worker", false, func(_ *AnswerAggregateFact, i *SourceInventoryObservation) {
			i.Sets[0].Members[0].Line = 13
		}},
		{"missing-name", true, SourceInventoryCoverageObserved, "Worker", false, func(_ *AnswerAggregateFact, i *SourceInventoryObservation) {
			i.Sets[0].Members[0].Name = ""
		}},
		{"partial-missing-member", true, SourceInventoryCoverageObserved, "Worker", false, func(f *AnswerAggregateFact, _ *SourceInventoryObservation) {
			f.Members = append(f.Members, "Other")
		}},
		{"complete-missing-member", true, SourceInventoryCoverageObserved, "Worker", false, func(f *AnswerAggregateFact, i *SourceInventoryObservation) {
			f.Members = append(f.Members, "Other")
			i.Complete = true
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fact, rm, _ := b1700SourceMemberFixture()
			fact.Members[0] = test.member
			inventory := SourceInventoryObservation{Active: test.active, Sets: []SourceInventoryObservationSet{{
				Role: AnswerCandidateRoleType, Members: []SourceInventoryObservationMember{{Name: "Worker", File: "src/worker.go", Line: 12,
					CoverageState: test.state, Note: "Worker owns runtime outcome", SurfaceTerms: []string{"Worker owns runtime outcome"}}},
			}}}
			if test.edit != nil {
				test.edit(&fact, &inventory)
			}
			for _, lane := range []string{"direct", "successful-tool", "failed-tool"} {
				t.Run(lane, func(t *testing.T) {
					input := ObservationLedgerInput{SourceInventoryObservation: inventory}
					if lane != "direct" {
						input = ObservationLedgerInput{ToolResults: []ToolResult{{Success: lane == "successful-tool", SourceInventory: &inventory}}}
					}
					want := test.want && lane != "failed-tool"
					before, _ := json.Marshal(input)
					if got := AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &rm, input); got != want {
						t.Fatalf("native inventory qualification=%v want=%v", got, want)
					}
					if got := aggregateFactHasIndependentTypedAuthorityWithSourceContext(fact, &rm, input); got != want {
						t.Fatalf("native inventory independent proof=%v want=%v", got, want)
					}
					after, _ := json.Marshal(input)
					if string(before) != string(after) || len(input.EvidenceItems) != 0 {
						t.Fatal("native inventory qualification synthesized or changed source evidence")
					}
				})
			}
		})
	}
}

func TestB1700SourceSymbolRequiresExactDefinitionAnchor(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*ObservationLedgerInput, *string, *string, *int)
		want bool
	}{
		{"definition", nil, true},
		{"qualified-definition", func(s *ObservationLedgerInput, name, _ *string, _ *int) {
			s.EvidenceItems[0].AnchorSymbol, s.EvidenceItems[0].Subject = "Worker", "pkg.Worker"
			*name = "pkg.Worker"
		}, true},
		{"local-definition", func(s *ObservationLedgerInput, _, _ *string, _ *int) {
			s.EvidenceItems[0].AnchorSymbol, s.EvidenceItems[0].Subject = "Worker", "pkg.Worker"
		}, true},
		{"wrong-qualified-owner", func(s *ObservationLedgerInput, name, _ *string, _ *int) {
			s.EvidenceItems[0].AnchorSymbol, s.EvidenceItems[0].Subject = "Worker", "pkg.Worker"
			*name = "other.Worker"
		}, false},
		{"call-site", func(s *ObservationLedgerInput, _, _ *string, _ *int) { s.EvidenceItems[0].AnchorKind = AnchorCall }, false},
		{"inside-definition-range", func(s *ObservationLedgerInput, _, _ *string, line *int) {
			s.EvidenceItems[0].LineEnd, *line = 20, 14
		}, false},
		{"different-file-same-name", func(_ *ObservationLedgerInput, _, file *string, _ *int) { *file = "other/worker.go" }, false},
		{"read-without-identity", func(s *ObservationLedgerInput, _, _ *string, _ *int) {
			s.EvidenceItems = nil
			s.ToolResults = []ToolResult{{ToolName: "read_file", Success: true, ReadCoverage: &ToolReadCoverage{Path: "src/worker.go", LineStart: 1, LineEnd: 20, TotalLines: 20}}}
		}, false},
		{"source-excluded", func(s *ObservationLedgerInput, _, _ *string, _ *int) {
			s.RequestModel.ExternalObservationPolicy = &ExternalObservationPolicy{CurrentSourceMode: ExternalObservationCurrentSourceExclude,
				ExclusionKind: ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"typed source exclusion"}}
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fact, rm, source := b1700SourceMemberFixture()
			source.RequestModel = &rm
			name, file, line := "Worker", "src/worker.go", 12
			if test.edit != nil {
				test.edit(&source, &name, &file, &line)
			}
			if got := AnswerSourceSymbolDefinitionObserved(name, file, line, source); got != test.want {
				t.Fatalf("source symbol definition admission=%v want=%v", got, test.want)
			}
			if test.name == "call-site" && !AnswerAggregateFactHasObservedSourceMembers(fact, source) {
				t.Fatal("narrow symbol admission must not change ordinary observed call-member eligibility")
			}
		})
	}
}

func TestB1700SourceSymbolInventoryRetainsNonIdentifierAnchors(t *testing.T) {
	for _, name := range []string{"http.port", "GET /health", "ready state"} {
		source := ObservationLedgerInput{SourceInventoryObservation: SourceInventoryObservation{Active: true,
			Sets: []SourceInventoryObservationSet{{Members: []SourceInventoryObservationMember{{
				Name: name, File: "src/config.go", Line: 7, CoverageState: SourceInventoryCoverageObserved,
			}}}}}}
		if !AnswerSourceSymbolDefinitionObserved(name, "src/config.go", 7, source) {
			t.Fatalf("observed native inventory identity %q was rejected merely for non-identifier shape", name)
		}
	}
}

func TestB1700SourceSymbolAndAutomaticCitationStayInTheirContext(t *testing.T) {
	fact, rm, source := b1700SourceMemberFixture()
	observed := AnswerAggregateSourceContextFromBusContext(&BusContext{AnalysisIR: &AnalysisIR{RequestModel: rm},
		EvidenceItems: source.EvidenceItems, Mutable: NewMutableState("observed source")})
	empty := AnswerAggregateSourceContextFromAgentContext(&AgentContext{AnalysisIR: &AnalysisIR{RequestModel: rm},
		Mutable: NewMutableState("independent source context")})
	if !AnswerSourceSymbolDefinitionObserved("Worker", "src/worker.go", 12, observed) || !AnswerAggregateFactHasObservedSourceMembers(fact, observed) {
		t.Fatal("observed context lost its source identity")
	}
	if AnswerSourceSymbolDefinitionObserved("Worker", "src/worker.go", 12, empty) || AnswerAggregateFactHasObservedSourceMembers(fact, empty) {
		t.Fatal("an independent context inherited another context's source identity")
	}
	fact.Provenance = SourceInventoryPrincipalRowSetAggregateProvenance
	fact.Dimensions = []AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}}
	if AnswerAggregateFactHasObservedSourceMembers(fact, empty) {
		t.Fatal("origin or marker minted automatic source citation eligibility")
	}
	fact.Members[0] = "Ghost"
	if AnswerAggregateFactHasObservedSourceMembers(fact, observed) {
		t.Fatal("observed coordinates proved an unrelated automatic citation member")
	}
	fact.Members[0] = "Worker"
	observed.RequestModel.ExternalObservationPolicy = &ExternalObservationPolicy{CurrentSourceMode: ExternalObservationCurrentSourceExclude,
		ExclusionKind: ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"typed source exclusion"}}
	if AnswerAggregateFactHasObservedSourceMembers(fact, observed) {
		t.Fatal("source exclusion lost to native provenance or external support")
	}
}

func b1700ContextFreeAuthorityCalls(file *ast.File) []token.Pos {
	var positions []token.Pos
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		var name string
		switch function := call.Fun.(type) {
		case *ast.Ident:
			name = function.Name
		case *ast.SelectorExpr:
			name = function.Sel.Name
		}
		if name == "AnswerAggregateFactAuthorizesPrincipalContract" || name == "EnumerationDisplaySetAuthorizesPrincipalContract" {
			positions = append(positions, call.Pos())
		}
		return true
	})
	return positions
}

func TestB1700ProductionAggregateAuthorityRequiresObservationContext(t *testing.T) {
	for _, directory := range []string{".", "../agent", "../tool"} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			set := token.NewFileSet()
			file, err := parser.ParseFile(set, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, position := range b1700ContextFreeAuthorityCalls(file) {
				t.Errorf("production authority bypasses observed context at %s", set.Position(position))
			}
		}
	}
	file, err := parser.ParseFile(token.NewFileSet(), "self_red.go", "package p; func f(){ types.AnswerAggregateFactAuthorizesPrincipalContract(fact, rm); EnumerationDisplaySetAuthorizesPrincipalContract(rm, fact, set) }", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(b1700ContextFreeAuthorityCalls(file)); got != 2 {
		t.Fatalf("census self-RED detected %d bypasses, want 2", got)
	}
}

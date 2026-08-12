// Package tool — emit_investigation_complete_principal_span_test.go
// (2026-05-12).
//
// Mirrors emit_investigation_complete_waiver_test.go for the typed
// principal_span_waiver escape: invalid optional payloads are ignored
// and never honored, valid payloads store on acceptance, clear+set
// remains a precise hard conflict, and an active waiver bypasses the
// callChainPrincipalSpanDowngrade gate.
package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func orderedCallChainEndpoints(source, sink string) *types.CallChainEndpointProfile {
	return &types.CallChainEndpointProfile{Source: source, Sink: sink}
}

func TestCallChainCodeTermMatchesUsesTypedQualifiedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		candidate string
		endpoint  string
		want      bool
	}{
		{candidate: "pkg::Gate::Run", endpoint: "pkg.Gate.Run", want: true},
		{candidate: "Run", endpoint: "pkg::Gate::Run", want: true},
		{candidate: "other::Gate::Run", endpoint: "pkg.Gate.Run", want: false},
		{candidate: "pkg::Gate::RunWith", endpoint: "pkg.Gate.Run", want: false},
		{candidate: "prefixpkg::Gate::Runsuffix", endpoint: "pkg.Gate.Run", want: false},
	} {
		if got := callChainCodeTermMatches(tc.candidate, tc.endpoint); got != tc.want {
			t.Errorf("callChainCodeTermMatches(%q,%q)=%t want %t", tc.candidate, tc.endpoint, got, tc.want)
		}
	}
}

func TestCallChainDirectedPathStatus_AllExecutableLanguageSurfaces(t *testing.T) {
	tests := []struct {
		name   string
		source string
		start  string
		middle string
		end    string
	}{
		{"go", "main.go", "app.Start", "svc.Step", "gate.Run"},
		{"java", "Main.java", "Controller.start", "Service.step", "Gate.run"},
		{"kotlin", "Main.kt", "Controller.start", "Service.step", "Gate.run"},
		{"c", "main.c", "app_start", "service_step", "gate_run"},
		{"cpp", "main.cc", "App::Start", "Service::Step", "Gate::Run"},
		{"rust", "main.rs", "app::start", "service::step", "gate::run"},
		{"python", "main.py", "App.start", "Service.step", "Gate.run"},
		{"javascript", "main.js", "App.start", "Service.step", "Gate.run"},
		{"typescript", "main.ts", "App.start", "Service.step", "Gate.run"},
		{"ruby", "main.rb", "App.start", "Service.step", "Gate.run"},
		{"swift", "Main.swift", "App.start", "Service.step", "Gate.run"},
		{"lua", "main.lua", "App.start", "Service.step", "Gate.run"},
		{"proto", "main.proto", "App.Start", "Service.Step", "Gate.Run"},
		{"arkts", "main.ets", "App.start", "Service.step", "Gate.run"},
		{"cangjie", "main.cj", "App.start", "Service.step", "Gate.run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := []types.EvidenceItem{
				{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: tt.start, Predicate: "calls", Object: tt.middle, AnchorSymbol: tt.middle, Source: tt.source, LineStart: 10, GroundingStatus: types.GroundingGrounded},
				{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: tt.middle, Predicate: "calls", Object: tt.end, AnchorSymbol: tt.end, Source: tt.source, LineStart: 20, GroundingStatus: types.GroundingGrounded},
			}
			status, edges := callChainDirectedPathStatusForEvidence(evidence, tt.start, tt.end)
			if edges != 2 || !status.StartResolved || !status.EndResolved || len(status.Path) != 3 {
				t.Fatalf("status=%+v edges=%d, want resolved 3-node directed path", status, edges)
			}
		})
	}
}

func TestCallChainDirectedPathStatus_DefinitionAndPrefixSiblingDoNotReachExactSink(t *testing.T) {
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Predicate: "calls", Object: "gate.RunWith", AnchorSymbol: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2666, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, Subject: "gate.Run", AnchorSymbol: "Run", Source: "internal/analysis/gate/gate.go", LineStart: 134, GroundingStatus: types.GroundingGrounded},
	}
	status, edges := callChainDirectedPathStatusForEvidence(evidence, "buildAnalysisIR", "gate.Run")
	if edges != 1 || !status.StartResolved || status.EndResolved || len(status.Path) != 0 {
		t.Fatalf("status=%+v edges=%d, definition/prefix sibling must not mint exact sink reachability", status, edges)
	}
}

func TestCallChainDirectedPathStatus_AmbiguousShortEndpointsRequireEveryCandidateCovered(t *testing.T) {
	covered := []types.EvidenceItem{
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "A.Start", Predicate: "calls", Object: "A.Run", Source: "a.go", LineStart: 10, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "B.Start", Predicate: "calls", Object: "B.Run", Source: "b.go", LineStart: 10, GroundingStatus: types.GroundingGrounded},
	}
	status, _ := callChainDirectedPathStatusForEvidence(covered, "Start", "Run")
	if !status.StartResolved || !status.EndResolved || !status.StartAmbiguous || !status.EndAmbiguous || len(status.Path) == 0 {
		t.Fatalf("all ambiguous candidates are covered by typed paths: %+v", status)
	}
	partial := append([]types.EvidenceItem(nil), covered[:1]...)
	partial = append(partial,
		types.EvidenceItem{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "B.Start", Predicate: "calls", Object: "Dead.end", Source: "b.go", LineStart: 10, GroundingStatus: types.GroundingGrounded},
		types.EvidenceItem{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "Other.start", Predicate: "calls", Object: "B.Run", Source: "b.go", LineStart: 20, GroundingStatus: types.GroundingGrounded},
	)
	status, _ = callChainDirectedPathStatusForEvidence(partial, "Start", "Run")
	if !status.StartResolved || !status.EndResolved || !status.StartAmbiguous || !status.EndAmbiguous || len(status.Path) != 0 {
		t.Fatalf("partially covered ambiguity must be represented, not called absent or proven: %+v", status)
	}
}

func TestPreCompleteCallChain_UnorderedEntityPairCannotCreateDirection(t *testing.T) {
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Predicate: "calls", Object: "gate.RunWith", AnchorSymbol: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2666, GroundingStatus: types.GroundingGrounded},
	}
	mut := types.NewMutableState("opaque request bytes are not consulted")
	mut.AppendEvidence(evidence)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentTrace,
		PredicateAxis: types.AxisCall,
		Predicates:    types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			Kind:              string(types.ReqCallChain),
			MentionedEntities: []string{"buildAnalysisIR", "gate.Run", "analyzer.go", "orchestrator.go"},
		},
	}}}

	if completionExactCallChainEndpointShape(ctx) {
		t.Fatal("precondition: analyzer intentionally omitted ExactTargets")
	}
	if completionDirectedCallChainEndpointShape(ctx) {
		t.Fatal("two entity identities remain unordered and must not activate a directed hard gate")
	}
	got := preCompleteContractCheckWithEvidence(ctx, "", evidence)
	if strings.Contains(got, "not directionally proven") {
		t.Fatalf("unordered entity mention order must not drive reachability, got:\n%s", got)
	}
	ctx.AnalysisIR.RequestModel.CallChainEndpointProfile = orderedCallChainEndpoints("buildAnalysisIR", "gate.Run")
	if !completionDirectedCallChainEndpointShape(ctx) {
		t.Fatal("typed ordered source/sink profile should activate directed reachability")
	}
	got = preCompleteContractCheckWithEvidence(ctx, "", evidence)
	if !strings.Contains(got, "not directionally proven") || !strings.Contains(got, "`buildAnalysisIR` -> `gate.Run`") {
		t.Fatalf("typed ordered endpoint profile must retain exact direction, got:\n%s", got)
	}
}

func TestCompletionDirectedCallChainEndpointShape_AmbiguousFallbackStaysAdvisory(t *testing.T) {
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		PredicateAxis: types.AxisCall,
		Predicates:    types.SemanticPredicates{IsRelationalLookup: true},
		AnalyzerHints: types.AnalyzerHints{
			Kind:              string(types.ReqCallChain),
			MentionedEntities: []string{"source.Start", "possible.Middle", "sink.Run"},
		},
	}}}
	if completionDirectedCallChainEndpointShape(ctx) {
		t.Fatal("three fallback symbols do not establish an unambiguous source/sink pair")
	}
}

func TestEmitInvestigationComplete_NoDirectedPathWaiverCarriesModelOwnedBoundary(t *testing.T) {
	mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
	mut.AppendEvidence([]types.EvidenceItem{
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Predicate: "calls", Object: "gate.RunWith", AnchorSymbol: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2666, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "gate.Run", Predicate: "calls", Object: "gate.RunWith", AnchorSymbol: "gate.RunWith", Source: "internal/analysis/gate/gate.go", LineStart: 135, GroundingStatus: types.GroundingGrounded},
	})
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/agent/analyzer.go":     true,
		"internal/analysis/gate/gate.go": true,
	})
	bus := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:                   types.IntentTrace,
			PredicateAxis:            types.AxisCall,
			CallChainEndpointProfile: orderedCallChainEndpoints("buildAnalysisIR", "gate.Run"),
			AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"buildAnalysisIR", "gate.Run"}, MentionedEntities: []string{"buildAnalysisIR", "gate.Run"}},
		},
	}}
	const stalePathReason = "STALE_PATH_REASON_buildAnalysisIR_reaches_gate_Run"
	const auditOnlyRationale = "AUDIT_ONLY_RATIONALE_gate_RunWith_is_outside_buildAnalysisIR"
	params := json.RawMessage(`{"reason":"` + stalePathReason + `","confidence":"high","result_kind":"resolved","principal_span_waiver":{"reason":"no_directed_path","rationale":"` + auditOnlyRationale + `"}}`)
	res, err := (&EmitInvestigationComplete{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.Success || !mut.IsInvestigationComplete() {
		t.Fatalf("typed no_directed_path boundary should close model-owned investigation: %+v", res)
	}
	for _, want := range []string{"principal_span_waiver=no_directed_path", "Show a reverse/parallel relationship only when its own typed call edge is present", "do not turn endpoint definitions into a call edge"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q: %s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, stalePathReason) {
		t.Fatalf("tool handoff must not replay a model-authored path claim beside the typed no-path boundary: %s", res.Summary)
	}
	if strings.Contains(res.Summary, auditOnlyRationale) {
		t.Fatalf("model-authored waiver rationale must remain audit-only: %s", res.Summary)
	}
	if got := mut.StableInvestigationCompleteReason(); got != stalePathReason {
		t.Fatalf("raw model closure reason must remain available for audit/resume, got %q", got)
	}
	if got := mut.PrincipalSpanWaiver(); got == nil || got.Rationale != auditOnlyRationale {
		t.Fatalf("raw waiver rationale must remain available in audit state, got %+v", got)
	}
	caveats := mut.EvidenceClosure().CompletionCaveats()
	if len(caveats) == 0 || caveats[len(caveats)-1].ReasonCode != "call_chain_no_directed_path" {
		t.Fatalf("typed completion caveat missing: %+v", caveats)
	}
}

func TestEmitInvestigationComplete_NoDirectedPathWaiverRequiresExactEndpointExistence(t *testing.T) {
	mut := types.NewMutableState("opaque request bytes are not consulted")
	mut.AppendEvidence([]types.EvidenceItem{
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2666, GroundingStatus: types.GroundingGrounded},
	})
	bus := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
		CallChainEndpointProfile: orderedCallChainEndpoints("buildAnalysisIR", "gate.Run"),
		AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"buildAnalysisIR", "gate.Run"}},
	}}}
	params := json.RawMessage(`{"reason":"source inspection found no path","confidence":"high","result_kind":"resolved","aggregate_facts":[{"kind":"member_set","role":"principal_answer","label":"nearest proven path","members":["buildAnalysisIR","gate.RunWith"],"support_refs":["internal/agent/analyzer.go:2666","internal/agent/analyzer.go:2666"]}],"principal_span_waiver":{"reason":"no_directed_path","rationale":"the collected graph reaches only gate.RunWith"}}`)
	res, err := (&EmitInvestigationComplete{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.Success || mut.IsInvestigationComplete() {
		t.Fatalf("missing exact sink proof must keep investigation open: %+v", res)
	}
	for _, want := range []string{"lacks exact endpoint existence proof", "`gate.Run`", "prefix sibling"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("downgrade missing %q: %s", want, res.Summary)
		}
	}
	repairs := mut.EvidenceClosure().ActiveRepairs()
	if len(repairs) == 0 || repairs[len(repairs)-1].Origin != "pre_complete.call_chain_no_path_endpoint_existence" {
		t.Fatalf("typed endpoint repair missing: %+v", repairs)
	}

	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition,
		// Grounded emitters may retain the enclosing package as Subject while
		// the visible definition anchor stays local. Unique current-source scope
		// must qualify that anchor without minting a call edge.
		Subject: "gate", AnchorSymbol: "Run", Source: "internal/analysis/gate/gate.go",
		LineStart: 134, GroundingStatus: types.GroundingGrounded,
	}})
	mut.EvidenceClosure().SetReadSet(map[string]bool{"internal/agent/analyzer.go": true})
	mut.EvidenceClosure().SetReadRanges(map[string][]types.LineRange{
		"internal/agent/analyzer.go": {{Start: 2600, End: 2750}},
	})
	res, err = (&EmitInvestigationComplete{}).Execute(bus, params)
	if err != nil || !res.Success || mut.IsInvestigationComplete() {
		t.Fatalf("existence without direct endpoint-body inspection must keep the negative topology boundary open: res=%+v err=%v", res, err)
	}
	for _, want := range []string{"lacks direct endpoint-body inspection", "`gate.Run`"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("body-read downgrade missing %q: %s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "`buildAnalysisIR`, `gate.Run`") {
		t.Fatalf("already-read caller body must not be re-requested: %s", res.Summary)
	}
	pending := mut.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].File != "internal/analysis/gate/gate.go" || len(pending[0].LineRanges) != 1 {
		t.Fatalf("production-shaped body repair must request one bounded sink file range: %+v", pending)
	}
	if got := pending[0].LineRanges[0]; got.Start != 134 || got.End != 150 {
		t.Fatalf("without a parser graph, endpoint repair must use the bounded definition fallback [134,150], got %+v", got)
	}
	mut.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
		"internal/analysis/gate/gate.go": {
			RelPath: "internal/analysis/gate/gate.go", Language: "go", Package: "gate",
			Symbols: []repotypes.Symbol{
				{Name: "Run", Kind: "function", File: "internal/analysis/gate/gate.go", Line: 134, EndLine: 136},
				{Name: "RunWith", Kind: "function", File: "internal/analysis/gate/gate.go", Line: 143, EndLine: 200},
			},
			Relations: []repotypes.Relation{{
				Kind: "call", File: "internal/analysis/gate/gate.go", Line: 135,
				ToEP: repotypes.RelationEndpoint{Name: "RunWith"}, Confidence: repotypes.ConfidenceAST,
				Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_ast_call",
			}},
		},
	}})
	if start, end := callChainEndpointBodyReadRange(bus, "internal/analysis/gate/gate.go", 134, "gate.Run"); start != 134 || end != 136 {
		t.Fatalf("parser-owned symbol span must narrow the endpoint read to Run's exact body, got [%d,%d]", start, end)
	}
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/agent/analyzer.go":     true,
		"internal/analysis/gate/gate.go": true,
	})
	mut.EvidenceClosure().SetReadRanges(map[string][]types.LineRange{
		"internal/agent/analyzer.go":     {{Start: 1, End: 3000}},
		"internal/analysis/gate/gate.go": {{Start: 1, End: 220}},
	})
	mut.EvidenceClosure().DrainSatisfiedPendingReads()
	res, err = (&EmitInvestigationComplete{}).Execute(bus, params)
	if err != nil || !res.Success || !mut.IsInvestigationComplete() {
		t.Fatalf("both exact endpoint bodies read with no directed path should retain model-owned typed escape: res=%+v err=%v", res, err)
	}
	if !callChainTypedEvidenceContainsExactParserRelation(mut.EmittedEvidence(), "internal/analysis/gate/gate.go", 135, "gate.Run", "gate.RunWith") {
		t.Fatalf("accepted boundary must carry the already-read reverse/parallel wrapper edge into typed evidence: %+v", mut.EmittedEvidence())
	}
}

func TestCallChainExactEndpointBodyInspected_AllExecutableLanguageSurfaces(t *testing.T) {
	tests := []struct {
		name, file, endpoint, local string
	}{
		{"go", "internal/gate/gate.go", "gate.Run", "Run"},
		{"java", "src/Gate.java", "Gate.run", "run"},
		{"kotlin", "src/Gate.kt", "Gate.run", "run"},
		{"c", "src/gate.c", "gate_run", "gate_run"},
		{"cpp", "src/Gate.cc", "Gate::Run", "Run"},
		{"rust", "src/gate.rs", "gate::run", "run"},
		{"python", "src/gate.py", "gate.run", "run"},
		{"javascript", "src/gate.js", "gate.run", "run"},
		{"typescript", "src/gate.ts", "gate.run", "run"},
		{"ruby", "lib/Gate.rb", "Gate#run", "run"},
		{"swift", "Sources/Gate.swift", "Gate.run", "run"},
		{"lua", "src/gate.lua", "gate.run", "run"},
		{"arkts", "entry/src/gate.ets", "gate.run", "run"},
		{"cangjie", "src/gate/entry.cj", "gate.run", "run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			closure := types.NewEvidenceClosure("")
			closure.SetReadSet(map[string]bool{tt.file: true})
			closure.SetReadRanges(map[string][]types.LineRange{tt.file: {{Start: 20, End: 24}}})
			evidence := []types.EvidenceItem{{
				Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition,
				Subject: tt.local, AnchorSymbol: tt.local, Source: tt.file, LineStart: 20,
				GroundingStatus: types.GroundingGrounded,
			}}
			if !callChainExactEndpointBodyInspected(closure, evidence, tt.endpoint) {
				t.Fatalf("read exact definition did not prove endpoint body inspection for %q", tt.endpoint)
			}
			closure.SetReadRanges(map[string][]types.LineRange{tt.file: {{Start: 21, End: 24}}})
			if callChainExactEndpointBodyInspected(closure, evidence, tt.endpoint) {
				t.Fatalf("definition outside read range must not prove endpoint body inspection for %q", tt.endpoint)
			}
		})
	}
}

func TestCallChainExactEndpointBodyInspected_InboundEdgeDoesNotProveCalleeBody(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"src/source.go": true})
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		Subject: "source.Run", Object: "sink.Run", AnchorSymbol: "sink.Run",
		Source: "src/source.go", LineStart: 30, GroundingStatus: types.GroundingGrounded,
	}}
	if callChainExactEndpointBodyInspected(closure, evidence, "sink.Run") {
		t.Fatal("an inbound callsite is in the caller body and must not prove callee-body inspection")
	}
	if !callChainExactEndpointBodyInspected(closure, evidence, "source.Run") {
		t.Fatal("an already-read outbound callsite should prove caller-body inspection")
	}
}

func TestEmitInvestigationComplete_ExactEndpointGateNeverConvergesAway(t *testing.T) {
	SetDowngradeConvergenceHardThreshold(2)
	defer SetDowngradeConvergenceHardThreshold(0)

	mut := types.NewMutableState("opaque request bytes are not consulted")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		Subject: "buildAnalysisIR", Object: "gate.RunWith",
		Source: "internal/agent/analyzer.go", LineStart: 2666,
		GroundingStatus: types.GroundingGrounded,
	}})
	bus := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
		CallChainEndpointProfile: orderedCallChainEndpoints("buildAnalysisIR", "gate.Run"),
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqCallChain), ExactTargets: []string{"buildAnalysisIR", "gate.Run"},
		},
	}}}
	params := json.RawMessage(`{"reason":"investigation complete","confidence":"high","result_kind":"resolved"}`)
	tool := &EmitInvestigationComplete{}
	for attempt := 1; attempt <= 6; attempt++ {
		res, err := tool.Execute(bus, params)
		if err != nil {
			t.Fatalf("attempt %d Execute error: %v", attempt, err)
		}
		if !res.Success || mut.IsInvestigationComplete() {
			t.Fatalf("attempt %d: retry count must not erase exact endpoint authority: %+v", attempt, res)
		}
		if !strings.Contains(res.Summary, "not directionally proven") {
			t.Fatalf("attempt %d: precise downgrade missing: %s", attempt, res.Summary)
		}
	}
	repairs := mut.EvidenceClosure().ActiveRepairs()
	if len(repairs) == 0 {
		t.Fatal("exact endpoint repair missing")
	}
	wantTools := []string{"repo_map", "grep", "read_file", "emit_evidence"}
	if got := types.RepairDirectiveRequiredTools(repairs[0]); !reflect.DeepEqual(got, wantTools) {
		t.Fatalf("exact endpoint repair tools=%v, want %v", got, wantTools)
	}
}

func TestEmitInvestigationComplete_NonBoundaryWaiverCannotBypassMissingDirectedPath(t *testing.T) {
	mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
	mut.AppendEvidence([]types.EvidenceItem{
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Predicate: "calls", Object: "gate.RunWith", AnchorSymbol: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2666, GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "gate.Run", Predicate: "calls", Object: "gate.RunWith", AnchorSymbol: "gate.RunWith", Source: "internal/analysis/gate/gate.go", LineStart: 135, GroundingStatus: types.GroundingGrounded},
	})
	bus := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:                   types.IntentTrace,
			PredicateAxis:            types.AxisCall,
			CallChainEndpointProfile: orderedCallChainEndpoints("buildAnalysisIR", "gate.Run"),
			AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"buildAnalysisIR", "gate.Run"}, MentionedEntities: []string{"buildAnalysisIR", "gate.Run"}},
		},
	}}
	params := json.RawMessage(`{"reason":"the reachable sibling is adjacent","confidence":"high","result_kind":"resolved","principal_span_waiver":{"reason":"endpoints_directly_adjacent","rationale":"buildAnalysisIR directly invokes gate.RunWith on one statement"}}`)
	res, err := (&EmitInvestigationComplete{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.Success || mut.IsInvestigationComplete() {
		t.Fatalf("adjacency to a sibling must not close the missing exact endpoint path: %+v", res)
	}
	for _, want := range []string{"principal_span_waiver=endpoints_directly_adjacent cannot waive", "`gate.Run` from `buildAnalysisIR`", "principal_span_waiver.reason=no_directed_path", "reachable sibling"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("rejection missing %q: %s", want, res.Summary)
		}
	}
}

func TestEmitInvestigationComplete_PrincipalSpanWaiver_IgnoresInvalidReason(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	params := `{
		"reason":"investigation done",
		"confidence":"high",
		"result_kind":"resolved",
		"principal_span_waiver":{"reason":"not_a_real_reason","rationale":"x"}
	}`
	res := runEIC(t, mut, params)
	if !res.Success {
		t.Fatalf("invalid optional waiver should be ignored, not reject; Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored principal_span_waiver.reason") {
		t.Errorf("summary must audit ignored optional waiver: %q", res.Summary)
	}
	if mut.PrincipalSpanWaiver() != nil {
		t.Errorf("ignored waiver must NOT be stored")
	}
}

func TestEmitInvestigationComplete_PrincipalSpanWaiver_IgnoresBlankRationale(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"principal_span_waiver":{"reason":"endpoints_directly_adjacent","rationale":"   "}
	}`
	res := runEIC(t, mut, params)
	if !res.Success {
		t.Fatalf("blank optional waiver rationale should be ignored, not reject; Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored principal_span_waiver=endpoints_directly_adjacent because rationale is missing") {
		t.Errorf("summary must audit ignored optional waiver: %q", res.Summary)
	}
	if mut.PrincipalSpanWaiver() != nil {
		t.Errorf("ignored waiver must NOT be stored")
	}
}

func TestEmitInvestigationComplete_PrincipalSpanWaiver_IgnoresMissingReason(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"principal_span_waiver":{"rationale":"intermediate is plumbing only"}
	}`
	res := runEIC(t, mut, params)
	if !res.Success {
		t.Fatalf("missing optional waiver reason should be ignored, not reject; Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored principal_span_waiver because reason is missing") {
		t.Errorf("summary must audit ignored optional waiver: %q", res.Summary)
	}
	if mut.PrincipalSpanWaiver() != nil {
		t.Errorf("ignored waiver must NOT be stored")
	}
}

func TestEmitInvestigationComplete_PrincipalSpanWaiver_AcceptsAllReasons(t *testing.T) {
	for _, reason := range types.PrincipalSpanWaiverReasonValues() {
		t.Run(string(reason), func(t *testing.T) {
			mut := types.NewMutableState("trace foo to bar")
			params := `{
				"reason":"done",
				"confidence":"high",
				"result_kind":"resolved",
				"principal_span_waiver":{"reason":"` + string(reason) + `","rationale":"concrete one-sentence reason"}
			}`
			res := runEIC(t, mut, params)
			if !res.Success {
				// Other completion gates may still fail this — but a
				// gate failure must NOT be the typed-waiver validation.
				// Surface the offending summary so we can audit.
				if strings.Contains(res.Summary, "principal_span_waiver") {
					t.Fatalf("typed reason %q should pass schema validation; got %q", reason, res.Summary)
				}
			}
			stored := mut.PrincipalSpanWaiver()
			if stored == nil || stored.Reason != reason {
				t.Errorf("stored waiver mismatch: want reason=%q got=%+v", reason, stored)
			}
		})
	}
}

func TestEmitInvestigationComplete_PrincipalSpanWaiver_ClearRejectsWithNewWaiver(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"principal_span_waiver":{"reason":"inlined_call","rationale":"r"},
		"clear_principal_span_waiver":true
	}`
	res := runEIC(t, mut, params)
	if res.Success {
		t.Fatalf("clear+set combo must reject")
	}
	if !strings.Contains(res.Summary, "clear_principal_span_waiver") {
		t.Errorf("rejection must name the conflicting flags: %q", res.Summary)
	}
}

func TestEmitInvestigationComplete_PrincipalSpanWaiver_ClearRetracts(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	mut.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
		Reason:    types.PrincipalSpanWaiverEndpointsDirectlyAdjacent,
		Rationale: "prior",
	})
	if mut.PrincipalSpanWaiver() == nil {
		t.Fatalf("pre-state: waiver should be present")
	}
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"clear_principal_span_waiver":true
	}`
	_ = runEIC(t, mut, params)
	if mut.PrincipalSpanWaiver() != nil {
		t.Errorf("clear must retract: still have %+v", mut.PrincipalSpanWaiver())
	}
}

func TestPrincipalSpanWaiver_BypassesGate(t *testing.T) {
	// Manufacture the gate's preconditions: IntentTrace + two endpoint
	// entities + emitted evidence at source and sink with no
	// intermediate item. Without a waiver the gate must reject; with
	// an active waiver the gate must pass through.
	// Span must exceed the gate's minSpanForLateCoverageGate (120 lines)
	// so the hard gate is actually exercised — anything shorter is
	// treated as a compact straight-line helper that does not need
	// intermediate evidence in the first place.
	makeCtx := func() *types.BusContext {
		mut := types.NewMutableState("trace foo to bar")
		mut.AppendEvidence([]types.EvidenceItem{
			{
				ID:         "ev1",
				Kind:       types.EvidenceDirect,
				AnchorKind: types.AnchorCall,
				Subject:    "foo",
				Object:     "internal",
				Source:     "x.go",
				LineStart:  10,
				LineEnd:    10,
			},
			{
				ID:         "ev2",
				Kind:       types.EvidenceDirect,
				AnchorKind: types.AnchorCall,
				Subject:    "internal",
				Object:     "bar",
				Source:     "x.go",
				LineStart:  300,
				LineEnd:    300,
			},
		})
		return &types.BusContext{
			Mutable: mut,
			AnalysisIR: &types.AnalysisIR{
				RequestModel: types.RequestModel{
					RawRequest:               "trace foo to bar",
					Intent:                   types.IntentTrace,
					CallChainEndpointProfile: orderedCallChainEndpoints("foo", "bar"),
					AnalyzerHints: types.AnalyzerHints{
						MentionedEntities: []string{"foo", "bar"},
					},
				},
			},
		}
	}

	t.Run("no waiver - gate fires", func(t *testing.T) {
		ctx := makeCtx()
		closure := &types.EvidenceClosure{}
		got := callChainPrincipalSpanDowngrade(ctx, closure)
		if got == "" {
			t.Fatalf("gate should fire when no intermediate evidence and no waiver")
		}
		if !strings.Contains(got, "principal span") && !strings.Contains(got, "intermediate evidence") {
			t.Errorf("downgrade message should describe the span gap: %q", got)
		}
	})

	t.Run("active waiver - gate bypasses", func(t *testing.T) {
		ctx := makeCtx()
		ctx.Mutable.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
			Reason:    types.PrincipalSpanWaiverNoIntermediateUserCode,
			Rationale: "between foo and bar are nil-check plus logger setup",
		})
		closure := &types.EvidenceClosure{}
		got := callChainPrincipalSpanDowngrade(ctx, closure)
		if got != "" {
			t.Errorf("active waiver must bypass the gate; got downgrade=%q", got)
		}
	})

	t.Run("waiver with empty rationale - gate still fires", func(t *testing.T) {
		ctx := makeCtx()
		ctx.Mutable.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
			Reason:    types.PrincipalSpanWaiverInlinedCall,
			Rationale: "",
		})
		closure := &types.EvidenceClosure{}
		got := callChainPrincipalSpanDowngrade(ctx, closure)
		if got == "" {
			t.Fatalf("waiver with empty rationale must NOT bypass (IsActive()=false)")
		}
	})

	t.Run("waiver with invalid reason - gate still fires", func(t *testing.T) {
		ctx := makeCtx()
		ctx.Mutable.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
			Reason:    "made_up_reason",
			Rationale: "trying to sneak past",
		})
		closure := &types.EvidenceClosure{}
		got := callChainPrincipalSpanDowngrade(ctx, closure)
		if got == "" {
			t.Fatalf("waiver with invalid reason must NOT bypass (IsActive()=false)")
		}
	})
}

func TestQueueProactiveCallChainClosureRepairs_QueuesPrincipalSpanBeforeCompletion(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:         "source",
			Kind:       types.EvidenceDirect,
			AnchorKind: types.AnchorCall,
			Subject:    "foo",
			Object:     "handoff",
			Source:     "x.go",
			LineStart:  10,
		},
		{
			ID:         "sink",
			Kind:       types.EvidenceDirect,
			AnchorKind: types.AnchorCall,
			Subject:    "handoff",
			Object:     "bar",
			Source:     "x.go",
			LineStart:  300,
		},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest:               "trace foo to bar",
				Intent:                   types.IntentTrace,
				CallChainEndpointProfile: orderedCallChainEndpoints("foo", "bar"),
				AnalyzerHints: types.AnalyzerHints{
					Kind:              string(types.ReqCallChain),
					MentionedEntities: []string{"foo", "bar"},
				},
			},
		},
	}

	if !QueueProactiveCallChainClosureRepairs(ctx) {
		t.Fatal("expected proactive call-chain span repair to queue")
	}
	repairs := mut.EvidenceClosure().ActiveRepairs()
	if len(repairs) == 0 || repairs[0].Kind != types.RepairReadFile {
		t.Fatalf("active repairs = %+v, want read_file repair", repairs)
	}
	if len(repairs[0].Files) != 1 || repairs[0].Files[0] != "x.go" {
		t.Fatalf("repair files = %+v, want x.go", repairs[0].Files)
	}
	if len(repairs[0].LineRanges) != 1 || repairs[0].LineRanges[0].Start <= 10 || repairs[0].LineRanges[0].End >= 300 {
		t.Fatalf("repair line ranges = %+v, want interior source→sink span", repairs[0].LineRanges)
	}
	pending := mut.EvidenceClosure().PendingReads()
	if len(pending) != 1 || len(pending[0].LineRanges) != 1 {
		t.Fatalf("pending read should carry the surgical range too, got %+v", pending)
	}
}

func TestQueueProactiveCallChainClosureRepairs_AlreadyReadSpanQueuesEmitEvidence(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "source", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, Subject: "foo", Object: "handoff", Source: "x.go", LineStart: 10},
		{ID: "sink", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, Subject: "handoff", Object: "bar", Source: "x.go", LineStart: 300},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"x.go": true})
	closure.SetReadRanges(map[string][]types.LineRange{"x.go": {{Start: 1, End: 300}}})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest:               "trace foo to bar",
				Intent:                   types.IntentTrace,
				CallChainEndpointProfile: orderedCallChainEndpoints("foo", "bar"),
				AnalyzerHints: types.AnalyzerHints{
					Kind:              string(types.ReqCallChain),
					MentionedEntities: []string{"foo", "bar"},
				},
			},
		},
	}

	if !QueueProactiveCallChainClosureRepairs(ctx) {
		t.Fatal("expected proactive already-read call-chain span repair to queue")
	}
	repairs := closure.ActiveRepairs()
	if len(repairs) != 1 || repairs[0].Kind != types.RepairEmitEvidence {
		t.Fatalf("active repairs = %+v, want emit_evidence repair", repairs)
	}
	if len(closure.PendingReads()) != 0 {
		t.Fatalf("already-read span should not queue pending reads, got %+v", closure.PendingReads())
	}
}

func TestPartitionPendingReadsForAcceptedClosure_PrincipalSpanWaiverAdvisesStaleSpanRead(t *testing.T) {
	mut := types.NewMutableState("trace foo to bar")
	mut.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
		Reason:    types.PrincipalSpanWaiverNoIntermediateUserCode,
		Rationale: "the pointer dispatch has no source-visible intermediate user-code node",
	})
	ctx := &types.BusContext{Mutable: mut}
	// The counter-example rides a citation-class origin: after the §29.60
	// forced-read split, coverage-class origins (primary_anchor, phase1,
	// chain_promotion) demote unconditionally at completion, so the
	// "must keep blocking" control needs an origin the typed routing still
	// treats as load-bearing (grounder-reject / unknown origins).
	pending := []types.PendingRead{
		{File: "x.go", Origin: "auto_bridge.pre_complete.call_chain_principal_span", LineRanges: []types.LineRange{{Start: 50, End: 80}}},
		{File: "y.go", Origin: "finalizer_grounder_reject"},
	}

	blocking, advisory := partitionPendingReadsForAcceptedClosure(ctx, pending, nil, nil)
	if len(advisory) != 1 || advisory[0].File != "x.go" {
		t.Fatalf("principal-span waiver should demote only stale span pending read to advisory, got blocking=%+v advisory=%+v", blocking, advisory)
	}
	if len(blocking) != 1 || blocking[0].File != "y.go" {
		t.Fatalf("citation-class pending read must keep blocking, got blocking=%+v advisory=%+v", blocking, advisory)
	}
}

func TestCallChainPrincipalSpanDemand_IgnoresStaticBindingEndpoint(t *testing.T) {
	evidence := []types.EvidenceItem{
		{
			ID:              "impl",
			Kind:            types.EvidenceDirect,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "HashImpl",
			Subject:         "HashImpl",
			Source:          "x.c",
			LineStart:       100,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "reg",
			Kind:            types.EvidenceRegistration,
			AnchorKind:      types.AnchorInitializer,
			AnchorSymbol:    "ops_table",
			Subject:         "ops_table",
			Predicate:       "registers",
			Object:          "HashImpl",
			Source:          "x.c",
			LineStart:       300,
			GroundingStatus: types.GroundingGrounded,
		},
	}
	if demand, ok := callChainPrincipalSpanDemandForEvidence(evidence, "HashImpl", "ops_table"); ok {
		t.Fatalf("static registration binding must not synthesize a source-to-sink span demand: %+v", demand)
	}
}

func TestCallChainQualifiedIntermediateDowngrade_RequiresTypedHandoffForReadQualifiedCalls(t *testing.T) {
	mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:           "start",
			Kind:         types.EvidenceDirect,
			AnchorKind:   types.AnchorDefinition,
			AnchorSymbol: "buildAnalysisIR",
			Subject:      "buildAnalysisIR",
			Source:       "internal/agent/analyzer.go",
			LineStart:    1289,
		},
		{
			ID:           "compile",
			Kind:         types.EvidenceDirect,
			AnchorKind:   types.AnchorCall,
			AnchorSymbol: "compiler.Compile",
			Subject:      "buildAnalysisIR",
			Object:       "compiler.Compile",
			Source:       "internal/agent/analyzer.go",
			LineStart:    1842,
		},
		{
			ID:           "bind",
			Kind:         types.EvidenceDirect,
			AnchorKind:   types.AnchorCall,
			AnchorSymbol: "binder.BindByRelevance",
			Subject:      "buildAnalysisIR",
			Object:       "binder.BindByRelevance",
			Source:       "internal/agent/analyzer.go",
			LineStart:    1868,
		},
		{
			ID:           "gate",
			Kind:         types.EvidenceDirect,
			AnchorKind:   types.AnchorCall,
			AnchorSymbol: "gate.RunWith",
			Subject:      "buildAnalysisIR",
			Object:       "gate.RunWith",
			Source:       "internal/agent/analyzer.go",
			LineStart:    1994,
		},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest:               "trace buildAnalysisIR to gate.Run",
				Intent:                   types.IntentTrace,
				CallChainEndpointProfile: orderedCallChainEndpoints("buildAnalysisIR", "gate.RunWith"),
				AnalyzerHints: types.AnalyzerHints{
					MentionedEntities: []string{"buildAnalysisIR", "gate.RunWith"},
				},
			},
		},
	}
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 1842,
		"out := compiler.Compile(rm, sig)",
		"if out.AnswerContract.Language == \"\" {",
		"out.AnswerContract.Language = rm.Language",
		"}",
		"",
		"// Risk matrix and hypothesis planning.",
		"rm.RiskMatrix = risk.Evaluate(rm, rm.RiskMatrix)",
		"hypotheses := hdp.Plan(rm)",
		"",
		"// Recompute budget with the real hypothesis count.",
		"sig.HypothesisCount = len(hypotheses)",
		"compiler.RecomputeBudget(&out, rm, sig)",
		"",
		"// Amplifier post-compile pass.",
		"for _, obs := range amplifier.AmplifyPostCompile(rm, &out.AnswerContract) {",
		"recordReconcileObservation(ctxMutable(ctx), reconcileEvent(",
		"obs.Field, obs.Before, obs.After, 0, obs.Reason, rm.Predicates,",
		"))",
		"}",
		"",
		"// Relevance-based hypothesis binding.",
		"if err := binder.BindByRelevance(&out.TaskGraph, hypotheses, binder.Options{}); err != nil {",
		"return nil, fmt.Errorf(\"binder: %w\", err)",
		"}",
	)

	got := callChainQualifiedIntermediateDowngrade(ctx, &types.EvidenceClosure{})
	if got == "" {
		t.Fatalf("qualified intermediate gate should require typed handoff for already-read calls")
	}
	for _, want := range []string{"risk.Evaluate", "hdp.Plan", "compiler.RecomputeBudget", "amplifier.AmplifyPostCompile"} {
		if !strings.Contains(got, want) {
			t.Fatalf("downgrade missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "fmt.Errorf") {
		t.Fatalf("error-return helper should not be promoted as a principal intermediate:\n%s", got)
	}
}

func TestCallChainQualifiedIntermediateDowngrade_PassesWhenCandidatesAreTyped(t *testing.T) {
	mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
	base := []types.EvidenceItem{
		{ID: "start", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR", Subject: "buildAnalysisIR", Source: "internal/agent/analyzer.go", LineStart: 1289},
		{ID: "compile", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "compiler.Compile", Subject: "buildAnalysisIR", Object: "compiler.Compile", Source: "internal/agent/analyzer.go", LineStart: 1842},
		{ID: "risk", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "risk.Evaluate", Subject: "buildAnalysisIR", Object: "risk.Evaluate", Source: "internal/agent/analyzer.go", LineStart: 1848},
		{ID: "hdp", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "hdp.Plan", Subject: "buildAnalysisIR", Object: "hdp.Plan", Source: "internal/agent/analyzer.go", LineStart: 1849},
		{ID: "budget", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "compiler.RecomputeBudget", Subject: "buildAnalysisIR", Object: "compiler.RecomputeBudget", Source: "internal/agent/analyzer.go", LineStart: 1853},
		{ID: "post", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "amplifier.AmplifyPostCompile", Subject: "buildAnalysisIR", Object: "amplifier.AmplifyPostCompile", Source: "internal/agent/analyzer.go", LineStart: 1861},
		{ID: "bind", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "binder.BindByRelevance", Subject: "buildAnalysisIR", Object: "binder.BindByRelevance", Source: "internal/agent/analyzer.go", LineStart: 1868},
		{ID: "gate", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith", Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 1994},
	}
	mut.AppendEvidence(base)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest:               "trace buildAnalysisIR to gate.Run",
				Intent:                   types.IntentTrace,
				CallChainEndpointProfile: orderedCallChainEndpoints("buildAnalysisIR", "gate.RunWith"),
				AnalyzerHints: types.AnalyzerHints{
					MentionedEntities: []string{"buildAnalysisIR", "gate.RunWith"},
				},
			},
		},
	}
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 1848,
		"rm.RiskMatrix = risk.Evaluate(rm, rm.RiskMatrix)",
		"hypotheses := hdp.Plan(rm)",
		"sig.HypothesisCount = len(hypotheses)",
		"compiler.RecomputeBudget(&out, rm, sig)",
		"for _, obs := range amplifier.AmplifyPostCompile(rm, &out.AnswerContract) {",
		"}",
	)

	if got := callChainQualifiedIntermediateDowngrade(ctx, &types.EvidenceClosure{}); got != "" {
		t.Fatalf("typed qualified candidates should satisfy the gate, got:\n%s", got)
	}
}

func TestCallChainQualifiedIntermediateDowngrade_DenseCoverageKeepsCandidatesAdvisory(t *testing.T) {
	mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
	evidence := []types.EvidenceItem{
		{ID: "start", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR", Subject: "buildAnalysisIR", Source: "internal/agent/analyzer.go", LineStart: 100},
		{ID: "a", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "compiler.Compile", Subject: "buildAnalysisIR", Object: "compiler.Compile", Source: "internal/agent/analyzer.go", LineStart: 120},
		{ID: "b", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "risk.Evaluate", Subject: "buildAnalysisIR", Object: "risk.Evaluate", Source: "internal/agent/analyzer.go", LineStart: 140},
		{ID: "c", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "hdp.Plan", Subject: "buildAnalysisIR", Object: "hdp.Plan", Source: "internal/agent/analyzer.go", LineStart: 160},
		{ID: "d", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "amplifier.AmplifyPostCompile", Subject: "buildAnalysisIR", Object: "amplifier.AmplifyPostCompile", Source: "internal/agent/analyzer.go", LineStart: 180},
		{ID: "e", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "binder.BindByRelevance", Subject: "buildAnalysisIR", Object: "binder.BindByRelevance", Source: "internal/agent/analyzer.go", LineStart: 200},
		{ID: "f", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "analyzerRequiredFiles", Subject: "buildAnalysisIR", Object: "analyzerRequiredFiles", Source: "internal/agent/analyzer.go", LineStart: 220},
		{ID: "gate", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith", Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 240},
	}
	mut.AppendEvidence(evidence)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest:               "trace buildAnalysisIR to gate.Run",
				Intent:                   types.IntentTrace,
				CallChainEndpointProfile: orderedCallChainEndpoints("buildAnalysisIR", "gate.RunWith"),
				AnalyzerHints: types.AnalyzerHints{
					MentionedEntities: []string{"buildAnalysisIR", "gate.RunWith"},
				},
			},
		},
	}
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 120,
		"out := compiler.Compile(rm, sig)",
		"ctx.Mutable.RequestModel()",
		"rm.RiskMatrix = risk.Evaluate(rm, rm.RiskMatrix)",
		"hypotheses := hdp.Plan(rm)",
		"for _, obs := range amplifier.AmplifyPostCompile(rm, &out.AnswerContract) {",
		"if err := binder.BindByRelevance(&out.TaskGraph, hypotheses, binder.Options{}); err != nil {",
		"required := analyzerRequiredFiles(rm)",
		"ir.QualityGate = gate.RunWith(ir, gate.GlobalThresholds(), mode, gate.RunOptions{Resolver: resolver})",
	)

	closure := &types.EvidenceClosure{}
	if got := callChainQualifiedIntermediateDowngrade(ctx, closure); got != "" {
		t.Fatalf("dense structured call-chain coverage should keep residual qualified calls advisory, got:\n%s", got)
	}
	for _, repair := range closure.ActiveRepairs() {
		if repair.Origin == "pre_complete.call_chain_qualified_intermediate" {
			t.Fatalf("dense residual qualified calls must not become an active/blocking repair, got %+v", closure.ActiveRepairs())
		}
	}
	var found bool
	for _, repair := range closure.PendingRepairs() {
		if repair.Origin == "pre_complete.call_chain_qualified_intermediate" && repair.Kind == types.RepairEmitEvidence && repair.Advisory {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("residual qualified calls should remain visible as advisory repair, got %+v", closure.PendingRepairs())
	}
}

func TestCallChainReadParserRelationHandoffEvidence_AlreadyReadRosterEdgeIsProjected(t *testing.T) {
	evidence := []types.EvidenceItem{
		{ID: "main-run", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "main", Predicate: "calls", Object: "run", AnchorSymbol: "run", Source: "src/main.rs", LineStart: 10, GroundingStatus: types.GroundingGrounded},
		{ID: "run-collect", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "run", Predicate: "calls", Object: "walker::collect_files", AnchorSymbol: "walker::collect_files", Source: "src/main.rs", LineStart: 20, GroundingStatus: types.GroundingGrounded},
		{ID: "run-index", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "run", Predicate: "calls", Object: "index_file", AnchorSymbol: "index_file", Source: "src/main.rs", LineStart: 23, GroundingStatus: types.GroundingGrounded},
		{ID: "index-match", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "index_file", Predicate: "calls", Object: "Matcher::is_match", AnchorSymbol: "Matcher::is_match", Source: "src/main.rs", LineStart: 30, GroundingStatus: types.GroundingGrounded},
		{ID: "collect-def", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "collect_files", Subject: "collect_files", Source: "src/walker.rs", LineStart: 4, GroundingStatus: types.GroundingGrounded},
		{ID: "walk-def", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "walk", Subject: "walk", Source: "src/walker.rs", LineStart: 10, GroundingStatus: types.GroundingGrounded},
	}
	mut := types.NewMutableState("opaque call-chain request")
	mut.AppendEvidence(evidence)
	mut.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
		"src/walker.rs": {
			RelPath: "src/walker.rs", Language: "rust", Package: "walker",
			Symbols: []repotypes.Symbol{
				{Name: "collect_files", Kind: "function", File: "src/walker.rs", Line: 4, EndLine: 8},
				{Name: "walk", Kind: "function", File: "src/walker.rs", Line: 10, EndLine: 30},
			},
			Relations: []repotypes.Relation{{Kind: "call", File: "src/walker.rs", Line: 6, ToEP: repotypes.RelationEndpoint{Name: "walk"}, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "rust_ast_call"}},
		},
	}})
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Complexity: types.ComplexityComplex, PredicateAxis: types.AxisCall,
		Predicates:               types.SemanticPredicates{IsCrossComponent: true},
		CallChainEndpointProfile: orderedCallChainEndpoints("main", "Matcher::is_match"),
		AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"main", "Matcher::is_match"}, PrimaryEntities: []string{"main", "Matcher::is_match"}},
	}}}
	facts := []types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "cross-module call-chain nodes", Role: types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"main", "run", "walker::collect_files", "walk", "index_file", "Matcher::is_match"},
		SupportRefs: []string{"src/main.rs:10", "src/main.rs:20", "src/main.rs:20", "src/walker.rs:10", "src/main.rs:23", "src/main.rs:30"},
	}}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"src/walker.rs": true})
	mut.EvidenceClosure().SetReadSet(map[string]bool{"src/walker.rs": true})

	got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence)
	if !callChainTypedEvidenceContainsExactParserRelation(got, "src/walker.rs", 6, "walker::collect_files", "walk") {
		t.Fatalf("already-read AST call between principal roster members must be projected: %+v", got)
	}
	foundProjection := false
	for _, item := range got {
		if item.Source == "src/walker.rs" && item.LineStart == 6 &&
			item.Producer == types.EvidenceProducerRepoMapPrincipalMemberCall && item.IsCitable() {
			foundProjection = true
		}
	}
	if !foundProjection {
		t.Fatalf("missing exact parser projection carrier: %+v", got)
	}
	for _, repair := range closure.ActiveRepairs() {
		if repair.Origin == "pre_complete.call_chain_read_parser_relation_handoff" {
			t.Fatalf("exact parser projection must not force the model to re-emit evidence: %+v", closure.ActiveRepairs())
		}
	}
	seedReadFileHistory(ctx, "src/walker.rs", 4,
		"pub fn collect_files(root: &str) -> Vec<String> {",
		"let mut out = Vec::new();",
		"walk(root, &mut out);",
		"out",
		"}",
	)
	refreshClosureReadSnapshot(ctx, mut.EvidenceClosure())
	refreshed := callChainReadParserRelationHandoffEvidence(ctx, mut.EvidenceClosure(), facts, evidence)
	if !callChainTypedEvidenceContainsExactParserRelation(refreshed, "src/walker.rs", 6, "walker::collect_files", "walk") {
		t.Fatalf("refreshed production closure should retain the projected parser relation: %+v", refreshed)
	}
	view := buildCompletionPreflightView(ctx, "", "", evidence, facts, nil)
	if got := preCompleteContractCheckWithEvidence(ctx, "", evidence, facts); strings.Contains(got, "already-read parser call relations lack typed handoff") {
		t.Fatalf("pre-complete must consume exact parser projection without a model copy loop; effective_facts=%+v got:\n%s", view.EffectiveAggregateFacts, got)
	}

	withEdge := append(append([]types.EvidenceItem(nil), evidence...), types.EvidenceItem{
		ID: "collect-walk", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		Subject: "walker::collect_files", Predicate: "calls", Object: "walk", AnchorSymbol: "walk",
		Source: "src/walker.rs", LineStart: 6, GroundingStatus: types.GroundingGrounded,
	})
	before := len(mut.EmittedEvidence())
	projected := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, withEdge)
	if after := len(mut.EmittedEvidence()); after != before {
		t.Fatalf("equivalent citable call edge must not append another system projection: before=%d after=%d", before, after)
	}
	projectedRows := 0
	for _, item := range projected {
		if item.Producer == types.EvidenceProducerRepoMapPrincipalMemberCall {
			projectedRows++
		}
	}
	if projectedRows != 1 {
		t.Fatalf("existing parser projection should remain single-copy in effective evidence: %+v", projected)
	}
}

func TestCallChainReadParserRelationHandoffEvidence_SameNameWrapperCoreUsesExactMemberLocations(t *testing.T) {
	evidence := []types.EvidenceItem{
		{ID: "python-wrapper", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "FastTokenizer.tokenize", Predicate: "calls", Object: "_fastlex.tokenize_bytes", AnchorSymbol: "_fastlex.tokenize_bytes", Source: "bindings-py/fastlex/tokenizer.py", LineStart: 21, GroundingStatus: types.GroundingGrounded},
		{ID: "wrapper-def", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, Subject: "#[pyfunction] tokenize_bytes", AnchorSymbol: "tokenize_bytes", Source: "core-rs/src/lib.rs", LineStart: 40, GroundingStatus: types.GroundingGrounded},
		{ID: "core-def", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, Subject: "pub fn tokenize_bytes", AnchorSymbol: "tokenize_bytes", Source: "core-rs/src/lib.rs", LineStart: 10, GroundingStatus: types.GroundingGrounded},
	}
	core := &repotypes.Symbol{Name: "tokenize_bytes", Kind: "function", File: "src/lib.rs", Line: 10, EndLine: 18}
	wrapper := repotypes.Symbol{Name: "tokenize_bytes", Kind: "function", Parent: "py", File: "src/lib.rs", Line: 40, EndLine: 43}
	fi := &repotypes.FileInfo{
		RelPath: "src/lib.rs", Language: repotypes.LangRust,
		Symbols: []repotypes.Symbol{*core, wrapper},
		Relations: []repotypes.Relation{{
			Kind: "call", File: "src/lib.rs", Line: 42,
			FromEP:     repotypes.RelationEndpoint{Name: "tokenize_bytes", Receiver: "py", File: "src/lib.rs", Line: 42},
			ToEP:       repotypes.RelationEndpoint{Name: "tokenize_bytes", Receiver: "super", File: "src/lib.rs", Line: 42},
			Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "rust_ast_scoped_call",
		}},
	}
	graph := &repotypes.Graph{
		FileIndex: map[string]*repotypes.FileInfo{"src/lib.rs": fi},
		MethodIndex: map[repotypes.MethodKey]*repotypes.Symbol{
			{Pkg: "src", Receiver: "", Name: "tokenize_bytes"}: core,
		},
	}
	mut := types.NewMutableState("opaque cross-language call-chain request")
	mut.AppendEvidence(evidence)
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Complexity: types.ComplexityComplex, PredicateAxis: types.AxisCall,
		Predicates:               types.SemanticPredicates{IsCrossComponent: true},
		CallChainEndpointProfile: orderedCallChainEndpoints("FastTokenizer.tokenize", "tokenize_bytes"),
		AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), PrimaryEntities: []string{"FastTokenizer.tokenize", "tokenize_bytes"}},
	}}}
	facts := []types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "complete cross-language chain", Role: types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"FastTokenizer.tokenize", "_fastlex.tokenize_bytes", "#[pyfunction] tokenize_bytes", "#[pymodule] _fastlex", "pub fn tokenize_bytes", "best_merge"},
		SupportRefs: []string{"bindings-py/fastlex/tokenizer.py:18", "bindings-py/fastlex/tokenizer.py:21", "core-rs/src/lib.rs:40", "core-rs/src/lib.rs:46", "core-rs/src/lib.rs:10", "core-rs/src/lib.rs:20"},
	}}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"src/lib.rs": true})

	got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence)
	if !callChainTypedEvidenceContainsExactParserRelation(got, "src/lib.rs", 42, "py.tokenize_bytes", "tokenize_bytes") {
		t.Fatalf("same-name wrapper/core parser edge must use exact member support locations: %+v", got)
	}

	// A location collision is not uniquely resolvable and must remain closed.
	badRefs := append([]string(nil), facts[0].SupportRefs...)
	badRefs[4] = "core-rs/src/lib.rs:40"
	facts[0].SupportRefs = badRefs
	mut2 := types.NewMutableState("opaque ambiguous cross-language call-chain request")
	mut2.AppendEvidence(evidence)
	mut2.SetSearchGraph(graph)
	ctx.Mutable = mut2
	if ambiguous := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence); len(ambiguous) != len(evidence) {
		t.Fatalf("same-name roles sharing one location must remain ambiguous: %+v", ambiguous)
	}
}

func TestCallChainReadParserRelationHandoffEvidence_MissingMemberSupportCannotHideASTEdge(t *testing.T) {
	evidence := []types.EvidenceItem{{
		ID: "dispatch-def", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition,
		AnchorSymbol: "HttpTransport.dispatchOnce", Subject: "HttpTransport.dispatchOnce",
		Source: "packages/core/src/transport.ts", LineStart: 39, GroundingStatus: types.GroundingGrounded,
	}}
	mut := types.NewMutableState("opaque complete call-chain request")
	mut.AppendEvidence(evidence)
	mut.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
		"packages/core/src/transport.ts": {
			RelPath: "packages/core/src/transport.ts", Language: "typescript",
			Symbols: []repotypes.Symbol{{Name: "HttpTransport.dispatchOnce", Kind: "method", File: "packages/core/src/transport.ts", Line: 39, EndLine: 43}},
			Relations: []repotypes.Relation{{
				Kind: "call", File: "packages/core/src/transport.ts", Line: 40,
				ToEP: repotypes.RelationEndpoint{Name: "fetch"}, Confidence: repotypes.ConfidenceAST,
				Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "typescript_ast_call",
			}},
		},
	}})
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Complexity: types.ComplexityComplex, PredicateAxis: types.AxisCall,
		CallChainEndpointProfile: orderedCallChainEndpoints("HttpTransport.dispatchOnce", "fetch"),
		AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), PrimaryEntities: []string{"HttpTransport.dispatchOnce", "fetch"}},
	}}}
	facts := []types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "complete request path", Role: types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"HttpTransport.dispatchOnce", "fetch (Node.js native)"},
		// Deliberately omit support for the decorated fetch member. Requiring
		// all-member support here would make the missing call edge hide itself.
		SupportRefs: []string{"packages/core/src/transport.ts:39"},
	}}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"packages/core/src/transport.ts": true})

	got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence)
	if !callChainTypedEvidenceContainsExactParserRelation(got, "packages/core/src/transport.ts", 40, "HttpTransport.dispatchOnce", "fetch") {
		t.Fatalf("unsupported decorated member must not hide its already-read AST edge: %+v", got)
	}
}

func TestCallChainReadParserRelationHandoffEvidence_NoDirectedPathProjectsExactBoundaryWrapperEdge(t *testing.T) {
	evidence := []types.EvidenceItem{
		{ID: "source-def", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, Subject: "buildAnalysisIR", AnchorSymbol: "buildAnalysisIR", Source: "internal/agent/analyzer.go", LineStart: 1876, GroundingStatus: types.GroundingGrounded},
		{ID: "source-runwith", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "buildAnalysisIR", Predicate: "calls", Object: "gate.RunWith", AnchorSymbol: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 2720, GroundingStatus: types.GroundingGrounded},
		{ID: "sink-def", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, Subject: "gate.Run", AnchorSymbol: "gate.Run", Source: "internal/analysis/gate/gate.go", LineStart: 134, GroundingStatus: types.GroundingGrounded},
	}
	makeCtx := func(withBoundary bool, provenance string, member string) (*types.BusContext, *types.EvidenceClosure, []types.AnswerAggregateFact) {
		mut := types.NewMutableState("opaque exact call-chain request")
		mut.AppendEvidence(evidence)
		if withBoundary {
			mut.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
				Reason: types.PrincipalSpanWaiverNoDirectedPath, Rationale: "the requested sink is a wrapper around the reachable sibling",
			})
		}
		mut.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
			"internal/analysis/gate/gate.go": {
				RelPath: "internal/analysis/gate/gate.go", Language: "go", Package: "gate",
				Symbols: []repotypes.Symbol{
					{Name: "Run", Kind: "function", File: "internal/analysis/gate/gate.go", Line: 134, EndLine: 136},
					{Name: "RunWith", Kind: "function", File: "internal/analysis/gate/gate.go", Line: 143, EndLine: 200},
				},
				Relations: []repotypes.Relation{{
					Kind: "call", File: "internal/analysis/gate/gate.go", Line: 135,
					ToEP: repotypes.RelationEndpoint{Name: "RunWith"}, Confidence: repotypes.ConfidenceAST,
					Provenance: provenance, ResolvedBy: "go_ast_call",
				}},
			},
		}})
		ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
			CallChainEndpointProfile: orderedCallChainEndpoints("buildAnalysisIR", "gate.Run"),
			AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"buildAnalysisIR", "gate.Run"}},
		}}}
		closure := types.NewEvidenceClosure("")
		closure.SetReadSet(map[string]bool{"internal/analysis/gate/gate.go": true})
		facts := []types.AnswerAggregateFact{{
			Kind: types.AnswerAggregateMemberSet, Label: "nearest proven path", Role: types.AnswerAggregateRolePrincipalAnswer,
			Members:     []string{"buildAnalysisIR", member},
			SupportRefs: []string{"internal/agent/analyzer.go:1876", "internal/agent/analyzer.go:2720"},
		}}
		return ctx, closure, facts
	}

	ctx, closure, facts := makeCtx(true, repotypes.ProvenanceTreeSitter, "gate.RunWith")
	got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence)
	if !callChainTypedEvidenceContainsExactParserRelation(got, "internal/analysis/gate/gate.go", 135, "gate.Run", "gate.RunWith") {
		t.Fatalf("typed no-path boundary must carry the already-read exact wrapper direction into finalizer evidence: %+v", got)
	}

	// The principal aggregate may enumerate only intraprocedural work inside
	// the source and omit the terminal sibling itself. The exact source still
	// reaches RunWith through accepted typed evidence, so an already-read AST
	// edge from the exact requested sink back to that reachable peer remains a
	// precise parallel-convergence fact. It must not depend on roster spelling.
	ctx, closure, facts = makeCtx(true, repotypes.ProvenanceTreeSitter, "binder.BindByRelevance")
	got = callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence)
	if !callChainTypedEvidenceContainsExactParserRelation(got, "internal/analysis/gate/gate.go", 135, "gate.Run", "gate.RunWith") {
		t.Fatalf("typed no-path boundary must carry exact requested-sink -> source-reachable peer even when peer is absent from the roster: %+v", got)
	}

	ctx, closure, facts = makeCtx(false, repotypes.ProvenanceTreeSitter, "gate.RunWith")
	if got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence); len(got) != len(evidence) {
		t.Fatalf("endpoint-to-roster expansion must be inactive without a typed no_directed_path boundary: %+v", got)
	}
	ctx, closure, facts = makeCtx(true, repotypes.ProvenanceRegexFallback, "gate.RunWith")
	if got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence); len(got) != len(evidence) {
		t.Fatalf("regex boundary relation must not acquire typed authority: %+v", got)
	}
	ctx, closure, facts = makeCtx(true, repotypes.ProvenanceTreeSitter, "other.Member")
	if got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence); !callChainTypedEvidenceContainsExactParserRelation(got, "internal/analysis/gate/gate.go", 135, "gate.Run", "gate.RunWith") {
		t.Fatalf("requested-sink call to a source-reachable peer must not depend on unrelated roster spelling: %+v", got)
	}
	if callChainNoDirectedPathReachableBoundaryPeerRelation(true, "buildAnalysisIR", "gate.Run", "gate.Run", "unrelated.Helper", evidence) {
		t.Fatal("requested sink must not pull an arbitrary endpoint-body call whose callee is outside the typed source-reachable graph")
	}
	if callChainNoDirectedPathReachableBoundaryPeerRelation(true, "buildAnalysisIR", "gate.Run", "gate.RunWith", "gate.Run", evidence) {
		t.Fatal("peer -> requested sink direction must never be auto-carried after a typed no_directed_path boundary")
	}
}

func TestCallChainReadParserRelationHandoffEvidence_ExactStatementSiblingUnderCompleteness(t *testing.T) {
	evidence := []types.EvidenceItem{{
		ID: "delay-call", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		Subject: "HttpTransport.send", Predicate: "calls", Object: "this.retry.nextDelay", AnchorSymbol: "this.retry.nextDelay",
		Source: "packages/core/src/transport.ts", LineStart: 29, GroundingStatus: types.GroundingGrounded,
	}}
	mut := types.NewMutableState("opaque complete call-chain request")
	mut.AppendEvidence(evidence)
	mut.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
		"packages/core/src/transport.ts": {
			RelPath: "packages/core/src/transport.ts", Language: "typescript",
			Symbols: []repotypes.Symbol{{Name: "HttpTransport.send", Kind: "method", File: "packages/core/src/transport.ts", Line: 18, EndLine: 32}},
			Relations: []repotypes.Relation{
				{Kind: "call", File: "packages/core/src/transport.ts", Line: 29, ToEP: repotypes.RelationEndpoint{Name: "this.retry.nextDelay"}, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "typescript_ast_call"},
				{Kind: "call", File: "packages/core/src/transport.ts", Line: 29, ToEP: repotypes.RelationEndpoint{Name: "sleep"}, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "typescript_ast_call"},
			},
		},
	}})
	makeCtx := func(complete bool) *types.BusContext {
		rm := types.RequestModel{
			Intent: types.IntentTrace, Complexity: types.ComplexityComplex, PredicateAxis: types.AxisCall,
			CallChainEndpointProfile: orderedCallChainEndpoints("HttpTransport.send", "HttpTransport.dispatchOnce"),
			AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), PrimaryEntities: []string{"HttpTransport.send", "HttpTransport.dispatchOnce"}},
		}
		if complete {
			rm.CompletenessObligation = &types.CompletenessObligation{Required: true, SourceQuote: "complete call chain"}
		}
		return &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	}
	facts := []types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "request path", Role: types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"HttpTransport.send", "HttpTransport.dispatchOnce"},
		SupportRefs: []string{"packages/core/src/transport.ts:18", "packages/core/src/transport.ts:23"},
	}}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"packages/core/src/transport.ts": true})

	got := callChainReadParserRelationHandoffEvidence(makeCtx(true), closure, facts, evidence)
	if !callChainTypedEvidenceContainsExactParserRelation(got, "packages/core/src/transport.ts", 29, "HttpTransport.send", "sleep") {
		t.Fatalf("typed sibling call on the same exact statement must be projected under completeness: %+v", got)
	}
	projectedDelay := 0
	for _, item := range got {
		if item.Producer == types.EvidenceProducerRepoMapPrincipalMemberCall &&
			types.CallChainEndpointCompatible(item.Object, "this.retry.nextDelay") {
			projectedDelay++
		}
	}
	if projectedDelay != 0 {
		t.Fatalf("already-emitted exact call must not be projected again: %+v", got)
	}
	nonCompleteClosure := types.NewEvidenceClosure("")
	nonCompleteClosure.SetReadSet(map[string]bool{"packages/core/src/transport.ts": true})
	before := len(mut.EmittedEvidence())
	_ = callChainReadParserRelationHandoffEvidence(makeCtx(false), nonCompleteClosure, facts, evidence)
	if after := len(mut.EmittedEvidence()); after != before {
		t.Fatalf("non-roster sibling call without a typed completeness obligation must not append a projection: before=%d after=%d", before, after)
	}
}

func TestCallChainReadParserRelationHandoffEvidence_PreciseBoundaries(t *testing.T) {
	makeFixture := func(provenance string, read bool, intent types.Intent, members []string) (*types.BusContext, *types.EvidenceClosure, []types.AnswerAggregateFact, []types.EvidenceItem) {
		evidence := []types.EvidenceItem{
			{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "start", Predicate: "calls", Object: "finish", Source: "src/pipeline.cj", LineStart: 2, GroundingStatus: types.GroundingGrounded},
			{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "collect", Subject: "collect", Source: "src/pipeline.cj", LineStart: 4, GroundingStatus: types.GroundingGrounded},
			{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "walk", Subject: "walk", Source: "src/pipeline.cj", LineStart: 10, GroundingStatus: types.GroundingGrounded},
		}
		mut := types.NewMutableState("opaque")
		mut.AppendEvidence(evidence)
		mut.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{"src/pipeline.cj": {
			RelPath: "src/pipeline.cj", Language: "cangjie", Package: "demo",
			Symbols:   []repotypes.Symbol{{Name: "collect", Kind: "function", File: "src/pipeline.cj", Line: 4, EndLine: 8}, {Name: "walk", Kind: "function", File: "src/pipeline.cj", Line: 10, EndLine: 20}},
			Relations: []repotypes.Relation{{Kind: "call", File: "src/pipeline.cj", Line: 6, ToEP: repotypes.RelationEndpoint{Name: "walk"}, Confidence: repotypes.ConfidenceAST, Provenance: provenance, ResolvedBy: "fixture"}},
		}}})
		ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: intent, Complexity: types.ComplexityComplex, PredicateAxis: types.AxisCall,
			Predicates:               types.SemanticPredicates{IsCrossComponent: true},
			CallChainEndpointProfile: orderedCallChainEndpoints("start", "finish"),
			AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), PrimaryEntities: []string{"start", "finish"}},
		}}}
		closure := types.NewEvidenceClosure("")
		if read {
			closure.SetReadSet(map[string]bool{"src/pipeline.cj": true})
		}
		facts := []types.AnswerAggregateFact{{Kind: types.AnswerAggregateMemberSet, Label: "chain", Role: types.AnswerAggregateRolePrincipalAnswer, Members: members, SupportRefs: []string{"src/pipeline.cj:4", "src/pipeline.cj:10"}}}
		return ctx, closure, facts, evidence
	}

	ctx, closure, facts, evidence := makeFixture(repotypes.ProvenanceCangjieParser, true, types.IntentTrace, []string{"demo::collect", "walk"})
	if got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence); !callChainTypedEvidenceContainsExactParserRelation(got, "src/pipeline.cj", 6, "demo::collect", "walk") {
		t.Fatalf("Cangjie AST-grade relation should use the language-neutral projection: %+v", got)
	}
	ctx, closure, facts, evidence = makeFixture(repotypes.ProvenanceRegexFallback, true, types.IntentTrace, []string{"demo::collect", "walk"})
	if got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence); len(got) != len(evidence) {
		t.Fatalf("regex fallback is noisy and must not gain typed authority: %+v", got)
	}
	ctx, closure, facts, evidence = makeFixture(repotypes.ProvenanceCangjieParser, false, types.IntentTrace, []string{"demo::collect", "walk"})
	if got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence); len(got) != len(evidence) {
		t.Fatalf("unread parser relation must not be promoted from repository-wide graph state: %+v", got)
	}
	ctx, closure, facts, evidence = makeFixture(repotypes.ProvenanceCangjieParser, true, types.IntentTrace, []string{"demo::collect", "walk"})
	ctx.AttachedHitrace = "sched_switch: runtime trace"
	if got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence); len(got) != len(evidence) {
		t.Fatalf("runtime Trace investigation is outside the source-code roster contract: %+v", got)
	}
	ctx, closure, facts, evidence = makeFixture(repotypes.ProvenanceCangjieParser, true, types.IntentTrace, []string{"demo::collect", "other::collect"})
	if got := callChainReadParserRelationHandoffEvidence(ctx, closure, facts, evidence); len(got) != len(evidence) {
		t.Fatalf("ambiguous or absent roster endpoint must fail open instead of guessing identity: %+v", got)
	}
}

func TestPreCompleteCallChain_PrincipalAggregateMemberSetCannotBypassDirectedEndpointProof(t *testing.T) {
	evidence := []types.EvidenceItem{
		{ID: "start", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR", Subject: "buildAnalysisIR", Source: "internal/agent/analyzer.go", LineStart: 100, GroundingStatus: types.GroundingGrounded},
		{ID: "mid", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, AnchorSymbol: "normalizer.Normalize", Subject: "buildAnalysisIR", Object: "normalizer.Normalize", Source: "internal/agent/analyzer.go", LineStart: 160, GroundingStatus: types.GroundingGrounded},
		{ID: "gate", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith", Subject: "prepareGate", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 300, GroundingStatus: types.GroundingGrounded},
		{ID: "reverse", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith", Subject: "gate.Run", Object: "gate.RunWith", Source: "internal/analysis/gate/gate.go", LineStart: 135, GroundingStatus: types.GroundingGrounded},
	}
	makeCtx := func() *types.BusContext {
		mut := types.NewMutableState("trace buildAnalysisIR to gate.RunWith")
		mut.AppendEvidence(evidence)
		mut.AppendDispatchToolResult(types.ToolResult{
			ToolName: "read_file",
			Success:  true,
			Summary: strings.Join([]string{
				"[internal/agent/analyzer.go: showing lines 160-160 of 320 total]",
				"  160│ rm.TermGraph = normalizer.Normalize(rm.TermGraph, resolver)",
				"[internal/agent/analyzer.go: showing lines 300-300 of 320 total]",
				"  300│ ir.QualityGate = gate.RunWith(ir, gate.GlobalThresholds(), mode, gate.RunOptions{Resolver: resolver})",
			}, "\n"),
		})
		return &types.BusContext{
			Mutable: mut,
			AnalysisIR: &types.AnalysisIR{
				RequestModel: types.RequestModel{
					RawRequest:               "trace buildAnalysisIR to gate.RunWith",
					Intent:                   types.IntentTrace,
					PredicateAxis:            types.AxisCall,
					CallChainEndpointProfile: orderedCallChainEndpoints("buildAnalysisIR", "gate.RunWith"),
					AnalyzerHints: types.AnalyzerHints{
						Kind:              string(types.ReqCallChain),
						ExactTargets:      []string{"buildAnalysisIR", "gate.RunWith"},
						MentionedEntities: []string{"buildAnalysisIR", "gate.RunWith"},
					},
				},
			},
		}
	}

	if got := preCompleteContractCheckWithEvidence(makeCtx(), "", evidence); !strings.Contains(got, "not directionally proven") {
		t.Fatalf("precondition: exact directed endpoint gate should fire, got:\n%s", got)
	}

	aggregateFacts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "principal call chain",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"normalizer.Normalize (internal/agent/analyzer.go:160)", "gate.RunWith (internal/agent/analyzer.go:300)"},
		SupportRefs: []string{
			"normalizer.Normalize: internal/agent/analyzer.go:160",
			"gate.RunWith: internal/agent/analyzer.go:300",
		},
	}}
	skipCtx := makeCtx()
	if !callChainAggregateMemberSetCompletesPrincipalBoundary(skipCtx, aggregateFacts, evidence) {
		support := buildAggregateMemberSupportIndexWithEvidence(skipCtx, evidence)
		var unusable []string
		for _, member := range aggregateFacts[0].Members {
			if !aggregateMemberSetMemberUsable(aggregateFacts[0], member, support) {
				unusable = append(unusable, member)
			}
		}
		startHint, endHint, _ := callChainPrincipalEndpointHints(skipCtx.AnalysisIR.RequestModel)
		span, spanOK := callChainPrincipalSpanContextForEvidence(evidence, startHint, endHint)
		t.Fatalf("helper should accept principal member_set; spanOK=%v spanSupport=%v span=%+v unusable=%v support_refs=%v", spanOK, aggregateMemberSetHasSupportInsideCallChainSpan(aggregateFacts[0], span), span, unusable, aggregateFacts[0].SupportRefs)
	}
	if got := preCompleteContractCheckWithEvidence(skipCtx, "", evidence, aggregateFacts); !strings.Contains(got, "not directionally proven") {
		t.Fatalf("principal aggregate member_set must not mint source-to-sink direction, got:\n%s", got)
	}
}

func TestPreCompleteCallChain_PrincipalAggregateMemberSetSkipsOnlySpanAfterDirectedProof(t *testing.T) {
	evidence := []types.EvidenceItem{
		{ID: "mid", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, AnchorSymbol: "normalizer.Normalize", Subject: "buildAnalysisIR", Object: "normalizer.Normalize", Source: "internal/agent/analyzer.go", LineStart: 160, GroundingStatus: types.GroundingGrounded},
		{ID: "helper", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, AnchorSymbol: "prepareGate", Subject: "normalizer.Normalize", Object: "prepareGate", Source: "internal/agent/analyzer.go", LineStart: 280, GroundingStatus: types.GroundingGrounded},
		{ID: "gate", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, AnchorSymbol: "gate.Run", Subject: "prepareGate", Object: "gate.Run", Source: "internal/agent/analyzer.go", LineStart: 300, GroundingStatus: types.GroundingGrounded},
	}
	mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
	mut.AppendEvidence(evidence)
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/analyzer.go: showing lines 160-300 of 320 total]",
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:                   types.IntentTrace,
			PredicateAxis:            types.AxisCall,
			CallChainEndpointProfile: orderedCallChainEndpoints("buildAnalysisIR", "gate.Run"),
			AnalyzerHints: types.AnalyzerHints{
				Kind:              string(types.ReqCallChain),
				ExactTargets:      []string{"buildAnalysisIR", "gate.Run"},
				MentionedEntities: []string{"buildAnalysisIR", "gate.Run"},
			},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "principal call chain",
		Value:       "3",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"normalizer.Normalize", "prepareGate", "gate.Run"},
		SupportRefs: []string{"internal/agent/analyzer.go:160", "internal/agent/analyzer.go:280", "internal/agent/analyzer.go:300"},
	}}
	if got := preCompleteContractCheckWithEvidence(ctx, "", evidence, facts); got != "" {
		t.Fatalf("directed endpoint proof plus grounded member_set should close only the interior-span contract, got:\n%s", got)
	}
}

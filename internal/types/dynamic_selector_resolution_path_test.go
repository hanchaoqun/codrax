package types

import "testing"

func dynamicSelectorTestEvidence(id string, kind EvidenceKind, anchor AnchorKind, subject, object, owner, snippet string) EvidenceItem {
	return EvidenceItem{
		ID:              id,
		Kind:            kind,
		Subject:         subject,
		Predicate:       string(anchor),
		Object:          object,
		Source:          "src/registry.py",
		LineStart:       10,
		LineEnd:         10,
		Scope:           ScopeLine,
		AnchorKind:      anchor,
		AnchorSymbol:    firstNonEmptyDynamicSelectorIdentity(subject, object),
		OwnerSymbol:     owner,
		Snippet:         snippet,
		GroundingStatus: GroundingGrounded,
	}
}

func dynamicSelectorCompleteEvidence() []EvidenceItem {
	application := dynamicSelectorTestEvidence("E-app", EvidenceRelationship, AnchorDefinition, `@register("json")`, "JsonPlugin", "JsonPlugin", "")
	application.Predicate = "decorator_selector_application"
	application.Producer = EvidenceProducerRepoMapDecoratorApplication
	application.Source = "src/plugins.py"
	application.SelectorApplication = &EvidenceSelectorApplication{Owner: "register", Literal: "json"}

	binding := dynamicSelectorTestEvidence("E-bind", EvidenceRegistration, AnchorAssignment, "REGISTRY[name]", "cls", "register", "REGISTRY[name] = cls")
	binding.Predicate = "binds"
	lookup := dynamicSelectorTestEvidence("E-lookup", EvidenceRelationship, AnchorAssignment, "cls", "REGISTRY[name]", "resolve", "cls = REGISTRY[name]")
	lookup.Predicate = "assigns"
	ret := dynamicSelectorTestEvidence("E-return", EvidenceDirect, AnchorReturn, "resolve", "cls()", "resolve", "return cls()")
	ret.Predicate = "returns"
	entry := dynamicSelectorTestEvidence("E-entry", EvidenceRelationship, AnchorCall, "run_pipeline", "resolve", "run_pipeline", "plugin = resolve(kind)")
	entry.Predicate = "calls"
	argument := dynamicSelectorTestEvidence("E-argument", EvidenceRelationship, AnchorArgument, "kind", "resolve", "run_pipeline", "plugin = resolve(kind)")
	argument.Predicate = "argument_flow"
	callbackCall := dynamicSelectorTestEvidence("E-callback-call", EvidenceRelationship, AnchorCall, "run_pipeline", "loop.run_in_executor", "run_pipeline", "loop.run_in_executor(None, plugin.handle, payload)")
	callbackCall.Predicate = "calls"
	callback := dynamicSelectorTestEvidence("E-callback", EvidenceRelationship, AnchorCallback, "loop.run_in_executor", "plugin.handle", "run_pipeline", "loop.run_in_executor(None, plugin.handle, payload)")
	callback.Predicate = "callback_handoff"
	typeRelation := dynamicSelectorTestEvidence("E-type", EvidenceRelationship, AnchorDefinition, "JsonPlugin", "BasePlugin", "JsonPlugin", "class JsonPlugin(BasePlugin):")
	typeRelation.Predicate = "inheritance"
	typeRelation.Producer = EvidenceProducerRepoMapStructuralRelation
	typeRelation.RelationOrdinal = 1

	return []EvidenceItem{application, binding, lookup, ret, entry, argument, callbackCall, callback, typeRelation}
}

func TestCompileDynamicSelectorResolutionPaths_PreservesTypedHopKindsAndEvidence(t *testing.T) {
	compiled := CompileDynamicSelectorResolutionPaths(dynamicSelectorCompleteEvidence(), "run_pipeline")
	if compiled.Version != DynamicSelectorResolutionPathVersion || len(compiled.Rejected) != 0 || len(compiled.Candidates) != 1 {
		t.Fatalf("unexpected compilation: %+v", compiled)
	}
	path := compiled.Candidates[0]
	if path.Status != DynamicSelectorResolutionCandidateOnly || path.SelectorLiteral != "json" ||
		path.SelectorArgument != "kind" || path.ContainerIdentity != "REGISTRY" || path.LookupIdentity != "resolve" || path.CandidateIdentity != "JsonPlugin" {
		t.Fatalf("unexpected candidate identity: %+v", path)
	}
	wantRoles := []DynamicSelectorResolutionHopRole{
		DynamicSelectorHopEntryCall,
		DynamicSelectorHopSelectorArgument,
		DynamicSelectorHopSelectorApplication,
		DynamicSelectorHopRegistration,
		DynamicSelectorHopLookupAssignment,
		DynamicSelectorHopFactoryReturn,
	}
	wantRelations := []DiagramRelationKind{DiagramRelCall, DiagramRelArgumentFlow, DiagramRelUnknown, DiagramRelRegister, DiagramRelAssignment, DiagramRelReturn}
	wantEvidence := []string{"E-entry", "E-argument", "E-app", "E-bind", "E-lookup", "E-return"}
	if len(path.Hops) != len(wantRoles) {
		t.Fatalf("hop count=%d, want %d: %+v", len(path.Hops), len(wantRoles), path.Hops)
	}
	for i := range wantRoles {
		if path.Hops[i].Role != wantRoles[i] || path.Hops[i].RelationKind != wantRelations[i] || path.Hops[i].EvidenceID != wantEvidence[i] {
			t.Fatalf("hop[%d]=%+v, want role=%q relation=%q evidence=%q", i, path.Hops[i], wantRoles[i], wantRelations[i], wantEvidence[i])
		}
	}
	if len(path.CallbackHops) != 2 ||
		path.CallbackHops[0].Role != DynamicSelectorHopCallbackReceiverCall || path.CallbackHops[0].RelationKind != DiagramRelCall || path.CallbackHops[0].EvidenceID != "E-callback-call" ||
		path.CallbackHops[1].Role != DynamicSelectorHopCallbackHandoff || path.CallbackHops[1].RelationKind != DiagramRelCallback || path.CallbackHops[1].EvidenceID != "E-callback" {
		t.Fatalf("callback receiver call and handoff were not preserved independently: %+v", path.CallbackHops)
	}
	if len(path.TypeRoster) != 1 || path.TypeRoster[0].RelationKind != DiagramRelTypeRelation || path.TypeRoster[0].EvidenceID != "E-type" {
		t.Fatalf("candidate type roster was not preserved independently: %+v", path.TypeRoster)
	}
}

func TestCompileDynamicSelectorResolutionPaths_EntryUsesTypedCallSubjectBeforeQualifiedOwner(t *testing.T) {
	evidence := dynamicSelectorCompleteEvidence()
	evidence[4].OwnerSymbol = "pipeline.runner.run_pipeline"

	compiled := CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Rejected) != 0 || len(compiled.Candidates) != 1 {
		t.Fatalf("qualified enclosing owner must not hide the exact typed call source endpoint: %+v", compiled)
	}
	entry := compiled.Candidates[0].Hops[0]
	if entry.FromIdentity != "run_pipeline" || entry.ToIdentity != "resolve" || entry.EvidenceID != "E-entry" {
		t.Fatalf("entry hop must preserve the call edge's typed subject/object endpoints: %+v", entry)
	}

	evidence[4].Subject = ""
	compiled = CompileDynamicSelectorResolutionPaths(evidence, "pipeline.runner.run_pipeline")
	if len(compiled.Rejected) != 0 || len(compiled.Candidates) != 1 ||
		compiled.Candidates[0].Hops[0].FromIdentity != "pipeline.runner.run_pipeline" {
		t.Fatalf("legacy subject-empty call rows should still fall back to their exact qualified owner: %+v", compiled)
	}
}

func TestCompileDynamicSelectorResolutionPaths_AcceptsExactIndexedAssignmentWithoutRelabelingRegistration(t *testing.T) {
	evidence := dynamicSelectorCompleteEvidence()
	evidence[1].Kind = EvidenceConcrete
	evidence[1].Predicate = "assigns"

	compiled := CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Rejected) != 0 || len(compiled.Candidates) != 1 {
		t.Fatalf("exact selector-owner indexed assignment should complete one candidate: %+v", compiled)
	}
	binding := compiled.Candidates[0].Hops[3]
	if binding.Role != DynamicSelectorHopRegistration || binding.RelationKind != DiagramRelAssignment ||
		binding.ClaimForm != ClaimAssignmentFact || binding.FromIdentity != "REGISTRY" || binding.ToIdentity != "cls" || binding.EvidenceID != "E-bind" {
		t.Fatalf("indexed assignment must preserve assignment semantics and evidence identity: %+v", binding)
	}
}

func TestCompileDynamicSelectorResolutionPaths_CollapsesSameOccurrenceAssignmentAndRegistrationProofs(t *testing.T) {
	evidence := dynamicSelectorCompleteEvidence()
	assignment := evidence[1]
	assignment.ID = "E-bind-assignment"
	assignment.Kind = EvidenceConcrete
	assignment.Predicate = "assigns"
	evidence = append(evidence, assignment)

	compiled := CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Rejected) != 0 || len(compiled.Candidates) != 1 {
		t.Fatalf("same-coordinate assignment and registration rows should corroborate one binding: %+v", compiled)
	}
	binding := compiled.Candidates[0].Hops[3]
	if binding.RelationKind != DiagramRelRegister || binding.ClaimForm != ClaimRegistrationEdge || binding.EvidenceID != "E-bind" {
		t.Fatalf("same-occurrence duplicate should retain the stronger exact registration row: %+v", binding)
	}

	otherOccurrence := assignment
	otherOccurrence.ID = "E-bind-other-line"
	otherOccurrence.LineStart = 11
	otherOccurrence.LineEnd = 11
	evidence = append(dynamicSelectorCompleteEvidence(), otherOccurrence)
	compiled = CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Candidates) != 0 || len(compiled.Rejected) != 1 || compiled.Rejected[0].Reason != DynamicSelectorRejectAmbiguousContainer {
		t.Fatalf("same endpoints at a different source occurrence must remain ambiguous: %+v", compiled)
	}
}

func TestCompileDynamicSelectorResolutionPaths_OrdinarySelectorOwnerPropertyAssignmentIsNotBinding(t *testing.T) {
	evidence := dynamicSelectorCompleteEvidence()
	evidence[1].Kind = EvidenceConcrete
	evidence[1].Subject = "cls.plugin_name"
	evidence[1].Object = "name"
	evidence[1].Snippet = "cls.plugin_name = name"

	compiled := CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Candidates) != 0 || len(compiled.Rejected) != 1 || compiled.Rejected[0].Reason != DynamicSelectorRejectBindingUnavailable {
		t.Fatalf("ordinary property assignment must not become selector binding authority: %+v", compiled)
	}
}

func TestCompileDynamicSelectorResolutionPaths_ArgumentFlowMustMatchCallSiteAndBeUnique(t *testing.T) {
	evidence := dynamicSelectorCompleteEvidence()
	evidence[5].LineStart = 11
	evidence[5].LineEnd = 11
	compiled := CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Candidates) != 0 || len(compiled.Rejected) != 1 || compiled.Rejected[0].Reason != DynamicSelectorRejectArgumentUnavailable {
		t.Fatalf("a selector argument from another call site must not be joined: %+v", compiled)
	}

	evidence = dynamicSelectorCompleteEvidence()
	other := evidence[5]
	other.ID = "E-argument-other"
	other.Subject = "fallback_kind"
	evidence = append(evidence, other)
	compiled = CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Candidates) != 0 || len(compiled.Rejected) != 1 || compiled.Rejected[0].Reason != DynamicSelectorRejectAmbiguousArgument {
		t.Fatalf("two exact selector arguments at one call site must fail closed: %+v", compiled)
	}
}

func TestCompileDynamicSelectorResolutionPaths_LookupMustBeExactIndexedRead(t *testing.T) {
	evidence := dynamicSelectorCompleteEvidence()
	evidence[2].Object = "REGISTRY"
	evidence[2].Snippet = "cls = REGISTRY"

	compiled := CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Candidates) != 0 || len(compiled.Rejected) != 1 || compiled.Rejected[0].Reason != DynamicSelectorRejectLookupUnavailable {
		t.Fatalf("whole-container assignment must not masquerade as selector lookup: %+v", compiled)
	}
}

func TestCompileDynamicSelectorResolutionPaths_AmbiguousCandidateFailsClosed(t *testing.T) {
	evidence := dynamicSelectorCompleteEvidence()
	other := evidence[0]
	other.ID = "E-app-other"
	other.Object = "OtherJsonPlugin"
	evidence = append(evidence, other)

	compiled := CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Candidates) != 0 || len(compiled.Rejected) != 1 || compiled.Rejected[0].Reason != DynamicSelectorRejectAmbiguousCandidate {
		t.Fatalf("same selector with two candidates must fail closed: %+v", compiled)
	}
}

func TestCompileDynamicSelectorResolutionPaths_AmbiguousContainerFailsClosed(t *testing.T) {
	evidence := dynamicSelectorCompleteEvidence()
	other := evidence[1]
	other.ID = "E-bind-other"
	other.Subject = "SECONDARY[name]"
	other.Snippet = "SECONDARY[name] = cls"
	evidence = append(evidence, other)

	compiled := CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Candidates) != 0 || len(compiled.Rejected) != 1 || compiled.Rejected[0].Reason != DynamicSelectorRejectAmbiguousContainer {
		t.Fatalf("selector owner with two containers must fail closed: %+v", compiled)
	}
}

func TestCompileDynamicSelectorResolutionPaths_DoesNotParseDisplayOrProse(t *testing.T) {
	evidence := dynamicSelectorCompleteEvidence()
	evidence[0].SelectorApplication = nil
	evidence[0].Summary = `@register("json") selects JsonPlugin`

	compiled := CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Candidates) != 0 || len(compiled.Rejected) != 0 {
		t.Fatalf("display syntax or summary must not mint selector authority: %+v", compiled)
	}
}

func TestCompileDynamicSelectorResolutionPaths_NormalizesLanguageSeparatorsOnly(t *testing.T) {
	evidence := dynamicSelectorCompleteEvidence()
	evidence[2].OwnerSymbol = "Factory::resolve"
	evidence[3].OwnerSymbol = "Factory.resolve"
	evidence[3].Subject = "Factory::resolve"
	evidence[4].Object = "Factory/resolve"
	evidence[5].Object = "Factory.resolve"

	compiled := CompileDynamicSelectorResolutionPaths(evidence, "run_pipeline")
	if len(compiled.Candidates) != 1 || compiled.Candidates[0].LookupIdentity != "Factory::resolve" {
		t.Fatalf("language-native separators should preserve one exact identity: %+v", compiled)
	}
}

func TestEvidenceSelectorApplicationParticipatesInStableIdentityAndClone(t *testing.T) {
	base := dynamicSelectorCompleteEvidence()[0]
	other := base
	other.SelectorApplication = &EvidenceSelectorApplication{Owner: "register", Literal: "csv"}
	if StableEvidenceID(base) == StableEvidenceID(other) {
		t.Fatal("distinct typed selector literals must not share a stable evidence ID")
	}
	clone := cloneEvidenceItemForMutableStorage(base)
	clone.SelectorApplication.Literal = "mutated"
	if base.SelectorApplication.Literal != "json" {
		t.Fatal("mutable evidence clone must deep-copy selector metadata")
	}
}

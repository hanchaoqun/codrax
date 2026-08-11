package types

import (
	"strings"
	"testing"
)

func discoveryEvidence(kind EvidenceKind, anchor AnchorKind, subject, object string) EvidenceItem {
	return EvidenceItem{
		Kind: kind, AnchorKind: anchor, Subject: subject, Object: object,
		Source: "src/factory.cpp", LineStart: 10, Scope: ScopeLine,
		GroundingStatus: GroundingGrounded,
	}
}

func TestCallChainDiscoverySelectionEvidence_FactoryReturnMustConnectToCall(t *testing.T) {
	call := discoveryEvidence(EvidenceDirect, AnchorCall, "make_sink", "SinkRegistry.create")
	ret := discoveryEvidence(EvidenceDirect, AnchorReturn, "SinkRegistry::create", "ConsoleSink")
	if got := CallChainDiscoverySelectionEvidence([]EvidenceItem{call, ret}); len(got) != 1 || got[0].Object != "ConsoleSink" {
		t.Fatalf("connected cross-language factory return should select a target: %+v", got)
	}

	unrelated := ret
	unrelated.Subject = "OtherFactory::create"
	if got := CallChainDiscoverySelectionEvidence([]EvidenceItem{call, unrelated}); len(got) != 0 {
		t.Fatalf("unrelated return must not close discovery selection: %+v", got)
	}
}

func TestCallChainDiscoverySelectionEvidence_RegistrationIsDirectAuthority(t *testing.T) {
	registration := discoveryEvidence(EvidenceRegistration, AnchorDefinition, "SinkRegistry.console", "ConsoleSink")
	if got := CallChainDiscoverySelectionEvidence([]EvidenceItem{registration}); len(got) != 1 {
		t.Fatalf("exact registration should directly select a target: %+v", got)
	}
	reversed := registration
	reversed.Object = ""
	if got := CallChainDiscoverySelectionEvidence([]EvidenceItem{reversed}); len(got) != 0 {
		t.Fatalf("incomplete registration must fail closed: %+v", got)
	}
}

func TestCallChainDiscoverySelectionEvidence_AssignmentConnectsDynamicReceiverAcrossLanguages(t *testing.T) {
	for _, tc := range []struct {
		name       string
		callTarget string
		subject    string
		object     string
	}{
		{"cpp", "sink_->write", "sink_", "ConsoleSink"},
		{"arkts", "handler.handle", "handler", "ConsoleHandler"},
		{"cangjie", "processor.process", "processor", "FastProcessor"},
		{"python", "backend.run", "backend", "LocalBackend"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := discoveryEvidence(EvidenceDirect, AnchorCall, "entry.run", tc.callTarget)
			assignment := discoveryEvidence(EvidenceDirect, AnchorAssignment, tc.subject, tc.object)
			assignment.Snippet = tc.subject + " = " + tc.object
			if got := CallChainDiscoverySelectionEvidence([]EvidenceItem{call, assignment}); len(got) != 1 {
				t.Fatalf("connected assignment should select a target: %+v", got)
			}
		})
	}
}

func TestCallChainDiscoverySelectionEvidence_InitializerConnectsNormalizedConcreteReceiver(t *testing.T) {
	call := discoveryEvidence(EvidenceRelationship, AnchorCall, "VisitRepository.insert", "AuditLog.record")
	initializer := discoveryEvidence(EvidenceDirect, AnchorInitializer, "audit", "AuditLog")
	initializer.Snippet = "private final AuditLog audit = new AuditLog();"

	got := CallChainDiscoverySelectionEvidence([]EvidenceItem{call, initializer})
	if len(got) != 1 || got[0].Subject != "audit" || got[0].Object != "AuditLog" {
		t.Fatalf("initializer RHS must join a parser-normalized concrete receiver: %+v", got)
	}
}

func TestCallChainDiscoverySelectionEvidence_RequiresCitableTypedFact(t *testing.T) {
	call := discoveryEvidence(EvidenceDirect, AnchorCall, "entry.run", "handler.handle")
	assignment := discoveryEvidence(EvidenceDirect, AnchorAssignment, "handler", "ConcreteHandler")
	assignment.Snippet = "handler = ConcreteHandler"
	assignment.GroundingStatus = GroundingUngrounded
	if HasCallChainDiscoverySelectionEvidence([]EvidenceItem{call, assignment}) {
		t.Fatal("ungrounded assignment must not authorize runtime target selection")
	}
}

func TestCallChainDiscoverySelectionEmissionGuideKeepsMinimalFormsExclusive(t *testing.T) {
	for _, want := range []string{
		"evidence_kind=registration",
		"evidence_kind=direct with anchor_kind=assignment or initializer",
		"evidence_kind=direct with anchor_kind=return",
		"actual return statement",
		"evidence_kind=conditional with anchor_kind=condition",
		"Never combine evidence_kind=registration with anchor_kind=return",
	} {
		if !strings.Contains(CallChainDiscoverySelectionEmissionGuide, want) {
			t.Fatalf("single-source selection guide missing %q: %s", want, CallChainDiscoverySelectionEmissionGuide)
		}
	}
}

func TestCallChainDiscoverySelectionEvidenceSeesSameAnchorEndpointAmendment(t *testing.T) {
	mu := NewMutableState("opaque")
	base := discoveryEvidence(EvidenceRegistration, AnchorReturn, "", "ConsoleSink")
	base.Predicate = "constructs"
	base.Condition = `kind == "console"`
	base.AnchorSymbol = "create"
	base.OwnerSymbol = "create"
	base.ID = StableEvidenceID(base)
	complete := base
	complete.Subject = "SinkRegistry::create"
	complete.Condition = ""
	complete.ID = StableEvidenceID(complete)

	mu.AppendEvidence([]EvidenceItem{base})
	mu.AppendEvidence([]EvidenceItem{complete})
	got := mu.EmittedEvidence()
	if len(got) != 1 || !HasCallChainDiscoverySelectionEvidence(got) {
		t.Fatalf("answer-grade snapshot must expose the completed typed selection fact: %+v", got)
	}
}

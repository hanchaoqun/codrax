package types

import "testing"

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
			if got := CallChainDiscoverySelectionEvidence([]EvidenceItem{call, assignment}); len(got) != 1 {
				t.Fatalf("connected assignment should select a target: %+v", got)
			}
		})
	}
}

func TestCallChainDiscoverySelectionEvidence_RequiresCitableTypedFact(t *testing.T) {
	call := discoveryEvidence(EvidenceDirect, AnchorCall, "entry.run", "handler.handle")
	assignment := discoveryEvidence(EvidenceDirect, AnchorAssignment, "handler", "ConcreteHandler")
	assignment.GroundingStatus = GroundingUngrounded
	if HasCallChainDiscoverySelectionEvidence([]EvidenceItem{call, assignment}) {
		t.Fatal("ungrounded assignment must not authorize runtime target selection")
	}
}

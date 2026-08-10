package types

import "testing"

func TestAssignmentEvidenceEndpointsMatchCrossLanguageSimpleTransfers(t *testing.T) {
	tests := []struct {
		name, snippet, subject, object string
		anchor                         AnchorKind
	}{
		{"go_member", `o.busCtx.AnalysisIR = output.AnalysisIR`, "busCtx.AnalysisIR", "output.AnalysisIR", AnchorAssignment},
		{"go_initializer", `Agent: binding.Agent,`, "Agent", "binding.Agent", AnchorInitializer},
		{"java", `this.handler = selectedHandler;`, "handler", "selectedHandler", AnchorAssignment},
		{"javascript", `const handler = selectedHandler;`, "handler", "selectedHandler", AnchorAssignment},
		{"kotlin", `val handler = selectedHandler`, "handler", "selectedHandler", AnchorAssignment},
		{"arkts", `this.state = nextState`, "this.state", "nextState", AnchorAssignment},
		{"cangjie", `this.value = incoming`, "value", "incoming", AnchorAssignment},
		{"rust", `self.value = next;`, "self.value", "next", AnchorAssignment},
		{"python", `self.backend = LocalBackend()`, "backend", "LocalBackend", AnchorAssignment},
		{"ruby", `@handler = selected_handler`, "handler", "selected_handler", AnchorAssignment},
		{"swift", `let handler = selectedHandler`, "handler", "selectedHandler", AnchorAssignment},
		{"lua", `local handler = selected_handler`, "handler", "selected_handler", AnchorAssignment},
		{"c", `handler = selected_handler;`, "handler", "selected_handler", AnchorAssignment},
		{"cpp", `this->sink_ = sink;`, "sink_", "sink", AnchorAssignment},
		{"typescript_typed", `const handler: Handler = selectedHandler;`, "handler", "selectedHandler", AnchorAssignment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := EvidenceItem{AnchorKind: tc.anchor, Subject: tc.subject, Object: tc.object, Snippet: tc.snippet}
			if !AssignmentEvidenceEndpointsMatch(item) {
				receiver, value, ok := AssignmentEvidenceEndpoints(item)
				t.Fatalf("exact endpoints did not match: receiver=%q value=%q ok=%t item=%+v", receiver, value, ok, item)
			}
		})
	}
}

func TestAssignmentEvidenceEndpointsRejectsFalseOrAmbiguousAuthority(t *testing.T) {
	tests := []EvidenceItem{
		{AnchorKind: AnchorAssignment, Subject: "applyStageOutput", Object: "output.AnalysisIR", Snippet: `o.busCtx.AnalysisIR = output.AnalysisIR`},
		{AnchorKind: AnchorAssignment, Subject: "left", Object: "value", Snippet: `left, right = value, other`},
		{AnchorKind: AnchorAssignment, Subject: "state", Object: "next", Snippet: `state = cond ? next : fallback`},
		{AnchorKind: AnchorAssignment, Subject: "state", Object: "left", Snippet: `state = left + right`},
		{AnchorKind: AnchorAssignment, Subject: "state", Object: "left", Snippet: `state = left - right`},
		{AnchorKind: AnchorAssignment, Subject: "state", Object: "next", Snippet: `state += next`},
		{AnchorKind: AnchorAssignment, Subject: "name", Object: "1", Snippet: `string name = 1;`}, // proto field number, not value flow
		{AnchorKind: AnchorAssignment, Subject: "name", Object: "literal", Snippet: `name = "literal"`},
		{AnchorKind: AnchorAssignment, Subject: "state", Object: "next"},
	}
	for _, item := range tests {
		if AssignmentEvidenceEndpointsMatch(item) {
			t.Fatalf("false/ambiguous assignment became exact authority: %+v", item)
		}
	}
}

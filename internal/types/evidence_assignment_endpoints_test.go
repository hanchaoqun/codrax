package types

import (
	"strings"
	"testing"
)

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

func TestAssignmentEvidenceEndpointsAcceptsExactFullRHSExpressionButKeepsCanonicalIdentity(t *testing.T) {
	tests := []struct {
		name, snippet, subject, object, wantValue string
	}{
		{"go", `firstFinalizeDraft = strings.TrimSpace(out.FinalAnswer)`, "firstFinalizeDraft", "strings.TrimSpace(out.FinalAnswer)", "strings.TrimSpace"},
		{"java", `this.handler = Objects.requireNonNull(selectedHandler);`, "handler", "Objects.requireNonNull(selectedHandler)", "Objects.requireNonNull"},
		{"python", `self.backend = LocalBackend(config)`, "backend", "LocalBackend(config)", "LocalBackend"},
		{"arkts", `this.state = await service.load(next)`, "this.state", "await service.load(next)", "service.load"},
		{"cangjie", `this.value = makeValue(input)`, "value", "makeValue(input)", "makeValue"},
		{"rust", `self.value = Arc::new(next);`, "self.value", "Arc::new(next)", "Arc::new"},
		{"cpp", `this->sink_ = std::move(sink);`, "sink_", "std::move(sink)", "std::move"},
		{"go_composite", `state = RuntimeState{Value: next}`, "state", "RuntimeState{Value: next}", "RuntimeState"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := EvidenceItem{AnchorKind: AnchorAssignment, Subject: tc.subject, Object: tc.object, Snippet: tc.snippet}
			if !AssignmentEvidenceEndpointsMatch(item) {
				t.Fatalf("exact full RHS expression should be accepted: %+v", item)
			}
			_, value, ok := AssignmentEvidenceEndpoints(item)
			if !ok || value != tc.wantValue {
				t.Fatalf("canonical RHS identity=%q ok=%t want %q", value, ok, tc.wantValue)
			}
			wrong := item
			wrong.Object = strings.Replace(tc.object, "next", "other", 1)
			if wrong.Object == item.Object {
				wrong.Object += ".unexpected"
			}
			if AssignmentEvidenceEndpointsMatch(wrong) {
				t.Fatalf("different full RHS expression gained authority: %+v", wrong)
			}
		})
	}
}

func TestAssignmentEvidenceEndpointsSelectsExactReceiverFromMultiResultAssignment(t *testing.T) {
	tests := []struct {
		name, snippet, subject, anchor, object, wantReceiver, wantValue string
	}{
		{
			name:    "go_multi_result",
			snippet: `o.busCtx.EvidenceItems, evidenceChanged = agent.MergeEvidenceItemsIfChanged(o.busCtx.EvidenceItems, output.EvidenceItems)`,
			subject: "o.busCtx.EvidenceItems", anchor: "o.busCtx.EvidenceItems",
			object: "agent.MergeEvidenceItemsIfChanged", wantReceiver: "o.busCtx.EvidenceItems", wantValue: "agent.MergeEvidenceItemsIfChanged",
		},
		{
			name:    "python_tuple_unpack",
			snippet: `(result, changed) = merge(current, incoming)`,
			subject: "result", anchor: "result", object: "merge(current, incoming)",
			wantReceiver: "result", wantValue: "merge",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := EvidenceItem{AnchorKind: AnchorAssignment, Subject: tc.subject, AnchorSymbol: tc.anchor, Object: tc.object, Snippet: tc.snippet}
			if !AssignmentEvidenceEndpointsMatch(item) {
				t.Fatalf("exact selected multi-result receiver did not match: %+v", item)
			}
			receiver, value, ok := AssignmentEvidenceEndpoints(item)
			if !ok || receiver != tc.wantReceiver || value != tc.wantValue {
				t.Fatalf("endpoints=(%q,%q,%t), want (%q,%q,true)", receiver, value, ok, tc.wantReceiver, tc.wantValue)
			}
		})
	}
}

func TestAssignmentEvidenceEndpointsMultiResultFailsClosedOnBroadOrConflictingSelection(t *testing.T) {
	line := `o.busCtx.EvidenceItems, evidenceChanged = agent.MergeEvidenceItemsIfChanged(o.busCtx.EvidenceItems, output.EvidenceItems)`
	for _, item := range []EvidenceItem{
		{AnchorKind: AnchorAssignment, Subject: "o.busCtx", Object: "output.EvidenceItems", Snippet: line},
		{AnchorKind: AnchorAssignment, Subject: "o.busCtx.EvidenceItems", AnchorSymbol: "evidenceChanged", Object: "agent.MergeEvidenceItemsIfChanged", Snippet: line},
		{AnchorKind: AnchorAssignment, Subject: "o.busCtx.EvidenceItems", Object: "output.EvidenceItems", Snippet: line},
	} {
		if AssignmentEvidenceEndpointsMatch(item) {
			t.Fatalf("broad, conflicting, or wrong-source multi-result row gained authority: %+v", item)
		}
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

func TestAssignmentEvidenceStateMatchesScalarStatesWithoutMintingRelationEndpoints(t *testing.T) {
	tests := []EvidenceItem{
		{AnchorKind: AnchorAssignment, Subject: "_HAVE_NATIVE", Object: "True", Snippet: `_HAVE_NATIVE = True`},
		{AnchorKind: AnchorAssignment, Subject: "_HAVE_NATIVE", Object: "False", Snippet: `_HAVE_NATIVE = False`},
		{AnchorKind: AnchorAssignment, Subject: "state", Object: "null", Snippet: `state = null;`},
		{AnchorKind: AnchorAssignment, Subject: "retryCount", Object: "3", Snippet: `retryCount = 3`},
		{AnchorKind: AnchorInitializer, Subject: "mode", Object: `"safe"`, Snippet: `mode: "safe",`},
	}
	for _, item := range tests {
		if !AssignmentEvidenceStateMatches(item) {
			t.Fatalf("exact scalar assignment state did not match: %+v", item)
		}
		if AssignmentEvidenceEndpointsMatch(item) {
			t.Fatalf("scalar state must not mint code-identity relation endpoints: %+v", item)
		}
	}
}

func TestAssignmentEvidenceStateRejectsModelEndpointsAndExpressions(t *testing.T) {
	tests := []EvidenceItem{
		{AnchorKind: AnchorAssignment, Subject: "FastTokenizer.tokenize", Object: "True", Snippet: `_HAVE_NATIVE = True`},
		{AnchorKind: AnchorAssignment, Subject: "state", Object: "True", Snippet: `state = false`},
		{AnchorKind: AnchorAssignment, Subject: "state", Object: "next", Snippet: `state = cond ? next : fallback`},
		{AnchorKind: AnchorAssignment, Subject: "state", Object: "left", Snippet: `state = left + right`},
		{AnchorKind: AnchorAssignment, Subject: "state", Object: "True"},
	}
	for _, item := range tests {
		if AssignmentEvidenceStateMatches(item) {
			t.Fatalf("false/ambiguous scalar assignment became state authority: %+v", item)
		}
	}
}

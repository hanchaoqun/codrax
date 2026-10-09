package tool

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Public emit, retained state, and later normalization must not turn a display
// label match into proof that two reported objects are the same object.
func TestEmitInvestigationCompleteMemberIdentityPreservesQualifiedObjects(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		members, refs, notes []string
	}{
		{
			name:    "live_framework_roster",
			members: []string{"ArkUI (FlushMessages)", "Flutter (进程200)", "Web渲染 (VizCompositor/IssueBeginFrame, 进程300)", "React Native (RNViewBase::onDraw, 进程400)", "KMP/Compose (Recomposer::recompose, 进程500)", "Flutter (进程600)", "Web渲染 (VizCompositor, 进程600)", "游戏引擎 (Unity::PlayerLoop, 进程700)", "React Native (进程900)"},
			refs:    []string{"events.systrace:行3-6", "events.systrace:行6-9", "events.systrace:行10-13", "events.systrace:行14-15", "events.systrace:行16-17", "events.systrace:行18-21", "events.systrace:行22-23", "events.systrace:行24-27", "events.systrace:行28-29"},
			notes:   []string{"first", "second", "third", "fourth", "fifth", "sixth", "seventh", "eighth", "ninth"},
		},
		{
			name:    "non_runtime_unknown_qualifiers",
			members: []string{"Cache (tenant alpha)", "Cache (tenant beta)", "Worker (background)", "Worker (supporting)"},
			refs:    []string{"record:a", "record:b", "", "record:d"},
			notes:   []string{"alpha owns one instance", "beta owns another", "", "role is not identity"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Label: "observed objects", Value: strconv.Itoa(len(tc.members)), Role: types.AnswerAggregateRolePrincipalAnswer, Members: tc.members, SupportRefs: tc.refs, MemberNotes: tc.notes}
			mu := types.NewMutableState("list observed objects")
			bus := &types.BusContext{Mutable: mu}
			if tc.name == "live_framework_roster" {
				bus, mu = supprefTolWitnessBus(supprefTolFaithfulH9Policy())
			} else {
				// Domain selection prevents code-shaped external object names from
				// becoming current-source claims; it grants no identity/evidence.
				fact.Dimensions = []types.AnswerAggregateDimension{{Name: "origin", Value: "external_document"}}
			}
			params, err := json.Marshal(map[string]any{"reason": "object enumeration complete", "confidence": "high", "result_kind": "resolved", "aggregate_facts": []types.AnswerAggregateFact{fact}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&EmitInvestigationComplete{}).Execute(bus, params)
			if err != nil || !result.Success {
				t.Fatalf("public emit failed: %+v %v", result, err)
			}
			got := mu.StableInvestigationAggregateFacts()
			for pass := 0; pass < 2; pass++ {
				if len(got) != 1 || got[0].Value != fact.Value || !reflect.DeepEqual(got[0].Members, tc.members) || !reflect.DeepEqual(got[0].SupportRefs, tc.refs) || !reflect.DeepEqual(got[0].MemberNotes, tc.notes) {
					t.Fatalf("pass %d merged separate identities or shifted metadata: %+v", pass, got)
				}
				got = types.NormalizeAnswerAggregateMemberSetSurfaces(got)
			}
		})
	}
}

func TestMemberIdentityFormRepairCannotEraseUnknownQualifier(t *testing.T) {
	for _, ref := range []string{"", "record:a", "src/cache.go:12"} {
		fact := types.AnswerAggregateFact{
			Kind: types.AnswerAggregateMemberSet, Label: "objects", Value: "2",
			Members:     []string{"Cache (tenant alpha)", "Cache (tenant beta)"},
			MemberNotes: []string{"existing alpha note", "existing beta note"},
			SupportRefs: []string{ref, ref}, Provenance: "claimed:same_source",
		}
		got, notes := normalizeDecoratedMemberSetFormDebt(&types.BusContext{Mutable: types.NewMutableState("q")}, "resolved", []types.AnswerAggregateFact{fact}, nil)
		if len(notes) != 0 || len(got) != 1 || !reflect.DeepEqual(got[0], fact) {
			t.Fatalf("support text or source self-description erased identity: ref=%q got=%+v notes=%v", ref, got, notes)
		}
	}
}

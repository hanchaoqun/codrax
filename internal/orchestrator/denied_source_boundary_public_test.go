package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the published caveat API after the actual denial checker. This
// tests a disclosure boundary, not the authority of an artifact observation:
// no fixture observation is promoted and no global name exception is added.
func TestDeniedSourceBoundaryPublicDoesNotDenyRuntimeObservation(t *testing.T) {
	for _, class := range []types.TypedDenialClass{
		types.TypedDenialExternalPerfStallUnresolved,
		types.TypedDenialExternalLogFrameUnresolved,
		types.TypedDenialOracleSymbolUnverified,
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(string(class)+"/"+lang, func(t *testing.T) {
				const token = "captured_wait_site"
				const original = "业务记录保持。 captured_wait_site observed at 2.003–2.014; mapping remains a separate question."
				denials := types.NewTypedDenialSet()
				denials.Add(types.TypedDenial{Class: class, Token: token})
				doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: original}}}
				before, _ := json.Marshal([]any{doc, denials})
				violations := runDeniedTokenAnswerCheck(doc, denials)
				if len(violations) != 1 || violations[0].Kind != types.ViolDeniedTokenUndeclared {
					t.Fatalf("disclosure repair must not remove the existing precise denial: %+v", violations)
				}
				if !strings.Contains(violations[0].Repair, "source mapping") || strings.Contains(violations[0].Repair, "is NOT present") {
					t.Errorf("repair must describe unverified source mapping, not assert absence: %s", violations[0].Repair)
				}
				ctx := &types.BusContext{Language: lang, Mutable: types.NewMutableState("bounded source mapping"), TypedDenials: denials}
				ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
				got := AppendSoftContractCaveatsToAnswerForBus(original, violations, lang, ctx)
				if !strings.HasPrefix(got, original) {
					t.Fatal("caveat changed model-owned prose")
				}
				caveat := strings.TrimPrefix(got, original)
				want := "当前源码映射尚未验证"
				if lang == "en" {
					want = "repository-source mapping is not yet verified"
				}
				if !strings.Contains(caveat, token) || !strings.Contains(caveat, want) {
					t.Errorf("source-only denial lost its scope: %s", caveat)
				}
				for _, bad := range []string{"尚未由当前证据确认", "current evidence does not yet verify", "not yet verified by the current evidence"} {
					if strings.Contains(caveat, bad) {
						t.Errorf("source denial became a claim about all runtime evidence: %s", caveat)
					}
				}
				after, _ := json.Marshal([]any{doc, denials})
				if string(before) != string(after) {
					t.Fatal("disclosure mutated document or denial authority")
				}
			})
		}
	}
}

func TestDeniedSourceBoundaryPublicMalformedAndTruncatedNames(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, malformed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/malformed=%t", lang, malformed), func(t *testing.T) {
				var violations []types.Violation
				for _, token := range []string{"a", "b", "c", "d"} {
					detail := fmt.Sprintf("answer block %q names token %q without disclosing it as unverified / external", "summary", token)
					if malformed {
						detail = "unparseable legacy detail"
					}
					violations = append(violations, types.Violation{Kind: types.ViolDeniedTokenUndeclared, Detail: detail})
				}
				got := MaterializeUnresolvedViolationsAsCaveats(violations, lang)
				if len(got) != 1 {
					t.Fatalf("existing one-family aggregation changed: %v", got)
				}
				want := "当前源码映射尚未验证"
				if lang == "en" {
					want = "repository-source mapping is not yet verified"
				}
				if !strings.Contains(got[0], want) {
					t.Errorf("fallback must retain source boundary without claiming missing input: %s", got[0])
				}
				if strings.Contains(got[0], `"d"`) || strings.Contains(got[0], "“d”") {
					t.Fatal("bounded three-name rendering changed")
				}
			})
		}
	}
}

package tool_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1700SymbolFallbackContext(t *testing.T, role types.AnswerCandidateRole) *types.BusContext {
	t.Helper()
	root := t.TempDir()
	return &types.BusContext{RepoRoot: root, WorkDir: filepath.Join(root, ".codrax"), Mutable: types.NewMutableState("enumerate source members"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentEnumerate,
			Predicates:             types.SemanticPredicates{IsCategoryEnumeration: true},
			SourceInventoryProfile: &types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{role}, Confidence: 0.95},
		}}}
}

func b1700SymbolFallbackFact(name, file string, line int) types.AnswerAggregateFact {
	return types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Role: types.AnswerAggregateRolePrincipalAnswer,
		Label: "selected source members", Value: "1", Members: []string{name}, SupportRefs: []string{fmt.Sprintf("%s @ %s:%d", name, file, line)},
		MemberNotes: []string{"retained model-owned explanation"}, Provenance: "system:source_inventory"}
}

func b1700InvokeEmptySymbolFallback(t *testing.T, ctx *types.BusContext, fact types.AnswerAggregateFact, completeness string) types.ToolResult {
	t.Helper()
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{fact})
	ctx.Mutable.RetainInvestigationAggregateFacts()
	before := ctx.Mutable.StableInvestigationAggregateFacts()
	payload, _ := json.Marshal(map[string]any{"items": []any{}, "completeness": completeness})
	result, err := (&tool.EmitAnswerSymbol{}).Execute(ctx, payload)
	if err != nil || !result.Success {
		t.Fatalf("empty slate is a valid completion shape; observation qualification must not invent an error: %v %+v", err, result)
	}
	if !reflect.DeepEqual(before, ctx.Mutable.StableInvestigationAggregateFacts()) {
		t.Fatal("fallback admission must not rewrite model members, notes or references")
	}
	return result
}

func TestB1700ActualSymbolFallbackRejectsUnobservedMembersAcrossOrigins(t *testing.T) {
	for _, role := range []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleType} {
		for _, completeness := range []string{"unknown", "complete"} {
			for _, origin := range []string{"untyped", "runtime", "mcp", "mixed"} {
				t.Run(string(role)+"/"+completeness+"/"+origin, func(t *testing.T) {
					ctx := b1700SymbolFallbackContext(t, role)
					fact := b1700SymbolFallbackFact("NeverObserved", "unread.go", 4)
					switch origin {
					case "runtime":
						fact.Dimensions = []types.AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}}
						fact.SupportRefs = append(fact.SupportRefs, "trace_query:window_stats:E7")
					case "mcp":
						fact.Dimensions = []types.AnswerAggregateDimension{{Name: "origin", Value: "mcp_resource"}}
						fact.SupportRefs = append(fact.SupportRefs, "mcp_resource: mcp://fixture/report#L7")
					case "mixed":
						fact.Dimensions = []types.AnswerAggregateDimension{{Name: "origin", Value: "current_source"}, {Name: "origin", Value: "runtime_artifact"}}
						fact.SupportRefs = append(fact.SupportRefs, "trace_query:window_stats:E7")
					}
					result := b1700InvokeEmptySymbolFallback(t, ctx, fact, completeness)
					syms, claim, selection := ctx.Mutable.EmittedAnswerSymbolsWithOrigin()
					if len(syms) != 0 || selection == types.AnswerSymbolSelectionInventoryMaterialized ||
						completeness == "unknown" && claim != types.CompletenessUnknown || strings.Contains(result.Summary, "materialized 1 answer-symbol") {
						t.Fatalf("unobserved model reference minted a source slate: symbols=%+v claim=%s origin=%s result=%s", syms, claim, selection, result.Summary)
					}
				})
			}
		}
	}
}

func b1700SymbolFallbackSource(t *testing.T, role types.AnswerCandidateRole) (*types.BusContext, string, int) {
	t.Helper()
	ctx := b1700SymbolFallbackContext(t, role)
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "declarations.go"), []byte("package fixture\ntype ObservedKind struct{}\nfunc ObservedWork() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if role == types.AnswerCandidateRoleType {
		return ctx, "ObservedKind", 2
	}
	return ctx, "ObservedWork", 3
}

func b1700RequireObservedSymbolFallback(t *testing.T, ctx *types.BusContext, name string, line int) {
	t.Helper()
	result := b1700InvokeEmptySymbolFallback(t, ctx, b1700SymbolFallbackFact(name, "declarations.go", line), "unknown")
	syms, claim, selection := ctx.Mutable.EmittedAnswerSymbolsWithOrigin()
	if len(syms) != 1 || syms[0].Name != name || syms[0].File != "declarations.go" || syms[0].Line != line || claim != types.CompletenessComplete || selection != types.AnswerSymbolSelectionInventoryMaterialized {
		t.Fatalf("independently observed member lost deterministic materialization: %+v %s %s; %s", syms, claim, selection, result.Summary)
	}
	// B1604 origin ownership remains tied to the actual accepted producer,
	// not whichever request profile happens to be active after the handoff.
	ctx.AnalysisIR.RequestModel.SourceInventoryProfile = nil
	forkSymbols, forkClaim, forkOrigin := ctx.Mutable.ForkForExploreDispatch().EmittedAnswerSymbolsWithOrigin()
	if !reflect.DeepEqual(forkSymbols, syms) || forkClaim != claim || forkOrigin != types.AnswerSymbolSelectionInventoryMaterialized {
		t.Fatalf("profile change/fork lost actual materialization ownership: %+v %s %s", forkSymbols, forkClaim, forkOrigin)
	}
}

func TestB1700ActualInventoryStillMaterializesSourceSymbols(t *testing.T) {
	for _, role := range []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleType} {
		t.Run(string(role), func(t *testing.T) {
			ctx, name, line := b1700SymbolFallbackSource(t, role)
			params, _ := json.Marshal(map[string]any{"path": ".", "view": "source_inventory", "roles": []types.AnswerCandidateRole{role}})
			inventory, err := (&repomap.RepoMapV2{}).Execute(ctx, params)
			if err != nil || !inventory.Success || inventory.SourceInventory == nil || !inventory.SourceInventory.Active {
				t.Fatalf("native source inventory failed: %v %+v", err, inventory)
			}
			ctx.Mutable.AppendDispatchToolResult(inventory)
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("inventory positive must not manufacture grounded EvidenceItems")
			}
			b1700RequireObservedSymbolFallback(t, ctx, name, line)
		})
	}
}

func TestB1700ActualReadThenEmitStillMaterializesSourceSymbols(t *testing.T) {
	for _, role := range []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleType} {
		t.Run(string(role), func(t *testing.T) {
			ctx, name, line := b1700SymbolFallbackSource(t, role)
			read, err := (&tool.ReadFile{}).Execute(ctx, json.RawMessage(`{"path":"declarations.go","limit":20}`))
			if err != nil || !read.Success || read.ReadCoverage == nil {
				t.Fatalf("real source read failed: %v %+v", err, read)
			}
			ctx.Mutable.AppendDispatchToolResult(read)
			params, _ := json.Marshal(map[string]any{"items": []map[string]any{{"evidence_kind": "direct", "scope": "line", "source": "declarations.go", "line_start": line, "anchor_kind": "definition", "anchor_symbol": name, "subject": name, "summary": "declares the source member"}}})
			emitted, err := (&tool.EmitEvidence{}).Execute(ctx, params)
			if err != nil || !emitted.Success || len(ctx.Mutable.EmittedEvidence()) != 1 {
				t.Fatalf("real grounded emit failed: %v %+v", err, emitted)
			}
			status := ctx.Mutable.EmittedEvidence()[0].GroundingStatus
			if status != types.GroundingGrounded && status != types.GroundingRecovered {
				t.Fatalf("producer did not establish the source proof: %s", status)
			}
			b1700RequireObservedSymbolFallback(t, ctx, name, line)
		})
	}
}

package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The real zero-origin fixture is intentionally shared with the failed live
// audit: native physical identity and scheduler quantities are obtained from
// TraceQuery, not manufactured as ObservationRecords by this regression.
func artifactIdentityPublicTrace(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../eval/cases/trace_query_zero_origin_wait_account.case")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, opened := strings.Cut(string(data), "HTRACE='")
	raw, _, closed := strings.Cut(rest, "\n'\n")
	if !opened || !closed || !strings.Contains(raw, "0.000000: sched_switch:") {
		t.Fatal("zero-origin eval must still carry a physical scheduler trace")
	}
	return raw + "\n"
}

func artifactIdentityPublicQuery(t *testing.T, ctx *types.BusContext, path string) {
	t.Helper()
	for _, view := range []string{"thread_timeline", "window_stats"} {
		result := businessRefTestQuery(t, ctx, map[string]any{
			"source": "path", "path": path, "view": view, "pid": 41,
			"time_start": 0, "time_end": .010, "limit": 32,
			"trace_flavor": "harmony_hitrace",
		})
		if !result.Success || len(result.Observations) == 0 {
			t.Fatalf("native %s failed: %s", view, result.Summary)
		}
	}
}

func artifactIdentityPublicLedger(ctx *types.BusContext) types.ObservationLedger {
	return types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
}

func artifactIdentityPublicComplete(t *testing.T, ctx *types.BusContext, dims []types.AnswerAggregateDimension) types.AnswerAggregateFact {
	t.Helper()
	fact := types.AnswerAggregateFact{
		Kind: types.AnswerAggregateScalar, Label: "Observed running time", Value: "7", Unit: "ms",
		Role: types.AnswerAggregateRolePrincipalAnswer, Provenance: "trace_query:thread_timeline+window_stats", Dimensions: dims,
	}
	raw, err := json.Marshal(map[string]any{
		"reason": "The selected native scheduler intervals are measured", "confidence": "high", "result_kind": "resolved",
		"aggregate_facts": []types.AnswerAggregateFact{fact},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&EmitInvestigationComplete{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("public investigation completion failed: %v; %s", err, result.Summary)
	}
	got := ctx.Mutable.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Kind != fact.Kind || got[0].Label != fact.Label || got[0].Value != fact.Value || got[0].Unit != fact.Unit {
		t.Fatalf("physical-identity qualification may not discard the model's quantitative fact: got %+v want %+v", got, fact)
	}
	// Existing completion normalization may demote an unbound model summary
	// to advisory support and append its origin. That valid boundary is not
	// what this test changes; the caller's dimensions must remain lossless.
	for _, dim := range dims {
		found := false
		for _, actual := range got[0].Dimensions {
			found = found || actual == dim
		}
		if !found {
			t.Fatalf("model dimension was discarded: %+v in %+v", dim, got[0])
		}
	}
	return got[0]
}

func TestRuntimeArtifactIdentityPublicModelScopeCannotMintCapture(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name string
			dims []types.AnswerAggregateDimension
		}{
			{"thread_scope", []types.AnswerAggregateDimension{{Name: "scope", Value: "client-41_pid=41"}}},
			{"business_scope", []types.AnswerAggregateDimension{{Name: "scope", Value: "订单响应#8"}}},
			{"asserted_path", []types.AnswerAggregateDimension{{Name: "scope", Value: "client-41_pid=41"}, {Name: "path", Value: "/captures/unproven.systrace"}}},
			{"explicit_artifact_token", []types.AnswerAggregateDimension{{Name: "artifact_id", Value: "unproven-capture"}, {Name: "artifact_kind", Value: "trace"}}},
			{"no_identity_control", nil},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				ctx, path := businessRefTestContext(t, artifactIdentityPublicTrace(t))
				optimizationCaliberPublicRequest(ctx, path, lang, "client-41", 41, 0, .010, true)
				ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
					SourceQuotes:      []string{"Only inspect this trace"}, Confidence: 1,
				}
				ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
				artifactIdentityPublicQuery(t, ctx, path)
				native := artifactIdentityPublicLedger(ctx)
				if authority := types.BuildRuntimeArtifactPairRelationAuthority(native); authority.Active {
					t.Fatalf("one real native capture already became a pair: %+v", authority)
				}
				fact := artifactIdentityPublicComplete(t, ctx, tc.dims)
				ledger := artifactIdentityPublicLedger(ctx)
				var modelRecords int
				for _, record := range ledger.Records {
					if record.Summary == fact.Label {
						modelRecords++
						if record.ClaimAuthority != types.ObservationClaimAuthorityModelInference || record.Value != "7" || record.Unit != "ms" {
							t.Errorf("model aggregate fact must remain model-owned and quantitatively intact: %+v", record)
						}
					}
				}
				if modelRecords != 1 {
					t.Fatalf("model observation was lost or duplicated: %d", modelRecords)
				}
				if authority := types.BuildRuntimeArtifactPairRelationAuthority(ledger); authority.Active {
					t.Errorf("model-authored scope/provenance minted an independent physical capture: %+v", authority)
				}
				face, _ := optimizationCaliberPublicPublish(t, ctx, path, lang, true)
				for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
					if block.ID == runtimeArtifactPairRelationAuthorityBlockID && RuntimeTraceSystemBlock(block) {
						t.Errorf("public answer publication manufactured a cross-artifact relation: %s", face)
					}
				}
				for _, original := range native.Records {
					found := false
					for _, after := range artifactIdentityPublicLedger(ctx).Records {
						if original.ID == after.ID && reflect.DeepEqual(original, after) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("native measurement or provenance changed: %s", original.ID)
					}
				}
			})
		}
	}
}

func TestRuntimeArtifactIdentityPublicTwoNativeCapturesRemainDistinct(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", lang, reverse), func(t *testing.T) {
				ctx, first := businessRefTestContext(t, artifactIdentityPublicTrace(t))
				_, second := businessRefTestContext(t, artifactIdentityPublicTrace(t))
				optimizationCaliberPublicRequest(ctx, first, lang, "client-41", 41, 0, .010, true)
				ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Source: second, Carrier: "path"})
				paths := []string{first, second}
				if reverse {
					paths[0], paths[1] = paths[1], paths[0]
				}
				for _, path := range paths {
					artifactIdentityPublicQuery(t, ctx, path)
				}
				authority := types.BuildRuntimeArtifactPairRelationAuthority(artifactIdentityPublicLedger(ctx))
				if !authority.Active || len(authority.Artifacts) != 2 || len(authority.Pairs) != 1 {
					t.Fatalf("two genuine native capture identities must remain distinct: %+v", authority)
				}
				pair := authority.Pairs[0]
				if pair.DirectTimeAlignment != types.RuntimeArtifactPairRelationUnproven || pair.SharedClockOrigin != types.RuntimeArtifactPairRelationUnproven {
					t.Error("equal native clock labels/timestamps must not prove a cross-capture clock relation")
				}
				face, _ := optimizationCaliberPublicPublish(t, ctx, first, lang, true)
				block := projectionClusterBlock(ctx.Mutable.AnswerDocumentV2().Blocks, runtimeArtifactPairRelationAuthorityBlockID)
				if block == nil || !RuntimeTraceSystemBlock(*block) || len(block.Items) != 1 || !strings.Contains(face, "↔") {
					t.Fatalf("public publication lost the genuine two-capture boundary: %s", face)
				}
			})
		}
	}
}

func TestRuntimeArtifactIdentityPublicModelCannotReparentNativeCapture(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, carrier := range []string{"payload_ref", "raw_ref", "row_set_ref", "page_ref"} {
			t.Run(lang+"/"+carrier, func(t *testing.T) {
				ctx, first := businessRefTestContext(t, artifactIdentityPublicTrace(t))
				_, second := businessRefTestContext(t, artifactIdentityPublicTrace(t))
				first, second = selfRunningScopePublicPath(t, first), selfRunningScopePublicPath(t, second)
				optimizationCaliberPublicRequest(ctx, first, lang, "client-41", 41, 0, .010, true)
				ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
					SourceQuotes:      []string{"Only inspect these traces"}, Confidence: 1,
				}
				ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
				ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Source: second, Carrier: "path"})
				artifactIdentityPublicQuery(t, ctx, first)
				artifactIdentityPublicQuery(t, ctx, second)
				before := types.BuildRuntimeArtifactPairRelationAuthority(artifactIdentityPublicLedger(ctx))
				if len(before.Artifacts) != 2 || len(before.Pairs) != 1 {
					t.Fatalf("expected two independently parsed captures before model completion: %+v", before)
				}
				// Merely repeating an existing path does not grant this model fact
				// authority to declare the other physical capture its derived output.
				artifactIdentityPublicComplete(t, ctx, []types.AnswerAggregateDimension{
					{Name: "scope", Value: "client-41_pid=41"}, {Name: "path", Value: first}, {Name: carrier, Value: second},
				})
				after := types.BuildRuntimeArtifactPairRelationAuthority(artifactIdentityPublicLedger(ctx))
				if !reflect.DeepEqual(before, after) {
					t.Errorf("model-owned carrier metadata reparented a real native capture: before %+v after %+v", before, after)
				}
				face, _ := optimizationCaliberPublicPublish(t, ctx, first, lang, true)
				block := projectionClusterBlock(ctx.Mutable.AnswerDocumentV2().Blocks, runtimeArtifactPairRelationAuthorityBlockID)
				if block == nil || !RuntimeTraceSystemBlock(*block) || len(block.Items) != 1 {
					t.Fatalf("model carrier claim suppressed the real cross-capture boundary: %s", face)
				}
			})
		}
	}
}

package agent

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These are the declaration identities in cangjie_minimal used by r1027.
// Cart's class and extension deliberately share a name and file, not a row.
func mechanicalDeclarationHandoffContext(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx := sourceInventoryMechanicalLandingContextForExplorerTest(false)
	obs := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	obs.SourceClasses = nil
	obs.Sets = nil
	for _, declaration := range []struct {
		name, file, family, pkg string
		line, packageLine       int
		role                    types.AnswerCandidateRole
	}{
		{"Cart", "cart/Cart.cj", "public class", "demo.cart", 14, 4, types.AnswerCandidateRoleType},
		{"Cart", "cart/Cart.cj", "extend", "demo.cart", 30, 4, types.AnswerCandidateRoleType},
		{"Bridge", "bridge/Bridge.cj", "public class", "demo.bridge", 15, 4, types.AnswerCandidateRoleType},
		{"App", "main.cj", "public class", "demo.app", 11, 6, types.AnswerCandidateRoleType},
		{"native_add", "bridge/Bridge.cj", "foreign func", "demo.bridge", 6, 4, types.AnswerCandidateRoleFunction},
	} {
		member := types.SourceInventoryObservationMember{
			Name: declaration.name, Key: fmt.Sprintf("%s:%d", declaration.file, declaration.line),
			Role: declaration.role, File: declaration.file, Line: declaration.line,
			Language: "cangjie", CoverageState: types.SourceInventoryCoverageObserved,
			SupportRef:   fmt.Sprintf("%s: %s:%d", declaration.name, declaration.file, declaration.line),
			Provenance:   []string{"repo_lens:direct_children"},
			SurfaceTerms: []string{declaration.family, declaration.family + " " + declaration.name},
			Attributes: []types.SourceInventoryObservationAttribute{{
				Role: types.AnswerCandidateRolePackage, Name: declaration.pkg,
				File: declaration.file, Line: declaration.packageLine,
				CoverageState: types.SourceInventoryCoverageObserved,
			}},
		}
		index := -1
		for i := range obs.Sets {
			if obs.Sets[i].Role == declaration.role {
				index = i
			}
		}
		if index < 0 {
			obs.Sets = append(obs.Sets, types.SourceInventoryObservationSet{Role: declaration.role, Complete: true})
			index = len(obs.Sets) - 1
		}
		obs.Sets[index].Members = append(obs.Sets[index].Members, member)
		obs.Sets[index].Count++
		obs.Sets[index].Total++
	}
	ctx.Mutable = types.NewMutableState("mechanical declaration handoff")
	ctx.Mutable.SetSourceInventoryObservation(obs)
	ctx.AnalysisIR.RequestModel.SourceInventoryProfile.TargetRoles = []types.AnswerCandidateRole{
		types.AnswerCandidateRoleType, types.AnswerCandidateRoleFunction,
	}
	return ctx
}

func TestMechanicalInventoryHandoffExactDeclarationRowsReachExplorerAndFinalizer(t *testing.T) {
	ctx := mechanicalDeclarationHandoffContext(t)
	e := &explorerEvaluator{sourceInventoryLensSurfaceReleased: true}
	if !e.sourceInventoryMechanicalLandingSurfaceActive(ctx) {
		t.Fatal("fixture must enter the existing mechanical landing lane")
	}
	before := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	explorer := e.BuildInitialInstruction(ctx, nil)
	finalizer := renderAnswerDocSourceInventoryHandoff(ctx)
	for _, set := range before.Sets {
		for _, member := range set.Members {
			var found string
			for _, line := range strings.Split(explorer, "\n") {
				if strings.Contains(line, "member=`"+member.Name+"`") &&
					strings.Contains(line, fmt.Sprintf("location=`%s:%d`", member.File, member.Line)) {
					found = line
				}
			}
			if found == "" {
				t.Fatalf("mechanical landing lost exact declaration %s:%d:\n%s", member.File, member.Line, explorer)
			}
			for _, want := range []string{
				"surface_family=`" + types.SourceInventorySurfaceFamilyKey(member.SurfaceTerms) + "`",
				"support_ref=`" + member.SupportRef + "`",
				"package:" + member.Attributes[0].Name,
			} {
				if !strings.Contains(found, want) {
					t.Errorf("declaration handoff lost %q: %s", want, found)
				}
			}
			if !strings.Contains(finalizer, found) {
				t.Errorf("explorer and finalizer must share the same declaration row: %s", found)
			}
		}
	}
	if strings.Count(explorer, "- member=") != 5 {
		t.Fatalf("expected five identities, not same-name deduplication:\n%s", explorer)
	}
	if ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("rendering context must not complete the model's investigation")
	}
	if after := types.SourceInventoryObservationFromMutable(ctx.Mutable); !reflect.DeepEqual(before, after) {
		t.Fatal("rendering context must not change source inventory facts")
	}
	schemas := e.FilterToolSchemas(ctx, []llm.ToolSchema{{Name: "read_file"}, {Name: "emit_investigation_complete"}})
	if len(schemas) != 1 || schemas[0].Name != "emit_investigation_complete" {
		t.Fatalf("information repair must not widen the existing tool surface: %+v", schemas)
	}
	for _, unavailable := range []string{"read_file", "grep", "emit_evidence"} {
		if strings.Contains(explorer, unavailable) {
			t.Errorf("closed landing prompt teaches unavailable tool %q:\n%s", unavailable, explorer)
		}
	}
}

func TestMechanicalInventoryHandoffPreservesIndependentSurfaceFamilies(t *testing.T) {
	ctx := mechanicalDeclarationHandoffContext(t)
	obs := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	obs.Sets[0].Members[0].SurfaceTerms = []string{"@Entry", "@Component", "struct", "struct Cart"}
	obs.Sets[0].Members[0].Language = "arkts"
	ctx.Mutable.SetSourceInventoryObservation(obs)
	e := &explorerEvaluator{sourceInventoryLensSurfaceReleased: true}
	for surface, out := range map[string]string{
		"explorer":  e.BuildInitialInstruction(ctx, nil),
		"finalizer": renderAnswerDocSourceInventoryHandoff(ctx),
	} {
		for _, want := range []string{"surface_families=`@entry`, `@component`, `struct`", "language=arkts", "surface_family=`extend`"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s omitted independent typed family %q:\n%s", surface, want, out)
			}
		}
	}
}

func TestMechanicalInventoryHandoffDisclosesBoundedRowsAndAttributes(t *testing.T) {
	ctx := mechanicalDeclarationHandoffContext(t)
	obs := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	for i := 0; i < 38; i++ {
		member := obs.Sets[0].Members[0]
		member.Name, member.Key = fmt.Sprintf("Extra%d", i), fmt.Sprintf("extra:%d", i)
		member.File, member.Line = "extra.cj", i+1
		member.SupportRef = fmt.Sprintf("%s: extra.cj:%d", member.Name, member.Line)
		member.SurfaceTerms = []string{"public class", "public class " + member.Name}
		obs.Sets[0].Members = append(obs.Sets[0].Members, member)
	}
	obs.Sets[0].Count, obs.Sets[0].Total = len(obs.Sets[0].Members), len(obs.Sets[0].Members)
	for i := 0; i < 5; i++ {
		obs.Sets[1].Members[0].Attributes = append(obs.Sets[1].Members[0].Attributes,
			types.SourceInventoryObservationAttribute{Role: types.AnswerCandidateRolePackage, Name: fmt.Sprintf("attr%d", i)})
	}
	ctx.Mutable.SetSourceInventoryObservation(obs)
	e := &explorerEvaluator{sourceInventoryLensSurfaceReleased: true}
	out := e.BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"principal=43 (+11 hidden)", "11 additional principal row(s)", "+2 more", "omitted from this bounded prompt view"} {
		if !strings.Contains(out, want) {
			t.Errorf("bounded prompt must honestly disclose %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "- member=") != 32 {
		t.Fatalf("existing snapshot row bound must stay unchanged: %d", strings.Count(out, "- member="))
	}
}

func TestMechanicalInventoryHandoffDoesNotClaimOtherLanes(t *testing.T) {
	for _, kind := range []string{"source_text", "lens_not_released", "ordinary_relation", "missing_required_file"} {
		t.Run(kind, func(t *testing.T) {
			ctx := mechanicalDeclarationHandoffContext(t)
			e := &explorerEvaluator{sourceInventoryLensSurfaceReleased: true}
			switch kind {
			case "source_text":
				ctx.AnalysisIR.RequestModel.SourceInventoryProfile.RequestedFields = append(ctx.AnalysisIR.RequestModel.SourceInventoryProfile.RequestedFields, types.SourceInventoryFieldSummary)
			case "lens_not_released":
				ctx.ExploreToolSurface = types.ExploreToolSurfaceSourceInventoryLens
				e.sourceInventoryLensSurfaceReleased = false
			case "ordinary_relation":
				ctx.AnalysisIR.RequestModel.SourceInventoryProfile = nil
			case "missing_required_file":
				ctx.AnalysisIR.EvidencePlan.RequiredFiles = []string{"not_observed.cj"}
			}
			if e.sourceInventoryMechanicalLandingSurfaceActive(ctx) {
				t.Fatal("fixture must remain outside mechanical landing")
			}
			if out := e.BuildInitialInstruction(ctx, nil); strings.Contains(out, "## Mechanical source inventory handoff") {
				t.Fatalf("mechanical handoff must not claim an unrelated lane:\n%s", out)
			}
		})
	}
}

func TestMechanicalInventoryHandoffPreservesTypedRelationPrincipalAuthority(t *testing.T) {
	ctx := mechanicalDeclarationHandoffContext(t)
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "selected implementations", Value: "1",
		Role:       types.AnswerAggregateRolePrincipalAnswer,
		Provenance: types.TypedRelationPrincipalMemberSetAggregateProvenance,
		Members:    []string{"Cart"}, SupportRefs: []string{"Cart @ cart/Cart.cj:14"},
	}})
	ctx.Mutable.SetInvestigationComplete("typed relation set accepted")
	ctx.Mutable.SetInvestigationResultKind("resolved")
	ctx.Mutable.RetainInvestigationAggregateFacts()
	e := &explorerEvaluator{sourceInventoryLensSurfaceReleased: true}
	if e.sourceInventoryMechanicalLandingSurfaceActive(ctx) {
		t.Fatal("inventory rows must not override the accepted typed relation principal set")
	}
	if out := e.BuildInitialInstruction(ctx, nil); strings.Contains(out, "## Mechanical source inventory handoff") {
		t.Fatalf("mechanical handoff claimed the ordinary relation answer:\n%s", out)
	}
}

func TestMechanicalInventoryHandoffDoesNotReintroduceExcludedSource(t *testing.T) {
	ctx := requestedDimensionEvidenceOwnershipContext()
	ctx.AnalysisIR.RequestModel.PerfTrace = &types.PerfBundle{Observations: []types.PerfObservation{{Kind: "root_cause_rank", Subject: "app-17"}}}
	ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"opaque anchored user boundary"},
	}
	// A retained navigation observation is not a new source-inventory request.
	ctx.Mutable = types.NewMutableState("runtime-only with source navigation")
	ctx.Mutable.SetSourceInventoryObservation(types.SourceInventoryObservationFromMutable(mechanicalDeclarationHandoffContext(t).Mutable))
	before := ctx.AnalysisIR.RequestModel
	e := &explorerEvaluator{sourceInventoryLensSurfaceReleased: true}
	if e.sourceInventoryMechanicalLandingSurfaceActive(ctx) {
		t.Fatal("retained navigation rows must not claim runtime-only answer authority")
	}
	out := e.BuildInitialInstruction(ctx, nil)
	for _, unexpected := range []string{"## Mechanical source inventory handoff", "### Requested Explanation Evidence Ownership"} {
		if strings.Contains(out, unexpected) {
			t.Errorf("runtime-only prompt reopened source authority %q:\n%s", unexpected, out)
		}
	}
	if !reflect.DeepEqual(before, ctx.AnalysisIR.RequestModel) {
		t.Fatal("information handoff must not rewrite the typed source exclusion or requested dimensions")
	}
}

func TestMechanicalInventorySharedRendererPreservesLegacyFamilyAndAllMarkers(t *testing.T) {
	row := types.SourceInventoryRow{
		Role: types.AnswerCandidateRoleType, SurfaceFamily: "legacy family",
		Member: types.SourceInventoryObservationMember{
			Name: "Page", File: "Page.ets", Line: 8,
			SurfaceTerms: []string{"@Entry", "@Component"},
		},
	}
	var b strings.Builder
	renderSourceInventoryRows(&b, []types.SourceInventoryRow{row}, false)
	for _, want := range []string{"surface_family=`legacy family`", "surface_families=`@entry`, `@component`", "location=`Page.ets:8`"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("shared renderer dropped exact typed field %q: %s", want, b.String())
		}
	}
}

func TestMechanicalInventorySharedRendererBoundsAdditionalFamilies(t *testing.T) {
	terms := []string{"@Entry", "@" + strings.Repeat("x", 2000)}
	for i := 0; i < 60; i++ {
		terms = append(terms, fmt.Sprintf("@Marker%d", i))
	}
	row := types.SourceInventoryRow{
		Role: types.AnswerCandidateRoleType, SurfaceFamily: "@entry",
		Member: types.SourceInventoryObservationMember{Name: "Page", SurfaceTerms: terms},
	}
	before := append([]string(nil), terms...)
	var b strings.Builder
	renderSourceInventoryRows(&b, []types.SourceInventoryRow{row}, false)
	out := b.String()
	for _, want := range []string{"surface_family=`@entry`", "surface_families=`@entry`", "+1905 characters omitted", "+54 families omitted"} {
		if !strings.Contains(out, want) {
			t.Errorf("bounded family display omitted disclosure %q: %s", want, out)
		}
	}
	if len(out) > 1200 || strings.Contains(out, strings.Repeat("x", 2000)) || strings.Contains(out, "surface_terms=") {
		t.Fatalf("row display duplicated unbounded raw terms (%d bytes): %s", len(out), out)
	}
	if !reflect.DeepEqual(before, row.Member.SurfaceTerms) {
		t.Fatal("display clipping must not change the typed family carrier")
	}
}

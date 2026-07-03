// Package agent — declarative_trigger_test.go (2026-05-10).
//
// L1 of the forced-read remediation: the declarative-registration
// path no longer fires on every "enumeration && isEnumeration"
// question. It now requires PrimaryEntities to structurally land in
// declarative-shaped files (Registry / Routes / Manifest / Defaults /
// Wire / Topology / Schema), reusing the existing
// `declarative.Classifier`.
//
// Test invariants:
//  1. Function-body enumerations (s1a regression) → no trigger.
//  2. Real registry enumerations → still trigger.
//  3. registration / config_mapping kinds → unchanged (still
//     trigger unconditionally).
//  4. AxisRegister bypass preserved.
//  5. Empty PrimaryEntities → fail-open.
//  6. Nil graph → fail-open.
package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/declarative"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// === declarativeFocusRelevant ===

func TestDeclarativeFocusRelevant_RegistrationKind_AlwaysFires(t *testing.T) {
	if !declarativeFocusRelevant("registration", false, types.AxisUnknown, false) {
		t.Error("registration kind should trigger unconditionally")
	}
}

func TestDeclarativeFocusRelevant_ConfigMappingKind_AlwaysFires(t *testing.T) {
	if !declarativeFocusRelevant("config_mapping", false, types.AxisUnknown, false) {
		t.Error("config_mapping kind should trigger unconditionally")
	}
}

func TestDeclarativeFocusRelevant_EnumerationWithRegistrationShape_Fires(t *testing.T) {
	if !declarativeFocusRelevant("enumeration", true, types.AxisDefine, true) {
		t.Error("enumeration + isEnumeration + registration-shape should trigger")
	}
}

// s1a regression — function-body enumeration must NOT trigger.
func TestDeclarativeFocusRelevant_S1ARegression_FunctionBodyEnumeration_NoTrigger(t *testing.T) {
	// s1a: question_kind=enumeration, isEnumeration=true,
	// axis=define, primaryEntitiesRegistrationShape=false (gate.Run /
	// GateCheck land in non-registry files).
	if declarativeFocusRelevant("enumeration", true, types.AxisDefine, false) {
		t.Error("s1a regression: function-body enumeration must NOT trigger declarative path")
	}
}

func TestDeclarativeFocusRelevant_EnumerationWithAxisRegister_Bypasses(t *testing.T) {
	// AxisRegister is an explicit register signal from analyzer.
	// Even when primaryEntitiesRegistrationShape is false, AxisRegister
	// preserves the legacy trigger.
	if !declarativeFocusRelevant("enumeration", true, types.AxisRegister, false) {
		t.Error("AxisRegister should bypass the primaryEntitiesRegistrationShape gate")
	}
}

func TestDeclarativeFocusRelevant_NonEnumerationKind_FollowsLegacyDefault(t *testing.T) {
	// Default branch: trigger only when isEnumeration AND AxisRegister.
	// Legacy semantic preserved (the L1 tightening only touched the
	// "enumeration" case branch, not the default).
	if declarativeFocusRelevant("mechanism", true, types.AxisDefine, true) {
		t.Error("mechanism + AxisDefine should not trigger (default branch needs AxisRegister)")
	}
	if !declarativeFocusRelevant("mechanism", true, types.AxisRegister, false) {
		t.Error("mechanism + isEnumeration + AxisRegister should trigger (legacy default)")
	}
	if declarativeFocusRelevant("mechanism", false, types.AxisRegister, true) {
		t.Error("mechanism + AxisRegister but isEnumeration=false should not trigger")
	}
}

func TestDeclarativeFocusRelevant_FallbackBranch_RequiresBothEnumAndRegister(t *testing.T) {
	// The default branch (unknown question_kind) requires both
	// isEnumeration AND axis==AxisRegister. primaryRegistrationShape
	// is irrelevant on this fallback path.
	if !declarativeFocusRelevant("", true, types.AxisRegister, false) {
		t.Error("unknown kind + isEnumeration + AxisRegister should trigger (legacy fallback)")
	}
	if declarativeFocusRelevant("", true, types.AxisDefine, true) {
		t.Error("unknown kind without AxisRegister should not trigger")
	}
}

// === primaryEntitiesLookLikeRegistration ===

func TestPrimaryEntitiesLookLikeRegistration_NilIR_FailOpen(t *testing.T) {
	got := primaryEntitiesLookLikeRegistration(nil, nil, map[declarative.Kind]bool{declarative.KindRegistry: true})
	if !got {
		t.Error("nil ir should fail-open (return true)")
	}
}

func TestPrimaryEntitiesLookLikeRegistration_NilGraph_FailOpen(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnalyzerHints.PrimaryEntities = []string{"foo"}
	got := primaryEntitiesLookLikeRegistration(ir, nil, map[declarative.Kind]bool{declarative.KindRegistry: true})
	if !got {
		t.Error("nil graph should fail-open (return true)")
	}
}

func TestPrimaryEntitiesLookLikeRegistration_EmptyPrimaryEntities_FailOpen(t *testing.T) {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnalyzerHints.PrimaryEntities = nil
	graph := &repomap.Graph{}
	got := primaryEntitiesLookLikeRegistration(ir, graph, map[declarative.Kind]bool{declarative.KindRegistry: true})
	if !got {
		t.Error("empty PrimaryEntities should fail-open (return true)")
	}
}

func TestPrimaryEntitiesLookLikeRegistration_S1ARegression_NoRegistryShape(t *testing.T) {
	// s1a: PrimaryEntities resolve to non-registry files → return false.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnalyzerHints.PrimaryEntities = []string{"gate.Run", "GateCheck"}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomaptypes.FileInfo{
			"internal/analysis/gate/gate.go": {RelPath: "internal/analysis/gate/gate.go", Package: "gate"},
			"internal/types/analysis_ir.go":  {RelPath: "internal/types/analysis_ir.go", Package: "types"},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"Run": {
				{Name: "Run", File: "internal/analysis/gate/gate.go", Kind: "function"},
			},
			"GateCheck": {
				{Name: "GateCheck", File: "internal/types/analysis_ir.go", Kind: "type"},
			},
		},
	}
	allowed := map[declarative.Kind]bool{
		declarative.KindRegistry: true,
		declarative.KindRoutes:   true,
	}
	got := primaryEntitiesLookLikeRegistration(ir, graph, allowed)
	if got {
		t.Error("s1a regression: gate.Run + GateCheck don't land in registry-shape files; should return false")
	}
}

func TestPrimaryEntitiesLookLikeRegistration_RegistryEntity_True(t *testing.T) {
	// "ViolationKind" defined in a registry-shape file → return true.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnalyzerHints.PrimaryEntities = []string{"ViolationKind"}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomaptypes.FileInfo{
			"internal/types/violation_registry.go": {RelPath: "internal/types/violation_registry.go", Package: "types"},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"ViolationKind": {
				{Name: "ViolationKind", File: "internal/types/violation_registry.go", Kind: "type"},
			},
		},
	}
	allowed := map[declarative.Kind]bool{
		declarative.KindRegistry: true,
	}
	got := primaryEntitiesLookLikeRegistration(ir, graph, allowed)
	if !got {
		t.Error("ViolationKind in violation_registry.go should match registry shape")
	}
}

func TestPrimaryEntitiesLookLikeRegistration_PartialRegistryShape_True(t *testing.T) {
	// At least one entity matches → return true.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnalyzerHints.PrimaryEntities = []string{"gate.Run", "ViolationKind"}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomaptypes.FileInfo{
			"internal/analysis/gate/gate.go":       {RelPath: "internal/analysis/gate/gate.go", Package: "gate"},
			"internal/types/violation_registry.go": {RelPath: "internal/types/violation_registry.go", Package: "types"},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"Run":           {{Name: "Run", File: "internal/analysis/gate/gate.go", Kind: "function"}},
			"ViolationKind": {{Name: "ViolationKind", File: "internal/types/violation_registry.go", Kind: "type"}},
		},
	}
	allowed := map[declarative.Kind]bool{declarative.KindRegistry: true}
	got := primaryEntitiesLookLikeRegistration(ir, graph, allowed)
	if !got {
		t.Error("partial registry shape (one of two entities matches) should return true")
	}
}

func TestPrimaryEntitiesLookLikeRegistration_RoutesShape_True(t *testing.T) {
	// "RouteTable" in a routes-shape file → match.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnalyzerHints.PrimaryEntities = []string{"RouteTable"}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomaptypes.FileInfo{
			"server/routes.go": {RelPath: "server/routes.go", Package: "server"},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"RouteTable": {{Name: "RouteTable", File: "server/routes.go", Kind: "type"}},
		},
	}
	allowed := map[declarative.Kind]bool{declarative.KindRoutes: true, declarative.KindRegistry: true}
	got := primaryEntitiesLookLikeRegistration(ir, graph, allowed)
	if !got {
		t.Error("RouteTable in routes.go should match routes shape")
	}
}

func TestPrimaryEntitiesLookLikeRegistration_DottedFormResolves(t *testing.T) {
	// Cross-language: "pkg.RouteTable" should resolve via the
	// qualified-name resolver (B fix) and match.
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnalyzerHints.PrimaryEntities = []string{"server.RouteTable"}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomaptypes.FileInfo{
			"server/routes.go": {RelPath: "server/routes.go", Package: "server"},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"RouteTable": {{Name: "RouteTable", File: "server/routes.go", Kind: "type"}},
		},
	}
	allowed := map[declarative.Kind]bool{declarative.KindRoutes: true}
	got := primaryEntitiesLookLikeRegistration(ir, graph, allowed)
	if !got {
		t.Error("dotted form 'server.RouteTable' should resolve via qualified-name resolver and match")
	}
}

func TestPrimaryEntitiesLookLikeRegistration_NoSymbolMatch_False(t *testing.T) {
	// Entity doesn't resolve in graph → return false (no entity
	// landed in any file at all).
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnalyzerHints.PrimaryEntities = []string{"NonexistentSymbol"}
	graph := &repomap.Graph{
		FileIndex:  map[string]*repomaptypes.FileInfo{},
		SymbolDefs: map[string][]*repomap.Symbol{},
	}
	allowed := map[declarative.Kind]bool{declarative.KindRegistry: true}
	got := primaryEntitiesLookLikeRegistration(ir, graph, allowed)
	if got {
		t.Error("entity that doesn't resolve in graph should return false (no registration shape found)")
	}
}

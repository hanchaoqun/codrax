package types

import "testing"

// TestRouteReferenceEdgesFeedRefCount pins the previously-latent
// consumer the P0.4 design flagged: Kind="reference" had ZERO real
// producers before route edges landed, so rank_index's RefCount
// branch had never run against production data. Route->handler
// reference edges now legitimately boost the handler's name-keyed
// reference fan-in — pinned here so the behavior is a recorded
// decision, not an accident a future refactor silently drops.
func TestRouteReferenceEdgesFeedRefCount(t *testing.T) {
	files := []*FileInfo{
		{
			RelPath:  "api/routes.go",
			Language: LangGo,
			Package:  "api",
			Symbols: []Symbol{
				{Name: "GET /users/:id", Kind: "route", File: "api/routes.go", Line: 7, EndLine: 7, Exported: true},
				{Name: "GetUser", Kind: "function", File: "api/routes.go", Line: 12, EndLine: 20, Exported: true},
			},
			Relations: []Relation{
				{
					Kind:       "reference",
					FromEP:     RelationEndpoint{Name: "GET /users/:id", File: "api/routes.go", Line: 7},
					ToEP:       RelationEndpoint{Name: "GetUser", File: "api/routes.go", Line: 7},
					File:       "api/routes.go",
					Line:       7,
					Confidence: ConfidenceRouteLiteral,
					Provenance: ProvenanceRouteResolver,
					ResolvedBy: "gin_route_literal",
				},
			},
		},
	}
	g := &Graph{Root: "/repo", Files: files}
	idx := BuildRankIndex(g)
	if idx.RefCount["GetUser"] != 1 {
		t.Fatalf("route reference edge did not feed RefCount: got %d, want 1", idx.RefCount["GetUser"])
	}
}

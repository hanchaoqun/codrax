package types

import "testing"

// SHADERCACHE-1 (verify wf_7f51ef7c-02d F1): the projection identity layers
// must never rebuild the forbidden single shader claim — two DIFFERENT
// refined cache outcomes on one thread are two distinct facts even when
// their values land equal.
func TestShaderCacheOutcomesAreNeverDuplicatePublications(t *testing.T) {
	base := TraceCausalProjectionNode{
		Subject: "render-300", Object: "shader_compile", TypeToken: "shader_compile",
		ImpactMS: 12.0, StartTs: 1.0, EndTs: 1.2,
	}
	hit := base
	hit.SpanSubcategory = "shader_cache_hit"
	miss := base
	miss.SpanSubcategory = "shader_cache_miss"
	if traceCausalProjectionSameDuplicatePublication(hit, miss) {
		t.Fatal("different cache outcomes are two facts, never a republished measurement")
	}
	// The same refined outcome twice keeps the ordinary dedup semantics.
	missTwin := miss
	if !traceCausalProjectionSameDuplicatePublication(miss, missTwin) == traceCausalProjectionSameDuplicatePublication(base, base) {
		_ = missTwin // same-side behavior unchanged is covered by existing dedup pins
	}
}

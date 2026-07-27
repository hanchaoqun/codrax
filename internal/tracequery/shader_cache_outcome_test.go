package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

// shader_cache_outcome_test.go — SHADERCACHE-1 (customer ruling 2026-07-26):
// shader_compile spans come in TWO kinds — cache_hit (the compiled shader was
// served from cache) and cache_miss (real compilation happened). The subtype
// rides the EXISTING Subcategory typed lane (shader_cache_hit /
// shader_cache_miss; plain "shader" = no cache claim, 禁猜), proven either by
// the span's own name or by a child span nested inside it. The two kinds fold
// into SEPARATE rank families with their own values and label words, so the
// on-chain mention obligation binds per kind: cache_miss is the actionable
// compilation cost (precompile/cache-warm direction), cache_hit must never be
// narrated as compilation cost.

func TestShaderCacheOutcomeLexicon(t *testing.T) {
	cases := map[string]string{
		"shader_compile cache_miss":   "cache_miss",
		"ShaderCompile:CacheMiss":     "cache_miss",
		"compile shader (cache miss)": "cache_miss",
		"shader_compile cache_hit":    "cache_hit",
		"ShaderCompileCacheHit":       "cache_hit",
		"shader_compile":              "",
		"CompileShaderProgram":        "",
		"shader cache_hit cache_miss": "", // both tokens: ambiguous, no claim
		"texture cache miss":          "cache_miss",
		"":                            "",
	}
	for name, want := range cases {
		if got, _ := traceSpanShaderCacheOutcome(name); got != want {
			t.Fatalf("outcome(%q)=%q, want %q", name, got, want)
		}
	}
	// The both-token shape is typed AMBIGUOUS, distinct from plain absence.
	if _, ambiguous := traceSpanShaderCacheOutcome("shader cache_hit cache_miss"); !ambiguous {
		t.Fatal("both tokens must read as ambiguous")
	}
	if _, ambiguous := traceSpanShaderCacheOutcome("shader_compile"); ambiguous {
		t.Fatal("token absence is not ambiguity")
	}
}

func TestShaderCacheSubcategoryFromOwnName(t *testing.T) {
	spans := []TraceSpanSummary{{
		Thread: ThreadRef{Comm: "render", PID: 300}, Name: "ShaderCompile_cache_miss",
		Category: "shader_compile", Subcategory: "shader", SemanticClass: "shader_compile",
		StartTs: 1.0, EndTs: 1.01, DurationMs: 10,
	}}
	stampShaderCacheOutcomeSubcategories(spans, spans)
	if spans[0].Subcategory != "shader_cache_miss" {
		t.Fatalf("own-name cache_miss must refine the subcategory: %+v", spans[0])
	}
}

func TestShaderCacheSubcategoryFromChildSpan(t *testing.T) {
	parent := TraceSpanSummary{
		Thread: ThreadRef{Comm: "render", PID: 300}, Name: "ShaderCompile",
		Category: "shader_compile", Subcategory: "shader", SemanticClass: "shader_compile",
		StartTs: 1.0, EndTs: 1.02, DurationMs: 20,
	}
	child := TraceSpanSummary{
		Thread: ThreadRef{Comm: "render", PID: 300}, Name: "cache_hit",
		StartTs: 1.005, EndTs: 1.006, DurationMs: 1,
	}
	otherThread := TraceSpanSummary{
		Thread: ThreadRef{Comm: "other", PID: 301}, Name: "cache_miss",
		StartTs: 1.005, EndTs: 1.006, DurationMs: 1,
	}
	foreignCache := TraceSpanSummary{
		Thread: ThreadRef{Comm: "render", PID: 300}, Name: "texture cache miss",
		StartTs: 1.007, EndTs: 1.008, DurationMs: 1,
	}
	counterNoise := TraceSpanSummary{
		Thread: ThreadRef{Comm: "render", PID: 300}, Name: "FlushCacheHitCounters",
		StartTs: 1.012, EndTs: 1.013, DurationMs: 1,
	}
	otherArtifact := TraceSpanSummary{
		Thread: ThreadRef{Comm: "render", PID: 300}, Name: "cache_miss", SourcePath: "other.systrace",
		StartTs: 1.009, EndTs: 1.0095, DurationMs: 0.5,
	}
	asyncChild := TraceSpanSummary{
		Thread: ThreadRef{Comm: "render", PID: 300}, Name: "cache_miss", Kind: "async",
		StartTs: 1.014, EndTs: 1.015, DurationMs: 1,
	}
	bounded := []TraceSpanSummary{parent}
	inventory := []TraceSpanSummary{parent, child, otherThread}
	stampShaderCacheOutcomeSubcategories(bounded, inventory)
	if bounded[0].Subcategory != "shader_cache_hit" {
		t.Fatalf("a nested same-thread child span must prove the outcome: %+v", bounded[0])
	}
	// Ambiguous children (both kinds nested) make no claim.
	miss := child
	miss.Name = "cache_miss"
	miss.StartTs, miss.EndTs = 1.010, 1.011
	bounded2 := []TraceSpanSummary{parent}
	stampShaderCacheOutcomeSubcategories(bounded2, []TraceSpanSummary{parent, child, miss})
	if bounded2[0].Subcategory != "shader" {
		t.Fatalf("both outcomes nested = ambiguous = no claim: %+v", bounded2[0])
	}
	// No evidence at all: the plain subcategory stays (禁猜).
	bounded3 := []TraceSpanSummary{parent}
	stampShaderCacheOutcomeSubcategories(bounded3, []TraceSpanSummary{parent, otherThread})
	if bounded3[0].Subcategory != "shader" {
		t.Fatalf("no cache evidence must make no cache claim: %+v", bounded3[0])
	}
	// Verify-round arms (wf_7f51ef7c-02d): another subsystem's cache, token
	// noise, a different artifact, and async pairing are NEVER shader proof.
	for name, noise := range map[string]TraceSpanSummary{
		"foreign cache":  foreignCache,
		"counter noise":  counterNoise,
		"other artifact": otherArtifact,
		"async child":    asyncChild,
	} {
		bounded := []TraceSpanSummary{parent}
		stampShaderCacheOutcomeSubcategories(bounded, []TraceSpanSummary{parent, noise})
		if bounded[0].Subcategory != "shader" {
			t.Fatalf("%s must not mint a shader cache claim: %+v", name, bounded[0])
		}
	}
	// Cross-level conflict: own-name says hit, a nested child says miss —
	// silently preferring either side would mask real compilation cost.
	conflicted := parent
	conflicted.Name = "ShaderCompile_cache_hit"
	missChild := child
	missChild.Name = "shader cache_miss"
	bounded4 := []TraceSpanSummary{conflicted}
	stampShaderCacheOutcomeSubcategories(bounded4, []TraceSpanSummary{conflicted, missChild})
	if bounded4[0].Subcategory != "shader" {
		t.Fatalf("own-name vs child conflict must claim nothing: %+v", bounded4[0])
	}
}

func TestShaderCacheOutcomesFoldIntoSeparateFamilies(t *testing.T) {
	thread := ThreadRef{Comm: "render", PID: 300}
	spans := []TraceSpanSummary{
		{Thread: thread, Name: "ShaderCompile#1", Category: "shader_compile", Subcategory: "shader_cache_miss",
			SemanticClass: "shader_compile", Kind: "sync", StartTs: 1.00, EndTs: 1.02, DurationMs: 20, StartLine: 1, EndLine: 2},
		{Thread: thread, Name: "ShaderCompile#2", Category: "shader_compile", Subcategory: "shader_cache_miss",
			SemanticClass: "shader_compile", Kind: "sync", StartTs: 1.03, EndTs: 1.04, DurationMs: 10, StartLine: 3, EndLine: 4},
		{Thread: thread, Name: "ShaderCompile#3", Category: "shader_compile", Subcategory: "shader_cache_hit",
			SemanticClass: "shader_compile", Kind: "sync", StartTs: 1.05, EndTs: 1.051, DurationMs: 1, StartLine: 5, EndLine: 6},
	}
	families := FoldSemanticSpanFamilies(&ChainResult{}, spans)
	if len(families) != 2 {
		t.Fatalf("cache_miss and cache_hit must fold into SEPARATE families, got %d: %+v", len(families), families)
	}
	bySub := map[string]SemanticSpanFamily{}
	for _, fam := range families {
		if len(fam.Members) > 0 {
			bySub[fam.Members[0].Subcategory] = fam
		}
	}
	missFamily, okMiss := bySub["shader_cache_miss"]
	hitFamily, okHit := bySub["shader_cache_hit"]
	if !okMiss || !okHit {
		t.Fatalf("both outcome families must exist: %+v", bySub)
	}
	if len(missFamily.Members) != 2 || len(hitFamily.Members) != 1 {
		t.Fatalf("members must split by outcome: miss=%d hit=%d", len(missFamily.Members), len(hitFamily.Members))
	}
}

func TestShaderCacheLabelQualifiesRankSummary(t *testing.T) {
	span := TraceSpanSummary{
		Thread: ThreadRef{Comm: "render", PID: 300}, Name: "ShaderCompile_cache_miss",
		Category: "shader_compile", Subcategory: "shader_cache_miss",
		SemanticClass: "shader_compile", Kind: "sync",
		StartTs: 1.00, EndTs: 1.02, DurationMs: 20, StartLine: 1, EndLine: 2,
	}
	item, ok := rootCauseItemFromSemanticTraceSpan(Query{TimeStart: 1, TimeEnd: 2}, ChainResult{}, span, false)
	if !ok {
		t.Fatalf("classified span must mint a semantic candidate")
	}
	if item.SpanSubcategory != "shader_cache_miss" {
		t.Fatalf("the outcome must ride the typed span_subcategory lane: %+v", item)
	}
	if !strings.Contains(item.Summary, "(cache_miss)") {
		t.Fatalf("the label word must qualify the cache outcome: %q", item.Summary)
	}
	// The hit face never wears the compilation word (the obligation the
	// Description teaches would otherwise contradict the engine face).
	hitSpan := span
	hitSpan.Name = "ShaderCompile_cache_hit"
	hitSpan.Subcategory = "shader_cache_hit"
	hitItem, ok := rootCauseItemFromSemanticTraceSpan(Query{TimeStart: 1, TimeEnd: 2}, ChainResult{}, hitSpan, false)
	if !ok {
		t.Fatalf("hit span must still mint")
	}
	if !strings.Contains(hitItem.Summary, "shader cache-hit") || strings.Contains(hitItem.Summary, "shader compilation") {
		t.Fatalf("the hit label must speak cache-served work, never compilation: %q", hitItem.Summary)
	}
	// The unqualified shape stays byte-honest: no cache wording without proof.
	plain := span
	plain.Name = "ShaderCompile"
	plain.Subcategory = "shader"
	plainItem, ok := rootCauseItemFromSemanticTraceSpan(Query{TimeStart: 1, TimeEnd: 2}, ChainResult{}, plain, false)
	if !ok {
		t.Fatalf("plain shader span must still mint")
	}
	if strings.Contains(plainItem.Summary, "cache_") {
		t.Fatalf("no cache claim without proof: %q", plainItem.Summary)
	}
}

// F4 (verify wf_7f51ef7c-02d): the RELEASE WIRING pin — the stamp runs on
// the real computeTraceMarksWithInventory path, on the FULL pre-bound
// inventory (the child proving the outcome is squeezed OUT of the bounded
// display list by longer generic spans, yet still proves its parent).
// Deleting the production stamp call goes red here (单元 pin 对删挂点零判别力).
func TestShaderCacheOutcomeWiredThroughWindowStats(t *testing.T) {
	var b strings.Builder
	line := func(s string) { b.WriteString(s + "\n") }
	// Parent shader compile span 1.000..1.020 with a nested cache_miss child
	// 1.005..1.006 on the same thread.
	line(`render-300 ( 300) [000] .... 1.000000: tracing_mark_write: B|300|ShaderCompile`)
	line(`render-300 ( 300) [000] .... 1.005000: tracing_mark_write: B|300|cache_miss`)
	line(`render-300 ( 300) [000] .... 1.006000: tracing_mark_write: E|300`)
	line(`render-300 ( 300) [000] .... 1.020000: tracing_mark_write: E|300`)
	// Nine longer generic filler spans (30ms each) so the 1ms child cannot
	// hold a seat in the bounded (max=8) generic display list.
	for i := 0; i < 9; i++ {
		start := 1.030 + float64(i)*0.070
		line(fmt.Sprintf(`filler-4%02d ( 4%02d) [001] .... %.6f: tracing_mark_write: B|4%02d|filler_span_%d`, i, i, start, i, i))
		line(fmt.Sprintf(`filler-4%02d ( 4%02d) [001] .... %.6f: tracing_mark_write: E|4%02d`, i, i, start+0.030, i))
	}
	idx := buildTraceIndex(t, "shadercache_e2e.systrace", b.String())
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.0, TimeEnd: 2.0, MinDurationMs: 0.05, Limit: 8})
	var shader *TraceSpanSummary
	childVisible := false
	for i := range stats.TraceSpans {
		if stats.TraceSpans[i].Name == "ShaderCompile" {
			shader = &stats.TraceSpans[i]
		}
		if stats.TraceSpans[i].Name == "cache_miss" {
			childVisible = true
		}
	}
	if shader == nil {
		t.Fatalf("shader span must survive the semantic bound: %+v", stats.TraceSpans)
	}
	if childVisible {
		t.Fatalf("fixture precondition: the child must be squeezed below the bounded display list")
	}
	if shader.Subcategory != "shader_cache_miss" {
		t.Fatalf("the production wiring must stamp the child-proven outcome from the FULL inventory: %+v", shader)
	}
}

package tool

import (
	"strings"
	"testing"
)

// SHADERCACHE-1 word-face pin: the Description teaches the per-kind cache
// obligation — retiring or muddling the split must go red.
func TestShaderCacheDescriptionTeachesOutcomeObligation(t *testing.T) {
	desc := (&TraceQuery{}).Description()
	for _, want := range []string{
		"span_subcategory=shader_cache_miss",
		"span_subcategory=shader_cache_hit",
		"never as compilation cost",
		"never advise precompilation from hit rows",
		"never sum cache_hit and cache_miss into one shader claim",
	} {
		if !strings.Contains(desc, want) {
			t.Fatalf("description must teach the shader cache-outcome obligation, missing %q", want)
		}
	}
}

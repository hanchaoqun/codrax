package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestExtractCPreprocessorBranchesKeepDuplicateNamedFunctionBodies(t *testing.T) {
	const source = `#if defined(_WIN32)
uint64_t monotonic_now_ns(void) {
    QueryPerformanceFrequency(&freq);
    QueryPerformanceCounter(&counter);
}
#elif defined(__APPLE__)
uint64_t monotonic_now_ns(void) {
    mach_timebase_info(&info);
    return mach_absolute_time();
}
#else
uint64_t monotonic_now_ns(void) {
    clock_gettime(CLOCK_MONOTONIC, &ts);
}
#endif
`
	root := parseSourceFor(t, types.LangC, source)
	_, symbols, _, relations := extractCCpp(root, []byte(source), "clock.c", types.LangC)

	wantSymbols := map[int]bool{2: false, 7: false, 12: false}
	for _, symbol := range symbols {
		if symbol.Name != "monotonic_now_ns" || symbol.Kind != "function" {
			continue
		}
		if _, ok := wantSymbols[symbol.Line]; ok {
			wantSymbols[symbol.Line] = true
		}
	}
	for line, found := range wantSymbols {
		if !found {
			t.Fatalf("platform branch function at line %d missing; symbols=%+v", line, symbols)
		}
	}

	wantCalls := map[int]string{
		3:  "QueryPerformanceFrequency",
		4:  "QueryPerformanceCounter",
		8:  "mach_timebase_info",
		9:  "mach_absolute_time",
		13: "clock_gettime",
	}
	for _, relation := range relations {
		if relation.Kind != "call" {
			continue
		}
		if want, ok := wantCalls[relation.Line]; ok && relation.ToEP.Name == want {
			delete(wantCalls, relation.Line)
		}
	}
	if len(wantCalls) != 0 {
		t.Fatalf("preprocessor branch calls missing: want=%v relations=%+v", wantCalls, relations)
	}
}

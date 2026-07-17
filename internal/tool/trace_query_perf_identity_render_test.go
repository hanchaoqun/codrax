package tool

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func traceQueryPerfIdentityExactForTest(exact bool) *bool {
	return &exact
}

func TestTraceQueryPerfTypedIdentityRendersGenerationsAliasesAndCaveat(t *testing.T) {
	gen1 := tracequery.PerfThreadIdentity{
		TID:            77,
		TGID:           7,
		Generation:     1,
		DisplayComm:    "worker-v2",
		CommAliases:    []string{"worker-v1", "worker-v2"},
		CommAliasCount: 2,
	}
	gen2 := tracequery.PerfThreadIdentity{
		TID:            77,
		TGID:           8,
		Generation:     2,
		DisplayComm:    "reused-worker",
		CommAliases:    []string{"reused-worker"},
		CommAliasCount: 1,
	}
	ctx := tracequery.PerfContext{
		SampleCount:              2,
		TotalPeriod:              30,
		ThreadIdentityCount:      2,
		ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true),
		Caveats: []string{
			"perf identity-dependent joins were withdrawn for an ambiguous generation",
			"perf identity-dependent joins were withdrawn for an ambiguous generation",
		},
		TopSymbols: []tracequery.PerfHotspot{{
			Symbol:                   "hot",
			Period:                   30,
			SampleCount:              2,
			ThreadIdentityCount:      2,
			ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true),
			ThreadIdentities:         []tracequery.PerfThreadIdentity{gen1, gen2},
			// A conflicting legacy roster proves that the typed identity is the
			// preferred display source when both fields are present.
			Threads: []tracequery.ThreadRef{{Comm: "legacy", PID: 77}},
		}},
		TopThreads: []tracequery.PerfThreadSummary{
			{Thread: tracequery.ThreadRef{Comm: "worker-v1", PID: 77}, Identity: &gen1, Period: 20, SampleCount: 1},
			{Thread: tracequery.ThreadRef{Comm: "legacy", PID: 77}, Identity: &gen2, Period: 10, SampleCount: 1},
		},
	}
	result := tracequery.Result{
		View:        "window_stats",
		WindowStats: &tracequery.WindowStats{PerfSamples: &ctx},
	}

	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"perf_samples sample_count=2 total_sample_weight=30 thread_identity_count=2 thread_identities_omitted=0",
		"threads=[worker-v2-77@g1,reused-worker-77@g2] thread_identity_count=2 thread_identities_omitted=0",
		"perf_top_thread thread=worker-v2-77@g1",
		"perf_top_thread thread=reused-worker-77@g2",
		"perf_context_thread_identity=worker-v2-77@g1 tid=77 tgid=7 generation=1 comm_alias_count=2 comm_aliases=[worker-v1,worker-v2]",
		"perf_context_thread_identity=reused-worker-77@g2 tid=77 tgid=8 generation=2 comm_alias_count=1 comm_aliases=[reused-worker]",
		"perf_context_caveat=perf identity-dependent joins were withdrawn for an ambiguous generation",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("typed perf summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "threads=[legacy-77]") || strings.Contains(summary, "perf_top_thread thread=legacy-77 ") {
		t.Fatalf("legacy comm must not override typed (TID,generation) identity:\n%s", summary)
	}
	if got := strings.Count(summary, "perf_context_caveat=perf identity-dependent joins were withdrawn for an ambiguous generation"); got != 1 {
		t.Fatalf("renderer must defensively deduplicate exact caveat strings, got %d:\n%s", got, summary)
	}
	if got := strings.Count(summary, "- perf_top_thread thread=worker-v2-77@g1"); got != 1 {
		t.Fatalf("a same-generation rename must retain one top-thread seat, got %d:\n%s", got, summary)
	}
	if strings.Contains(summary, "- perf_top_thread thread=worker-v1-77@g1") {
		t.Fatalf("a comm alias must not re-enter the hard-identity board as another seat:\n%s", summary)
	}
	var role strings.Builder
	writeTracePerfContextRole(&role, "bundle_perf_samples", &ctx)
	if want := "summary=samples=2 sample_weight=30 thread_identity_count=2 thread_identities_omitted=0"; !strings.Contains(role.String(), want) {
		t.Fatalf("perf role compact summary lost exact identity totals %q:\n%s", want, role.String())
	}
}

func TestTraceQueryPerfTimelinePrefersTypedIdentity(t *testing.T) {
	identity := tracequery.PerfThreadIdentity{
		TID:            77,
		TGID:           7,
		Generation:     3,
		DisplayComm:    "timeline-worker",
		CommAliases:    []string{"old-worker", "timeline-worker"},
		CommAliasCount: 2,
	}
	result := tracequery.Result{
		View: "perf_timeline",
		PerfTimeline: &tracequery.PerfTimelineResult{
			BucketMs: 1,
			Caveats: []string{
				"timeline identity projection is advisory",
				"timeline identity projection is advisory",
			},
			Buckets: []tracequery.PerfTimelineBucket{
				{
					StartTs:                  1,
					EndTs:                    1.001,
					SampleCount:              1,
					Period:                   10,
					ThreadIdentityCount:      1,
					ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true),
					Threads:                  []tracequery.ThreadRef{{Comm: "legacy", PID: 77}},
					ThreadIdentities:         []tracequery.PerfThreadIdentity{identity},
				},
				{
					StartTs:                  1.001,
					EndTs:                    1.002,
					SampleCount:              1,
					Period:                   10,
					ThreadIdentityCount:      1,
					ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true),
					Threads:                  []tracequery.ThreadRef{{Comm: "legacy", PID: 77}},
					ThreadIdentities:         []tracequery.PerfThreadIdentity{identity},
				},
			},
		},
	}

	summary := traceQuerySummary(result, traceQueryParams{View: "perf_timeline"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"threads=[timeline-worker-77@g3] thread_identity_count=1 thread_identities_omitted=0",
		"perf_timeline_thread_identity=timeline-worker-77@g3 tid=77 tgid=7 generation=3 comm_alias_count=2 comm_aliases=[old-worker,timeline-worker]",
		"perf_timeline_thread_identity_projection=complete global_thread_identity_count=1 global_thread_identity_count_exact=true global_thread_identities_omitted=0",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("typed perf timeline missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "threads=[legacy-77]") {
		t.Fatalf("timeline must prefer typed identity over the legacy comm roster:\n%s", summary)
	}
	if got := strings.Count(summary, "perf_timeline_thread_identity=timeline-worker-77@g3"); got != 1 {
		t.Fatalf("timeline identity detail must be deduplicated across buckets, got %d:\n%s", got, summary)
	}
	if got := strings.Count(summary, "perf_timeline_caveat=timeline identity projection is advisory"); got != 1 {
		t.Fatalf("timeline renderer must defensively deduplicate exact caveat strings, got %d:\n%s", got, summary)
	}
}

func TestTraceQueryPerfIdentityMissingOrInconsistentExactCountsStayUnknown(t *testing.T) {
	first := tracequery.PerfThreadIdentity{
		TID:         41,
		Generation:  1,
		DisplayComm: "worker-new",
		CommAliases: []string{"worker-old", "worker-new"},
		// CommAliasCount is intentionally absent: an additive old producer
		// published a visible roster, not a completeness proof.
	}
	second := tracequery.PerfThreadIdentity{TID: 42, Generation: 1, DisplayComm: "peer"}
	ctx := tracequery.PerfContext{
		SampleCount: 2,
		TotalPeriod: 2,
		TopSymbols: []tracequery.PerfHotspot{{
			Symbol:           "hot",
			ThreadIdentities: []tracequery.PerfThreadIdentity{first, second},
		}},
	}

	var b strings.Builder
	writeTracePerfContext(&b, ctx)
	got := b.String()
	for _, want := range []string{
		"thread_identity_count_at_least=2 thread_identity_count_exact=false thread_identities_omitted=unknown",
		"comm_alias_count_at_least=2 comm_alias_count_exact=false comm_aliases=[worker-old,worker-new] comm_aliases_omitted=unknown",
		"perf_context_thread_identity_count_at_least=2 perf_context_thread_identity_count_exact=false perf_context_thread_identity_omitted=unknown see=payload_ref",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("old additive producer must retain unknown completeness %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "thread_identity_count=2") || strings.Contains(got, "comm_alias_count=2") {
		t.Fatalf("visible rosters must not be promoted to exact totals:\n%s", got)
	}

	if fields := traceQueryPerfIdentityCountFields(1, 2); fields != " reported_thread_identity_count=1 thread_identity_count_at_least=2 thread_identity_count_exact=false thread_identity_count_inconsistent=true thread_identities_omitted=unknown" {
		t.Fatalf("inconsistent producer count must fail closed, got %q", fields)
	}
	var inconsistent strings.Builder
	writeTracePerfIdentityDetails(&inconsistent, "", "identity", []tracequery.PerfThreadIdentity{first, second}, 1)
	for _, want := range []string{
		"identity_reported_count=1",
		"identity_count_at_least=2",
		"identity_count_exact=false",
		"identity_count_inconsistent=true",
		"identity_omitted=unknown",
	} {
		if !strings.Contains(inconsistent.String(), want) {
			t.Fatalf("identity detail did not disclose inconsistent total %q: %s", want, inconsistent.String())
		}
	}
}

func TestTraceQueryPerfTimelineTruncatedBucketsPublishOnlyGlobalLowerBound(t *testing.T) {
	identity := func(tid int) tracequery.PerfThreadIdentity {
		return tracequery.PerfThreadIdentity{TID: tid, Generation: 1, DisplayComm: fmt.Sprintf("worker-%d", tid)}
	}
	first := make([]tracequery.PerfThreadIdentity, 0, 8)
	second := make([]tracequery.PerfThreadIdentity, 0, 8)
	for tid := 100; tid < 108; tid++ {
		first = append(first, identity(tid))
	}
	for tid := 100; tid < 104; tid++ {
		second = append(second, identity(tid))
	}
	for tid := 200; tid < 204; tid++ {
		second = append(second, identity(tid))
	}
	result := tracequery.Result{
		View: "perf_timeline",
		PerfTimeline: &tracequery.PerfTimelineResult{Buckets: []tracequery.PerfTimelineBucket{
			{StartTs: 1, EndTs: 2, ThreadIdentityCount: 100, ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true), ThreadIdentities: first},
			{StartTs: 2, EndTs: 3, ThreadIdentityCount: 100, ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true), ThreadIdentities: second},
		}},
	}

	summary := traceQuerySummary(result, traceQueryParams{View: "perf_timeline"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"perf_timeline_thread_identity_visible_projection=worker-100-100@g1",
		"perf_timeline_thread_identity_visible_projection_count=12",
		"global_thread_identity_count_at_least=100",
		"global_thread_identity_count_exact=false",
		"global_thread_identities_omitted_at_least=92",
		"see=payload_ref",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("truncated timeline must publish only a global lower bound %q:\n%s", want, summary)
		}
	}
	if got := strings.Count(summary, "perf_timeline_thread_identity_visible_projection="); got != 8 {
		t.Fatalf("visible timeline detail projection must remain capped at 8, got %d:\n%s", got, summary)
	}
	for _, forbidden := range []string{
		"perf_timeline_thread_identity=worker-",
		"global_thread_identity_count=12",
		"global_thread_identity_count_exact=true",
	} {
		if strings.Contains(summary, forbidden) {
			t.Fatalf("truncated bucket union was presented as exact via %q:\n%s", forbidden, summary)
		}
	}
}

func TestTraceQueryPerfTimelineMixedLegacyBucketCannotClaimExactTypedUnion(t *testing.T) {
	identity := tracequery.PerfThreadIdentity{TID: 77, Generation: 2, DisplayComm: "typed"}
	result := tracequery.Result{
		View: "perf_timeline",
		PerfTimeline: &tracequery.PerfTimelineResult{Buckets: []tracequery.PerfTimelineBucket{
			{StartTs: 1, EndTs: 2, ThreadIdentityCount: 1, ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true), ThreadIdentities: []tracequery.PerfThreadIdentity{identity}},
			{StartTs: 2, EndTs: 3, Threads: []tracequery.ThreadRef{{Comm: "legacy", PID: 88}}},
		}},
	}

	summary := traceQuerySummary(result, traceQueryParams{View: "perf_timeline"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"perf_timeline_thread_identity_visible_projection=typed-77@g2",
		"global_thread_identity_count_at_least=1",
		"global_thread_identity_count_exact=false",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("mixed legacy/typed timeline must fail closed %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "global_thread_identity_count_exact=true") {
		t.Fatalf("legacy-only bucket made typed global union incomplete:\n%s", summary)
	}
}

func TestTraceQueryPerfTimelineAnonymousNonemptyBucketMakesTypedUnionLowerBound(t *testing.T) {
	identity := tracequery.PerfThreadIdentity{TID: 77, Generation: 2, DisplayComm: "typed"}
	result := tracequery.Result{
		View: "perf_timeline",
		PerfTimeline: &tracequery.PerfTimelineResult{Buckets: []tracequery.PerfTimelineBucket{
			{StartTs: 1, EndTs: 2, SampleCount: 1, Period: 10, ThreadIdentityCount: 1, ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true), ThreadIdentities: []tracequery.PerfThreadIdentity{identity}},
			// A source-only or locally withdrawn sample remains valid global
			// inventory but cannot contribute a typed thread identity.
			{StartTs: 2, EndTs: 3, SampleCount: 1, Period: 10, ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(false), ThreadIdentityUnknownSampleCount: 1},
			// A genuinely empty bucket is neutral and must not be the reason
			// completeness is withdrawn.
			{StartTs: 3, EndTs: 4},
		}},
	}

	summary := traceQuerySummary(result, traceQueryParams{View: "perf_timeline"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"perf_timeline_thread_identity_visible_projection=typed-77@g2",
		"global_thread_identity_count_at_least=1",
		"global_thread_identity_count_exact=false",
		"global_thread_identity_unknown_sample_count=1",
		"see=payload_ref",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("anonymous nonempty bucket must make the typed union a lower bound %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "global_thread_identity_count_exact=true") {
		t.Fatalf("anonymous sample was omitted from an allegedly exact typed union:\n%s", summary)
	}
}

func TestTraceQueryPerfTimelineMixedTypedAnonymousSameBucketIsLowerBound(t *testing.T) {
	identity := tracequery.PerfThreadIdentity{TID: 77, Generation: 2, DisplayComm: "typed"}
	result := tracequery.Result{
		View: "perf_timeline",
		PerfTimeline: &tracequery.PerfTimelineResult{Buckets: []tracequery.PerfTimelineBucket{{
			StartTs: 1, EndTs: 2, SampleCount: 2, Period: 20,
			ThreadIdentityCount: 1, ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(false),
			ThreadIdentityUnknownSampleCount: 1,
			ThreadIdentities:                 []tracequery.PerfThreadIdentity{identity},
		}}},
	}

	summary := traceQuerySummary(result, traceQueryParams{View: "perf_timeline"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"threads=[typed-77@g2] thread_identity_count_at_least=1 thread_identity_count_exact=false thread_identity_unknown_sample_count=1",
		"global_thread_identity_count_at_least=1",
		"global_thread_identity_count_exact=false",
		"global_thread_identity_unknown_sample_count=1",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("same-bucket anonymous coverage missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "global_thread_identity_count_exact=true") {
		t.Fatalf("same-bucket anonymous sample was erased from exactness:\n%s", summary)
	}
}

func TestTraceQueryPerfLegacyAndExplicitIncompleteCoverageFailClosed(t *testing.T) {
	identity := tracequery.PerfThreadIdentity{TID: 77, Generation: 1, DisplayComm: "typed"}
	for _, tc := range []struct {
		name  string
		exact *bool
	}{
		{name: "legacy_absent", exact: nil},
		{name: "explicit_incomplete", exact: traceQueryPerfIdentityExactForTest(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tracequery.PerfContext{
				SampleCount: 1, TotalPeriod: 10, ThreadIdentityCount: 1,
				ThreadIdentityCountExact: tc.exact,
				TopThreads:               []tracequery.PerfThreadSummary{{Identity: &identity}},
				TopSymbols: []tracequery.PerfHotspot{{
					Symbol: "hot", SampleCount: 1, Period: 10,
					ThreadIdentityCount: 1, ThreadIdentityCountExact: tc.exact,
					ThreadIdentities: []tracequery.PerfThreadIdentity{identity},
				}},
			}
			var b strings.Builder
			writeTracePerfContext(&b, ctx)
			got := b.String()
			if !strings.Contains(got, "thread_identity_count_at_least=1 thread_identity_count_exact=false") || strings.Contains(got, "thread_identity_count=1 thread_identities_omitted=0") {
				t.Fatalf("%s coverage was presented as exact:\n%s", tc.name, got)
			}
		})
	}
}

func TestTraceQueryPerfAllAnonymousCoverageHasNoFabricatedTypedIdentity(t *testing.T) {
	ctx := tracequery.PerfContext{
		SampleCount: 1, TotalPeriod: 10,
		ThreadIdentityCountExact:         traceQueryPerfIdentityExactForTest(false),
		ThreadIdentityUnknownSampleCount: 1,
	}
	var b strings.Builder
	writeTracePerfContext(&b, ctx)
	got := b.String()
	for _, want := range []string{
		"thread_identity_count_at_least=0",
		"thread_identity_count_exact=false",
		"thread_identity_unknown_sample_count=1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("anonymous coverage missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "@g") || strings.Contains(got, "thread_identity_count_exact=true") {
		t.Fatalf("anonymous sample fabricated a typed identity face:\n%s", got)
	}
}

func TestTraceQueryPerfContextAnonymousSamplesMakeIdentityCountsLowerBounds(t *testing.T) {
	identity := tracequery.PerfThreadIdentity{TID: 77, Generation: 1, DisplayComm: "typed"}
	ctx := tracequery.PerfContext{
		SampleCount: 2, TotalPeriod: 20,
		ThreadIdentityCount: 1, ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(false), ThreadIdentityUnknownSampleCount: 1,
		TopThreads: []tracequery.PerfThreadSummary{{Identity: &identity, Thread: tracequery.ThreadRef{PID: 77, Comm: "typed"}, SampleCount: 1, Period: 10}},
		TopSymbols: []tracequery.PerfHotspot{{
			Symbol: "Shared::hot", SampleCount: 2, Period: 20,
			ThreadIdentityCount: 1, ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(false), ThreadIdentityUnknownSampleCount: 1,
			ThreadIdentities: []tracequery.PerfThreadIdentity{identity},
		}},
	}
	result := tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{PerfSamples: &ctx}}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"perf_samples sample_count=2 total_sample_weight=20 thread_identity_count_at_least=1 thread_identity_count_exact=false thread_identity_unknown_sample_count=1 thread_identities_omitted=unknown",
		"perf_top_symbol symbol=Shared::hot",
		"thread_identity_count_at_least=1 thread_identity_count_exact=false thread_identity_unknown_sample_count=1 thread_identities_omitted=unknown",
		"perf_context_thread_identity_count_at_least=1 perf_context_thread_identity_count_exact=false perf_context_thread_identity_unknown_sample_count=1 perf_context_thread_identity_omitted=unknown",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("anonymous perf identity coverage missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "thread_identity_count=1 thread_identities_omitted=0") {
		t.Fatalf("known typed roster was presented as the exact global identity union:\n%s", summary)
	}
}

func TestTraceQueryPerfTypedIdentityPublishesExactCountAndOmittedProjection(t *testing.T) {
	all := make([]tracequery.PerfThreadIdentity, 0, 100)
	for i := 0; i < 100; i++ {
		all = append(all, tracequery.PerfThreadIdentity{
			TID:         1000 + i,
			Generation:  1,
			DisplayComm: fmt.Sprintf("worker-%d", i),
		})
	}
	visible := all[:8]
	ctx := tracequery.PerfContext{
		SampleCount:              100,
		TotalPeriod:              1000,
		ThreadIdentityCount:      100,
		ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true),
		TopSymbols: []tracequery.PerfHotspot{{
			Symbol:                   "hot",
			Period:                   1000,
			SampleCount:              100,
			ThreadIdentityCount:      100,
			ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true),
			ThreadIdentities:         visible,
		}},
	}
	result := tracequery.Result{
		View:        "window_stats",
		WindowStats: &tracequery.WindowStats{PerfSamples: &ctx},
		PerfTimeline: &tracequery.PerfTimelineResult{Buckets: []tracequery.PerfTimelineBucket{{
			StartTs:                  1,
			EndTs:                    2,
			SampleCount:              100,
			Period:                   1000,
			ThreadIdentityCount:      100,
			ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true),
			ThreadIdentities:         visible,
		}}},
	}

	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"perf_samples sample_count=100 total_sample_weight=1000 thread_identity_count=100 thread_identities_omitted=92",
		"threads=[worker-0-1000@g1,worker-1-1001@g1,worker-2-1002@g1,worker-3-1003@g1,worker-4-1004@g1,worker-5-1005@g1,worker-6-1006@g1,worker-7-1007@g1] thread_identity_count=100 thread_identities_omitted=92",
		"perf_context_thread_identity_omitted=92 see=payload_ref",
		"perf_bucket 1.000000..2.000000 sample_weight=1000 samples=100",
		"thread_identity_count=100 thread_identities_omitted=92 lines=0-0",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("exact typed identity projection missing %q:\n%s", want, summary)
		}
	}
	if got := strings.Count(summary, "perf_context_thread_identity=worker-"); got != 8 {
		t.Fatalf("context detail projection must stay capped at 8, got %d:\n%s", got, summary)
	}
	if got := traceQueryPerfIdentityLabelsOrLegacy(all, nil); strings.Count(got, ",") != 7 || strings.Contains(got, "worker-8-1008@g1") {
		t.Fatalf("label helper must enforce its own cap=8, got %q", got)
	}
	var emptyProjection strings.Builder
	writeTracePerfIdentityDetails(&emptyProjection, "", "identity", nil, 100)
	if got, want := emptyProjection.String(), "identity_omitted=100 see=payload_ref\n"; got != want {
		t.Fatalf("exact empty projection disclosure mismatch: got %q want %q", got, want)
	}
}

func TestTraceQueryPerfIdentityTokensAreDelimiterAndRuneSafe(t *testing.T) {
	identity := tracequery.PerfThreadIdentity{
		TID:            77,
		Generation:     2,
		DisplayComm:    "  worker, fake=9|peer@7 [x](y){z}:tail\u202eevil  ",
		CommAliases:    []string{"old, alias", "peer|tid=9@3", "bidi\u202ealias", strings.Repeat("线程", 80)},
		CommAliasCount: 300,
	}
	ctx := tracequery.PerfContext{
		SampleCount:              1,
		TotalPeriod:              1,
		ThreadIdentityCount:      1,
		ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true),
		TopSymbols: []tracequery.PerfHotspot{{
			Symbol:                   "hot",
			ThreadIdentityCount:      1,
			ThreadIdentityCountExact: traceQueryPerfIdentityExactForTest(true),
			ThreadIdentities:         []tracequery.PerfThreadIdentity{identity},
		}},
	}
	var b strings.Builder
	writeTracePerfContext(&b, ctx)
	got := b.String()
	if !utf8.ValidString(got) {
		t.Fatalf("identity renderer emitted invalid UTF-8: %q", got)
	}
	for _, forbidden := range []string{"worker, fake", "fake=9", "peer@7", "old, alias", "peer|tid=9@3", "\u202e"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("identity metadata leaked grammar delimiter %q:\n%s", forbidden, got)
		}
	}
	for _, want := range []string{
		"worker_fake_9_peer_7_x_y_z_tail_evil-77@g2",
		"comm_aliases=[old_alias,peer_tid_9_3,bidi_alias,",
		"comm_alias_count=300",
		"comm_aliases_omitted=296",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("safe identity token missing %q:\n%s", want, got)
		}
	}
	truncated := identity
	truncated.CommAliasCount = 0
	truncated.CommAliasCountAtLeast = 257
	truncated.CommAliasesTruncated = true
	var truncatedOut strings.Builder
	writeTracePerfThreadIdentityDetail(&truncatedOut, "", "identity", &truncated)
	for _, want := range []string{
		"comm_alias_count_at_least=257",
		"comm_aliases_truncated=true",
		"comm_aliases_omitted_at_least=253",
	} {
		if !strings.Contains(truncatedOut.String(), want) {
			t.Fatalf("truncated alias lower bound missing %q: %s", want, truncatedOut.String())
		}
	}
	if strings.Contains(truncatedOut.String(), " comm_alias_count=") || strings.Contains(truncatedOut.String(), " comm_aliases_omitted=") {
		t.Fatalf("truncated alias projection must not claim an exact count: %s", truncatedOut.String())
	}
}

func TestWriteTracePerfContextLegacyOutputFailsClosedWithoutFabricatingIdentity(t *testing.T) {
	ctx := tracequery.PerfContext{
		SampleCount: 1,
		TotalPeriod: 9,
		TopSymbols: []tracequery.PerfHotspot{{
			Symbol:      "hot",
			DSO:         "lib.so",
			Period:      9,
			SampleCount: 1,
			Threads:     []tracequery.ThreadRef{{Comm: "legacy", PID: 42}},
		}},
		TopThreads: []tracequery.PerfThreadSummary{{
			Thread:      tracequery.ThreadRef{Comm: "legacy", PID: 42},
			Period:      9,
			SampleCount: 1,
		}},
	}
	var b strings.Builder
	writeTracePerfContext(&b, ctx)
	want := "- perf_samples sample_count=1 total_sample_weight=9 thread_identity_count_at_least=0 thread_identity_count_exact=false thread_identities_omitted=unknown\n" +
		"- perf_top_symbol symbol=hot dso=lib.so event= weight_unit= source= symbolization_status= sample_weight=9 samples=1 percent=0.00 cpus=[] threads=[legacy-42] thread_identity_count_at_least=0 thread_identity_count_exact=false thread_identities_omitted=unknown lines=0-0 example=\n" +
		"- perf_top_thread thread=legacy-42 sample_weight=9 samples=1 percent=0.00 cpus=[] lines=0-0 example=\n"
	if got := b.String(); got != want {
		t.Fatalf("legacy perf coverage disclosure drifted:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
	compact := traceQueryPerfContextCompact(&ctx)
	if !strings.Contains(compact, "thread_identity_count_exact=false") || strings.Contains(compact, "thread_identity_count=0 ") {
		t.Fatalf("legacy compact summary did not fail closed: %s", compact)
	}
}

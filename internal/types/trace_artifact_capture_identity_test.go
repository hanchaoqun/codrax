package types

// CMP-6 F1 pins (adversarial review 2026-07-04): the preflight capture-
// identity census reuses THE answer-side partition identity semantics
// (canonpath canonicalisation + the F5a ≥2-segment verbatim suffix-alias
// merge) plus one census-only fold — same-directory same-stem members of a
// single capture (the .systrace/.perftrace sibling pair, tracebundle-expanded
// sub-artifacts). Case is preserved end-to-end; the old lowercase fold was the
// OPPOSITE of the partitioner's semantics and is gone.

import (
	"reflect"
	"testing"
)

func TestTraceArtifactCaptureIdentityPaths_SingleCaptureShapesFoldToOne(t *testing.T) {
	cases := []struct {
		name      string
		spellings []string
		want      []string
	}{
		{
			// F1(a): a tracebundle expanded into same-capture sub-artifacts.
			// The expansion site (outputdump bundle expansion) leaves no typed
			// parent-source field on the sub-artifacts, so the same-directory
			// same-stem family fold is the covering rule.
			"tracebundle expansion",
			[]string{"/tmp/berlin.tracebundle.json", "/tmp/berlin.systrace", "/tmp/berlin.perftrace"},
			[]string{"/tmp/berlin.tracebundle.json"},
		},
		{
			// F1(a): the .systrace/.perftrace same-stem sibling pair.
			"sibling pair",
			[]string{"cap/run.systrace", "cap/run.perftrace"},
			[]string{"cap/run.systrace"},
		},
		{
			// F1(b): relative vs absolute spellings of ONE file — the
			// partitioner's F5a suffix-alias merge; the longer spelling
			// survives, exactly like the partitioner.
			"relative vs absolute",
			[]string{"logs/berlin.systrace", "/repo/logs/berlin.systrace"},
			[]string{"/repo/logs/berlin.systrace"},
		},
		{
			// Multi-segment stem chains strip only the KNOWN trace suffix
			// chain: record.sys.ftrace + record.sys.perftrace share the stem
			// "record.sys".
			"chained suffix stem",
			[]string{"record_trace.sys.ftrace", "record_trace.sys.perftrace"},
			[]string{"record_trace.sys.ftrace"},
		},
	}
	for _, c := range cases {
		if got := TraceArtifactCaptureIdentityPaths(c.spellings); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: TraceArtifactCaptureIdentityPaths(%v) = %v, want %v", c.name, c.spellings, got, c.want)
		}
	}
}

func TestTraceArtifactCaptureIdentityPaths_TrueDistinctCapturesStayApart(t *testing.T) {
	cases := []struct {
		name      string
		spellings []string
		wantCount int
	}{
		// Two different-stem trace files = a true two-capture comparison.
		{"two different stems", []string{"a.systrace", "b.systrace"}, 2},
		// Same basename in DIFFERENT directories never folds (mirrors the
		// partitioner's refusal to alias 1-segment bare basenames).
		{"same basename different dirs", []string{"run1/berlin.systrace", "run2/berlin.systrace"}, 2},
		// Case-distinct stems are DISTINCT identities — canonpath is
		// case-preserving and the census mirrors it (F1(c): no lowercase fold).
		{"case-distinct stems", []string{"a.systrace", "A.SYSTRACE"}, 2},
	}
	for _, c := range cases {
		if got := TraceArtifactCaptureIdentityPaths(c.spellings); len(got) != c.wantCount {
			t.Errorf("%s: TraceArtifactCaptureIdentityPaths(%v) = %v, want %d identities", c.name, c.spellings, got, c.wantCount)
		}
	}
}

func TestRuntimeTracePreflightCaptureIdentityPaths_TraceKindOnly(t *testing.T) {
	profile := RuntimeArtifactPreflightProfile{
		SourceNavigationOptional: true,
		Artifacts: []RuntimeArtifactPreflightArtifact{
			{Kind: "trace", Source: "a.systrace", Carrier: "request_path"},
			// Bundle-expanded sub-artifact kinds resolve to "trace" via their
			// own path shape and join the census.
			{Kind: "perftrace", Source: "a.perftrace", Carrier: "request_path"},
			// Log artifacts never join the TRACE capture census.
			{Kind: "log", Source: "app.log", Carrier: "request_path"},
		},
	}
	got := RuntimeTracePreflightCaptureIdentityPaths(profile)
	if !reflect.DeepEqual(got, []string{"a.systrace"}) {
		t.Fatalf("preflight census must fold the sibling pair and exclude log artifacts: %v", got)
	}
}

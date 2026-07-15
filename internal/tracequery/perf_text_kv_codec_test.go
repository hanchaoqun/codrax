package tracequery

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func perfTextCodecBody() string {
	return "cpu=5 cpu_known=true pid=12 tid=34 thread_comm=" + tracewire.QuotePerfKVValue("Render") +
		" sample_weight=11 event=" + tracewire.QuotePerfKVValue("cpu-cycles") +
		" symbol=" + tracewire.QuotePerfKVValue(`Hot" tid=999 cpu=7 sample_weight=999`) +
		" dso=" + tracewire.QuotePerfKVValue(`C:\Program Files\鸿蒙\libfoo.dll`) +
		" source=fixture sample_kind=on_cpu thread_identity_known=true resolution=resolved lifecycle_unverified=false"
}

func parsePerfTextCodecRow(t *testing.T, body string) Event {
	t.Helper()
	line := "Render-34 ( 12) [005] .... 1.000000: perf_sample: " + body
	ev, ok := ParseLine(1, line, newStringInterner())
	if !ok || ev.Type != EventPerfSample || ev.PerfFields == nil {
		t.Fatalf("perf inventory row did not parse: ok=%t event=%+v", ok, ev)
	}
	return ev
}

func TestPerfTextKVQuotedMetadataCannotReopenHardKeys(t *testing.T) {
	ev := parsePerfTextCodecRow(t, perfTextCodecBody())
	pf := ev.PerfFields
	if ev.PID != 34 || ev.TGID != 12 || pf.PID != 12 || pf.TID != 34 {
		t.Fatalf("quoted metadata replaced thread identity: %+v", ev)
	}
	if ev.CPU != 5 || !perfSampleHasKnownCPU(ev) || pf.Period != 11 || perfSampleEffectiveWeight(ev) != 11 {
		t.Fatalf("quoted metadata replaced CPU/weight: %+v", ev)
	}
	if got, want := pf.Symbol, `Hot" tid=999 cpu=7 sample_weight=999`; got != want {
		t.Fatalf("symbol round-trip drift: got=%q want=%q", got, want)
	}
	if got, want := pf.DSO, `C:\Program Files\鸿蒙\libfoo.dll`; got != want {
		t.Fatalf("DSO round-trip drift: got=%q want=%q", got, want)
	}
	if pf.PerfTextIntegrity != "" || perfSampleWeightUnit(ev) != "cycles" || !perfSampleHasOnCPUExecutionCoordinate(ev) {
		t.Fatalf("legal row was degraded: %+v", pf)
	}
}

func TestPerfTextKVHardFamiliesFailClosedByDimension(t *testing.T) {
	base := perfTextCodecBody()
	tests := []struct {
		name      string
		body      string
		dimension string
		issue     string
	}{
		{name: "tid duplicate identical", body: base + " tid=34", dimension: "thread", issue: "tid_duplicate_identical"},
		{name: "pid alias conflict", body: base + " tgid=13", dimension: "thread", issue: "pid_duplicate_conflict"},
		{name: "pid missing", body: strings.Replace(base, "pid=12 ", "", 1), dimension: "thread", issue: "pid_missing"},
		{name: "tid overflow", body: strings.Replace(base, "tid=34", "tid=2147483648", 1), dimension: "thread", issue: "tid_not_canonical_pid"},
		{name: "tid quoted", body: strings.Replace(base, "tid=34", `tid="34"`, 1), dimension: "thread", issue: "tid_quoted_scalar"},
		{name: "provenance invalid", body: strings.Replace(base, "source=fixture", "source=fixture,", 1), dimension: "thread", issue: "source_not_canonical_token"},
		{name: "resolution invalid", body: strings.Replace(base, "resolution=resolved", "resolution=future", 1), dimension: "thread", issue: "resolution_not_closed_enum"},
		{name: "perf source coordinate invalid", body: base + " perf_source_tid=-1", dimension: "thread", issue: "perf_source_tid_not_canonical_pid"},
		{name: "CPU duplicate", body: base + " cpu=5", dimension: "cpu", issue: "cpu_duplicate_identical"},
		{name: "CPU negative", body: strings.Replace(base, "cpu=5", "cpu=-2", 1), dimension: "cpu", issue: "cpu_not_unsigned_decimal"},
		{name: "CPU quoted", body: strings.Replace(base, "cpu=5", `cpu="5"`, 1), dimension: "cpu", issue: "cpu_quoted_scalar"},
		{name: "weight alias conflict", body: base + " period=12", dimension: "weight", issue: "sample_weight_duplicate_conflict"},
		{name: "weight missing", body: strings.Replace(base, "sample_weight=11 ", "", 1), dimension: "weight", issue: "sample_weight_missing"},
		{name: "weight quoted", body: strings.Replace(base, "sample_weight=11", `sample_weight="11"`, 1), dimension: "weight", issue: "sample_weight_quoted_scalar"},
		{name: "sample kind open enum", body: strings.Replace(base, "sample_kind=on_cpu", "sample_kind=ON_CPU", 1), dimension: "kind", issue: "sample_kind_not_closed_enum"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := parsePerfTextCodecRow(t, tc.body)
			pf := ev.PerfFields
			if !strings.Contains(pf.PerfTextIntegrity, tc.issue) {
				t.Fatalf("missing integrity code %q: %q", tc.issue, pf.PerfTextIntegrity)
			}
			switch tc.dimension {
			case "thread":
				if perfSampleHasTypedThreadIdentity(ev) || ev.PID != 0 || ev.TGID != 0 || pf.PID != 0 || pf.TID != 0 {
					t.Fatalf("invalid thread family retained authority: %+v", ev)
				}
				if pf.Symbol == "" {
					t.Fatal("thread withdrawal erased safe symbol inventory")
				}
			case "cpu":
				if perfSampleHasKnownCPU(ev) || ev.CPU != -1 {
					t.Fatalf("invalid CPU family retained authority: %+v", ev)
				}
				if !perfSampleHasTypedThreadIdentity(ev) {
					t.Fatal("CPU-local failure erased valid thread identity")
				}
			case "weight":
				if !pf.PerfWeightInvalid || pf.Period != 0 || perfSampleEffectiveWeight(ev) != 1 || perfSampleWeightUnit(ev) != "sample_count_unweighted" {
					t.Fatalf("invalid weight did not become unweighted inventory: %+v", pf)
				}
			case "kind":
				if pf.SampleKind != "" || perfSampleHasOnCPUExecutionCoordinate(ev) {
					t.Fatalf("invalid sample kind entered on-CPU lane: %+v", pf)
				}
			}
		})
	}
}

func TestPerfTextKVMalformedWireRetainsAnonymousInventoryAndCensus(t *testing.T) {
	body := strings.Replace(perfTextCodecBody(), `symbol="Hot\" tid=999 cpu=7 sample_weight=999"`, `symbol="bad\q"`, 1)
	ev := parsePerfTextCodecRow(t, body)
	pf := ev.PerfFields
	if perfSampleHasTypedThreadIdentity(ev) || perfSampleHasKnownCPU(ev) || !pf.PerfWeightInvalid || pf.SampleKind != "" {
		t.Fatalf("malformed lexical boundary retained a hard dimension: %+v", ev)
	}
	if pf.Symbol != "" || !strings.Contains(pf.PerfTextIntegrity, "wire_invalid_escape") {
		t.Fatalf("malformed row retained partial metadata or lost witness: %+v", pf)
	}
	ctx := computePerfContext(&Index{Events: []Event{ev}}, Query{TimeStart: 0.5, TimeEnd: 1.5}, 8)
	if ctx == nil || ctx.SampleCount != 1 || ctx.TotalPeriod != 1 || ctx.Quality == nil || len(ctx.Quality.InputIntegrityIssues) == 0 {
		t.Fatalf("anonymous inventory/census missing: %+v", ctx)
	}
	if !containsSubstring(ctx.Quality.Caveats, "perf_text_integrity_degraded=true") ||
		!containsSubstring(ctx.Quality.Caveats, "unweighted sample-count inventory only") {
		t.Fatalf("model-facing repair disclosure missing: %+v", ctx.Quality.Caveats)
	}
}

func TestPerfTextKVMetadataDuplicatesAreFieldLocalAndAliasesUsePhysicalOrder(t *testing.T) {
	base := perfTextCodecBody()
	duplicate := parsePerfTextCodecRow(t, base+` symbol="other"`)
	if duplicate.PerfFields.Symbol != "" || !perfSampleHasTypedThreadIdentity(duplicate) || !perfSampleHasKnownCPU(duplicate) ||
		!strings.Contains(duplicate.PerfFields.PerfTextIntegrity, "symbol_metadata_duplicate_conflict") {
		t.Fatalf("metadata duplicate was not field-local: %+v", duplicate)
	}

	physical := strings.Replace(base, `symbol="Hot\" tid=999 cpu=7 sample_weight=999"`, `func="first" symbol="later"`, 1)
	ev := parsePerfTextCodecRow(t, physical)
	if ev.PerfFields.Symbol != "first" {
		t.Fatalf("metadata alias priority ignored physical order: %+v", ev.PerfFields)
	}
}

func TestPerfTextKVEnvelopeMismatchAndNegativeProofCannotBeRescued(t *testing.T) {
	mismatch := parsePerfTextCodecRow(t, strings.Replace(perfTextCodecBody(), "tid=34", "tid=35", 1))
	if perfSampleHasTypedThreadIdentity(mismatch) || !strings.Contains(mismatch.PerfFields.PerfTextIntegrity, "thread_identity_envelope_body_mismatch") {
		t.Fatalf("dual envelope/body identities survived: %+v", mismatch)
	}

	negativeConflict := parsePerfTextCodecRow(t, perfTextCodecBody()+" thread_identity_known=false")
	if perfSampleHasTypedThreadIdentity(negativeConflict) || !strings.Contains(negativeConflict.PerfFields.PerfTextIntegrity, "thread_identity_known_duplicate_conflict") {
		t.Fatalf("negative provenance duplicate was rescued: %+v", negativeConflict)
	}
}

func TestPerfTextKVZeroIdentityEnvelopeBoundaries(t *testing.T) {
	body := "cpu=5 cpu_known=true pid=0 tid=0 thread_comm=swapper sample_weight=1 event=cpu-cycles symbol=idle source=fixture sample_kind=on_cpu"
	positiveEnvelope := parsePerfTextCodecRow(t, body)
	if perfSampleHasTypedThreadIdentity(positiveEnvelope) ||
		!strings.Contains(positiveEnvelope.PerfFields.PerfTextIntegrity, "thread_identity_envelope_body_mismatch") {
		t.Fatalf("positive envelope rescued explicit zero body identity: %+v", positiveEnvelope)
	}

	line := "swapper-0 (  0) [005] .... 1.000000: perf_sample: " + body
	zero, ok := ParseLine(1, line, newStringInterner())
	if !ok || zero.PerfFields == nil {
		t.Fatalf("canonical swapper inventory row was lost: %+v", zero)
	}
	if zero.PID != 0 || zero.TGID != 0 || zero.PerfFields.PID != 0 || zero.PerfFields.TID != 0 ||
		zero.PerfFields.PerfTextIntegrity != "" || zero.PerfFields.Symbol != "idle" {
		t.Fatalf("legal header0/body0 row was poisoned: %+v", zero)
	}
}

func TestPerfTextKVCPUIntegrityConsumesSameTypedVerdict(t *testing.T) {
	line := "Render-34 ( 12) [005] .... 1.000000: perf_sample: " + perfTextCodecBody() + " cpu=7"
	failures := cpuInputValidationFailures(1, line)
	if len(failures) != 1 || failures[0].Field != "cpu" || failures[0].ReasonCode != "duplicate_conflict" {
		t.Fatalf("CPU audit drifted from parser verdict: %+v", failures)
	}
}

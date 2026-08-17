package tracequery

// TQ-CPU-SCALAR-STRICT A mechanical contract pins.  These fixtures stay
// deliberately small: the production Donghu corpus is the positive witness,
// while this file isolates every scalar/ownership ambiguity that used to be
// erased by parseKV last-write-wins or atoiFloatTolerant coercion.

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unsafe"
)

func parseCPUStrictLine(t *testing.T, line string) Event {
	t.Helper()
	ev, ok := ParseLine(1, line, newStringInterner())
	if !ok {
		t.Fatalf("CPU control row must remain searchable inventory: %q", line)
	}
	return ev
}

func TestCPUScalarStrictParserVersionV36(t *testing.T) {
	if ParserVersion != "tracequery-v40" {
		t.Fatalf("CPU scalar authority changed without its parser-cache generation pin: %q", ParserVersion)
	}
}

func cpuStrictFailureFields(line string) []string {
	failures := cpuInputValidationFailures(1, line)
	fields := make([]string, 0, len(failures))
	for _, failure := range failures {
		if failure.Field != "header_cpu" {
			fields = append(fields, failure.Field)
		}
	}
	sort.Strings(fields)
	return fields
}

func requireCPUScalarFailure(t *testing.T, line, field string) Event {
	t.Helper()
	ev := parseCPUStrictLine(t, line)
	if !ev.CPUInputInvalid {
		t.Fatalf("malformed hard scalar did not mark the row invalid: %+v", ev)
	}
	fields := cpuStrictFailureFields(line)
	if field == "" && len(fields) != 0 {
		return ev
	}
	for _, got := range fields {
		if got == field {
			return ev
		}
	}
	t.Fatalf("malformed hard scalar has no %q typed witness: fields=%v event=%+v", field, fields, ev)
	return Event{}
}

func TestCPUScalarStrictPositiveGrammarAndDonghuHeaderMismatch(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantType  EventType
		wantValue uint64
		wantCPU   int
	}{
		{"idle zero", `idle-0 (0) [003] .... 1.000000: cpu_idle: state=0 cpu_id=0`, EventCPUIdle, 0, 0},
		{"idle dot zero compatibility", `idle-0 (0) [003] .... 1.000000: cpu_idle: state=1.0 cpu_id=4095`, EventCPUIdle, 1, 4095},
		{"idle exit sentinel", `idle-0 (0) [003] .... 1.000000: cpu_idle: state=4294967295 cpu_id=7`, EventCPUIdle, math.MaxUint32, 7},
		{"frequency state alias and header mismatch", `tppmgr-9 (9) [003] .... 1.000000: cpu_frequency: state=2200000 cpu_id=7`, EventCPUFrequency, 2200000, 7},
		{"frequency long alias", `tppmgr-9 (9) [003] .... 1.000000: cpu_frequency: frequency=2200000.0 cpu_id=7`, EventCPUFrequency, 2200000, 7},
		{"frequency short alias", `tppmgr-9 (9) [003] .... 1.000000: cpu_frequency: freq=4294967295 cpu_id=7`, EventCPUFrequency, math.MaxUint32, 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := parseCPUStrictLine(t, tc.line)
			if ev.Type != tc.wantType || ev.CPUInputInvalid || !ev.CPUForFieldValid || eventCPUForStats(ev) != tc.wantCPU {
				t.Fatalf("positive scalar/owner rejected: %+v", ev)
			}
			var got uint64
			if ev.Type == EventCPUIdle {
				got = uint64(ev.State)
			} else {
				got = uint64(ev.Frequency)
				if !isPerCPUFrequencySample(ev) && ev.Frequency > 0 {
					t.Fatalf("valid positive frequency lost hard admission: %+v", ev)
				}
			}
			if got != tc.wantValue {
				t.Fatalf("value=%d want=%d event=%+v", got, tc.wantValue, ev)
			}
		})
	}

	zero := parseCPUStrictLine(t, `tppmgr-9 (9) [003] .... 1.000000: cpu_frequency: state=0 cpu_id=7`)
	if zero.CPUInputInvalid || !zero.CPUForFieldValid || zero.Frequency != 0 || isPerCPUFrequencySample(zero) {
		t.Fatalf("frequency zero is typed inventory but not a curve sample: %+v", zero)
	}
}

func TestCPUScalarStrictDonghuProductionCorpusPreserved(t *testing.T) {
	const (
		path       = "../../eval/fixtures/real_traces/donghu.ftrace"
		wantSHA256 = "e15d3dfc7963739c648a3f4f40095cabff19716575949bf38ea02ef732672b25"
	)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	hash := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(f, hash))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	intern := newStringInterner()
	counts := map[string]int{}
	idleExitSentinels := 0
	frequencyHeaderMismatch := 0
	limitsHeaderMismatch := 0
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		family := ""
		switch {
		case strings.Contains(line, ": cpu_idle:"):
			family = "idle"
		case strings.Contains(line, ": cpu_frequency:"):
			family = "frequency"
		case strings.Contains(line, ": cpu_frequency_limits:"):
			family = "limits"
		case strings.Contains(line, ": clock_set_rate:"):
			family = "clock"
		}
		if family == "" {
			continue
		}
		ev, ok := ParseLine(lineNo, line, intern)
		if !ok || ev.CPUInputInvalid {
			t.Fatalf("Donghu canonical %s row was rejected/degraded at line %d: ok=%t event=%+v", family, lineNo, ok, ev)
		}
		counts[family]++
		switch family {
		case "idle":
			if ev.Type != EventCPUIdle || !ev.CPUForFieldValid {
				t.Fatalf("Donghu idle authority drift at line %d: %+v", lineNo, ev)
			}
			if ev.State == math.MaxUint32 {
				idleExitSentinels++
			}
		case "frequency":
			if ev.Type != EventCPUFrequency || !ev.CPUForFieldValid || !isPerCPUFrequencySample(ev) {
				t.Fatalf("Donghu frequency authority drift at line %d: %+v", lineNo, ev)
			}
			if ev.CPU != ev.CPUForField {
				frequencyHeaderMismatch++
			}
		case "limits":
			if ev.Type != EventCPUFrequencyLimit || !ev.CPUForFieldValid {
				t.Fatalf("Donghu limits authority drift at line %d: %+v", lineNo, ev)
			}
			if ev.CPU != ev.CPUForField {
				limitsHeaderMismatch++
			}
		case "clock":
			if (ev.Type != EventClockSetRate && ev.Type != EventCPUFrequency) || ev.ClockName == "" || !eventCPUScalarKnown(ev) {
				t.Fatalf("Donghu clock authority drift at line %d: %+v", lineNo, ev)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != wantSHA256 {
		t.Fatalf("Donghu witness changed: sha256=%s want=%s", got, wantSHA256)
	}
	if got := counts["idle"] + counts["frequency"] + counts["limits"] + counts["clock"]; got != 3906 ||
		counts["idle"] != 2265 || counts["frequency"] != 830 || counts["limits"] != 44 || counts["clock"] != 767 {
		t.Fatalf("Donghu CPU corpus was not preserved: total=%d counts=%v", got, counts)
	}
	if idleExitSentinels != 1137 || frequencyHeaderMismatch != 799 || limitsHeaderMismatch != 42 {
		t.Fatalf("Donghu ownership/sentinel evidence drifted: idle_exit=%d frequency_header_mismatch=%d limits_header_mismatch=%d",
			idleExitSentinels, frequencyHeaderMismatch, limitsHeaderMismatch)
	}
}

func TestCPUScalarStrictRejectsCoercionDuplicateAliasAndQuotedPseudoKeys(t *testing.T) {
	base := `tppmgr-9 (9) [003] .... 1.000000: cpu_frequency: %s cpu_id=7`
	for _, scalar := range []string{
		``, `state=-1`, `state=+1`, `state=01`, `state=1.00`, `state=1.5`,
		`state=1e6`, `state=NaN`, `state=Inf`, `state=4294967296`, `state=1junk`,
		`state="2200000"`, `state=100 state=200`, `state=100 frequency=100`,
		`note="state=2200000"`, `note="state=1" state=2200000 freq=2200000`,
	} {
		line := fmt.Sprintf(base, scalar)
		ev := requireCPUScalarFailure(t, line, "")
		if isPerCPUFrequencySample(ev) {
			t.Fatalf("invalid frequency entered a hard curve: scalar=%q event=%+v", scalar, ev)
		}
	}

	// A key-looking token inside quoted metadata is ignored by the occurrence
	// lexer; the one real scalar remains the sole authority.
	quoted := parseCPUStrictLine(t, fmt.Sprintf(base, `note="state=1 freq=2 cpu_id=3" state=2200000`))
	if quoted.CPUInputInvalid || quoted.Frequency != 2200000 || eventCPUForStats(quoted) != 7 || !isPerCPUFrequencySample(quoted) {
		t.Fatalf("quoted pseudo-key contaminated the real scalar: %+v", quoted)
	}

	idleBad := requireCPUScalarFailure(t,
		`idle-0 (0) [003] .... 1.000000: cpu_idle: state=4294967296 cpu_id=7`, "")
	if idleBad.State != 0 {
		t.Fatalf("overflow idle sentinel was coerced into a state: %+v", idleBad)
	}
}

func TestCPUScalarIntegrityWitnessUsesPayloadCPUAndFindsUppercaseGeneralizedName(t *testing.T) {
	knownOwner := `idle-0 (0) [003] .... 1.000000: vendor_CPU_FREQ: state=broken cpu_id=7`
	if !cpuInputRawCandidate(knownOwner) {
		t.Fatal("case-insensitive generalized CPU family escaped the raw integrity prescreen")
	}
	failures := cpuInputValidationFailures(1, knownOwner)
	if len(failures) != 1 || failures[0].Field != "state|frequency|freq" || failures[0].CPU != 7 {
		t.Fatalf("scalar failure did not retain its controlled payload CPU: %+v", failures)
	}

	unknownOwner := `idle-0 (0) [003] .... 1.100000: vendor_CPU_FREQ: state=broken`
	failures = cpuInputValidationFailures(2, unknownOwner)
	if len(failures) != 2 {
		t.Fatalf("malformed scalar with missing owner must publish both typed issues: %+v", failures)
	}
	for _, failure := range failures {
		if failure.CPU != -1 {
			t.Fatalf("unknown payload owner inherited emitter CPU3: %+v", failures)
		}
	}

	idx := buildTraceIndex(t, "cpu-scalar-diagnostic-wording.systrace", knownOwner+"\n")
	caveats := cpuInputIntegrityCaveats(idx, Query{})
	if !containsSubstring(caveats, "CPU/scalar control inputs") || containsSubstring(caveats, "CPU identities were ignored") {
		t.Fatalf("scalar/control degradation retained the identity-only wording: %v", caveats)
	}
}

func TestCPUScalarStrictRequiresPayloadCPUWithoutHeaderFallback(t *testing.T) {
	for _, tc := range []struct {
		name  string
		line  string
		field string
	}{
		{"missing", `idle-0 (0) [003] .... 1.000000: cpu_frequency: state=1200000`, "cpu_id"},
		{"duplicate", `idle-0 (0) [003] .... 1.000000: cpu_frequency: state=1200000 cpu_id=2 cpu_id=3`, "cpu_id"},
		{"quoted pseudo only", `idle-0 (0) [003] .... 1.000000: cpu_frequency: state=1200000 note="cpu_id=3"`, "cpu_id"},
		{"signed", `idle-0 (0) [003] .... 1.000000: cpu_frequency: state=1200000 cpu_id=+3`, "cpu_id"},
		{"leading zero", `idle-0 (0) [003] .... 1.000000: cpu_frequency: state=1200000 cpu_id=03`, "cpu_id"},
		{"above maximum", `idle-0 (0) [003] .... 1.000000: cpu_frequency: state=1200000 cpu_id=4096`, "cpu_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := requireCPUScalarFailure(t, tc.line, tc.field)
			if eventCPUForStats(ev) != -1 || isPerCPUFrequencySample(ev) {
				t.Fatalf("bad/missing payload cpu inherited header cpu3: %+v", ev)
			}
		})
	}
}

func TestCPUFrequencyLimitsStrictPairAtomicProfiles(t *testing.T) {
	for _, tc := range []struct {
		line    string
		wantMin uint64
		wantMax uint64
	}{
		{`idle-0 (0) [003] .... 1.000000: cpu_frequency_limits: min=0 max=4294967295 cpu_id=7`, 0, math.MaxUint32},
		{`idle-0 (0) [003] .... 1.000000: cpu_frequency_limits: min_freq=500000.0 max_freq=2200000.0 cpu_id=7`, 500000, 2200000},
	} {
		ev := parseCPUStrictLine(t, tc.line)
		if ev.CPUInputInvalid || uint64(ev.FrequencyMin) != tc.wantMin || uint64(ev.FrequencyMax) != tc.wantMax || eventCPUForStats(ev) != 7 {
			t.Fatalf("valid atomic limits pair rejected: %+v", ev)
		}
	}

	for _, fields := range []string{
		`min=1`, `max=2`, `min=1 max_freq=2`, `min_freq=1 max=2`,
		`min=1 min_freq=1 max=2`, `min=1 max=2 max=2`, `min=3 max=2`,
		`min=-1 max=2`, `min=1.00 max=2`, `min=1 max=4294967296`,
		`note="min=1 max=2"`, `min="1" max=2`,
	} {
		line := `idle-0 (0) [003] .... 1.000000: cpu_frequency_limits: ` + fields + ` cpu_id=7`
		ev := requireCPUScalarFailure(t, line, "")
		if _, ok := isPerCPULimitSample(ev); ok {
			t.Fatalf("non-atomic limits pair entered hard ceiling: fields=%q event=%+v", fields, ev)
		}
	}
}

func TestClockSetRateStrictProfilesAndCPUOwnership(t *testing.T) {
	for _, tc := range []struct {
		line      string
		wantRate  int64
		wantCPU   int
		cpuKnown  bool
		wantClock string
	}{
		{`clk-9 (9) [003] .... 1.000000: clock_set_rate: ddr_clk state=1866000000 cpu_id=7`, 1866000000, 7, true, "ddr_clk"},
		{`clk-9 (9) [003] .... 1.000000: clock_set_rate: ddr_clk frequency=1866000000.0 cpu_id=7`, 1866000000, 7, true, "ddr_clk"},
		{`clk-9 (9) [003] .... 1.000000: clock_set_rate: ddr_clk frequency=0`, 0, -1, false, "ddr_clk"},
		{`clk-9 (9) [003] .... 1.000000: clock_set_rate: ddr_clk 1866000000`, 1866000000, -1, false, "ddr_clk"},
		{`clk-9 (9) [003] .... 1.000000: clock_set_rate: ddr_clk 1866000000.0`, 1866000000, -1, false, "ddr_clk"},
	} {
		ev := parseCPUStrictLine(t, tc.line)
		if ev.CPUInputInvalid || ev.Frequency != tc.wantRate || ev.ClockName != tc.wantClock || ev.CPUForFieldValid != tc.cpuKnown || eventCPUForStats(ev) != tc.wantCPU {
			t.Fatalf("valid clock profile rejected or inherited header CPU: %+v", ev)
		}
	}

	for _, fields := range []string{
		`ddr_clk`, `ddr_clk -1`, `ddr_clk 01`, `ddr_clk 1.00`, `ddr_clk 1.5`, `ddr_clk 9223372036854775808`,
		`ddr_clk state=1 2`, `ddr_clk 1 state=1`, `ddr_clk state=1 freq=1`,
		`ddr_clk state=1 state=2`, `ddr_clk state="1"`, `ddr_clk state=1 trailing`,
		`"ddr rail" state=1`, `ddr_clk state=1 cpu_id=7 cpu_id=8`, `ddr_clk state=1 cpu_id=4096`,
	} {
		line := `clk-9 (9) [003] .... 1.000000: clock_set_rate: ` + fields
		ev := requireCPUScalarFailure(t, line, "")
		if isClusterRailEvent(ev) {
			t.Fatalf("malformed clock row entered rail evidence: fields=%q event=%+v", fields, ev)
		}
	}
}

func TestClockSetRateMalformedInventoryDoesNotMintSupplySignal(t *testing.T) {
	idx := buildTraceIndex(t, "cpu-scalar-clock-supply.systrace", strings.Join([]string{
		`clk-9 (9) [003] .... 1.000000: clock_set_rate: ddr_clk state=broken cpu_id=3`,
		`clk-9 (9) [003] .... 1.100000: clock_set_rate: ddr_clk state=0 cpu_id=3`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.2})
	if stats.SupplyPressureSummary == nil {
		t.Fatal("valid zero clock transition is typed supply activity and must remain visible")
	}
	if got := stats.SupplyPressureSummary.ClockSetRateCount; got != 1 {
		t.Fatalf("malformed clock inventory entered supply semantics or valid zero was lost: count=%d summary=%+v", got, stats.SupplyPressureSummary)
	}
	if stats.CPUFrequencySampleRowCount != 0 || stats.ClockSetRateEventCount != 1 {
		t.Fatalf("frequency/clock authority census merged lanes or admitted malformed rows: cpu_frequency_rows=%d clock_set_rate_events=%d",
			stats.CPUFrequencySampleRowCount, stats.ClockSetRateEventCount)
	}
	if stats.SupplyPressureSummary.DDREventCount != 1 || !containsSubstring(stats.Caveats, "cpu_input_integrity_degraded=true") {
		t.Fatalf("clock supply/caveat split drifted: summary=%+v caveats=%v", stats.SupplyPressureSummary, stats.Caveats)
	}
}

func TestCPUScalarMalformedTransitionPoisonsCarryInAtCorrectScope(t *testing.T) {
	t.Run("known payload CPU poisons only that CPU", func(t *testing.T) {
		idx := buildTraceIndex(t, "cpu-scalar-poison.systrace", strings.Join([]string{
			`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1000000 cpu_id=2`,
			`idle-0 (0) [000] .... 1.100000: cpu_frequency: state=broken cpu_id=2`,
			`idle-0 (0) [000] .... 1.200000: cpu_frequency: state=1800000 cpu_id=3`,
		}, "\n")+"\n")
		timelines, ok := idx.fullFrequencyTimelines()
		if !ok {
			t.Fatal("complete physical scan must publish the non-poisoned curve set")
		}
		if len(timelines[2]) != 0 || len(timelines[3]) != 1 || timelines[3][0].khz != 1800000 {
			t.Fatalf("known-owner poison crossed CPUs or bridged the old value: %+v", timelines)
		}
	})

	t.Run("unknown payload CPU poisons the source family", func(t *testing.T) {
		idx := buildTraceIndex(t, "cpu-scalar-family-poison.systrace", strings.Join([]string{
			`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1000000 cpu_id=2`,
			`idle-0 (0) [000] .... 1.100000: cpu_frequency: state=broken`,
			`idle-0 (0) [000] .... 1.200000: cpu_frequency: state=1800000 cpu_id=3`,
		}, "\n")+"\n")
		timelines, ok := idx.fullFrequencyTimelines()
		if !ok {
			t.Fatal("complete physical scan should publish an explicitly empty poisoned family")
		}
		if len(timelines) != 0 {
			t.Fatalf("unknown-owner malformed transition left family carry-in alive: %+v", timelines)
		}
	})
}

func TestCPUScalarIndexedAndStreamingEventSearchParity(t *testing.T) {
	path := buildTraceIndex(t, "cpu-scalar-parity.systrace", strings.Join([]string{
		`idle-0 (0) [003] .... 1.000000: cpu_frequency: state=1200000 cpu_id=7`,
		`idle-0 (0) [003] .... 1.100000: cpu_frequency: state=1200000 state=1800000 cpu_id=7`,
		`idle-0 (0) [003] .... 1.200000: cpu_frequency: frequency=1800000.0 cpu_id=7`,
	}, "\n")+"\n")
	q := Query{View: "event_search", EventTypes: []EventType{EventCPUFrequency}, Limit: 20}
	indexed := Run(path, q)
	streamed, err := StreamEventSearch(context.Background(), path.Path, q)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := func(events []EventView) []string {
		out := make([]string, 0, len(events))
		for _, view := range events {
			out = append(out, fmt.Sprintf("%d/%s/%d/%d/%t/%t", view.Line, view.Type, view.Frequency, eventCPUForStats(view.Event), view.CPUInputInvalid, isPerCPUFrequencySample(view.Event)))
		}
		return out
	}
	if got, want := fingerprint(streamed.Events), fingerprint(indexed.Events); !reflect.DeepEqual(got, want) {
		t.Fatalf("stream/index receipt drift:\nstream=%v\n index=%v", got, want)
	}
}

func TestCPUScalarReceiptKeepsEventCoreAt688Bytes(t *testing.T) {
	// 688→680 (NEXTINFO P1, 2026-07-25): four bounded next_info fields went
	// int→int32 (-16B) paying for the *NextInfoRichFields side-table pointer
	// (+8B); shrinking is welcome per the P4 ratchet.
	if got := unsafe.Sizeof(Event{}); got != 672 {
		t.Fatalf("CPU scalar receipt grew or reshaped the hot Event core: got=%d want=672; use an existing padding slot or rare side table", got)
	}
}

func TestCPUScalarFullIndexMalformedTransitionBlocksWindowCarryIn(t *testing.T) {
	idx := buildTraceIndex(t, "cpu-scalar-full-poison.systrace", strings.Join([]string{
		`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.100000: cpu_frequency: state=broken cpu_id=0`,
		`idle-0 (0) [000] .... 1.200000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120`,
		`app-42 (42) [000] .... 1.800000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.2, TimeEnd: 1.8})
	if !containsSubstring(stats.Caveats, "family=cpu_frequency affected_cpus=[0]") {
		t.Fatalf("full monotonic index lost the malformed transition barrier: %v", stats.Caveats)
	}
	for _, cpu := range stats.CPU {
		if cpu.CPU == 0 && (cpu.Frequency != 0 || len(cpu.FrequencyResidency) != 0) {
			t.Fatalf("old frequency bridged a malformed pre-window transition: %+v", cpu)
		}
	}
}

func TestCPUScalarColdWindowMalformedTransitionBlocksCarryIn(t *testing.T) {
	for _, tc := range []struct {
		name       string
		family     durationOrderFamily
		valid      string
		malformed  string
		wantCaveat string
	}{
		{
			name:       "frequency known CPU",
			family:     durationOrderCPUFrequency,
			valid:      `idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
			malformed:  `idle-0 (0) [000] .... 1.100000: cpu_frequency: state=broken cpu_id=0`,
			wantCaveat: "family=cpu_frequency affected_cpus=[0]",
		},
		{
			name:       "limits known CPU",
			family:     durationOrderCPUFreqLimit,
			valid:      `idle-0 (0) [000] .... 1.000000: cpu_frequency_limits: min=400000 max=1800000 cpu_id=0`,
			malformed:  `idle-0 (0) [000] .... 1.100000: cpu_frequency_limits: min=broken max=1800000 cpu_id=0`,
			wantCaveat: "family=cpu_frequency_limits affected_cpus=[0]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cold-window.systrace")
			body := strings.Join([]string{
				tc.valid,
				tc.malformed,
				`idle-0 (0) [000] .... 2.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120`,
				`app-42 (42) [000] .... 2.500000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
			}, "\n") + "\n"
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
				AllowWindowedParse: true,
				TimeStart:          2,
				TimeStartSet:       true,
				TimeEnd:            2.5,
				TimeEndSet:         true,
				TimePaddingBefore:  2,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !idx.Windowed {
				t.Fatal("fixture must exercise the cold windowed parser")
			}
			integrity := frequencyOrderIntegrityForQuery(idx, Query{TimeStart: 2, TimeEnd: 2.5})
			if tc.family == durationOrderCPUFrequency && !integrity.frequencyUnsafe(0) ||
				tc.family == durationOrderCPUFreqLimit && !integrity.limitUnsafe(0) {
				t.Fatalf("cold-window malformed transition did not poison %s: failures=%+v", tc.family, idx.durationOrderFailures)
			}
			stats := ComputeWindowStats(idx, Query{TimeStart: 2, TimeEnd: 2.5})
			if !containsSubstring(stats.Caveats, tc.wantCaveat) {
				t.Fatalf("cold-window carry barrier missing from public caveat: %v", stats.Caveats)
			}
			if tc.family == durationOrderCPUFrequency {
				for _, cpu := range stats.CPU {
					if cpu.CPU == 0 && (cpu.Frequency != 0 || len(cpu.FrequencyResidency) != 0) {
						t.Fatalf("old frequency bridged a cold-window malformed transition: %+v", cpu)
					}
				}
			} else {
				for _, limit := range stats.CPUFrequencyLimits {
					if limit.CPU == 0 {
						t.Fatalf("old limits survived a cold-window malformed transition: %+v", stats.CPUFrequencyLimits)
					}
				}
			}
		})
	}
}

func TestCPUScalarZeroBarrierIsNotCAPSample(t *testing.T) {
	idx := buildTraceIndex(t, "cpu-scalar-zero-cap-floor.systrace", strings.Join([]string{
		`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
		`idle-0 (0) [001] .... 1.000001: cpu_frequency: state=1200000 cpu_id=1`,
		`idle-0 (0) [000] .... 1.100000: cpu_frequency: state=0 cpu_id=0`,
		`idle-0 (0) [001] .... 1.100001: cpu_frequency: state=0 cpu_id=1`,
	}, "\n")+"\n")
	transitions := indexFreqTransitionTimelines(idx)
	hardSamples := indexFreqSampleTimelines(idx)
	if len(transitions[0]) != 2 || len(transitions[1]) != 2 || len(hardSamples[0]) != 1 || len(hardSamples[1]) != 1 {
		t.Fatalf("zero barrier leaked into or erased the wrong projection: transitions=%+v hard=%+v", transitions, hardSamples)
	}
	cache := newChainQueryCache(idx, nil)
	if got := cache.frequencyAt(0, 1.2); got != 0 {
		t.Fatalf("state timeline lost the zero carry barrier: %d", got)
	}
	capability := cache.coreCapability("")
	if capability.usable() || !capability.comoveFloorTripped {
		t.Fatalf("zero barrier counted as a second CAP/co-movement sample: %+v", capability)
	}
}

func TestCPUScalarZeroTransitionsCutCarryWithoutMintingZeroKHz(t *testing.T) {
	idx := buildTraceIndex(t, "cpu-scalar-zero-barrier.systrace", strings.Join([]string{
		`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.100000: cpu_frequency: state=0 cpu_id=0`,
		`idle-0 (0) [000] .... 1.200000: cpu_frequency_limits: min=400000 max=1800000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.300000: cpu_frequency_limits: min=0 max=0 cpu_id=0`,
		`idle-0 (0) [000] .... 1.400000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120`,
		`app-42 (42) [000] .... 1.800000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.4, TimeEnd: 1.8})
	for _, cpu := range stats.CPU {
		if cpu.CPU == 0 && (cpu.Frequency != 0 || len(cpu.FrequencyResidency) != 0) {
			t.Fatalf("zero transition was treated as a 0kHz sample or old carry survived: %+v", cpu)
		}
	}
	cache := newChainQueryCache(idx, nil)
	if got := cache.frequencyAt(0, 1.5); got != 0 {
		t.Fatalf("positive frequency crossed canonical zero transition: %d", got)
	}
	if got := cache.governedLimitMaxKHz(0, 1.4, 1.8); got != 0 {
		t.Fatalf("positive limit crossed canonical max=0 transition: %d", got)
	}
	events := []Event{
		{Ts: 1.0, CPU: 0, Type: EventCPUFrequency, Name: "cpu_frequency", Frequency: 1200000, CPUForFieldPresent: true, CPUForFieldValid: true},
		{Ts: 1.5, CPU: 0, Type: EventCPUFrequency, Name: "cpu_frequency", Frequency: 0, CPUForFieldPresent: true, CPUForFieldValid: true},
	}
	if got := segmentFrequencyStats(events, 1.2, 1.8); got.known || got.weightedKHz != 0 {
		t.Fatalf("a partial zero-governed segment minted a low-frequency value: %+v", got)
	}
}

func TestCPUScalarRejectedOuterHeaderStillPoisonsPayloadCPU(t *testing.T) {
	idx := buildTraceIndex(t, "cpu-scalar-header-poison.systrace", strings.Join([]string{
		`idle-0 (0) [000] .... 1.000000: cpu_frequency: state=1200000 cpu_id=0`,
		`idle-0 (0) [5000] .... 1.100000: cpu_frequency: state=1800000 cpu_id=0`,
		`idle-0 (0) [000] .... 1.200000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120`,
		`app-42 (42) [000] .... 1.800000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	}, "\n")+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.2, TimeEnd: 1.8})
	if !containsSubstring(stats.Caveats, "family=cpu_frequency affected_cpus=[0]") {
		t.Fatalf("parser-rejected outer CPU erased the payload-local barrier: %v failures=%+v", stats.Caveats, idx.durationOrderFailures)
	}
	if timelines, ok := idx.fullFrequencyTimelines(); !ok || len(timelines[0]) != 0 {
		t.Fatalf("full-frequency side lane bridged the rejected row: ok=%t timelines=%+v", ok, timelines)
	}
}

func TestCPUScalarGeneralizedNameAndMixedBoundsKeepBarrier(t *testing.T) {
	idx := buildTraceIndex(t, "cpu-scalar-generalized-poison.systrace", strings.Join([]string{
		`idle-0 (0) [000] .... 1.000000: vendor_CPU_FREQ: state=1200000 cpu_id=0`,
		`idle-0 (0) [000] .... 9.000000: vendor_CPU_FREQ: state=broken cpu_id=0`,
	}, "\n")+"\n")
	integrity := frequencyOrderIntegrityForQuery(idx, Query{LineStart: 1, LineEnd: 2, TimeEnd: 2})
	if !integrity.frequencyUnsafe(0) {
		t.Fatalf("generalized-name or mixed line/time bounds scoped out the barrier: %+v", idx.durationOrderFailures)
	}
}

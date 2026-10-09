package tracequery

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func pfrTestRecord(id, ts, dur, rate int64, pid int) tracewire.ProcessMeasureInterval {
	r := processMeasureTestRecord(id, ts, dur, rate, pid)
	r.Name = "H:PreferredFrameRate"
	return r
}
func pfrTestRun(t *testing.T, rows []tracewire.ProcessMeasureInterval, q Query) *PreferredFrameRateResult {
	t.Helper()
	idx := renderingFixture(t, processMeasureTestText(t, rows...))
	q.View = ViewPreferredFrameRate
	got := Run(idx, q)
	if got.PreferredFrameRate == nil || !ValidPreferredFrameRate(*got.PreferredFrameRate) {
		t.Fatalf("invalid pfr: %+v", got.PreferredFrameRate)
	}
	if got.RootCauseRank != nil || got.WindowStats != nil || got.Timeline != nil || got.ProcessMeasurements != nil {
		t.Fatal("unrelated authority")
	}
	return got.PreferredFrameRate
}

func TestPreferredFrameRateUnionConflictUnknownAndExactWindow(t *testing.T) {
	rows := []tracewire.ProcessMeasureInterval{pfrTestRecord(1, -10, 30, 120, 101), pfrTestRecord(2, 10, 30, 120, 101), pfrTestRecord(3, 25, 5, 60, 101), pfrTestRecord(4, 40, 10, 0, 101), pfrTestRecord(5, 60, 0, 90, 101), pfrTestRecord(6, 70, 10, 144, 101), pfrTestRecord(7, 0, 70, 90, 202), pfrTestRecord(8, 0, 70, 30, 101), pfrTestRecord(9, 0, 1, 10, 101)}
	rows[3].Value = tracewire.ProcessMeasureScalar{Status: "null", StorageClass: "null"}
	rows[7].FilterID = processMeasureTestInt(2)
	rows[8].StartNS = tracewire.ProcessMeasureScalar{Status: "null", StorageClass: "null"}
	p := pfrTestRun(t, rows, Query{TimeStartSet: true, TimeEnd: 70e-9})
	if p.Status != "available" || p.TotalSeries != 3 || p.TotalObservations != 7 || p.UnpositionedRows != 1 {
		t.Fatalf("wrong census: %+v", p)
	}
	var s PreferredFrameRateSeries
	for _, candidate := range p.Series {
		if candidate.PID != nil && *candidate.PID == 101 && candidate.FilterID == "1" {
			s = candidate
		}
	}
	if s.KnownDurationNS != 35 || s.ConflictDurationNS != 5 || s.UnknownValueDurationNS != 10 || s.UnobservedDurationNS != 20 || s.Observations != 5 || s.UnpositionedRows != 1 {
		t.Fatalf("coverage: %+v", s)
	}
	if len(s.Distribution) != 1 || s.Distribution[0].RateHz != "120" || s.Distribution[0].DurationNS != 35 || s.Distribution[0].Intervals != 2 || s.Distribution[0].WindowPercent != 50 {
		t.Fatalf("union distribution: %+v", s.Distribution)
	}
	if p.Observations[0].Record.StartNS.Value != "-10" || *p.Observations[0].ClippedStartNS != 0 {
		t.Fatal("carry-in lost")
	}
	p = pfrTestRun(t, rows, Query{PID: 999, TimeStartSet: true, TimeEnd: 70e-9})
	if p.Status != "available" || p.TotalSeries != 0 || p.TotalObservations != 0 {
		t.Fatal("empty owner fallback", p)
	}
}

func TestPreferredFrameRateFiniteRealProtocolOnly(t *testing.T) {
	for _, tc := range []struct{ storage, status, value, want string }{{"integer", "known", "120", "120"}, {"real", "invalid_storage", "120.0", "120"}, {"real", "invalid_storage", "119.88", "119.88"}, {"real", "invalid_storage", "1.2e2", "120"}, {"integer", "known", "9007199254740993", "9007199254740993"}, {"real", "invalid_storage", "NaN", ""}, {"real", "invalid_storage", "+Inf", ""}, {"real", "invalid_storage", "-Inf", ""}, {"real", "invalid_storage", "0", ""}, {"integer", "known", "-1", ""}, {"text", "invalid_storage", "120", ""}, {"null", "null", "", ""}} {
		t.Run(tc.storage+tc.value, func(t *testing.T) {
			r := pfrTestRecord(1, 0, 100, 1, 101)
			r.Value = tracewire.ProcessMeasureScalar{StorageClass: tc.storage, Status: tc.status, Value: tc.value}
			p := pfrTestRun(t, []tracewire.ProcessMeasureInterval{r}, Query{TimeStartSet: true, TimeEnd: 1e-7})
			s := p.Series[0]
			if !reflect.DeepEqual(p.Observations[0].Record.Value, r.Value) {
				t.Fatal("raw value changed")
			}
			if tc.want == "" {
				if s.UnknownValueDurationNS != 100 || len(s.Distribution) != 0 {
					t.Fatal("unknown rate became known", s)
				}
			} else if len(s.Distribution) != 1 || s.Distribution[0].RateHz != tc.want {
				t.Fatal("real/int protocol lost", s)
			}
		})
	}
	for _, name := range []string{"PreferredFrameRate", "H:PreferredFrameRateExtra", "prefixH:PreferredFrameRate", "RSS"} {
		r := pfrTestRecord(1, 0, 100, 120, 101)
		r.Name = name
		p := pfrTestRun(t, []tracewire.ProcessMeasureInterval{r}, Query{TimeStartSet: true, TimeEnd: 1e-7})
		if p.TotalSeries != 0 {
			t.Fatal("unregistered name got Hz", name)
		}
	}
}

func TestPreferredFrameRateScopeUnknownOwnersAndDisplayBound(t *testing.T) {
	unknown := pfrTestRecord(1, 0, 10, 120, 101)
	unknown.OwnerStatus = "unknown"
	unknown.PID = nil
	unknown.ProcessName = ""
	other := unknown
	other.RowID = 2
	p := pfrTestRun(t, []tracewire.ProcessMeasureInterval{unknown, other}, Query{TimeStartSet: true, TimeEnd: 2e-8})
	if len(p.Series) != 2 {
		t.Fatal("unknown owners merged")
	}
	for _, q := range []Query{{TimeEnd: 2e-8}, {TimeStartSet: true}, {TimeStartSet: true, TimeEnd: 2e-8, Pattern: "H:PreferredFrameRate"}} {
		p = pfrTestRun(t, []tracewire.ProcessMeasureInterval{unknown}, q)
		if p.Status != "unavailable" {
			t.Fatal("unproven scope accepted", q, p)
		}
	}
	var rows []tracewire.ProcessMeasureInterval
	for i := 0; i < 200; i++ {
		rows = append(rows, pfrTestRecord(int64(i+1), int64(i*2), 1, int64(30+i), 101))
	}
	p = pfrTestRun(t, rows, Query{TimeStartSet: true, TimeEnd: 4e-7})
	s := p.Series[0]
	if p.TotalObservations != 200 || len(p.Observations) != 64 || p.OmittedObservations != 136 || s.KnownDurationNS != 200 || s.UnobservedDurationNS != 200 || s.TotalDistribution != 200 || s.OmittedDistribution != 72 || s.TotalIntervals != 400 || s.OmittedIntervals != 272 {
		t.Fatalf("bounded output changed population: %+v", s)
	}
	for _, mutate := range []func(*PreferredFrameRateResult){func(p *PreferredFrameRateResult) { p.Series[0].KnownDurationNS++ }, func(p *PreferredFrameRateResult) { p.Series[0].Timeline[0].StartNS++ }, func(p *PreferredFrameRateResult) { p.Series[0].Distribution[0].WindowPercent++ }, func(p *PreferredFrameRateResult) { p.Window.EndInclusive = true }, func(p *PreferredFrameRateResult) { p.TargetPID = 999 }} {
		raw, _ := json.Marshal(p)
		var bad PreferredFrameRateResult
		json.Unmarshal(raw, &bad)
		mutate(&bad)
		if ValidPreferredFrameRate(bad) {
			t.Fatal("invalid publication accepted")
		}
	}
}

func TestPreferredFrameRateStreamCompleteAndCanceled(t *testing.T) {
	rows := []tracewire.ProcessMeasureInterval{pfrTestRecord(1, -10, 20, 120, 101), pfrTestRecord(2, 5, 5, 60, 101)}
	path := filepath.Join(t.TempDir(), "capture.systrace")
	os.WriteFile(path, []byte(processMeasureTestText(t, rows...)), 0600)
	q := Query{View: ViewPreferredFrameRate, TimeStartSet: true, TimeEnd: 2e-8}
	got, err := StreamPreferredFrameRate(context.Background(), path, q)
	if err != nil || got.PreferredFrameRate == nil || !ValidPreferredFrameRate(*got.PreferredFrameRate) || got.PreferredFrameRate.Series[0].ConflictDurationNS != 5 {
		t.Fatalf("stream: %v %+v", err, got.PreferredFrameRate)
	}
	got, err = streamProcessMeasurements(context.Background(), path, q, 1, math.MaxInt)
	if err != nil || got.PreferredFrameRate.Status != "unavailable" || got.PreferredFrameRate.TotalObservations != 0 || !strings.Contains(strings.Join(got.PreferredFrameRate.Caveats, " "), "retention_limit") {
		t.Fatal("partial scan population", err, got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = StreamPreferredFrameRate(ctx, path, q)
	if !errors.Is(err, context.Canceled) || got.PreferredFrameRate != nil {
		t.Fatal("canceled publication", err, got)
	}
}

func TestPreferredFrameRateAdjacentRateUnionKeepsCompleteProvenance(t *testing.T) {
	rows := []tracewire.ProcessMeasureInterval{pfrTestRecord(1, 0, 20, 120, 101), pfrTestRecord(2, 0, 10, 120, 101), pfrTestRecord(3, 10, 10, 120, 101)}
	p := pfrTestRun(t, rows, Query{TimeStartSet: true, TimeEnd: 2e-8})
	s := p.Series[0]
	if len(s.Timeline) != 2 || s.Timeline[0].EndNS != 10 || s.Timeline[1].StartNS != 10 || s.Timeline[0].TotalSourceLines != 2 || s.Timeline[1].TotalSourceLines != 2 {
		t.Fatalf("different complete active sets merged through abbreviated samples: %+v", s.Timeline)
	}
	if len(s.Distribution) != 1 || s.Distribution[0].DurationNS != 20 || s.Distribution[0].Intervals != 1 {
		t.Fatal("provenance boundary changed equal-rate union", s.Distribution)
	}
}

func TestPreferredFrameRateSourceCompletenessAndBundleIdentity(t *testing.T) {
	body := processMeasureTestText(t, pfrTestRecord(1, -10, 30, 120, 101))
	for _, tail := range []string{body, tracewire.ProcessMeasureIntervalPrefix + " record=bad\n"} {
		idx := renderingFixture(t, body+tail)
		result, err := StreamPreferredFrameRate(context.Background(), idx.Path, Query{TimeStartSet: true, TimeEnd: 1e-8})
		if err != nil || result.PreferredFrameRate.Status != "unavailable" || result.PreferredFrameRate.TotalSeries != 0 {
			t.Fatal("bad/duplicate EOF tail created partial authority", err, result)
		}
	}
	idx := renderingFixture(t, body)
	idx.Windowed = true
	got := Run(idx, Query{View: ViewPreferredFrameRate, TimeStartSet: true, TimeEnd: 1e-8})
	if got.PreferredFrameRate == nil || got.PreferredFrameRate.Status != "unavailable" {
		t.Fatal("cropped index proved complete coverage")
	}
	for _, kind := range []string{"identity", "multi", "affine", "stale"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			child, bundle := filepath.Join(dir, "events.systrace"), filepath.Join(dir, "capture.tracebundle.json")
			if err := os.WriteFile(child, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			manifest := `{"systrace":"events.systrace"}`
			if kind == "multi" {
				os.WriteFile(filepath.Join(dir, "other.systrace"), []byte(body), 0600)
				manifest = `{"systrace":"events.systrace","artifacts":[{"type":"systrace","path":"other.systrace"}]}`
			}
			if kind == "affine" {
				manifest = `{"systrace":"events.systrace","perf_clock_alignments":[{"artifact_path":"events.systrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","offset_sec":1,"slope":1,"calibrated":true}]}`
			}
			writeTraceBundleV2ForTest(t, bundle, []byte(manifest))
			if kind == "stale" {
				os.WriteFile(child, []byte(body+"# changed\n"), 0600)
			}
			result, err := StreamPreferredFrameRate(context.Background(), bundle, Query{TimeStartSet: true, TimeEnd: 1e-8})
			if kind != "identity" {
				if err == nil || result.PreferredFrameRate != nil {
					t.Fatal("foreign or stale bundle published", err, result)
				}
				return
			}
			if err != nil || result.PreferredFrameRate == nil || !ValidPreferredFrameRate(*result.PreferredFrameRate) || len(result.PreferredFrameRate.Series) != 1 || result.PreferredFrameRate.Series[0].SourcePath != canonicalTraceIndexPath(child) {
				t.Fatal("identity bundle lost source", err, result)
			}
		})
	}
}

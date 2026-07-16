package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRawPerfCaptureCompletenessDistinguishesNotReportedAndExactZero(t *testing.T) {
	dir := t.TempDir()
	read := func(name string, records ...[]byte) rawPerfData {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, syntheticRawPerfDataWithQualityRecords(true, records...), 0o644); err != nil {
			t.Fatal(err)
		}
		data, err := readRawPerfData(context.Background(), path, nil)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	notReported := read("not-reported.data").CaptureCompleteness
	for name, total := range map[string]RawPerfAggregateTotal{
		"lost_events":  notReported.LostEvents,
		"lost_samples": notReported.LostSamples,
		"aux_bytes":    notReported.AuxBytes,
	} {
		if total != (RawPerfAggregateTotal{State: rawPerfAggregateNotReported}) {
			t.Fatalf("%s not-reported state drifted: %+v", name, total)
		}
	}
	if notReported.SampleRecords != (RawPerfRecordCensus{Physical: 1, Accepted: 1}) {
		t.Fatalf("sample census did not close: %+v", notReported.SampleRecords)
	}

	exactZero := read("exact-zero.data",
		rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 0)),
		rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(0)),
		rawPerfRecord(perfRecordAux, rawPerfAuxPayload(0)),
	).CaptureCompleteness
	for name, total := range map[string]RawPerfAggregateTotal{
		"lost_events":  exactZero.LostEvents,
		"lost_samples": exactZero.LostSamples,
		"aux_bytes":    exactZero.AuxBytes,
	} {
		if total != (RawPerfAggregateTotal{State: rawPerfAggregateExact}) {
			t.Fatalf("%s exact zero was confused with not-reported: %+v", name, total)
		}
	}
	if issue, err := rawPerfCaptureHasPublicationIssue(exactZero); err != nil || issue {
		t.Fatalf("exact zero loss and accepted AUX must not open inventory publication: %+v", exactZero)
	}
	wire, err := json.Marshal(exactZero)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"profile":"raw_perf_record_census_v1"`,
		`"source":"linux_perf_data_record_stream"`,
		`"lost_events":{"state":"exact","value":0}`,
		`"lost_samples":{"state":"exact","value":0}`,
		`"aux_bytes":{"state":"exact","value":0}`,
	} {
		if !strings.Contains(string(wire), want) {
			t.Fatalf("typed exact-zero JSON missing %q: %s", want, wire)
		}
	}
}

func TestRawPerfCaptureCompletenessAccountsMalformedRecordFamilies(t *testing.T) {
	sampleType := uint64(perfSampleIP | perfSampleTID | perfSampleTime | perfSampleCPU | perfSamplePeriod)
	samplePayload := rawPerfSamplePayload(sampleType)
	if len(samplePayload) != 40 {
		t.Fatalf("test fixture sample payload=%d, want 40", len(samplePayload))
	}
	validRecords := [][]byte{
		rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7)),
		rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(5)),
		rawPerfRecord(perfRecordAux, rawPerfAuxPayload(4096)),
	}
	tests := []struct {
		name      string
		malformed []byte
		want      RawPerfCaptureCompleteness
	}{
		{
			name:      "sample",
			malformed: rawPerfRecord(perfRecordSample, samplePayload[:len(samplePayload)-1]),
		},
		{
			name:      "lost",
			malformed: rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7)[:15]),
		},
		{
			name:      "lost_samples",
			malformed: rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(5)[:7]),
		},
		{
			name:      "aux",
			malformed: rawPerfRecord(perfRecordAux, rawPerfAuxPayload(4096)[:23]),
		},
	}
	for i := range tests {
		want := newRawPerfCaptureCompleteness()
		want.SampleRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
		want.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
		want.LostSampleRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
		want.AuxRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
		want.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 7}
		want.LostSamples = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 5}
		want.AuxBytes = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 4096}
		switch tests[i].name {
		case "sample":
			want.SampleRecords = RawPerfRecordCensus{Physical: 2, Accepted: 1, Rejected: 1}
		case "lost":
			want.LostRecords = RawPerfRecordCensus{Physical: 2, Accepted: 1, Rejected: 1}
			want.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownMalformedAggregate}
		case "lost_samples":
			want.LostSampleRecords = RawPerfRecordCensus{Physical: 2, Accepted: 1, Rejected: 1}
			want.LostSamples = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownMalformedAggregate}
		case "aux":
			want.AuxRecords = RawPerfRecordCensus{Physical: 2, Accepted: 1, Rejected: 1}
			want.AuxBytes = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownMalformedAggregate}
		}
		tests[i].want = want
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.name+".data")
			records := append([][]byte(nil), validRecords...)
			records = append(records, test.malformed)
			if err := os.WriteFile(path, syntheticRawPerfDataWithQualityRecords(true, records...), 0o644); err != nil {
				t.Fatal(err)
			}
			data, err := readRawPerfData(context.Background(), path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if data.CaptureCompleteness != test.want {
				t.Fatalf("typed malformed census mismatch:\n got=%+v\nwant=%+v", data.CaptureCompleteness, test.want)
			}
			if len(data.Samples) != 1 || data.LostRecords != 1 || data.LostEvents.Value != 7 ||
				data.LostSampleRecords != 1 || data.LostSamples.Value != 5 ||
				data.AuxRecords != 1 || data.AuxBytes.Value != 4096 {
				t.Fatalf("malformed %s erased an accepted sibling: %+v", test.name, data)
			}
			if reason := validateRawPerfCaptureCompleteness(data.CaptureCompleteness); reason != "" {
				t.Fatalf("parser produced invalid completeness: %s", reason)
			}
		})
	}
}

func TestRawPerfCaptureCompletenessPureOverflowIsDimensionLocal(t *testing.T) {
	tests := []struct {
		name    string
		records [][]byte
		get     func(RawPerfCaptureCompleteness) (RawPerfRecordCensus, RawPerfAggregateTotal)
	}{
		{
			name: "lost_events",
			records: [][]byte{
				rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, math.MaxUint64)),
				rawPerfRecord(perfRecordLost, rawPerfLostPayload(2, 1)),
				rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(5)),
				rawPerfRecord(perfRecordAux, rawPerfAuxPayload(4096)),
			},
			get: func(c RawPerfCaptureCompleteness) (RawPerfRecordCensus, RawPerfAggregateTotal) {
				return c.LostRecords, c.LostEvents
			},
		},
		{
			name: "lost_samples",
			records: [][]byte{
				rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7)),
				rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(math.MaxUint64)),
				rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(1)),
				rawPerfRecord(perfRecordAux, rawPerfAuxPayload(4096)),
			},
			get: func(c RawPerfCaptureCompleteness) (RawPerfRecordCensus, RawPerfAggregateTotal) {
				return c.LostSampleRecords, c.LostSamples
			},
		},
		{
			name: "aux_bytes",
			records: [][]byte{
				rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7)),
				rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(5)),
				rawPerfRecord(perfRecordAux, rawPerfAuxPayload(math.MaxUint64)),
				rawPerfRecord(perfRecordAux, rawPerfAuxPayload(1)),
			},
			get: func(c RawPerfCaptureCompleteness) (RawPerfRecordCensus, RawPerfAggregateTotal) {
				return c.AuxRecords, c.AuxBytes
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "overflow.data")
			if err := os.WriteFile(path, syntheticRawPerfDataWithQualityRecords(true, test.records...), 0o644); err != nil {
				t.Fatal(err)
			}
			data, err := readRawPerfData(context.Background(), path, nil)
			if err != nil {
				t.Fatal(err)
			}
			records, total := test.get(data.CaptureCompleteness)
			if records != (RawPerfRecordCensus{Physical: 2, Accepted: 2}) ||
				total != (RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownAggregateOverflow}) {
				t.Fatalf("pure overflow typed verdict mismatch: records=%+v total=%+v", records, total)
			}
			for name, healthy := range map[string]RawPerfAggregateTotal{
				"lost_events":  data.CaptureCompleteness.LostEvents,
				"lost_samples": data.CaptureCompleteness.LostSamples,
				"aux_bytes":    data.CaptureCompleteness.AuxBytes,
			} {
				if name == test.name {
					continue
				}
				want := map[string]uint64{"lost_events": 7, "lost_samples": 5, "aux_bytes": 4096}[name]
				if healthy != (RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: want}) {
					t.Fatalf("overflow in %s contaminated %s: %+v", test.name, name, healthy)
				}
			}
		})
	}
}

func TestRawPerfCaptureCompletenessMalformedAndOverflowIsOrderIndependent(t *testing.T) {
	orders := [][][]byte{
		{
			rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, math.MaxUint64)),
			rawPerfRecord(perfRecordLost, rawPerfLostPayload(2, 1)),
			rawPerfRecord(perfRecordLost, rawPerfLostPayload(3, 1)[:15]),
		},
		{
			rawPerfRecord(perfRecordLost, rawPerfLostPayload(3, 1)[:15]),
			rawPerfRecord(perfRecordLost, rawPerfLostPayload(2, 1)),
			rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, math.MaxUint64)),
		},
	}
	var first RawPerfCaptureCompleteness
	for i, records := range orders {
		path := filepath.Join(t.TempDir(), "order.data")
		if err := os.WriteFile(path, syntheticRawPerfDataWithQualityRecords(true, records...), 0o644); err != nil {
			t.Fatal(err)
		}
		data, err := readRawPerfData(context.Background(), path, nil)
		if err != nil {
			t.Fatal(err)
		}
		got := data.CaptureCompleteness
		if got.LostRecords != (RawPerfRecordCensus{Physical: 3, Accepted: 2, Rejected: 1}) ||
			got.LostEvents != (RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownMalformedAndOverflow}) {
			t.Fatalf("combined malformed/overflow verdict drifted: %+v", got)
		}
		if i == 0 {
			first = got
		} else if got != first {
			t.Fatalf("physical record order changed typed verdict:\nfirst=%+v\nsecond=%+v", first, got)
		}
	}
}

func TestRawPerfCaptureCompletenessIssueGateIsEvidenceBound(t *testing.T) {
	tests := []struct {
		name    string
		records [][]byte
		want    bool
	}{
		{name: "no quality records"},
		{name: "exact zero lost", records: [][]byte{rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 0))}},
		{name: "accepted aux", records: [][]byte{rawPerfRecord(perfRecordAux, rawPerfAuxPayload(4096))}},
		{name: "aux aggregate overflow is not loss", records: [][]byte{
			rawPerfRecord(perfRecordAux, rawPerfAuxPayload(math.MaxUint64)),
			rawPerfRecord(perfRecordAux, rawPerfAuxPayload(1)),
		}},
		{name: "positive lost", records: [][]byte{rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 1))}, want: true},
		{name: "lost aggregate overflow", records: [][]byte{
			rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, math.MaxUint64)),
			rawPerfRecord(perfRecordLost, rawPerfLostPayload(2, 1)),
		}, want: true},
		{name: "malformed sample", records: [][]byte{rawPerfRecord(perfRecordSample, nil)}, want: true},
		{name: "malformed aux", records: [][]byte{rawPerfRecord(perfRecordAux, make([]byte, 23))}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "issue.data")
			if err := os.WriteFile(path, syntheticRawPerfDataWithQualityRecords(false, test.records...), 0o644); err != nil {
				t.Fatal(err)
			}
			data, err := readRawPerfData(context.Background(), path, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := rawPerfCaptureHasPublicationIssue(data.CaptureCompleteness)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("issue gate=%v want=%v: %+v", got, test.want, data.CaptureCompleteness)
			}
		})
	}
}

func TestRawPerfCaptureCompletenessValidatorFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RawPerfCaptureCompleteness)
	}{
		{name: "profile", mutate: func(c *RawPerfCaptureCompleteness) { c.Profile = "raw_perf" }},
		{name: "source", mutate: func(c *RawPerfCaptureCompleteness) { c.Source = "perf_record_stream" }},
		{name: "record sum", mutate: func(c *RawPerfCaptureCompleteness) { c.SampleRecords.Accepted = 1 }},
		{name: "not reported value", mutate: func(c *RawPerfCaptureCompleteness) { c.LostEvents.Value = 1 }},
		{name: "not reported reason", mutate: func(c *RawPerfCaptureCompleteness) { c.LostEvents.Reason = rawPerfUnknownAggregateOverflow }},
		{name: "physical not reported", mutate: func(c *RawPerfCaptureCompleteness) {
			c.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
		}},
		{name: "unknown numeric prefix", mutate: func(c *RawPerfCaptureCompleteness) {
			c.LostRecords = RawPerfRecordCensus{Physical: 2, Accepted: 2}
			c.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Value: 1, Reason: rawPerfUnknownAggregateOverflow}
		}},
		{name: "overflow one record", mutate: func(c *RawPerfCaptureCompleteness) {
			c.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
			c.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownAggregateOverflow}
		}},
		{name: "malformed without rejection", mutate: func(c *RawPerfCaptureCompleteness) {
			c.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
			c.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownMalformedAggregate}
		}},
		{name: "exact with reason", mutate: func(c *RawPerfCaptureCompleteness) {
			c.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
			c.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 1, Reason: rawPerfUnknownMalformedAggregate}
		}},
		{name: "combined without rejection", mutate: func(c *RawPerfCaptureCompleteness) {
			c.LostRecords = RawPerfRecordCensus{Physical: 2, Accepted: 2}
			c.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownMalformedAndOverflow}
		}},
		{name: "combined without two accepted", mutate: func(c *RawPerfCaptureCompleteness) {
			c.LostRecords = RawPerfRecordCensus{Physical: 2, Accepted: 1, Rejected: 1}
			c.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownMalformedAndOverflow}
		}},
		{name: "unknown reason", mutate: func(c *RawPerfCaptureCompleteness) {
			c.LostRecords = RawPerfRecordCensus{Physical: 2, Accepted: 2}
			c.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: "trust_me"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := newRawPerfCaptureCompleteness()
			test.mutate(&capture)
			if reason := validateRawPerfCaptureCompleteness(capture); reason == "" {
				t.Fatalf("forged completeness was accepted: %+v", capture)
			}
		})
	}
	invalid := newRawPerfCaptureCompleteness()
	invalid.Profile = "raw_perf"
	if issue, err := rawPerfCaptureHasPublicationIssue(invalid); err == nil || issue {
		t.Fatalf("invalid completeness reached issue decision: issue=%v err=%v", issue, err)
	}
}

func TestRawPerfCaptureCompletenessMalformedCensusDoesNotChangePerftraceWire(t *testing.T) {
	sampleType := uint64(perfSampleIP | perfSampleTID | perfSampleTime | perfSampleCPU | perfSamplePeriod)
	samplePayload := rawPerfSamplePayload(sampleType)
	malformed := map[string][]byte{
		"sample":       rawPerfRecord(perfRecordSample, samplePayload[:len(samplePayload)-1]),
		"lost":         rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7)[:15]),
		"lost_samples": rawPerfRecord(perfRecordLostSamples, rawPerfLostSamplesPayload(5)[:7]),
		"aux":          rawPerfRecord(perfRecordAux, rawPerfAuxPayload(4096)[:23]),
	}
	dir := t.TempDir()
	convert := func(name string, records ...[]byte) []byte {
		t.Helper()
		input := filepath.Join(dir, name+".data")
		output := filepath.Join(dir, name+".perftrace")
		if err := os.WriteFile(input, syntheticRawPerfDataWithQualityRecords(true, records...), 0o644); err != nil {
			t.Fatal(err)
		}
		data, err := readRawPerfData(context.Background(), input, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, caveat := range data.Caveats {
			if strings.Contains(caveat, "malformed_aggregate") || strings.Contains(caveat, "records_rejected") {
				t.Fatalf("R0 leaked typed census into legacy caveats: %q", caveat)
			}
		}
		if err := ConvertRawPerfDataFileToPerfTrace(context.Background(), input, output); err != nil {
			t.Fatal(err)
		}
		wire, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		return wire
	}
	baseline := convert("baseline")
	for name, record := range malformed {
		t.Run(name, func(t *testing.T) {
			got := convert(name, record)
			if !bytes.Equal(got, baseline) {
				t.Fatalf("R0 malformed census changed legacy wire:\nbaseline=%q\n%s=%q", baseline, name, got)
			}
		})
	}
}

func TestFinishRawPerfCaptureCompletenessRejectsInternalLedgerDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*rawPerfData)
		want   string
	}{
		{name: "sample", want: "sample accepted", mutate: func(data *rawPerfData) {
			data.CaptureCompleteness.SampleRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
		}},
		{name: "lost", want: "lost accepted", mutate: func(data *rawPerfData) {
			data.CaptureCompleteness.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
		}},
		{name: "lost_samples", want: "lost_samples accepted", mutate: func(data *rawPerfData) {
			data.CaptureCompleteness.LostSampleRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
		}},
		{name: "aux", want: "aux accepted", mutate: func(data *rawPerfData) {
			data.CaptureCompleteness.AuxRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := rawPerfData{CaptureCompleteness: newRawPerfCaptureCompleteness()}
			test.mutate(&data)
			if err := finishRawPerfCaptureCompleteness(&data); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("internal %s ledger drift was not rejected: %v", test.name, err)
			}
		})
	}
}

func TestRawPerfCaptureCompletenessR0KeepsLostOnlyDirectAPIFailure(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "lost-only.data")
	output := filepath.Join(dir, "lost-only.perftrace")
	if err := os.WriteFile(input, syntheticRawPerfDataWithQualityRecords(false,
		rawPerfRecord(perfRecordLost, rawPerfLostPayload(1, 7))), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ConvertRawPerfDataFileToPerfTrace(context.Background(), input, output); err == nil ||
		!strings.Contains(err.Error(), "contains no supported sample records") {
		t.Fatalf("lost-only direct API must remain an error in R0, got %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("lost-only direct API leaked an output: %v", err)
	}
}

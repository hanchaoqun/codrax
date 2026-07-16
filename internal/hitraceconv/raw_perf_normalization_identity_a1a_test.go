package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// These tests freeze PERF-NORMALIZATION-IDENTITY-a1a at the provider boundary.
// A structurally readable PERF_RECORD_SAMPLE is not automatically a queryable
// sample: TIME and TID presence authorize coordinates, PERIOD presence
// authorizes event weight, and every public scalar must fit the shared wire
// domain. Rejected samples remain capture inventory and never acquire made-up
// ts=0/tid=1/sample_weight=1 coordinates.

const a1aRawPerfCompleteSampleType = uint64(perfSampleIP | perfSampleTID | perfSampleTime | perfSampleCPU | perfSamplePeriod)

type a1aRawPerfSampleValues struct {
	IP     uint64
	PID    uint32
	TID    uint32
	TimeNS uint64
	CPU    uint32
	Period uint64
}

type a1aRawPerfPublication struct {
	artifact  Artifact
	decision  PerfProviderDecision
	capture   RawPerfCaptureCompleteness
	admission RawPerfSampleAdmission
	wire      string
	perfEvent []tracequery.Event
}

func TestPerfNormalizationIdentityA1ARawPresenceSeparatesPresentZeroFromAbsent(t *testing.T) {
	t.Run("present zero remains a proved zero coordinate", func(t *testing.T) {
		input := a1aRawPerfData(a1aRawPerfCompleteSampleType, a1aRawPerfSamplePayload(
			a1aRawPerfCompleteSampleType,
			a1aRawPerfSampleValues{IP: 0x1234, PID: 0, TID: 0, TimeNS: 0, CPU: 0, Period: 0},
		))
		publication, err := a1aPublishRawPerf(t, input)
		if err != nil {
			t.Fatalf("present-zero sample must publish: %v", err)
		}
		wantAdmission := newRawPerfSampleAdmission()
		wantAdmission.Candidates, wantAdmission.QueryRows = 1, 1
		a1aAssertRawPerfProjection(t, publication, 1, wantAdmission, true)
		if len(publication.perfEvent) != 1 {
			t.Fatalf("present-zero sample count=%d want=1\n%s", len(publication.perfEvent), publication.wire)
		}
		event := publication.perfEvent[0]
		if event.PerfFields == nil {
			t.Fatalf("present-zero sample lost typed perf fields: %+v", event)
		}
		if event.Ts != 0 || event.CPU != 0 || event.PerfFields.PID != 0 || event.PerfFields.TID != 0 {
			t.Fatalf("present zero was rewritten into a fabricated coordinate: %+v", event)
		}
		if event.PerfFields.CPUKnown == nil || !*event.PerfFields.CPUKnown {
			t.Fatalf("present cpu=0 lost its explicit presence: %+v", event.PerfFields)
		}
		if event.PerfFields.PerfWeightInvalid {
			t.Fatalf("present period=0 was confused with an absent PERIOD field: %+v", event.PerfFields)
		}
		if event.PerfFields.Period != 1 {
			t.Fatalf("present period=0 lost the raw producer's explicit or-1 weight semantics: %+v", event.PerfFields)
		}
	})

	for _, test := range []struct {
		name       string
		sampleType uint64
		reason     func(*RawPerfSampleAdmission)
	}{
		{name: "absent TIME", sampleType: a1aRawPerfCompleteSampleType &^ perfSampleTime, reason: func(admission *RawPerfSampleAdmission) { admission.MissingTime = 1 }},
		{name: "absent TID", sampleType: a1aRawPerfCompleteSampleType &^ perfSampleTID, reason: func(admission *RawPerfSampleAdmission) { admission.MissingTID = 1 }},
	} {
		t.Run(test.name+" is inventory without coordinates", func(t *testing.T) {
			values := a1aRawPerfSampleValues{IP: 0x1234, PID: 1234, TID: 5678, TimeNS: 1_234_567_000, CPU: 5, Period: 99}
			publication, err := a1aPublishRawPerf(t, a1aRawPerfData(test.sampleType, a1aRawPerfSamplePayload(test.sampleType, values)))
			if err != nil {
				t.Fatalf("all-rejected identity input must publish capture inventory: %v", err)
			}
			wantAdmission := newRawPerfSampleAdmission()
			wantAdmission.Candidates, wantAdmission.InventoryOnly = 1, 1
			test.reason(&wantAdmission)
			a1aAssertRawPerfProjection(t, publication, 1, wantAdmission, false)
			a1aAssertNoQueryCoordinates(t, publication)
		})
	}

	t.Run("absent PERIOD is inventory rather than fabricated weight one", func(t *testing.T) {
		sampleType := a1aRawPerfCompleteSampleType &^ perfSamplePeriod
		values := a1aRawPerfSampleValues{IP: 0x1234, PID: 1234, TID: 5678, TimeNS: 1_234_567_000, CPU: 5, Period: 0}
		publication, err := a1aPublishRawPerf(t, a1aRawPerfData(sampleType, a1aRawPerfSamplePayload(sampleType, values)))
		if err != nil {
			t.Fatalf("missing-period record must publish capture inventory: %v", err)
		}
		wantAdmission := newRawPerfSampleAdmission()
		wantAdmission.Candidates, wantAdmission.InventoryOnly, wantAdmission.MissingPeriod = 1, 1, 1
		a1aAssertRawPerfProjection(t, publication, 1, wantAdmission, false)
		a1aAssertNoQueryCoordinates(t, publication)
	})
}

func TestPerfNormalizationIdentityA1AOutOfDomainSamplesPublishAllRejectedInventory(t *testing.T) {
	valid := a1aRawPerfSampleValues{IP: 0x1234, PID: 1234, TID: 5678, TimeNS: 1_234_567_000, CPU: 5, Period: 99}
	for _, test := range []struct {
		name   string
		mutate func(*a1aRawPerfSampleValues)
		reason func(*RawPerfSampleAdmission)
	}{
		{name: "PID MaxInt32 plus one", mutate: func(sample *a1aRawPerfSampleValues) { sample.PID = 1 << 31 }, reason: func(admission *RawPerfSampleAdmission) { admission.InvalidIdentity = 1 }},
		{name: "TID MaxInt32 plus one", mutate: func(sample *a1aRawPerfSampleValues) { sample.TID = 1 << 31 }, reason: func(admission *RawPerfSampleAdmission) { admission.InvalidIdentity = 1 }},
		{name: "CPU 4096", mutate: func(sample *a1aRawPerfSampleValues) { sample.CPU = 4096 }, reason: func(admission *RawPerfSampleAdmission) { admission.InvalidCPU = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			sample := valid
			test.mutate(&sample)
			publication, err := a1aPublishRawPerf(t, a1aRawPerfData(
				a1aRawPerfCompleteSampleType,
				a1aRawPerfSamplePayload(a1aRawPerfCompleteSampleType, sample),
			))
			if err != nil {
				t.Fatalf("out-of-domain sample must degrade to published inventory, not abort the provider: %v", err)
			}
			wantAdmission := newRawPerfSampleAdmission()
			wantAdmission.Candidates, wantAdmission.InventoryOnly = 1, 1
			test.reason(&wantAdmission)
			a1aAssertRawPerfProjection(t, publication, 1, wantAdmission, false)
			a1aAssertNoQueryCoordinates(t, publication)
		})
	}
}

func TestPerfNormalizationIdentityA1ABadSampleDoesNotPoisonGoodSibling(t *testing.T) {
	good := a1aRawPerfSampleValues{IP: 0x1234, PID: 1234, TID: 5678, TimeNS: 1_234_567_000, CPU: 5, Period: 99}
	bad := a1aRawPerfSampleValues{IP: 0x5678, PID: 1234, TID: 1 << 31, TimeNS: 1_234_568_000, CPU: 4096, Period: 100}

	for _, test := range []struct {
		name    string
		samples []a1aRawPerfSampleValues
	}{
		{name: "good then bad", samples: []a1aRawPerfSampleValues{good, bad}},
		{name: "bad then good", samples: []a1aRawPerfSampleValues{bad, good}},
	} {
		t.Run(test.name, func(t *testing.T) {
			payloads := make([][]byte, 0, len(test.samples))
			for _, sample := range test.samples {
				payloads = append(payloads, a1aRawPerfSamplePayload(a1aRawPerfCompleteSampleType, sample))
			}
			publication, err := a1aPublishRawPerf(t, a1aRawPerfData(a1aRawPerfCompleteSampleType, payloads...))
			if err != nil {
				t.Fatalf("bad sample must not roll back a healthy sibling: %v", err)
			}
			wantAdmission := newRawPerfSampleAdmission()
			wantAdmission.Candidates, wantAdmission.QueryRows, wantAdmission.InventoryOnly, wantAdmission.InvalidIdentity = 2, 1, 1, 1
			a1aAssertRawPerfProjection(t, publication, 2, wantAdmission, true)
			if len(publication.perfEvent) != 1 {
				t.Fatalf("perf events=%d want healthy sibling only\n%s", len(publication.perfEvent), publication.wire)
			}
			event := publication.perfEvent[0]
			if event.PerfFields == nil || event.PerfFields.PID != int(good.PID) || event.PerfFields.TID != int(good.TID) ||
				event.CPU != int(good.CPU) || event.PerfFields.Period != int64(good.Period) || event.Ts != float64(good.TimeNS)/1e9 {
				t.Fatalf("healthy sibling identity drifted: %+v", event)
			}
			if strings.Contains(publication.wire, "tid=2147483648") || strings.Contains(publication.wire, "cpu=4096") {
				t.Fatalf("rejected sibling leaked typed coordinates:\n%s", publication.wire)
			}
		})
	}
}

func a1aAssertRawPerfProjection(t *testing.T, publication a1aRawPerfPublication, structuralSamples uint64, wantAdmission RawPerfSampleAdmission, ready bool) {
	t.Helper()
	want := RawPerfRecordCensus{Physical: structuralSamples, Accepted: structuralSamples}
	if publication.capture.SampleRecords != want {
		t.Fatalf("sample census=%+v want=%+v", publication.capture.SampleRecords, want)
	}
	if publication.admission != wantAdmission {
		t.Fatalf("sample admission=%+v want=%+v", publication.admission, wantAdmission)
	}
	if publication.artifact.Perf == nil || publication.artifact.Perf.TraceQueryReady != ready || publication.decision.TraceQueryReady != ready {
		t.Fatalf("readiness drifted: artifact=%+v decision=%+v", publication.artifact.Perf, publication.decision)
	}
	if !ready && !strings.Contains(strings.Join(publication.artifact.Caveats, "\n"), "capture-quality inventory only") {
		t.Fatalf("inventory publication lost its customer boundary: %+v", publication.artifact.Caveats)
	}
}

func a1aAssertNoQueryCoordinates(t *testing.T, publication a1aRawPerfPublication) {
	t.Helper()
	if len(publication.perfEvent) != 0 || strings.Contains(publication.wire, "perf_sample:") {
		t.Fatalf("rejected sample acquired queryable coordinates: events=%+v\n%s", publication.perfEvent, publication.wire)
	}
	for _, forbidden := range []string{"tid=1 thread_comm=", " 0.000000: perf_sample:", "sample_weight=1 event="} {
		if strings.Contains(publication.wire, forbidden) {
			t.Fatalf("rejected sample leaked fabricated token %q:\n%s", forbidden, publication.wire)
		}
	}
}

func a1aPublishRawPerf(t *testing.T, inputBytes []byte) (a1aRawPerfPublication, error) {
	t.Helper()
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "capture.perf.data")
	outputPath := filepath.Join(dir, "capture.perftrace")
	if err := os.WriteFile(inputPath, inputBytes, 0o600); err != nil {
		return a1aRawPerfPublication{}, err
	}
	authority, err := openConversionInputAuthority(inputPath)
	if err != nil {
		return a1aRawPerfPublication{}, err
	}
	ledger, err := newConversionFileLedgerForAuthority(authority)
	if err != nil {
		_ = authority.Close()
		return a1aRawPerfPublication{}, err
	}
	t.Cleanup(func() {
		if cleanupErr := ledger.cleanup(); cleanupErr != nil {
			t.Errorf("cleanup raw perf a1a fixture: %v", cleanupErr)
		}
		if closeErr := authority.Close(); closeErr != nil {
			t.Errorf("close raw perf a1a authority: %v", closeErr)
		}
	})
	binding, err := newDirectPerfInputBinding(authority, perfInputLinuxPerfData)
	if err != nil {
		return a1aRawPerfPublication{}, err
	}
	artifact, caveat, decisions, err := maybeConvertRawPerfDataFromInputWithDecision(
		context.Background(), Options{PerfParser: "raw"}, binding, outputPath, "",
		perfProviderStageDirectInput, false, ledger,
	)
	if err != nil {
		return a1aRawPerfPublication{}, err
	}
	if artifact.Path == "" || len(decisions) != 1 || !decisions[0].Succeeded {
		return a1aRawPerfPublication{}, fmt.Errorf("provider did not publish: artifact=%+v decisions=%+v caveat=%q", artifact, decisions, caveat)
	}
	if artifact.Perf == nil || artifact.Perf.RawCaptureCompleteness == nil || artifact.Perf.RawSampleAdmission == nil {
		return a1aRawPerfPublication{}, fmt.Errorf("raw perf artifact lost typed capture/admission census: %+v", artifact)
	}
	wire, err := os.ReadFile(outputPath)
	if err != nil {
		return a1aRawPerfPublication{}, err
	}
	index, err := tracequery.BuildIndex(context.Background(), outputPath)
	if err != nil {
		return a1aRawPerfPublication{}, err
	}
	publication := a1aRawPerfPublication{
		artifact:  artifact,
		decision:  decisions[0],
		capture:   *artifact.Perf.RawCaptureCompleteness,
		admission: *artifact.Perf.RawSampleAdmission,
		wire:      string(wire),
	}
	for _, event := range index.Events {
		if event.Type == tracequery.EventPerfSample {
			publication.perfEvent = append(publication.perfEvent, event)
		}
	}
	return publication, nil
}

func a1aRawPerfData(sampleType uint64, samplePayloads ...[]byte) []byte {
	const headerSize = 104
	const attrSize = 48
	var records bytes.Buffer
	for _, payload := range samplePayloads {
		records.Write(a1aRawPerfRecord(perfRecordSample, payload))
	}
	dataOffset := headerSize + attrSize
	out := make([]byte, dataOffset)
	copy(out[0:8], []byte(perfMagic2))
	binary.LittleEndian.PutUint64(out[8:16], headerSize)
	binary.LittleEndian.PutUint64(out[16:24], attrSize)
	binary.LittleEndian.PutUint64(out[24:32], headerSize)
	binary.LittleEndian.PutUint64(out[32:40], attrSize)
	binary.LittleEndian.PutUint64(out[40:48], uint64(dataOffset))
	binary.LittleEndian.PutUint64(out[48:56], uint64(records.Len()))
	attr := out[headerSize:dataOffset]
	binary.LittleEndian.PutUint32(attr[0:4], 0)
	binary.LittleEndian.PutUint32(attr[4:8], 40)
	binary.LittleEndian.PutUint64(attr[24:32], sampleType)
	return append(out, records.Bytes()...)
}

func a1aRawPerfSamplePayload(sampleType uint64, sample a1aRawPerfSampleValues) []byte {
	var out bytes.Buffer
	writeU64 := func(value uint64) {
		var raw [8]byte
		binary.LittleEndian.PutUint64(raw[:], value)
		out.Write(raw[:])
	}
	writeU32Pair := func(first, second uint32) {
		var raw [8]byte
		binary.LittleEndian.PutUint32(raw[0:4], first)
		binary.LittleEndian.PutUint32(raw[4:8], second)
		out.Write(raw[:])
	}
	if sampleType&perfSampleIP != 0 {
		writeU64(sample.IP)
	}
	if sampleType&perfSampleTID != 0 {
		writeU32Pair(sample.PID, sample.TID)
	}
	if sampleType&perfSampleTime != 0 {
		writeU64(sample.TimeNS)
	}
	if sampleType&perfSampleCPU != 0 {
		writeU32Pair(sample.CPU, 0)
	}
	if sampleType&perfSamplePeriod != 0 {
		writeU64(sample.Period)
	}
	return out.Bytes()
}

func a1aRawPerfRecord(recordType int, payload []byte) []byte {
	size := 8 + len(payload)
	out := make([]byte, size)
	binary.LittleEndian.PutUint32(out[0:4], uint32(recordType))
	binary.LittleEndian.PutUint16(out[6:8], uint16(size))
	copy(out[8:], payload)
	return out
}

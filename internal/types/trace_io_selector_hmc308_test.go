package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func hmc308IOAliasRecord(id, path string, nameSelector bool) ObservationRecord {
	r := hmc233IORecord(id, path, 10, 20)
	r.SourceRef.QueryTargetPID = 100
	if nameSelector {
		r.SourceRef.QueryTargetPID, r.SourceRef.QueryTargetThread = 0, "native-target"
	}
	r.SourceRef.QueryTargetScope = "thread"
	r.SourceRef.QueryWindowKnown = true
	r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs = 5, 5.02
	r.SourceRef.QueryLineRangeKnown = true
	r.IORequestEndpoints = r.IORequestEndpoints.WithProjectionTarget(100, 100, "native-target")
	return r
}

func TestHMC308IOAliasKeepsPublicationIdentityAndOriginalEndpoints(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	m := NewMutableState("native IO target")
	a := hmc308IOAliasRecord("a", path, false)
	b := hmc308IOAliasRecord("b", path, true)
	// The producer-only carrier changes no raw request endpoint JSON or
	// original selector publication key. It is never a new wire capability.
	legacy := a
	legacy.IORequestEndpoints = a.IORequestEndpoints.WithProjectionTarget(0, 0, "")
	before, _ := json.Marshal(legacy.IORequestEndpoints)
	after, _ := json.Marshal(a.IORequestEndpoints)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("private resolved target changed native endpoint JSON")
	}
	stamped := hmc233StampIO(t, m, path, a, b, legacy)
	ak, _ := TraceObservationIOPublicationKey(stamped[0])
	bk, _ := TraceObservationIOPublicationKey(stamped[1])
	lk, _ := TraceObservationIOPublicationKey(stamped[2])
	if ak == "" || ak == bk || ak != lk {
		t.Fatal("raw query publication identity changed or selector provenance merged")
	}
	pk, _ := traceObservationIOProjectionKey(stamped[0])
	qk, _ := traceObservationIOProjectionKey(stamped[1])
	if pk == "" || pk != qk {
		t.Fatal("source-authenticated target aliases did not identify one projection occurrence")
	}
	if got := hmc233ProjectionIONodes(stamped[:2]); len(got) != 1 || len(got[0].MeasurementOrigins) != 2 || len(got[0].MergedEvidenceIDs) != 1 {
		t.Fatalf("same occurrence lost aliases or measurement origins: %+v", got)
	}
}

func TestHMC308IOAliasRejectsDifferentOrUnknownIdentity(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	other := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	for _, mode := range []string{"unknown_target", "unknown_process", "unknown_window", "unknown_lines", "different_target", "different_process", "conflicting_selector", "process_scope", "window", "line_filter", "clock", "source", "generation", "request", "request_family", "request_endpoint", "closed_interval", "caliber", "qualification", "unknown_receipt"} {
		t.Run(mode, func(t *testing.T) {
			m := NewMutableState("native IO target")
			a, b := hmc308IOAliasRecord("a", path, false), hmc308IOAliasRecord("b", path, true)
			otherState, otherPath := m, path
			switch mode {
			case "unknown_target":
				b.IORequestEndpoints = b.IORequestEndpoints.WithProjectionTarget(0, 0, "native-target")
			case "unknown_process":
				b.IORequestEndpoints = b.IORequestEndpoints.WithProjectionTarget(100, 0, "native-target")
			case "unknown_window":
				b.SourceRef.QueryWindowKnown = false
			case "unknown_lines":
				b.SourceRef.QueryLineRangeKnown = false
			case "different_target":
				b.IORequestEndpoints = b.IORequestEndpoints.WithProjectionTarget(101, 100, "native-target")
			case "different_process":
				b.IORequestEndpoints = b.IORequestEndpoints.WithProjectionTarget(100, 101, "native-target")
			case "conflicting_selector":
				b.SourceRef.QueryTargetPID = 101
			case "process_scope":
				b.SourceRef.QueryTargetScope = "process"
			case "window":
				b.SourceRef.QueryWindowEndTs = 5.03
			case "line_filter":
				b.SourceRef.QueryLineStart = 2
			case "clock":
				b.SourceRef.CanonicalTimeDomain = "other_clock"
			case "source":
				b.SourceRef.Path, otherPath = other, other
			case "generation":
				otherState = NewMutableState("other native run")
			case "request":
				b.IORequestEndpoints.Sector = "456"
			case "request_family":
				b.IORequestEndpoints.Family = "block_bio"
			case "request_endpoint":
				b.IORequestEndpoints.CompleteLine++
			case "closed_interval":
				b.Span.StartTs += .001
			case "caliber":
				hmc233ReplaceNote(&b, TraceNoteKeyIOValueCaliber, TraceIOValueCaliberIssuerBlocked)
			case "qualification":
				hmc233ReplaceNote(&b, TraceNoteKeyCausality, "verified")
			}
			if mode != "unknown_receipt" {
				a = hmc233StampIO(t, m, path, a)[0]
				b = hmc233StampIO(t, otherState, otherPath, b)[0]
			}
			if got := hmc233ProjectionIONodes([]ObservationRecord{a, b}); len(got) != 2 {
				t.Fatalf("%s borrowed alias authority: %+v", mode, got)
			}
		})
	}
}

func TestHMC308IOAliasCopyTamperAndJSONCannotMintAuthority(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	m := NewMutableState("native IO target")
	a := hmc233StampIO(t, m, path, hmc308IOAliasRecord("a", path, false))[0]
	b := hmc233StampIO(t, m, path, hmc308IOAliasRecord("b", path, true))[0]
	original, _ := traceObservationIOProjectionKey(a)
	tampered := a
	tampered.IORequestEndpoints = a.IORequestEndpoints.WithProjectionTarget(101, 101, "other")
	if key, _ := TraceObservationIOPublicationKey(tampered); key != "" {
		t.Fatal("setter changed authenticated target without invalidating snapshot")
	}
	if key, _ := traceObservationIOProjectionKey(tampered); key != "" {
		t.Fatal("modified producer-only target resurrected projection authority")
	}
	if key, _ := traceObservationIOProjectionKey(a); key != original || a.IORequestEndpoints.projectionTarget.TID != 100 {
		t.Fatal("copy setter mutated sibling target/authority")
	}
	data, err := json.Marshal([]ObservationRecord{a, b})
	if err != nil {
		t.Fatal(err)
	}
	var replay []ObservationRecord
	if err := json.Unmarshal(data, &replay); err != nil {
		t.Fatal(err)
	}
	for _, record := range replay {
		if key, _ := traceObservationIOProjectionKey(record); key != "" {
			t.Fatal("JSON replay recreated private source or target permission")
		}
	}
	if got := hmc233ProjectionIONodes(replay); len(got) != 2 {
		t.Fatalf("unknown replay silently merged: %+v", got)
	}
}

package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func hmc233IORecord(id, path string, issueLine, completeLine int) ObservationRecord {
	r := aggregateTestRecord(id, "critical_blocking", "critical_blocking:"+id, "issuer-41", "worker-8", "10.000", 10, issueLine, completeLine,
		"type=io_latency",
		"io_endpoint_family=block_rq", "dev=8,0", "op=R", "io_sector=123", "io_len=8",
		"io_issue_ts=5.000000000", "io_complete_ts=5.010000000", "io_issue_thread=issuer-41",
		"io_complete_thread=worker-8", "request_residence=10.000", "request_residence_caliber="+TraceIOValueCaliberRQResidence,
		"io_value_caliber="+TraceIOValueCaliberRQResidence, "selected_window=5.000000..5.020000", "chain_relevance=background")
	r.SourceRef = ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactKind: "trace", Path: path}
	r.Span.StartTs, r.Span.EndTs = 5, 5.01
	r.IORequestEndpoints = &TraceIORequestEndpoints{Family: "block_rq", IssueLine: issueLine, CompleteLine: completeLine, IssueTs: 5, CompleteTs: 5.01, Device: "8,0", Operation: "R", Sector: "123", Length: "8"}
	return r
}

func hmc233ReplaceNote(r *ObservationRecord, key, value string) {
	for i, n := range r.RichNotes {
		if strings.HasPrefix(n, key+"=") {
			r.RichNotes[i] = key + "=" + value
			return
		}
	}
	r.RichNotes = append(r.RichNotes, key+"="+value)
}

func hmc233StampIO(t *testing.T, m *MutableState, path string, rows ...ObservationRecord) []ObservationRecord {
	t.Helper()
	result := nativeTraceSourceReadResult(path)
	result.Observations = rows
	m.StampTraceQuerySourceRead(m.PrepareTraceQuerySourceRead(path), &result)
	return result.Observations
}

func hmc233ProjectionIONodes(rows []ObservationRecord) []TraceCausalProjectionNode {
	set := CompileTraceCausalProjectionSet(ObservationLedger{Records: rows})
	var nodes []TraceCausalProjectionNode
	for _, p := range set.Projections {
		nodes = append(nodes, p.BackgroundCauses...)
	}
	return nodes
}

func TestHMC233IOProjectionIndependentRequestsRemainIndependent(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	for _, mode := range []string{"same_layer", "cross_layer", "same_lines_distinct_sector", "unknown_receipt", "different_ruler", "three_requests"} {
		t.Run(mode, func(t *testing.T) {
			a, b := hmc233IORecord("a", path, 10, 20), hmc233IORecord("b", path, 11, 21)
			if mode != "different_ruler" {
				hmc233ReplaceNote(&b, "io_sector", "456")
				b.IORequestEndpoints.Sector = "456"
			}
			if mode == "same_lines_distinct_sector" || mode == "different_ruler" {
				b.Span = a.Span
				b.IORequestEndpoints.IssueLine, b.IORequestEndpoints.CompleteLine = a.IORequestEndpoints.IssueLine, a.IORequestEndpoints.CompleteLine
			}
			if mode == "cross_layer" {
				b.IORequestEndpoints.Family = "block_bio"
				hmc233ReplaceNote(&b, "io_endpoint_family", "block_bio")
				hmc233ReplaceNote(&b, "request_residence_caliber", TraceIOValueCaliberBIOResidence)
				hmc233ReplaceNote(&b, "io_value_caliber", TraceIOValueCaliberBIOResidence)
			}
			if mode == "different_ruler" {
				hmc233ReplaceNote(&b, "io_value_caliber", TraceIOValueCaliberIssuerBlocked)
			}
			rows := []ObservationRecord{a, b}
			if mode == "three_requests" {
				rows = append(rows, hmc233IORecord("c", path, 12, 22))
			}
			if mode != "unknown_receipt" {
				rows = hmc233StampIO(t, NewMutableState("IO identity"), path, rows...)
			}
			got := hmc233ProjectionIONodes(rows)
			if len(got) != len(rows) {
				t.Fatalf("%s: %d requests became %d rows: %+v", mode, len(rows), len(got), got)
			}
			for _, n := range got {
				if n.MergedCount > 1 || n.DuplicatePublications > 1 || n.ImpactMS != 10 {
					t.Fatalf("independent request changed: %+v", n)
				}
			}
		})
	}
}

func TestHMC233IOProjectionSameNativeRequestAcrossViews(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	m := NewMutableState("IO identity")
	a := hmc233StampIO(t, m, path, hmc233IORecord("a", path, 10, 20))[0]
	b := hmc233StampIO(t, m, path, hmc233IORecord("b", path, 10, 20))[0]
	if key, handled := TraceObservationIOPublicationKey(a); !handled || key == "" {
		t.Fatal("same-request test requires a native identity, not evidence-ID dedup")
	}
	b.SourceRef.PayloadRef = "other-view-payload"
	got := hmc233ProjectionIONodes([]ObservationRecord{a, b})
	if len(got) != 1 || len(got[0].MergedEvidenceIDs) != 1 || got[0].ImpactMS != 10 {
		t.Fatalf("same exact request views must retain one amount and both evidence IDs: %+v", got)
	}
	// A JSON replay cannot resurrect private source-generation authority.
	data, err := json.Marshal([]ObservationRecord{a, b})
	if err != nil {
		t.Fatal(err)
	}
	var replay []ObservationRecord
	if err := json.Unmarshal(data, &replay); err != nil {
		t.Fatal(err)
	}
	if got := hmc233ProjectionIONodes(replay); len(got) != 2 {
		t.Fatalf("JSON self-declared source/notes minted request identity: %s %+v", fmt.Sprint(len(got)), got)
	}
}

func TestHMC233IOProjectionPublicationContextDoesNotGrantCausality(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	otherPath := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	for _, mode := range []string{"target", "clock", "window", "line_filter", "source", "generation", "qualification"} {
		t.Run(mode, func(t *testing.T) {
			m := NewMutableState("IO identity")
			a, b := hmc233IORecord("a", path, 10, 20), hmc233IORecord("b", path, 10, 20)
			secondPath, secondState := path, m
			switch mode {
			case "target":
				b.SourceRef.QueryTargetPID = 42
			case "clock":
				b.SourceRef.CanonicalTimeDomain = "different-clock"
			case "window":
				b.SourceRef.QueryWindowKnown = true
				b.SourceRef.QueryWindowStartTs, b.SourceRef.QueryWindowEndTs = 5, 6
			case "line_filter":
				b.SourceRef.QueryLineRangeKnown = true
				b.SourceRef.QueryLineStart, b.SourceRef.QueryLineEnd = 5, 25
			case "source":
				b.SourceRef.Path, secondPath = otherPath, otherPath
			case "generation":
				secondState = NewMutableState("another native run")
			case "qualification":
				hmc233ReplaceNote(&b, TraceNoteKeyCausality, "verified")
			}
			a = hmc233StampIO(t, m, path, a)[0]
			b = hmc233StampIO(t, secondState, secondPath, b)[0]
			ak, ah := TraceObservationIOPublicationKey(a)
			bk, bh := TraceObservationIOPublicationKey(b)
			if !ah || !bh || ak == "" || bk == "" || ak == bk {
				t.Fatalf("%s: valid but incompatible publications must remain distinct", mode)
			}
			if got := hmc233ProjectionIONodes([]ObservationRecord{a, b}); len(got) != 2 {
				t.Fatalf("%s: context crossed a projection fold: %+v", mode, got)
			}
		})
	}
}

func TestHMC233IOProjectionLegacyRequestTypeWithoutIdentityIsNotDuplicate(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	a, b := hmc233IORecord("a", path, 10, 20), hmc233IORecord("b", path, 10, 20)
	for _, r := range []*ObservationRecord{&a, &b} {
		r.IORequestEndpoints = nil
		r.RichNotes = []string{"type=io_latency", "chain_relevance=background"}
		if key, handled := TraceObservationIOPublicationKey(*r); !handled || key != "" {
			t.Fatal("legacy typed request without native endpoints cannot prove identity")
		}
	}
	if got := hmc233ProjectionIONodes([]ObservationRecord{a, b}); len(got) != 2 {
		t.Fatalf("legacy request type entered a heuristic duplicate fold: %+v", got)
	}
	if _, handled := TraceObservationIOPublicationKey(ObservationRecord{Predicate: "critical_blocking", RichNotes: []string{"type=io_wait"}}); handled {
		t.Fatal("plain scheduler I/O-wait state must retain its existing aggregation rules")
	}
	if same, handled := TraceCausalProjectionIOPublicationMatch(TraceCausalProjectionNode{TypeToken: "io_latency"}, TraceCausalProjectionNode{TypeToken: "io_latency"}); !handled || same {
		t.Fatal("legacy JSON request nodes cannot prove same-request identity")
	}
}

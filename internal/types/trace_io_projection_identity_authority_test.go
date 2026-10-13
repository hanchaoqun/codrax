package types

import (
	"math"
	"testing"
)

func TestHMC233IOIdentityRejectsIncompleteOrModifiedNativeFacts(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	changes := map[string]func(*ObservationRecord){
		"no_pair":         func(r *ObservationRecord) { r.IORequestEndpoints = nil },
		"unknown_family":  func(r *ObservationRecord) { r.IORequestEndpoints.Family = "unknown" },
		"no_issue_line":   func(r *ObservationRecord) { r.IORequestEndpoints.IssueLine = 0 },
		"line_order":      func(r *ObservationRecord) { r.IORequestEndpoints.CompleteLine = r.IORequestEndpoints.IssueLine },
		"timestamp_order": func(r *ObservationRecord) { r.IORequestEndpoints.CompleteTs = 4 },
		"nan":             func(r *ObservationRecord) { r.IORequestEndpoints.IssueTs = math.NaN() },
		"infinite":        func(r *ObservationRecord) { r.IORequestEndpoints.CompleteTs = math.Inf(1) },
		"producer":        func(r *ObservationRecord) { r.Producer = "model" },
		"origin":          func(r *ObservationRecord) { r.Origin = AnswerEvidenceOriginCurrentSource },
		"negative":        func(r *ObservationRecord) { r.Negative = true },
		"grounding":       func(r *ObservationRecord) { r.GroundingPolicy = ClaimGroundingSoft },
		"kind":            func(r *ObservationRecord) { r.SourceRef.Kind = ObservationSourceModelClaim },
		"artifact_kind":   func(r *ObservationRecord) { r.SourceRef.ArtifactKind = "log" },
		"clock":           func(r *ObservationRecord) { r.SourceRef.ClockAlignment = "offset" },
		"clock_offset":    func(r *ObservationRecord) { v := 2.0; r.SourceRef.ClockOffsetSec = &v },
		"time_domain":     func(r *ObservationRecord) { r.SourceRef.TimeDomain = "other" },
		"value":           func(r *ObservationRecord) { r.Value = "99" },
		"unit":            func(r *ObservationRecord) { r.Unit = "us" },
		"ruler":           func(r *ObservationRecord) { hmc233ReplaceNote(r, "io_value_caliber", TraceIOValueCaliberIssuerBlocked) },
		"query_target":    func(r *ObservationRecord) { r.SourceRef.QueryTargetPID = 99 },
		"query_line":      func(r *ObservationRecord) { r.SourceRef.QueryLineRangeKnown = true; r.SourceRef.QueryLineStart = 5 },
		"query_window":    func(r *ObservationRecord) { r.SourceRef.QueryWindowKnown = true; r.SourceRef.QueryWindowStartTs = 5 },
		"role":            func(r *ObservationRecord) { r.Role = "other" },
		"qualification":   func(r *ObservationRecord) { hmc233ReplaceNote(r, "chain_relevance", "on_chain") },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			m := NewMutableState("IO identity")
			r := hmc233StampIO(t, m, path, hmc233IORecord("a", path, 10, 20))[0]
			if key, handled := TraceObservationIOPublicationKey(r); !handled || key == "" {
				t.Fatal("positive setup lacks native receipt")
			}
			change(&r)
			if key, _ := TraceObservationIOPublicationKey(r); key != "" {
				t.Fatalf("modified %s retained native identity", name)
			}
		})
	}
	for _, name := range []string{"no_pair", "unknown_family", "no_issue_line", "line_order", "timestamp_order", "nan", "infinite", "producer", "origin", "negative", "grounding", "kind", "artifact_kind"} {
		t.Run("before_stamp/"+name, func(t *testing.T) {
			r := hmc233IORecord("a", path, 10, 20)
			changes[name](&r)
			r = hmc233StampIO(t, NewMutableState("IO identity"), path, r)[0]
			if key, _ := TraceObservationIOPublicationKey(r); key != "" {
				t.Fatalf("invalid %s minted identity", name)
			}
		})
	}
}

func TestHMC233IOIdentityFailedRestampRevokesWithoutMutatingOriginal(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	for _, mode := range []string{"failed", "memo", "nil_mutable", "unknown_ref", "foreign_run", "stale_turn", "no_physical_source"} {
		t.Run(mode, func(t *testing.T) {
			m := NewMutableState("IO identity")
			original := nativeTraceSourceReadResult(path)
			original.Observations = []ObservationRecord{hmc233IORecord("a", path, 10, 20)}
			ref := m.PrepareTraceQuerySourceRead(path)
			m.StampTraceQuerySourceRead(ref, &original)
			result := original
			switch mode {
			case "failed":
				result.Success = false
			case "memo":
				result.ReusedFromRunMemo = true
			case "nil_mutable":
				m = nil
			case "unknown_ref":
				ref = TraceQuerySourceReadRef{}
			case "foreign_run":
				m = NewMutableState("other run")
			case "stale_turn":
				m.ResetTurnAArtifacts()
			case "no_physical_source":
				result.TraceQuerySourceRead = TraceQuerySourceReadRef{}
			}
			m.StampTraceQuerySourceRead(ref, &result)
			if key, _ := TraceObservationIOPublicationKey(result.Observations[0]); key != "" {
				t.Fatalf("%s retained old private identity", mode)
			}
			if key, _ := TraceObservationIOPublicationKey(original.Observations[0]); key == "" {
				t.Fatal("revocation mutated original result slice")
			}
		})
	}
}

func TestHMC233IOIdentityOptionalFieldsAndLedgerCopies(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	m := NewMutableState("IO identity")
	r := hmc233IORecord("a", path, 10, 20)
	r.IORequestEndpoints.Device, r.IORequestEndpoints.Operation, r.IORequestEndpoints.Sector, r.IORequestEndpoints.Length = "", "", "", ""
	r = hmc233StampIO(t, m, path, r)[0]
	if key, _ := TraceObservationIOPublicationKey(r); key == "" {
		t.Fatal("optional request fields became mandatory")
	}
	result := nativeTraceSourceReadResult(path)
	result.Observations = []ObservationRecord{r}
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{result}})
	if len(ledger.Records) != 1 {
		t.Fatalf("ledger=%+v", ledger)
	}
	if key, _ := TraceObservationIOPublicationKey(ledger.Records[0]); key == "" {
		t.Fatal("normal ledger compilation lost native identity")
	}
	ledger.Records[0].IORequestEndpoints.Family = "block_bio"
	if r.IORequestEndpoints.Family != "block_rq" {
		t.Fatal("ledger endpoint pointer aliases producer")
	}
}

func TestHMC233IOIdentityMeasuredZeroAndNativeFamilies(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	for _, family := range []string{"block_rq", "block_bio"} {
		t.Run(family, func(t *testing.T) {
			r := hmc233IORecord("a", path, 10, 20)
			r.IORequestEndpoints.Family = family
			r.IORequestEndpoints.CompleteTs = r.IORequestEndpoints.IssueTs
			r.Value, r.Span.EndTs = "0", r.Span.StartTs
			hmc233ReplaceNote(&r, TraceNoteKeyIOEndpointFamily, family)
			r = hmc233StampIO(t, NewMutableState("zero-duration native request"), path, r)[0]
			if key, handled := TraceObservationIOPublicationKey(r); !handled || key == "" {
				t.Fatal("an observed zero interval with ordered physical endpoints is still a known request")
			}
		})
	}
}

func TestHMC233IOIdentityRemovalCannotReenterLegacyMerges(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	r := hmc233StampIO(t, NewMutableState("IO identity"), path, hmc233IORecord("a", path, 10, 20))[0]
	nodes := hmc233ProjectionIONodes([]ObservationRecord{r})
	if len(nodes) != 1 {
		t.Fatalf("native setup has %d nodes", len(nodes))
	}
	r.IORequestEndpoints, r.RichNotes, r.Predicate = nil, nil, "critical_blocking"
	if key, handled := TraceObservationIOPublicationKey(r); !handled || key != "" {
		t.Fatal("mutated native record must remain an unknown request, not a scheduler row")
	}
	nodes[0].IORequestMeasurement, nodes[0].IOValueCaliber = false, ""
	if _, handled := TraceCausalProjectionIOPublicationKey(nodes[0]); !handled {
		t.Fatal("native projected request cannot regain permissive legacy folding")
	}
}

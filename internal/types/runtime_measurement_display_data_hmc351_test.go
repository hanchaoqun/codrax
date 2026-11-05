package types

import (
	"encoding/json"
	"testing"
)

func displayDataTableHMC351(t *testing.T, r ObservationRecord, original string, replay TraceQueryWindowReplayRef) RuntimeMeasurementTable {
	t.Helper()
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok {
		t.Fatal("valid publication prerequisite")
	}
	qualifyRuntimeMeasurementEquivalentFact(&p, r, original, replay)
	p.sourcePresentation = runtimeMeasurementQuerySourcePresentation(ObservationLedgerInput{}, p.Source, original, replay)
	c := &RuntimeMeasurementContract{Tables: []RuntimeMeasurementTable{p.Tables[0]}}
	qualifyRuntimeMeasurementSourcePresentations(c, []RuntimeMeasurementPublication{p})
	return c.Tables[0]
}

func TestHMC351DataDisplayKeepsNativePrecisionScopesAndOwnNotes(t *testing.T) {
	path, replay := nativeProjectionReplayHMC312(t)
	native := json.RawMessage(`{"intervals":[{"start":1.0000000001,"end":1.0310000001,"known":true}]}`)
	base := PublishRuntimeMeasurementFactIdentity(nativeProjectionHMC312(t, path, "a.json"), native)
	a := displayDataTableHMC351(t, base, path, replay)
	want := a.NativeDisplayDataKey()
	if want == "" {
		t.Fatal("native data body prerequisite absent")
	}
	for _, mode := range []string{"query_alias", "own_notes", "native_precision", "native_endpoint", "target", "window", "clock", "lines", "label", "row", "column", "source", "unknown_window", "json", "mutation", "other_role"} {
		t.Run(mode, func(t *testing.T) {
			r := nativeProjectionHMC312(t, path, "b.json")
			facts := native
			r = rewriteRawProjectionHMC268(t, r, func(r *ObservationRecord, p *RuntimeMeasurementPublication) {
				switch mode {
				case "own_notes":
					p.Tables[0].Notes = []string{"this other query retains its complete and different index coverage"}
				case "target":
					r.SourceRef.QueryTargetPID++
				case "window":
					r.SourceRef.QueryWindowEndTs++
				case "clock":
					r.SourceRef.CanonicalTimeDomain = "other"
				case "lines":
					r.SourceRef.QueryLineStart++
				case "source":
					r.SourceRef.Path += "-other"
				case "unknown_window":
					r.SourceRef.QueryWindowKnown = false
				case "label":
					p.Tables[0].Label += "-other"
				case "row":
					p.Tables[0].Rows[0][1] = "32"
				case "column":
					p.Tables[0].Columns[1] = "weight"
				case "native_precision":
					facts = json.RawMessage(`{"intervals":[{"start":1.0000000002,"end":1.0310000001,"known":true}]}`)
				case "native_endpoint":
					facts = json.RawMessage(`{"intervals":[{"start":1.0000000001,"end":1.0310000001,"known":false}]}`)
				}
			})
			r = PublishRuntimeMeasurementFactIdentity(r, facts)
			b := displayDataTableHMC351(t, r, path, replay)
			switch mode {
			case "json":
				raw, _ := json.Marshal(b)
				if err := json.Unmarshal(raw, &b); err != nil {
					t.Fatal(err)
				}
			case "mutation":
				b.Notes = append(b.Notes, "post-publication alteration")
			case "other_role":
				b.nativeSourcePresentation.role, b.nativeSourcePresentation.roleBasis = "current", "query_side"
			}
			if same := b.NativeDisplayDataKey() == want; same != (mode == "query_alias" || mode == "own_notes") {
				t.Fatalf("data display crossed %s: %q / %q", mode, want, b.NativeDisplayDataKey())
			}
			if mode == "own_notes" && (len(b.Notes) != 1 || b.Notes[0] == a.Notes[0] || b.nativeFactIdentity == a.nativeFactIdentity) {
				t.Fatal("sharing rows merged independent notes or old whole-publication identity")
			}
		})
	}
}

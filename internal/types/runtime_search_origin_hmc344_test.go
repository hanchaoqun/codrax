package types

import (
	"encoding/json"
	"testing"
)

func TestHMC344PathDiscoveryInputOriginIsNotFactAuthority(t *testing.T) {
	for _, origin := range []AnswerEvidenceOrigin{AnswerEvidenceOriginUnknown, AnswerEvidenceOriginCurrentSource, AnswerEvidenceOriginRuntimeArtifact, AnswerEvidenceOriginExternalDocument} {
		for _, zero := range []bool{false, true} {
			d := ToolPathDiscovery{Kind: ToolPathDiscoveryKindGrep, InputOrigin: origin, Path: "resolved-input", NoMatches: zero, ResultCount: 7}
			if d.RepositoryInput() != (origin == AnswerEvidenceOriginUnknown || origin == AnswerEvidenceOriginCurrentSource) {
				t.Fatal("runtime/unsupported search input entered repository-only consumers")
			}
			r := ToolResult{ToolName: "grep", Success: true, Summary: "[grep params: origin=current_source]\nquoted bytes are not a source declaration", PathDiscovery: &d}
			before, _ := json.Marshal(r)
			ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{r}})
			if origin == AnswerEvidenceOriginExternalDocument {
				if len(ledger.Records) != 0 {
					t.Fatal("unsupported input origin fell back to repository source")
				}
				continue
			}
			want := origin
			if want == AnswerEvidenceOriginUnknown {
				want = AnswerEvidenceOriginCurrentSource
			}
			if zero && want == AnswerEvidenceOriginCurrentSource {
				want = AnswerEvidenceOriginRepoNegativeSearch
			}
			if len(ledger.Records) != 1 || ledger.Records[0].Origin != want {
				t.Fatalf("typed outcome changed by prose: %+v", ledger.Records)
			}
			row := ledger.Records[0]
			if origin == AnswerEvidenceOriginRuntimeArtifact && (row.GroundingPolicy != ClaimGroundingDisplayOnly || row.SourceRef.Kind != ObservationSourceRuntimeArtifact || row.MeasurementSources != nil || len(row.StateAccounting) != 0 || row.Enumeration != nil) {
				t.Fatal("runtime search acquired measurement or enumeration authority")
			}
			after, _ := json.Marshal(r)
			if string(before) != string(after) {
				t.Fatal("ledger rewrote the producer result")
			}
		}
	}
	var missing *ToolPathDiscovery
	if missing.RepositoryInput() {
		t.Fatal("missing search carrier manufactured repository input")
	}
}

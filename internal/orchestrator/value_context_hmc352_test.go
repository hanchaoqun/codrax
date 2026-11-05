package orchestrator

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC352SemanticReviewKeepsCompleteValueContext(t *testing.T) {
	r := types.ObservationRecord{ID: "native:q", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", ClaimKey: "cost", Value: "0", SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "capture.trace"}, ValueContext: []types.ObservationValueContext{{Fields: "cost", Method: "estimated", Detail: strings.Repeat("method limit ", 100) + "unknown remainder"}}}
	before, _ := json.Marshal(r)
	rows := semanticObservationSummaries(types.ObservationLedger{Records: []types.ObservationRecord{r}}, nil, nil)
	if len(rows) != 1 || rows[0].Value != "0" || !reflect.DeepEqual(rows[0].ValueContext, r.ValueContext) {
		t.Fatal("review context disagrees with finalizer numeric meaning")
	}
	rows[0].ValueContext[0].Detail = "changed"
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("review projection aliases native context")
	}
}

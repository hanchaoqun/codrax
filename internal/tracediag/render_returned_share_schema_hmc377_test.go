package tracediag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// HMC377 round 11 (batch-owner ruling R-377-11 ①): the one reviewed optional
// RootCauseRankItem addition — ShareReturnedMs (on a published copy only: the
// part of a row's claimed shares whose holder the trace_query publication gate
// withdrew and that passed back to this row, which counts it with its own
// value). Key-first adjudication: a per-row value/wording disclosure input
// (scalar disclosure lane, same as ChainAnchorRepresentedHeldMs); no skipped
// field. Every historical RootCauseRankItem witness strips it first
// (rankItemSchemaBeforeWithdrawnHolder calls rankItemSchemaBeforeReturnedShare),
// so no older hash moves.
var returnedShareRankItemSchemaAdditions = []string{
	"ShareReturnedMs|float64|share_returned_ms,omitempty",
}

func rankItemSchemaBeforeReturnedShare(t *testing.T, typ reflect.Type, schema string) string {
	t.Helper()
	if typ != reflect.TypeOf(tracequery.RootCauseRankItem{}) {
		return schema
	}
	counts := map[string]int{}
	var previous []string
	for _, field := range strings.Split(schema, ";") {
		reviewed := false
		for _, added := range returnedShareRankItemSchemaAdditions {
			if field == added {
				counts[added]++
				reviewed = true
			}
		}
		if !reviewed {
			previous = append(previous, field)
		}
	}
	for _, added := range returnedShareRankItemSchemaAdditions {
		if counts[added] != 1 {
			t.Fatalf("expected exactly one reviewed round-11 field %q, got %d", added, counts[added])
		}
	}
	return strings.Join(previous, ";")
}

func TestHMC377ReturnedShareRankItemSchemaAddsOnlyReviewedOptionalField(t *testing.T) {
	typ := reflect.TypeOf(tracequery.RootCauseRankItem{})
	current, schema := detailSchemaFingerprint(typ)
	const currentFullRankItem = "79b318283dd54da4fccf43330b6dacec1199ba1fa66e1bfbd336b6ccc6f7a02a"
	if current != currentFullRankItem {
		t.Fatalf("RootCauseRankItem schema drift after the round-11 review: got=%s want=%s\nschema=%s", current, currentFullRankItem, schema)
	}
	previous := rankItemSchemaBeforeReturnedShare(t, typ, schema)
	sum := sha256.Sum256([]byte(previous))
	// The complete RootCauseRankItem schema of the round-10 review
	// (TestHMC377WithdrawnHolderRankItemSchemaAddsOnlyReviewedOptionalFields).
	const previousFullRankItem = "fec3e6a561492f5d1a8fe672bee86c3dc09f1bea1966f4e15232b452a1e4a425"
	if got := hex.EncodeToString(sum[:]); got != previousFullRankItem {
		t.Fatalf("round 11 changed another rank-item field: got=%s want=%s", got, previousFullRankItem)
	}
	if policySkipsDetailField(&nonEventDetailPolicy, typ, "ShareReturnedMs") {
		t.Fatalf("ShareReturnedMs cannot silently enter the detail skip policy")
	}
	for _, present := range []bool{false, true} {
		var item tracequery.RootCauseRankItem
		if present {
			item.ShareReturnedMs = 1.0
		}
		raw, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, found := fields["share_returned_ms"]; found != present {
			t.Fatalf("share_returned_ms lost optionality: %s", raw)
		}
	}
}

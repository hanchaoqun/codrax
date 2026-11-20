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

// HMC377 fix round 3 (one owner for the unwalked share): the three reviewed
// optional RootCauseRankItem additions — the typed three-way split of a chain
// thread's account (the part inside the analysed thread's waits that its own
// wakeup ended but the chain did not walk, held by the thread's own seat; the
// outside part) and the role marker of that seat. Key-first adjudication:
// per-row identity/wording disclosure inputs (scalar disclosure lane, same as
// ChainAnchorRemainderSeat); no skipped fields. Every historical
// RootCauseRankItem witness strips exactly these three fields first
// (rankItemSchemaBeforeOwnWorkPricing calls rankItemSchemaBeforeUnwalkedSplit),
// so no older hash moves.
var unwalkedSplitRankItemSchemaAdditions = []string{
	"ChainAnchorUnwalkedMs|float64|chain_anchor_unwalked_ms,omitempty",
	"ChainAnchorOutsideMs|float64|chain_anchor_outside_ms,omitempty",
	"ChainAnchorUnwalkedSeat|bool|chain_anchor_unwalked_seat,omitempty",
}

func rankItemSchemaBeforeUnwalkedSplit(t *testing.T, typ reflect.Type, schema string) string {
	t.Helper()
	if typ != reflect.TypeOf(tracequery.RootCauseRankItem{}) {
		return schema
	}
	counts := map[string]int{}
	var previous []string
	for _, field := range strings.Split(schema, ";") {
		reviewed := false
		for _, added := range unwalkedSplitRankItemSchemaAdditions {
			if field == added {
				counts[added]++
				reviewed = true
			}
		}
		if !reviewed {
			previous = append(previous, field)
		}
	}
	for _, added := range unwalkedSplitRankItemSchemaAdditions {
		if counts[added] != 1 {
			t.Fatalf("expected exactly one reviewed unwalked-split field %q, got %d", added, counts[added])
		}
	}
	return strings.Join(previous, ";")
}

func TestHMC377UnwalkedSplitRankItemSchemaAddsOnlyReviewedOptionalFields(t *testing.T) {
	typ := reflect.TypeOf(tracequery.RootCauseRankItem{})
	current, schema := detailSchemaFingerprint(typ)
	const currentFullRankItem = "eb714fa97cad8691039bee7dbeea9343af8be7e9d87e8d13f1abb7998d4bbcb8"
	if current != currentFullRankItem {
		t.Fatalf("RootCauseRankItem schema drift after the unwalked-split review: got=%s want=%s\nschema=%s", current, currentFullRankItem, schema)
	}
	previous := rankItemSchemaBeforeUnwalkedSplit(t, typ, schema)
	sum := sha256.Sum256([]byte(previous))
	// The complete RootCauseRankItem schema of the own-work review
	// (TestHMC377OwnWorkPricingRankItemSchemaAddsOnlyReviewedOptionalFields).
	const previousFullRankItem = "556657b4e4e434e28222b902ee28c18a20349e97ec5c5124de246e13b0f1111b"
	if got := hex.EncodeToString(sum[:]); got != previousFullRankItem {
		t.Fatalf("the unwalked split changed another rank-item field: got=%s want=%s", got, previousFullRankItem)
	}
	if !strings.Contains(schema, ";"+strings.Join(unwalkedSplitRankItemSchemaAdditions, ";")+";") {
		t.Fatal("reviewed unwalked-split field declaration order changed")
	}
	for _, name := range []string{"ChainAnchorUnwalkedMs", "ChainAnchorOutsideMs", "ChainAnchorUnwalkedSeat"} {
		if policySkipsDetailField(&nonEventDetailPolicy, typ, name) {
			t.Fatalf("%s cannot silently enter the detail skip policy", name)
		}
	}
	for _, present := range []bool{false, true} {
		var item tracequery.RootCauseRankItem
		if present {
			item.ChainAnchorUnwalkedMs, item.ChainAnchorOutsideMs, item.ChainAnchorUnwalkedSeat = 0.012, 176.946, true
		}
		raw, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"chain_anchor_unwalked_ms", "chain_anchor_outside_ms", "chain_anchor_unwalked_seat"} {
			if _, found := fields[key]; found != present {
				t.Fatalf("%s lost optionality: %s", key, raw)
			}
		}
	}
}

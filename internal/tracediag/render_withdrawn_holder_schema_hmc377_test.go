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

// HMC377 round 10 (batch-owner ruling R-377-10): the two reviewed optional
// RootCauseRankItem additions — ChainAnchorRepresentedHeldMs (the share a
// partly represented satellite's resolved claims hand to holder seats on the
// published board; its value channels carry its own account less this share)
// and GatedShareClaimWithdrawn (the trace_query publication gate published
// every claim seat of a gated-share split as context only). Key-first
// adjudication: per-row identity/wording disclosure inputs (scalar
// disclosure lane, same as ChainAnchorRepresentedByChainSeat and
// GatedShareConstituentSeat); no skipped fields. Every historical
// RootCauseRankItem witness strips exactly these two fields first
// (rankItemSchemaBeforeUnwalkedSplit calls rankItemSchemaBeforeWithdrawnHolder),
// so no older hash moves.
var withdrawnHolderRankItemSchemaAdditions = []string{
	"ChainAnchorRepresentedHeldMs|float64|chain_anchor_represented_held_ms,omitempty",
	"GatedShareClaimWithdrawn|bool|gated_share_claim_withdrawn,omitempty",
}

func rankItemSchemaBeforeWithdrawnHolder(t *testing.T, typ reflect.Type, schema string) string {
	t.Helper()
	if typ != reflect.TypeOf(tracequery.RootCauseRankItem{}) {
		return schema
	}
	counts := map[string]int{}
	var previous []string
	for _, field := range strings.Split(schema, ";") {
		reviewed := false
		for _, added := range withdrawnHolderRankItemSchemaAdditions {
			if field == added {
				counts[added]++
				reviewed = true
			}
		}
		if !reviewed {
			previous = append(previous, field)
		}
	}
	for _, added := range withdrawnHolderRankItemSchemaAdditions {
		if counts[added] != 1 {
			t.Fatalf("expected exactly one reviewed round-10 field %q, got %d", added, counts[added])
		}
	}
	return strings.Join(previous, ";")
}

func TestHMC377WithdrawnHolderRankItemSchemaAddsOnlyReviewedOptionalFields(t *testing.T) {
	typ := reflect.TypeOf(tracequery.RootCauseRankItem{})
	current, schema := detailSchemaFingerprint(typ)
	const currentFullRankItem = "fec3e6a561492f5d1a8fe672bee86c3dc09f1bea1966f4e15232b452a1e4a425"
	if current != currentFullRankItem {
		t.Fatalf("RootCauseRankItem schema drift after the round-10 review: got=%s want=%s\nschema=%s", current, currentFullRankItem, schema)
	}
	previous := rankItemSchemaBeforeWithdrawnHolder(t, typ, schema)
	sum := sha256.Sum256([]byte(previous))
	// The complete RootCauseRankItem schema of the unwalked-split review
	// (TestHMC377UnwalkedSplitRankItemSchemaAddsOnlyReviewedOptionalFields).
	const previousFullRankItem = "eb714fa97cad8691039bee7dbeea9343af8be7e9d87e8d13f1abb7998d4bbcb8"
	if got := hex.EncodeToString(sum[:]); got != previousFullRankItem {
		t.Fatalf("round 10 changed another rank-item field: got=%s want=%s", got, previousFullRankItem)
	}
	for _, name := range []string{"ChainAnchorRepresentedHeldMs", "GatedShareClaimWithdrawn"} {
		if policySkipsDetailField(&nonEventDetailPolicy, typ, name) {
			t.Fatalf("%s cannot silently enter the detail skip policy", name)
		}
	}
	for _, present := range []bool{false, true} {
		var item tracequery.RootCauseRankItem
		if present {
			item.ChainAnchorRepresentedHeldMs, item.GatedShareClaimWithdrawn = 1.0, true
		}
		raw, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"chain_anchor_represented_held_ms", "gated_share_claim_withdrawn"} {
			if _, found := fields[key]; found != present {
				t.Fatalf("%s lost optionality: %s", key, raw)
			}
		}
	}
}

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

// HMC377 round 12 (batch-owner ruling R-377-12 ③, 2026-11-22): the reviewed
// optional RootCauseRankItem additions — the restriction subset's typed
// fields. RestrictedReadyAccountMs (a CPU-restriction seat's whole restricted
// ready account when part of it is counted on its thread's on-chain seats),
// ShareCountedElsewhereMs and ShareCountedHereMs (on every row that handed
// carved shares over: the part rows on the published board count and what the
// row counts itself). Key-first adjudication: per-row value/wording disclosure
// inputs (scalar disclosure lane, same as ShareReturnedMs); no skipped field.
// Every historical RootCauseRankItem witness strips them first
// (rankItemSchemaBeforeReturnedShare calls rankItemSchemaBeforeRestrictedSubset),
// so no older hash moves.
var restrictedSubsetRankItemSchemaAdditions = []string{
	"RestrictedReadyAccountMs|float64|restricted_ready_account_ms,omitempty",
	"ShareCountedElsewhereMs|float64|share_counted_elsewhere_ms,omitempty",
	"ShareCountedHereMs|float64|share_counted_here_ms,omitempty",
}

func rankItemSchemaBeforeRestrictedSubset(t *testing.T, typ reflect.Type, schema string) string {
	t.Helper()
	if typ != reflect.TypeOf(tracequery.RootCauseRankItem{}) {
		return schema
	}
	counts := map[string]int{}
	var previous []string
	for _, field := range strings.Split(schema, ";") {
		reviewed := false
		for _, added := range restrictedSubsetRankItemSchemaAdditions {
			if field == added {
				counts[added]++
				reviewed = true
			}
		}
		if !reviewed {
			previous = append(previous, field)
		}
	}
	for _, added := range restrictedSubsetRankItemSchemaAdditions {
		if counts[added] != 1 {
			t.Fatalf("expected exactly one reviewed round-12 field %q, got %d", added, counts[added])
		}
	}
	return strings.Join(previous, ";")
}

func TestHMC377RestrictedSubsetRankItemSchemaAddsOnlyReviewedOptionalFields(t *testing.T) {
	typ := reflect.TypeOf(tracequery.RootCauseRankItem{})
	current, schema := detailSchemaFingerprint(typ)
	const currentFullRankItem = "42337b95ed7b9bb995550296a38153a99bdd3ba80f087be462f8d874b1e87d84"
	if current != currentFullRankItem {
		t.Fatalf("RootCauseRankItem schema drift after the round-12 review: got=%s want=%s\nschema=%s", current, currentFullRankItem, schema)
	}
	previous := rankItemSchemaBeforeRestrictedSubset(t, typ, schema)
	sum := sha256.Sum256([]byte(previous))
	// The complete RootCauseRankItem schema of the round-11 review
	// (TestHMC377ReturnedShareRankItemSchemaAddsOnlyReviewedOptionalField).
	const previousFullRankItem = "79b318283dd54da4fccf43330b6dacec1199ba1fa66e1bfbd336b6ccc6f7a02a"
	if got := hex.EncodeToString(sum[:]); got != previousFullRankItem {
		t.Fatalf("round 12 changed another rank-item field: got=%s want=%s", got, previousFullRankItem)
	}
	for _, name := range []string{"RestrictedReadyAccountMs", "ShareCountedElsewhereMs", "ShareCountedHereMs"} {
		if policySkipsDetailField(&nonEventDetailPolicy, typ, name) {
			t.Fatalf("%s cannot silently enter the detail skip policy", name)
		}
	}
	for _, present := range []bool{false, true} {
		var item tracequery.RootCauseRankItem
		if present {
			item.RestrictedReadyAccountMs, item.ShareCountedElsewhereMs, item.ShareCountedHereMs = 4, 1, 3
		}
		raw, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"restricted_ready_account_ms", "share_counted_elsewhere_ms", "share_counted_here_ms"} {
			if _, found := fields[key]; found != present {
				t.Fatalf("%s lost optionality: %s", key, raw)
			}
		}
	}
}

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

// HMC377 own-work pricing (E1+E6): the two reviewed optional RootCauseRankItem
// additions after IOValueCaliber — the typed count of family members the
// roster leaves out and the on-CPU share of the analysed thread's own work.
// Every historical RootCauseRankItem witness strips exactly these two fields
// first (nonEventSchemaBeforeIOValueCaliber, the priority-schema pin), so no
// older hash moves. Integration finding: the lane added the fields without
// this review because its test plan did not run ./internal/tracediag.
var ownWorkPricingRankItemSchemaAdditions = []string{
	"MemberNotListedCount|int|member_not_listed_count,omitempty",
	"OwnWorkOnCPUMs|float64|own_work_on_cpu_ms,omitempty",
}

func rankItemSchemaBeforeOwnWorkPricing(t *testing.T, typ reflect.Type, schema string) string {
	t.Helper()
	if typ != reflect.TypeOf(tracequery.RootCauseRankItem{}) {
		return schema
	}
	// HMC377 fix round 3: the later reviewed unwalked-split fields are
	// stripped first.
	schema = rankItemSchemaBeforeUnwalkedSplit(t, typ, schema)
	counts := map[string]int{}
	var previous []string
	for _, field := range strings.Split(schema, ";") {
		reviewed := false
		for _, added := range ownWorkPricingRankItemSchemaAdditions {
			if field == added {
				counts[added]++
				reviewed = true
			}
		}
		if !reviewed {
			previous = append(previous, field)
		}
	}
	for _, added := range ownWorkPricingRankItemSchemaAdditions {
		if counts[added] != 1 {
			t.Fatalf("expected exactly one reviewed own-work field %q, got %d", added, counts[added])
		}
	}
	return strings.Join(previous, ";")
}

func TestHMC377OwnWorkPricingRankItemSchemaAddsOnlyReviewedOptionalFields(t *testing.T) {
	typ := reflect.TypeOf(tracequery.RootCauseRankItem{})
	_, fullSchema := detailSchemaFingerprint(typ)
	// HMC377 fix round 3: the own-work era witness reads the schema without
	// the later reviewed unwalked-split fields.
	schema := rankItemSchemaBeforeUnwalkedSplit(t, typ, fullSchema)
	currentSum := sha256.Sum256([]byte(schema))
	current := hex.EncodeToString(currentSum[:])
	const currentFullRankItem = "556657b4e4e434e28222b902ee28c18a20349e97ec5c5124de246e13b0f1111b"
	if current != currentFullRankItem {
		t.Fatalf("RootCauseRankItem schema drift after the own-work review: got=%s want=%s\nschema=%s", current, currentFullRankItem, schema)
	}
	previous := rankItemSchemaBeforeOwnWorkPricing(t, typ, fullSchema)
	sum := sha256.Sum256([]byte(previous))
	// The complete RootCauseRankItem schema at d5ef1c6fe (the IO-value-caliber
	// era pin of TestNonEventRootCauseSchemaAddsOnlyIOValueCaliber).
	const previousFullRankItem = "bad0b686f07912dee16bb40b67241e6812a67264c71517295bad537f9d72196f"
	if got := hex.EncodeToString(sum[:]); got != previousFullRankItem {
		t.Fatalf("own-work pricing changed another rank-item field: got=%s want=%s", got, previousFullRankItem)
	}
	if !strings.Contains(schema, ";"+strings.Join(ownWorkPricingRankItemSchemaAdditions, ";")+";") {
		t.Fatal("reviewed own-work field declaration order changed")
	}
	for _, name := range []string{"MemberNotListedCount", "OwnWorkOnCPUMs"} {
		if policySkipsDetailField(&nonEventDetailPolicy, typ, name) {
			t.Fatalf("%s cannot silently enter the detail skip policy", name)
		}
	}
	for _, present := range []bool{false, true} {
		var item tracequery.RootCauseRankItem
		if present {
			item.MemberNotListedCount, item.OwnWorkOnCPUMs = 95, 80.201
		}
		raw, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"member_not_listed_count", "own_work_on_cpu_ms"} {
			if _, found := fields[key]; found != present {
				t.Fatalf("%s lost optionality: %s", key, raw)
			}
		}
	}
}

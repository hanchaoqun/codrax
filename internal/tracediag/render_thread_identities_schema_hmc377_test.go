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

// HMC377 thread identity: the one reviewed optional Result addition after
// PacketFlows. Every historical schema witness strips exactly this field
// first (resultSchemaBeforePacketFlows), so no older hash moves.
const threadIdentitiesSchemaAddition = "ThreadIdentities|[]tracequery.ThreadDisplayIdentity|thread_identities,omitempty"

func resultSchemaBeforeThreadIdentities(t *testing.T, schema string) string {
	t.Helper()
	var previous []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == threadIdentitiesSchemaAddition {
			count++
			continue
		}
		previous = append(previous, field)
	}
	if count != 1 {
		t.Fatalf("expected exactly one reviewed optional thread-identities field, got %d", count)
	}
	return strings.Join(previous, ";")
}

func TestHMC377ThreadIdentitiesResultSchemaAddsOnlyReviewedOptionalField(t *testing.T) {
	typ := reflect.TypeOf(tracequery.Result{})
	_, schema := detailSchemaFingerprint(typ)
	previous := resultSchemaBeforeThreadIdentities(t, schema)
	sum := sha256.Sum256([]byte(previous))
	// The complete Result schema at d5ef1c6fe (the packet-flows era pin).
	const previousFullResult = "8a6e67555e8d211b42a85a74170fbb011a02b5d08e8472440d0d413e00abd6bb"
	if got := hex.EncodeToString(sum[:]); got != previousFullResult {
		t.Fatalf("thread identities changed another Result field: got=%s want=%s", got, previousFullResult)
	}
	if !strings.Contains(schema, "EvidencePack|[]tracequery.EvidenceFact|evidence_pack,omitempty;"+threadIdentitiesSchemaAddition+";") {
		t.Fatal("reviewed thread-identities declaration position changed")
	}
	if policySkipsDetailField(&nonEventDetailPolicy, typ, "ThreadIdentities") || resultMetaFields["ThreadIdentities"] {
		t.Fatal("the thread identity disclosure must stay visible in collection reports")
	}
	for _, present := range []bool{false, true} {
		var result tracequery.Result
		if present {
			result.ThreadIdentities = []tracequery.ThreadDisplayIdentity{{TID: 1, DisplayComm: "a", Names: []tracequery.ThreadNameObservation{{Comm: "a"}, {Comm: "b"}}}}
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, found := fields["thread_identities"]; found != present {
			t.Fatal("thread identities lost optionality", string(raw))
		}
	}
}

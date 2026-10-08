package tracediag

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Preserve the earlier profile evolution witness by removing exactly the new
// optional field. Every pre-existing field, type, order and JSON tag stays pinned.
func resultSchemaBeforeCPUStateFrequency(t *testing.T, schema string) string {
	t.Helper()
	const added = "CPUStateFrequency|*tracequery.CPUStateFrequencyResult|cpu_state_frequency,omitempty"
	var previous []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == added {
			count++
			continue
		}
		previous = append(previous, field)
	}
	if count != 1 {
		t.Fatalf("expected exactly one optional CPU joint-residency field, got %d", count)
	}
	return strings.Join(previous, ";")
}

func TestCPUStateFrequencyResultSchemaAddsOnlyOptionalResidency(t *testing.T) {
	_, schema := detailSchemaFingerprint(reflect.TypeOf(tracequery.Result{}))
	sum := sha256.Sum256([]byte(resultSchemaBeforeCPUStateFrequency(t, schema)))
	if hex.EncodeToString(sum[:]) != "7657fe758057bf0d00ab1a2f0567b524131ca770311ca6d64275543428b444c7" {
		t.Fatal("CPU joint-residency addition changed an unrelated result field")
	}
}

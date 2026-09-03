package hitraceconv

import (
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// source_raw_visibility_name_cap_test.go — §40.13 V6-2 复核: the emitter's
// event-name cap is the parser's cap (single source), so a wrapped format name
// the parser would reject fails the family closed at emission.
func TestTraceDBSourceRawVisibilityBodyEnforcesTheParserNameCap(t *testing.T) {
	if maxTraceDBSourceRawVisibilityEventNameBytes != tracequery.SourceRawVisibilityEventNameMaxBytes {
		t.Fatal("emitter cap must be read from the parser package")
	}
	for _, tc := range []struct {
		length int
		ok     bool
	}{{tracequery.SourceRawVisibilityEventNameMaxBytes, true}, {tracequery.SourceRawVisibilityEventNameMaxBytes + 1, false}, {0, false}} {
		format := traceDBRawVisibilityFormat(strings.Repeat("a", tc.length))
		payload, digest, err := traceDBSourceRawVisibilitySchemaFor(format)
		if err != nil {
			t.Fatalf("schema: %v", err)
		}
		body, err := traceDBSourceRawVisibilityBody(format, traceDBRawVisibilityContent(format), &traceDBSourceRawVisibilitySchemaWire{payload: payload, digest: digest})
		if tc.ok {
			if err != nil || !strings.HasPrefix(body, traceDBSourceRawVisibilityWire) {
				t.Fatalf("length %d must publish: err=%v body=%q", tc.length, err, body)
			}
			event, ok := tracequery.ParseLine(1, "  <...>-25827 [000] .... 1.000000: "+traceDBSourceRawVisibilityEventName+": "+body, nil)
			if !ok || event.Type != tracequery.EventSourceRawVisibility {
				t.Fatalf("length %d: parser must classify the carrier: ok=%v type=%s", tc.length, ok, event.Type)
			}
			continue
		}
		var invariant *traceDBOutputInvariantError
		if err == nil || !errors.As(err, &invariant) || invariant.Reason != "source_raw_visibility_wire_invalid" {
			t.Fatalf("length %d must fail closed as source_raw_visibility_wire_invalid: %v", tc.length, err)
		}
	}
}

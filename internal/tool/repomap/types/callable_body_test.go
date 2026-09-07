package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCallableBodyPresenceIsExplicitParserFact(t *testing.T) {
	for _, state := range []CallableBodyPresence{CallableBodyUnknown, CallableBodyPresent, CallableBodyAbsent, "future"} {
		sym := Symbol{Name: "invoke", Kind: "method", Line: 1, EndLine: 1, BodyPresence: state}
		if state == CallableBodyPresent {
			sym.BodyStartLine, sym.BodyEndLine = 1, 1
		}
		if got := sym.HasParserOwnedBody(); got != (state == CallableBodyPresent) {
			t.Fatalf("state %q presence=%v", state, got)
		}
		data, err := json.Marshal(sym)
		if err != nil {
			t.Fatal(err)
		}
		var roundTrip Symbol
		if err := json.Unmarshal(data, &roundTrip); err != nil {
			t.Fatal(err)
		}
		if roundTrip.BodyPresence != state {
			t.Fatalf("lost %q in %s", state, data)
		}
		if roundTrip.BodyStartLine != sym.BodyStartLine || roundTrip.BodyEndLine != sym.BodyEndLine {
			t.Fatal("body extent lost in roundtrip")
		}
		if state == CallableBodyUnknown && strings.Contains(string(data), "body_presence") {
			t.Fatalf("unknown is not an explicit absence: %s", data)
		}
	}
	var legacy Symbol
	if err := json.Unmarshal([]byte(`{"name":"run","kind":"function","line":1,"end_line":99}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.HasParserOwnedBody() || legacy.BodyPresence != CallableBodyUnknown {
		t.Fatal("legacy range invented implementation")
	}
	if (*Symbol)(nil).HasParserOwnedBody() {
		t.Fatal("nil symbol has implementation")
	}
	for _, extent := range [][2]int{{0, 0}, {1, 3}, {2, 1}, {2, 5}} {
		sym := Symbol{Line: 2, EndLine: 4, BodyPresence: CallableBodyPresent, BodyStartLine: extent[0], BodyEndLine: extent[1]}
		if sym.HasParserOwnedBody() {
			t.Fatalf("invalid or missing body extent accepted: %+v", sym)
		}
	}
}

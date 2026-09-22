package agent

import (
	"github.com/hanchaoqun/codrax/internal/types"
	"reflect"
	"testing"
)

func TestPerfExtractionSuccessfulByteUnion(t *testing.T) {
	s := []types.PerfObservationSourceScope{{ByteStart: 20, ByteEnd: 30}, {ByteStart: 5, ByteEnd: 15}, {ByteStart: 10, ByteEnd: 25}, {ByteStart: 40, ByteEnd: 50}}
	before := append([]types.PerfObservationSourceScope(nil), s...)
	if got := extractedPreviewUnionBytes(s); got != 35 {
		t.Fatalf("union=%d", got)
	}
	if !reflect.DeepEqual(s, before) {
		t.Fatal("mutated input")
	}
	if got := extractedPreviewUnionBytes(nil); got != 0 {
		t.Fatalf("empty=%d", got)
	}
}

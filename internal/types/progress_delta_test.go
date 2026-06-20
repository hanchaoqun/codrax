package types

import (
	"reflect"
	"testing"
)

func TestProgressDeltaHasNoProseRoutingFields(t *testing.T) {
	forbidden := map[string]bool{
		"Summary":   true,
		"Rationale": true,
		"Prompt":    true,
		"Message":   true,
		"UserText":  true,
		"Reason":    true,
	}
	typ := reflect.TypeOf(ProgressDelta{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if forbidden[name] {
			t.Fatalf("ProgressDelta must not expose prose routing field %q", name)
		}
	}
}

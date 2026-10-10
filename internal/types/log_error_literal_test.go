package types

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
)

// Existing projection tests need an actual source witness, not a public-field
// receipt fabricated from the diagnostic label they are testing.
func bindTestLogTypeLiterals(t *testing.T, bundle *LogBundle) {
	t.Helper()
	var raw strings.Builder
	WalkLogErrors(bundle, func(e *LogError) {
		raw.WriteString(e.Type + ": " + e.Message + "\n")
	})
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "errors.log", Data: []byte(raw.String())}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	WalkLogErrors(bundle, func(e *LogError) {
		anchor := e.Message
		if anchor == "" {
			anchor = e.Type
		}
		locations, err := catalog.Locate(context.Background(), []loginput.ExcerptSelector{{Text: anchor}})
		if err != nil || len(locations) != 1 {
			t.Fatalf("source locate: %v / %+v", err, locations)
		}
		e.SourceBinding = NewLogSourceBindingForError(locations[0], e.Type, e.Message)
		if e.ObservedTypeLiteral() != e.Type {
			t.Fatalf("source spelling unproved: %+v", e)
		}
	})
}

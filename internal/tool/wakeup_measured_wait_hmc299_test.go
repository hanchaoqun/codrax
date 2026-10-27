package tool

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC299MeasuredWaitDoesNotChangeWakeRelationIdentity(t *testing.T) {
	_, result, _ := runtimeWakeFixture(t)
	ledger := types.ObservationLedger{Records: result.Observations}
	events := types.RuntimeWakeupDiagramEvents(ledger, nil)
	rows := RuntimeDiagramRelations(ledger, nil)
	if len(events) != 2 || len(rows) != 2 {
		t.Fatalf("public event fixture: events=%d rows=%d", len(events), len(rows))
	}
	legacy := append([]types.ObservationRecord(nil), result.Observations...)
	carriers := 0
	for i := range legacy {
		legacy[i].RichNotes = nil
		for _, note := range result.Observations[i].RichNotes {
			if strings.HasPrefix(note, types.TraceNoteKeyWakeupMeasuredWait+"=") {
				carriers++
				continue
			}
			legacy[i].RichNotes = append(legacy[i].RichNotes, note)
		}
	}
	if carriers != 2 {
		t.Fatalf("fixture did not exercise new optional carrier: %d", carriers)
	}
	for _, records := range [][]types.ObservationRecord{legacy, append(legacy, result.Observations...)} {
		gotEvents := types.RuntimeWakeupDiagramEvents(types.ObservationLedger{Records: records}, nil)
		gotRows := RuntimeDiagramRelations(types.ObservationLedger{Records: records}, nil)
		if len(gotEvents) != len(events) || !reflect.DeepEqual(rows, gotRows) {
			t.Fatal("optional measurement changed legacy/mixed event count or relation IDs")
		}
		for i := range events {
			if !bytes.Equal(events[i].Coordinates, gotEvents[i].Coordinates) || !bytes.Equal(events[i].Fingerprint, gotEvents[i].Fingerprint) {
				t.Fatal("optional measurement changed event credential bytes")
			}
		}
	}
	// The existing JSON event carrier is intentionally unchanged. Restoring
	// display data alone is not a new query, source, relationship or cause.
	before, _ := json.Marshal(events[0].Event)
	after, _ := json.Marshal(types.RuntimeWakeupDiagramEvents(types.ObservationLedger{Records: legacy}, nil)[0].Event)
	if !bytes.Equal(before, after) {
		t.Fatal("event wire shape changed")
	}
}

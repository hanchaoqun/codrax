package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Exact note strings are reusable data, not reusable observations. Every row
// still owns its source, window and values. The proposed directory is committed
// only together with its first actual referencing row under the same budget.
type samplingNoteDirectory map[string]string

func (d samplingNoteDirectory) propose(notes []string) ([]string, string, samplingNoteDirectory) {
	var refs []string
	var b strings.Builder
	pending := samplingNoteDirectory{}
	for _, note := range notes {
		id := d[note]
		if id == "" {
			id = pending[note]
		}
		if id == "" {
			id = fmt.Sprintf("sampling-note-%d", len(d)+len(pending)+1)
			value, _ := json.Marshal(note)
			fmt.Fprintf(&b, "native_note_ref=%q value=%s\n", id, value)
			pending[note] = id
		}
		refs = append(refs, id)
	}
	return refs, b.String(), pending
}

func (d samplingNoteDirectory) commit(pending samplingNoteDirectory) {
	for note, id := range pending {
		d[note] = id
	}
}

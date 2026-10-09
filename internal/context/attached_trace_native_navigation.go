package context

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Decode only the small navigation face, not full export diagnostics or raw
// capture receipts. This remains a disclosure, not an admission receipt.
type attachedTraceCoverageNavigation struct {
	Family         string   `json:"family"`
	Table          string   `json:"table"`
	ArtifactPath   string   `json:"artifact_path"`
	Found          bool     `json:"found"`
	RowsEmitted    int      `json:"rows_emitted"`
	Error          string   `json:"error"`
	ColumnsMissing []string `json:"columns_missing"`
}

// Full held manifest metadata, never the preview, supplies these candidates.
// Export counters are disclosures only: the chosen query rechecks the actual
// physical source and its own coverage/selector contract before publishing.
func attachedTraceNativeNavigation(metadata attachedTraceBundleMetadata) string {
	text, _ := attachedTraceNativeNavigationContent(metadata)
	return text
}

func attachedTraceNativeNavigationContent(metadata attachedTraceBundleMetadata) (string, map[tracequery.EventType]bool) {
	const maxRows, maxBytes = 8, 4096
	type row struct {
		tracequery.NativeIntervalNavigation
		DeclaredArtifact string `json:"declared_artifact,omitempty"`
	}
	var rows []string
	families := map[tracequery.EventType]bool{}
	omitted, used := 0, 0
	for _, lane := range [][]attachedTraceCoverageNavigation{metadata.TraceDBCoverage, metadata.TraceCoverage} {
		for _, c := range lane {
			if !c.Found || c.RowsEmitted <= 0 || c.Error != "" || len(c.ColumnsMissing) > 0 {
				continue
			}
			n, ok := tracequery.NativeIntervalNavigationForCoverage(c.Family, c.Table)
			if !ok {
				continue
			}
			b, err := json.Marshal(row{n, c.ArtifactPath})
			if err != nil {
				continue
			}
			text := strings.ReplaceAll(string(b), "`", `\u0060`)
			duplicate := false
			for _, prior := range rows {
				if text == prior {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			if len(rows) == maxRows || len(text) > maxBytes-used {
				omitted++
				continue
			}
			rows = append(rows, text)
			families[n.EventType] = true
			used += len(text)
		}
	}
	if len(rows) == 0 && omitted == 0 {
		return "", nil
	}
	return nativeIntervalNavigationPreamble("source export disclosures") + strings.Join(rows, "\n") +
		fmt.Sprintf("\nNavigation display: shown=%d omitted_disclosures=%d.\n", len(rows), omitted), families
}

func visibleTraceNativeNavigation(families map[tracequery.EventType]bool) string {
	var rows []string
	for _, n := range tracequery.NativeIntervalNavigations() {
		if families[n.EventType] {
			b, _ := json.Marshal(n)
			rows = append(rows, string(b))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	return nativeIntervalNavigationPreamble("parsed visible event families") + strings.Join(rows, "\n") + "\n"
}

func nativeIntervalNavigationPreamble(basis string) string {
	return "\nNative interval navigation (" + basis + "; navigation only):\n" +
		"Use the listed trace_query view on the same source and explicit window for interval overlap, including starts before the window. event_search discovers event time points; it does not replace interval accounting. Preserve explicit line/owner/pattern selectors; when a view cannot represent them, do not silently drop them. Source values, types, unknown durations and separate filter identities remain intact; names and source references alone prove no units, resource pairing, activity meaning, statistics or causality. Manifest candidates are not proof that their parser family is present; only the actual query can establish availability.\n"
}

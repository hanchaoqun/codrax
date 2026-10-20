package agent

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A table alias needs both the current native source/population and the entire
// producer table (rows, units, notes, omissions and ordering). Tree equivalence
// alone cannot certify flat symbol values or table completeness.
func (p *answerDocNativePresentationReceipt) prepareSamplingTableIdentities(ctx *types.AgentContext) {
	if p == nil || p.samplingTableIdentities != nil {
		return
	}
	p.prepareSamplingIdentities(ctx)
	p.samplingTableIdentities = map[samplingFactViewKey]string{}
	bridge := samplingFactPublicationBridge(ctx)
	for _, record := range answerDocObservationLedger(ctx).Records {
		key := traceEventPublicationRecordKey(record)
		population := p.samplingIdentities[nativePresentationQueryKey(record)]
		if population == "" || p.nativeSources[key] == "" {
			continue
		}
		pub, valid := bridge[key]
		if !valid {
			continue
		}
		for _, table := range pub.Tables {
			if !table.HasNativeSamplingFacts() {
				continue
			}
			copy := table
			copy.ObservationID = ""
			data, err := json.Marshal(copy)
			if err == nil {
				p.samplingTableIdentities[samplingFactViewKey{table.ObservationID, table.View}] = population + "\n" + string(data)
			}
		}
	}
}

func (p *answerDocNativePresentationReceipt) samplingTableKey(table types.RuntimeMeasurementTable) string {
	if p == nil || !table.HasNativeSamplingFacts() {
		return ""
	}
	return p.samplingTableIdentities[samplingFactViewKey{table.ObservationID, table.View}]
}

// Equivalent previews consume rows once; selectors remain individually
// listed. If the representative is truncated, aliases inherit no completeness.
func samplingDistinctPreviewRows(tables []types.RuntimeMeasurementTable, p *answerDocNativePresentationReceipt) []int {
	copy := append([]types.RuntimeMeasurementTable(nil), tables...)
	seen := map[string]bool{}
	for i, table := range tables {
		key := p.samplingTableKey(table)
		if key == "" {
			continue
		}
		if seen[key] {
			copy[i].Rows = nil
		}
		seen[key] = true
	}
	return runtimeMeasurementHandoffPreviewRows(copy)
}

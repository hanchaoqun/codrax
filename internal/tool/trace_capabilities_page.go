package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type traceCapabilitiesInput struct {
	View   string `json:"view,omitempty"`
	Detail bool   `json:"detail"`
	Cursor string `json:"cursor,omitempty"`
}

type traceCapabilitiesPayload struct {
	tracequery.CapabilityCatalog
	ToolEntryAliases map[string]string            `json:"tool_entry_aliases"`
	MetricPage       *traceCapabilitiesMetricPage `json:"metric_page,omitempty"`
}

// A page retains every view, input-format condition and catalog qualification.
// Only complete metric contracts move between pages; no field or string is
// clipped. A last page is still a partial selection when Offset is nonzero.
type traceCapabilitiesMetricPage struct {
	Offset               int                     `json:"offset"`
	Returned             int                     `json:"returned"`
	Total                int                     `json:"total"`
	AllMetricsInResponse bool                    `json:"all_metrics_in_response"`
	HasMore              bool                    `json:"has_more"`
	NextCall             *traceCapabilitiesInput `json:"next_call,omitempty"`
	Contract             string                  `json:"contract"`
}

const traceCapabilitiesPageContract = "This response preserves all view and input-format descriptors but contains only the indicated complete metric contracts. Metrics outside this page are not absent or unsupported. For exhaustive detailed documentation, follow next_call until has_more=false and retain every page; a final page alone is not the complete metric catalog. A selected view with detail=true is also available for focused documentation."

func marshalTraceCapabilitiesPage(catalog tracequery.CapabilityCatalog, aliases map[string]string, input traceCapabilitiesInput, maxBytes int) ([]byte, error) {
	full := traceCapabilitiesPayload{CapabilityCatalog: catalog, ToolEntryAliases: aliases}
	body, err := json.Marshal(full)
	if err != nil {
		return nil, err
	}
	if input.Cursor == "" && len(body) <= maxBytes {
		return body, nil
	}
	digest := sha256.Sum256(body)
	identity := hex.EncodeToString(digest[:])
	offset := 0
	if input.Cursor != "" {
		parts := strings.Split(input.Cursor, ".")
		if !input.Detail || len(parts) != 3 || parts[0] != "v1" || parts[1] != identity {
			return nil, fmt.Errorf("invalid or stale catalog cursor; repeat the original view/detail selection without cursor")
		}
		offset, err = strconv.Atoi(parts[2])
		if err != nil || offset <= 0 || offset >= len(catalog.Metrics) || strconv.Itoa(offset) != parts[2] {
			return nil, fmt.Errorf("invalid catalog metric offset; restart the selected catalog without cursor")
		}
	}
	if !input.Detail || len(catalog.Metrics) == 0 {
		return nil, fmt.Errorf("catalog descriptors exceed the bounded document size; select one view for a complete contract")
	}
	// Reserve pagination metadata before testing the bound. Even the last page
	// publishes its offset/total so it cannot masquerade as an entire catalog.
	var kept []byte
	for end := offset + 1; end <= len(catalog.Metrics); end++ {
		page := traceCapabilitiesMetricPage{Offset: offset, Returned: end - offset, Total: len(catalog.Metrics),
			AllMetricsInResponse: offset == 0 && end == len(catalog.Metrics), HasMore: end < len(catalog.Metrics), Contract: traceCapabilitiesPageContract}
		if page.HasMore {
			page.NextCall = &traceCapabilitiesInput{View: input.View, Detail: true, Cursor: "v1." + identity + "." + strconv.Itoa(end)}
		}
		payload := full
		payload.Metrics = catalog.Metrics[offset:end]
		payload.MetricPage = &page
		candidate, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		if len(candidate) > maxBytes {
			break
		}
		kept = candidate
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("one complete metric contract and its catalog descriptors exceed the bounded document size; select one view (no contract was clipped)")
	}
	return kept, nil
}

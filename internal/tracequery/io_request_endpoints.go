package tracequery

import (
	"strconv"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RequestEndpoints carries the original native pair through views which may
// clip its residence window or publish the issuer's independently measured
// blocked interval. It grants neither identity nor causal authority by itself.
func (io IOLatencySummary) RequestEndpoints() *types.TraceIORequestEndpoints {
	return &types.TraceIORequestEndpoints{
		Family: io.EndpointFamily, IssueLine: io.IssueLine, CompleteLine: io.CompleteLine,
		IssueTs: io.IssueTs, CompleteTs: io.CompleteTs, Device: io.Dev, Operation: io.Op,
		Sector: strconv.FormatInt(io.Sector, 10), Length: strconv.FormatInt(io.Len, 10),
	}
}

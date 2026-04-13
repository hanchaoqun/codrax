// Package risk evaluates the six-dimensional risk matrix for a
// RequestModel and derives a RunPolicy that the orchestrator is bound
// to for the rest of the run. It is one of the three components
// (compiler, risk, hdp) that the analyzer agent will call after the
// LLM produces a RequestModel in batch b5.
//
// The evaluator is deliberately conservative: it uses term-graph
// heuristics to nudge dimensions upward from a baseline of zero and
// leaves the final calibration to the LLM-supplied risk evidence
// that b5 will layer on top. When the LLM already provides a
// non-zero matrix, the heuristic only raises levels (never lowers),
// so the analyzer cannot silently downgrade an LLM-flagged concern.
package risk

import "github.com/hanchaoqun/codrax/internal/types"

// Evaluate returns a RiskMatrix derived from the request's term
// graph and a starting matrix (usually the one the LLM put on the
// IR, or a zero value). The result never reduces a level the caller
// already supplied — only monotonic upgrades are allowed so an LLM
// warning is not erased by heuristic oversight.
func Evaluate(rm types.RequestModel, start types.RiskMatrix) types.RiskMatrix {
	out := start
	hits := scanTerms(rm.TermGraph)
	bump := func(dim *types.RiskLevel, level int, reason string) {
		if level > dim.Level {
			dim.Level = level
		}
		if reason != "" {
			dim.Evidence = append(dim.Evidence, reason)
		}
	}
	for reason, dims := range hits {
		for _, d := range dims {
			switch d.dim {
			case "security":
				bump(&out.Security, d.level, reason)
			case "data_integrity":
				bump(&out.DataIntegrity, d.level, reason)
			case "compatibility":
				bump(&out.Compatibility, d.level, reason)
			case "performance":
				bump(&out.Performance, d.level, reason)
			case "ops":
				bump(&out.Ops, d.level, reason)
			case "compliance":
				bump(&out.Compliance, d.level, reason)
			}
		}
	}
	return out
}

// dimHit describes a heuristic bump: which dimension to raise and by
// how much. A single term can bump multiple dimensions (e.g. "auth"
// raises both security and compliance).
type dimHit struct {
	dim   string
	level int
}

// riskTerms is a small curated list of term surface substrings that
// map to dimension bumps. Keys are lowercased canonical-term IDs.
// This list is intentionally short — broader coverage should come
// from LLM-side risk reasoning, not from a hand-maintained table.
var riskTerms = map[string][]dimHit{
	// security
	"en:auth":       {{dim: "security", level: 3}, {dim: "compliance", level: 2}},
	"en:password":   {{dim: "security", level: 4}, {dim: "compliance", level: 3}},
	"en:token":      {{dim: "security", level: 3}},
	"en:secret":     {{dim: "security", level: 4}},
	"en:credential": {{dim: "security", level: 4}, {dim: "compliance", level: 3}},
	"en:session":    {{dim: "security", level: 2}},
	"en:crypto":     {{dim: "security", level: 3}},
	"en:sanitize":   {{dim: "security", level: 3}},
	"zh:认证":         {{dim: "security", level: 3}},
	"zh:密码":         {{dim: "security", level: 4}},
	"zh:令牌":         {{dim: "security", level: 3}},

	// data integrity
	"en:migration": {{dim: "data_integrity", level: 4}, {dim: "compatibility", level: 3}},
	"en:schema":    {{dim: "data_integrity", level: 3}, {dim: "compatibility", level: 3}},
	"en:database":  {{dim: "data_integrity", level: 2}},
	"en:backup":    {{dim: "data_integrity", level: 3}, {dim: "ops", level: 2}},
	"en:delete":    {{dim: "data_integrity", level: 3}},
	"en:drop":      {{dim: "data_integrity", level: 4}},
	"zh:迁移":        {{dim: "data_integrity", level: 4}, {dim: "compatibility", level: 3}},
	"zh:删除":        {{dim: "data_integrity", level: 3}},

	// compatibility
	"en:api":     {{dim: "compatibility", level: 2}},
	"en:version": {{dim: "compatibility", level: 2}},
	"en:upgrade": {{dim: "compatibility", level: 3}},
	"en:breaking": {{dim: "compatibility", level: 4}},

	// performance
	"en:performance": {{dim: "performance", level: 2}},
	"en:latency":     {{dim: "performance", level: 3}},
	"en:throughput":  {{dim: "performance", level: 3}},
	"en:bottleneck":  {{dim: "performance", level: 3}},
	"en:slow":        {{dim: "performance", level: 2}},
	"zh:性能":          {{dim: "performance", level: 2}},
	"zh:瓶颈":          {{dim: "performance", level: 3}},

	// ops
	"en:deploy":   {{dim: "ops", level: 2}},
	"en:rollback": {{dim: "ops", level: 3}},
	"en:monitor":  {{dim: "ops", level: 2}},

	// compliance
	"en:gdpr":     {{dim: "compliance", level: 5}, {dim: "security", level: 3}},
	"en:pii":      {{dim: "compliance", level: 4}, {dim: "security", level: 3}},
	"en:audit":    {{dim: "compliance", level: 2}},
}

// scanTerms returns a map of "reason text" → []dimHit for every
// canonical term in the request's term graph that matches a known
// risk keyword. The reason text is the surface form so the resulting
// RiskLevel.Evidence list is human-readable.
func scanTerms(tg types.TermGraph) map[string][]dimHit {
	out := make(map[string][]dimHit)
	for _, c := range tg.Canonical {
		if hits, ok := riskTerms[c.ID]; ok {
			out[c.Surface] = append(out[c.Surface], hits...)
		}
	}
	return out
}

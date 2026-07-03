package types

import (
	"sort"
	"strings"
)

// Anchor obligation lane (A1, 2026-07-03). Per-class fix for the
// "user-named / analyzer-confirmed anchors never consumed" eval failure
// class: qf_architecture's analyzer pre-scan named stage_binding.go yet the
// explorer anchored on a same-named decoy family and never read it; s1a's
// answer never surfaced the user-named gate.Run. The lane compiles the
// typed anchors the analyzer ALREADY verified (EntityProvenance resolved
// entities — the consumer that projection deliberately reserved —, the
// EvidencePlan's RequiredFiles, and user-named ExactTargets) into
// AnchorObligation rows and tracks consumption with PRECISE booleans
// (path ∈ read set; symbol verbatim on accepted evidence anchors).
//
// Red-line posture: anchor RELEVANCE is a noisy judgment, so every
// consumer of this lane is SOFT — the explore checkpoint reminder and the
// completion-gate advisory note. Nothing here may deny completion, gate
// evidence, or rewrite answers.

type AnchorObligationKind string

const (
	AnchorObligationFile   AnchorObligationKind = "file"
	AnchorObligationScope  AnchorObligationKind = "scope"
	AnchorObligationSymbol AnchorObligationKind = "symbol"
)

type AnchorObligationOrigin string

const (
	AnchorOriginExactTarget  AnchorObligationOrigin = "user_exact_target"
	AnchorOriginEntity       AnchorObligationOrigin = "analyzer_entity"
	AnchorOriginRequiredFile AnchorObligationOrigin = "analyzer_required_file"
)

type AnchorObligation struct {
	Token  string                 `json:"token"`
	Kind   AnchorObligationKind   `json:"kind"`
	Origin AnchorObligationOrigin `json:"origin"`
}

// anchorObligationLimit bounds the compiled set: obligations are reminders,
// not a work queue — a handful of the strongest anchors beats a wall.
const anchorObligationLimit = 8

// AnchorTokenShaped reports whether a token is code-identifier or
// path-shaped (structural character-class check — never prose semantics).
func AnchorTokenShaped(token string) bool {
	if token == "" || len(token) < 3 {
		return false
	}
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '.', r == '/', r == ':':
		default:
			return false
		}
	}
	return true
}

// CompileAnchorObligations derives the typed anchor set from the analyzer's
// own verified carriers. Deterministic and bounded; origin priority is
// user exact targets, then resolved entities, then required files.
func CompileAnchorObligations(ir *AnalysisIR) []AnchorObligation {
	if ir == nil {
		return nil
	}
	var out []AnchorObligation
	seen := map[string]bool{}
	add := func(token string, kind AnchorObligationKind, origin AnchorObligationOrigin) {
		token = strings.TrimSpace(token)
		if !AnchorTokenShaped(token) {
			return
		}
		key := strings.ToLower(token)
		if seen[key] || len(out) >= anchorObligationLimit {
			return
		}
		seen[key] = true
		out = append(out, AnchorObligation{Token: token, Kind: kind, Origin: origin})
	}
	rm := ir.RequestModel
	for _, target := range rm.AnalyzerHints.ExactTargets {
		kind := AnchorObligationSymbol
		if strings.ContainsAny(target, "/") || strings.Contains(target, ".go") {
			kind = AnchorObligationFile
		}
		add(target, kind, AnchorOriginExactTarget)
	}
	for _, prov := range rm.AnalyzerHints.EntityProvenance {
		if !prov.Resolved {
			continue
		}
		switch prov.Resolution {
		case EntityResolutionFile:
			add(prov.Surface, AnchorObligationFile, AnchorOriginEntity)
		case EntityResolutionScope:
			add(prov.Surface, AnchorObligationScope, AnchorOriginEntity)
		case EntityResolutionSymbol:
			add(prov.Surface, AnchorObligationSymbol, AnchorOriginEntity)
		}
	}
	for _, file := range ir.EvidencePlan.RequiredFiles {
		add(file, AnchorObligationFile, AnchorOriginRequiredFile)
	}
	return out
}

// UnconsumedAnchorObligations filters to obligations with NO precise
// consumption proof yet: file/scope anchors resolve against the read set
// (alias-aware), symbol anchors against accepted evidence anchor fields
// verbatim. Order is stable (compile order).
func UnconsumedAnchorObligations(obligations []AnchorObligation, closure *EvidenceClosure, evidence []EvidenceItem) []AnchorObligation {
	if len(obligations) == 0 {
		return nil
	}
	symbolSeen := map[string]bool{}
	for _, item := range evidence {
		for _, field := range []string{item.AnchorSymbol, item.Subject, item.Object} {
			field = strings.TrimSpace(field)
			if field != "" {
				symbolSeen[field] = true
			}
		}
	}
	var out []AnchorObligation
	for _, ob := range obligations {
		switch ob.Kind {
		case AnchorObligationFile:
			if closure.HasReadPath(ob.Token) {
				continue
			}
		case AnchorObligationScope:
			if closure.HasReadPathUnder(ob.Token) {
				continue
			}
		case AnchorObligationSymbol:
			if symbolSeen[ob.Token] {
				continue
			}
			// A dotted user spelling (pkg.Symbol) counts as consumed when
			// the bare trailing segment is anchored — verbatim segment
			// equality, not fuzzy matching.
			if dot := strings.LastIndex(ob.Token, "."); dot > 0 && dot < len(ob.Token)-1 {
				if symbolSeen[ob.Token[dot+1:]] {
					continue
				}
			}
		}
		out = append(out, ob)
	}
	return out
}

// SummarizeAnchorObligations renders "token(kind)" listing sorted by
// compile order — display text for soft reminders only.
func SummarizeAnchorObligations(obligations []AnchorObligation) string {
	if len(obligations) == 0 {
		return ""
	}
	parts := make([]string, 0, len(obligations))
	for _, ob := range obligations {
		parts = append(parts, ob.Token+"("+string(ob.Kind)+")")
	}
	sort.SliceStable(parts, func(i, j int) bool { return false }) // keep compile order
	return strings.Join(parts, ", ")
}

package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type structuredRelationAuthorityDemand struct {
	Name      string
	Files     []string
	Targets   []structuredRelationAuthorityTarget
	Subject   string
	Rationale string
	Origin    string
}

type structuredRelationAuthorityTarget struct {
	Candidate types.TypedRelationCandidate
}

type structuredRelationAuthorityProvider interface {
	Demand(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) (structuredRelationAuthorityDemand, bool)
}

func structuredRelationAuthorityPreCompleteDowngrade(ctx *types.BusContext, closure *types.EvidenceClosure, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) string {
	if ctx == nil || closure == nil {
		return ""
	}
	demands := structuredRelationAuthorityDemands(ctx, facts, evidence)
	if len(demands) == 0 {
		return ""
	}
	for _, demand := range demands {
		files := append([]string(nil), demand.Files...)
		for _, target := range demand.Targets {
			files = append(files, target.Candidate.Member.File)
		}
		files = dedupStringsPreserveOrder(files)
		if len(files) == 0 {
			continue
		}
		var readButUnemitted []string
		for _, target := range demand.Targets {
			candidate := target.Candidate
			file := normalizeStructuredRelationAuthorityPath(candidate.Member.File)
			if file == "" || structuredRelationAuthorityEvidenceHasTypedCandidate(evidence, candidate) {
				continue
			}
			if closure.HasReadLine(file, candidate.Member.Line) {
				readButUnemitted = append(readButUnemitted, file)
				continue
			}
			markStructuredRelationAuthorityFilesScanned(closure, []string{file})
			directive := types.RepairDirective{
				Kind:      types.RepairReadFile,
				Files:     []string{file},
				Subject:   demand.Subject,
				Rationale: demand.Rationale,
				Origin:    demand.Origin,
				Stage:     string(types.StageExplore),
			}
			if candidate.Member.Line > 0 {
				directive.LineRanges = []types.LineRange{{Start: candidate.Member.Line, End: candidate.Member.Line}}
			}
			closure.AddRepair(directive)
		}
		for _, file := range demand.Files {
			file = normalizeStructuredRelationAuthorityPath(file)
			if file == "" || structuredRelationAuthorityEvidenceHasSource(evidence, file) {
				continue
			}
			if closure.HasRead(file) {
				readButUnemitted = append(readButUnemitted, file)
				continue
			}
			markStructuredRelationAuthorityFilesScanned(closure, []string{file})
			closure.AddRepair(types.RepairDirective{
				Kind:      types.RepairReadFile,
				Files:     []string{file},
				Subject:   demand.Subject,
				Rationale: demand.Rationale,
				Origin:    demand.Origin,
				Stage:     string(types.StageExplore),
			})
		}
		if len(readButUnemitted) == 0 {
			continue
		}
		readButUnemitted = dedupStringsPreserveOrder(readButUnemitted)
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairEmitEvidence,
			Files:     readButUnemitted,
			Subject:   demand.Subject,
			Rationale: demand.Rationale,
			Origin:    demand.Origin,
			Stage:     string(types.StageExplore),
		})
		var b strings.Builder
		b.WriteString(EmitInvestigationCompleteDowngradePrefix)
		b.WriteString(" — structured relation authority evidence is not materialized.\n\n")
		b.WriteString("The model emitted structured relation members that reference both sides of a machine-known relation, and the authority source has already been read but not emitted as grounded evidence.\n\n")
		b.WriteString("## Evidence Materialization\n")
		for _, file := range readButUnemitted {
			fmt.Fprintf(&b, "  - %s — %s\n", file, demand.Rationale)
		}
		b.WriteString("\nEmit grounded evidence from the already-read authority source, then re-call emit_investigation_complete. If the relation was only background, drop that aggregate member_set instead of forcing it into the final answer.")
		return b.String()
	}
	return ""
}

func structuredRelationAuthorityDemands(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) []structuredRelationAuthorityDemand {
	providers := structuredRelationAuthorityProviders()
	var out []structuredRelationAuthorityDemand
	for _, provider := range providers {
		if demand, ok := provider.Demand(ctx, facts, evidence); ok {
			out = append(out, demand)
		}
	}
	return out
}

// structuredRelationAuthorityProviders is deliberately a short explicit
// registry. Each blocking provider owns an exact typed trigger, carrier,
// repair path, and no-trigger tests; repository-specific conventions and
// request/model prose are not providers.
func structuredRelationAuthorityProviders() []structuredRelationAuthorityProvider {
	return []structuredRelationAuthorityProvider{
		typedImplementerDiagramAuthorityProvider{},
	}
}

// typedImplementerDiagramAuthorityProvider closes the graph-roster to
// finalizer-evidence handoff for required implementation diagrams. The exact
// graph may prove the principal roster, but it is not itself a citation: each
// included member's declaration line must be read so Explorer's existing
// cross-language relation producer can mint a citable typed edge.
type typedImplementerDiagramAuthorityProvider struct{}

func (typedImplementerDiagramAuthorityProvider) Demand(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) (structuredRelationAuthorityDemand, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return structuredRelationAuthorityDemand{}, false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.PredicateAxis != types.AxisImplement ||
		rm.DiagramHint == nil || !rm.DiagramHint.Required || rm.DiagramHint.Kind == types.DiagramNone {
		return structuredRelationAuthorityDemand{}, false
	}
	candidates := relationCandidatesForRequestAllSourceRoles(ctx, rm)
	if len(candidates) == 0 {
		return structuredRelationAuthorityDemand{}, false
	}
	var targets []structuredRelationAuthorityTarget
	for _, candidate := range candidates {
		candidate.Member.File = normalizeStructuredRelationAuthorityPath(candidate.Member.File)
		if candidate.Relation != types.TypedRelationImplements ||
			!candidate.CoverageGateEligible() ||
			candidate.Member.File == "" || candidate.Member.Line <= 0 ||
			!relationSourceInRequestedScope(candidate.Member.File, rm) ||
			!structuredRelationAuthoritySourceExists(ctx, candidate.Member.File) {
			continue
		}
		included, excluded := relationPrincipalMemberSetCandidateCoverage(facts, &rm, candidate)
		if !included || excluded || structuredRelationAuthorityEvidenceHasTypedCandidate(evidence, candidate) {
			continue
		}
		targets = append(targets, structuredRelationAuthorityTarget{Candidate: candidate})
	}
	if len(targets) == 0 {
		return structuredRelationAuthorityDemand{}, false
	}
	return structuredRelationAuthorityDemand{
		Name:      "typed_implementer_diagram_authority",
		Targets:   targets,
		Subject:   "typed implementer relation evidence for the selected principal diagram roster",
		Rationale: "read each included implementer's exact declaration line so the cross-language RepoMap relation producer can materialize a citable same-direction implementation edge",
		Origin:    "pre_complete.relation_authority.typed_implementer_diagram",
	}, true
}

func structuredRelationAuthoritySurfaces(facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) []string {
	var out []string
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateMemberSet {
			continue
		}
		out = append(out, fact.Members...)
	}
	for _, item := range evidence {
		out = append(out,
			item.Subject,
			item.Object,
			item.AnchorSymbol,
		)
	}
	return out
}

func structuredRelationAuthoritySurfaceAliases(surface string) []string {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return nil
	}
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	add(surface)
	for _, candidate := range types.AnswerAggregateMemberDisplayCandidates(surface) {
		add(candidate)
	}
	if base, qualifier, ok := types.AnswerAggregateDecoratedLabelParts(surface); ok {
		add(base)
		add(strings.Trim(qualifier, "`'\"“”‘’ "))
	}
	if left, right, ok := types.AnswerAggregateMemberRelationParts(surface); ok {
		add(left)
		add(right)
	}
	if label, _, ok := types.ParseAnswerSupportRefMemberLocation(surface); ok {
		add(label)
	}
	return dedupStringsPreserveOrder(out)
}

func structuredRelationAuthorityKey(surface string) string {
	surface = strings.TrimSpace(surface)
	surface = strings.Trim(surface, "`'\"“”‘’ ")
	if surface == "" {
		return ""
	}
	surface = strings.Join(strings.Fields(surface), " ")
	return strings.ToLower(surface)
}

func structuredRelationAuthorityEvidenceHasSource(evidence []types.EvidenceItem, source string) bool {
	source = normalizeStructuredRelationAuthorityPath(source)
	if source == "" {
		return false
	}
	for _, item := range evidence {
		if normalizeStructuredRelationAuthorityPath(item.Source) == source && item.LineStart > 0 {
			return true
		}
	}
	return false
}

func structuredRelationAuthorityEvidenceHasTypedCandidate(evidence []types.EvidenceItem, candidate types.TypedRelationCandidate) bool {
	file := normalizeStructuredRelationAuthorityPath(candidate.Member.File)
	member := structuredRelationAuthorityKey(candidate.Member.Name)
	source := structuredRelationAuthorityKey(candidate.SourceName)
	if file == "" || member == "" || source == "" || candidate.Relation != types.TypedRelationImplements {
		return false
	}
	for _, item := range evidence {
		if !types.IsRepoMapTypeRelationEvidence(item) || !item.IsCitable() ||
			normalizeStructuredRelationAuthorityPath(item.Source) != file ||
			structuredRelationAuthorityKey(item.Subject) != member ||
			structuredRelationAuthorityKey(item.Object) != source {
			continue
		}
		if candidate.Member.Line > 0 && (item.LineStart > candidate.Member.Line || item.LineEnd < candidate.Member.Line) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.Predicate)) {
		case "implements", "implementation":
			return true
		}
	}
	return false
}

func structuredRelationAuthoritySourceExists(ctx *types.BusContext, source string) bool {
	source = normalizeStructuredRelationAuthorityPath(source)
	if source == "" || ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return false
	}
	path := filepath.Join(ctx.RepoRoot, filepath.FromSlash(source))
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

func normalizeStructuredRelationAuthorityPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	return path
}

func markStructuredRelationAuthorityFilesScanned(closure *types.EvidenceClosure, files []string) {
	if closure == nil || len(files) == 0 {
		return
	}
	scanned := closure.ScannedSet()
	if len(scanned) == 0 {
		return
	}
	changed := false
	for _, file := range files {
		file = normalizeStructuredRelationAuthorityPath(file)
		if file == "" || scanned[file] {
			continue
		}
		scanned[file] = true
		changed = true
	}
	if changed {
		closure.SetScannedSet(scanned)
	}
}

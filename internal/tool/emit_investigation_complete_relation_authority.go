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
	Subject   string
	Rationale string
	Origin    string
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
		if len(demand.Files) == 0 {
			continue
		}
		var readButUnemitted []string
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

// structuredRelationAuthorityProviders intentionally returns no built-in
// repository-specific providers. This package supplies the handoff framework
// only; a blocking provider must be added explicitly with its own exact source
// of truth, trigger carriers, local repair path, and no-trigger tests.
func structuredRelationAuthorityProviders() []structuredRelationAuthorityProvider {
	return nil
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

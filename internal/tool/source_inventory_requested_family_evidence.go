package tool

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryRequestedUniverseFamilyAddFact(family *sourceInventoryRequestedUniverseFamily, fact types.AnswerAggregateFact, evidence sourceInventoryAcceptedEvidenceFamily) {
	for _, dim := range fact.Dimensions {
		sourceInventoryRequestedUniverseFamilyAddPath(family, dim.Value)
	}
	for _, surface := range append(append([]string(nil), fact.Members...), fact.SupportRefs...) {
		sourceInventoryRequestedUniverseFamilyAddSurface(family, surface)
		for _, ref := range evidence[sourceInventoryAggregateMemberKey(surface)] {
			sourceInventoryRequestedUniverseFamilyAddAcceptedEvidence(family, ref)
		}
	}
}

type sourceInventoryAcceptedEvidenceFamily map[string][]types.AcceptedEvidenceRef

func sourceInventoryAcceptedEvidenceFamilyIndex(ctx *types.BusContext) sourceInventoryAcceptedEvidenceFamily {
	out := sourceInventoryAcceptedEvidenceFamily{}
	if ctx == nil || ctx.Mutable == nil {
		return out
	}
	for _, ref := range ctx.Mutable.EvidenceClosure().AcceptedEvidenceRefs() {
		for _, raw := range []string{ref.Subject, ref.AnchorSymbol, ref.OwnerSymbol} {
			key := sourceInventoryFamilyLabelKey(raw)
			if key == "" {
				continue
			}
			out[key] = append(out[key], ref)
		}
	}
	return out
}

func sourceInventoryRequestedUniverseFamilyAddAcceptedEvidence(family *sourceInventoryRequestedUniverseFamily, ref types.AcceptedEvidenceRef) {
	sourceInventoryRequestedUniverseFamilyAddPath(family, ref.Source)
	if ref.SourcePathRole != "" && ref.SourcePathRole != types.SourcePathRoleUnknown {
		family.classes[ref.SourcePathRole] = true
	}
}

func sourceInventoryRequestedUniverseFamilyAddPath(family *sourceInventoryRequestedUniverseFamily, raw string) {
	if family == nil {
		return
	}
	path := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	if path == "" || !types.IsCodeOrConfigPathExtension(filepath.Ext(path)) {
		return
	}
	for _, lang := range sourceInventoryPathLanguages(path) {
		family.languages[lang] = true
	}
	if class := types.ClassifySourcePathRole(path); class != types.SourcePathRoleUnknown {
		family.classes[class] = true
	}
}

func sourceInventoryAggregateMemberKey(surface string) string {
	if label, _, ok := types.ParseAnswerSupportRefMemberLocation(surface); ok {
		return sourceInventoryFamilyLabelKey(label)
	}
	return sourceInventoryFamilyLabelKey(surface)
}

func sourceInventoryFamilyLabelKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, sep := range []string{" (", " @ ", " | ", "\t", ": "} {
		if idx := strings.Index(raw, sep); idx > 0 {
			raw = strings.TrimSpace(raw[:idx])
			break
		}
	}
	raw = strings.Trim(raw, "`'\" ")
	return strings.ToLower(raw)
}

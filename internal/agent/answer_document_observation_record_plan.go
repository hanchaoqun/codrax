package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The source coordinate is the only conditional field of a generic record.
// Build its independent facts once; a later display decision may substitute
// an exact, actually published source alias, never its span, claim or notes.
type answerDocObservationRecordPlan struct {
	record               types.ObservationPromptRecord
	prefix, source, tail string
}

func planAnswerDocObservationRecord(record types.ObservationPromptRecord, windowRole, queryScope string) answerDocObservationRecordPlan {
	plan := answerDocObservationRecordPlan{record: record, source: record.Source,
		prefix: fmt.Sprintf("- `%s`: origin=`%s`; producer=`%s`; source=`", record.ID, record.Origin, record.Producer)}
	var b strings.Builder
	fmt.Fprintf(&b, "`; role=`%s`; policy=`%s`", record.Role, record.GroundingPolicy)
	if record.ProvenanceLane.IsValid() {
		fmt.Fprintf(&b, "; lane=`%s`", record.ProvenanceLane)
	}
	if record.ClaimAuthority.IsValid() {
		fmt.Fprintf(&b, "; claim_authority=`%s`", record.ClaimAuthority)
	}
	if span := strings.TrimSpace(record.Span); span != "" {
		fmt.Fprintf(&b, "; span=%s", span)
		if windowRole != "" {
			fmt.Fprintf(&b, "; span_role=%s (not a continuous state interval)", windowRole)
		}
	}
	if queryScope != "" {
		fmt.Fprintf(&b, "; query_scope=%q", queryScope)
	}
	if claim := strings.TrimSpace(record.Claim); claim != "" {
		fmt.Fprintf(&b, "; claim=%q", claim)
	}
	if record.Negative {
		b.WriteString("; negative=true")
	}
	if record.ResultCount != nil {
		fmt.Fprintf(&b, "; result_count=%d", *record.ResultCount)
	}
	if value := strings.TrimSpace(record.Value); value != "" {
		fmt.Fprintf(&b, "; value=%q", value)
	}
	if summary := strings.TrimSpace(record.Summary); summary != "" {
		fmt.Fprintf(&b, "; summary=%q", summary)
	}
	if excerpt := strings.TrimSpace(record.Excerpt); excerpt != "" {
		fmt.Fprintf(&b, "; excerpt=%q", excerpt)
	}
	if excerpt := types.FormatObservationPromptSourceExcerpt(record.SourceExcerpt); excerpt != "" {
		fmt.Fprintf(&b, "; %s", excerpt)
	}
	if len(record.Notes) > 0 {
		if notes := renderAnswerDocObservationNotes(record.Notes, len(record.Notes)); notes != "" {
			fmt.Fprintf(&b, "; notes=%s", notes)
		}
	}
	if modelNotes := types.FormatObservationModelNotes(record.ModelNotes); modelNotes != "" {
		fmt.Fprintf(&b, "; %s", modelNotes)
	}
	if record.SupportRefCount > 0 {
		fmt.Fprintf(&b, "; support_refs=%d", record.SupportRefCount)
	}
	b.WriteByte('\n')
	plan.tail = b.String()
	return plan
}

func (p answerDocObservationRecordPlan) bytes(source string) int {
	return len(p.prefix) + len(source) + len(p.tail)
}

func (p answerDocObservationRecordPlan) write(b *strings.Builder, source string) {
	b.WriteString(p.prefix)
	b.WriteString(source)
	b.WriteString(p.tail)
}

type answerDocGenericPublicationUnit struct {
	record            answerDocObservationRecordPlan
	sourceAlias       string
	mirror            string
	inventoryMetadata bool
}

type answerDocGenericPublicationPlan struct {
	aliasRoster string
	units       []answerDocGenericPublicationUnit
}

func (g *answerDocNativeDeferredGroups) planGeneric(principal types.ObservationLedger, p *answerDocNativePresentationReceipt) answerDocGenericPublicationPlan {
	records := make([]types.ObservationPromptRecord, len(g.generic))
	for i, unit := range g.generic {
		records[i] = unit.record
	}
	aliases, roster := traceEventInventoryPromptSourceAliases(records, traceEventInventorySourceReferences(principal.Records, g.publishedMetadata))
	plan := answerDocGenericPublicationPlan{aliasRoster: roster}
	mirrors := p.samplingMirrorReferences(principal)
	for _, unit := range g.generic {
		planned := answerDocGenericPublicationUnit{record: unit, sourceAlias: aliases[unit.record.ID],
			inventoryMetadata: g.publishedMetadata[unit.record.ID] && unit.record.Claim == types.TraceEventSearchInventoryPredicate}
		if reference := mirrors[unit.record.ID]; reference != "" {
			var navigation strings.Builder
			if p.writePresentationNavigation(&navigation, reference) {
				planned.mirror = navigation.String()
			}
		}
		plan.units = append(plan.units, planned)
	}
	return plan
}

func (p answerDocGenericPublicationPlan) bytes(withInventory bool) int {
	n := 0
	if withInventory {
		n += len(p.aliasRoster)
	}
	for _, unit := range p.units {
		switch {
		case unit.mirror != "":
			n += len(unit.mirror)
		case withInventory && unit.inventoryMetadata:
			continue
		case withInventory && unit.sourceAlias != "":
			n += unit.record.bytes(unit.sourceAlias)
		default:
			n += unit.record.bytes(unit.record.source)
		}
	}
	return n
}

func (p answerDocGenericPublicationPlan) render(withInventory bool) string {
	var out strings.Builder
	if withInventory {
		out.WriteString(p.aliasRoster)
	}
	for _, unit := range p.units {
		switch {
		case unit.mirror != "":
			out.WriteString(unit.mirror)
		case withInventory && unit.inventoryMetadata:
			continue
		case withInventory && unit.sourceAlias != "":
			unit.record.write(&out, unit.sourceAlias)
		default:
			unit.record.write(&out, unit.record.source)
		}
	}
	return out.String()
}

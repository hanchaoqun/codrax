package types

import (
	"strconv"
	"strings"
)

const (
	ToolHandoffCarrierVersion = 1

	toolHandoffMaxCarriers         = 64
	toolHandoffMaxAcceptedEvidence = 64
	toolHandoffMaxObservationRefs  = 64
	toolHandoffMaxFields           = 48
	toolHandoffMaxTextLen          = 240
)

// ToolJSONSurfaceDescriptor is the compact, schema-bound retry surface exposed
// by a tool failure. It is derived from typed repair metadata, not from the
// rendered summary text, so retry consumers can render valid JSON guidance
// without reverse-engineering prose.
type ToolJSONSurfaceDescriptor struct {
	ToolName          string              `json:"tool_name,omitempty"`
	ReasonCode        string              `json:"reason_code,omitempty"`
	FailingFieldPaths []string            `json:"failing_field_paths,omitempty"`
	AcceptedEnums     map[string][]string `json:"accepted_enums,omitempty"`
}

// AcceptedEvidenceRef is the bounded evidence identity that survives stage
// handoff. It intentionally carries coordinates and owner/anchor identifiers
// only; downstream hard gates still consume EvidenceItem / ObservationRecord.
type AcceptedEvidenceRef struct {
	ID              string          `json:"id,omitempty"`
	Kind            EvidenceKind    `json:"kind,omitempty"`
	Scope           EvidenceScope   `json:"scope,omitempty"`
	Source          string          `json:"source,omitempty"`
	LineStart       int             `json:"line_start,omitempty"`
	LineEnd         int             `json:"line_end,omitempty"`
	Subject         string          `json:"subject,omitempty"`
	OwnerSymbol     string          `json:"owner_symbol,omitempty"`
	AnchorSymbol    string          `json:"anchor_symbol,omitempty"`
	SourcePathRole  SourcePathRole  `json:"source_path_role,omitempty"`
	GroundingStatus GroundingStatus `json:"grounding_status,omitempty"`
}

// ToolObservationRef is the ToolResult observation companion used by handoff
// consumers that only need identity, source coordinates, and producer lane.
type ToolObservationRef struct {
	ID              string               `json:"id,omitempty"`
	Origin          AnswerEvidenceOrigin `json:"origin,omitempty"`
	Producer        string               `json:"producer,omitempty"`
	Source          string               `json:"source,omitempty"`
	LineStart       int                  `json:"line_start,omitempty"`
	LineEnd         int                  `json:"line_end,omitempty"`
	ClaimKey        string               `json:"claim_key,omitempty"`
	Subject         string               `json:"subject,omitempty"`
	GroundingPolicy ClaimGroundingPolicy `json:"grounding_policy,omitempty"`
	GroundingStatus GroundingStatus      `json:"grounding_status,omitempty"`
}

// ToolHandoffCarrier is the lane-neutral cross-stage carrier for structured
// tool repair and accepted-evidence handoff. It is a projection of typed
// artifacts already produced by tools; it must not be populated by parsing
// user prose, model rationale, or summary text.
type ToolHandoffCarrier struct {
	Version          int                        `json:"version"`
	ToolName         string                     `json:"tool_name,omitempty"`
	ReasonCode       string                     `json:"reason_code,omitempty"`
	RepairCode       string                     `json:"repair_code,omitempty"`
	Repair           *ToolRepair                `json:"repair,omitempty"`
	PlanRepairPack   *PlanRepairPack            `json:"plan_repair_pack,omitempty"`
	SupportedJSON    *ToolJSONSurfaceDescriptor `json:"supported_json,omitempty"`
	AcceptedEvidence []AcceptedEvidenceRef      `json:"accepted_evidence,omitempty"`
	ObservationRefs  []ToolObservationRef       `json:"observation_refs,omitempty"`
}

func AttachToolHandoffCarrier(result ToolResult) ToolResult {
	if result.Handoff != nil {
		carrier := NormalizeToolHandoffCarrier(*result.Handoff)
		if !carrier.Empty() {
			result.Handoff = &carrier
		} else {
			result.Handoff = nil
		}
		return result
	}
	carrier, ok := ToolHandoffCarrierFromToolResult(result)
	if !ok {
		return result
	}
	result.Handoff = &carrier
	return result
}

func ToolHandoffCarrierFromToolResult(result ToolResult) (ToolHandoffCarrier, bool) {
	carrier := ToolHandoffCarrier{
		Version:  ToolHandoffCarrierVersion,
		ToolName: trimToolHandoffText(result.ToolName),
	}
	if result.Repair != nil {
		repair := cloneToolRepairPtr(result.Repair)
		carrier.Repair = repair
		carrier.RepairCode = trimToolHandoffText(repair.Code)
		carrier.ReasonCode = toolRepairReasonCode(repair)
		if pack, ok := PlanRepairPackFromToolRepair(repair); ok {
			pack = NormalizePlanRepairPack(pack)
			carrier.PlanRepairPack = &pack
			if carrier.ReasonCode == "" {
				carrier.ReasonCode = pack.ReasonCode
			}
			surface := toolJSONSurfaceFromPlanRepairPack(pack)
			if !surface.Empty() {
				carrier.SupportedJSON = &surface
			}
		}
	}
	for _, obs := range result.Observations {
		if ref, ok := ToolObservationRefFromObservationRecord(obs); ok {
			carrier.ObservationRefs = append(carrier.ObservationRefs, ref)
		}
	}
	if carrier.ReasonCode == "" {
		switch {
		case carrier.RepairCode != "":
			carrier.ReasonCode = carrier.RepairCode
		case len(carrier.ObservationRefs) > 0:
			carrier.ReasonCode = "tool_observation_handoff"
		}
	}
	carrier = NormalizeToolHandoffCarrier(carrier)
	return carrier, !carrier.Empty()
}

func ToolHandoffCarriersFromTurnAInputs(results []ToolResult, evidence []EvidenceItem, explicit []ToolHandoffCarrier) []ToolHandoffCarrier {
	carriers := make([]ToolHandoffCarrier, 0, len(results)+len(explicit)+1)
	for _, carrier := range explicit {
		carriers = append(carriers, carrier)
	}
	for _, result := range results {
		result = AttachToolHandoffCarrier(result)
		if result.Handoff != nil {
			carriers = append(carriers, *result.Handoff)
		}
	}
	var evidenceRefs []AcceptedEvidenceRef
	for _, item := range evidence {
		if ref, ok := AcceptedEvidenceRefFromEvidenceItem(item); ok {
			evidenceRefs = append(evidenceRefs, ref)
		}
	}
	if len(evidenceRefs) > 0 {
		carriers = append(carriers, ToolHandoffCarrier{
			Version:          ToolHandoffCarrierVersion,
			ToolName:         "emit_evidence",
			ReasonCode:       "accepted_evidence_handoff",
			AcceptedEvidence: evidenceRefs,
		})
	}
	return NormalizeToolHandoffCarriers(carriers)
}

func NormalizeToolHandoffCarriers(in []ToolHandoffCarrier) []ToolHandoffCarrier {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]int{}
	out := make([]ToolHandoffCarrier, 0, len(in))
	for _, carrier := range in {
		carrier = NormalizeToolHandoffCarrier(carrier)
		if carrier.Empty() {
			continue
		}
		key := toolHandoffCarrierKey(carrier)
		if idx, ok := seen[key]; ok {
			out[idx] = mergeToolHandoffCarrier(out[idx], carrier)
			continue
		}
		seen[key] = len(out)
		out = append(out, carrier)
	}
	if len(out) > toolHandoffMaxCarriers {
		out = out[len(out)-toolHandoffMaxCarriers:]
	}
	return out
}

func NormalizeToolHandoffCarrier(in ToolHandoffCarrier) ToolHandoffCarrier {
	if in.Version <= 0 {
		in.Version = ToolHandoffCarrierVersion
	}
	in.ToolName = trimToolHandoffText(in.ToolName)
	in.ReasonCode = trimToolHandoffText(in.ReasonCode)
	in.RepairCode = trimToolHandoffText(in.RepairCode)
	in.Repair = normalizeToolRepairPtr(in.Repair)
	if in.PlanRepairPack != nil {
		pack := NormalizePlanRepairPack(*in.PlanRepairPack)
		if pack.ReasonCode != "" {
			in.PlanRepairPack = &pack
			if in.ReasonCode == "" {
				in.ReasonCode = pack.ReasonCode
			}
		} else {
			in.PlanRepairPack = nil
		}
	}
	if in.SupportedJSON != nil {
		surface := NormalizeToolJSONSurfaceDescriptor(*in.SupportedJSON)
		if surface.Empty() {
			in.SupportedJSON = nil
		} else {
			in.SupportedJSON = &surface
		}
	}
	in.AcceptedEvidence = normalizeAcceptedEvidenceRefs(in.AcceptedEvidence)
	in.ObservationRefs = normalizeToolObservationRefs(in.ObservationRefs)
	if in.ReasonCode == "" {
		switch {
		case in.RepairCode != "":
			in.ReasonCode = in.RepairCode
		case len(in.AcceptedEvidence) > 0:
			in.ReasonCode = "accepted_evidence_handoff"
		case len(in.ObservationRefs) > 0:
			in.ReasonCode = "tool_observation_handoff"
		}
	}
	return in
}

func (c ToolHandoffCarrier) Empty() bool {
	return c.ToolName == "" &&
		c.ReasonCode == "" &&
		c.RepairCode == "" &&
		c.Repair == nil &&
		c.PlanRepairPack == nil &&
		c.SupportedJSON == nil &&
		len(c.AcceptedEvidence) == 0 &&
		len(c.ObservationRefs) == 0
}

func NormalizeToolJSONSurfaceDescriptor(in ToolJSONSurfaceDescriptor) ToolJSONSurfaceDescriptor {
	in.ToolName = trimToolHandoffText(in.ToolName)
	in.ReasonCode = trimToolHandoffText(in.ReasonCode)
	in.FailingFieldPaths = normalizeToolHandoffStrings(in.FailingFieldPaths, toolHandoffMaxFields, 160)
	for k, vals := range in.AcceptedEnums {
		key := trimToolHandoffText(k)
		normalized := normalizeToolHandoffStrings(vals, 64, 120)
		if key == "" || len(normalized) == 0 {
			delete(in.AcceptedEnums, k)
			continue
		}
		if key != k {
			delete(in.AcceptedEnums, k)
		}
		if in.AcceptedEnums == nil {
			in.AcceptedEnums = map[string][]string{}
		}
		in.AcceptedEnums[key] = normalized
	}
	if len(in.AcceptedEnums) == 0 {
		in.AcceptedEnums = nil
	}
	return in
}

func (s ToolJSONSurfaceDescriptor) Empty() bool {
	return s.ToolName == "" &&
		s.ReasonCode == "" &&
		len(s.FailingFieldPaths) == 0 &&
		len(s.AcceptedEnums) == 0
}

func AcceptedEvidenceRefFromEvidenceItem(item EvidenceItem) (AcceptedEvidenceRef, bool) {
	source := trimToolHandoffText(item.Source)
	ref := AcceptedEvidenceRef{
		ID:              trimToolHandoffText(item.ID),
		Kind:            item.Kind,
		Scope:           item.Scope,
		Source:          source,
		LineStart:       item.LineStart,
		LineEnd:         item.LineEnd,
		Subject:         trimToolHandoffText(item.Subject),
		OwnerSymbol:     trimToolHandoffText(item.OwnerSymbol),
		AnchorSymbol:    trimToolHandoffText(item.AnchorSymbol),
		SourcePathRole:  ClassifySourcePathRole(source),
		GroundingStatus: item.GroundingStatus,
	}
	if ref.ID == "" && ref.Source != "" && ref.LineStart > 0 {
		ref.ID = "evidence:" + ref.Source + ":" + strconv.Itoa(ref.LineStart)
	}
	if ref.LineEnd > 0 && ref.LineStart > 0 && ref.LineEnd < ref.LineStart {
		ref.LineEnd = ref.LineStart
	}
	ref = normalizeAcceptedEvidenceRef(ref)
	return ref, !ref.Empty()
}

func (r AcceptedEvidenceRef) Empty() bool {
	return r.ID == "" &&
		r.Source == "" &&
		r.Subject == "" &&
		r.OwnerSymbol == "" &&
		r.AnchorSymbol == ""
}

func ToolObservationRefFromObservationRecord(obs ObservationRecord) (ToolObservationRef, bool) {
	source := trimToolHandoffText(obs.SourceRef.Path)
	ref := ToolObservationRef{
		ID:              trimToolHandoffText(obs.ID),
		Origin:          obs.Origin,
		Producer:        trimToolHandoffText(obs.Producer),
		Source:          source,
		LineStart:       obs.Span.LineStart,
		LineEnd:         obs.Span.LineEnd,
		ClaimKey:        trimToolHandoffText(obs.ClaimKey),
		Subject:         trimToolHandoffText(obs.Subject),
		GroundingPolicy: obs.GroundingPolicy,
		GroundingStatus: obs.GroundingStatus,
	}
	if ref.ID == "" && ref.Source != "" && ref.LineStart > 0 {
		ref.ID = "observation:" + ref.Source + ":" + strconv.Itoa(ref.LineStart)
	}
	if ref.LineEnd > 0 && ref.LineStart > 0 && ref.LineEnd < ref.LineStart {
		ref.LineEnd = ref.LineStart
	}
	ref = normalizeToolObservationRef(ref)
	return ref, !ref.Empty()
}

func (r ToolObservationRef) Empty() bool {
	return r.ID == "" &&
		r.Source == "" &&
		r.ClaimKey == "" &&
		r.Subject == ""
}

func mergeToolHandoffCarrier(a, b ToolHandoffCarrier) ToolHandoffCarrier {
	if a.ToolName == "" {
		a.ToolName = b.ToolName
	}
	if a.ReasonCode == "" {
		a.ReasonCode = b.ReasonCode
	}
	if a.RepairCode == "" {
		a.RepairCode = b.RepairCode
	}
	if a.Repair == nil {
		a.Repair = cloneToolRepairPtr(b.Repair)
	}
	if a.PlanRepairPack == nil && b.PlanRepairPack != nil {
		pack := NormalizePlanRepairPack(*b.PlanRepairPack)
		a.PlanRepairPack = &pack
	}
	if a.SupportedJSON == nil && b.SupportedJSON != nil {
		surface := NormalizeToolJSONSurfaceDescriptor(*b.SupportedJSON)
		a.SupportedJSON = &surface
	}
	a.AcceptedEvidence = append(a.AcceptedEvidence, b.AcceptedEvidence...)
	a.ObservationRefs = append(a.ObservationRefs, b.ObservationRefs...)
	return NormalizeToolHandoffCarrier(a)
}

func toolHandoffCarrierKey(c ToolHandoffCarrier) string {
	if len(c.AcceptedEvidence) > 0 && c.ToolName == "emit_evidence" {
		return "accepted_evidence:" + c.ToolName
	}
	if c.ToolName != "" && c.ReasonCode != "" {
		return c.ToolName + ":" + c.ReasonCode
	}
	if c.ToolName != "" && c.RepairCode != "" {
		return c.ToolName + ":" + c.RepairCode
	}
	return c.ToolName + ":" + strconv.Itoa(len(c.AcceptedEvidence)) + ":" + strconv.Itoa(len(c.ObservationRefs))
}

func toolRepairReasonCode(repair *ToolRepair) string {
	if repair == nil {
		return ""
	}
	for _, key := range []string{"reason_code", "repair_status"} {
		if repair.Metadata == nil {
			continue
		}
		if v := trimToolHandoffText(repair.Metadata[key]); v != "" {
			return v
		}
	}
	return trimToolHandoffText(repair.Code)
}

func toolJSONSurfaceFromPlanRepairPack(pack PlanRepairPack) ToolJSONSurfaceDescriptor {
	return NormalizeToolJSONSurfaceDescriptor(ToolJSONSurfaceDescriptor{
		ToolName:          pack.ToolName,
		ReasonCode:        pack.ReasonCode,
		FailingFieldPaths: append([]string(nil), pack.FailingFieldPaths...),
		AcceptedEnums:     cloneStringSliceMap(pack.AcceptedEnums),
	})
}

func cloneToolRepairPtr(in *ToolRepair) *ToolRepair {
	if in == nil {
		return nil
	}
	out := *in
	out.Fields = append([]string(nil), in.Fields...)
	if in.Targets != nil {
		out.Targets = append([]ToolRepairTarget(nil), in.Targets...)
		for i := range out.Targets {
			out.Targets[i].Lines = append([]int(nil), in.Targets[i].Lines...)
		}
	}
	if in.Metadata != nil {
		out.Metadata = make(map[string]string, len(in.Metadata))
		for k, v := range in.Metadata {
			out.Metadata[k] = v
		}
	}
	return &out
}

func normalizeToolRepairPtr(in *ToolRepair) *ToolRepair {
	out := cloneToolRepairPtr(in)
	if out == nil {
		return nil
	}
	out.Code = trimToolHandoffText(out.Code)
	out.Hint = trimToolHandoffLongText(out.Hint, 480)
	out.Fields = normalizeToolHandoffStrings(out.Fields, toolHandoffMaxFields, 160)
	targets := out.Targets[:0]
	seen := map[string]bool{}
	for _, target := range out.Targets {
		target.File = trimToolHandoffText(target.File)
		target.Action = trimToolHandoffText(target.Action)
		if len(target.Lines) > 32 {
			target.Lines = append([]int(nil), target.Lines[:32]...)
		}
		key := target.File + ":" + target.Action + ":" + intsKey(target.Lines)
		if key == "::" || seen[key] {
			continue
		}
		seen[key] = true
		targets = append(targets, target)
		if len(targets) >= 32 {
			break
		}
	}
	out.Targets = targets
	for k, v := range out.Metadata {
		key := trimToolHandoffText(k)
		val := trimToolHandoffLongText(v, 480)
		if key == "" || val == "" {
			delete(out.Metadata, k)
			continue
		}
		if key != k {
			delete(out.Metadata, k)
			out.Metadata[key] = val
		} else {
			out.Metadata[k] = val
		}
	}
	if len(out.Metadata) == 0 {
		out.Metadata = nil
	}
	if out.Code == "" && out.Hint == "" && len(out.Fields) == 0 && len(out.Targets) == 0 && len(out.Metadata) == 0 {
		return nil
	}
	return out
}

func normalizeAcceptedEvidenceRefs(in []AcceptedEvidenceRef) []AcceptedEvidenceRef {
	seen := map[string]bool{}
	out := make([]AcceptedEvidenceRef, 0, len(in))
	for _, ref := range in {
		ref = normalizeAcceptedEvidenceRef(ref)
		if ref.Empty() {
			continue
		}
		key := ref.ID + ":" + ref.Source + ":" + strconv.Itoa(ref.LineStart) + ":" + strconv.Itoa(ref.LineEnd)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ref)
		if len(out) >= toolHandoffMaxAcceptedEvidence {
			break
		}
	}
	return out
}

func normalizeAcceptedEvidenceRef(ref AcceptedEvidenceRef) AcceptedEvidenceRef {
	ref.ID = trimToolHandoffText(ref.ID)
	ref.Source = trimToolHandoffText(ref.Source)
	ref.Subject = trimToolHandoffText(ref.Subject)
	ref.OwnerSymbol = trimToolHandoffText(ref.OwnerSymbol)
	ref.AnchorSymbol = trimToolHandoffText(ref.AnchorSymbol)
	if ref.SourcePathRole == "" && ref.Source != "" {
		ref.SourcePathRole = ClassifySourcePathRole(ref.Source)
	}
	return ref
}

func normalizeToolObservationRefs(in []ToolObservationRef) []ToolObservationRef {
	seen := map[string]bool{}
	out := make([]ToolObservationRef, 0, len(in))
	for _, ref := range in {
		ref = normalizeToolObservationRef(ref)
		if ref.Empty() {
			continue
		}
		key := ref.ID + ":" + ref.Source + ":" + strconv.Itoa(ref.LineStart) + ":" + strconv.Itoa(ref.LineEnd)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ref)
		if len(out) >= toolHandoffMaxObservationRefs {
			break
		}
	}
	return out
}

func normalizeToolObservationRef(ref ToolObservationRef) ToolObservationRef {
	ref.ID = trimToolHandoffText(ref.ID)
	ref.Producer = trimToolHandoffText(ref.Producer)
	ref.Source = trimToolHandoffText(ref.Source)
	ref.ClaimKey = trimToolHandoffText(ref.ClaimKey)
	ref.Subject = trimToolHandoffText(ref.Subject)
	return ref
}

func normalizeToolHandoffStrings(in []string, maxItems, maxLen int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = trimToolHandoffLongText(s, maxLen)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if maxItems > 0 && len(out) >= maxItems {
			break
		}
	}
	return out
}

func trimToolHandoffText(s string) string {
	return trimToolHandoffLongText(s, toolHandoffMaxTextLen)
}

func trimToolHandoffLongText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "..."
}

func cloneStringSliceMap(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for k, vals := range in {
		out[k] = append([]string(nil), vals...)
	}
	return out
}

func intsKey(vals []int) string {
	if len(vals) == 0 {
		return ""
	}
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, strconv.Itoa(v))
	}
	return strings.Join(parts, ",")
}

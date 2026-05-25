package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitHypothesisVerdict is the structured channel through which the
// extractor agent (P2.1 Turn B) emits a verdict for each hypothesis
// in AnalysisIR.HypothesisSet. It is the LLM-write half of the D7
// deferred item from project_p1_3_deferred_items.md: P1.3 wired
// hdp.Plan/Bind at analyze time + gate.Run at quality-gate time, but
// nothing wrote back the per-hypothesis verdict during exploration.
// P2.1 Turn B closes that loop.
//
// Status enum is closed: confirmed, rejected, inconclusive.
//   - confirmed: evidence in the transcript supports the hypothesis
//   - rejected:  evidence in the transcript falsifies the hypothesis
//   - inconclusive: the hypothesis was investigated but evidence is
//     insufficient. This is structurally distinct from
//     the IR-default "unknown" (= never investigated).
//
// HypothesisID is matched against AnalysisIR.HypothesisSet[*].ID at
// drain time (in MutableState.MarkHypothesis, landing in P6). Unknown
// IDs are diagnosed loudly so a typo does not silently disappear a
// real hypothesis.
//
// Citation is required for current-repo confirmed/rejected verdicts
// (you cannot confirm or falsify code facts without a concrete cite)
// and optional for inconclusive (the whole point of inconclusive is
// "we looked but couldn't find a definitive cite"). External-only
// runtime artifact frames are not current-repo citations: Execute
// accepts typed origin-specific citations when the accepted observation
// ledger proves that origin is present, moves them into the rationale
// as external evidence context, and leaves Citation empty so downstream
// renderers do not mistake them for repo proof. Runtime log / trace
// frame cites still require exact artifact-frame or artifact-local line
// matches. Current-source cites keep the same file:line[-end] shape used
// by runContractCheck — structural, no extension list.
//
// Like the other emit_* tools: ReadOnly + NonEvidenceTool. Mutating
// BusContext is not a filesystem write, and the verdict carries
// evaluative judgement, not a repo fact.
type EmitHypothesisVerdict struct {
	ReadOnly
	NonEvidenceTool
}

var emitHypothesisVerdictAllowedStatuses = map[string]types.HypothesisStatus{
	"confirmed":    types.HypConfirmed,
	"rejected":     types.HypRejected,
	"inconclusive": types.HypInconclusive,
}

type emitHypothesisVerdictParams struct {
	Items []emitHypothesisVerdictItem `json:"items"`
}

type emitHypothesisVerdictItem struct {
	HypothesisID string `json:"hypothesis_id"`
	Status       string `json:"status"`
	Rationale    string `json:"rationale,omitempty"`
	Citation     string `json:"citation,omitempty"`
	EvidenceID   string `json:"evidence_id,omitempty"`

	ArtifactCitationAccepted bool   `json:"-"`
	ArtifactCitation         string `json:"-"`
}

const EmitHypothesisVerdictProducer = "explorer.emit_hypothesis_verdict"

func (t *EmitHypothesisVerdict) Name() string { return "emit_hypothesis_verdict" }

func (t *EmitHypothesisVerdict) Description() string {
	return "Emit a verdict for each hypothesis from the prior hypothesis set that the " +
		"investigation has reached a conclusion on. Call this after the " +
		"investigation transcript has been read, with one item per hypothesis you can confidently " +
		"judge. hypothesis_id MUST match a real ID from the prior hypothesis set; typos are " +
		"flagged. status is one of: confirmed (transcript supports it), rejected (transcript " +
		"falsifies it), inconclusive (investigated but evidence is insufficient — distinct from " +
		"'never investigated'). Current-repo confirmed and rejected verdicts require a citation in " +
		"the form 'path:line' or 'path:line-end'. For observation-only external log / trace questions, " +
		"a citation that exactly matches an attached runtime frame or an artifact-local gutter line " +
		"(for example 'log:3', 'trace:5-6', or 'runtime_artifact:1-5') is accepted as artifact " +
		"context and is not published as a repo citation. For accepted external observation origins " +
		"(VCS metadata/diff, command measurements, cross-repo index, external documents, web, MCP, " +
		"connectors), an origin-specific citation such as 'git_log: commit abc123', 'git_diff: HEAD~1', " +
		"'exec_command: <tool result>', 'mcp_resource: <uri>', or 'web_page: <url>' is accepted only " +
		"when that typed origin exists in the accepted observation ledger. Inconclusive verdicts may omit the citation."
}

func (t *EmitHypothesisVerdict) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "items": {
      "type": "array",
      "description": "Batch of hypothesis verdicts. Send the full batch in one call — do not invoke the tool per item.",
      "items": {
        "type": "object",
        "properties": {
          "hypothesis_id": {"type": "string", "description": "Hypothesis ID listed in the Hypotheses section of the prompt. Required. Unknown IDs are diagnosed."},
          "status":        {"type": "string", "enum": ["confirmed", "rejected", "inconclusive"], "description": "Verdict. Current-repo confirmed/rejected verdicts require a citation; observation-only runtime-frame, artifact-local line citations, or typed origin-specific external observation refs are accepted as external context when the ledger proves that origin; inconclusive may omit it."},
          "rationale":     {"type": "string", "description": "Rationale for the verdict — explain the mechanism or invariant that produced the status. Reference load-bearing identifiers with inline ` + "`" + `code` + "`" + `. Strongly recommended."},
          "citation":      {"type": "string", "description": "Concrete current-repo code anchor in the form 'path:line' or 'path:line-end'. For external observation evidence, this may instead be an exact runtime artifact line/frame or a typed origin-specific ref such as 'git_log: commit abc123', 'git_diff: HEAD~1', 'exec_command: <tool result>', 'mcp_resource: <uri>', or 'web_page: <url>'; the tool preserves accepted external refs as context rather than repo proof. Required for current-repo confirmed/rejected unless evidence_id names an already accepted grounded evidence item."},
          "evidence_id":   {"type": "string", "description": "Optional local-model compatibility shortcut: id of an already accepted grounded evidence item. The tool resolves it to citation automatically; do not invent ids."}
        },
        "required": ["hypothesis_id", "status"]
      }
    }
  },
  "required": ["items"]
}`)
}

func (t *EmitHypothesisVerdict) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	now := time.Now()
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_hypothesis_verdict requires a writable context; the caller did not provide one (sub-agents are not supported)",
			Timestamp: now,
		}, nil
	}

	params = applyStructuredPayloadCompat(t.Name(), params, t.Parameters())

	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	var p emitHypothesisVerdictParams
	if err := dec.Decode(&p); err != nil {
		return failStrictDecode(t.Name(), now, err, nil)
	}
	if len(p.Items) == 0 {
		return failEmit(t.Name(), now, "items is empty; emit at least one verdict object per call")
	}
	if repaired := resolveHypothesisVerdictEvidenceRefs(ctx, p.Items); repaired > 0 {
		logging.Warning("[emit_hypothesis_verdict] resolved %d evidence_id reference(s) to citation(s)", repaired)
	}
	if normalized := normalizeHypothesisVerdictArtifactCitations(ctx, p.Items); normalized > 0 {
		logging.Warning("[emit_hypothesis_verdict] normalized %d external artifact citation(s) out of repo citation fields", normalized)
	}

	groundCtx := ground.BuildContext(ctx)
	built := make([]types.HypothesisVerdict, 0, len(p.Items))
	for i, in := range p.Items {
		v, perr := buildEmitHypothesisVerdictItem(in, i)
		if perr != nil {
			return failEmit(t.Name(), now, "%v", perr)
		}
		if perr := validateHypothesisVerdictCitationGrounding(ctx, groundCtx, v, i); perr != nil {
			return failEmit(t.Name(), now, "%v", perr)
		}
		built = append(built, v)
	}

	ctx.Mutable.AppendEmittedHypothesisVerdicts(built)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   renderEmitHypothesisVerdictSummary(built),
		Timestamp: now,
	}, nil
}

func resolveHypothesisVerdictEvidenceRefs(ctx *types.BusContext, items []emitHypothesisVerdictItem) int {
	if ctx == nil || ctx.Mutable == nil || len(items) == 0 {
		return 0
	}
	fixed := 0
	for i := range items {
		evidenceID := strings.TrimSpace(items[i].EvidenceID)
		if evidenceID == "" {
			continue
		}
		ev, ok := uniqueGroundedEvidenceForID(ctx, evidenceID)
		if !ok || strings.TrimSpace(ev.Source) == "" || ev.LineStart <= 0 {
			continue
		}
		citation := fmt.Sprintf("%s:%d", ev.Source, ev.LineStart)
		if ev.LineEnd > ev.LineStart {
			citation = fmt.Sprintf("%s-%d", citation, ev.LineEnd)
		}
		if strings.TrimSpace(items[i].Citation) == citation {
			continue
		}
		items[i].Citation = citation
		fixed++
	}
	return fixed
}

func uniqueGroundedEvidenceForID(ctx *types.BusContext, evidenceID string) (types.EvidenceItem, bool) {
	evidenceID = strings.TrimSpace(evidenceID)
	if evidenceID == "" {
		return types.EvidenceItem{}, false
	}
	var matched *types.EvidenceItem
	for _, ev := range preEmitAnswerEvidenceItems(ctx) {
		if ev.GroundingStatus == types.GroundingUngrounded {
			continue
		}
		if !evidenceIDMatchesEvidence(evidenceID, ev) {
			continue
		}
		candidate := ev
		if matched != nil && (matched.Source != candidate.Source ||
			matched.LineStart != candidate.LineStart ||
			matched.LineEnd != candidate.LineEnd) {
			return types.EvidenceItem{}, false
		}
		matched = &candidate
	}
	if matched == nil {
		return types.EvidenceItem{}, false
	}
	return *matched, true
}

func evidenceIDMatchesEvidence(id string, ev types.EvidenceItem) bool {
	if strings.TrimSpace(ev.ID) == id {
		return true
	}
	if stable := types.StableEvidenceID(ev); stable != "" && stable == id {
		return true
	}
	return false
}

// buildEmitHypothesisVerdictItem validates one decoded item and
// converts it into a types.HypothesisVerdict. This layer validates
// the wire shape; citation grounding runs immediately after building
// because it needs BusContext history. The cross-reference against
// HypothesisSet[*].ID remains the drain layer's responsibility, see
// MutableState.MarkHypothesis in P6.
func buildEmitHypothesisVerdictItem(in emitHypothesisVerdictItem, index int) (types.HypothesisVerdict, error) {
	hypID := strings.TrimSpace(in.HypothesisID)
	if hypID == "" {
		return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: hypothesis_id is required", index)
	}
	statusKey := strings.ToLower(strings.TrimSpace(in.Status))
	status, ok := emitHypothesisVerdictAllowedStatuses[statusKey]
	if !ok {
		return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: unknown status %q (allowed: confirmed, rejected, inconclusive)", index, in.Status)
	}
	citation := strings.TrimSpace(in.Citation)
	rationale := strings.TrimSpace(in.Rationale)

	// confirmed REQUIRES a concrete citation — asserting "this is the
	// case" without pointing at a line is the exact failure mode this
	// gate exists to prevent. rejected can be justified two ways:
	//   1. Positive counter-evidence — a file:line that contradicts
	//      the hypothesis. Preferred when available.
	//   2. Absence of evidence — the hypothesis claimed something that
	//      would exist in the repo, a thorough investigation found
	//      zero matches, so the hypothesis is rejected by absence
	//      ("no .py files" → "no Python bindings" is rejected). A
	//      line citation for absence does not exist; the rationale
	//      MUST be non-empty and load-bearing.
	// inconclusive requires neither — it is the honest "investigated
	// but could not decide" verdict.
	switch status {
	case types.HypConfirmed:
		if citation == "" {
			if !in.ArtifactCitationAccepted || rationale == "" {
				return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: status %q requires a citation (path:line or path:line-end). Use 'inconclusive' if you cannot point at concrete code.", index, status)
			}
		}
		if citation != "" && !looksLikeCitation(citation) {
			return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: citation %q does not look like 'path:line' or 'path:line-end'", index, in.Citation)
		}
	case types.HypRejected:
		if citation != "" {
			// When provided, it must still be shape-valid.
			if !looksLikeCitation(citation) {
				return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: citation %q does not look like 'path:line' or 'path:line-end'", index, in.Citation)
			}
		} else if rationale == "" {
			// Absence-based rejection requires rationale to carry the
			// load since there is no cite to anchor on.
			return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: status 'rejected' without a citation requires a non-empty rationale explaining the absence (e.g. 'no .py files found in repo').", index)
		}
	}

	return types.HypothesisVerdict{
		HypothesisID: hypID,
		Status:       status,
		Rationale:    rationale,
		Citation:     citation,
	}, nil
}

func normalizeHypothesisVerdictArtifactCitations(ctx *types.BusContext, items []emitHypothesisVerdictItem) int {
	if ctx == nil || len(items) == 0 {
		return 0
	}
	normalized := 0
	for i := range items {
		citation := strings.TrimSpace(items[i].Citation)
		if citation == "" {
			if artifactCitation, ok := hypothesisVerdictRationaleOnlyArtifactCitation(ctx, items[i].Rationale); ok {
				items[i].ArtifactCitationAccepted = true
				items[i].ArtifactCitation = artifactCitation
				items[i].Rationale = appendHypothesisArtifactCitationNote(ctx, items[i].Rationale, artifactCitation)
				normalized++
			}
			continue
		}
		if artifactCitation, ok := hypothesisVerdictLocalArtifactCitation(ctx, citation); ok {
			items[i].ArtifactCitationAccepted = true
			items[i].ArtifactCitation = artifactCitation
			items[i].Citation = ""
			items[i].Rationale = appendHypothesisArtifactCitationNote(ctx, items[i].Rationale, artifactCitation)
			normalized++
			continue
		}
		if externalCitation, ok := hypothesisVerdictOriginSpecificCitation(ctx, citation); ok {
			items[i].ArtifactCitationAccepted = true
			items[i].ArtifactCitation = externalCitation
			items[i].Citation = ""
			items[i].Rationale = appendHypothesisOriginSpecificCitationNote(ctx, items[i].Rationale, externalCitation)
			normalized++
			continue
		}
		cit, err := parseEmitHypothesisCitation(extractHypothesisArtifactCitationCandidate(citation))
		if err != nil {
			continue
		}
		artifactCitation, ok := hypothesisVerdictExternalArtifactCitation(ctx, cit)
		if !ok {
			continue
		}
		items[i].ArtifactCitationAccepted = true
		items[i].ArtifactCitation = artifactCitation
		items[i].Citation = ""
		items[i].Rationale = appendHypothesisArtifactCitationNote(ctx, items[i].Rationale, artifactCitation)
		normalized++
	}
	return normalized
}

func hypothesisVerdictOriginSpecificCitation(ctx *types.BusContext, raw string) (string, bool) {
	ref := cleanHypothesisOriginSpecificCitation(raw)
	if ref == "" {
		return "", false
	}
	origin, genericCarrier := hypothesisVerdictOriginSpecificCitationOrigin(ref)
	if genericCarrier {
		if !hypothesisVerdictHasAnyOriginSpecificSupport(ctx) {
			return "", false
		}
		return ref, true
	}
	if origin == types.AnswerEvidenceOriginUnknown ||
		!types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) ||
		!hypothesisVerdictHasOriginSpecificSupport(ctx, origin) {
		return "", false
	}
	return ref, true
}

func cleanHypothesisOriginSpecificCitation(raw string) string {
	ref := strings.TrimSpace(raw)
	ref = strings.Trim(ref, "`\"' \t\r\n")
	if strings.HasPrefix(ref, "[") && strings.Contains(ref, "]") {
		body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(ref, "["), "]"))
		if strings.Contains(body, ":") {
			ref = body
		}
	}
	return strings.TrimSpace(ref)
}

func hypothesisVerdictOriginSpecificCitationOrigin(ref string) (types.AnswerEvidenceOrigin, bool) {
	prefix := ref
	if before, _, ok := strings.Cut(ref, ":"); ok {
		prefix = before
	} else if fields := strings.Fields(ref); len(fields) > 0 {
		prefix = fields[0]
	}
	token := strings.ToLower(strings.TrimSpace(prefix))
	token = strings.ReplaceAll(token, "-", "_")
	switch token {
	case "exec_command", "command", "tool_result", "external_observation":
		return types.AnswerEvidenceOriginUnknown, true
	}
	origin := types.AnswerEvidenceOriginFromStructuredToken(token)
	if origin == types.AnswerEvidenceOriginRuntimeArtifact {
		return types.AnswerEvidenceOriginUnknown, false
	}
	return origin, false
}

func hypothesisVerdictHasOriginSpecificSupport(ctx *types.BusContext, origin types.AnswerEvidenceOrigin) bool {
	if ctx == nil || origin == types.AnswerEvidenceOriginUnknown ||
		!types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	for _, record := range ledger.Records {
		if record.Origin == origin {
			return true
		}
	}
	if ctx.AnalysisIR != nil && ctx.Mutable != nil {
		rm := &ctx.AnalysisIR.RequestModel
		for _, fact := range ctx.Mutable.StableInvestigationAggregateFacts() {
			for _, factOrigin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
				if factOrigin == origin {
					return true
				}
			}
		}
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			for _, fact := range ta.AcceptedAggregateFacts {
				for _, factOrigin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
					if factOrigin == origin {
						return true
					}
				}
			}
		}
	}
	return false
}

func hypothesisVerdictHasAnyOriginSpecificSupport(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	for _, record := range ledger.Records {
		if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
			return true
		}
	}
	if ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return false
	}
	rm := &ctx.AnalysisIR.RequestModel
	allFacts := append([]types.AnswerAggregateFact(nil), ctx.Mutable.StableInvestigationAggregateFacts()...)
	if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
		allFacts = append(allFacts, ta.AcceptedAggregateFacts...)
	}
	for _, fact := range allFacts {
		for _, origin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
			if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
				return true
			}
		}
	}
	return false
}

func hypothesisVerdictRationaleOnlyArtifactCitation(ctx *types.BusContext, rationale string) (string, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || !ctx.AnalysisIR.RequestModel.HasObservationOnlyRuntimeArtifact() ||
		strings.TrimSpace(rationale) == "" {
		return "", false
	}
	hasLog, hasTrace := hypothesisVerdictHasLogTraceArtifacts(ctx)
	if hypothesisVerdictPrefersChinese(ctx) {
		switch {
		case hasLog && !hasTrace:
			return "附件日志", true
		case hasTrace && !hasLog:
			return "附件 trace", true
		default:
			return "附件运行时材料", true
		}
	}
	switch {
	case hasLog && !hasTrace:
		return "attached log", true
	case hasTrace && !hasLog:
		return "attached trace", true
	default:
		return "attached runtime artifact", true
	}
}

func hypothesisVerdictLocalArtifactCitation(ctx *types.BusContext, raw string) (string, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || !ctx.AnalysisIR.RequestModel.HasObservationOnlyRuntimeArtifact() {
		return "", false
	}
	candidates := []string{strings.TrimSpace(raw)}
	extracted := strings.TrimSpace(extractHypothesisArtifactCitationCandidate(raw))
	if extracted != "" && extracted != candidates[0] {
		candidates = append(candidates, extracted)
	}
	for _, candidate := range candidates {
		prefix, start, end, ok := parseHypothesisLocalArtifactLineCitation(candidate)
		if !ok || !hypothesisVerdictArtifactPrefixMatches(ctx, prefix) {
			continue
		}
		return renderHypothesisLocalArtifactCitation(ctx, prefix, start, end), true
	}
	return "", false
}

func parseHypothesisLocalArtifactLineCitation(raw string) (prefix string, start, end int, ok bool) {
	s := strings.Trim(strings.TrimSpace(raw), "` \t\r\n.,;()[]{}")
	idx := strings.LastIndex(s, ":")
	if idx <= 0 || idx == len(s)-1 {
		return "", 0, 0, false
	}
	prefix = normalizeHypothesisArtifactPrefix(s[:idx])
	if !isHypothesisArtifactLinePrefix(prefix) {
		return "", 0, 0, false
	}
	linePart := s[idx+1:]
	startText, endText := linePart, ""
	if dash := strings.Index(linePart, "-"); dash > 0 {
		startText = linePart[:dash]
		endText = linePart[dash+1:]
	}
	start, err := strconv.Atoi(startText)
	if err != nil || start <= 0 {
		return "", 0, 0, false
	}
	end = start
	if endText != "" {
		end, err = strconv.Atoi(endText)
		if err != nil || end < start {
			return "", 0, 0, false
		}
	}
	return prefix, start, end, true
}

func normalizeHypothesisArtifactPrefix(prefix string) string {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	prefix = strings.ReplaceAll(prefix, "_", "-")
	prefix = strings.ReplaceAll(prefix, " ", "-")
	return prefix
}

func isHypothesisArtifactLinePrefix(prefix string) bool {
	switch prefix {
	case "runtime-artifact", "artifact", "attached-artifact",
		"log", "attached-log", "runtime-log",
		"trace", "hitrace", "atrace", "perf-trace", "attached-trace":
		return true
	default:
		return false
	}
}

func hypothesisVerdictArtifactPrefixMatches(ctx *types.BusContext, prefix string) bool {
	hasLog, hasTrace := hypothesisVerdictHasLogTraceArtifacts(ctx)
	switch prefix {
	case "log", "attached-log", "runtime-log":
		return hasLog
	case "trace", "hitrace", "atrace", "perf-trace", "attached-trace":
		return hasTrace
	default:
		return hasLog || hasTrace
	}
}

func hypothesisVerdictHasLogTraceArtifacts(ctx *types.BusContext) (hasLog, hasTrace bool) {
	if ctx == nil {
		return false, false
	}
	if ctx.Mutable != nil {
		hasLog = ctx.Mutable.LogTriage() != nil
		hasTrace = ctx.Mutable.PerfTrace() != nil
	}
	if ctx.AnalysisIR != nil {
		hasLog = hasLog || ctx.AnalysisIR.RequestModel.LogTriage != nil
		hasTrace = hasTrace || ctx.AnalysisIR.RequestModel.PerfTrace != nil
	}
	return hasLog, hasTrace
}

func renderHypothesisLocalArtifactCitation(ctx *types.BusContext, prefix string, start, end int) string {
	hasLog, hasTrace := hypothesisVerdictHasLogTraceArtifacts(ctx)
	nounZH, nounEN := "附件运行时材料", "attached runtime artifact"
	switch prefix {
	case "log", "attached-log", "runtime-log":
		nounZH, nounEN = "附件日志", "attached log"
	case "trace", "hitrace", "atrace", "perf-trace", "attached-trace":
		nounZH, nounEN = "附件 trace", "attached trace"
	default:
		if hasLog && !hasTrace {
			nounZH, nounEN = "附件日志", "attached log"
		} else if hasTrace && !hasLog {
			nounZH, nounEN = "附件 trace", "attached trace"
		}
	}
	line := strconv.Itoa(start)
	if end > start {
		line = fmt.Sprintf("%d-%d", start, end)
	}
	if hypothesisVerdictPrefersChinese(ctx) {
		sep := ""
		if strings.Contains(nounZH, "trace") {
			sep = " "
		}
		return nounZH + sep + "行：" + line
	}
	return nounEN + " line " + line
}

func extractHypothesisArtifactCitationCandidate(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if _, err := parseEmitHypothesisCitation(s); err == nil {
		return s
	}
	candidates := make([]string, 0, 4)
	if open := strings.LastIndex(s, "("); open >= 0 {
		if closeRel := strings.Index(s[open+1:], ")"); closeRel >= 0 {
			candidates = append(candidates, strings.TrimSpace(s[open+1:open+1+closeRel]))
		}
	}
	if open := strings.LastIndex(s, "`"); open >= 0 {
		prefix := s[:open]
		if prev := strings.LastIndex(prefix, "`"); prev >= 0 {
			candidates = append(candidates, strings.TrimSpace(s[prev+1:open]))
		}
	}
	fields := strings.Fields(s)
	if len(fields) > 0 {
		candidates = append(candidates, strings.Trim(fields[len(fields)-1], " \t\r\n.,;:()[]{}"))
	}
	for _, candidate := range candidates {
		if _, err := parseEmitHypothesisCitation(candidate); err == nil {
			return candidate
		}
	}
	return s
}

func hypothesisVerdictExternalArtifactCitation(ctx *types.BusContext, cit types.Citation) (string, bool) {
	for _, bundle := range hypothesisVerdictLogBundles(ctx) {
		if bundle == nil || !bundle.IsExternalSource() {
			continue
		}
		var matched string
		types.WalkLogFrames(bundle, func(frame types.LogFrame) {
			if matched != "" {
				return
			}
			if artifactCitationMatchesLogFrame(cit, frame) {
				matched = artifactFrameCitationSurface(frame)
			}
		})
		if matched != "" {
			return matched, true
		}
	}
	for _, perf := range hypothesisVerdictPerfBundles(ctx) {
		if perf == nil || !perf.IsExternalSource() {
			continue
		}
		for _, frame := range perf.LogFrames() {
			if artifactCitationMatchesLogFrame(cit, frame) {
				return artifactFrameCitationSurface(frame), true
			}
		}
	}
	return "", false
}

func hypothesisVerdictLogBundles(ctx *types.BusContext) []*types.LogBundle {
	if ctx == nil {
		return nil
	}
	var out []*types.LogBundle
	add := func(bundle *types.LogBundle) {
		if bundle != nil {
			out = append(out, bundle)
		}
	}
	if ctx.Mutable != nil {
		add(ctx.Mutable.LogTriage())
	}
	if ctx.AnalysisIR != nil {
		add(ctx.AnalysisIR.RequestModel.LogTriage)
	}
	return out
}

func hypothesisVerdictPerfBundles(ctx *types.BusContext) []*types.PerfBundle {
	if ctx == nil {
		return nil
	}
	var out []*types.PerfBundle
	add := func(bundle *types.PerfBundle) {
		if bundle != nil {
			out = append(out, bundle)
		}
	}
	if ctx.Mutable != nil {
		add(ctx.Mutable.PerfTrace())
	}
	if ctx.AnalysisIR != nil {
		add(ctx.AnalysisIR.RequestModel.PerfTrace)
	}
	return out
}

func artifactCitationMatchesLogFrame(cit types.Citation, frame types.LogFrame) bool {
	if cit.Line <= 0 {
		return false
	}
	frameFile, frameLine := artifactFrameCitationParts(frame)
	if frameLine <= 0 {
		return false
	}
	if frameLine < cit.Line || frameLine > cit.LineEnd {
		return false
	}
	return artifactCitationPathMatches(cit.File, frameFile)
}

func artifactCitationPathMatches(citationPath, framePath string) bool {
	citationPath = canonicalArtifactCitationPath(citationPath)
	framePath = canonicalArtifactCitationPath(framePath)
	if citationPath == "" || framePath == "" {
		return false
	}
	if citationPath == framePath {
		return true
	}
	if strings.HasPrefix(citationPath, "/") && strings.HasSuffix(citationPath, "/"+framePath) {
		return true
	}
	if strings.HasPrefix(framePath, "/") && strings.HasSuffix(framePath, "/"+citationPath) {
		return true
	}
	return false
}

func canonicalArtifactCitationPath(path string) string {
	path = normalizeArtifactCitationPath(path)
	path = strings.TrimPrefix(path, "./")
	for strings.Contains(path, "/./") {
		path = strings.ReplaceAll(path, "/./", "/")
	}
	return path
}

func artifactFrameCitationSurface(frame types.LogFrame) string {
	file, line := artifactFrameCitationParts(frame)
	if file == "" || line <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", file, line)
}

func artifactFrameCitationParts(frame types.LogFrame) (file string, line int) {
	file = canonicalArtifactCitationPath(frame.File)
	line = frame.Line
	if file != "" && line > 0 {
		return file, line
	}
	raw := strings.TrimSpace(frame.Raw)
	if raw == "" {
		return file, line
	}
	candidate := extractHypothesisArtifactCitationCandidate(raw)
	cit, err := parseEmitHypothesisCitation(candidate)
	if err != nil {
		return file, line
	}
	if file == "" {
		file = canonicalArtifactCitationPath(cit.File)
	}
	if line <= 0 {
		line = cit.Line
	}
	return file, line
}

func appendHypothesisArtifactCitationNote(ctx *types.BusContext, rationale, artifactCitation string) string {
	rationale = strings.TrimSpace(rationale)
	artifactCitation = strings.TrimSpace(artifactCitation)
	if artifactCitation == "" || strings.Contains(rationale, artifactCitation) {
		return rationale
	}
	note := "External runtime frame: " + artifactCitation
	if hypothesisVerdictPrefersChinese(ctx) {
		note = "外部运行时帧：" + artifactCitation
	}
	if rationale == "" {
		return note
	}
	return rationale + "\n" + note
}

func appendHypothesisOriginSpecificCitationNote(ctx *types.BusContext, rationale, ref string) string {
	rationale = strings.TrimSpace(rationale)
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(rationale, ref) {
		return rationale
	}
	note := "Origin-specific evidence anchor: " + ref
	if hypothesisVerdictPrefersChinese(ctx) {
		note = "外部证据锚点：" + ref
	}
	if rationale == "" {
		return note
	}
	return rationale + "\n" + note
}

func hypothesisVerdictPrefersChinese(ctx *types.BusContext) bool {
	lang := requestedAnswerDocumentLanguage(ctx)
	if lang == "" && ctx != nil && ctx.AnalysisIR != nil {
		lang = strings.ToLower(strings.TrimSpace(ctx.AnalysisIR.RequestModel.Language))
	}
	return lang == "" || strings.HasPrefix(lang, "zh")
}

func validateHypothesisVerdictCitationGrounding(ctx *types.BusContext, gc *ground.Context, v types.HypothesisVerdict, index int) error {
	if strings.TrimSpace(v.Citation) == "" {
		return nil
	}
	if gc == nil || (len(gc.LineIndex) == 0 && gc.Graph == nil) {
		return nil
	}
	cit, err := parseEmitHypothesisCitation(v.Citation)
	if err != nil {
		return fmt.Errorf("items[%d]: %v", index, err)
	}
	claimText := strings.TrimSpace(v.Rationale)
	if h := hypothesisStatementForVerdict(ctx, v.HypothesisID); h != "" {
		claimText += "\n" + h
	}
	report := ground.GroundClaimCitation(cit, claimText, gc)
	if !report.Valid {
		return fmt.Errorf("items[%d]: citation %q is not grounded: %s", index, v.Citation, report.Reason)
	}
	if report.Corroborated {
		return nil
	}
	return fmt.Errorf(
		"items[%d]: citation %q does not corroborate the hypothesis/rationale identifiers near line %d. Use one of these candidate anchors instead: %s",
		index, v.Citation, cit.Line, ground.FormatClaimCitationCandidates(report.Candidates))
}

func parseEmitHypothesisCitation(raw string) (types.Citation, error) {
	s := strings.TrimSpace(raw)
	idx := strings.LastIndex(s, ":")
	if idx <= 0 || idx == len(s)-1 {
		return types.Citation{}, fmt.Errorf("citation %q does not look like 'path:line' or 'path:line-end'", raw)
	}
	file := s[:idx]
	linePart := s[idx+1:]
	startText, endText := linePart, ""
	if dash := strings.Index(linePart, "-"); dash > 0 {
		startText = linePart[:dash]
		endText = linePart[dash+1:]
	}
	start, err := strconv.Atoi(startText)
	if err != nil || start <= 0 {
		return types.Citation{}, fmt.Errorf("citation %q has a non-positive start line", raw)
	}
	end := start
	if endText != "" {
		end, err = strconv.Atoi(endText)
		if err != nil || end <= 0 {
			return types.Citation{}, fmt.Errorf("citation %q has a non-positive end line", raw)
		}
		if end < start {
			return types.Citation{}, fmt.Errorf("citation %q has a reversed line range", raw)
		}
	}
	return types.Citation{File: file, Line: start, LineEnd: end}, nil
}

func hypothesisStatementForVerdict(ctx *types.BusContext, id string) string {
	if ctx == nil || ctx.AnalysisIR == nil || strings.TrimSpace(id) == "" {
		return ""
	}
	for _, h := range ctx.AnalysisIR.HypothesisSet {
		if h.ID == id {
			return h.Statement
		}
	}
	return ""
}

// looksLikeCitation reports whether s is shaped like 'path:line' or
// 'path:line-end'. Reuses the same structural shape the contract
// checker accepts (no curated extension list, accepts nested paths
// and bare README.md). Logic is intentionally simple: split on the
// LAST ':' so paths containing ':' (rare on Unix, possible) still
// parse, then verify the trailing segment is a positive integer or
// 'int-int' range.
func looksLikeCitation(s string) bool {
	if !emitLooksLikePath(s) {
		return false
	}
	idx := strings.LastIndex(s, ":")
	if idx <= 0 || idx == len(s)-1 {
		return false
	}
	pathPart := s[:idx]
	linePart := s[idx+1:]
	if !emitLooksLikePath(pathPart) {
		return false
	}
	// linePart is either "N" or "N-M" with N, M positive ints.
	if dash := strings.Index(linePart, "-"); dash > 0 {
		first := linePart[:dash]
		second := linePart[dash+1:]
		return isPositiveInt(first) && isPositiveInt(second)
	}
	return isPositiveInt(linePart)
}

func isPositiveInt(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	// Reject leading zeros like "0", "01" — line 0 fails the
	// positive-line guard everywhere else in the codebase.
	if s == "0" {
		return false
	}
	return true
}

func renderEmitHypothesisVerdictSummary(items []types.HypothesisVerdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "emit_hypothesis_verdict accepted %d verdict(s)\n", len(items))
	counts := make(map[types.HypothesisStatus]int)
	for _, v := range items {
		counts[v.Status]++
	}
	for _, st := range []types.HypothesisStatus{types.HypConfirmed, types.HypRejected, types.HypInconclusive} {
		if n := counts[st]; n > 0 {
			fmt.Fprintf(&b, "  %s: %d\n", st, n)
		}
	}
	return b.String()
}

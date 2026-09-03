package tool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

// normalizeMisroutedTraceRootCausePatchField moves a submitted full-emit
// selector to the patch field only when that field is absent. It never
// chooses between competing payloads or invents a selector/version. Preserve
// the inner object fields (or decode exactly one string wrapper), including
// array order, then leave all semantic checks to the existing binder.
func normalizeMisroutedTraceRootCausePatchField(raw json.RawMessage) (json.RawMessage, bool) {
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return raw, false
	}
	if _, exists := root["replace_trace_root_causes"]; exists {
		return raw, false
	}
	carrier, exists := root["trace_root_causes"]
	if !exists || !traceRootCausePatchHasUniqueObjectKeys(raw) {
		return raw, false
	}
	report, ok := decodeTraceRootCauseReportCarrier(carrier)
	if !ok {
		return raw, false
	}
	if _, exists := report["root_causes"]; !exists {
		return raw, false
	}
	var encoded string
	if json.Unmarshal(carrier, &encoded) == nil {
		carrier = json.RawMessage(encoded)
	}
	if !traceRootCausePatchHasUniqueObjectKeys(carrier) {
		return raw, false
	}
	root["replace_trace_root_causes"] = carrier
	delete(root, "trace_root_causes")
	out, err := json.Marshal(root)
	if err != nil {
		return raw, false
	}
	return out, true
}

func traceRootCausePatchHasUniqueObjectKeys(raw json.RawMessage) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return false
		}
		seen[key] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return false
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}

// normalizeMisplacedTraceRootCauseSchemaVersion repairs one unambiguous
// structural carrier drift seen in production tool calls: the model authors
// the ordered candidate selections inside trace_root_causes but places the
// fixed schema_version discriminator at the document top level. The public
// answer schema has no top-level schema_version field, and the nested report
// is otherwise complete, so moving only the exact current constant is
// lossless. Candidate IDs, their order, visible answer text, and every
// conclusion remain untouched. Wrong/ambiguous values fail open to the
// existing optional-sidecar validation path.
func normalizeMisplacedTraceRootCauseSchemaVersion(raw json.RawMessage, reportField string) (json.RawMessage, bool) {
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return raw, false
	}
	outerVersion, ok := root["schema_version"]
	if !ok || !isExactTraceRootCauseSchemaVersion(outerVersion) {
		return raw, false
	}
	report, ok := decodeTraceRootCauseReportCarrier(root[reportField])
	if !ok {
		return raw, false
	}
	if _, exists := report["schema_version"]; exists {
		return raw, false
	}
	if _, exists := report["root_causes"]; !exists {
		return raw, false
	}
	canonicalVersion, err := json.Marshal(types.TraceRootCauseReportSchemaVersion)
	if err != nil {
		return raw, false
	}
	report["schema_version"] = canonicalVersion
	reportRaw, err := json.Marshal(report)
	if err != nil {
		return raw, false
	}
	root[reportField] = reportRaw
	delete(root, "schema_version")
	repaired, err := json.Marshal(root)
	if err != nil {
		return raw, false
	}
	return repaired, true
}

// normalizeMisplacedTraceRootCauseSelection re-homes a selector whose whole
// report body was lifted to the document top level: a top-level
// `root_causes` array of `{candidate_id}` objects (optionally beside the exact
// current `schema_version`) with NO report carrier present. It is the sibling
// of normalizeMisplacedTraceRootCauseSchemaVersion — the same local-model
// flattening, one level deeper (eval witness
// trace_query_donghu_real_frame_multicausal 2026-09-02, §40.29.1 ★20: the
// unknown-field quarantine dropped `$.root_causes` and the accepted 7-seat
// selection shipped as `unavailable`). Lossless by construction: candidate
// ids and their order are copied verbatim; any item that is not an object
// carrying a string candidate_id fails open (nothing moves, the quarantine
// keeps its say), and every semantic check stays with the binder.
func normalizeMisplacedTraceRootCauseSelection(raw json.RawMessage, reportField string) (json.RawMessage, bool) {
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return raw, false
	}
	if _, exists := root["trace_root_causes"]; exists {
		return raw, false
	}
	if _, exists := root["replace_trace_root_causes"]; exists {
		return raw, false
	}
	selection, exists := root["root_causes"]
	if !exists {
		return raw, false
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(selection, &items) != nil {
		return raw, false
	}
	for _, item := range items {
		var id string
		if raw, ok := item["candidate_id"]; !ok || json.Unmarshal(raw, &id) != nil || strings.TrimSpace(id) == "" {
			return raw, false
		}
	}
	canonicalVersion, err := json.Marshal(types.TraceRootCauseReportSchemaVersion)
	if err != nil {
		return raw, false
	}
	report := map[string]json.RawMessage{"schema_version": canonicalVersion, "root_causes": selection}
	if outer, ok := root["schema_version"]; ok {
		if !isExactTraceRootCauseSchemaVersion(outer) {
			return raw, false
		}
		delete(root, "schema_version")
	}
	reportRaw, err := json.Marshal(report)
	if err != nil {
		return raw, false
	}
	root[reportField] = reportRaw
	delete(root, "root_causes")
	repaired, err := json.Marshal(root)
	if err != nil {
		return raw, false
	}
	return repaired, true
}

// decodeTraceRootCauseReportCarrier accepts the native object and the one
// lossless streaming shape where that complete object was JSON-encoded once
// more as a string. json.Unmarshal on the inner text must consume one complete
// object; malformed, scalar, array, or null carriers remain untouched. This
// helper does not normalize any report field or choose any candidate.
func decodeTraceRootCauseReportCarrier(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var report map[string]json.RawMessage
	if err := json.Unmarshal(raw, &report); err == nil && report != nil {
		return report, true
	}
	var encoded string
	if json.Unmarshal(raw, &encoded) != nil {
		return nil, false
	}
	if json.Unmarshal([]byte(encoded), &report) != nil || report == nil {
		return nil, false
	}
	return report, true
}

func isExactTraceRootCauseSchemaVersion(raw json.RawMessage) bool {
	var version int
	if json.Unmarshal(raw, &version) == nil {
		return version == types.TraceRootCauseReportSchemaVersion
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return false
	}
	return text == strconv.Itoa(types.TraceRootCauseReportSchemaVersion)
}

// errTraceRootCauseSelectorShape is the fixed, model-facing wording for a
// selector that does not decode as the taught object. The Go decoder's own
// text (struct/type names) stays on the log line only.
var errTraceRootCauseSelectorShape = errors.New("selector is not the taught object {schema_version, root_causes:[{candidate_id}]}")

// resolveTraceRootCauseReportForEmit validates and binds the optional selector.
// An emit that OMITS the selector (absent / null) inherits the last accepted
// report — for patches and for full re-emits alike (eval witness
// trace_query_donghu_real_frame_multicausal 2026-09-02: the finalizer's
// iter=2 full re-emit, issued after an accepted iter=1 selection to revise
// answer blocks, omitted `trace_root_causes` and the sidecar shipped as
// `unavailable`; omission is not a withdrawal — the model withdraws with an
// explicit `"root_causes": []`, which binds to the empty report). The
// contract is frozen for the run, so the retained selection stays valid.
// Sidecar errors and the binder's typed advisories (part-drops and
// non-dropping notes) are returned for the disclosure lane but never own
// whether the full answer mutation is accepted.
func resolveTraceRootCauseReportForEmit(ctx *types.BusContext, submitted json.RawMessage, patch bool) (*types.TraceRootCauseReportV2, []tracefinding.RootCauseSelectionAdvisory, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil, nil
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || !contract.RootCauseReportEnabled {
		if traceRootCauseSelectorSubmitted(submitted) {
			return nil, nil, fmt.Errorf("trace_root_causes is not enabled for this request")
		}
		return nil, nil, nil
	}
	if !traceRootCauseSelectorSubmitted(submitted) {
		if report := ctx.Mutable.TraceRootCauseReport(); report != nil {
			return report, nil, nil
		}
		// §40.31.1 ★16: the model's last VALID selection rode an emit that was
		// rejected for answer-structure reasons; the accepted re-emit/patch
		// that omits the selector inherits it.
		return ctx.Mutable.PendingTraceRootCauseReport(), nil, nil
	}
	var selection types.TraceRootCauseReportV2
	if err := json.Unmarshal(submitted, &selection); err != nil {
		if patch {
			return ctx.Mutable.TraceRootCauseReport(), nil, fmt.Errorf("%w: %v", errTraceRootCauseSelectorShape, err)
		}
		return nil, nil, fmt.Errorf("%w: %v", errTraceRootCauseSelectorShape, err)
	}
	report, advisories, err := tracefinding.BindRootCauseReportSelectionWithAdvisories(&selection, contract)
	if err != nil && patch {
		return ctx.Mutable.TraceRootCauseReport(), advisories, err
	}
	return report, advisories, err
}

func traceRootCauseSelectorSubmitted(submitted json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(submitted))
	return trimmed != "" && trimmed != "null"
}

// traceRootCauseSelection is the resolved state of the optional selector
// carrier for one emit/patch: the report to store when the document commit
// succeeds (bound from this submission, or inherited), whether THIS call's
// submission bound validly (staged on a rejected commit — §40.31.1 ★16) or
// was rejected by the binder (customer reason_code
// model_root_cause_selection_rejected when no valid selection follows —
// §40.44). The typed disclosures the model must see (V2-3, §40.19: an
// optional carrier's failure never rejects the main transaction but is never
// silent) are minted on the call's optionalCarrierLedger at resolve time and
// reach whichever result the call returns through its finalize choke point.
type traceRootCauseSelection struct {
	report          *types.TraceRootCauseReportV2
	boundFromSubmit bool
	rejected        bool
}

// resolveTraceRootCauseSelectionForEmit runs the binder BEFORE the document
// persist (so a patch can still see the previously accepted report when
// deciding between "ignored" and "retained_previous") and mints every
// binder error / part-drop advisory as a typed outcome on the ledger.
func resolveTraceRootCauseSelectionForEmit(ctx *types.BusContext, carriers *optionalCarrierLedger, submitted json.RawMessage, patch bool) traceRootCauseSelection {
	carrier := "trace_root_causes"
	if patch {
		carrier = "replace_trace_root_causes"
	}
	report, advisories, err := resolveTraceRootCauseReportForEmit(ctx, submitted, patch)
	selection := traceRootCauseSelection{report: report}
	if err != nil {
		selection.rejected = traceRootCauseSelectorSubmitted(submitted)
		carriers.mint(traceRootCauseIgnoredOutcome(ctx, carrier, err, patch), "detail="+strconv.Quote(err.Error()))
	} else if traceRootCauseSelectorSubmitted(submitted) {
		selection.boundFromSubmit = report != nil
	}
	// §40.44 G-emit-faces fold-in #2: the advisory's typed kind is the precise
	// signal for the disclosure lane. Only a part the binder did NOT honour
	// (a dropped description) mints the typed part_dropped outcome with the
	// resend hint; a non-dropping note (the description is published as
	// written) is soft guidance and rides as one plain Summary line — minting
	// it as part_dropped would be a false closed-set status inviting a
	// needless patch round.
	for _, advisory := range advisories {
		if !advisory.Dropped() {
			carriers.note(advisory.Reason)
			continue
		}
		carriers.mint(types.OptionalCarrierOutcome{
			Carrier: carrier, Status: types.OptionalCarrierStatusPartDropped, Reason: advisory.Reason,
			Hint: "the typed selection is kept; to publish a description, resend that item with customer-readable prose via replace_trace_root_causes in the next emit_answer_document_patch",
		})
	}
	return selection
}

// traceRootCauseIgnoredOutcome words one ignored selector: the binder's own
// precise reason (candidate ids quoted), the retained/ignored status decided
// against the report that is accepted RIGHT NOW (before persist nulls it),
// and the repair route with the selectable roster ids.
func traceRootCauseIgnoredOutcome(ctx *types.BusContext, carrier string, err error, patch bool) types.OptionalCarrierOutcome {
	outcome := types.OptionalCarrierOutcome{Carrier: carrier, Status: types.OptionalCarrierStatusIgnored}
	if err != nil {
		outcome.Reason = err.Error()
		if errors.Is(err, errTraceRootCauseSelectorShape) {
			outcome.Reason = errTraceRootCauseSelectorShape.Error()
		}
	}
	if ctx == nil || ctx.Mutable == nil {
		return outcome
	}
	if patch && ctx.Mutable.TraceRootCauseReport() != nil {
		outcome.Status = types.OptionalCarrierStatusRetainedPrevious
	}
	contract := ctx.Mutable.TraceFindingContract()
	selectable := tracefinding.SelectableRootCauseCandidates(contract)
	if contract == nil || !contract.RootCauseReportEnabled || len(selectable) == 0 {
		outcome.Hint = "no root-cause selection is offered for this request; omit the field"
		return outcome
	}
	ids := make([]string, 0, len(selectable))
	for _, candidate := range selectable {
		ids = append(ids, candidate.Decision.CandidateID)
	}
	outcome.Hint = "resend the complete ordered selection as replace_trace_root_causes in the next emit_answer_document_patch (list untouched blocks in unchanged_block_ids) or as trace_root_causes on a full re-emit; selectable candidate_id: " + strings.Join(ids, ", ")
	return outcome
}

// commitTraceRootCauseSelection is the ONE commit tail for the selector on
// every emit/patch exit after the selector was resolved: an accepted
// document commit stores the resolved report (the document commit itself
// nulls the sidecar), ANY rejected exit stages a validly bound submission
// (§40.31.1 ★16), and a submission the binder rejected is recorded on the
// state so the customer sidecar can say "rejected" rather than "never
// selected" (§40.44 residual a). The typed outcomes themselves reach the
// tool result through the call's ledger finalize — a structurally rejected
// emit also learns that its selector was bad.
func commitTraceRootCauseSelection(ctx *types.BusContext, res types.ToolResult, persistErr error, selection traceRootCauseSelection) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	accepted := persistErr == nil && res.Success
	switch {
	case accepted && selection.report != nil:
		ctx.Mutable.SetTraceRootCauseReport(selection.report)
	case !accepted && selection.boundFromSubmit && selection.report != nil:
		ctx.Mutable.SetPendingTraceRootCauseReport(selection.report)
	}
	if selection.rejected {
		ctx.Mutable.MarkTraceRootCauseSelectorRejected()
	}
}

package tool

import (
	"bytes"
	"encoding/json"
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

// resolveTraceFindingForEmit enforces the opt-in contract before the shared
// document persist path is allowed to commit either artifact.
func resolveTraceFindingForEmit(ctx *types.BusContext, submitted *types.TraceFindingV1, patch bool) (*types.TraceFindingV1, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || !contract.Required {
		if submitted != nil {
			return nil, fmt.Errorf("trace_finding is not enabled for this request")
		}
		return nil, nil
	}
	finding := submitted
	if patch && finding == nil {
		finding = ctx.Mutable.TraceFinding()
	}
	if err := tracefinding.Validate(finding, contract); err != nil {
		return nil, err
	}
	return finding, nil
}

// resolveTraceRootCauseReportForEmit validates and binds the optional selector.
// An emit that OMITS the selector (absent / null) inherits the last accepted
// report — for patches and for full re-emits alike (eval witness
// trace_query_donghu_real_frame_multicausal 2026-09-02: the finalizer's
// iter=2 full re-emit, issued after an accepted iter=1 selection to revise
// answer blocks, omitted `trace_root_causes` and the sidecar shipped as
// `unavailable`; omission is not a withdrawal — the model withdraws with an
// explicit `"root_causes": []`, which binds to the empty report). The
// contract is frozen for the run, so the retained selection stays valid.
// Sidecar errors are returned for diagnostics but never own whether the
// full answer mutation is accepted.
func resolveTraceRootCauseReportForEmit(ctx *types.BusContext, submitted json.RawMessage, patch bool) (*types.TraceRootCauseReportV2, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || !contract.RootCauseReportEnabled {
		if len(strings.TrimSpace(string(submitted))) > 0 && string(submitted) != "null" {
			return nil, fmt.Errorf("trace_root_causes is not enabled for this request")
		}
		return nil, nil
	}
	if len(strings.TrimSpace(string(submitted))) == 0 || string(submitted) == "null" {
		if report := ctx.Mutable.TraceRootCauseReport(); report != nil {
			return report, nil
		}
		// §40.31.1 ★16: the model's last VALID selection rode an emit that was
		// rejected for answer-structure reasons; the accepted re-emit/patch
		// that omits the selector inherits it.
		return ctx.Mutable.PendingTraceRootCauseReport(), nil
	}
	var selection types.TraceRootCauseReportV2
	if err := json.Unmarshal(submitted, &selection); err != nil {
		if patch {
			return ctx.Mutable.TraceRootCauseReport(), fmt.Errorf("decode optional trace_root_causes selector: %w", err)
		}
		return nil, fmt.Errorf("decode optional trace_root_causes selector: %w", err)
	}
	report, advisories, err := tracefinding.BindRootCauseReportSelectionWithAdvisories(&selection, contract)
	if len(advisories) > 0 {
		ctx.Mutable.SetTraceRootCauseSelectorAdvisories(advisories)
	}
	if err != nil && patch {
		return ctx.Mutable.TraceRootCauseReport(), err
	}
	return report, err
}

// traceRootCauseSelectorAdvisorySuffix renders the binder's selector
// disclosures onto an accepted tool summary (SIDECAR-NARR-1 复核: an
// optional carrier's dropped part is reported, never only logged).
func traceRootCauseSelectorAdvisorySuffix(ctx *types.BusContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	advisories := ctx.Mutable.TakeTraceRootCauseSelectorAdvisories()
	if len(advisories) == 0 {
		return ""
	}
	return " [trace_root_causes: " + strings.Join(advisories, "; ") + "]"
}

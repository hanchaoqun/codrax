package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitLogTriage is the log_triager agent's structured exit channel.
// The LLM reads the attached runtime log and emits a bundle carrying
// Layer 1 (Meta: lang + signals + summary), Layer 2 (Errors: recursive
// cause tree + frames), and Layer 3 (Observations + Residue:
// structured non-stack facts plus unknown_chunks). Layer 4
// (ResolvedFiles / Entities / IntentHint / Coverage) is system-derived
// by logtriage.ValidateBundle — the LLM has no channel to write it
// because those fields are not in the tool's JSON schema.
//
// Classified ReadOnly + NonEvidenceTool by the same logic as
// emit_analysis: the emission mutates BusContext, not the filesystem,
// and the payload is a classification / routing artefact, not a
// citable repo fact.
type EmitLogTriage struct {
	ReadOnly
	NonEvidenceTool
}

// emitLogTriageParams is the wire shape of the emit. It carries
// EXACTLY Layer 1-3; the validator adds Layer 4 after os.Stat / dedup.
// The LLM cannot fill Layer 4 because those field names are not in
// this struct and the JSON schema locks `additionalProperties: false`.
type emitLogTriageParams struct {
	Meta          emitLogTriageMeta          `json:"meta"`
	Errors        []emitLogTriageError       `json:"errors,omitempty"`
	Observations  []emitLogTriageObservation `json:"observations,omitempty"`
	UnknownChunks []string                   `json:"unknown_chunks,omitempty"`
	RawLogBytes   int                        `json:"-"` // filled by Execute from BusContext
	_             map[string]struct{}        `json:"-"`
}

type emitLogTriageMeta struct {
	Lang    string   `json:"lang"`
	Signals []string `json:"signals,omitempty"`
	Summary string   `json:"summary,omitempty"`
}

type emitLogTriageError struct {
	Type    string               `json:"type"`
	Message string               `json:"message,omitempty"`
	Frames  []emitLogTriageFrame `json:"frames,omitempty"`
	Cause   *emitLogTriageError  `json:"cause,omitempty"`
}

type emitLogTriageFrame struct {
	Lang       string  `json:"lang,omitempty"`
	File       string  `json:"file,omitempty"`
	Line       int     `json:"line,omitempty"`
	Func       string  `json:"func,omitempty"`
	Pkg        string  `json:"pkg,omitempty"`
	Raw        string  `json:"raw"`
	Confidence float64 `json:"confidence"`
}

type emitLogTriageObservation struct {
	Kind       string  `json:"kind"`
	Severity   string  `json:"severity,omitempty"`
	Subject    string  `json:"subject,omitempty"`
	Summary    string  `json:"summary"`
	Evidence   string  `json:"evidence,omitempty"`
	LineStart  int     `json:"line_start,omitempty"`
	LineEnd    int     `json:"line_end,omitempty"`
	Diagnostic bool    `json:"diagnostic"`
	Confidence float64 `json:"confidence"`
}

// Name returns the tool's stable identifier used by providers + the
// agent's emit-gate ShouldStop check.
func (t *EmitLogTriage) Name() string { return "emit_log_triage" }

// Description is one sentence: what the tool does and its one-call
// constraint. All strategy guidance — what to put in Signals, how
// deep to chase the Cause chain, when to punt to unknown_chunks —
// lives in the log-triage-skill system prompt, not here.
func (t *EmitLogTriage) Description() string {
	return "Emits the structured triage bundle extracted from the attached runtime log. " +
		"Call EXACTLY once per dispatch. The system validates paths against the " +
		"repository and derives the resolved-files list automatically — do not " +
		"try to resolve paths yourself."
}

// Parameters returns the strict JSON schema with additionalProperties:
// false on every object layer. The LLM cannot invent fields; the
// schema rejects any Layer 4 field name (resolved_files, entities,
// intent_hint, coverage) because they are not listed.
func (t *EmitLogTriage) Parameters() json.RawMessage {
	emitLogTriageSchemaOnce.Do(buildEmitLogTriageSchema)
	return emitLogTriageSchemaCache
}

var (
	emitLogTriageSchemaOnce  sync.Once
	emitLogTriageSchemaCache json.RawMessage
)

// buildEmitLogTriageSchema assembles the recursive schema. Go's encoding/json
// does not produce $ref-style JSON Schema natively, so the Error object
// is expanded up to a build-time depth of 5 (matching LogBundleCaps.
// MaxCauseDepth). The nested Cause at the deepest level omits its own
// cause property — the LLM receives a structural ceiling for free and
// the validator enforces the runtime cap. This mirrors how many
// published OpenAPI / function-calling specs handle recursive types
// (OpenAI function-calling in particular doesn't honour $ref well).
func buildEmitLogTriageSchema() {
	signalEnum := make([]string, 0, len(types.AllLogSignals()))
	for _, s := range types.AllLogSignals() {
		signalEnum = append(signalEnum, string(s))
	}
	observationKindEnum := make([]string, 0, len(types.AllLogObservationKinds()))
	for _, k := range types.AllLogObservationKinds() {
		observationKindEnum = append(observationKindEnum, string(k))
	}
	observationSeverityEnum := make([]string, 0, len(types.AllLogObservationSeverities()))
	for _, s := range types.AllLogObservationSeverities() {
		observationSeverityEnum = append(observationSeverityEnum, string(s))
	}

	// Language enum: canonical list for the Meta.Lang field. New
	// entries require coordinated changes in:
	//   - logtriage.ValidateBundle (validate.go switch)
	//   - the skill's user-facing prompt (see answer / log-triage
	//     skills for how to teach the LLM a new language)
	//
	// `arkts`   — HarmonyOS ArkTS (V8-style stack frames + .ets files)
	// `cangjie` — HarmonyOS Cangjie 1.0.0 LTS (JVM-like stack frames
	//             + .cj files; `at demo.cart.Cart.func(Cart.cj:42)`)
	// `kotlin`  — Kotlin on Android / JVM; frames look like
	//             `at com.example.Foo$bar(Foo.kt:42)` — same JVM
	//             shape as Java, different extension
	langEnum := []string{"go", "java", "cpp", "python", "node", "rust", "ruby", "swift", "lua", "proto", "csharp", "kotlin", "arkts", "cangjie", "unknown", "other"}

	frameSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"raw", "confidence"},
		"properties": map[string]any{
			"lang":       map[string]any{"type": "string"},
			"file":       map[string]any{"type": "string"},
			"line":       map[string]any{"type": "integer", "minimum": 0},
			"func":       map[string]any{"type": "string"},
			"pkg":        map[string]any{"type": "string"},
			"raw":        map[string]any{"type": "string", "minLength": 1, "maxLength": 300},
			"confidence": map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0},
		},
	}

	// errorSchemaAtDepth builds the Error object with a Cause pointer
	// that recurses until depth 0, at which point the Cause property
	// is dropped — the LLM cannot chain deeper than MaxCauseDepth and
	// the validator truncates anyway.
	var errorSchemaAtDepth func(depth int) map[string]any
	errorSchemaAtDepth = func(depth int) map[string]any {
		props := map[string]any{
			"type":    map[string]any{"type": "string", "maxLength": 80},
			"message": map[string]any{"type": "string", "maxLength": 500},
			"frames": map[string]any{
				"type":     "array",
				"maxItems": 30,
				"items":    frameSchema,
			},
		}
		if depth > 0 {
			props["cause"] = errorSchemaAtDepth(depth - 1)
		}
		return map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"type"},
			"properties":           props,
		}
	}

	errorSchema := errorSchemaAtDepth(types.LogBundleCaps.MaxCauseDepth - 1)
	observationSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"kind", "summary", "diagnostic", "confidence"},
		"properties": map[string]any{
			"kind": map[string]any{
				"type":        "string",
				"enum":        observationKindEnum,
				"description": "Coarse family for a non-stack operational observation.",
			},
			"severity": map[string]any{
				"type": "string",
				"enum": observationSeverityEnum,
			},
			"subject":    map[string]any{"type": "string", "maxLength": 120},
			"summary":    map[string]any{"type": "string", "minLength": 1, "maxLength": 240},
			"evidence":   map[string]any{"type": "string", "maxLength": 300},
			"line_start": map[string]any{"type": "integer", "minimum": 1},
			"line_end":   map[string]any{"type": "integer", "minimum": 1},
			"diagnostic": map[string]any{"type": "boolean"},
			"confidence": map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0},
		},
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"meta", "errors"},
		"properties": map[string]any{
			"meta": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"lang", "signals"},
				"properties": map[string]any{
					"lang": map[string]any{
						"type": "string",
						"enum": langEnum,
					},
					"signals": map[string]any{
						"type":     "array",
						"maxItems": 6,
						"items": map[string]any{
							"type": "string",
							"enum": signalEnum,
						},
					},
					"summary": map[string]any{"type": "string", "maxLength": 200},
				},
			},
			"errors": map[string]any{
				"type":     "array",
				"maxItems": 5,
				"items":    errorSchema,
			},
			"observations": map[string]any{
				"type":        "array",
				"maxItems":    types.LogBundleCaps.MaxObservations,
				"description": "Structured non-stack operational observations: retries, validator/reviewer failures, topical mismatch, line-mapping drift, state transitions, or similar runtime/process facts.",
				"items":       observationSchema,
			},
			"unknown_chunks": map[string]any{
				"type":     "array",
				"maxItems": 8,
				"items":    map[string]any{"type": "string", "maxLength": 500},
			},
		},
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		emitLogTriageSchemaCache = json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":"emit_log_triage schema build failed: %s"}`, err))
		return
	}
	emitLogTriageSchemaCache = raw
}

// Execute is the runtime quality gate: unmarshal → convert to
// ValidateInput → ValidateBundle → SetLogTriage. The ToolResult
// Summary carries post-validation counts so an operator can see at a
// glance how many frames were dropped as hallucinations.
func (t *EmitLogTriage) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_log_triage requires a writable context",
			Timestamp: time.Now(),
		}, nil
	}

	params = applyStructuredPayloadCompatWithLegacyStringFieldRepair(t.Name(), params, t.Parameters())

	var p emitLogTriageParams
	decoded := false
	if !decoded {
		if err := json.Unmarshal(params, &p); err != nil {
			if salvaged, fields, ok := salvageLogTriageStringWrappedArrays(params); ok {
				p = salvaged
				decoded = true
				logging.Info("[emit_log_triage] salvaged string-wrapped array fields from partially malformed payload: %v", fields)
			}
		}
	}
	if !decoded {
		if err := json.Unmarshal(params, &p); err != nil {
			return failStrictDecodeWithError(t.Name(), time.Now(), err, nil)
		}
	}

	// Cross-field sanity: at least one error, observation, or
	// unknown_chunk must be present. The schema declares errors as
	// required but allows an empty array; a triage where every content
	// channel is empty is meaningless.
	if len(p.Errors) == 0 && len(p.Observations) == 0 && len(p.UnknownChunks) == 0 {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_log_triage rejected: errors[], observations[], and unknown_chunks[] all empty — emit at least one structured fact or punt the input to unknown_chunks",
			Timestamp: time.Now(),
		}, nil
	}

	// Convert wire-shape to ValidateInput. The conversion walks the
	// full Cause chain so recursive errors keep their tree intact.
	in := logtriage.ValidateInput{
		Meta:         toValidateMeta(p.Meta),
		Errors:       toValidateErrors(p.Errors),
		Observations: toValidateObservations(p.Observations),
		Residue:      types.LogResidue{UnknownChunks: p.UnknownChunks},
		RawLogBytes:  len(ctx.AttachedLog),
	}

	bundle, clearedFrames := logtriage.ValidateBundleWithDenials(in, ctx.RepoRoot)
	if bundle == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_log_triage rejected: empty bundle after validation (no parseable errors, no residue)",
			Timestamp: time.Now(),
		}, nil
	}

	// P-TypedDenials A.3 (2026-05-08): every frame whose File the
	// corroborate gate cleared (real file did not contain the named
	// function) becomes a typed denial. Downstream consumers
	// (tool registry read_file gate / LLM prompt sanitiser /
	// answer validator) then refuse to ground or cite this path.
	// Closes the bypass where the LLM extracted the path from
	// frame.Raw (still verbatim) and called read_file with it.
	for _, cf := range clearedFrames {
		ctx.TypedDenials.Add(types.TypedDenial{
			Class:  types.TypedDenialExternalLogFrameUnresolved,
			Token:  cf.OriginalFile,
			Reason: fmt.Sprintf("frame func %q does not appear in %s", cf.Func, cf.OriginalFile),
		})
	}

	// Count pre-validation vs post-validation frame totals for the
	// Summary diff line.
	rawFrames := 0
	for _, e := range p.Errors {
		rawFrames += countFramesInEmitError(&e)
	}
	keptFrames := 0
	for _, e := range bundle.Errors {
		keptFrames += countFramesInBundleError(&e)
	}

	// P-BugClass Phase E.3 (2026-05-08): deterministic cross-language
	// bug-pattern detection on the raw attached log. Stamps bundle.
	// Meta.BugClasses with canonical (HumanZh / HumanEn) labels and
	// matched signature substrings. Downstream context.Builder
	// renders these as a "## Detected Failure Patterns" section in
	// the log_triage prompt, giving the LLM canonical terminology
	// up-front so it does not have to derive (and possibly invent)
	// it from raw panic text — closes the same hallucination class
	// the negative-knowledge channel (TypedDenials) guards against,
	// from the positive direction (give the right answer instead of
	// only deny the wrong ones).
	if bundle.Meta.BugClasses == nil {
		if detected := logtriage.DetectBugClasses(ctx.AttachedLog); len(detected) > 0 {
			bundle.Meta.BugClasses = detected
		}
	}

	ctx.Mutable.SetLogTriage(bundle)

	summary := fmt.Sprintf(
		"[emit_log_triage: lang=%s signals=%d errors=%d observations=%d frames=%d→%d resolved=%d entities=%d intent=%q coverage=%.2f evidence_origin=%s]\n"+
			"emit_log_triage recorded",
		bundle.Meta.Lang, len(bundle.Meta.Signals),
		len(bundle.Errors), len(bundle.Observations), rawFrames, keptFrames,
		len(bundle.ResolvedFiles), len(bundle.Entities),
		string(bundle.IntentHint), bundle.Coverage, string(types.AnswerEvidenceOriginRuntimeArtifact))

	logging.Info("[log_triage] validated: lang=%s errors=%d observations=%d frames_in=%d frames_kept=%d resolved=%d entities=%d intent=%q coverage=%.2f",
		bundle.Meta.Lang, len(bundle.Errors), len(bundle.Observations), rawFrames, keptFrames,
		len(bundle.ResolvedFiles), len(bundle.Entities),
		bundle.IntentHint, bundle.Coverage)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		Timestamp: time.Now(),
	}, nil
}

func salvageLogTriageStringWrappedArrays(raw json.RawMessage) (emitLogTriageParams, []string, bool) {
	var out emitLogTriageParams
	var fields []string
	if metaRaw, ok := extractTopLevelJSONValue(raw, "meta"); ok {
		_ = json.Unmarshal(metaRaw, &out.Meta)
	}
	if encoded, ok := extractTopLevelJSONStringField(raw, "errors"); ok {
		var errors []emitLogTriageError
		if decodeStringWrappedJSONArray(encoded, &errors) == nil {
			out.Errors = errors
			fields = append(fields, "errors")
		}
	}
	if encoded, ok := extractTopLevelJSONStringField(raw, "observations"); ok {
		var observations []emitLogTriageObservation
		if decodeStringWrappedJSONArray(encoded, &observations) == nil {
			out.Observations = observations
			fields = append(fields, "observations")
		}
	}
	if encoded, ok := extractTopLevelJSONStringField(raw, "unknown_chunks"); ok {
		var chunks []string
		if decodeStringWrappedJSONArray(encoded, &chunks) == nil {
			out.UnknownChunks = chunks
			fields = append(fields, "unknown_chunks")
		}
	}
	if len(fields) == 0 {
		return emitLogTriageParams{}, nil, false
	}
	return out, fields, true
}

func decodeStringWrappedJSONArray(encoded string, out any) error {
	encoded = strings.TrimSpace(encoded)
	if !strings.HasPrefix(encoded, "[") {
		return fmt.Errorf("not an array")
	}
	return json.Unmarshal([]byte(encoded), out)
}

func extractTopLevelJSONStringField(raw json.RawMessage, field string) (string, bool) {
	value, ok := extractTopLevelJSONValue(raw, field)
	if !ok || len(value) == 0 || value[0] != '"' {
		return "", false
	}
	var out string
	if err := json.Unmarshal(value, &out); err != nil {
		return "", false
	}
	return out, true
}

func extractTopLevelJSONValue(raw json.RawMessage, field string) (json.RawMessage, bool) {
	if len(raw) == 0 || field == "" {
		return nil, false
	}
	quotedField, _ := json.Marshal(field)
	searchFrom := 0
	for searchFrom < len(raw) {
		idx := bytes.Index(raw[searchFrom:], quotedField)
		if idx < 0 {
			return nil, false
		}
		pos := searchFrom + idx + len(quotedField)
		for pos < len(raw) && isJSONSpace(raw[pos]) {
			pos++
		}
		if pos >= len(raw) || raw[pos] != ':' {
			searchFrom = searchFrom + idx + len(quotedField)
			continue
		}
		pos++
		for pos < len(raw) && isJSONSpace(raw[pos]) {
			pos++
		}
		if pos >= len(raw) {
			return nil, false
		}
		dec := json.NewDecoder(bytes.NewReader(raw[pos:]))
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, false
		}
		return value, true
	}
	return nil, false
}

func isJSONSpace(b byte) bool {
	switch b {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}

// toValidateMeta converts wire-shape to validator input, dropping
// any signal outside the canonical enum (defence-in-depth; the
// schema's enum constraint catches them first).
func toValidateMeta(m emitLogTriageMeta) types.LogMeta {
	out := types.LogMeta{
		Lang:    m.Lang,
		Summary: m.Summary,
	}
	for _, s := range m.Signals {
		sig := types.LogSignal(s)
		if types.IsValidLogSignal(sig) {
			out.Signals = append(out.Signals, sig)
		}
	}
	return out
}

func toValidateErrors(in []emitLogTriageError) []types.LogError {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.LogError, len(in))
	for i, e := range in {
		out[i] = toValidateError(&e)
	}
	return out
}

func toValidateObservations(in []emitLogTriageObservation) []types.LogObservation {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.LogObservation, 0, len(in))
	for _, obs := range in {
		kind := types.LogObservationKind(obs.Kind)
		if !types.IsValidLogObservationKind(kind) {
			continue
		}
		severity := types.LogObservationSeverity(obs.Severity)
		if severity != "" && !types.IsValidLogObservationSeverity(severity) {
			severity = ""
		}
		lineStart, lineEnd := obs.LineStart, obs.LineEnd
		if lineStart <= 0 {
			lineStart, lineEnd = 0, 0
		} else if lineEnd > 0 && lineEnd < lineStart {
			lineEnd = lineStart
		}
		out = append(out, types.LogObservation{
			Kind:       kind,
			Severity:   severity,
			Subject:    obs.Subject,
			Summary:    obs.Summary,
			Evidence:   obs.Evidence,
			LineStart:  lineStart,
			LineEnd:    lineEnd,
			Diagnostic: obs.Diagnostic,
			Confidence: obs.Confidence,
		})
	}
	return out
}

func toValidateError(e *emitLogTriageError) types.LogError {
	if e == nil {
		return types.LogError{}
	}
	out := types.LogError{
		Type:    e.Type,
		Message: e.Message,
	}
	if len(e.Frames) > 0 {
		out.Frames = make([]types.LogFrame, len(e.Frames))
		for j, f := range e.Frames {
			out.Frames[j] = types.LogFrame{
				Lang:       f.Lang,
				File:       f.File,
				Line:       f.Line,
				Func:       f.Func,
				Pkg:        f.Pkg,
				Raw:        f.Raw,
				Confidence: f.Confidence,
			}
		}
	}
	if e.Cause != nil {
		c := toValidateError(e.Cause)
		out.Cause = &c
	}
	return out
}

func countFramesInEmitError(e *emitLogTriageError) int {
	if e == nil {
		return 0
	}
	n := len(e.Frames)
	if e.Cause != nil {
		n += countFramesInEmitError(e.Cause)
	}
	return n
}

// countFramesInBundleError counts only RESOLVED frames — ones the
// validator kept File+Line on. That's what the operator reading the
// Summary banner cares about: "how many did we resolve against the
// repo, vs how many did the LLM emit". Raw-only frames (File cleared
// because path hallucinated) don't count as resolved.
func countFramesInBundleError(e *types.LogError) int {
	if e == nil {
		return 0
	}
	n := 0
	for _, f := range e.Frames {
		if f.File != "" && f.Line > 0 {
			n++
		}
	}
	if e.Cause != nil {
		n += countFramesInBundleError(e.Cause)
	}
	return n
}

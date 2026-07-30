package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func decodeStrictToolParams(name string, raw json.RawMessage, schema json.RawMessage, dst any, hints []MisplacedFieldHint) (json.RawMessage, *types.ToolResult, error) {
	normalized := applyStructuredPayloadCompat(name, raw, schema)
	dec := json.NewDecoder(strings.NewReader(string(normalized)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		// LT-HYG decoder-remap hint (§29.75 立案, 2026-07-14): the schema is
		// in hand on this entry — fabricated-field rejections teach the real
		// top-level parameter list (reflected, never hand-copied).
		res, retErr := failStrictDecodeWithErrorSchema(name, time.Now(), err, hints, normalized, schema)
		appendStrictDecodeUnknownFieldCensus(&res, &retErr, err, normalized, dst, hints)
		return normalized, &res, retErr
	}
	return normalized, nil, nil
}

func decodeStrictNormalizedToolParams(name string, normalized json.RawMessage, dst any, hints []MisplacedFieldHint) (json.RawMessage, *types.ToolResult, error) {
	dec := json.NewDecoder(strings.NewReader(string(normalized)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		res, retErr := failStrictDecodeWithError(name, time.Now(), err, hints, normalized)
		appendStrictDecodeUnknownFieldCensus(&res, &retErr, err, normalized, dst, hints)
		return normalized, &res, retErr
	}
	return normalized, nil, nil
}

// appendStrictDecodeUnknownFieldCensus — EMITBURN-2 (NG-1, §13.4): Go's
// DisallowUnknownFields aborts on the FIRST unknown key with no path, no
// count, no roster, so a fabricated field living in two containers and
// several array items burned one retry round per occurrence (the no_touying
// fourth-replay 2m51s form). On the unknown-field class, one reject now
// enumerates EVERY unknown key with its JSON path. Report layer only: the
// verdict and the single-field message stay byte-identical (the roster note
// is appended only when there is more than one occurrence to teach).
//
// R6-1 (round-6 sweep, eval/sweep_round6_findings_20260730.md): when a
// MisplacedFieldHint APPLIES to the rejected field (same top-level-gated
// predicate as the remap/repair consumers — strictDecodeHintFor), the
// message already carries THE imperative for it (rename / relocate); the
// census's blanket "remove every one of them" contradicted it on the exact
// lane EVALFIX-2A was built to fix. One imperative per failure: the hinted
// field leaves the roster, and any REMAINING unknowns keep an imperative
// of their own that no longer speaks about the hinted field. Un-hinted
// rejects keep the pre-fix wording byte-identical.
func appendStrictDecodeUnknownFieldCensus(res *types.ToolResult, retErr *error, err error, normalized json.RawMessage, dst any, hints []MisplacedFieldHint) {
	field := extractUnknownFieldName(err)
	if field == "" {
		appendStrictDecodeStringifiedObjectHint(res, retErr, err)
		return
	}
	census := strictDecodeUnknownFieldCensus(normalized, dst)
	// GAP-EVAL-R1 (audit 2026-07-26): a SINGLE unknown key used to return
	// silently here — the analyzer's required_/requested_ near-miss burned a
	// full retry round with no guidance. A lone unknown key now teaches too,
	// but ONLY when it carries a did-you-mean suggestion (the no-suggestion
	// single-unknown message stays byte-identical to the pre-census form).
	if len(census) == 0 {
		return
	}
	if len(census) == 1 && !strings.Contains(census[0], "did you mean") {
		return
	}
	note := fmt.Sprintf("; all unknown fields in this payload (remove every one of them in a single retry): %s",
		strings.Join(census, ", "))
	if strictDecodeHintFor(hints, field, normalized) != nil {
		rest := strictDecodeCensusWithoutTopLevelField(census, field)
		if len(rest) == 0 {
			return
		}
		note = fmt.Sprintf("; the payload also carries these additional unknown fields — fix them in the same retry: %s",
			strings.Join(rest, ", "))
	}
	res.Summary += note
	if *retErr != nil {
		*retErr = fmt.Errorf("%w%s", *retErr, note)
	}
}

// strictDecodeCensusWithoutTopLevelField drops the roster entry whose
// JSON path is exactly the hinted top-level field (its instruction is
// the rename/relocate above). Nested occurrences of the same spelling
// keep their rows — their did-you-mean speaks about their own container
// (R6-2).
func strictDecodeCensusWithoutTopLevelField(census []string, field string) []string {
	out := make([]string, 0, len(census))
	for _, entry := range census {
		path := entry
		if i := strings.Index(path, " (did you mean"); i >= 0 {
			path = path[:i]
		}
		if path == field {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// appendStrictDecodeStringifiedObjectHint — GAP-EVAL-R1 second half: the
// other burn shape is a nested object emitted as a JSON-ENCODED STRING
// ("field": "{\"a\":1}"). Go reports it as a type mismatch with the field
// path in hand; the reject now teaches the mechanical fix instead of leaving
// the model to guess. Soft guidance only — the verdict stays the reject.
func appendStrictDecodeStringifiedObjectHint(res *types.ToolResult, retErr *error, err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if !strings.Contains(msg, "cannot unmarshal string into Go struct field") {
		return
	}
	if !strings.Contains(msg, "of type") {
		return
	}
	// Only object/array-shaped targets get the note (a plain scalar type
	// mismatch is a different lesson).
	tail := msg[strings.LastIndex(msg, "of type ")+len("of type "):]
	tail = strings.TrimSpace(tail)
	if !strings.HasPrefix(tail, "types.") && !strings.HasPrefix(tail, "[]") && !strings.HasPrefix(tail, "map[") && !strings.Contains(tail, "struct") {
		return
	}
	note := "; the value looks like a JSON-ENCODED STRING — emit the nested object/array directly as JSON (no quoting, no escaping) in the retry"
	res.Summary += note
	if *retErr != nil {
		*retErr = fmt.Errorf("%w%s", *retErr, note)
	}
}

// strictDecodeUnknownFieldCensus walks the payload against dst's reflected
// json-tag tree (the schema IS the struct — never hand-copied) and returns
// every unknown key as a JSON path, deterministically ordered. Any parse or
// shape irregularity fails open to nil: the roster is an additive teaching
// note, never a gate.
func strictDecodeUnknownFieldCensus(normalized json.RawMessage, dst any) []string {
	var payload any
	if json.Unmarshal(normalized, &payload) != nil {
		return nil
	}
	var out []string
	strictDecodeCensusWalk(payload, reflect.TypeOf(dst), "", &out)
	const rosterCap = 12
	if len(out) > rosterCap {
		out = append(out[:rosterCap], fmt.Sprintf("(+%d more)", len(out)-rosterCap))
	}
	return out
}

var strictDecodeRawMessageType = reflect.TypeOf(json.RawMessage{})

func strictDecodeCensusWalk(node any, t reflect.Type, path string, out *[]string) {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t == strictDecodeRawMessageType || t.Kind() == reflect.Interface {
		return
	}
	switch value := node.(type) {
	case map[string]any:
		switch t.Kind() {
		case reflect.Struct:
			fields := strictDecodeStructJSONFields(t)
			keys := make([]string, 0, len(value))
			for key := range value {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				childPath := key
				if path != "" {
					childPath = path + "." + key
				}
				fieldType, ok := strictDecodeLookupJSONField(fields, key)
				if !ok {
					// GAP-EVAL-R1: bounded near-miss suggestion against the
					// SAME reflected schema level (soft guidance — the roster
					// is a teaching note, never a gate).
					if suggestion := strictDecodeNearestFieldSuggestion(fields, key); suggestion != "" {
						childPath += fmt.Sprintf(" (did you mean %q?)", suggestion)
					}
					*out = append(*out, childPath)
					continue
				}
				strictDecodeCensusWalk(value[key], fieldType, childPath, out)
			}
		case reflect.Map:
			keys := make([]string, 0, len(value))
			for key := range value {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				childPath := key
				if path != "" {
					childPath = path + "." + key
				}
				strictDecodeCensusWalk(value[key], t.Elem(), childPath, out)
			}
		}
	case []any:
		if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
			for i, child := range value {
				strictDecodeCensusWalk(child, t.Elem(), fmt.Sprintf("%s[%d]", path, i), out)
			}
		}
	}
}

// strictDecodeStructJSONFields maps json tag names (embedded structs
// promoted) to field types, mirroring encoding/json's naming rules.
func strictDecodeStructJSONFields(t reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	var walk func(reflect.Type)
	walk = func(structType reflect.Type) {
		for i := 0; i < structType.NumField(); i++ {
			field := structType.Field(i)
			if field.Anonymous {
				embedded := field.Type
				for embedded.Kind() == reflect.Pointer {
					embedded = embedded.Elem()
				}
				if embedded.Kind() == reflect.Struct {
					walk(embedded)
					continue
				}
			}
			if !field.IsExported() {
				continue
			}
			tag := strings.Split(field.Tag.Get("json"), ",")[0]
			if tag == "-" {
				continue
			}
			if tag == "" {
				tag = field.Name
			}
			if _, exists := fields[tag]; !exists {
				fields[tag] = field.Type
			}
		}
	}
	walk(t)
	return fields
}

// strictDecodeLookupJSONField mirrors encoding/json's exact-then-fold match.
func strictDecodeLookupJSONField(fields map[string]reflect.Type, key string) (reflect.Type, bool) {
	if fieldType, ok := fields[key]; ok {
		return fieldType, true
	}
	for tag, fieldType := range fields {
		if strings.EqualFold(tag, key) {
			return fieldType, true
		}
	}
	return nil, false
}

// strictDecodeNearestFieldSuggestion — GAP-EVAL-R1: the nearest schema field
// within a bounded edit distance (≤3, and under a quarter of the key length
// so short keys never false-suggest). Deterministic: ties break by the
// lexicographically first candidate. Noisy signal, soft consumer only.
func strictDecodeNearestFieldSuggestion(fields map[string]reflect.Type, key string) string {
	const maxDistance = 3
	if len(key) < 6 {
		return ""
	}
	best, bestDistance := "", maxDistance+1
	for name := range fields {
		if name == "" || name == key {
			continue
		}
		limit := maxDistance
		if quarter := len(key) / 4; quarter < limit {
			limit = quarter
		}
		if d := strictDecodeBoundedEditDistance(key, name, limit); d >= 0 &&
			(d < bestDistance || (d == bestDistance && name < best)) {
			best, bestDistance = name, d
		}
	}
	return best
}

// strictDecodeBoundedEditDistance returns the Levenshtein distance of a and
// b when it is ≤ limit, else -1 (early-exit banded computation).
func strictDecodeBoundedEditDistance(a, b string, limit int) int {
	if limit < 0 {
		return -1
	}
	la, lb := len(a), len(b)
	if la-lb > limit || lb-la > limit {
		return -1
	}
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			m := prev[j] + 1
			if cur[j-1]+1 < m {
				m = cur[j-1] + 1
			}
			if prev[j-1]+cost < m {
				m = prev[j-1] + cost
			}
			cur[j] = m
			if m < rowMin {
				rowMin = m
			}
		}
		if rowMin > limit {
			return -1
		}
		prev, cur = cur, prev
	}
	if prev[lb] > limit {
		return -1
	}
	return prev[lb]
}

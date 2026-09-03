package tracequery

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
)

// SourceRawVisibilityWire is the converter-owned visibility carrier's exact
// wire token: the first body token of every carrier row. The converter
// (internal/hitraceconv) reads it from here so the emission side and the
// parser side can never drift apart.
const SourceRawVisibilityWire = "codrax_source_raw_visibility/v1"

// SourceRawVisibilityEventName is the ONE reserved ftrace header name every
// visibility carrier row is published under (colleague_merge_audit §40.13,
// V6-2). A carrier is a metadata row, not a semantic row: publishing it under
// the wrapped record's original name (sched_migrate_task, irq_handler_entry,
// cpu_frequency, ...) made every header-name-keyed consumer — most damagingly
// the pre-parse integrity prefilters that run BEFORE the payload grammar can
// classify the row — audit it as a malformed semantic row. The reserved name
// isolates the namespace once at the producer; the original name stays
// losslessly recoverable from the event_name_b64 token and the schema payload.
// The parser itself remains name-agnostic (legacy artifacts still parse), so
// this constant is a producer census contract, not a parser gate.
const SourceRawVisibilityEventName = "codrax_source_raw_event"

const sourceRawVisibilityWire = SourceRawVisibilityWire

// SourceRawVisibilityEventNameMaxBytes caps the decoded original event name a
// carrier may wrap. The converter reads the cap from here so the emitter
// fails a longer name closed with the same bound the parser applies.
const SourceRawVisibilityEventNameMaxBytes = 128

// sourceRawVisibilityOnlyPayload recognizes the converter-owned carrier by
// its complete canonical token grammar. This is a precise wire discriminator,
// not a keyword/prose heuristic. Invalid or extended lookalikes remain normal
// source rows and receive no advisory classification.
func sourceRawVisibilityOnlyPayload(fields string) bool {
	tokens := strings.Fields(fields)
	if len(tokens) != 6 && len(tokens) != 7 {
		return false
	}
	if tokens[0] != sourceRawVisibilityWire || tokens[1] != "semantic_authority=none" {
		return false
	}
	formatID, ok := sourceRawVisibilityToken(tokens[2], "format_id")
	if !ok {
		return false
	}
	id, err := strconv.ParseUint(formatID, 10, 16)
	if err != nil || id == 0 {
		return false
	}
	eventName, ok := sourceRawVisibilityToken(tokens[3], "event_name_b64")
	if !ok {
		return false
	}
	decodedName, err := base64.RawURLEncoding.DecodeString(eventName)
	if err != nil || len(decodedName) == 0 || len(decodedName) > SourceRawVisibilityEventNameMaxBytes ||
		base64.RawURLEncoding.EncodeToString(decodedName) != eventName {
		return false
	}
	schemaDigest, ok := sourceRawVisibilityToken(tokens[4], "schema_sha256")
	if !ok || len(schemaDigest) != sha256.Size*2 || strings.ToLower(schemaDigest) != schemaDigest {
		return false
	}
	if _, err := hex.DecodeString(schemaDigest); err != nil {
		return false
	}
	payload, ok := sourceRawVisibilityToken(tokens[5], "payload_b64")
	if !ok {
		return false
	}
	decodedPayload, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || len(decodedPayload) < 2 ||
		base64.RawURLEncoding.EncodeToString(decodedPayload) != payload {
		return false
	}
	if len(tokens) == 6 {
		return true
	}
	schema, ok := sourceRawVisibilityToken(tokens[6], "schema_b64")
	if !ok {
		return false
	}
	decodedSchema, err := base64.RawURLEncoding.DecodeString(schema)
	if err != nil || len(decodedSchema) == 0 ||
		base64.RawURLEncoding.EncodeToString(decodedSchema) != schema {
		return false
	}
	digest := sha256.Sum256(decodedSchema)
	return hex.EncodeToString(digest[:]) == schemaDigest
}

func sourceRawVisibilityPayloadClaimed(fields string) bool {
	return fields == sourceRawVisibilityWire || strings.HasPrefix(fields, sourceRawVisibilityWire+" ")
}

func sourceRawVisibilityToken(token, key string) (string, bool) {
	prefix := key + "="
	if !strings.HasPrefix(token, prefix) || len(token) == len(prefix) {
		return "", false
	}
	return token[len(prefix):], true
}

func sourceRawVisibilityAdvisory(ev Event) bool {
	return ev.Type == EventSourceRawVisibility
}

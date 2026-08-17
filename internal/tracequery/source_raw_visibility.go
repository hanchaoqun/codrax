package tracequery

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
)

const sourceRawVisibilityWire = "codrax_source_raw_visibility/v1"

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
	if err != nil || len(decodedName) == 0 || len(decodedName) > 128 ||
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

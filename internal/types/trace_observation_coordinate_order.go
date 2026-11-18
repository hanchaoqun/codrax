package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// traceQueryPublicationCoordinatePrefix opens a trace_query observation
// coordinate minted from a complete query receipt:
//
//	trace_query:publication-<sha256 hex>#<kind>[:<ordinal>...]
//
// The digest hashes the whole source receipt — the capture's absolute path,
// the stored-result location and the query — so that two publications can
// never share a lookup key. That makes it an identity and nothing else: its
// lexical position changes when the same capture bytes are read from another
// directory, or when the session keeps its results somewhere else.
const traceQueryPublicationCoordinatePrefix = "trace_query:publication-"

// TraceObservationCoordinateOrderKey returns the part of an observation
// coordinate that may decide an order or break a tie between observations.
//
// For the exact full-digest publication grammar above only the producer's own
// in-publication suffix (kind and ordinal) is returned: it is the one part of
// the coordinate that depends on the capture content and the query alone.
// Every other coordinate — legacy result-scoped ids, other producers, a
// malformed digest — is returned unchanged, so their existing order is kept
// byte for byte.
//
// The key is an ordering input only. It is never an identity: two
// publications of one query shape share it by construction, and callers must
// keep comparing the full coordinate wherever they ask "is this the same
// observation".
func TraceObservationCoordinateOrderKey(id string) string {
	coordinate, versioned := strings.CutPrefix(id, traceQueryPublicationCoordinatePrefix)
	if !versioned {
		return id
	}
	digest, suffix, separated := strings.Cut(coordinate, "#")
	if !separated || suffix == "" || len(digest) != 2*sha256.Size {
		return id
	}
	if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != sha256.Size {
		return id
	}
	return "trace_query:publication#" + suffix
}

// TraceObservationPublicationOrder gives every receipt-minted publication the
// ordinal of its first appearance in one ordered record set, for lists whose
// PRIMARY order is the observation coordinate (so that one publication's rows
// stay together, as they did under the raw-coordinate order). SortKey replaces
// the digest with that ordinal; the in-publication suffix order and the whole
// order of every other coordinate are unchanged. The zero value is ready.
//
// Like the order key it is an ordering input only, valid inside the record
// set it was built from.
type TraceObservationPublicationOrder struct {
	ordinals map[string]int
}

// SortKey must be called once per record in the record set's own order.
func (o *TraceObservationPublicationOrder) SortKey(id string) string {
	id = strings.TrimSpace(id)
	if TraceObservationCoordinateOrderKey(id) == id {
		return id
	}
	publication, suffix, _ := strings.Cut(id, "#")
	if o.ordinals == nil {
		o.ordinals = map[string]int{}
	}
	ordinal, seen := o.ordinals[publication]
	if !seen {
		ordinal = len(o.ordinals) + 1
		o.ordinals[publication] = ordinal
	}
	return traceQueryPublicationCoordinatePrefix + fmt.Sprintf("%08d", ordinal) + "#" + suffix
}

// TraceObservationCoordinateLess orders two observation coordinates by their
// order keys. Coordinates with equal keys are left unordered (false both
// ways): callers sort stably, so observations that differ only in which
// publication carried them keep the order in which they were published.
func TraceObservationCoordinateLess(a, b string) bool {
	return TraceObservationCoordinateOrderKey(a) < TraceObservationCoordinateOrderKey(b)
}

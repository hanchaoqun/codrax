package hitraceconv

import (
	"context"
	"fmt"
	"math"
)

type traceDBActivityITIDProfile uint8

const (
	traceDBActivityITIDUnsupported traceDBActivityITIDProfile = iota
	traceDBActivityITIDCanonical
	traceDBActivityITIDSignedInt32
)

// traceDBBoundedSQLiteIntegerTransport consumes the closed typeof() verdict
// that guarded a SQL CASE projection. Non-INTEGER cells never cross the driver
// boundary as their potentially unbounded TEXT/BLOB payload; nil is only a
// rejection token for Go admission and is never accepted as a default value.
func traceDBBoundedSQLiteIntegerTransport(typeRaw, valueRaw any) any {
	typeName, ok := typeRaw.(string)
	if !ok || typeName != "integer" {
		return nil
	}
	return valueRaw
}

func (profile traceDBActivityITIDProfile) decode(value any) (int64, bool) {
	switch profile {
	case traceDBActivityITIDCanonical:
		return traceDBStrictInternalID(value)
	case traceDBActivityITIDSignedInt32:
		return traceDBStrictSignedInt32InternalID(value)
	default:
		return 0, false
	}
}

// decodeStableRowID decodes a producer row identity without borrowing the
// INVALID_UINT32 sentinel rule used by internal identities. In particular,
// current frame_slice/native_hook can expose the valid uint32 row id
// 0xffffffff as SQLite INTEGER -1 after their audited int32 projection.
func (profile traceDBActivityITIDProfile) decodeStableRowID(value any) (int64, bool) {
	raw, ok := traceDBStrictSQLiteInt(value)
	if !ok {
		return 0, false
	}
	switch profile {
	case traceDBActivityITIDCanonical:
		return raw, raw >= 0 && raw <= math.MaxUint32
	case traceDBActivityITIDSignedInt32:
		if raw < math.MinInt32 || raw > math.MaxInt32 {
			return 0, false
		}
		if raw < 0 {
			return raw + (int64(1) << 32), true
		}
		return raw, true
	default:
		return 0, false
	}
}

func (profile traceDBActivityITIDProfile) provenance() string {
	switch profile {
	case traceDBActivityITIDCanonical:
		return "strict canonical internal uint32 in 0..UINT32_MAX-1"
	case traceDBActivityITIDSignedInt32:
		return "strict signed-int32 projection to internal uint32; -1 sentinel and positive high-half encodings rejected"
	default:
		return "unsupported schema profile"
	}
}

// traceDBActivityProfile selects the producer-specific wire decoder. It does
// not inspect row values and never falls back per row: a malformed schema
// cannot gain authority by choosing whichever interpretation accepts a value.
func traceDBActivityProfile(ctx context.Context, queryer traceDBQueryer, table string) (traceDBActivityITIDProfile, string, error) {
	switch table {
	case "sched_slice", "thread_state", "native_hook":
		return traceDBActivityITIDCanonical, "canonical producer profile", nil
	case "syscall":
		return traceDBActivityITIDSignedInt32, "current syscall.itid signed-int32 producer profile", nil
	case "frame_slice":
		columns, err := traceDBColumnNames(ctx, queryer, table)
		if err != nil {
			return traceDBActivityITIDUnsupported, "", err
		}
		hasID, hasType := false, false
		for _, column := range columns {
			hasID = hasID || sqliteASCIIIdentifierEqual(column, "id")
			hasType = hasType || sqliteASCIIIdentifierEqual(column, "type")
		}
		switch {
		case hasID && hasType:
			return traceDBActivityITIDSignedInt32, "current frame_slice id+type signed-int32 producer profile", nil
		case !hasID && !hasType:
			return traceDBActivityITIDCanonical, "legacy frame_slice no-id/no-type canonical compatibility profile", nil
		default:
			return traceDBActivityITIDUnsupported,
				fmt.Sprintf("unsupported frame_slice schema profile: id_present=%t type_present=%t", hasID, hasType), nil
		}
	default:
		return traceDBActivityITIDUnsupported, "unsupported activity table", nil
	}
}

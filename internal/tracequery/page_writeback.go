package tracequery

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// pageCacheMutationKind is the one typed authority consumed by both
// isPageCacheEvent and accumulatePageCache.  Event.Name, MemoryKind,
// SubsystemKind and prose are observation signals only and must never mint a
// mutation.
type pageCacheMutationKind uint8

const (
	pageCacheMutationNone pageCacheMutationKind = iota
	pageCacheMutationAdd
	pageCacheMutationDelete
)

const (
	pageCacheAddEventName     = "mm_filemap_add_to_page_cache"
	pageCacheDeleteEventName  = "mm_filemap_delete_from_page_cache"
	writebackSetEventName     = "filemap_set_wb_err"
	writebackAdvanceEventName = "file_check_and_advance_wb_err"
)

type pageCacheMutationPayload struct {
	kind   pageCacheMutationKind
	dev    string
	inode  string
	offset int64
}

// parsePageCacheMutationPayload admits only the three source-pinned textual
// profiles:
//
//   - Linux/OpenHarmony 5.10: optional page= pointer, no order;
//   - Codrax canonical: no page and no order;
//   - Linux 6.6: no page, optional order=0..255.
//
// The event name is byte-exact.  The complete tuple is parsed before any
// typed projection is published; an exact-but-malformed row therefore remains
// searchable inventory while carrying pageCacheMutationNone and no partial
// dev/inode/offset values.
func parsePageCacheMutationPayload(name, fields string) (pageCacheMutationPayload, bool) {
	kind := pageCacheMutationNone
	switch name {
	case pageCacheAddEventName:
		kind = pageCacheMutationAdd
	case pageCacheDeleteEventName:
		kind = pageCacheMutationDelete
	default:
		return pageCacheMutationPayload{}, false
	}

	tokens := strings.Fields(fields)
	if len(tokens) < 6 || tokens[0] != "dev" || tokens[2] != "ino" {
		return pageCacheMutationPayload{}, false
	}
	dev, ok := canonicalPageDevice(tokens[1])
	if !ok {
		return pageCacheMutationPayload{}, false
	}
	inodeValue, ok := parseCanonicalOptionalHex(tokens[3], math.MaxUint64)
	if !ok {
		return pageCacheMutationPayload{}, false
	}

	position := 4
	pagePresent := false
	if position < len(tokens) && strings.HasPrefix(tokens[position], "page=") {
		pagePresent = true
		if !validCanonicalPagePointer(strings.TrimPrefix(tokens[position], "page=")) {
			return pageCacheMutationPayload{}, false
		}
		position++
	}
	if position+2 > len(tokens) {
		return pageCacheMutationPayload{}, false
	}
	pfnRaw, ok := exactTokenValue(tokens[position], "pfn")
	if !ok {
		return pageCacheMutationPayload{}, false
	}
	if _, ok := parseCanonicalDecimal(pfnRaw, math.MaxUint64); !ok {
		if _, ok := parseCanonicalPrefixedHex(pfnRaw, math.MaxUint64); !ok {
			return pageCacheMutationPayload{}, false
		}
	}
	position++
	offsetRaw, ok := exactTokenValue(tokens[position], "ofs")
	if !ok {
		return pageCacheMutationPayload{}, false
	}
	offset, ok := parseCanonicalDecimal(offsetRaw, math.MaxInt64)
	if !ok || offset%4096 != 0 {
		return pageCacheMutationPayload{}, false
	}
	position++

	if position < len(tokens) {
		// page= is the 5.10 formatter-only shape; order= belongs to the 6.6
		// shape whose producer does not print page.  Accepting both would
		// fabricate a fourth, unattested profile.
		if pagePresent || position+1 != len(tokens) {
			return pageCacheMutationPayload{}, false
		}
		orderRaw, ok := exactTokenValue(tokens[position], "order")
		if !ok {
			return pageCacheMutationPayload{}, false
		}
		if _, ok := parseCanonicalDecimal(orderRaw, math.MaxUint8); !ok {
			return pageCacheMutationPayload{}, false
		}
		position++
	}
	if position != len(tokens) {
		return pageCacheMutationPayload{}, false
	}

	return pageCacheMutationPayload{
		kind:   kind,
		dev:    dev,
		inode:  fmt.Sprintf("0x%x", inodeValue),
		offset: int64(offset),
	}, true
}

func exactTokenValue(token, key string) (string, bool) {
	prefix := key + "="
	if !strings.HasPrefix(token, prefix) || len(token) == len(prefix) {
		return "", false
	}
	return token[len(prefix):], true
}

func parseCanonicalDecimal(raw string, max uint64) (uint64, bool) {
	if raw == "" {
		return 0, false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, false
		}
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value > max || strconv.FormatUint(value, 10) != raw {
		return 0, false
	}
	return value, true
}

func parseCanonicalPrefixedHex(raw string, max uint64) (uint64, bool) {
	if len(raw) <= 2 || !strings.HasPrefix(raw, "0x") {
		return 0, false
	}
	digits := raw[2:]
	for i := 0; i < len(digits); i++ {
		c := digits[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return 0, false
		}
	}
	value, err := strconv.ParseUint(digits, 16, 64)
	if err != nil || value > max {
		return 0, false
	}
	return value, true
}

func parseCanonicalOptionalHex(raw string, max uint64) (uint64, bool) {
	if strings.HasPrefix(raw, "0x") {
		return parseCanonicalPrefixedHex(raw, max)
	}
	if raw == "" {
		return 0, false
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return 0, false
		}
	}
	value, err := strconv.ParseUint(raw, 16, 64)
	if err != nil || value > max {
		return 0, false
	}
	return value, true
}

func validCanonicalPagePointer(raw string) bool {
	// Two explicit legacy converter spellings remain input-compatible.  They
	// are observations only (the page pointer is never projected), but keeping
	// them exact avoids widening the authoritative kernel %p profile.
	if raw == "0" || raw == "0x0" || raw == "(____ptrval____)" {
		return true
	}
	digits := raw
	if strings.HasPrefix(digits, "0x") {
		digits = digits[2:]
	}
	// Kernel %p is fixed-width: two hex digits per pointer byte.  Admit only
	// the 32-bit and 64-bit shapes; arbitrary short/long hex strings are not a
	// source-pinned page profile and must not authorize a mutation.
	if len(digits) != 8 && len(digits) != 16 {
		return false
	}
	_, ok := parseCanonicalOptionalHex(raw, math.MaxUint64)
	return ok
}

// parseCanonicalPointer admits the exact kernel %p text families used by the
// source-pinned profiles: optional-0x hexadecimal (including fixed-width
// hashed/redacted output) and the kernel's pre-hash sentinel.  numeric is
// false only for that sentinel, whose actual pointer value is intentionally
// unavailable.
func parseCanonicalPointer(raw string) (value uint64, numeric, ok bool) {
	if raw == "(____ptrval____)" {
		return 0, false, true
	}
	if raw == "" {
		return 0, false, false
	}
	// Kernel %p output is commonly a fixed-width hexadecimal token without
	// 0x (including all-zero redaction); the legacy page=0 spelling remains a
	// valid input observation.  It is validated but never projected.
	value, ok = parseCanonicalOptionalHex(raw, math.MaxUint64)
	if !ok {
		return 0, false, false
	}
	return value, true, true
}

func canonicalPageDevice(raw string) (string, bool) {
	majorRaw, minorRaw, ok := strings.Cut(raw, ":")
	if !ok || strings.Contains(minorRaw, ":") {
		return "", false
	}
	major, majorOK := parseCanonicalDecimal(majorRaw, 0xfff)
	minor, minorOK := parseCanonicalDecimal(minorRaw, 0xfffff)
	if !majorOK || !minorOK {
		return "", false
	}
	return fmt.Sprintf("%d:%d", major, minor), true
}

func pageCacheMutationKindForEvent(ev Event) pageCacheMutationKind {
	if ev.FileFields == nil {
		return pageCacheMutationNone
	}
	return ev.FileFields.pageCacheMutation
}

func exactWritebackObservationName(name string) bool {
	return name == writebackSetEventName || name == writebackAdvanceEventName
}

func isWritebackObservation(ev Event) bool {
	return ev.Type == EventFilesystem && exactWritebackObservationName(ev.Name)
}

// populateWritebackObservationFields is a dedicated projection.  It validates
// the full canonical producer payload but publishes only dev/inode; file is a
// kernel pointer, not a path or directory entry, and errseq/old/new are
// observation scalars rather than byte, offset, latency or pairing identity.
func populateWritebackObservationFields(ev *Event, fields string, intern *stringInterner) {
	if ev == nil || ev.FileFields == nil || !exactWritebackObservationName(ev.Name) {
		return
	}
	var devRaw, inodeRaw string
	switch ev.Name {
	case writebackSetEventName:
		tokens := strings.Fields(fields)
		if len(tokens) != 3 {
			return
		}
		var ok bool
		if devRaw, ok = exactTokenValue(tokens[0], "dev"); !ok {
			return
		}
		if inodeRaw, ok = exactTokenValue(tokens[1], "ino"); !ok {
			return
		}
		errseqRaw, ok := exactTokenValue(tokens[2], "errseq")
		if !ok {
			return
		}
		if _, ok := parseCanonicalPrefixedHex(errseqRaw, math.MaxUint32); !ok {
			return
		}
	case writebackAdvanceEventName:
		tokens := strings.Fields(fields)
		if len(tokens) != 5 {
			return
		}
		fileRaw, ok := exactTokenValue(tokens[0], "file")
		if !ok {
			return
		}
		file, numeric, ok := parseCanonicalPointer(fileRaw)
		if !ok || (numeric && file == 0) {
			return
		}
		if devRaw, ok = exactTokenValue(tokens[1], "dev"); !ok {
			return
		}
		if inodeRaw, ok = exactTokenValue(tokens[2], "ino"); !ok {
			return
		}
		oldRaw, ok := exactTokenValue(tokens[3], "old")
		if !ok {
			return
		}
		newRaw, ok := exactTokenValue(tokens[4], "new")
		if !ok {
			return
		}
		if _, ok := parseCanonicalPrefixedHex(oldRaw, math.MaxUint32); !ok {
			return
		}
		if _, ok := parseCanonicalPrefixedHex(newRaw, math.MaxUint32); !ok {
			return
		}
	}

	dev, ok := canonicalPageDevice(devRaw)
	if !ok {
		return
	}
	inode, ok := parseCanonicalPrefixedHex(inodeRaw, math.MaxUint64)
	if !ok {
		return
	}
	ev.FileFields.Dev = intern.intern(dev)
	ev.FileFields.Ino = intern.intern(fmt.Sprintf("0x%x", inode))
}

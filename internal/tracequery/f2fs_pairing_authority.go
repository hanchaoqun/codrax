package tracequery

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// f2fsPairingAdmission is the parse-time handoff for the six exact F2FS
// endpoint names.  It prevents generic FileFields projection (which is
// intentionally display-friendly) from becoming a second hard authority.
type f2fsPairingAdmission struct {
	identityKnown   bool
	payloadAdmitted bool
	device          string
	inode           string
	operation       string
}

func exactF2FSPairingProfile(name string) (pairingEndpointProfile, bool) {
	switch name {
	case "f2fs_sync_file_enter":
		return pairingEndpointProfile{Family: PairingEndpointStorage, Phase: PairingEndpointStart, SemanticBase: "f2fs_sync_file", Layer: "f2fs", IdleAllowed: true}, true
	case "f2fs_sync_file_exit":
		return pairingEndpointProfile{Family: PairingEndpointStorage, Phase: PairingEndpointDone, SemanticBase: "f2fs_sync_file", Layer: "f2fs", IdleAllowed: true}, true
	case "f2fs_direct_IO_enter":
		return pairingEndpointProfile{Family: PairingEndpointStorage, Phase: PairingEndpointStart, SemanticBase: "f2fs_direct_io", Layer: "f2fs", IdleAllowed: true}, true
	case "f2fs_direct_IO_exit":
		return pairingEndpointProfile{Family: PairingEndpointStorage, Phase: PairingEndpointDone, SemanticBase: "f2fs_direct_io", Layer: "f2fs", IdleAllowed: true}, true
	case "f2fs_write_begin":
		return pairingEndpointProfile{Family: PairingEndpointStorage, Phase: PairingEndpointStart, SemanticBase: "f2fs_write", Layer: "f2fs", IdleAllowed: true}, true
	case "f2fs_write_end":
		return pairingEndpointProfile{Family: PairingEndpointStorage, Phase: PairingEndpointDone, SemanticBase: "f2fs_write", Layer: "f2fs", IdleAllowed: true}, true
	default:
		return pairingEndpointProfile{}, false
	}
}

// f2fsElapsedPairingNameCandidate is the broad negative gate for F2FS names
// that a generic phase parser could mistake for elapsed endpoints. The closed
// hard-family spelling authority stays centralized in the exported predicate;
// the second arm only prevents other F2FS phase observations (for example,
// dataread_start/end) from falling back to generic duration pairing. Neither
// arm authorizes a payload.
func f2fsElapsedPairingNameCandidate(name string) bool {
	if F2FSClosedEndpointNameCandidate(name) {
		return true
	}
	canonicalLooking := strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(canonicalLooking, "f2fs_") {
		for _, phaseToken := range []string{"_start", "_enter", "_begin", "_done", "_exit", "_end", "_complete"} {
			if strings.Contains(canonicalLooking, phaseToken) {
				return true
			}
		}
	}
	return false
}

// F2FSClosedEndpointNameCandidate reports whether name belongs to the closed
// F2FS hard endpoint families, including case/outer-space/underscore-loss and
// suffix drift. It is a negative gate only: true never authorizes a payload or
// a duration lane; exactF2FSPairingProfile plus the closed body parser remain
// the sole positive authority for the six byte-exact endpoints.
func F2FSClosedEndpointNameCandidate(name string) bool {
	compact := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "_", "")
	if !strings.HasPrefix(compact, "f2fs") {
		return false
	}
	rest := strings.TrimPrefix(compact, "f2fs")
	for _, family := range []string{"syncfile", "directio", "write"} {
		if !strings.HasPrefix(rest, family) {
			continue
		}
		phaseAndSuffix := strings.TrimPrefix(rest, family)
		for _, phase := range []string{"enter", "exit", "start", "done", "begin", "end", "complete"} {
			if strings.HasPrefix(phaseAndSuffix, phase) {
				return true
			}
		}
		return false
	}
	return false
}

func f2fsStorageWireAdmission(name, fieldText string) (genericStorageWireAdmission, bool) {
	profile, exact := exactF2FSPairingProfile(name)
	if !exact {
		if F2FSClosedEndpointNameCandidate(name) {
			return genericStorageWireAdmission{}, true
		}
		return genericStorageWireAdmission{}, false
	}
	tokens, order, duplicates, tokenOK := f2fsClosedTokens(fieldText)
	dev, devOK := f2fsDeviceToken(tokens["dev"])
	inode, inodeOK := f2fsInodeToken(tokens["ino"], true)
	op, opOK := f2fsOperationForProfile(profile, tokens)
	identityUnique := !duplicates["dev"] && !duplicates["ino"]
	if profile.SemanticBase == "f2fs_direct_io" {
		identityUnique = identityUnique && !duplicates["rw"]
	}
	admission := genericStorageWireAdmission{
		identityKnown: identityUnique && devOK && inodeOK && opOK,
		dev:           dev, inode: inode, op: op,
	}
	if !tokenOK {
		return admission, true
	}
	admission.payloadAdmitted = admission.identityKnown && f2fsCanonicalBody(name, tokens, order)
	return admission, true
}

func f2fsAdmissionFromEvent(ev Event) (genericStorageWireAdmission, bool) {
	if ev.ResourceFields != nil && ev.ResourceFields.f2fsPairing != nil {
		item := ev.ResourceFields.f2fsPairing
		return genericStorageWireAdmission{
			identityKnown: item.identityKnown, payloadAdmitted: item.payloadAdmitted,
			dev: item.device, inode: item.inode, op: item.operation,
		}, true
	}
	return f2fsStorageWireAdmission(ev.Name, ev.FieldText)
}

func f2fsPairingVerdictFromEvent(ev Event) (PairingEndpointVerdict, genericStorageWireAdmission, bool) {
	admission, governed := f2fsAdmissionFromEvent(ev)
	if !governed {
		return PairingEndpointVerdict{}, genericStorageWireAdmission{}, false
	}
	verdict := FingerprintPairingEndpoint(PairingEndpointTypedInput{
		Name: ev.Name, HeaderTID: int64(ev.PID),
		StorageIdentityKnown:         admission.identityKnown,
		StoragePayloadAdmissionKnown: true, StoragePayloadAdmitted: admission.payloadAdmitted,
		StorageDevice: admission.dev, StorageInode: admission.inode, StorageOperation: admission.op,
	})
	return verdict, admission, true
}

func f2fsSemanticPayloadAdmitted(ev Event) bool {
	if !F2FSClosedEndpointNameCandidate(ev.Name) {
		return true
	}
	admission, governed := f2fsAdmissionFromEvent(ev)
	return governed && admission.payloadAdmitted
}

func f2fsClosedTokens(fieldText string) (map[string]string, []string, map[string]bool, bool) {
	parts := strings.Fields(fieldText)
	closed := len(parts) > 0 && strings.Join(parts, " ") == fieldText
	out := make(map[string]string, len(parts))
	order := make([]string, 0, len(parts))
	duplicates := make(map[string]bool)
	for key, count := range f2fsHardIdentityOccurrences(fieldText) {
		if count > 1 {
			duplicates[key] = true
		}
	}
	for _, part := range parts {
		key, value, found := strings.Cut(part, "=")
		if !found || key == "" || value == "" || strings.Contains(value, "=") || !isTraceKVKey(key) || !f2fsClosedScalar(value) {
			closed = false
			continue
		}
		if _, duplicate := out[key]; duplicate {
			duplicates[key] = true
			closed = false
			continue
		}
		out[key] = value
		order = append(order, key)
	}
	return out, order, duplicates, closed
}

// f2fsHardIdentityOccurrences is a negative-only physical declaration census.
// It deliberately runs before the canonical token/value parser: malformed
// declarations such as `dev = 8:0` or `garbage=1,dev=8:0` must still make a
// second exact hard-key declaration ambiguous, otherwise a valid endpoint on
// another lane can hide the bad row and rescue a pair across it. The census
// never returns values and therefore cannot authorize a spaced, punctuated,
// quoted, empty, or otherwise non-canonical scalar.
func f2fsHardIdentityOccurrences(fieldText string) map[string]int {
	counts := make(map[string]int, 3)
	for offset := 0; offset < len(fieldText); {
		r, width := utf8.DecodeRuneInString(fieldText[offset:])
		if width <= 0 {
			break
		}
		if r != 'd' && r != 'i' && r != 'r' {
			offset += width
			continue
		}
		if offset > 0 {
			previous, _ := utf8.DecodeLastRuneInString(fieldText[:offset])
			if f2fsIdentityWordRune(previous) {
				offset += width
				continue
			}
		}
		key := ""
		for _, candidate := range [...]string{"dev", "ino", "rw"} {
			if strings.HasPrefix(fieldText[offset:], candidate) {
				key = candidate
				break
			}
		}
		if key == "" {
			offset += width
			continue
		}
		position := offset + len(key)
		if position < len(fieldText) {
			next, _ := utf8.DecodeRuneInString(fieldText[position:])
			if f2fsIdentityWordRune(next) {
				offset += width
				continue
			}
		}
		for position < len(fieldText) {
			next, nextWidth := utf8.DecodeRuneInString(fieldText[position:])
			if !unicode.IsSpace(next) {
				break
			}
			position += nextWidth
		}
		if position < len(fieldText) && fieldText[position] == '=' {
			counts[key]++
		}
		offset += len(key)
	}
	return counts
}

func f2fsIdentityWordRune(r rune) bool {
	return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func f2fsClosedScalar(value string) bool {
	if value == "" || len(value) > 256 || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsAny(value, "'\"=,|[]{}()") {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func f2fsDeviceToken(raw string) (string, bool) {
	if strings.Count(raw, ":") != 1 || strings.Contains(raw, ",") {
		return "", false
	}
	canonical, ok := canonicalBlockDevice(raw)
	return canonical, ok && blockDeviceIdentifiesRequest(canonical)
}

func f2fsCanonicalDeviceToken(raw string) bool {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 || !f2fsUnsignedDecimal(parts[0], 32) || !f2fsUnsignedDecimal(parts[1], 32) {
		return false
	}
	_, ok := f2fsDeviceToken(raw)
	return ok
}

func f2fsInodeToken(raw string, nonzero bool) (string, bool) {
	if raw == "" || strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "-") {
		return "", false
	}
	base := 10
	digits := raw
	if strings.HasPrefix(raw, "0x") {
		base, digits = 16, raw[2:]
	}
	if digits == "" {
		return "", false
	}
	value, err := strconv.ParseUint(digits, base, 64)
	if err != nil || nonzero && value == 0 {
		return "", false
	}
	return strconv.FormatUint(value, 10), true
}

func f2fsCanonicalHexInode(raw string, nonzero bool) bool {
	if !strings.HasPrefix(raw, "0x") {
		return false
	}
	canonical, ok := f2fsInodeToken(raw, nonzero)
	if !ok {
		return false
	}
	value, err := strconv.ParseUint(canonical, 10, 64)
	return err == nil && raw == "0x"+strconv.FormatUint(value, 16)
}

func f2fsOperationForProfile(profile pairingEndpointProfile, tokens map[string]string) (string, bool) {
	switch profile.SemanticBase {
	case "f2fs_sync_file":
		return "sync", true
	case "f2fs_write":
		return "write", true
	case "f2fs_direct_io":
		switch tokens["rw"] {
		case "read", "write":
			return tokens["rw"], true
		default:
			return "", false
		}
	default:
		return "", false
	}
}

type f2fsWireScalar uint8

const (
	f2fsDev f2fsWireScalar = iota + 1
	f2fsInode
	f2fsOptionalInode
	f2fsU8Hex
	f2fsU16Hex
	f2fsU32Decimal
	f2fsU32Hex
	f2fsU64Decimal
	f2fsI32Decimal
	f2fsNonNegativeI64
	f2fsRWToken
)

type f2fsWireField struct {
	name string
	kind f2fsWireScalar
}

func f2fsCanonicalBody(name string, tokens map[string]string, order []string) bool {
	var variants [][]f2fsWireField
	switch name {
	case "f2fs_sync_file_enter":
		variants = [][]f2fsWireField{{
			{"dev", f2fsDev}, {"ino", f2fsInode}, {"pino", f2fsOptionalInode}, {"i_mode", f2fsU16Hex},
			{"i_size", f2fsNonNegativeI64}, {"i_nlink", f2fsU32Decimal}, {"i_blocks", f2fsU64Decimal}, {"i_advise", f2fsU8Hex},
		}}
	case "f2fs_sync_file_exit":
		variants = [][]f2fsWireField{{
			{"dev", f2fsDev}, {"ino", f2fsInode}, {"cp_reason", f2fsI32Decimal}, {"datasync", f2fsI32Decimal}, {"ret", f2fsI32Decimal},
		}}
	case "f2fs_direct_IO_enter":
		variants = [][]f2fsWireField{
			{{"dev", f2fsDev}, {"ino", f2fsInode}, {"pos", f2fsNonNegativeI64}, {"len", f2fsNonNegativeI64}, {"rw", f2fsRWToken}},
			{{"dev", f2fsDev}, {"ino", f2fsInode}, {"pos", f2fsNonNegativeI64}, {"len", f2fsNonNegativeI64}, {"ki_flags", f2fsU32Hex}, {"ki_ioprio", f2fsU16Hex}, {"rw", f2fsRWToken}},
		}
	case "f2fs_direct_IO_exit":
		variants = [][]f2fsWireField{{
			{"dev", f2fsDev}, {"ino", f2fsInode}, {"pos", f2fsNonNegativeI64}, {"len", f2fsNonNegativeI64}, {"rw", f2fsRWToken}, {"ret", f2fsI32Decimal},
		}}
	case "f2fs_write_begin":
		variants = [][]f2fsWireField{
			{{"dev", f2fsDev}, {"ino", f2fsInode}, {"pos", f2fsNonNegativeI64}, {"len", f2fsU32Decimal}},
			{{"dev", f2fsDev}, {"ino", f2fsInode}, {"pos", f2fsNonNegativeI64}, {"len", f2fsU32Decimal}, {"flags", f2fsU32Decimal}},
		}
	case "f2fs_write_end":
		variants = [][]f2fsWireField{{
			{"dev", f2fsDev}, {"ino", f2fsInode}, {"pos", f2fsNonNegativeI64}, {"len", f2fsU32Decimal}, {"copied", f2fsU32Decimal},
		}}
	default:
		return false
	}
	for _, fields := range variants {
		if f2fsExactWireFields(tokens, order, fields) {
			return true
		}
	}
	return false
}

func f2fsExactWireFields(tokens map[string]string, order []string, fields []f2fsWireField) bool {
	if len(tokens) != len(fields) || len(order) != len(fields) {
		return false
	}
	for index, field := range fields {
		if order[index] != field.name {
			return false
		}
		value, present := tokens[field.name]
		if !present || !f2fsWireValue(value, field.kind) {
			return false
		}
	}
	return true
}

func f2fsWireValue(value string, kind f2fsWireScalar) bool {
	switch kind {
	case f2fsDev:
		return f2fsCanonicalDeviceToken(value)
	case f2fsInode:
		return f2fsCanonicalHexInode(value, true)
	case f2fsOptionalInode:
		return f2fsCanonicalHexInode(value, false)
	case f2fsU8Hex:
		return f2fsUnsignedHex(value, 8)
	case f2fsU16Hex:
		return f2fsUnsignedHex(value, 16)
	case f2fsU32Decimal:
		return f2fsUnsignedDecimal(value, 32)
	case f2fsU32Hex:
		return f2fsUnsignedHex(value, 32)
	case f2fsU64Decimal:
		return f2fsUnsignedDecimal(value, 64)
	case f2fsI32Decimal:
		return f2fsSignedDecimal(value, 32)
	case f2fsNonNegativeI64:
		return f2fsUnsignedDecimal(value, 63)
	case f2fsRWToken:
		return value == "read" || value == "write"
	default:
		return false
	}
}

func f2fsUnsignedDecimal(value string, bits int) bool {
	if value == "" || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	parsed, err := strconv.ParseUint(value, 10, bits)
	return err == nil && strconv.FormatUint(parsed, 10) == value
}

func f2fsSignedDecimal(value string, bits int) bool {
	parsed, err := strconv.ParseInt(value, 10, bits)
	return err == nil && strconv.FormatInt(parsed, 10) == value
}

func f2fsUnsignedHex(value string, bits int) bool {
	if !strings.HasPrefix(value, "0x") || len(value) <= 2 {
		return false
	}
	digits := value[2:]
	for _, ch := range digits {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return false
		}
	}
	parsed, err := strconv.ParseUint(digits, 16, bits)
	return err == nil && strconv.FormatUint(parsed, 16) == digits
}

func f2fsTypedIdentity(profile pairingEndpointProfile, input PairingEndpointTypedInput) (genericStorageIdentity, bool) {
	dev, devOK := typedPairingDeviceText(input.StorageDevice, input.StorageDeviceNumber, input.StorageDeviceNumeric)
	inode, inodeOK := typedPairingInodeText(input.StorageInode, input.StorageInodeNumber, input.StorageInodeNumeric)
	if !devOK || !blockDeviceIdentifiesRequest(dev) || !inodeOK || inode == "" || inode == "0" {
		return genericStorageIdentity{}, false
	}
	op, opOK := strictPairingScalar(input.StorageOperation)
	if !opOK {
		return genericStorageIdentity{}, false
	}
	switch profile.SemanticBase {
	case "f2fs_sync_file":
		if op != "sync" {
			return genericStorageIdentity{}, false
		}
	case "f2fs_write":
		if op != "write" {
			return genericStorageIdentity{}, false
		}
	case "f2fs_direct_io":
		if op != "read" && op != "write" {
			return genericStorageIdentity{}, false
		}
	default:
		return genericStorageIdentity{}, false
	}
	return genericStorageIdentity{
		Layer: profile.Layer, Base: profile.SemanticBase, Dev: dev, Inode: inode, Op: op, PID: int(input.HeaderTID),
	}, true
}

package tracequery

import (
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// mmcPairingAdmission is the parse-time handoff for the exact MMC endpoint
// family. The official direct/structured bodies are longer than Event's
// 300-byte display inventory bound, so hard pairing consumers must use this
// verdict carrier rather than reparsing truncated FieldText.
type mmcPairingAdmission struct {
	identityKnown   bool
	payloadAdmitted bool
	device          string
	opcode          string
}

func exactMMCPairingProfile(name string) (pairingEndpointProfile, bool) {
	switch name {
	case "mmc_request_start":
		return pairingEndpointProfile{
			Family: PairingEndpointStorage, Phase: PairingEndpointStart,
			SemanticBase: "mmc_request", Layer: "mmc", IdleAllowed: true,
		}, true
	case "mmc_request_done":
		return pairingEndpointProfile{
			Family: PairingEndpointStorage, Phase: PairingEndpointDone,
			SemanticBase: "mmc_request", Layer: "mmc", IdleAllowed: true,
		}, true
	default:
		return pairingEndpointProfile{}, false
	}
}

// mmcPairingNameCandidate is deliberately broader than the exact registry,
// but only as a negative gate. It prevents case, whitespace and suffix drift
// from falling back through the generic storage prefix/suffix classifier.
func mmcPairingNameCandidate(name string) bool {
	canonicalLooking := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(canonicalLooking, "mmc_") || strings.Contains(canonicalLooking, "mmc_request_")
}

func mmcStorageWireAdmission(name, fieldText string) (genericStorageWireAdmission, bool) {
	profile, governed := exactMMCPairingProfile(name)
	if !governed {
		return genericStorageWireAdmission{}, false
	}
	trimmed := strings.TrimSpace(fieldText)
	if trimmed == "" || strings.ContainsAny(trimmed, "\t\r\n") || strings.Contains(trimmed, "  ") {
		return genericStorageWireAdmission{}, true
	}
	tokens := strings.Split(trimmed, " ")
	device, opcode, identityKnown := mmcCoarseWireIdentity(tokens, profile.Phase)
	admission := genericStorageWireAdmission{
		identityKnown: identityKnown,
		dev:           device,
		op:            opcode,
	}
	if profile.Phase == PairingEndpointStart {
		admission.payloadAdmitted = mmcCanonicalStartBody(tokens) || mmcCompactStartBody(tokens)
	} else {
		admission.payloadAdmitted = mmcCanonicalDoneBody(tokens, true) ||
			mmcCanonicalDoneBody(tokens, false) || mmcCompactDoneBody(tokens)
	}
	// A fully admitted closed profile necessarily proves its coarse identity.
	// Keep the implication explicit so a future profile edit cannot publish a
	// payload that the anti-rescue authority cannot locate.
	if admission.payloadAdmitted {
		admission.identityKnown = true
	}
	return admission, true
}

func mmcAdmissionFromEvent(ev Event) (genericStorageWireAdmission, bool) {
	if ev.ResourceFields != nil && ev.ResourceFields.mmcPairing != nil {
		item := ev.ResourceFields.mmcPairing
		return genericStorageWireAdmission{
			identityKnown: item.identityKnown, payloadAdmitted: item.payloadAdmitted,
			dev: item.device, op: item.opcode,
		}, true
	}
	return mmcStorageWireAdmission(ev.Name, ev.FieldText)
}

func mmcPairingVerdictFromEvent(ev Event) (PairingEndpointVerdict, genericStorageWireAdmission, bool) {
	admission, governed := mmcAdmissionFromEvent(ev)
	if !governed {
		return PairingEndpointVerdict{}, genericStorageWireAdmission{}, false
	}
	verdict := FingerprintPairingEndpoint(PairingEndpointTypedInput{
		Name: ev.Name, HeaderTID: int64(ev.PID),
		StorageIdentityKnown:         admission.identityKnown,
		StoragePayloadAdmissionKnown: true,
		StoragePayloadAdmitted:       admission.payloadAdmitted,
		StorageDevice:                admission.dev,
		StorageOperation:             admission.op,
	})
	return verdict, admission, true
}

func mmcSemanticPayloadAdmitted(ev Event) bool {
	if !mmcPairingNameCandidate(ev.Name) {
		return true
	}
	admission, governed := mmcAdmissionFromEvent(ev)
	return governed && admission.payloadAdmitted
}

func mmcCoarseWireIdentity(tokens []string, phase PairingEndpointPhase) (device, opcode string, ok bool) {
	if len(tokens) == 0 {
		return "", "", false
	}
	device = tokens[0]
	canonicalPrefix := false
	if len(tokens) >= 4 && strings.HasSuffix(device, ":") {
		device = strings.TrimSuffix(device, ":")
		canonicalPrefix = mmcDeviceToken(device) &&
			tokens[1] == mmcPhaseWord(phase) && tokens[2] == "struct" &&
			mmcRequestPointerToken(tokens[3])
	}
	compactPrefix := mmcDeviceToken(device) && !strings.HasSuffix(tokens[0], ":")
	if !canonicalPrefix && !compactPrefix {
		return "", "", false
	}
	want := "opcode"
	if canonicalPrefix {
		want = "cmd_opcode"
	}
	seen := false
	for _, token := range tokens {
		key, value, found := strings.Cut(token, "=")
		if !found || key != want {
			continue
		}
		if seen || !mmcUnsignedDecimal(value, 32) {
			return "", "", false
		}
		opcode, seen = value, true
	}
	return device, opcode, seen
}

func mmcPhaseWord(phase PairingEndpointPhase) string {
	if phase == PairingEndpointStart {
		return "start"
	}
	if phase == PairingEndpointDone {
		return "end"
	}
	return ""
}

func mmcCanonicalPrefix(tokens []string, phase PairingEndpointPhase) bool {
	if len(tokens) < 4 || !strings.HasSuffix(tokens[0], ":") {
		return false
	}
	return mmcDeviceToken(strings.TrimSuffix(tokens[0], ":")) &&
		tokens[1] == mmcPhaseWord(phase) && tokens[2] == "struct" &&
		mmcRequestPointerToken(tokens[3])
}

type mmcScalarKind uint8

const (
	mmcU32Decimal mmcScalarKind = iota + 1
	mmcU32Hex
	mmcI32Decimal
	mmcResponse4
)

type mmcFieldSpec struct {
	name string
	kind mmcScalarKind
}

var mmcStartCanonicalFields = [...]mmcFieldSpec{
	{"cmd_opcode", mmcU32Decimal}, {"cmd_arg", mmcU32Hex}, {"cmd_flags", mmcU32Hex}, {"cmd_retries", mmcU32Decimal},
	{"stop_opcode", mmcU32Decimal}, {"stop_arg", mmcU32Hex}, {"stop_flags", mmcU32Hex}, {"stop_retries", mmcU32Decimal},
	{"sbc_opcode", mmcU32Decimal}, {"sbc_arg", mmcU32Hex}, {"sbc_flags", mmcU32Hex}, {"sbc_retires", mmcU32Decimal},
	{"blocks", mmcU32Decimal}, {"block_size", mmcU32Decimal}, {"blk_addr", mmcU32Decimal}, {"data_flags", mmcU32Hex},
	{"tag", mmcI32Decimal}, {"can_retune", mmcU32Decimal}, {"doing_retune", mmcU32Decimal}, {"retune_now", mmcU32Decimal},
	{"need_retune", mmcI32Decimal}, {"hold_retune", mmcI32Decimal}, {"retune_period", mmcU32Decimal},
}

var mmcDoneDirectFields = [...]mmcFieldSpec{
	{"cmd_opcode", mmcU32Decimal}, {"cmd_err", mmcI32Decimal}, {"cmd_resp", mmcResponse4}, {"cmd_retries", mmcU32Decimal},
	{"stop_opcode", mmcU32Decimal}, {"stop_err", mmcI32Decimal}, {"stop_resp", mmcResponse4}, {"stop_retries", mmcU32Decimal},
	{"sbc_opcode", mmcU32Decimal}, {"sbc_err", mmcI32Decimal}, {"sbc_resp", mmcResponse4}, {"sbc_retries", mmcU32Decimal},
	{"bytes_xfered", mmcU32Decimal}, {"data_err", mmcI32Decimal}, {"tag", mmcI32Decimal},
	{"can_retune", mmcU32Decimal}, {"doing_retune", mmcU32Decimal}, {"retune_now", mmcU32Decimal},
	{"need_retune", mmcI32Decimal}, {"hold_retune", mmcI32Decimal}, {"retune_period", mmcU32Decimal},
}

var mmcDoneStructuredFields = [...]mmcFieldSpec{
	{"cmd_opcode", mmcU32Decimal}, {"cmd_err", mmcI32Decimal}, {"cmd_retries", mmcU32Decimal},
	{"stop_opcode", mmcU32Decimal}, {"stop_err", mmcI32Decimal}, {"stop_retries", mmcU32Decimal},
	{"sbc_opcode", mmcU32Decimal}, {"sbc_err", mmcI32Decimal}, {"sbc_retries", mmcU32Decimal},
	{"bytes_xfered", mmcU32Decimal}, {"data_err", mmcI32Decimal}, {"tag", mmcI32Decimal},
	{"can_retune", mmcU32Decimal}, {"doing_retune", mmcU32Decimal}, {"retune_now", mmcU32Decimal},
	{"need_retune", mmcI32Decimal}, {"hold_retune", mmcI32Decimal}, {"retune_period", mmcU32Decimal},
}

func mmcCanonicalStartBody(tokens []string) bool {
	return mmcCanonicalPrefix(tokens, PairingEndpointStart) &&
		mmcOrderedCanonicalFields(tokens[4:], mmcStartCanonicalFields[:])
}

func mmcCanonicalDoneBody(tokens []string, responses bool) bool {
	if !mmcCanonicalPrefix(tokens, PairingEndpointDone) {
		return false
	}
	fields := mmcDoneStructuredFields[:]
	if responses {
		fields = mmcDoneDirectFields[:]
	}
	return mmcOrderedCanonicalFields(tokens[4:], fields)
}

func mmcOrderedCanonicalFields(tokens []string, specs []mmcFieldSpec) bool {
	position := 0
	for _, spec := range specs {
		if position >= len(tokens) {
			return false
		}
		key, value, found := strings.Cut(tokens[position], "=")
		if !found || key != spec.name || value == "" {
			return false
		}
		position++
		switch spec.kind {
		case mmcU32Decimal:
			if !mmcUnsignedDecimal(value, 32) {
				return false
			}
		case mmcU32Hex:
			if !mmcCanonicalHex(value, 32, true) {
				return false
			}
		case mmcI32Decimal:
			if !mmcSignedDecimal(value, 32) {
				return false
			}
		case mmcResponse4:
			if !mmcCanonicalHex(value, 32, true) || position+3 > len(tokens) {
				return false
			}
			for _, response := range tokens[position : position+3] {
				if !mmcCanonicalHex(response, 32, true) {
					return false
				}
			}
			position += 3
		default:
			return false
		}
	}
	return position == len(tokens)
}

func mmcCompactStartBody(tokens []string) bool {
	if len(tokens) != 6 || !mmcDeviceToken(tokens[0]) {
		return false
	}
	return mmcExactField(tokens[1], "tag", func(v string) bool { return mmcSignedDecimal(v, 64) }) &&
		mmcExactField(tokens[2], "opcode", func(v string) bool { return mmcUnsignedDecimal(v, 63) }) &&
		mmcExactField(tokens[3], "blocks", func(v string) bool { return mmcUnsignedDecimal(v, 63) }) &&
		mmcExactField(tokens[4], "block_size", func(v string) bool { return mmcUnsignedDecimal(v, 63) }) &&
		mmcExactField(tokens[5], "blk_addr", func(v string) bool { return mmcUnsignedDecimal(v, 63) })
}

func mmcCompactDoneBody(tokens []string) bool {
	if len(tokens) < 5 || len(tokens) > 7 || !mmcDeviceToken(tokens[0]) ||
		!mmcExactField(tokens[1], "tag", func(v string) bool { return mmcSignedDecimal(v, 64) }) ||
		!mmcExactField(tokens[2], "opcode", func(v string) bool { return mmcUnsignedDecimal(v, 63) }) ||
		!mmcExactField(tokens[3], "bytes_xfered", func(v string) bool { return mmcUnsignedDecimal(v, 63) }) {
		return false
	}
	position := 4
	for _, key := range []string{"ret", "cmd_err", "data_err"} {
		if position < len(tokens) && strings.HasPrefix(tokens[position], key+"=") {
			if !mmcExactField(tokens[position], key, func(v string) bool { return mmcSignedDecimal(v, 64) }) {
				return false
			}
			position++
		}
	}
	return position == len(tokens) && position > 4
}

func mmcExactField(token, key string, valid func(string) bool) bool {
	got, value, found := strings.Cut(token, "=")
	return found && got == key && value != "" && valid(value)
}

func mmcDeviceToken(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value || !utf8.ValidString(value) ||
		strings.ContainsAny(value, ":[]=,|'\"") {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func mmcRequestPointerToken(value string) bool {
	const prefix = "mmc_request["
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, "]:") {
		return false
	}
	pointer := strings.TrimSuffix(strings.TrimPrefix(value, prefix), "]:")
	return mmcCanonicalHex(pointer, 64, false)
}

func mmcUnsignedDecimal(value string, bits int) bool {
	if !mmcCanonicalUnsignedLexeme(value) {
		return false
	}
	_, err := strconv.ParseUint(value, 10, bits)
	return err == nil
}

func mmcSignedDecimal(value string, bits int) bool {
	if value == "" || strings.HasPrefix(value, "+") {
		return false
	}
	negative := strings.HasPrefix(value, "-")
	digits := value
	if negative {
		digits = strings.TrimPrefix(value, "-")
		if digits == "0" {
			return false
		}
	}
	if !mmcCanonicalUnsignedLexeme(digits) {
		return false
	}
	_, err := strconv.ParseInt(value, 10, bits)
	return err == nil
}

func mmcCanonicalUnsignedLexeme(value string) bool {
	if value == "0" {
		return true
	}
	if value == "" || value[0] == '0' {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func mmcCanonicalHex(value string, bits int, zeroAllowed bool) bool {
	if !strings.HasPrefix(value, "0x") {
		return false
	}
	digits := strings.TrimPrefix(value, "0x")
	if digits == "" || len(digits) > bits/4 || len(digits) > 1 && digits[0] == '0' {
		return false
	}
	for _, r := range digits {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	parsed, err := strconv.ParseUint(digits, 16, bits)
	return err == nil && (zeroAllowed || parsed != 0) && parsed <= math.MaxUint64
}

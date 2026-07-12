package tracequery

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// PairingEndpointFamily is the closed set of wire families whose endpoints
// can mint a duration or causal edge. It is exported only so the SQL trace
// adapter can consume the same fingerprint authority before publishing rows.
type PairingEndpointFamily string

const (
	PairingEndpointBinder    PairingEndpointFamily = "binder"
	PairingEndpointWorkqueue PairingEndpointFamily = "workqueue"
	PairingEndpointDMAFence  PairingEndpointFamily = "dma_fence"
	PairingEndpointBlock     PairingEndpointFamily = "block"
	PairingEndpointStorage   PairingEndpointFamily = "storage"
)

type PairingEndpointPhase string

const (
	PairingEndpointStart PairingEndpointPhase = "start"
	PairingEndpointDone  PairingEndpointPhase = "done"
)

type pairingEndpointProfile struct {
	Family                  PairingEndpointFamily
	Phase                   PairingEndpointPhase
	SemanticBase            string
	Layer                   string
	IdleAllowed             bool
	RequiresPositiveEmitter bool
}

// pairingEndpointProfileForName is the only exact endpoint-name registry.
// Case drift is inventory, never a hard endpoint.
func pairingEndpointProfileForName(name string) (pairingEndpointProfile, bool) {
	switch strings.TrimSpace(name) {
	case "binder_transaction":
		return pairingEndpointProfile{Family: PairingEndpointBinder, Phase: PairingEndpointStart, SemanticBase: "binder_transaction", RequiresPositiveEmitter: true}, true
	case "binder_transaction_received":
		return pairingEndpointProfile{Family: PairingEndpointBinder, Phase: PairingEndpointDone, SemanticBase: "binder_transaction", RequiresPositiveEmitter: true}, true
	case "workqueue_execute_start":
		return pairingEndpointProfile{Family: PairingEndpointWorkqueue, Phase: PairingEndpointStart, SemanticBase: "workqueue_execute", RequiresPositiveEmitter: true}, true
	case "workqueue_execute_end":
		return pairingEndpointProfile{Family: PairingEndpointWorkqueue, Phase: PairingEndpointDone, SemanticBase: "workqueue_execute", RequiresPositiveEmitter: true}, true
	case "dma_fence_wait_start":
		return pairingEndpointProfile{Family: PairingEndpointDMAFence, Phase: PairingEndpointStart, SemanticBase: "dma_fence_wait", RequiresPositiveEmitter: true}, true
	case "dma_fence_wait_end":
		return pairingEndpointProfile{Family: PairingEndpointDMAFence, Phase: PairingEndpointDone, SemanticBase: "dma_fence_wait", RequiresPositiveEmitter: true}, true
	case "block_rq_issue":
		return pairingEndpointProfile{Family: PairingEndpointBlock, Phase: PairingEndpointStart, SemanticBase: blockEndpointFamilyRQ, IdleAllowed: true}, true
	case "block_rq_complete":
		return pairingEndpointProfile{Family: PairingEndpointBlock, Phase: PairingEndpointDone, SemanticBase: blockEndpointFamilyRQ, IdleAllowed: true}, true
	case "block_bio_queue":
		return pairingEndpointProfile{Family: PairingEndpointBlock, Phase: PairingEndpointStart, SemanticBase: blockEndpointFamilyBIO, IdleAllowed: true}, true
	case "block_bio_complete":
		return pairingEndpointProfile{Family: PairingEndpointBlock, Phase: PairingEndpointDone, SemanticBase: blockEndpointFamilyBIO, IdleAllowed: true}, true
	default:
		return pairingEndpointProfile{}, false
	}
}

// PairingEndpointVerdict separates endpoint recognition from key admission.
// Recognized+!KeyKnown requires source-family fail-close when provenance is
// known (whole-family only when source is unresolved); a known semantic key
// can be quarantined without suppressing unrelated lanes. PayloadAdmitted is
// deliberately separate: a fully located but semantically invalid endpoint
// (for example zero-length read) quarantines that exact lane and mints no pair.
// Source is deliberately absent from SemanticKey: LaneKey adds the physical
// artifact namespace without pretending that it is a payload request field.
type PairingEndpointVerdict struct {
	Recognized              bool
	KeyKnown                bool
	PayloadAdmitted         bool
	Family                  PairingEndpointFamily
	Phase                   PairingEndpointPhase
	SemanticKey             string
	RequiresPositiveEmitter bool
	IdleAllowed             bool
	EmitterKnown            bool
	EmitterAdmitted         bool
}

// PairingEndpointTypedInput lets deterministic adapters submit already
// validated key fields without re-rendering malformed non-key metadata. The
// exact name registry and every canonical key constructor still live here.
type PairingEndpointTypedInput struct {
	Name      string
	HeaderTID int64

	Transaction            string
	TransactionNumber      uint64
	TransactionNumberKnown bool
	Work                   string
	WorkAddress            uint64
	WorkAddressKnown       bool
	Driver                 string
	Timeline               string
	Context                string
	ContextNumber          uint64
	ContextNumberKnown     bool
	Seqno                  string
	SeqnoNumber            uint64
	SeqnoNumberKnown       bool

	BlockIdentityKnown         bool
	BlockPayloadAdmissionKnown bool
	BlockPayloadAdmitted       bool
	BlockDevice                string
	BlockDeviceNumber          uint64
	BlockDeviceNumeric         bool
	BlockOperation             string
	BlockSector                int64
	BlockLength                int64

	StorageIdentityKnown         bool
	StoragePayloadAdmissionKnown bool
	StoragePayloadAdmitted       bool
	StorageDevice                string
	StorageDeviceNumber          uint64
	StorageDeviceNumeric         bool
	StorageInode                 string
	StorageInodeNumber           uint64
	StorageInodeNumeric          bool
	StorageOperation             string
}

func (verdict PairingEndpointVerdict) LaneKey(source string) (string, bool) {
	if !verdict.Recognized || !verdict.KeyKnown || verdict.Family == "" || verdict.SemanticKey == "" || strings.TrimSpace(source) == "" {
		return "", false
	}
	return encodePairingKey(source, string(verdict.Family), verdict.SemanticKey), true
}

// DecodePairingEndpoint is the single source-neutral endpoint fingerprint
// authority. name and fieldText are the exact ftrace event column and payload;
// headerTID is the row-header task identity (zero is the exact idle identity).
// Numeric identities are canonicalized before key construction so equivalent
// wire spellings cannot split one physical lane.
func DecodePairingEndpoint(name, fieldText string, headerTID int64) PairingEndpointVerdict {
	verdict, _ := decodePairingEndpointWire(name, fieldText, headerTID)
	return verdict
}

type pairingEndpointDecodedFields struct {
	work, function                   string
	driver, timeline, context, seqno string
	storage                          genericStorageWireAdmission
}

// decodePairingEndpointWire is the one-pass wire adapter shared by the public
// verdict API and hot retained-event consumers. It tokenizes a KV payload at
// most once and carries the exact decoded display fields beside the verdict so
// replay never invokes a second hard-field parser.
func decodePairingEndpointWire(name, fieldText string, headerTID int64) (PairingEndpointVerdict, pairingEndpointDecodedFields) {
	name = strings.TrimSpace(name)
	fieldText = strings.TrimSpace(fieldText)
	input := PairingEndpointTypedInput{Name: name, HeaderTID: headerTID}
	decoded := pairingEndpointDecodedFields{}
	if profile, ok := pairingEndpointProfileForName(name); ok {
		switch profile.Family {
		case PairingEndpointBinder:
			tokens, lexOK := tokenizePairingKV(fieldText)
			input.Transaction, _ = strictUniquePairingAliasTokens(tokens, lexOK, "transaction", "debug_id", "transaction_id")
		case PairingEndpointWorkqueue:
			tokens, lexOK := tokenizePairingKV(fieldText)
			decoded.work, decoded.function = workqueueExactEndpointFieldsFromTokens(fieldText, tokens, lexOK)
			input.Work = decoded.work
		case PairingEndpointDMAFence:
			tokens, lexOK := tokenizePairingKV(fieldText)
			decoded.driver, decoded.timeline, decoded.context, decoded.seqno = dmaFenceExactEndpointFieldsFromTokens(tokens, lexOK)
			input.Driver, input.Timeline, input.Context, input.Seqno = decoded.driver, decoded.timeline, decoded.context, decoded.seqno
		case PairingEndpointBlock:
			input.BlockDevice, input.BlockOperation, input.BlockSector, input.BlockLength, input.BlockIdentityKnown, input.BlockPayloadAdmitted = parseBlockRequestFingerprint(name, fieldText)
			input.BlockPayloadAdmissionKnown = input.BlockIdentityKnown
		}
		return FingerprintPairingEndpoint(input), decoded
	}
	tokens, lexOK := tokenizePairingKV(fieldText)
	verdict, admission := decodeGenericStoragePairingEndpoint(input, fieldText, tokens, lexOK)
	decoded.storage = admission
	return verdict, decoded
}

// fingerprintPairingEvent is the sole retained-Event adapter. Non-empty wire
// payloads are decoded verbatim and can never be rescued by pre-populated
// typed fields. Empty payloads retain the package's historical hand-built
// Block/Storage fixture surface, but still pass through the same exported
// typed fingerprint and emitter policy consumed by deterministic adapters.
func fingerprintPairingEvent(ev Event) PairingEndpointVerdict {
	if strings.TrimSpace(ev.FieldText) != "" {
		return DecodePairingEndpoint(ev.Name, ev.FieldText, int64(ev.PID))
	}
	name := strings.TrimSpace(ev.Name)
	if name == "" {
		switch ev.Type {
		case EventBlockIssue:
			name = "block_rq_issue"
		case EventBlockComplete:
			name = "block_rq_complete"
		}
	}
	if profile, ok := pairingEndpointProfileForName(name); ok && profile.Family == PairingEndpointBlock {
		blk := ev.BlockIOFields
		if blk == nil {
			return FingerprintPairingEndpoint(PairingEndpointTypedInput{Name: name, HeaderTID: int64(ev.PID)})
		}
		identityKnown := true
		return FingerprintPairingEndpoint(PairingEndpointTypedInput{
			Name: name, HeaderTID: int64(ev.PID), BlockIdentityKnown: identityKnown,
			BlockPayloadAdmissionKnown: blk.IdentityParsed, BlockPayloadAdmitted: blk.IdentityValid,
			BlockDevice: blk.Dev, BlockOperation: blk.Op, BlockSector: blk.Sector, BlockLength: blk.Len,
		})
	}
	if ev.Type == EventStorage || ev.Type == EventFilesystem {
		identity, _, endpoint := genericStorageEndpoint(ev)
		if endpoint {
			return FingerprintPairingEndpoint(PairingEndpointTypedInput{
				Name: name, HeaderTID: int64(ev.PID), StorageIdentityKnown: true,
				StorageDevice: identity.Dev, StorageInode: identity.Inode, StorageOperation: identity.Op,
			})
		}
	}
	return DecodePairingEndpoint(name, "", int64(ev.PID))
}

// FingerprintPairingEndpoint is the typed constructor shared by wire parsing
// and deterministic converters. It never accepts a caller-supplied family,
// phase, or semantic base; those remain bound to the exact name registry.
func FingerprintPairingEndpoint(input PairingEndpointTypedInput) PairingEndpointVerdict {
	input.Name = strings.TrimSpace(input.Name)
	profile, ok := pairingEndpointProfileForName(input.Name)
	if !ok {
		profile, ok = genericStoragePairingProfile(input.Name)
	}
	if !ok {
		return PairingEndpointVerdict{}
	}
	verdict := pairingEndpointBase(profile.Family, profile.Phase, profile.IdleAllowed, profile.RequiresPositiveEmitter)
	verdict.EmitterKnown = validNonNegativePairingEmitter(input.HeaderTID)
	verdict.EmitterAdmitted = verdict.EmitterKnown && (!profile.RequiresPositiveEmitter || input.HeaderTID > 0)
	switch profile.Family {
	case PairingEndpointBinder:
		transaction, valid := canonicalTypedPositiveIdentity(input.Transaction, input.TransactionNumber, input.TransactionNumberKnown)
		if !valid {
			return verdict
		}
		verdict.KeyKnown = true
		verdict.PayloadAdmitted = true
		verdict.SemanticKey = encodePairingKey(transaction)
	case PairingEndpointWorkqueue:
		if !verdict.EmitterKnown {
			return verdict
		}
		work, valid := canonicalTypedWorkqueueIdentity(input.Work, input.WorkAddress, input.WorkAddressKnown)
		if !valid {
			return verdict
		}
		verdict.KeyKnown = true
		verdict.PayloadAdmitted = true
		verdict.SemanticKey = encodePairingKey(strconv.FormatInt(input.HeaderTID, 10), work, profile.SemanticBase)
	case PairingEndpointDMAFence:
		driver, driverOK := strictPairingScalar(input.Driver)
		timeline, timelineOK := strictPairingScalar(input.Timeline)
		if !verdict.EmitterKnown || !driverOK || !timelineOK || driver == "" || timeline == "" {
			return verdict
		}
		context, contextOK := canonicalTypedUnsignedIdentity(input.Context, input.ContextNumber, input.ContextNumberKnown)
		seqno, seqnoOK := canonicalTypedUnsignedIdentity(input.Seqno, input.SeqnoNumber, input.SeqnoNumberKnown)
		if !contextOK || !seqnoOK {
			return verdict
		}
		verdict.KeyKnown = true
		verdict.PayloadAdmitted = true
		verdict.SemanticKey = encodePairingKey(strconv.FormatInt(input.HeaderTID, 10), driver, timeline, context, seqno, profile.SemanticBase)
	case PairingEndpointBlock:
		dev, devOK := canonicalTypedPairingDevice(input.BlockDevice, input.BlockDeviceNumber, input.BlockDeviceNumeric)
		if !input.BlockIdentityKnown || !devOK || !blockDeviceIdentifiesRequest(dev) || !validBlockOperationToken(input.BlockOperation) || input.BlockSector < 0 || input.BlockLength < 0 || input.BlockLength > maxBlockSectorCount {
			return verdict
		}
		verdict.KeyKnown = true
		verdict.PayloadAdmitted = input.BlockLength > 0 || blockOperationAllowsZeroLength(input.BlockOperation, input.BlockSector, input.BlockLength)
		if input.BlockPayloadAdmissionKnown {
			verdict.PayloadAdmitted = verdict.PayloadAdmitted && input.BlockPayloadAdmitted
		}
		verdict.SemanticKey = encodePairingKey(profile.SemanticBase, dev, input.BlockOperation, strconv.FormatInt(input.BlockSector, 10), strconv.FormatInt(input.BlockLength, 10))
	case PairingEndpointStorage:
		if !input.StorageIdentityKnown || !verdict.EmitterKnown {
			return verdict
		}
		identity, identityOK := typedGenericStorageIdentity(profile, input)
		if !identityOK {
			return verdict
		}
		verdict.KeyKnown = true
		verdict.PayloadAdmitted = true
		if input.StoragePayloadAdmissionKnown {
			verdict.PayloadAdmitted = input.StoragePayloadAdmitted
		}
		verdict.SemanticKey = genericStoragePairingSemanticKey(identity)
	}
	return verdict
}

func typedGenericStorageIdentity(profile pairingEndpointProfile, input PairingEndpointTypedInput) (genericStorageIdentity, bool) {
	name := strings.TrimSpace(input.Name)
	lowerName := strings.ToLower(name)
	if strings.HasPrefix(lowerName, "mmc_") {
		// The current standard wire places the mmc device/opcode/tag
		// positionally; tracequery does not promote them into the coarse hard
		// identity. Keep the typed adapter byte-for-byte equivalent.
		return genericStorageIdentity{
			Layer: profile.Layer, Base: profile.SemanticBase,
			Dev: "unknown", Inode: "-", Op: profile.SemanticBase, PID: int(input.HeaderTID),
		}, true
	}
	dev, devOK := typedPairingDeviceText(input.StorageDevice, input.StorageDeviceNumber, input.StorageDeviceNumeric)
	if !devOK {
		return genericStorageIdentity{}, false
	}
	if strings.HasPrefix(lowerName, "scsi_") || strings.HasPrefix(lowerName, "ufshcd_") {
		// tag/lba/len/opcode/inode are useful inventory but remain outside the
		// established generic-storage coarse key pending request-token witness.
		return genericStorageIdentity{
			Layer: profile.Layer, Base: profile.SemanticBase,
			Dev: firstNonEmpty(dev, "unknown"), Inode: "-", Op: profile.SemanticBase, PID: int(input.HeaderTID),
		}, true
	}
	inode, inodeOK := typedPairingInodeText(input.StorageInode, input.StorageInodeNumber, input.StorageInodeNumeric)
	if !inodeOK {
		return genericStorageIdentity{}, false
	}
	op, opOK := strictPairingScalar(input.StorageOperation)
	if !opOK {
		return genericStorageIdentity{}, false
	}
	identity := genericStorageIdentity{
		Layer: profile.Layer, Base: profile.SemanticBase,
		Dev:   firstNonEmpty(dev, "unknown"),
		Inode: firstNonEmpty(inode, "-"),
		Op:    firstNonEmpty(normalizeFileRW(op), fileOperationFromEventName(name), profile.SemanticBase),
		PID:   int(input.HeaderTID),
	}
	switch {
	case strings.HasPrefix(lowerName, "android_fs_dataread"):
		identity.Op = "read"
	case strings.HasPrefix(lowerName, "android_fs_datawrite"):
		identity.Op = "write"
	}
	return identity, true
}

func pairingEndpointBase(family PairingEndpointFamily, phase PairingEndpointPhase, idleAllowed, positiveEmitter bool) PairingEndpointVerdict {
	return PairingEndpointVerdict{
		Recognized: true, Family: family, Phase: phase,
		IdleAllowed: idleAllowed, RequiresPositiveEmitter: positiveEmitter,
	}
}

func validNonNegativePairingEmitter(tid int64) bool {
	return tid >= 0 && tid <= math.MaxInt32
}

func canonicalPositiveDecimalIdentity(raw string) (string, bool) {
	var ok bool
	raw, ok = strictPairingScalar(raw)
	if !ok {
		return "", false
	}
	if !isAllDigits(raw) {
		return "", false
	}
	value, err := strconv.ParseUint(raw, 10, 63)
	if err != nil || value == 0 {
		return "", false
	}
	return strconv.FormatUint(value, 10), true
}

func canonicalTypedPositiveIdentity(raw string, numeric uint64, numericKnown bool) (string, bool) {
	text, textKnown := canonicalPositiveDecimalIdentity(raw)
	if !numericKnown {
		return text, textKnown
	}
	if numeric == 0 || numeric > math.MaxInt64 || (strings.TrimSpace(raw) != "" && !textKnown) {
		return "", false
	}
	canonical := strconv.FormatUint(numeric, 10)
	if textKnown && text != canonical {
		return "", false
	}
	return canonical, true
}

func canonicalUnsignedTraceIdentity(raw string) (string, bool) {
	var ok bool
	raw, ok = strictPairingScalar(raw)
	if !ok {
		return "", false
	}
	if raw == "" || strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "-") {
		return "", false
	}
	base := 10
	if strings.HasPrefix(strings.ToLower(raw), "0x") {
		base = 16
		raw = raw[2:]
		if raw == "" {
			return "", false
		}
	}
	value, err := strconv.ParseUint(raw, base, 64)
	if err != nil {
		return "", false
	}
	return strconv.FormatUint(value, 10), true
}

func canonicalTypedUnsignedIdentity(raw string, numeric uint64, numericKnown bool) (string, bool) {
	text, textKnown := canonicalUnsignedTraceIdentity(raw)
	if !numericKnown {
		return text, textKnown
	}
	if strings.TrimSpace(raw) != "" && !textKnown {
		return "", false
	}
	canonical := strconv.FormatUint(numeric, 10)
	if textKnown && text != canonical {
		return "", false
	}
	return canonical, true
}

func canonicalWorkqueuePointerIdentity(raw string) (string, bool) {
	var ok bool
	raw, ok = strictPairingScalar(raw)
	if !ok {
		return "", false
	}
	raw = strings.TrimRight(raw, ":")
	hadHexPrefix := strings.HasPrefix(strings.ToLower(raw), "0x")
	if hadHexPrefix {
		raw = raw[2:]
	}
	if raw == "" || len(raw) > 16 || (!hadHexPrefix && len(raw) != 8 && len(raw) != 16) {
		return "", false
	}
	value, err := strconv.ParseUint(raw, 16, 64)
	if err != nil || value == 0 {
		return "", false
	}
	return strconv.FormatUint(value, 16), true
}

func canonicalTypedWorkqueueIdentity(raw string, address uint64, addressKnown bool) (string, bool) {
	text, textKnown := canonicalWorkqueuePointerIdentity(raw)
	if !addressKnown {
		return text, textKnown
	}
	if address == 0 || (strings.TrimSpace(raw) != "" && !textKnown) {
		return "", false
	}
	canonical := strconv.FormatUint(address, 16)
	if textKnown && text != canonical {
		return "", false
	}
	return canonical, true
}

func canonicalTypedPairingDevice(raw string, numeric uint64, numericKnown bool) (string, bool) {
	text, textKnown := canonicalBlockDevice(raw)
	if !numericKnown {
		return text, textKnown
	}
	if strings.TrimSpace(raw) != "" && !textKnown {
		return "", false
	}
	canonical, canonicalOK := canonicalBlockDevice(fmt.Sprintf("%d,%d", numeric>>20, numeric&0xfffff))
	if !canonicalOK || (textKnown && text != canonical) {
		return "", false
	}
	return canonical, true
}

func typedPairingDeviceText(raw string, numeric uint64, numericKnown bool) (string, bool) {
	text, textOK := strictPairingScalar(raw)
	if !textOK {
		return "", false
	}
	if !numericKnown {
		return canonicalGenericStorageDeviceValidated(text)
	}
	canonical, canonicalOK := canonicalTypedPairingDevice("", numeric, true)
	if !canonicalOK {
		return "", false
	}
	if text != "" {
		textCanonical, canonicalTextOK := canonicalGenericStorageDeviceValidated(text)
		if !canonicalTextOK || textCanonical != canonical {
			return "", false
		}
	}
	return canonical, true
}

func typedPairingInodeText(raw string, numeric uint64, numericKnown bool) (string, bool) {
	rawText, rawOK := strictPairingScalar(raw)
	if !rawOK {
		return "", false
	}
	text, textOK := canonicalGenericStorageInodeValidated(rawText)
	if !textOK {
		return "", false
	}
	if !numericKnown {
		return text, true
	}
	canonical := strconv.FormatUint(numeric, 10)
	if text != "" && text != canonical {
		return "", false
	}
	return canonical, true
}

// strictUniquePairingAlias admits exactly one occurrence from an alias set.
// Duplicate or conflicting hard identities are not resolved by map overwrite
// order; the caller receives an unkeyable recognized endpoint and fail-closes
// the affected source/family according to its completeness contract.
func strictUniquePairingAlias(fieldText string, aliases ...string) (string, bool) {
	tokens, lexOK := tokenizePairingKV(fieldText)
	return strictUniquePairingAliasTokens(tokens, lexOK, aliases...)
}

func strictUniquePairingAliasTokens(tokens []pairingKVToken, lexOK bool, aliases ...string) (string, bool) {
	value, present, valid := strictOptionalPairingAliasTokens(tokens, lexOK, aliases...)
	return value, present && valid && value != ""
}

func strictOptionalPairingAliasTokens(tokens []pairingKVToken, lexOK bool, aliases ...string) (value string, present, valid bool) {
	if !lexOK {
		return "", false, false
	}
	valid = true
	for _, token := range tokens {
		if !pairingAliasAllowed(token.key, aliases) {
			continue
		}
		if present {
			return "", true, false
		}
		value, valid = strictPairingScalar(token.rawValue)
		if !valid || value == "" {
			return "", true, false
		}
		present = true
	}
	return value, present, valid
}

func pairingAliasAllowed(key string, aliases []string) bool {
	for _, alias := range aliases {
		if key == alias {
			return true
		}
	}
	return false
}

type pairingKVToken struct {
	key      string
	rawValue string
}

func tokenizePairingKV(fieldText string) ([]pairingKVToken, bool) {
	rawTokens, ok := tokenizeSchedSwitchSuffix(fieldText)
	if !ok {
		return nil, false
	}
	out := make([]pairingKVToken, 0, len(rawTokens))
	for i := 0; i < len(rawTokens); i++ {
		raw := strings.TrimSpace(rawTokens[i])
		if raw == "" {
			continue
		}
		key, value, found := strings.Cut(raw, "=")
		if found {
			if value == "" && i+1 < len(rawTokens) {
				next := strings.TrimSpace(rawTokens[i+1])
				if !pairingRawTokenDeclaresKey(next) {
					if scalar, scalarOK := strictPairingScalar(next); scalarOK && scalar != "" {
						i++
						value = next
					}
				}
			}
		} else if i+1 < len(rawTokens) && rawTokens[i+1] == "=" {
			key = raw
			i++
			if i+1 < len(rawTokens) {
				next := strings.TrimSpace(rawTokens[i+1])
				if !pairingRawTokenDeclaresKey(next) {
					if scalar, scalarOK := strictPairingScalar(next); scalarOK && scalar != "" {
						i++
						value = next
					}
				}
			}
			found = true
		} else if colon := strings.IndexByte(raw, ':'); colon > 0 {
			key, value, found = raw[:colon], raw[colon+1:], true
		}
		key = strings.TrimSpace(key)
		if !found || !isTraceKVKey(key) {
			continue
		}
		out = append(out, pairingKVToken{key: key, rawValue: value})
	}
	return out, true
}

func pairingRawTokenDeclaresKey(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "=" {
		return true
	}
	if key, _, found := strings.Cut(raw, "="); found && isTraceKVKey(strings.TrimSpace(key)) {
		return true
	}
	if colon := strings.IndexByte(raw, ':'); colon > 0 && isTraceKVKey(strings.TrimSpace(raw[:colon])) {
		return true
	}
	return false
}

func strictPairingScalar(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, ",")
	if raw == "" {
		return "", true
	}
	firstQuoted := raw[0] == '\'' || raw[0] == '"'
	lastQuoted := raw[len(raw)-1] == '\'' || raw[len(raw)-1] == '"'
	if firstQuoted || lastQuoted {
		if len(raw) < 2 || !firstQuoted || !lastQuoted || raw[0] != raw[len(raw)-1] {
			return "", false
		}
		raw = raw[1 : len(raw)-1]
	}
	if raw == "" || strings.ContainsAny(raw, "'\"= \t\r\n") || len(raw) > 256 {
		return "", false
	}
	return raw, true
}

// strictCoherentPairingAlias is reserved for non-endpoint metadata rows whose
// established canonical SQL wire may carry two distinct aliases. Repeating
// one physical key or disagreeing canonical values remains ambiguous.
func strictCoherentPairingAlias(fieldText string, aliases ...string) (string, bool) {
	seen := map[string]bool{}
	canonical := ""
	found := false
	tokens, lexOK := tokenizePairingKV(fieldText)
	if !lexOK {
		return "", false
	}
	for _, token := range tokens {
		key := token.key
		if !pairingAliasAllowed(key, aliases) {
			continue
		}
		if seen[key] {
			return "", false
		}
		seen[key] = true
		value, ok := canonicalPositiveDecimalIdentity(token.rawValue)
		if !ok || (found && value != canonical) {
			return "", false
		}
		canonical = value
		found = true
	}
	return canonical, found
}

// encodePairingKey is injective for arbitrary byte strings. Pairing payloads
// are normally single-line text, but correctness must not rely on a delimiter
// never appearing in a future vendor token.
func encodePairingKey(parts ...string) string {
	total := 0
	for _, part := range parts {
		total += binary.MaxVarintLen64 + len(part)
	}
	out := make([]byte, 0, total)
	var prefix [binary.MaxVarintLen64]byte
	for _, part := range parts {
		n := binary.PutUvarint(prefix[:], uint64(len(part)))
		out = append(out, prefix[:n]...)
		out = append(out, part...)
	}
	return string(out)
}

func genericStoragePairingProfile(name string) (pairingEndpointProfile, bool) {
	typ := classifyEventType("", name, "")
	if typ != EventStorage && typ != EventFilesystem {
		return pairingEndpointProfile{}, false
	}
	ev := Event{Name: name, Type: typ}
	layer := storageLatencyLayer(ev)
	base, phase := storageLatencyBaseAndPhase(ev)
	if layer == "" || base == "" || phase == "" {
		return pairingEndpointProfile{}, false
	}
	return pairingEndpointProfile{Family: PairingEndpointStorage, Phase: PairingEndpointPhase(phase), SemanticBase: base, Layer: layer, IdleAllowed: true}, true
}

type genericStorageWireAdmission struct {
	identityKnown   bool
	payloadAdmitted bool
	dev             string
	inode           string
	op              string
}

func strictOrderedSpacePairingFields(fieldText string, labels []string) (map[string]string, bool) {
	rawTokens, lexOK := tokenizeSchedSwitchSuffix(fieldText)
	values := make(map[string]string, len(labels))
	valid := lexOK && len(rawTokens) == len(labels)*2
	for i, label := range labels {
		position := i * 2
		if position+1 >= len(rawTokens) {
			valid = false
			break
		}
		if strings.TrimSpace(rawTokens[position]) != label {
			valid = false
			break
		}
		value, scalarOK := strictPairingScalar(rawTokens[position+1])
		if !scalarOK || value == "" {
			valid = false
			break
		}
		values[label] = value
	}
	return values, valid
}

// genericStorageSpaceWireAdmission covers exact upstream ext4 profiles whose
// official formatter uses alternating `label value` fields rather than KV.
// It is deliberately a closed name/label registry: the generic KV lexer is
// not widened, so quoted prose and future vendor bodies cannot mint keys by
// accidentally resembling one field pair.
func genericStorageSpaceWireAdmission(name, fieldText string) (genericStorageWireAdmission, bool) {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	var labels []string
	switch lowerName {
	case "ext4_da_write_begin":
		labels = []string{"dev", "ino", "pos", "len", "flags"}
	case "ext4_da_write_end":
		labels = []string{"dev", "ino", "pos", "len", "copied"}
	case "ext4_sync_file_enter":
		labels = []string{"dev", "ino", "parent", "datasync"}
	case "ext4_sync_file_exit":
		labels = []string{"dev", "ino", "ret"}
	default:
		return genericStorageWireAdmission{}, false
	}
	values, structuralOK := strictOrderedSpacePairingFields(fieldText, labels)
	dev, inode := values["dev"], values["ino"]
	devOK := dev != "" && pairingDevTScalarValid(dev)
	inodeOK := inode != "" && pairingUnsignedScalarValid(inode, true)
	payloadOK := structuralOK && devOK && inodeOK
	for _, label := range labels[2:] {
		value, present := values[label]
		signed := label == "pos" || label == "datasync" || label == "ret"
		if signed {
			payloadOK = payloadOK && pairingSignedScalarValid(value, present)
		} else {
			payloadOK = payloadOK && pairingUnsignedScalarValid(value, present)
		}
	}
	return genericStorageWireAdmission{
		identityKnown: devOK && inodeOK, payloadAdmitted: payloadOK,
		dev: dev, inode: inode,
	}, true
}

func genericStorageWireAlias(tokens []pairingKVToken, lexOK bool, aliases ...string) (string, bool, bool) {
	value, present, valid := strictOptionalPairingAliasTokens(tokens, lexOK, aliases...)
	return value, present, valid
}

func genericStorageWireAdmissionFor(name, fieldText string, tokens []pairingKVToken, lexOK bool) genericStorageWireAdmission {
	if admission, ok := genericStorageSpaceWireAdmission(name, fieldText); ok {
		return admission
	}
	lowerName := strings.ToLower(strings.TrimSpace(name))
	dev, devPresent, devValid := genericStorageWireAlias(tokens, lexOK, "dev", "s_dev", "fs_dev", "dev_t", "sdev", "dev_name", "devname")
	inode, inodePresent, inodeValid := genericStorageWireAlias(tokens, lexOK, "ino", "inode", "i_ino")
	op, opPresent, opValid := genericStorageWireAlias(tokens, lexOK, "rw", "rwbs", "op", "operation", "opcode", "cmd_opcode")
	bytes, bytesPresent, bytesValid := genericStorageWireAlias(tokens, lexOK, "bytes", "len", "length", "size", "transfer_len", "bytes_xfered")
	tag, tagPresent, tagValid := genericStorageWireAlias(tokens, lexOK, "tag")
	lba, lbaPresent, lbaValid := genericStorageWireAlias(tokens, lexOK, "lba", "sector", "blk_addr")
	blocks, blocksPresent, blocksValid := genericStorageWireAlias(tokens, lexOK, "blocks")
	blockSize, blockSizePresent, blockSizeValid := genericStorageWireAlias(tokens, lexOK, "block_size")
	ret, retPresent, retValid := genericStorageWireAlias(tokens, lexOK, "ret", "res", "error", "err")
	cmdErr, cmdErrPresent, cmdErrValid := genericStorageWireAlias(tokens, lexOK, "cmd_err")
	dataErr, dataErrPresent, dataErrValid := genericStorageWireAlias(tokens, lexOK, "data_err")
	stopErr, stopErrPresent, stopErrValid := genericStorageWireAlias(tokens, lexOK, "stop_err")
	sbcErr, sbcErrPresent, sbcErrValid := genericStorageWireAlias(tokens, lexOK, "sbc_err")
	bytesValid = bytesValid && pairingUnsignedScalarValid(bytes, bytesPresent)
	if strings.HasPrefix(lowerName, "mmc_") || strings.HasPrefix(lowerName, "scsi_") {
		tagValid = tagValid && pairingSignedScalarValid(tag, tagPresent)
	} else {
		tagValid = tagValid && pairingUnsignedScalarValid(tag, tagPresent)
	}
	lbaValid = lbaValid && pairingUnsignedScalarValid(lba, lbaPresent)
	blocksValid = blocksValid && pairingUnsignedScalarValid(blocks, blocksPresent)
	blockSizeValid = blockSizeValid && pairingUnsignedScalarValid(blockSize, blockSizePresent)
	retValid = retValid && pairingSignedScalarValid(ret, retPresent)
	cmdErrValid = cmdErrValid && pairingSignedScalarValid(cmdErr, cmdErrPresent)
	dataErrValid = dataErrValid && pairingSignedScalarValid(dataErr, dataErrPresent)
	stopErrValid = stopErrValid && pairingSignedScalarValid(stopErr, stopErrPresent)
	sbcErrValid = sbcErrValid && pairingSignedScalarValid(sbcErr, sbcErrPresent)
	if devPresent && !strings.HasPrefix(lowerName, "mmc_") && !strings.HasPrefix(lowerName, "ufshcd_") {
		devValid = devValid && pairingDevTScalarValid(dev)
	}
	if inodePresent {
		inodeValid = inodeValid && pairingUnsignedScalarValid(inode, true)
	}
	validHard := devValid && inodeValid && opValid
	validPayload := validHard && bytesValid && tagValid && lbaValid && blocksValid && blockSizeValid && retValid && cmdErrValid && dataErrValid && stopErrValid && sbcErrValid
	admission := genericStorageWireAdmission{dev: dev, inode: inode, op: op}
	switch {
	case lowerName == "mmc_request_start":
		devicePresent := devPresent || mmcPositionalDevicePresent(fieldText)
		legacy := devPresent && opPresent
		full := devicePresent && tagPresent && opPresent && blocksPresent && blockSizePresent && lbaPresent
		// The established MMC coarse key is source/layer/base/PID; request
		// metadata (tag/blocks/block size/LBA) remains deliberately outside it
		// until a production request-token witness exists. Once the device/op
		// envelope identifies this endpoint, malformed non-key metadata must
		// quarantine that exact lane rather than escalate to source scope.
		admission.identityKnown = validHard && devicePresent && opPresent
		admission.payloadAdmitted = admission.identityKnown && validPayload && (legacy || full)
	case lowerName == "mmc_request_done":
		devicePresent := devPresent || mmcPositionalDevicePresent(fieldText)
		legacy := devPresent && opPresent
		errorPresent := retPresent || cmdErrPresent || dataErrPresent
		full := devicePresent && tagPresent && opPresent && bytesPresent && errorPresent
		admission.identityKnown = validHard && devicePresent && opPresent
		admission.payloadAdmitted = admission.identityKnown && validPayload && (legacy || full)
	case strings.HasPrefix(lowerName, "scsi_"):
		// SCSI coarse identity uses dev/base/PID; tag/LBA/length/opcode are
		// payload evidence only and cannot turn a known lane into an unknown
		// source-family failure.
		admission.identityKnown = devPresent && devValid
		full := tagPresent && lbaPresent && bytesPresent && opPresent
		legacy := opPresent && bytesPresent
		admission.payloadAdmitted = admission.identityKnown && validPayload && (full || legacy)
	case strings.HasPrefix(lowerName, "ufshcd_"):
		// UFS's established coarse key deliberately permits dev=unknown and
		// does not include tag/opcode. Exact name+emitter therefore locate the
		// lane when dev is absent; malformed non-key tag/op metadata can only
		// lower PayloadAdmitted, never escalate an exact-lane quarantine to the
		// whole physical source. A present dev remains a hard key field.
		admission.identityKnown = lexOK && (!devPresent || devValid)
		admission.payloadAdmitted = admission.identityKnown && tagPresent && opPresent
		admission.payloadAdmitted = admission.payloadAdmitted && validPayload
	case strings.HasPrefix(lowerName, "android_fs_dataread"), strings.HasPrefix(lowerName, "android_fs_datawrite"):
		admission.identityKnown = devValid && inodeValid && (devPresent || inodePresent)
		admission.payloadAdmitted = admission.identityKnown && validPayload && inodePresent && bytesPresent
	case strings.HasPrefix(lowerName, "f2fs_direct_io"), strings.HasPrefix(lowerName, "f2fs_sync_file"):
		admission.identityKnown = validHard && devPresent && inodePresent
		admission.payloadAdmitted = admission.identityKnown && validPayload
	default:
		admission.identityKnown = validHard && (devPresent || inodePresent || opPresent || bytesPresent || tagPresent || lbaPresent)
		admission.payloadAdmitted = admission.identityKnown && validPayload
	}
	return admission
}

func pairingUnsignedScalarValid(raw string, present bool) bool {
	if !present {
		return true
	}
	_, ok := canonicalUnsignedTraceIdentity(raw)
	return ok
}

func pairingSignedScalarValid(raw string, present bool) bool {
	if !present {
		return true
	}
	base := 10
	unsigned := strings.TrimPrefix(strings.TrimPrefix(raw, "+"), "-")
	if strings.HasPrefix(strings.ToLower(unsigned), "0x") {
		base = 0
	}
	_, err := strconv.ParseInt(raw, base, 64)
	return err == nil
}

func pairingDevTScalarValid(raw string) bool {
	if _, ok := canonicalBlockDevice(raw); ok {
		return true
	}
	if !isAllDigits(raw) {
		return false
	}
	_, err := strconv.ParseUint(raw, 10, 32)
	return err == nil
}

func mmcPositionalDevicePresent(fieldText string) bool {
	fields := strings.Fields(strings.TrimSpace(fieldText))
	if len(fields) == 0 || strings.Contains(fields[0], "=") {
		return false
	}
	_, ok := strictPairingScalar(fields[0])
	return ok
}

func decodeGenericStoragePairingEndpoint(input PairingEndpointTypedInput, fieldText string, tokens []pairingKVToken, lexOK bool) (PairingEndpointVerdict, genericStorageWireAdmission) {
	typ := classifyEventType("", input.Name, fieldText)
	if typ != EventStorage && typ != EventFilesystem {
		return PairingEndpointVerdict{}, genericStorageWireAdmission{}
	}
	ev := Event{Name: input.Name, PID: int(input.HeaderTID), Type: typ, FieldText: fieldText, ResourceFields: &ResourceFields{}, FileFields: &FileFields{}}
	interner := newStringInterner()
	kv := parseKV(fieldText)
	admission := genericStorageWireAdmissionFor(input.Name, fieldText, tokens, lexOK)
	input.StorageIdentityKnown = admission.identityKnown
	input.StoragePayloadAdmissionKnown = true
	input.StoragePayloadAdmitted = admission.payloadAdmitted
	if !populateResourceFields(&ev, kv, interner) {
		input.StoragePayloadAdmitted = false
		input.StorageDevice = admission.dev
		input.StorageInode = admission.inode
		input.StorageOperation = admission.op
		return FingerprintPairingEndpoint(input), admission
	}
	populateFileIOFields(&ev, kv, interner)
	_, _, endpoint := genericStorageEndpoint(ev)
	if !endpoint {
		return PairingEndpointVerdict{}, admission
	}
	input.StorageIdentityKnown = admission.identityKnown
	input.StorageDevice = admission.dev
	input.StorageInode = admission.inode
	input.StorageOperation = admission.op
	return FingerprintPairingEndpoint(input), admission
}

package hitraceconv

import (
	"math"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// traceDBRawPairingOwner recovers only the physical ftrace row-header TID.
// It is intentionally narrower than publication admission: a conflicting PID
// can reject the row while the exact header TID still safely localizes a lane.
// A present-but-invalid/unresolvable ITID, or an ITID/TID disagreement, never
// falls back to the public TID claim.
func traceDBRawPairingOwner(raw traceDBRawEvent, authority traceDBSchedulerAuthority, identityScalarsValid bool) (tid int64, known bool, canonicalITID int64, canonicalKnown bool) {
	if !identityScalarsValid || !authority.initialized {
		return 0, false, 0, false
	}
	if raw.ITIDKnown {
		if raw.ITID == 0 {
			if raw.TIDKnown && raw.TID != 0 {
				return 0, false, 0, false
			}
			if _, ok := authority.schedulerSubjectFromExactITID(0, true); !ok {
				return 0, false, 0, false
			}
			return 0, true, 0, true
		}
		thread, _, resolution := authority.resolveThreadSubject(raw.ITID)
		if resolution != traceDBSchedulerThreadResolved || !traceDBRawThreadIdentityValid(thread) ||
			(raw.TIDKnown && raw.TID != thread.TID) {
			return 0, false, 0, false
		}
		return thread.TID, true, thread.ITID, true
	}
	if !raw.TIDKnown {
		return 0, false, 0, false
	}
	// An exact public TID is the physical row-header owner even when the
	// scheduler inventory cannot prove a unique generation. Generation proof
	// remains separate and is required before WQ/DMA/storage publication.
	tid, known = raw.TID, true
	if raw.TID == 0 {
		return tid, known, 0, false
	}
	items := authority.identities.ByTIDCandidates[raw.TID]
	var selected int64
	matches := 0
	for _, candidate := range items {
		thread, process, resolution := authority.resolveThreadSubject(candidate.ITID)
		if resolution != traceDBSchedulerThreadResolved {
			continue
		}
		if raw.PIDKnown && process.PID != raw.PID {
			continue
		}
		selected = thread.ITID
		matches++
	}
	if matches == 1 {
		canonicalITID, canonicalKnown = selected, true
	}
	return tid, known, canonicalITID, canonicalKnown
}

// traceDBRawPairingVerdict is the sole SQL-raw adapter into tracequery's
// exported fingerprint authority. It prepares typed fields only; family,
// phase, semantic base and hard-key construction remain owned by tracequery.
func traceDBRawPairingVerdict(name string, headerTID int64, args map[string]traceDBValue, invalidKeys map[string]bool, argsKnown bool) tracequery.PairingEndpointVerdict {
	// SQL raw F2FS publication is an explicitly deferred capability: the raw
	// tables have no source-pinned schema/witness for these six producer
	// profiles.  Keep them unsupported/inventory-only rather than letting the
	// shared tracequery fingerprint turn a lower-case sync name into a stage
	// poison or a compatibility argset into a newly claimed producer lane.
	if directF2FSNameGoverned(name) {
		return tracequery.PairingEndpointVerdict{}
	}
	input := tracequery.PairingEndpointTypedInput{Name: name, HeaderTID: headerTID}
	if argsKnown {
		lower := strings.ToLower(strings.TrimSpace(name))
		switch lower {
		case "binder_transaction", "binder_transaction_received":
			if value, ok := traceDBRawTypedPositiveInt(args, invalidKeys, "transaction", "debug_id", "transaction_id"); ok {
				input.TransactionNumber, input.TransactionNumberKnown = value, true
			}
		case "workqueue_execute_start", "workqueue_execute_end":
			if value, ok := traceDBRawTypedPointer(args, invalidKeys, "work", "addr", "address"); ok {
				input.WorkAddress, input.WorkAddressKnown = value, true
			}
		case "dma_fence_wait_start", "dma_fence_wait_end":
			input.Driver, _ = traceDBRawTypedWireText(args, invalidKeys, true, "driver")
			input.Timeline, _ = traceDBRawTypedWireText(args, invalidKeys, true, "timeline")
			if value, ok := traceDBRawTypedUnsignedInt(args, invalidKeys, true, "context"); ok {
				input.ContextNumber, input.ContextNumberKnown = value, true
			}
			if value, ok := traceDBRawTypedUnsignedInt(args, invalidKeys, true, "seqno"); ok {
				input.SeqnoNumber, input.SeqnoNumberKnown = value, true
			}
		case "block_rq_issue", "block_rq_complete", "block_bio_queue", "block_bio_complete":
			traceDBRawPopulateBlockPairingInput(&input, lower, args, invalidKeys)
		default:
			traceDBRawPopulateStoragePairingInput(&input, lower, args, invalidKeys)
		}
	}
	return fingerprintPairingEndpoint(input)
}

func traceDBRawAliasPresence(args map[string]traceDBValue, names ...string) bool {
	for _, name := range names {
		if _, ok := args[strings.ToLower(strings.TrimSpace(name))]; ok {
			return true
		}
	}
	return false
}

func traceDBRawTypedAlias(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, names ...string) (string, int64, bool, bool) {
	present, occurrences := false, 0
	for _, name := range names {
		if _, exists := args[strings.ToLower(strings.TrimSpace(name))]; exists {
			present = true
			occurrences++
		}
	}
	// Hard endpoint fields follow tracequery's strictUniquePairingAlias:
	// even byte-equal duplicate aliases are ambiguous source evidence and may
	// not be collapsed into one clean rendered key.
	if occurrences > 1 {
		return "", 0, true, false
	}
	text, ok := traceDBRawValidatedAlias(args, invalidKeys, required, names...)
	if !ok {
		return "", 0, present, false
	}
	if !present {
		return "", 0, false, !required
	}
	datatype := int64(-1)
	for _, name := range names {
		if value, exists := args[strings.ToLower(strings.TrimSpace(name))]; exists {
			if datatype == -1 {
				datatype = value.Datatype
			} else if datatype != value.Datatype {
				return "", 0, true, false
			}
		}
	}
	return text, datatype, true, true
}

func traceDBRawTypedPositiveInt(args map[string]traceDBValue, invalidKeys map[string]bool, names ...string) (uint64, bool) {
	value, ok := traceDBRawTypedUnsignedInt(args, invalidKeys, true, names...)
	return value, ok && value > 0 && value <= math.MaxInt64
}

func traceDBRawTypedUnsignedInt(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, names ...string) (uint64, bool) {
	text, datatype, present, ok := traceDBRawTypedAlias(args, invalidKeys, required, names...)
	if !ok || !present || datatype != 0 {
		return 0, false
	}
	value, err := strconv.ParseUint(text, 10, 64)
	return value, err == nil
}

func traceDBRawTypedSignedInt(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, names ...string) (int64, bool) {
	text, datatype, present, ok := traceDBRawTypedAlias(args, invalidKeys, required, names...)
	if !ok || !present || datatype != 0 {
		return 0, false
	}
	value, err := strconv.ParseInt(text, 10, 64)
	return value, err == nil
}

func traceDBRawTypedPointer(args map[string]traceDBValue, invalidKeys map[string]bool, names ...string) (uint64, bool) {
	text, datatype, present, ok := traceDBRawTypedAlias(args, invalidKeys, true, names...)
	if !ok || !present || datatype != 0 {
		return 0, false
	}
	base := 10
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		base, text = 16, text[2:]
	}
	value, err := strconv.ParseUint(text, base, 64)
	return value, err == nil && value > 0
}

func traceDBRawTypedWireText(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, names ...string) (string, bool) {
	text, datatype, present, ok := traceDBRawTypedAlias(args, invalidKeys, required, names...)
	if !ok || required && !present {
		return "", false
	}
	if !present {
		return "", true
	}
	return text, datatype == 1 && text != "" && !strings.ContainsAny(text, " \t\r\n=")
}

func traceDBRawTypedDevice(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, names ...string) (text string, numeric uint64, numericKnown, present, valid bool) {
	var datatype int64
	text, datatype, present, valid = traceDBRawTypedAlias(args, invalidKeys, required, names...)
	if !valid || !present {
		return text, 0, false, present, valid && !required
	}
	if datatype == 0 {
		numeric, err := strconv.ParseUint(text, 10, 32)
		return "", numeric, err == nil, true, err == nil
	}
	if datatype != 1 || !traceDBRawDeviceAlias(args, invalidKeys, names...) {
		return "", 0, false, true, false
	}
	return text, 0, false, true, true
}

func traceDBRawTypedInode(args map[string]traceDBValue, invalidKeys map[string]bool, required bool, names ...string) (text string, numeric uint64, numericKnown, present, valid bool) {
	text, datatype, present, valid := traceDBRawTypedAlias(args, invalidKeys, required, names...)
	if !valid || !present {
		return "", 0, false, present, valid && !required
	}
	if datatype == 0 {
		value, err := strconv.ParseUint(text, 10, 64)
		return "", value, err == nil, true, err == nil
	}
	if datatype != 1 {
		return "", 0, false, true, false
	}
	valueText, base := text, 10
	if strings.HasPrefix(strings.ToLower(valueText), "0x") {
		valueText, base = valueText[2:], 16
	}
	_, err := strconv.ParseUint(valueText, base, 64)
	return text, 0, false, true, err == nil && valueText != ""
}

func traceDBRawPopulateBlockPairingInput(input *tracequery.PairingEndpointTypedInput, name string, args map[string]traceDBValue, invalidKeys map[string]bool) {
	kind, kindOK := blockRenderKindForName(name)
	dev, devOK := traceDBBlockDevice(args, invalidKeys, "dev", "dev_t")
	readUint := func(max uint64, names ...string) (uint64, bool) {
		value, present, valid := traceDBBlockArg(args, invalidKeys, names...)
		if !present || !valid || value.Datatype != 0 || value.Text != strings.TrimSpace(value.Text) {
			return 0, false
		}
		parsed, err := strconv.ParseUint(value.Text, 10, 64)
		return parsed, err == nil && parsed <= max
	}
	sector, sectorOK := readUint(math.MaxInt64, "sector", "lba")
	length, lengthOK := readUint(math.MaxUint32, "nr_sector", "nr_sectors", "sectors")
	rwbs, rwbsOK := traceDBRawTypedWireText(args, invalidKeys, true, "rwbs", "rw", "op")
	identityKnown := kindOK && devOK && sectorOK && lengthOK && rwbsOK && validBlockRWBS(rwbs)
	input.BlockIdentityKnown = identityKnown
	input.BlockPayloadAdmissionKnown = true
	if identityKnown {
		input.BlockDeviceNumber, input.BlockDeviceNumeric = uint64(dev), true
		input.BlockOperation = rwbs
		input.BlockSector, input.BlockLength = int64(sector), int64(length)
	}
	_, fullOK := decodeTraceDBBlockPayload(name, args, invalidKeys)
	input.BlockPayloadAdmitted = fullOK && (kind == blockRenderRQIssue || kind == blockRenderRQComplete || kind == blockRenderBioQueue || kind == blockRenderBioComplete)
}

func traceDBRawPopulateStoragePairingInput(input *tracequery.PairingEndpointTypedInput, name string, args map[string]traceDBValue, invalidKeys map[string]bool) {
	devText, devNumber, devNumeric, devPresent, devOK := traceDBRawTypedDevice(args, invalidKeys, false,
		"dev", "s_dev", "fs_dev", "dev_t", "sdev", "dev_name", "devname")
	inodeText, inode, inodeNumeric, inodePresent, inodeOK := traceDBRawTypedInode(args, invalidKeys, false, "ino", "inode", "i_ino")
	opPresent := traceDBRawAliasPresence(args, "rw", "rwbs", "op", "operation", "opcode", "cmd_opcode")
	op, opOK := traceDBRawTypedWireText(args, invalidKeys, false, "rw", "rwbs", "op", "operation", "opcode", "cmd_opcode")
	input.StorageDevice, input.StorageDeviceNumber, input.StorageDeviceNumeric = devText, devNumber, devNumeric
	if inodePresent && inodeOK {
		input.StorageInode, input.StorageInodeNumber, input.StorageInodeNumeric = inodeText, inode, inodeNumeric
	}
	input.StorageOperation = op
	input.StoragePayloadAdmissionKnown = true
	input.StoragePayloadAdmitted = traceDBRawRequiredArgs(name, args, invalidKeys)
	switch {
	case name == "mmc_request_start" || name == "mmc_request_done":
		device, deviceOK := traceDBRawTypedWireText(args, invalidKeys, true, "name", "dev_name")
		opcode, opcodeOK := traceDBRawTypedUnsignedInt(args, invalidKeys, true, "cmd_opcode", "opcode")
		deviceOK = deviceOK && validProfilerMMCName(device)
		input.StorageDevice = device
		input.StorageOperation = strconv.FormatUint(opcode, 10)
		input.StorageIdentityKnown = deviceOK && opcodeOK
	case strings.HasPrefix(name, "scsi_"):
		input.StorageIdentityKnown = devPresent && devOK
	case strings.HasPrefix(name, "ufshcd_"):
		devRaw, datatype, present, valid := traceDBRawTypedAlias(args, invalidKeys, false, "dev", "dev_name", "devname")
		if present && valid {
			switch datatype {
			case 0:
				value, err := strconv.ParseUint(devRaw, 10, 32)
				valid = err == nil
				if valid {
					input.StorageDeviceNumber, input.StorageDeviceNumeric, input.StorageDevice = value, true, ""
				}
			case 1:
				valid = devRaw != "" && !strings.ContainsAny(devRaw, " \t\r\n=")
				if valid {
					input.StorageDevice, input.StorageDeviceNumeric = devRaw, false
				}
			default:
				valid = false
			}
		}
		input.StorageIdentityKnown = !present || valid
	case strings.HasPrefix(name, "android_fs_dataread"), strings.HasPrefix(name, "android_fs_datawrite"):
		input.StorageIdentityKnown = devPresent && devOK && inodePresent && inodeOK && opOK
	default:
		identityEvidence := devPresent || inodePresent || opPresent ||
			traceDBRawAliasPresence(args, "bytes", "len", "length", "size", "transfer_len", "bytes_xfered", "tag", "lba", "sector", "blk_addr")
		input.StorageIdentityKnown = devOK && inodeOK && opOK && identityEvidence
	}
}

func traceDBRawPairingWireParity(name, body string, headerTID int64, typed tracequery.PairingEndpointVerdict) bool {
	prefix := strings.TrimSpace(name) + ":"
	if !strings.HasPrefix(body, prefix) {
		return false
	}
	wire := tracequery.DecodePairingEndpoint(strings.TrimSpace(name), strings.TrimSpace(strings.TrimPrefix(body, prefix)), headerTID)
	return wire.Recognized == typed.Recognized && wire.KeyKnown == typed.KeyKnown &&
		wire.PayloadAdmitted == typed.PayloadAdmitted && wire.Family == typed.Family &&
		wire.Phase == typed.Phase && wire.SemanticKey == typed.SemanticKey &&
		wire.EmitterKnown == typed.EmitterKnown && wire.EmitterAdmitted == typed.EmitterAdmitted
}

package tracequery

import (
	"math"
	"strconv"
	"strings"
)

type ioActivityByteStatus uint8

const (
	ioActivityBytesUnknown ioActivityByteStatus = iota
	ioActivityBytesKnown
	ioActivityBytesInvalid
	ioActivityBytesOverflow
)

// Parse-time, immutable, full-wire handoff. Never reconstruct this from the
// clipped display preview or from legacy zero-default numeric projections.
// Admission here only licenses endpoint counting, never request pairing.
type ioActivityEndpoint struct {
	layer, family, phase, dev, direction, caliber string
	bytes                                         uint64
	byteStatus                                    ioActivityByteStatus
	admitted                                      bool
}

func parseBlockIOActivity(name, fields string, intern *stringInterner) *ioActivityEndpoint {
	var match []string
	item := &ioActivityEndpoint{layer: "block", caliber: "sector_bytes"}
	switch name {
	case "block_rq_issue":
		item.family, item.phase, item.caliber = "block_rq", "start", "request_bytes"
		match = blockRQIssueRE.FindStringSubmatch(strings.TrimSpace(fields))
	case "block_rq_complete":
		item.family, item.phase = "block_rq", "done"
		match = blockRQCompleteRE.FindStringSubmatch(strings.TrimSpace(fields))
	case "block_bio_queue":
		item.family, item.phase = "block_bio", "start"
		match = blockBioQueueRE.FindStringSubmatch(strings.TrimSpace(fields))
	case "block_bio_complete":
		item.family, item.phase = "block_bio", "done"
		match = blockBioCompleteRE.FindStringSubmatch(strings.TrimSpace(fields))
	default:
		return nil
	}
	dev, op, _, length, keyKnown, _ := parseBlockRequestFingerprint(name, fields)
	if !keyKnown || len(match) == 0 {
		return item
	}
	// Endpoint counting does not require a positive-length request or a
	// partner. Complete rows still require their complete non-key grammar.
	if item.phase == "done" && !blockSignedFits(match[len(match)-1], 32) {
		return item
	}
	item.dev, item.direction, item.admitted = intern.intern(dev), ioActivityBlockDirection(op), true
	item.byteStatus = ioActivityBytesKnown
	if name == "block_rq_issue" {
		value, err := strconv.ParseUint(match[3], 10, 32)
		if err != nil {
			item.byteStatus = ioActivityBytesOverflow
		} else {
			item.bytes = value
		}
	} else {
		// The validated native sector count is uint32; the kernel block
		// tracepoint unit is always 512 bytes, not device logical block size.
		item.bytes = uint64(length) * 512
	}
	return item
}

func ioActivityBlockDirection(op string) string {
	// Native RWBS begins with an operation followed by flag letters. A
	// conflicting second operation or arbitrary word is not an R/W event.
	if len(op) > 0 && (op[0] == 'R' || op[0] == 'W') {
		seen := map[byte]bool{}
		for i := 1; i < len(op); i++ {
			if !strings.ContainsRune("FASMPCVH", rune(op[i])) || seen[op[i]] {
				return "other"
			}
			seen[op[i]] = true
		}
		if op[0] == 'R' {
			return "read"
		}
		return "write"
	}
	return "other"
}

// Reuse the exact-body verdict and its existing allocation. The full-wire KV
// map only projects admitted scalars; it never licenses a body. Parsed remains
// separate from byte status so legacy/manual verdicts cannot become known zero.
func populateStorageIOActivity(name string, tokens map[string]string, retained *ResourceFields) {
	if profile, exact := exactMMCPairingProfile(name); exact {
		admission := retained.mmcPairing
		if admission == nil {
			return
		}
		admission.activityParsed = true
		if !admission.payloadAdmitted {
			return
		}
		admission.activityByteStatus = ioActivityBytesKnown
		if profile.Phase == PairingEndpointStart {
			blocks, _ := strconv.ParseUint(tokens["blocks"], 10, 64)
			size, _ := strconv.ParseUint(tokens["block_size"], 10, 64)
			if size != 0 && blocks > math.MaxUint64/size {
				admission.activityByteStatus = ioActivityBytesOverflow
			} else {
				admission.activityBytes = blocks * size
			}
		} else {
			admission.activityBytes, _ = strconv.ParseUint(tokens["bytes_xfered"], 10, 64)
		}
		return
	}
	profile, exact := exactF2FSPairingProfile(name)
	if !exact {
		return
	}
	admission := retained.f2fsPairing
	if admission == nil {
		return
	}
	admission.activityParsed = true
	if !admission.payloadAdmitted {
		return
	}
	if profile.SemanticBase == "f2fs_sync_file" {
		// i_size and i_blocks describe the inode, not this sync operation.
		return
	}
	admission.activityByteStatus = ioActivityBytesKnown
	key := "len"
	if profile.Phase == PairingEndpointDone {
		if profile.SemanticBase == "f2fs_write" {
			key = "copied"
		} else {
			key = "ret"
			if strings.HasPrefix(tokens[key], "-") {
				// A negative return code is not a zero-byte transfer.
				admission.activityByteStatus = ioActivityBytesInvalid
				return
			}
		}
	}
	admission.activityBytes, _ = strconv.ParseUint(tokens[key], 10, 64)
}

func ioActivityFromEvent(ev Event) (ioActivityEndpoint, bool) {
	if ev.BlockIOFields != nil && ev.BlockIOFields.ioActivity != nil {
		return *ev.BlockIOFields.ioActivity, true
	}
	if ev.ResourceFields == nil {
		return ioActivityEndpoint{}, false
	}
	if profile, exact := exactMMCPairingProfile(ev.Name); exact {
		admission := ev.ResourceFields.mmcPairing
		if admission == nil || !admission.activityParsed {
			return ioActivityEndpoint{}, false
		}
		item := ioActivityEndpoint{layer: "mmc", family: profile.SemanticBase, phase: string(profile.Phase), dev: admission.device,
			direction: "other", caliber: "request_bytes", bytes: admission.activityBytes, byteStatus: admission.activityByteStatus, admitted: admission.payloadAdmitted}
		if profile.Phase == PairingEndpointDone {
			item.caliber = "transferred_bytes"
		}
		switch admission.opcode {
		case "17", "18":
			item.direction = "read"
		case "24", "25":
			item.direction = "write"
		}
		return item, true
	}
	profile, exact := exactF2FSPairingProfile(ev.Name)
	if !exact || ev.ResourceFields.f2fsPairing == nil || !ev.ResourceFields.f2fsPairing.activityParsed {
		return ioActivityEndpoint{}, false
	}
	admission := ev.ResourceFields.f2fsPairing
	item := ioActivityEndpoint{layer: "f2fs", family: profile.SemanticBase, phase: string(profile.Phase), dev: admission.device,
		direction: admission.operation, caliber: "unspecified", bytes: admission.activityBytes, byteStatus: admission.activityByteStatus, admitted: admission.payloadAdmitted}
	if item.direction != "read" && item.direction != "write" {
		item.direction = "other"
	}
	if profile.SemanticBase != "f2fs_sync_file" {
		item.caliber = "request_bytes"
		if profile.Phase == PairingEndpointDone {
			item.caliber = "transferred_bytes"
			if profile.SemanticBase == "f2fs_write" {
				item.caliber = "copied_bytes"
			}
		}
	}
	return item, true
}

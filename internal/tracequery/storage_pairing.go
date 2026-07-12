package tracequery

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// genericStorageIdentity is the single hard/coarse lane identity used by
// generic storage elapsed pairing, the duration-order audit, and deterministic
// witness discovery.  It is intentionally not a request identity: missing
// vendor request tokens remain represented by the conservative dev/inode/op
// tuple, so concurrent equal lanes are suppressed rather than FIFO-guessed.
type genericStorageIdentity struct {
	Layer string
	Base  string
	Dev   string
	Inode string
	Op    string
	PID   int
}

// genericStorageReplayPlan is the physical-order authority for the only
// non-endpoint row that can mutate generic-storage pairing state: a scheduler
// lifecycle reset. Composite indexes are sorted into canonical timestamp order,
// so replaying idx.Events directly can move a regressed reset across a start or
// done endpoint and manufacture an elapsed interval. Event.Line remains the
// source-local physical order after its bundle virtual-line rebase; group by the
// resolved physical artifact first, then sort by that coordinate.
type genericStorageReplayPlan struct {
	eventIndexes              []int
	unresolvedLifecycleResets int
}

func buildGenericStorageReplayPlan(idx *Index, _ Query) genericStorageReplayPlan {
	if idx == nil {
		return genericStorageReplayPlan{}
	}
	relevantPIDs := map[int]bool{}
	for _, ev := range idx.Events {
		if _, _, endpoint := genericStorageEndpoint(ev); endpoint && ev.PID > 0 {
			relevantPIDs[ev.PID] = true
		}
	}

	bySource := map[string][]int{}
	plan := genericStorageReplayPlan{}
	for eventIndex, ev := range idx.Events {
		_, _, endpoint := genericStorageEndpoint(ev)
		resetPID, reset := schedulerLifecycleResetPID(ev)
		if !endpoint && (!reset || !relevantPIDs[resetPID]) {
			continue
		}
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			// Endpoint provenance is already handled by the endpoint integrity
			// pre-audit. A reset has no semantic lane key of its own: when its
			// source cannot be proven, continuing would let a pre-reset start
			// close in an arbitrary artifact, so the caller must fail the whole
			// generic-storage family closed.
			if reset && relevantPIDs[resetPID] {
				plan.unresolvedLifecycleResets++
			}
			continue
		}
		bySource[source] = append(bySource[source], eventIndex)
	}

	sources := make([]string, 0, len(bySource))
	for source := range bySource {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, source := range sources {
		indexes := bySource[source]
		sort.SliceStable(indexes, func(i, j int) bool {
			left, right := idx.Events[indexes[i]], idx.Events[indexes[j]]
			if left.Line != right.Line {
				return left.Line < right.Line
			}
			return left.Ts < right.Ts
		})
		plan.eventIndexes = append(plan.eventIndexes, indexes...)
	}
	return plan
}

func genericStorageEndpoint(ev Event) (genericStorageIdentity, string, bool) {
	// Exact block rq/bio endpoints have a stronger typed identity and must
	// never enter this suffix-based family.
	if ev.Type != EventStorage && ev.Type != EventFilesystem {
		return genericStorageIdentity{}, "", false
	}
	if !isStorageLatencyEvent(ev) {
		return genericStorageIdentity{}, "", false
	}
	layer := storageLatencyLayer(ev)
	base, phase := storageLatencyBaseAndPhase(ev)
	if layer == "" || base == "" || (phase != "start" && phase != "done") {
		return genericStorageIdentity{}, "", false
	}
	ff := ev.FileFields
	if ff == nil {
		ff = &FileFields{}
	}
	rf := ev.ResourceFields
	if rf == nil {
		rf = &ResourceFields{}
	}
	blk := ev.BlockIOFields
	if blk == nil {
		blk = &BlockIOFields{}
	}
	dev := firstNonEmpty(ff.Dev, blk.Dev, blk.SrcDev, "unknown")
	op := firstNonEmpty(ff.RW, rf.Op, blk.Op, fileOperationFromEventName(ev.Name), base)
	return genericStorageIdentity{
		Layer: layer,
		Base:  base,
		Dev:   dev,
		Inode: firstNonEmpty(ff.Ino, "-"),
		Op:    op,
		PID:   ev.PID,
	}, phase, true
}

func genericStoragePairingSemanticKey(id genericStorageIdentity) string {
	return encodePairingKey(
		id.Layer,
		id.Base,
		canonicalGenericStorageDevice(id.Dev),
		canonicalGenericStorageInode(id.Inode),
		id.Op,
		strconv.Itoa(id.PID),
	)
}

func canonicalGenericStorageDevice(raw string) string {
	canonical, ok := canonicalGenericStorageDeviceValidated(raw)
	if ok {
		return canonical
	}
	return strings.TrimSpace(raw)
}

func canonicalGenericStorageDeviceValidated(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if canonical, ok := canonicalBlockDevice(raw); ok {
		return canonical, true
	}
	if isAllDigits(raw) {
		value, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return "", false
		}
		return fmt.Sprintf("%d,%d", value>>20, value&0xfffff), true
	}
	return raw, true
}

func canonicalGenericStorageInode(raw string) string {
	canonical, ok := canonicalGenericStorageInodeValidated(raw)
	if ok {
		return canonical
	}
	return strings.TrimSpace(raw)
}

func canonicalGenericStorageInodeValidated(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return raw, true
	}
	base := 10
	if strings.HasPrefix(strings.ToLower(raw), "0x") {
		base = 16
		raw = raw[2:]
		if raw == "" {
			return "", false
		}
	} else if !isAllDigits(raw) {
		return "", false
	}
	value, err := strconv.ParseUint(raw, base, 64)
	if err != nil {
		return "", false
	}
	return strconv.FormatUint(value, 10), true
}

type genericStoragePairingDecoded struct {
	key         string
	source      string
	identity    genericStorageIdentity
	phase       string
	verdict     PairingEndpointVerdict
	endpoint    bool
	sourceKnown bool
	keyAdmitted bool
}

func decodeGenericStoragePairingEvent(idx *Index, ev Event) genericStoragePairingDecoded {
	identity, phase, endpoint := genericStorageEndpoint(ev)
	if !endpoint {
		return genericStoragePairingDecoded{}
	}
	out := genericStoragePairingDecoded{identity: identity, phase: phase, endpoint: true}
	out.source, out.sourceKnown = tracePairingSourceIdentity(idx, ev)
	admission := genericStorageWireAdmission{}
	if verdict, mmcAdmission, governed := mmcPairingVerdictFromEvent(ev); governed {
		out.verdict, admission = verdict, mmcAdmission
	} else if strings.TrimSpace(ev.FieldText) != "" {
		var decoded pairingEndpointDecodedFields
		out.verdict, decoded = decodePairingEndpointWire(ev.Name, ev.FieldText, int64(ev.PID))
		admission = decoded.storage
	} else {
		out.verdict = fingerprintPairingEvent(ev)
	}
	if out.sourceKnown && out.verdict.Family == PairingEndpointStorage && PairingEndpointPhase(phase) == out.verdict.Phase && out.verdict.PayloadAdmitted && out.verdict.EmitterAdmitted {
		out.key, out.keyAdmitted = out.verdict.LaneKey(out.source)
		if out.keyAdmitted {
			if strings.TrimSpace(ev.FieldText) != "" {
				out.identity.Dev = firstNonEmpty(admission.dev, "unknown")
				out.identity.Inode = firstNonEmpty(admission.inode, "-")
				out.identity.Op = firstNonEmpty(normalizeFileRW(admission.op), fileOperationFromEventName(ev.Name), out.identity.Base)
			}
			return out
		}
	}
	return out
}

func genericStoragePairingKey(idx *Index, ev Event) (key, source string, identity genericStorageIdentity, phase string, ok bool) {
	decoded := decodeGenericStoragePairingEvent(idx, ev)
	if !decoded.keyAdmitted {
		return "", "", genericStorageIdentity{}, "", false
	}
	return decoded.key, decoded.source, decoded.identity, decoded.phase, true
}

func genericStorageEndpointBytes(ev Event) int64 {
	if ff := ev.FileFields; ff != nil && ff.Len > 0 {
		return ff.Len
	}
	if rf := ev.ResourceFields; rf != nil && rf.Bytes > 0 {
		return rf.Bytes
	}
	return 0
}

func genericStorageIdentityLabel(id genericStorageIdentity) string {
	return fmt.Sprintf("layer=%s event=%s dev=%s inode=%s op=%s pid=%d", id.Layer, id.Base, id.Dev, id.Inode, id.Op, id.PID)
}

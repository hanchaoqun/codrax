package tracequery

import (
	"fmt"
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

func (id genericStorageIdentity) laneKey() string {
	return strings.Join([]string{
		id.Layer,
		id.Base,
		id.Dev,
		id.Inode,
		id.Op,
		strconv.Itoa(id.PID),
	}, "\x00")
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

func genericStoragePairingKey(idx *Index, ev Event) (key, source string, identity genericStorageIdentity, phase string, ok bool) {
	identity, phase, ok = genericStorageEndpoint(ev)
	if !ok {
		return "", "", genericStorageIdentity{}, "", false
	}
	source, ok = tracePairingSourceIdentity(idx, ev)
	if !ok {
		return "", "", genericStorageIdentity{}, "", false
	}
	return source + "\x00" + identity.laneKey(), source, identity, phase, true
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

package tracequery

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/pcapinput"
)

// PacketQueryResult is the shared native packet execution envelope. Payload
// is data, not permission: the private receipt binds its original bytes to
// the source generation actually scanned and cannot survive JSON restoration.
type PacketQueryResult struct {
	Payload Result
	receipt interface {
		ValidateCurrentSource() error
		SourceVersionToken() string
	}
	payloadHash [32]byte
}

func (r PacketQueryResult) ValidateCurrentSource() error {
	if r.receipt == nil {
		return fmt.Errorf("packet query receipt is uninitialized")
	}
	b, err := json.Marshal(r.Payload)
	if err != nil {
		return err
	}
	if sha256.Sum256(b) != r.payloadHash {
		return fmt.Errorf("packet query payload changed after native execution")
	}
	return r.receipt.ValidateCurrentSource()
}

func (r PacketQueryResult) SourceVersionToken() string {
	if r.receipt == nil {
		return ""
	}
	return r.receipt.SourceVersionToken()
}

// StreamPacketQuery executes one exact packet-domain view. Limit only bounds
// preview members; full aggregation and explicit partial status stay native.
// The ordinary indexed Run and Trace recovery adapters remain out of domain.
func StreamPacketQuery(ctx context.Context, path, view string, limit int) (PacketQueryResult, error) {
	var out PacketQueryResult
	if view != ViewPacketInventory && view != ViewPacketFlows {
		return out, fmt.Errorf("packet execution requires packet_inventory or packet_flows, got %q", view)
	}
	if limit < 0 || limit > pcapinput.MaxLimit {
		return out, fmt.Errorf("packet member limit must be between 0 and %d", pcapinput.MaxLimit)
	}
	out.Payload = Result{View: view, SourcePath: path}
	switch view {
	case ViewPacketInventory:
		r, err := pcapinput.Stream(ctx, path, limit)
		if err != nil {
			return PacketQueryResult{}, err
		}
		out.Payload.PacketInventory, out.receipt = &r.PacketInventoryResult, r
	case ViewPacketFlows:
		r, err := pcapinput.StreamFlows(ctx, path, limit)
		if err != nil {
			return PacketQueryResult{}, err
		}
		out.Payload.PacketFlows, out.receipt = &r.PacketFlowsResult, r
	}
	b, err := json.Marshal(out.Payload)
	if err != nil {
		return PacketQueryResult{}, err
	}
	out.payloadHash = sha256.Sum256(b)
	if err := out.ValidateCurrentSource(); err != nil {
		return PacketQueryResult{}, err
	}
	return out, nil
}

// PacketSourceVersion protects a multi-step packet report without selecting
// Trace sidecars or granting a Trace clock. Capture validates only the native
// classic-PCAP header; it is not a completed packet inventory.
type PacketSourceVersion struct {
	path     string
	identity filegeneration.Identity
}

func CapturePacketSourceVersion(ctx context.Context, path string) (out PacketSourceVersion, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	f, id, err := filegeneration.OpenRegularReadOnly(path)
	if err != nil {
		return out, err
	}
	defer func() {
		current, checkErr := filegeneration.FromFile(f)
		if checkErr == nil && !id.SameVersion(current) {
			checkErr = fmt.Errorf("packet source changed during header validation")
		}
		err = errors.Join(err, checkErr, ctx.Err(), f.Close())
	}()
	var header [24]byte
	n, _ := io.ReadFull(f, header[:])
	if _, err := pcapinput.ParseHeader(header[:n]); err != nil {
		return out, err
	}
	out = PacketSourceVersion{path: path, identity: id}
	if err := out.Validate(ctx); err != nil {
		return PacketSourceVersion{}, err
	}
	return out, nil
}

func (v PacketSourceVersion) Validate(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !v.identity.Initialized() || v.path == "" {
		return fmt.Errorf("packet source version is uninitialized")
	}
	current, err := filegeneration.FromPath(v.path)
	if err != nil {
		return err
	}
	if !v.identity.SameVersion(current) {
		return fmt.Errorf("packet source changed during multi-step collection; discard mixed-version report")
	}
	return nil
}

func (v PacketSourceVersion) Fingerprint() string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(v.identity.CacheToken())))
}
func (v PacketSourceVersion) SourceBytes() int64 { return v.identity.Size() }

// ContentDigest reads the same regular-file generation in fixed chunks. It
// cannot block by opening a substituted FIFO or turn a failed read into a
// successful provenance placeholder. The digest is not query completeness.
func (v PacketSourceVersion) ContentDigest(ctx context.Context) (digest string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := v.Validate(ctx); err != nil {
		return "", err
	}
	f, opened, err := filegeneration.OpenRegularReadOnly(v.path)
	if err != nil {
		return "", err
	}
	defer func() {
		current, checkErr := filegeneration.FromFile(f)
		if checkErr == nil && !v.identity.SameVersion(current) {
			checkErr = fmt.Errorf("packet source changed during content digest")
		}
		err = errors.Join(err, checkErr, v.Validate(ctx), f.Close())
	}()
	if !v.identity.SameVersion(opened) {
		return "", fmt.Errorf("packet source replaced before content digest")
	}
	h := sha256.New()
	var chunk [64 << 10]byte
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := f.Read(chunk[:])
		if n > 0 {
			_, _ = h.Write(chunk[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

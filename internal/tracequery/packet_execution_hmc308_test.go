package tracequery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/pcapinput"
)

func hmc308Capture(t *testing.T, tail []byte) string {
	t.Helper()
	b := make([]byte, 24)
	binary.LittleEndian.PutUint32(b, 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(b[4:], 2)
	binary.LittleEndian.PutUint16(b[6:], 4)
	binary.LittleEndian.PutUint32(b[16:], 65535)
	binary.LittleEndian.PutUint32(b[20:], 101)
	p := filepath.Join(t.TempDir(), "capture.bin")
	if err := os.WriteFile(p, append(b, tail...), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHMC308PacketEnvelopePreservesNativePayload(t *testing.T) {
	for _, view := range []string{ViewPacketInventory, ViewPacketFlows} {
		for _, partial := range []bool{false, true} {
			t.Run(view+map[bool]string{true: "/partial", false: "/empty"}[partial], func(t *testing.T) {
				var tail []byte
				if partial {
					tail = []byte{1}
				}
				path := hmc308Capture(t, tail)
				got, err := StreamPacketQuery(context.Background(), path, view, 20)
				if err != nil {
					t.Fatal(err)
				}
				want := Result{View: view, SourcePath: path}
				if view == ViewPacketInventory {
					r, err := pcapinput.Stream(context.Background(), path, 20)
					if err != nil {
						t.Fatal(err)
					}
					want.PacketInventory = &r.PacketInventoryResult
				} else {
					r, err := pcapinput.StreamFlows(context.Background(), path, 20)
					if err != nil {
						t.Fatal(err)
					}
					want.PacketFlows = &r.PacketFlowsResult
				}
				a, _ := json.Marshal(got.Payload)
				b, _ := json.Marshal(want)
				if !bytes.Equal(a, b) || got.SourceVersionToken() == "" || got.ValidateCurrentSource() != nil {
					t.Fatalf("native envelope drift: %s / %s", a, b)
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(a, &fields); err != nil {
					t.Fatal(err)
				}
				if len(fields) != 3 || fields[view] == nil {
					t.Fatalf("unexpected Trace/extra envelope fields: %s", a)
				}
				var revived PacketQueryResult
				raw, _ := json.Marshal(got)
				if err := json.Unmarshal(raw, &revived); err != nil {
					t.Fatal(err)
				}
				if revived.ValidateCurrentSource() == nil || revived.SourceVersionToken() != "" {
					t.Fatal("JSON revived native authority")
				}
			})
		}
	}
}

func TestHMC308PacketReceiptRejectsPayloadAndGenerationChanges(t *testing.T) {
	for _, change := range []string{"view", "source", "value", "replace"} {
		t.Run(change, func(t *testing.T) {
			path := hmc308Capture(t, nil)
			r, err := StreamPacketQuery(context.Background(), path, ViewPacketInventory, 1)
			if err != nil {
				t.Fatal(err)
			}
			v, err := CapturePacketSourceVersion(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "view":
				r.Payload.View = ViewPacketFlows
			case "source":
				r.Payload.SourcePath = "other"
			case "value":
				r.Payload.PacketInventory.PacketCount = 1
			case "replace":
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				replacement := path + ".replacement"
				if err := os.WriteFile(replacement, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			}
			if r.ValidateCurrentSource() == nil {
				t.Fatal("changed evidence retained authority")
			}
			if change == "replace" && v.Validate(context.Background()) == nil {
				t.Fatal("multi-step packet source version accepted replacement")
			}
		})
	}
}

func TestHMC308PacketExecutionRejectsBeforeReadAndHonorsCancel(t *testing.T) {
	for _, tc := range []struct {
		view  string
		limit int
	}{{"event_search", 1}, {"unknown", 1}, {ViewPacketFlows, 65}, {ViewPacketInventory, -1}} {
		_, err := StreamPacketQuery(context.Background(), "/does-not-exist", tc.view, tc.limit)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid request reached source: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := StreamPacketQuery(ctx, hmc308Capture(t, nil), ViewPacketInventory, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	path := hmc308Capture(t, nil)
	if err := os.WriteFile(path, []byte("worker-1 [000] 1.0: sched_wakeup: pid=2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CapturePacketSourceVersion(context.Background(), path); err == nil || !strings.Contains(err.Error(), "pcap_magic") {
		t.Fatalf("Trace input acquired packet source authority: %v", err)
	}
}

type hmc308ChunkCancelContext struct {
	context.Context
	checks int
}

func (c *hmc308ChunkCancelContext) Err() error {
	c.checks++
	if c.checks >= 4 {
		return context.Canceled
	}
	return nil
}

func TestHMC308PacketDigestSameGenerationAndChunkCancellation(t *testing.T) {
	path := hmc308Capture(t, make([]byte, 256<<10))
	v, err := CapturePacketSourceVersion(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.ContentDigest(context.Background())
	if want := fmt.Sprintf("%x", sha256.Sum256(b)); err != nil || got != want {
		t.Fatalf("exact digest=%s err=%v want=%s", got, err, want)
	}
	ctx := &hmc308ChunkCancelContext{Context: context.Background()}
	if _, err := v.ContentDigest(ctx); !errors.Is(err, context.Canceled) || ctx.checks < 4 {
		t.Fatalf("digest failed to poll between bounded chunks: %v checks=%d", err, ctx.checks)
	}
}

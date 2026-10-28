package orchestrator

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc304PacketSource(t *testing.T, dir, name string) string {
	t.Helper()
	var b bytes.Buffer
	for _, v := range []any{uint32(0xa1b2c3d4), uint16(2), uint16(4), int32(0), uint32(0), uint32(65535), uint32(147), uint32(5), uint32(2), uint32(3), uint32(5)} {
		if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	b.Write([]byte{1, 2, 3})
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, b.Bytes(), 0400); err != nil {
		t.Fatal(err)
	}
	return path
}

// Exercise Run's real entry, rather than constructing the tool bus by hand.
// The stub's intentional stop is after admission and does not claim an answer.
func TestHMC304PreparedPacketCrossesRunAdmission(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	path := hmc304PacketSource(t, repo, "normal.capture")
	m, err := traceinput.Prepare(t.Context(), traceinput.Options{InputPath: path, RuntimeAnchor: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	calls, events := 0, 0
	o := newTraceAdmissionTestOrchestrator(&calls, &events)
	o.SetAttachedHitrace(m.Preview())
	o.SetAttachedTraceMaterial(m)
	_, err = o.Run("请统计附件中的网络包", repo, "main")
	if calls == 0 || events == 0 {
		t.Fatalf("prepared packet rejected before any agent: calls=%d events=%d err=%v", calls, events, err)
	}
}

func TestHMC304NamedPacketUsesPreparedQueryDomain(t *testing.T) {
	for _, name := range []string{"normal.capture", "misleading.tracebundle.json"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := hmc304PacketSource(t, dir, name)
			c := traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()})
			bus := &types.BusContext{RepoRoot: dir, TraceInputPreparer: c}
			if err := prepareAndAdmitNamedTracePath(t.Context(), bus, path); err != nil {
				t.Fatalf("prepared packet rejected by named admission: %v", err)
			}
			if err := prepareAndAdmitNamedTracePath(t.Context(), bus, path); err != nil {
				t.Fatalf("same-generation reuse rejected: %v", err)
			}
			if len(c.PreparedMaterials()) != 1 || bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" {
				t.Fatal("named preparation duplicated or became a sticky attachment")
			}
		})
	}
}

func TestHMC304PacketAdmissionStillRejectsUnpreparedAndStale(t *testing.T) {
	dir := t.TempDir()
	path := hmc304PacketSource(t, dir, "ordinary.capture")
	bus := &types.BusContext{RepoRoot: dir}
	if err := prepareAndAdmitNamedTracePath(t.Context(), bus, path); err == nil {
		t.Fatal("unprepared binary gained text-query permission")
	}
	c := traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()})
	bus.TraceInputPreparer = c
	if _, err := c.Prepare(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	replacement := hmc304PacketSource(t, dir, "replacement.capture")
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if err := prepareAndAdmitNamedTracePath(t.Context(), bus, path); err == nil {
		t.Fatal("stale packet receipt was silently replaced")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := prepareAndAdmitNamedTracePath(ctx, bus, path); err == nil {
		t.Fatal("canceled packet admission succeeded")
	}
}

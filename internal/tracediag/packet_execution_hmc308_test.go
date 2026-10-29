package tracediag

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc308PacketFile(t *testing.T, records int, partial bool) string {
	t.Helper()
	b := make([]byte, 24)
	binary.LittleEndian.PutUint32(b, 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(b[4:], 2)
	binary.LittleEndian.PutUint16(b[6:], 4)
	binary.LittleEndian.PutUint32(b[16:], 65535)
	binary.LittleEndian.PutUint32(b[20:], 101)
	for i := 0; i < records; i++ {
		h := make([]byte, 16)
		binary.LittleEndian.PutUint32(h, uint32(1700000000+i))
		binary.LittleEndian.PutUint32(h[8:], 28)
		binary.LittleEndian.PutUint32(h[12:], 28)
		p := make([]byte, 28)
		p[0], p[9] = 0x45, 17
		binary.BigEndian.PutUint16(p[2:], 28)
		copy(p[12:], []byte{192, 0, 2, 1, 192, 0, 2, 2})
		binary.BigEndian.PutUint16(p[20:], 1234)
		binary.BigEndian.PutUint16(p[22:], 4321)
		binary.BigEndian.PutUint16(p[24:], 8)
		b = append(append(b, h...), p...)
	}
	if partial {
		b = append(b, 1)
	}
	p := filepath.Join(t.TempDir(), "ordinary-capture.data")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

type hmc308TrackingPreparer struct {
	delegate types.TraceInputPreparer
	calls    int
	after    func()
}

type hmc308ReceiptPreparer struct{ material *attachment.TraceMaterial }

func (p hmc308ReceiptPreparer) Prepare(context.Context, string) (*attachment.TraceMaterial, error) {
	return p.material, nil
}
func (p hmc308ReceiptPreparer) PreparedMaterials() []*attachment.TraceMaterial {
	return []*attachment.TraceMaterial{p.material}
}

func (p *hmc308TrackingPreparer) Prepare(ctx context.Context, path string) (*attachment.TraceMaterial, error) {
	p.calls++
	m, err := p.delegate.Prepare(ctx, path)
	if p.after != nil {
		p.after()
	}
	return m, err
}
func (p *hmc308TrackingPreparer) PreparedMaterials() []*attachment.TraceMaterial {
	return p.delegate.PreparedMaterials()
}

func TestHMC308PacketCompleteZeroPartialAndPreviewAreDistinct(t *testing.T) {
	for _, version := range []int{1, 2} {
		for _, tc := range []struct {
			name     string
			records  int
			partial  bool
			failures int
		}{{"zero", 0, false, 0}, {"preview", 70, false, 0}, {"partial", 2, true, 2}} {
			t.Run(fmt.Sprintf("v%d/%s", version, tc.name), func(t *testing.T) {
				path := hmc308PacketFile(t, tc.records, tc.partial)
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				p := &hmc308TrackingPreparer{delegate: traceinput.NewCoordinator(traceinput.Options{})}
				var out bytes.Buffer
				failed, err := Run(context.Background(), Options{ScriptPath: hmc308PacketScript(t, version, ""), TracePath: path, TraceInputPreparer: p}, &out)
				if err != nil || failed != tc.failures || p.calls != 1 {
					t.Fatalf("failed=%d err=%v calls=%d\n%s", failed, err, p.calls, out.String())
				}
				want := fmt.Sprintf("packet_count=%d", tc.records)
				if strings.Count(out.String(), want) != 2 {
					t.Fatalf("each view must retain its own full count %q:\n%s", want, out.String())
				}
				if tc.name == "preview" && (!strings.Contains(out.String(), "omitted_packets=6") || strings.Contains(out.String(), "packet_inventory.members[64]")) {
					t.Fatal("preview replaced full count/budget")
				}
				if tc.partial && (!strings.Contains(out.String(), "status=partial") || !strings.Contains(out.String(), "code=pcap_short_record_header") || strings.Contains(out.String(), "状态=成功")) {
					t.Fatal("partial became completed or lost evidence")
				}
				if tc.name == "zero" && (strings.Contains(out.String(), "epoch_seconds=") || strings.Contains(out.String(), "状态=失败")) {
					t.Fatal("empty capture invented time or failure")
				}
				for _, forbidden := range []string{"query=[", "event_count=", "scanned_lines=", filepath.Dir(path)} {
					if strings.Contains(out.String(), forbidden) {
						t.Errorf("invented Trace fact or exposed local path %q", forbidden)
					}
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("read-only source changed")
				}
			})
		}
	}
}

func TestHMC308PacketScriptDomainRejectsBeforeSourceOrPreparation(t *testing.T) {
	for _, body := range []string{
		"defaults: {pid: 41}", "defaults: {thread: worker}", "defaults: {window: '1..2'}",
		"steps:\n - {label: p, view: packet_inventory, line_start: 1}",
		"steps:\n - {label: p, view: packet_inventory, pattern: x}",
		"steps:\n - {label: p, view: packet_flows, event_names: [x]}",
		"steps:\n - {label: p, view: packet_flows}\n - {label: t, view: event_search}",
		"steps:\n - {label: unknown, view: not_a_view}",
		"discoveries:\n - {label: discovery, strategy: pairing_integrity}",
	} {
		t.Run(body, func(t *testing.T) {
			text := "version: 2\n" + body + "\n"
			if !strings.HasPrefix(body, "steps:") {
				text += "steps:\n - {label: packets, view: packet_inventory}\n"
			}
			path := filepath.Join(t.TempDir(), "script.yaml")
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			p := &hmc308TrackingPreparer{delegate: traceinput.NewCoordinator(traceinput.Options{})}
			var out bytes.Buffer
			_, err := Run(context.Background(), Options{ScriptPath: path, TracePath: path + ".absent", TraceInputPreparer: p}, &out)
			if err == nil || p.calls != 0 || out.Len() != 0 || strings.Contains(err.Error(), "trace file:") {
				t.Fatalf("invalid domain reached source/preparer: err=%v calls=%d report=%s", err, p.calls, out.String())
			}
		})
	}
}

func TestHMC308PacketGenerationChangePublishesNothing(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := hmc308PacketFile(t, 1, false)
			p := &hmc308TrackingPreparer{delegate: traceinput.NewCoordinator(traceinput.Options{})}
			p.after = func() {
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path+".next", b, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(path+".next", path); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			_, err := Run(context.Background(), Options{ScriptPath: hmc308PacketScript(t, version, ""), TracePath: path, TraceInputPreparer: p}, &out)
			if err == nil || out.Len() != 0 || p.calls != 1 {
				t.Fatalf("changed source published: %v %s", err, out.String())
			}
		})
	}
}

func TestHMC308PacketBetweenStepsChangeDiscardsBufferedReport(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := hmc308PacketFile(t, 2, false)
			path, err := filepath.EvalSymlinks(path)
			if err != nil {
				t.Fatal(err)
			}
			id, err := filegeneration.FromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			checks := 0
			m, err := attachment.BindTraceMaterialWithSourceCheck(path, path, "# codrax-source: "+path+"\npacket preview", map[string]filegeneration.Identity{path: id}, func(context.Context) error {
				checks++
				// Bind + prepare check + digest check + first-step pre/post
				// checks have succeeded; mutate before the second native scan.
				if checks == 6 {
					b, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					if err := os.WriteFile(path+".next", b, 0600); err != nil {
						return err
					}
					return os.Rename(path+".next", path)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			_, err = Run(context.Background(), Options{ScriptPath: hmc308PacketScript(t, version, ""), TracePath: path, TraceInputPreparer: hmc308ReceiptPreparer{m}}, &out)
			if checks != 6 || err == nil || out.Len() != 0 {
				t.Fatalf("mixed-generation partial report escaped: checks=%d err=%v report=%s", checks, err, out.String())
			}
		})
	}
}

func TestHMC308PacketUnsupportedSourceNeverInvokesConverter(t *testing.T) {
	for _, content := range []string{"worker-1 [000] 1.0: sched_wakeup: pid=2\n", "SQLite format 3\x00", "\x0a\x0d\x0d\x0a"} {
		path := filepath.Join(t.TempDir(), "capture.pcap")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		p := &hmc308TrackingPreparer{delegate: traceinput.NewCoordinator(traceinput.Options{})}
		var out bytes.Buffer
		_, err := Run(context.Background(), Options{ScriptPath: hmc308PacketScript(t, 1, ""), TracePath: path, TraceInputPreparer: p}, &out)
		if err == nil || p.calls != 0 || out.Len() != 0 {
			t.Fatalf("unsupported format reached prepare: %v calls=%d", err, p.calls)
		}
	}
}

func TestHMC308PacketShippedTemplateAndDisplayBudgets(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "tracediag", "collect_packet_capture.yaml")
	s, err := LoadScript(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Version != 1 || len(s.Steps) != 2 || s.Steps[0].View != "packet_inventory" || s.Steps[1].View != "packet_flows" {
		t.Fatal("shipped complete native face changed")
	}
	var out bytes.Buffer
	failed, err := Run(context.Background(), Options{ScriptPath: hmc308PacketScript(t, 1, "defaults: {max_lines: 8}"), TracePath: hmc308PacketFile(t, 70, false), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{}), TotalMaxLines: 28}, &out)
	if err != nil || failed != 0 || !strings.Contains(out.String(), "总行帽") || !strings.Contains(out.String(), "步骤状态摘要") || !strings.Contains(out.String(), "packet_count=70") {
		t.Fatalf("bounded display lost facts/status: %v failed=%d\n%s", err, failed, out.String())
	}
}

func TestHMC308PacketCancellationAndV2CapPublishNothing(t *testing.T) {
	for _, mode := range []string{"cancel_before", "cancel_after_prepare", "v2_cap", "digest_cancel", "digest_failure"} {
		t.Run(mode, func(t *testing.T) {
			path := hmc308PacketFile(t, 70, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := &hmc308TrackingPreparer{delegate: traceinput.NewCoordinator(traceinput.Options{})}
			opts := Options{ScriptPath: hmc308PacketScript(t, 2, ""), TracePath: path, TraceInputPreparer: p}
			switch mode {
			case "cancel_before":
				cancel()
			case "cancel_after_prepare":
				p.after = cancel
			case "v2_cap":
				opts.TotalMaxLines = 1
			case "digest_cancel":
				version, err := tracequery.CapturePacketSourceVersion(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				cancel()
				if _, err := version.ContentDigest(ctx); !errors.Is(err, context.Canceled) {
					t.Fatalf("digest ignored cancel: %v", err)
				}
				return
			case "digest_failure":
				version, err := tracequery.CapturePacketSourceVersion(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(path, path+".moved"); err != nil {
					t.Fatal(err)
				}
				if _, err := version.ContentDigest(ctx); err == nil {
					t.Fatal("digest failure replaced with placeholder")
				}
				return
			}
			var out bytes.Buffer
			_, err := Run(ctx, opts, &out)
			if err == nil || out.Len() != 0 {
				t.Fatalf("incomplete report published: %v %s", err, out.String())
			}
			if strings.HasPrefix(mode, "cancel_") && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel identity lost: %v", err)
			}
		})
	}
}

func hmc308PacketScript(t *testing.T, version int, extra string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "packet.yaml")
	body := fmt.Sprintf("version: %d\n%s\nsteps:\n  - {label: inventory, view: packet_inventory}\n  - {label: flows, view: packet_flows}\n", version, extra)
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Public RED: enumeration/rendering is not an execution adapter. Both script
// versions must actually scan a normal binary source through their public Run.
func TestHMC308PacketScriptsExecuteNativeBothVersions(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			var out bytes.Buffer
			failed, err := Run(context.Background(), Options{ScriptPath: hmc308PacketScript(t, version, ""), TracePath: hmc308PacketFile(t, 2, false), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{})}, &out)
			if err != nil || failed != 0 {
				t.Fatalf("native packet execution failed=%d err=%v report=%s", failed, err, out.String())
			}
			for _, want := range []string{"packet_inventory", "packet_flows", "packet_count=2", "pcap_epoch_seconds"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing native fact %q: %s", want, out.String())
				}
			}
		})
	}
}

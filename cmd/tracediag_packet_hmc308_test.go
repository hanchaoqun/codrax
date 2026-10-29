package cmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHMC308TraceDiagCLIPacketTemplateEmptyAndPartial(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "partial"}[partial], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "capture.data")
			b := make([]byte, 24)
			binary.LittleEndian.PutUint32(b, 0xa1b2c3d4)
			binary.LittleEndian.PutUint16(b[4:], 2)
			binary.LittleEndian.PutUint16(b[6:], 4)
			binary.LittleEndian.PutUint32(b[16:], 65535)
			binary.LittleEndian.PutUint32(b[20:], 101)
			if partial {
				b = append(b, 1)
			}
			if err := os.WriteFile(path, b, 0600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "report.txt")
			withTraceDiagFlags(t, filepath.Join("..", "examples", "tracediag", "collect_packet_capture.yaml"), path, out, "auto")
			var stdout bytes.Buffer
			err := runTraceDiagCLI(context.Background(), &stdout, nil)
			if (err != nil) != partial {
				t.Fatalf("partial=%v err=%v", partial, err)
			}
			report, readErr := os.ReadFile(out)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if strings.Count(string(report), "packet_count=0") != 2 || !strings.Contains(string(report), "步骤状态摘要") {
				t.Fatalf("native report missing: %s", report)
			}
			if partial && (!strings.Contains(string(report), "status=partial") || !strings.Contains(string(report), "2/2 步骤失败")) {
				t.Fatalf("partial became empty success: %s", report)
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(b, after) {
				t.Fatal("CLI changed original")
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil || len(entries) != 2 {
				t.Fatalf("unexpected persistent side effects: %v %v", entries, readErr)
			}
		})
	}
}

func TestHMC308TraceDiagCLIBadPacketPreservesPriorReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.pcap")
	out := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(path, []byte("not a packet capture"), 0600); err != nil {
		t.Fatal(err)
	}
	prior := []byte("prior immutable report\n")
	if err := os.WriteFile(out, prior, 0600); err != nil {
		t.Fatal(err)
	}
	withTraceDiagFlags(t, filepath.Join("..", "examples", "tracediag", "collect_packet_capture.yaml"), path, out, "auto")
	var stdout bytes.Buffer
	if err := runTraceDiagCLI(context.Background(), &stdout, nil); err == nil {
		t.Fatal("invalid packet source passed")
	}
	after, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(prior, after) {
		t.Fatal("failed run replaced old report")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("bad source left temporary/conversion products: %v %v", entries, err)
	}
}

package tracebundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureIDIsCanonicalAndOrderIndependent(t *testing.T) {
	one := CaptureMember{Type: "systrace", Path: "capture.systrace", Bytes: 0, SHA256: strings.Repeat("0", 64)}
	two := CaptureMember{Type: "perftrace", Path: "capture.perftrace", Bytes: 7, SHA256: strings.Repeat("a", 64)}

	forward, err := CaptureID([]CaptureMember{one, two})
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := CaptureID([]CaptureMember{two, one})
	if err != nil {
		t.Fatal(err)
	}
	if forward != reverse || !strings.HasPrefix(forward, "sha256:") || len(forward) != len("sha256:")+64 {
		t.Fatalf("capture identity is not canonical: forward=%q reverse=%q", forward, reverse)
	}
	const golden = "sha256:da2bc483e080f0d2817c75e1e15a3529722f89336ee9db247ced1338a5fa4503"
	if forward != golden {
		t.Fatalf("capture identity wire algorithm drifted: got=%q want=%q", forward, golden)
	}
	if err := ValidateCaptureID(forward); err != nil {
		t.Fatalf("generated capture identity rejected: %v", err)
	}

	changed := two
	changed.Bytes++
	different, err := CaptureID([]CaptureMember{one, changed})
	if err != nil {
		t.Fatal(err)
	}
	if different == forward {
		t.Fatal("byte-size change did not change capture identity")
	}
	empty, err := CaptureID(nil)
	if err != nil || ValidateCaptureID(empty) != nil || empty == forward {
		t.Fatalf("empty causal set must retain one schema-v2 identity: id=%q err=%v", empty, err)
	}
	const emptyGolden = "sha256:b5437d7630217085b1074804e342062a4667b6b823bac196c0933ff5f4788cd3"
	if empty != emptyGolden {
		t.Fatalf("empty capture identity wire algorithm drifted: got=%q want=%q", empty, emptyGolden)
	}
}

func TestCaptureIDRejectsMalformedOrDuplicateMembers(t *testing.T) {
	valid := CaptureMember{Type: "systrace", Path: "capture.systrace", Bytes: 1, SHA256: strings.Repeat("1", 64)}
	tests := []struct {
		name    string
		members []CaptureMember
	}{
		{name: "type", members: []CaptureMember{{Type: "trace_db", Path: valid.Path, Bytes: valid.Bytes, SHA256: valid.SHA256}}},
		{name: "absolute", members: []CaptureMember{{Type: valid.Type, Path: "/tmp/capture.systrace", Bytes: valid.Bytes, SHA256: valid.SHA256}}},
		{name: "escape", members: []CaptureMember{{Type: valid.Type, Path: "../capture.systrace", Bytes: valid.Bytes, SHA256: valid.SHA256}}},
		{name: "noncanonical", members: []CaptureMember{{Type: valid.Type, Path: "a/../capture.systrace", Bytes: valid.Bytes, SHA256: valid.SHA256}}},
		{name: "backslash", members: []CaptureMember{{Type: valid.Type, Path: `a\capture.systrace`, Bytes: valid.Bytes, SHA256: valid.SHA256}}},
		{name: "volume", members: []CaptureMember{{Type: valid.Type, Path: `C:/capture.systrace`, Bytes: valid.Bytes, SHA256: valid.SHA256}}},
		{name: "negative", members: []CaptureMember{{Type: valid.Type, Path: valid.Path, Bytes: -1, SHA256: valid.SHA256}}},
		{name: "uppercase", members: []CaptureMember{{Type: valid.Type, Path: valid.Path, Bytes: valid.Bytes, SHA256: strings.Repeat("A", 64)}}},
		{name: "nonhex", members: []CaptureMember{{Type: valid.Type, Path: valid.Path, Bytes: valid.Bytes, SHA256: strings.Repeat("g", 64)}}},
		{name: "duplicate", members: []CaptureMember{valid, valid}},
		{name: "duplicate_cross_type", members: []CaptureMember{valid, {Type: "perftrace", Path: valid.Path, Bytes: valid.Bytes, SHA256: valid.SHA256}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, err := CaptureID(test.members); err == nil || got != "" {
				t.Fatalf("malformed capture set accepted: id=%q err=%v", got, err)
			}
		})
	}
}

func TestMeasureFileBindsHeldGenerationAndContent(t *testing.T) {
	body := []byte("held trace generation\n")
	path := filepath.Join(t.TempDir(), "capture.systrace")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	bytesRead, digest, identity, err := MeasureFile(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(body)
	if bytesRead != int64(len(body)) || digest != hex.EncodeToString(want[:]) || !identity.Strong() {
		t.Fatalf("measurement mismatch: bytes=%d digest=%q identity=%s", bytesRead, digest, identity.CacheToken())
	}
	if err := ValidateFile(context.Background(), file, identity); err != nil {
		t.Fatalf("held generation did not validate: %v", err)
	}

	if _, err := file.WriteAt([]byte("X"), 0); err == nil {
		t.Fatal("read-only fixture unexpectedly allowed mutation")
	}
	if err := os.WriteFile(path, []byte("changed trace generation"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(context.Background(), file, identity); err == nil {
		t.Fatal("same-path generation mutation was not detected on the held descriptor")
	}
}

func TestMeasureFileHonorsPreCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.systrace")
	if err := os.WriteFile(path, []byte("trace"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := MeasureFile(ctx, file); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancellation lost: %v", err)
	}
}

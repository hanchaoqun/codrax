package hitraceconv

import (
	"os"
	"strings"
	"testing"
)

func TestConversionInputAuthorityOwnsTheOnlyContentOpenAndSharedIdentity(t *testing.T) {
	source, err := os.ReadFile("input_authority.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	if got := strings.Count(body, "file, err := openConversionInputFile(abs)"); got != 1 {
		t.Fatalf("content open count=%d want=1", got)
	}
	for _, required := range []string{
		"filegeneration.FromFile(file)",
		"filegeneration.FromPath(authority.requestedPath)",
		"filegeneration.FromPath(authority.canonicalPath)",
		"!identity.Strong()",
		"authority.identity.SameVersion(current)",
		"return authority.Section(0, authority.size)",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("input authority lost structural invariant %q", required)
		}
	}
	if strings.Contains(body, "tracequery.CaptureTraceSourceVersion") || strings.Contains(body, "os.ReadFile(") {
		t.Fatal("input authority used a source-universe identity or eager whole-file read")
	}
	viewStart := strings.Index(body, "type conversionInputView interface {")
	if viewStart < 0 {
		t.Fatal("conversion input parser view was not found")
	}
	viewEnd := strings.Index(body[viewStart:], "\n}")
	if viewEnd < 0 {
		t.Fatal("conversion input parser view was not terminated")
	}
	view := body[viewStart : viewStart+viewEnd]
	if strings.Contains(view, "CanonicalPath") {
		t.Fatal("parser-facing input view exposes canonical path reopening authority")
	}
	if !strings.Contains(body, "const conversionInputProbeSize = 64") || strings.Contains(body, "func (authority *conversionInputAuthority) Probe(maxBytes") {
		t.Fatal("input format probe is not mechanically fixed to 64 bytes")
	}
}

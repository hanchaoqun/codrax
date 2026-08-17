package tracequery

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sourceRawVisibilityTestLine(t *testing.T) string {
	t.Helper()
	schema := []byte(`{"version":1,"id":33086,"name":"hmfs_writepage","fields":[],"print_fmt":""}`)
	digest := sha256.Sum256(schema)
	body := strings.Join([]string{
		sourceRawVisibilityWire,
		"semantic_authority=none",
		"format_id=33086",
		"event_name_b64=" + base64.RawURLEncoding.EncodeToString([]byte("hmfs_writepage")),
		"schema_sha256=" + hex.EncodeToString(digest[:]),
		"payload_b64=" + base64.RawURLEncoding.EncodeToString([]byte{0x3e, 0x81, 0x04, 0x02}),
		"schema_b64=" + base64.RawURLEncoding.EncodeToString(schema),
	}, " ")
	return "worker-25827 (25827) [004] .... 32136.700490: hmfs_writepage: " + body
}

func TestSourceRawVisibilityCarrierIsExactAdvisoryOnly(t *testing.T) {
	line := sourceRawVisibilityTestLine(t)
	event, ok := ParseLine(1, line, nil)
	if !ok || event.Type != EventSourceRawVisibility || event.Name != "hmfs_writepage" ||
		event.SubsystemKind != "" || event.PID != 25827 || event.CPU != 4 {
		t.Fatalf("valid visibility carrier did not remain exact advisory-only: event=%+v ok=%t", event, ok)
	}

	path := filepath.Join(t.TempDir(), "visibility.systrace")
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if index.ParsedKnown != 1 || len(index.Events) != 0 {
		t.Fatalf("ordinary index admitted visibility carrier: known=%d events=%+v", index.ParsedKnown, index.Events)
	}
	streamed, err := StreamEventSearch(context.Background(), path, Query{
		View: "event_search", EventTypes: []EventType{EventSourceRawVisibility}, Limit: 10,
	})
	if err != nil || len(streamed.Events) != 1 ||
		streamed.Events[0].Type != EventSourceRawVisibility ||
		streamed.Events[0].SubsystemKind != "" {
		t.Fatalf("explicit streaming search lost visibility advisory: events=%+v err=%v", streamed.Events, err)
	}
}

func TestSourceRawVisibilityMalformedClaimCannotGainNameSemantics(t *testing.T) {
	valid := sourceRawVisibilityTestLine(t)
	for name, line := range map[string]string{
		"wrong_authority": strings.Replace(valid, "semantic_authority=none", "semantic_authority=filesystem", 1),
		"bad_schema_hash": strings.Replace(valid, "schema_sha256=", "schema_sha256=00", 1),
		"bad_payload":     strings.Replace(valid, "payload_b64=PoEEAg", "payload_b64=%", 1),
		"extra_token":     valid + " invented=1",
	} {
		t.Run(name, func(t *testing.T) {
			event, ok := ParseLine(1, line, nil)
			if !ok || event.Type != EventUnknown || event.SubsystemKind != "" {
				t.Fatalf("malformed reserved carrier gained event-name semantics: event=%+v ok=%t", event, ok)
			}
		})
	}
}

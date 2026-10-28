package traceinput

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestHMC304QueryDomainIsPreparationFactNotJSONOrFilename(t *testing.T) {
	dir := t.TempDir()
	var header bytes.Buffer
	for _, v := range []any{uint32(0xa1b2c3d4), uint16(2), uint16(4), int32(0), uint32(0), uint32(65535), uint32(147)} {
		if err := binary.Write(&header, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	// Incomplete record validation belongs to the actual query, not admission.
	body := append(header.Bytes(), 1, 2, 3)
	path := filepath.Join(dir, "capture.tracebundle.json")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := Prepare(t.Context(), Options{InputPath: path})
	if err != nil || m.QueryDomain() != attachment.TraceQueryDomainPacketCapture || m.SelfContainedText() {
		t.Fatalf("prepared domain lost or completeness overstated: %v %v", m, err)
	}
	if err := ValidateQueryMaterial(t.Context(), m, m.Preview()); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var restored attachment.TraceMaterial
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if err := ValidateQueryMaterial(t.Context(), &restored, m.Preview()); err == nil {
		t.Fatal("serialized receipt restored reader authority")
	}
	if err := ValidateQueryMaterial(t.Context(), nil, m.Preview()); err == nil {
		t.Fatal("missing receipt admitted")
	}
	if err := ValidateQueryMaterial(t.Context(), m, m.Preview()+"changed"); err == nil {
		t.Fatal("preview mismatch admitted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := ValidateQueryMaterial(ctx, m, m.Preview()); err == nil {
		t.Fatal("canceled admission succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("admission changed capture")
	}
}

func TestHMC304DeclaredPacketDomainStillChecksPhysicalHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-packets.pcap")
	if err := os.WriteFile(path, []byte("plain text, not a packet capture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := filegeneration.FromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	preview := "# codrax-source: " + path + "\npacket_inventory navigation\n"
	bindings := map[string]filegeneration.Identity{path: id}
	m, err := attachment.BindTraceMaterialForQueryDomain(path, path, preview, bindings, attachment.TraceQueryDomainPacketCapture)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateQueryMaterial(t.Context(), m, preview); err == nil {
		t.Fatal("reader declaration substituted for a physical packet header")
	}
	if _, err := attachment.BindTraceMaterialForQueryDomain(path, path, preview, bindings, attachment.TraceQueryDomain(255)); err == nil {
		t.Fatal("unknown query domain admitted")
	}
	legacy, err := Prepare(t.Context(), Options{InputPath: path})
	if err != nil || legacy.QueryDomain() != attachment.TraceQueryDomainTrace {
		t.Fatalf("filename changed actual text domain: %v %v", legacy, err)
	}
	if err := ValidateQueryMaterial(t.Context(), legacy, legacy.Preview()); err != nil {
		t.Fatal(err)
	}
}

package pcapinput

import (
	"encoding/binary"
	"encoding/json"
	"testing"
)

func TestHMC304PacketWireDoesNotReconstructReaderIdentity(t *testing.T) {
	path := putCapture(t, captureBytes(binary.LittleEndian, false, 101, 65535, ipv4UDP()))
	read, err := Stream(t.Context(), path, 1)
	if err != nil || read.ValidateCurrentSource() != nil {
		t.Fatalf("live reader lost its held source generation: %+v %v", read, err)
	}
	data, err := json.Marshal(read.PacketInventoryResult)
	if err != nil {
		t.Fatal(err)
	}
	var historical Result
	if err := json.Unmarshal(data, &historical); err != nil {
		t.Fatal(err)
	}
	if historical.PacketCount != 1 || historical.SourcePath != path || historical.ValidateCurrentSource() == nil {
		t.Fatalf("public packet facts reconstructed private source admission: %+v", historical)
	}
}

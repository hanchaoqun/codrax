package tracewire

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestHMC278ResourceSubtypeBackwardCompatibleCarrier(t *testing.T) {
	null := ResourceScalar{Status: "null"}
	e := ResourceEvent{SourceID: null, PID: 1, TID: 1, IPID: 1, ITID: 1, Thread: "app", Operation: "AllocEvent", CallchainID: null, Address: null, Size: null, EndNS: null, StackStatus: "unresolved_callchain"}
	r := ResourceStackRecord{TimestampNS: 1, EventRowID: 1, Event: &e}
	line, ok := FormatResourceStack(r)
	if !ok {
		t.Fatal("old carrier rejected")
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "subtype") || strings.Contains(string(b), "process") {
		t.Fatal("new fields changed old canonical bytes")
	}
	if _, ok := ParseResourceStack(ResourceStackPrefix + " record=" + base64.RawURLEncoding.EncodeToString(b)); !ok {
		t.Fatal("old canonical carrier no longer parses")
	}
	e.Subtype = &ResourceSubtype{ID: ResourceScalar{Status: "known", Value: "0"}, Name: ResourceText{Status: "known", Value: "缓存|纹理"}}
	line, ok = FormatResourceStack(r)
	if !ok {
		t.Fatal("new subtype rejected")
	}
	parsed, ok := ParseResourceStack(line)
	if !ok || parsed.Event.Subtype.Name.Value != "缓存|纹理" {
		t.Fatal("subtype did not round-trip")
	}
	e.Subtype.Name.Status = "guessed"
	if _, ok := FormatResourceStack(r); ok {
		t.Fatal("invalid typed subtype admitted")
	}
}

func TestHMC278ResourceFamilyUnitIsExplicit(t *testing.T) {
	for _, tc := range []struct{ operation, family, action, unit string }{
		{"AllocEvent", "heap", "allocate", "bytes"}, {"FreeEvent", "heap", "release", "bytes"}, {"MmapEvent", "mapping", "allocate", "bytes"}, {"MunmapEvent", "mapping", "release", "bytes"},
		{"FD_Open_Event", "FD", "open", "unspecified"}, {"FD_Close_Event", "FD", "close", "unspecified"}, {"THREAD_Create_Event", "THREAD", "create", "unspecified"}, {"THREAD_Destroy_Event", "THREAD", "destroy", "unspecified"},
		{"GPU_VK_Alloc_Event", "GPU_VK", "allocate", "unspecified"}, {"ARK_GLOBAL_HANDLE_Free_Event", "ARK_GLOBAL_HANDLE", "release", "unspecified"},
	} {
		family, action, unit, ok := ResourceOperationDescriptor(tc.operation)
		if !ok || family != tc.family || action != tc.action || unit != tc.unit {
			t.Fatalf("wrong descriptor for %s: %s %s %s", tc.operation, family, action, unit)
		}
	}
	for _, invalid := range []string{"malloc", "UNKNOWN_Alloc_Event", "AllocEvent noise"} {
		if _, _, _, ok := ResourceOperationDescriptor(invalid); ok {
			t.Fatalf("unregistered operation accepted %s", invalid)
		}
	}
}

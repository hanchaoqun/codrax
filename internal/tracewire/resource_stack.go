package tracewire

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

const ResourceStackPrefix = "# codrax_resource_stack/v1"

// Three 4096-byte frame texts can each expand six-fold in JSON and four-thirds
// in base64. 128 KiB covers that bound plus every scalar and envelope field.
// The converter sink accepts 1 MiB lines; the parser's scanner is larger still.
const ResourceStackMaxLineBytes = 128 << 10

// ResourceScalar retains SQLite INTEGERs as exact signed decimal strings. An
// address's unsigned bit pattern is derived only at presentation, never by
// floating-point conversion. NULL, absent and malformed are distinct.
type ResourceScalar struct {
	Status string `json:"status"`
	Value  string `json:"value,omitempty"`
}

func (s ResourceScalar) Valid() bool {
	switch s.Status {
	case "known":
		n, err := strconv.ParseInt(s.Value, 10, 64)
		return err == nil && strconv.FormatInt(n, 10) == s.Value
	case "null", "unavailable", "invalid":
		return s.Value == ""
	}
	return false
}

func (s ResourceScalar) Integer() (int64, bool) {
	if s.Status != "known" || !s.Valid() {
		return 0, false
	}
	n, _ := strconv.ParseInt(s.Value, 10, 64)
	return n, true
}

type ResourceText struct {
	Status string `json:"status"`
	Value  string `json:"value,omitempty"`
}

func (s ResourceText) Valid() bool {
	switch s.Status {
	case "known":
		return utf8.ValidString(s.Value) && len(s.Value) <= 4096
	case "null", "unavailable", "invalid", "ambiguous":
		return s.Value == ""
	}
	return false
}

// Frames are source-depth observations, not timed calls. Neither the deepest
// frame nor any fixed offset from it is automatically a business leaf.
type ResourceFrame struct {
	RowID        int64          `json:"row_id,string"`
	SourceID     ResourceScalar `json:"source_id"`
	Depth        ResourceScalar `json:"depth"`
	IP           ResourceScalar `json:"ip"`
	SymbolID     ResourceScalar `json:"symbol_id"`
	FileID       ResourceScalar `json:"file_id"`
	Offset       ResourceScalar `json:"offset"`
	SymbolOffset ResourceScalar `json:"symbol_offset"`
	VAddr        ResourceText   `json:"vaddr"`
	Symbol       ResourceText   `json:"symbol"`
	Library      ResourceText   `json:"library"`
}

func (f ResourceFrame) Valid() bool {
	return f.SourceID.Valid() && f.Depth.Valid() && f.IP.Valid() && f.SymbolID.Valid() && f.FileID.Valid() && f.Offset.Valid() && f.SymbolOffset.Valid() && f.VAddr.Valid() && f.Symbol.Valid() && f.Library.Valid()
}

// ResourceEvent carries an owner proven in the same sealed capture. CPU is
// deliberately absent: resource observation does not require sched execution.
type ResourceEvent struct {
	SourceID    ResourceScalar `json:"source_id"`
	PID         int            `json:"pid"`
	TID         int            `json:"tid"`
	IPID        int64          `json:"ipid,string"`
	ITID        int64          `json:"itid,string"`
	Thread      string         `json:"thread"`
	Operation   string         `json:"operation"`
	CallchainID ResourceScalar `json:"callchain_id"`
	Address     ResourceScalar `json:"address"`
	Size        ResourceScalar `json:"size"`
	EndNS       ResourceScalar `json:"end_ns"`
	FrameCount  int            `json:"frame_count"`
	StackStatus string         `json:"stack_status"`
}

func (e ResourceEvent) Valid() bool {
	if e.PID <= 0 || e.TID <= 0 || e.PID > math.MaxInt32 || e.TID > math.MaxInt32 || e.IPID <= 0 || e.ITID <= 0 || len(e.Thread) > 4096 || !utf8.ValidString(e.Thread) || !e.SourceID.Valid() || !e.CallchainID.Valid() || !e.Address.Valid() || !e.Size.Valid() || !e.EndNS.Valid() || e.FrameCount < 0 || e.FrameCount > 1_000_000 {
		return false
	}
	if !ValidResourceOperation(e.Operation) {
		return false
	}
	switch e.StackStatus {
	case "observed":
		n, ok := e.CallchainID.Integer()
		return ok && n >= 0 && n < math.MaxUint32 && e.FrameCount > 0
	case "no_frames", "frame_table_unavailable":
		n, ok := e.CallchainID.Integer()
		return ok && n >= 0 && n < math.MaxUint32 && e.FrameCount == 0
	case "unresolved_callchain":
		n, ok := e.CallchainID.Integer()
		return (!ok || n < 0 || n >= math.MaxUint32) && e.FrameCount == 0
	}
	return false
}

func ValidResourceOperation(s string) bool {
	normalized, _, ok := NormalizeResourceOperation(s)
	return ok && normalized == s
}

// NormalizeResourceOperation is shared by legacy instants and stack records.
func NormalizeResourceOperation(s string) (string, string, bool) {
	switch s {
	case "AllocEvent", "malloc":
		return "AllocEvent", "HeapSize", true
	case "FreeEvent", "free":
		return "FreeEvent", "HeapSize", true
	case "MmapEvent", "mmap":
		return "MmapEvent", "MmapSize", true
	case "MunmapEvent", "munmap":
		return "MunmapEvent", "MmapSize", true
	case "FD_Open_Event", "FD_Close_Event":
		return s, "NativeHook_FD_Active", true
	case "THREAD_Create_Event", "Thread_Create_Event":
		return "THREAD_Create_Event", "NativeHook_THREAD_Active", true
	case "THREAD_Destroy_Event", "Thread_Destroy_Event":
		return "THREAD_Destroy_Event", "NativeHook_THREAD_Active", true
	}
	for _, suffix := range []string{"_Alloc_Event", "_Free_Event"} {
		if strings.HasSuffix(s, suffix) {
			switch strings.TrimSuffix(s, suffix) {
			case "GPU_VK", "GPU_GLES", "GPU_CL", "OTHER", "ARKTS_HEAP", "JS_HEAP", "KMP_HEAP", "RN_HERMES_HEAP", "DART_HEAP", "ASHMEM", "ION", "SO", "ARK_GLOBAL_HANDLE", "ARK_LOCAL_HANDLE", "ARKTS_STATIC_HEAP":
				return s, "NativeHook_" + strings.TrimSuffix(s, suffix) + "_Active", true
			}
		}
	}
	return "", "", false
}

// Each frame refers to an event by stable physical row and exact timestamp.
// The timestamp belongs to the RESOURCE EVENT, never the frame execution.
// Per-frame records avoid a huge single JSON line or an export-time depth cap.
type ResourceStackRecord struct {
	TimestampNS int64          `json:"timestamp_ns,string"`
	EventRowID  int64          `json:"event_row_id,string"`
	Event       *ResourceEvent `json:"event,omitempty"`
	Frame       *ResourceFrame `json:"frame,omitempty"`
}

func (r ResourceStackRecord) Valid() bool {
	return r.TimestampNS >= 0 && ((r.Event != nil && r.Frame == nil && r.Event.Valid()) || (r.Frame != nil && r.Event == nil && r.Frame.Valid()))
}

func FormatResourceStack(r ResourceStackRecord) (string, bool) {
	if !r.Valid() {
		return "", false
	}
	b, err := json.Marshal(r)
	line := ResourceStackPrefix + " record=" + base64.RawURLEncoding.EncodeToString(b)
	return line, err == nil && len(line) <= ResourceStackMaxLineBytes
}

func ParseResourceStack(line string) (ResourceStackRecord, bool) {
	prefix := ResourceStackPrefix + " record="
	if !strings.HasPrefix(line, prefix) || len(line) > ResourceStackMaxLineBytes {
		return ResourceStackRecord{}, false
	}
	var r ResourceStackRecord
	b, err := base64.RawURLEncoding.Strict().DecodeString(line[len(prefix):])
	if err != nil || json.Unmarshal(b, &r) != nil {
		return r, false
	}
	canonical, ok := FormatResourceStack(r)
	return r, ok && canonical == line
}

package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// filemapRenderPayload is the sole value authority shared by direct RMQ,
// structured profiler and SQL-compatibility filemap rows. Page-cache events
// are instantaneous mutations, and writeback-error events are observations;
// no field in this payload is a duration endpoint.
type filemapRenderPayload struct {
	Kind         filemapRenderKind
	Name         string
	Dev          uint32
	Inode        uint64
	PFN          uint64
	Index        uint64
	Order        uint8
	OrderPresent bool
	File         uint64
	ErrSeq       uint32
	Old          uint32
	New          uint32
}

type filemapRenderKind uint8

const (
	filemapRenderUnknown filemapRenderKind = iota
	filemapRenderPageAdd
	filemapRenderPageDelete
	filemapRenderWritebackSet
	filemapRenderWritebackAdvance
)

type directFilemapFieldSpec struct {
	Name   string
	Type   string
	Offset int
	Size   int
	Signed bool
}

func directFilemapNameGoverned(name string) bool {
	_, ok := filemapRenderKindForName(name)
	return ok
}

func traceDBFilemapNameGoverned(name string) bool {
	switch name {
	case "mm_filemap_add_to_page_cache", "mm_filemap_delete_from_page_cache", "filemap_set_wb_err":
		return true
	default:
		return false
	}
}

func filemapRenderKindForName(name string) (filemapRenderKind, bool) {
	switch name {
	case "mm_filemap_add_to_page_cache":
		return filemapRenderPageAdd, true
	case "mm_filemap_delete_from_page_cache":
		return filemapRenderPageDelete, true
	case "filemap_set_wb_err":
		return filemapRenderWritebackSet, true
	case "file_check_and_advance_wb_err":
		return filemapRenderWritebackAdvance, true
	default:
		return filemapRenderUnknown, false
	}
}

func renderCanonicalFilemapPayload(payload filemapRenderPayload) (string, bool) {
	kind, governed := filemapRenderKindForName(payload.Name)
	if !governed || kind != payload.Kind {
		return "", false
	}
	dev := devMajorMinor(int64(payload.Dev), ":")
	switch kind {
	case filemapRenderPageAdd, filemapRenderPageDelete:
		if payload.Index > uint64(math.MaxInt64>>12) {
			return "", false
		}
		return fmt.Sprintf("dev %s ino 0x%x pfn=%d ofs=%d", dev, payload.Inode,
			payload.PFN, payload.Index<<12), true
	case filemapRenderWritebackSet:
		return fmt.Sprintf("dev=%s ino=0x%x errseq=0x%x", dev, payload.Inode, payload.ErrSeq), true
	case filemapRenderWritebackAdvance:
		if payload.File == 0 {
			return "", false
		}
		return fmt.Sprintf("file=0x%x dev=%s ino=0x%x old=0x%x new=0x%x", payload.File,
			dev, payload.Inode, payload.Old, payload.New), true
	default:
		return "", false
	}
}

func decodeDirectFilemapPayload(ev decodedEvent) (filemapRenderPayload, bodyAdmission, string) {
	kind, governed := filemapRenderKindForName(ev.format.Name)
	if !governed {
		return filemapRenderPayload{}, bodyUnsupported, ""
	}
	payload := filemapRenderPayload{Kind: kind, Name: ev.format.Name}
	fields, ok := directFilemapProfileFields(ev, kind)
	if !ok {
		return payload, bodyRejected, "missing_or_invalid_filemap_profile"
	}

	read := func(name string) uint64 {
		return directFilemapUint(fields[name])
	}
	switch kind {
	case filemapRenderPageAdd, filemapRenderPageDelete:
		payload.PFN = read("pfn")
		payload.Inode = read("i_ino")
		payload.Index = read("index")
		payload.Dev = uint32(read("s_dev"))
		if raw, present := fields["order"]; present {
			payload.Order = raw[0]
			payload.OrderPresent = true
		}
		if payload.Index > uint64(math.MaxInt64>>12) {
			return payload, bodyRejected, "filemap_index_out_of_range"
		}
	case filemapRenderWritebackSet:
		payload.Inode = read("i_ino")
		payload.Dev = uint32(read("s_dev"))
		payload.ErrSeq = uint32(read("errseq"))
	case filemapRenderWritebackAdvance:
		payload.File = read("file")
		payload.Inode = read("i_ino")
		payload.Dev = uint32(read("s_dev"))
		payload.Old = uint32(read("old"))
		payload.New = uint32(read("new"))
		if payload.File == 0 {
			return payload, bodyRejected, "filemap_file_pointer_zero"
		}
	default:
		return payload, bodyRejected, "invalid_filemap_kind"
	}
	return payload, bodyAdmitted, ""
}

func directFilemapProfileFields(ev decodedEvent, kind filemapRenderKind) (map[string][]byte, bool) {
	profiles := directFilemapProfiles(kind)
	var matched map[string][]byte
	for _, profile := range profiles {
		fields, ok := directFilemapExactFields(ev, profile)
		if !ok {
			continue
		}
		if matched != nil {
			return nil, false
		}
		matched = fields
	}
	return matched, matched != nil
}

func directFilemapProfiles(kind filemapRenderKind) [][]directFilemapFieldSpec {
	var profiles [][]directFilemapFieldSpec
	for _, wordSize := range []int{4, 8} {
		switch kind {
		case filemapRenderPageAdd, filemapRenderPageDelete:
			base := []directFilemapFieldSpec{
				{Name: "pfn", Type: "unsigned long", Offset: 8, Size: wordSize},
				{Name: "i_ino", Type: "unsigned long", Offset: 8 + wordSize, Size: wordSize},
				{Name: "index", Type: "unsigned long", Offset: 8 + 2*wordSize, Size: wordSize},
				{Name: "s_dev", Type: "dev_t", Offset: 8 + 3*wordSize, Size: 4},
			}
			profiles = append(profiles, base)
			modern := append([]directFilemapFieldSpec(nil), base...)
			modern = append(modern, directFilemapFieldSpec{
				Name: "order", Type: "unsigned char", Offset: 12 + 3*wordSize, Size: 1,
			})
			profiles = append(profiles, modern)
		case filemapRenderWritebackSet:
			profiles = append(profiles, []directFilemapFieldSpec{
				{Name: "i_ino", Type: "unsigned long", Offset: 8, Size: wordSize},
				{Name: "s_dev", Type: "dev_t", Offset: 8 + wordSize, Size: 4},
				{Name: "errseq", Type: "errseq_t", Offset: 12 + wordSize, Size: 4},
			})
		case filemapRenderWritebackAdvance:
			profiles = append(profiles, []directFilemapFieldSpec{
				{Name: "file", Type: "struct file *", Offset: 8, Size: wordSize},
				{Name: "i_ino", Type: "unsigned long", Offset: 8 + wordSize, Size: wordSize},
				{Name: "s_dev", Type: "dev_t", Offset: 8 + 2*wordSize, Size: 4},
				{Name: "old", Type: "errseq_t", Offset: 12 + 2*wordSize, Size: 4},
				{Name: "new", Type: "errseq_t", Offset: 16 + 2*wordSize, Size: 4},
			})
		}
	}
	return profiles
}

func directFilemapExactFields(ev decodedEvent, specs []directFilemapFieldSpec) (map[string][]byte, bool) {
	if len(specs) == 0 {
		return nil, false
	}
	expected := make(map[string]directFilemapFieldSpec, len(specs))
	for _, spec := range specs {
		expected[spec.Name] = spec
	}
	values := make(map[string][]byte, len(specs))
	common := make(map[string]bool, 4)
	for index, field := range ev.format.Fields {
		name := cleanFieldName(field.Name)
		if strings.HasPrefix(name, "common_") {
			spec, found := directFilemapCommonFieldSpec(name)
			if !found || common[name] || field.Name != spec.Name || field.Signed != spec.Signed ||
				normalizeFieldType(field.Type) != spec.Type || field.Offset != spec.Offset ||
				field.Size != spec.Size || !directDescriptorFieldIsolated(ev, index) {
				return nil, false
			}
			common[name] = true
			continue
		}
		spec, found := expected[name]
		if !found || values[name] != nil || field.Name != spec.Name || field.Signed != spec.Signed ||
			normalizeFieldType(field.Type) != spec.Type || field.Offset != spec.Offset ||
			field.Size != spec.Size || !directDescriptorFieldIsolated(ev, index) {
			return nil, false
		}
		raw, present := ev.fields[field.Name]
		if !present || len(raw) != field.Size {
			return nil, false
		}
		values[name] = raw
	}
	if len(common) != 4 || len(values) != len(specs) {
		return nil, false
	}
	return values, true
}

func directFilemapCommonFieldSpec(name string) (directFilemapFieldSpec, bool) {
	switch name {
	case "common_type":
		return directFilemapFieldSpec{Name: name, Type: "unsigned short", Offset: 0, Size: 2}, true
	case "common_flags":
		return directFilemapFieldSpec{Name: name, Type: "unsigned char", Offset: 2, Size: 1}, true
	case "common_preempt_count":
		return directFilemapFieldSpec{Name: name, Type: "unsigned char", Offset: 3, Size: 1}, true
	case "common_pid":
		return directFilemapFieldSpec{Name: name, Type: "int", Offset: 4, Size: 4, Signed: true}, true
	default:
		return directFilemapFieldSpec{}, false
	}
}

func directFilemapUint(raw []byte) uint64 {
	switch len(raw) {
	case 1:
		return uint64(raw[0])
	case 4:
		return uint64(binary.LittleEndian.Uint32(raw))
	case 8:
		return binary.LittleEndian.Uint64(raw)
	default:
		return 0
	}
}

func decodeTraceDBFilemapPayload(name string, args map[string]traceDBValue,
	invalidKeys map[string]bool,
) (filemapRenderPayload, bool) {
	if !traceDBFilemapNameGoverned(name) {
		return filemapRenderPayload{}, false
	}
	kind, _ := filemapRenderKindForName(name)
	payload := filemapRenderPayload{Kind: kind, Name: name}
	allowed := map[string]bool{}
	switch kind {
	case filemapRenderPageAdd, filemapRenderPageDelete:
		for _, key := range []string{"s_dev", "i_ino", "index", "pfn", "order"} {
			allowed[key] = true
		}
	case filemapRenderWritebackSet:
		for _, key := range []string{"s_dev", "i_ino", "errseq"} {
			allowed[key] = true
		}
	default:
		return payload, false
	}
	for key := range args {
		if !allowed[key] {
			return payload, false
		}
	}
	for key, invalid := range invalidKeys {
		if invalid || !allowed[key] {
			return payload, false
		}
	}
	read := func(key string, max uint64, required bool) (uint64, bool, bool) {
		value, present := args[key]
		if !present {
			return 0, false, !required
		}
		if !value.Valid || value.Datatype != 0 || value.Text == "" || value.Text != strings.TrimSpace(value.Text) {
			return 0, true, false
		}
		parsed, err := strconv.ParseUint(value.Text, 10, 64)
		if err != nil || parsed > max || strconv.FormatUint(parsed, 10) != value.Text {
			return 0, true, false
		}
		return parsed, true, true
	}
	dev, _, ok := read("s_dev", math.MaxUint32, true)
	if !ok {
		return payload, false
	}
	inode, _, ok := read("i_ino", math.MaxInt64, true)
	if !ok {
		return payload, false
	}
	payload.Dev, payload.Inode = uint32(dev), inode
	switch kind {
	case filemapRenderPageAdd, filemapRenderPageDelete:
		index, _, valid := read("index", uint64(math.MaxInt64>>12), true)
		if !valid {
			return payload, false
		}
		pfn, _, valid := read("pfn", math.MaxInt64, true)
		if !valid {
			return payload, false
		}
		order, present, valid := read("order", math.MaxUint8, false)
		if !valid {
			return payload, false
		}
		payload.Index, payload.PFN = index, pfn
		payload.Order, payload.OrderPresent = uint8(order), present
	case filemapRenderWritebackSet:
		errseq, _, valid := read("errseq", math.MaxUint32, true)
		if !valid {
			return payload, false
		}
		payload.ErrSeq = uint32(errseq)
	}
	return payload, true
}

func decodeProfilerFilemapPayload(event profilerFtraceEventRecord) (filemapRenderPayload, bodyAdmission, string) {
	name := ""
	kind := filemapRenderUnknown
	switch event.Field {
	case 1000:
		name, kind = "mm_filemap_add_to_page_cache", filemapRenderPageAdd
	case 1001:
		name, kind = "mm_filemap_delete_from_page_cache", filemapRenderPageDelete
	default:
		return filemapRenderPayload{}, bodyUnsupported, ""
	}
	payload := filemapRenderPayload{Kind: kind, Name: name}
	read := func(field int, max uint64) (uint64, bool) {
		value, state, _ := protoScalarUint(event.Payload, field)
		return value, state != protoScalarInvalid && value <= max
	}
	var ok bool
	if payload.PFN, ok = read(1, math.MaxUint64); !ok {
		return payload, bodyRejected, "filemap_pfn_invalid"
	}
	if payload.Inode, ok = read(2, math.MaxUint64); !ok {
		return payload, bodyRejected, "filemap_inode_invalid"
	}
	if payload.Index, ok = read(3, uint64(math.MaxInt64>>12)); !ok {
		return payload, bodyRejected, "filemap_index_invalid"
	}
	dev, ok := read(4, math.MaxUint32)
	if !ok {
		return payload, bodyRejected, "filemap_device_invalid"
	}
	payload.Dev = uint32(dev)
	order, state, _ := protoScalarUint(event.Payload, 5)
	if state == protoScalarInvalid || order > math.MaxUint8 {
		return payload, bodyRejected, "filemap_order_invalid"
	}
	if state == protoScalarPresent {
		payload.Order, payload.OrderPresent = uint8(order), true
	}
	return payload, bodyAdmitted, ""
}

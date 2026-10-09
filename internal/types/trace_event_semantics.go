package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	TraceEventSemanticsVersion       = 1
	TraceEventSemanticFieldLimit     = 32
	TraceEventSemanticValueByteLimit = 1024
	TraceEventSemanticsByteLimit     = 16 * 1024
)

// TraceEventSemantics is a producer-owned display projection of parsed fields.
// It proves neither a scheduler identity, a paired interval nor a causal edge.
// Nil is the legacy/no-supported-fields state, not an empty observed population.
type TraceEventSemantics struct {
	SchemaVersion int                       `json:"schema_version"`
	Fields        []TraceEventSemanticField `json:"fields"`
}

type TraceEventSemanticField struct {
	Key    string `json:"key"`
	Type   string `json:"type"`
	Unit   string `json:"unit,omitempty"`
	Status string `json:"status"`
	// A pointer preserves a known empty string. Numeric values remain exact
	// decimal strings; Type is checked against the fixed descriptor registry.
	Value       *string                     `json:"value,omitempty"`
	IssueReason string                      `json:"issue_reason,omitempty"`
	Omitted     *TraceEventSemanticOmission `json:"omitted,omitempty"`
}

type TraceEventSemanticOmission struct {
	OriginalBytes int    `json:"original_bytes"`
	SHA256        string `json:"sha256"`
}

// Descriptors, not model text or payload keys, define the meaning and unit of
// each admitted field. An empty unit explicitly makes no numeric-unit claim.
// New semantic families extend this registry and their native projector; they
// do not obtain any authority from being present in an event inventory.
type TraceEventSemanticDescriptor struct {
	Key    string
	Family string
	Type   string
	Unit   string
	Label  string
}

var traceEventSemanticDescriptors = []TraceEventSemanticDescriptor{
	{"plugin.domain", "plugin", "text", "", "业务域"},
	{"plugin.event_name", "plugin", "text", "", "业务事件名"},
	{"plugin.event_label", "plugin", "text", "", "解析事件标签（原字段是否存在未确定）"},
	{"plugin.metric", "plugin", "text", "", "指标键"},
	{"plugin.value", "plugin", "text", "", "指标原始值（单位未推定）"},
	{"plugin.category", "plugin", "text", "", "业务分类"},
	{"plugin.contents", "plugin", "text", "", "解析后的业务事件内容"},
	{"plugin.domain_ref", "plugin", "int64", "", "业务域原始字典引用"},
	{"plugin.event_name_ref", "plugin", "int64", "", "事件名原始字典引用"},
	{"marker.action", "marker", "text", "", "标记动作"},
	{"marker.name", "marker", "text", "", "标记标签（合成标签的源业务名另列）"},
	{"marker.business_name", "marker", "text", "", "源业务名称（可能未知，区别于合成标记标签）"},
	{"marker.business_name_ref", "marker", "int64", "", "业务名称原始字典引用"},
	{"marker.label_origin", "marker", "text", "", "标记标签的产生方式"},
	{"source.table", "source", "text", "", "源数据表（非业务域）"},
	{"source.row_id", "source", "int64", "", "源表物理记录编号（仅在同一源文件代次与表内定位，不是业务实例或因果关系）"},
	{"source.owner_ipid", "source", "int64", "", "源表进程引用（不是系统PID，不证明进程生命周期）"},
	{"source.owner_pid", "source", "int64", "", "源表所属进程PID（不是发射线程，不证明生命周期）"},
	{"source.subject_role", "source", "text", "", "源记录主体角色（进程所属区间不代表线程执行）"},
	{"source.emitter_tid", "source", "int64", "", "原始发射线程（缺测不猜测主线程）"},
	{"source.execution_cpu", "source", "int64", "", "执行CPU（缺测不补CPU0）"},
	{"source.start_ns", "source", "int64", "ns", "源记录完整区间起点（非查询窗起点）"},
	{"source.end_ns", "source", "int64", "ns", "源记录完整区间终点（不证明显示或可交互）"},
	{"source.duration_ns", "source", "int64", "ns", "源记录完整区间时长（非窗口内累计值）"},
	{"marker.payload_pid", "marker", "int64", "", "标记载荷进程标识（非发出线程）"},
	{"marker.track", "marker", "text", "", "标记轨道"},
	{"marker.value", "marker", "text", "", "标记原始附加值"},
	{"counter.value", "counter", "decimal", "", "计数器原始数值（单位未知）"},
	{"counter.raw_value", "counter", "text", "", "未接纳的计数器原始值"},
	{"counter.owner_scope", "counter", "text", "", "计数器归属范围"},
	{"counter.metadata", "counter", "text", "", "计数器元数据"},
	{"counter.output_level", "counter", "text", "", "计数器输出级别"},
	{"counter.tag_bits", "counter", "text", "", "计数器标签位"},
	{"counter.aggregation_status", "counter", "text", "", "既有数值聚合是否接纳"},
	{"counter.issue", "counter", "text", "", "计数器解析或聚合限制"},
	{"source.representation", "source", "text", "", "记录表示来源（不证明原生注入）"},
	{"source.timestamp_ns", "source", "int64", "ns", "源记录时刻"},
	{"source.tid", "source", "int64", "", "源记录线程标识（非调度身份凭证）"},
	{"source.tid_storage_class", "source", "text", "", "无效线程标识的存储类型"},
	{"source.tid_raw", "source", "text", "", "无效线程标识的原始值"},
	{"source.tid_base64", "source", "text", "", "无效线程标识的原始字节（Base64）"},
	{"source.contents", "source", "text", "", "源记录内容"},
	{"source.contents_storage_class", "source", "text", "", "源内容存储类型"},
	{"source.contents_base64", "source", "text", "", "源内容字节的Base64表示"},
	{"resource.operation", "resource", "text", "", "资源操作（瞬时观测，不是执行区间）"},
	{"resource.end_ts_ns", "resource", "int64", "ns", "资源结束原始时刻（不单独证明释放）"},
	{"resource.size", "resource", "int64", "", "资源原始数量（单位依资源种类，不统一假定字节）"},
	{"resource.callchain_id", "resource", "int64", "", "源调用栈引用（未展开，不证明函数执行）"},
	{"resource.address_i64", "resource", "int64", "", "地址原始有符号整数（负号不判有效性）"},
	{"resource.address_bits_hex", "resource", "text", "", "同一地址的64位十六进制位型（不判有效性）"},
	{"resource.sub_type_id", "resource", "int64", "", "资源子类原始引用"},
	{"resource.sub_type_name", "resource", "text", "", "已提供的资源子类名称"},
}

func LookupTraceEventSemanticDescriptor(key string) (TraceEventSemanticDescriptor, bool) {
	for _, descriptor := range traceEventSemanticDescriptors {
		if descriptor.Key == key {
			return descriptor, true
		}
	}
	return TraceEventSemanticDescriptor{}, false
}

func ValidateTraceEventSemantics(value *TraceEventSemantics) bool {
	if value == nil {
		return true
	}
	if value.SchemaVersion != TraceEventSemanticsVersion || len(value.Fields) == 0 || len(value.Fields) > TraceEventSemanticFieldLimit {
		return false
	}
	seen := make(map[string]bool, len(value.Fields))
	for _, field := range value.Fields {
		descriptor, ok := LookupTraceEventSemanticDescriptor(field.Key)
		if !ok || seen[field.Key] || field.Type != descriptor.Type || field.Unit != descriptor.Unit || !traceEventSemanticReasonValid(field.IssueReason) {
			return false
		}
		seen[field.Key] = true
		switch field.Status {
		case "known":
			if field.Value == nil || field.Omitted != nil || field.IssueReason != "" || !traceEventSemanticValueValid(field.Type, *field.Value) || !traceEventSemanticFieldValueValid(field.Key, *field.Value) {
				return false
			}
		case "unavailable", "invalid":
			if field.Value != nil || field.Omitted != nil || field.IssueReason == "" {
				return false
			}
		case "omitted":
			if field.Value != nil || field.Omitted == nil || field.IssueReason == "" || field.Omitted.OriginalBytes <= 0 {
				return false
			}
			digest, err := hex.DecodeString(field.Omitted.SHA256)
			if err != nil || len(digest) != sha256.Size || strings.ToLower(field.Omitted.SHA256) != field.Omitted.SHA256 {
				return false
			}
		default:
			return false
		}
	}
	wire, err := json.Marshal(value)
	return err == nil && len(wire) <= TraceEventSemanticsByteLimit
}

func traceEventSemanticFieldValueValid(key, value string) bool {
	switch key {
	case "marker.action":
		return len(value) == 1 && strings.Contains("BESFGHNIC", value) || value == "source_begin" || value == "source_end"
	case "source.subject_role":
		return value == "process_owned_interval" || value == "process_measurement_not_thread_execution"
	case "counter.owner_scope":
		return value == "global" || value == "payload_process"
	case "counter.aggregation_status":
		return value == "admitted" || value == "excluded" || value == "not_aggregated_unit_and_quantity_semantics_unknown"
	case "source.representation":
		return value == "parsed_plugin_fields" || value == "parsed_trace_marker" || value == "sql_hisysevent" || value == "sql_app_startup" || value == "sql_process_measure_interval"
	case "source.table":
		return value == "app_startup" || value == "hisys_all_event" || value == "process_measure"
	case "marker.label_origin":
		return value == "synthesized_sql_label"
	case "source.contents_storage_class", "source.tid_storage_class":
		return value == "null" || value == "text" || value == "blob" || value == "integer" || value == "real"
	case "source.timestamp_ns", "source.tid", "marker.payload_pid":
		return !strings.HasPrefix(value, "-")
	case "resource.address_bits_hex":
		if len(value) != 18 || !strings.HasPrefix(value, "0x") {
			return false
		}
		_, err := strconv.ParseUint(value[2:], 16, 64)
		return err == nil && value == strings.ToLower(value)
	}
	return true
}

func traceEventSemanticsMatchEventType(value *TraceEventSemantics, eventType string) bool {
	if value == nil {
		return true
	}
	if eventType == "process_measure_interval" {
		for _, field := range value.Fields {
			switch field.Key {
			case "source.representation", "source.table", "source.subject_role", "counter.aggregation_status":
				if field.Value == nil || !processMeasurementSemanticDiscriminator(field.Key, *field.Value) {
					return false
				}
			case "source.row_id", "source.start_ns", "source.duration_ns", "source.end_ns", "source.owner_ipid", "source.owner_pid", "plugin.metric", "plugin.category", "plugin.value", "counter.raw_value":
			default:
				return false
			}
		}
		return true
	}
	plugin := eventType == "hi_sysevent" || eventType == "ability_monitor" || eventType == "xpower"
	marker := eventType == "trace_mark"
	for _, field := range value.Fields {
		if field.Value != nil && processMeasurementSemanticDiscriminator(field.Key, *field.Value) {
			return false
		}
		descriptor, ok := LookupTraceEventSemanticDescriptor(field.Key)
		if !ok || descriptor.Family == "plugin" && !plugin || (descriptor.Family == "marker" || descriptor.Family == "counter" || descriptor.Family == "resource") && !marker || descriptor.Family == "source" && !plugin && !marker {
			return false
		}
	}
	return true
}

func processMeasurementSemanticDiscriminator(key, value string) bool {
	switch key {
	case "source.representation":
		return value == "sql_process_measure_interval"
	case "source.table":
		return value == "process_measure"
	case "source.subject_role":
		return value == "process_measurement_not_thread_execution"
	case "counter.aggregation_status":
		return value == "not_aggregated_unit_and_quantity_semantics_unknown"
	}
	return false
}

func traceEventSemanticReasonValid(reason string) bool {
	if len(reason) > 96 {
		return false
	}
	for _, ch := range reason {
		if ch != '_' && (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}

func traceEventSemanticValueValid(kind, value string) bool {
	if len(value) > TraceEventSemanticValueByteLimit || !utf8.ValidString(value) {
		return false
	}
	switch kind {
	case "text":
		return true
	case "int64":
		n, err := strconv.ParseInt(value, 10, 64)
		return err == nil && strconv.FormatInt(n, 10) == value
	case "uint64":
		n, err := strconv.ParseUint(value, 10, 64)
		return err == nil && strconv.FormatUint(n, 10) == value
	case "decimal":
		if value == "" {
			return false
		}
		digits, dots := 0, 0
		for n, ch := range value {
			if n == 0 && (ch == '-' || ch == '+') {
				continue
			}
			if ch == '.' {
				dots++
				if dots > 1 {
					return false
				}
				continue
			}
			if ch < '0' || ch > '9' {
				return false
			}
			digits++
		}
		return digits > 0
	}
	return false
}

// OmitTraceEventSemanticValue never shortens an identity into another identity.
// The digest and length describe the complete UTF-8 value, not its JSON quoting.
func OmitTraceEventSemanticValue(field TraceEventSemanticField, reason string) TraceEventSemanticField {
	if field.Value == nil || *field.Value == "" {
		return field
	}
	digest := sha256.Sum256([]byte(*field.Value))
	field.Omitted = &TraceEventSemanticOmission{OriginalBytes: len(*field.Value), SHA256: hex.EncodeToString(digest[:])}
	field.Value, field.Status, field.IssueReason = nil, "omitted", reason
	return field
}

func CloneTraceEventSemantics(in *TraceEventSemantics) *TraceEventSemantics {
	if in == nil {
		return nil
	}
	out := *in
	out.Fields = append([]TraceEventSemanticField(nil), in.Fields...)
	for n := range out.Fields {
		if in.Fields[n].Value != nil {
			value := *in.Fields[n].Value
			out.Fields[n].Value = &value
		}
		if in.Fields[n].Omitted != nil {
			value := *in.Fields[n].Omitted
			out.Fields[n].Omitted = &value
		}
	}
	return &out
}

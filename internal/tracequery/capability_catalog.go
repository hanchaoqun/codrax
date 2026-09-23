package tracequery

import "fmt"

// CapabilityCatalog describes code capabilities, never the contents of a
// capture. None of these descriptors participate in query admission or rank.
type CapabilityCatalog struct {
	Version             int                     `json:"version"`
	StaticOnly          bool                    `json:"static_only"`
	Evidence            bool                    `json:"evidence"`
	CaptureAvailability string                  `json:"capture_availability"`
	QueryTimeUnit       string                  `json:"query_time_unit"`
	QueryTimeAxis       string                  `json:"query_time_axis"`
	MissingDataPolicy   string                  `json:"missing_data_policy"`
	DescriptorScope     string                  `json:"descriptor_scope"`
	Views               []ViewCapability        `json:"views"`
	Metrics             []MetricCapability      `json:"metrics,omitempty"`
	InputFormats        []InputFormatCapability `json:"input_formats,omitempty"`
}

type ViewCapability struct {
	View         string   `json:"view"`
	Summary      string   `json:"summary"`
	Objects      []string `json:"objects"`
	InputFormats []string `json:"input_formats"`
	MetricRefs   []string `json:"metric_refs"`
	Components   []string `json:"components,omitempty"`
	Limitations  []string `json:"limitations"`
}

type MetricCapability struct {
	ID           string                 `json:"id"`
	Summary      string                 `json:"summary"`
	Outputs      []CapabilityOutput     `json:"outputs"`
	Requirements CapabilityRequirements `json:"requirements"`
	Limitations  []string               `json:"limitations"`
	Filter       *CapabilityFilter      `json:"filter,omitempty"`
}

type CapabilityFilter struct {
	View          string   `json:"view"`
	Parameter     string   `json:"parameter"`
	Fields        []string `json:"fields"`
	Operators     []string `json:"operators"`
	MaxPredicates int      `json:"max_predicates"`
}

// Section is a JSON path in Result; Fields are immediate fields on that
// object (or its array members). The catalog describes representative numeric
// rulers, not a replacement schema for every identity/evidence field.
type CapabilityOutput struct {
	Section string   `json:"section"`
	Fields  []string `json:"fields"`
	Unit    string   `json:"unit"`
	Caliber string   `json:"caliber"`
}

// Each AnyOf entry is an alternative set of jointly needed event families.
// Conditions state additional identity/coverage premises: merely finding an
// event name never proves they hold. These are descriptive, not executable gates.
type CapabilityRequirements struct {
	AllOf      []string   `json:"all_of,omitempty"`
	AnyOf      [][]string `json:"any_of,omitempty"`
	Optional   []string   `json:"optional,omitempty"`
	Conditions []string   `json:"conditions"`
}

type InputFormatCapability struct {
	ID           string `json:"id"`
	Handling     string `json:"handling"`
	Prerequisite string `json:"prerequisite"`
	Limitation   string `json:"limitation"`
}

// TraceCapabilities returns freshly constructed metadata. Empty view selects
// all views; non-empty selection is canonical-only. Tool-specific aliases are
// resolved by the tool's own schema, not silently added to the engine grammar.
func TraceCapabilities(view string, detail bool) (CapabilityCatalog, error) {
	catalog := CapabilityCatalog{
		Version: 1, StaticOnly: true, Evidence: false, CaptureAvailability: "not_evaluated",
		QueryTimeUnit: "seconds", QueryTimeAxis: "time_start/time_end and normalized event timestamps use artifact-local trace seconds. Native jank start_ts_ns/end_ts_ns remain nanoseconds on that same axis; conversion is a unit conversion, not a new clock alignment. Other metric units are declared separately; cross-source alignment requires its own receipt.",
		MissingDataPolicy: "No measurements are produced by this catalog. Inspect actual query coverage and optional fields; missing, unsupported, canceled or unmeasured is not measured zero. Static support proves neither capture contents nor causality.",
		DescriptorScope:   "Metric outputs are representative existing Result carriers across views, not a replacement result schema or a promise that every path appears in the selected view. Metric references describe related measurement contracts; components are conditional composition choices. Input-format references describe intake routes, not the event families present in a capture.",
	}
	views := capabilityViewDescriptors()
	metrics := capabilityMetricDescriptors()
	selectedMetrics := map[string]bool{}
	byView := map[string]ViewCapability{}
	for _, descriptor := range views {
		byView[descriptor.View] = descriptor
	}
	visited := map[string]bool{}
	var includeMetrics func(string)
	includeMetrics = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		descriptor := byView[name]
		for _, id := range descriptor.MetricRefs {
			selectedMetrics[id] = true
		}
		for _, component := range descriptor.Components {
			includeMetrics(component)
		}
	}
	for _, descriptor := range views {
		if view != "" && descriptor.View != view {
			continue
		}
		catalog.Views = append(catalog.Views, descriptor)
		includeMetrics(descriptor.View)
	}
	if len(catalog.Views) == 0 {
		return CapabilityCatalog{}, fmt.Errorf("unknown canonical trace view %q", view)
	}
	if detail {
		for _, metric := range metrics {
			if selectedMetrics[metric.ID] {
				catalog.Metrics = append(catalog.Metrics, metric)
			}
		}
		catalog.InputFormats = []InputFormatCapability{
			{"trace_text", "direct", "Parser-recognized ftrace/systrace/atrace/hitrace text; required semantic fields must parse.", "An extension or raw visibility row does not grant scheduler, duration or causal capability."},
			{"perf_trace_text", "direct", "Typed perf_sample text with event/weight unit and source provenance.", "Samples do not supply missing scheduler transitions; unaligned clocks cannot be joined."},
			{"tracebundle", "manifest", "Resolvable material paths with source identity and conversion/clock receipts.", "A manifest is not trace content; each referenced material keeps its own coverage and capability."},
			{"native_trace", "conditional_conversion", "Complete files: Harmony RMQ (CE 0A header; built-in decoder requires version/file_type support), OpenHarmony raw (49 DF), or OHOSPROF profiler containers. Non-built-in layouts need an available compatible trace_streamer/provider; query-ready semantic output is validated.", "Magic identifies a candidate, not complete compatibility: version, payload and provider must pass. No arbitrary binary or Perfetto support is promised. Binary stdin/inline is unsupported; a preview is not the complete query material."},
			{"native_perf", "conditional_conversion", "Complete Linux perf.data/HIPERF/simpleperf files detected by PERFILE2 or 2ELIFREP, or SIMPLEPERF report-sample protobuf, then admitted by format/endian validation and an available official or bounded raw conversion provider.", "Detection is not a guarantee of every endian/record feature. Conversion retains clock, identity, event/weight and symbolization quality; missing CPU/symbols/callchains/alignment remain missing and scheduler rows are not invented."},
			{"gzip", "conditional_transport", "One validated gzip member whose fully decoded content is accepted trace text or a supported native input; resource limits, integrity, source generation and transport receipt must pass.", "Gzip magic alone supplies no semantic capability; concatenated/nested or unsupported decoded content is not automatically admitted."},
			{"zip", "conditional_transport", "A validated ZIP with a uniquely selected regular .sys/.htrace member accepted by native conversion; archive/member integrity, generation and provenance receipts must pass.", "Ambiguous members, unsafe/encrypted/special/nested entries or unknown content are rejected; this is not a general ZIP attachment reader."},
			{"sqlite", "unsupported_direct_input", "An existing SQLite database requires explicit export to supported trace text before trace_query.", "A .db name does not select or authorize a database reader; ordinary trace_query does not automatically import SQLite."},
		}
	}
	return catalog, nil
}

// CapabilityViewNames preserves the established public schema order. The
// membership is checked bidirectionally against the engine capacity registry.
func CapabilityViewNames() []string {
	var names []string
	for _, view := range capabilityViewDescriptors() {
		names = append(names, view.View)
	}
	return names
}

package tracequery

// NativeIntervalNavigation connects an already parsed family (or a source
// export disclosure) to its existing interval-aware view. It is navigation,
// not proof of capture contents, unit semantics, identity, or computation.
// CoverageFamily/SourceTable are exact exporter keys; a manifest match is only
// a candidate, since older exporters may expose point rows from the same table.
type NativeIntervalNavigation struct {
	EventType      EventType `json:"event_type"`
	View           string    `json:"view"`
	SourceTable    string    `json:"source_table"`
	CoverageFamily string    `json:"coverage_family"`
}

// Return fresh values so callers cannot change shared capability metadata.
func NativeIntervalNavigations() []NativeIntervalNavigation {
	return []NativeIntervalNavigation{
		{EventMeasureInterval, ViewMeasurements, "measure", "measurement"},
		{EventProcessMeasureInterval, ViewProcessMeasurements, "process_measure", "counter"},
		{EventCPUMeasureInterval, ViewCPUStateFrequency, "measure", "counter"},
	}
}

func NativeIntervalNavigationForEvent(family EventType) (NativeIntervalNavigation, bool) {
	for _, item := range NativeIntervalNavigations() {
		if item.EventType == family {
			return item, true
		}
	}
	return NativeIntervalNavigation{}, false
}

func NativeIntervalNavigationForCoverage(family, table string) (NativeIntervalNavigation, bool) {
	for _, item := range NativeIntervalNavigations() {
		if item.CoverageFamily == family && item.SourceTable == table {
			return item, true
		}
	}
	return NativeIntervalNavigation{}, false
}

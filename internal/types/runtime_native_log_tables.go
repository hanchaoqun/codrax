package types

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Log query rows already have a strict source/record/null-preserving schema.
// Reuse it for display only; log producers do not enter trace causal authority.
func buildNativeLogPresentationTables(input ObservationLedgerInput) []RuntimeMeasurementTable {
	english := input.RequestModel != nil && strings.HasPrefix(strings.ToLower(input.RequestModel.Language), "en")
	text := func(zh, en string) string {
		if english {
			return en
		}
		return zh
	}
	var tables []RuntimeMeasurementTable
	var rosters []nativeLogDisplayRoster
	seen := map[string]int{}
	for _, result := range input.ToolResults {
		if !result.Success || result.ToolName != "log_query" {
			continue
		}
		var coverage *ObservationRecord
		ambiguous := false
		for i := range result.Observations {
			r := &result.Observations[i]
			if NativeRuntimeFactPresentationKind(*r) == "log_query_coverage" {
				if coverage != nil {
					ambiguous = true
				}
				coverage = r
			}
		}
		if coverage == nil || ambiguous {
			continue
		}
		matched, err := strconv.ParseInt(coverage.Value, 10, 64)
		if err != nil || matched < 0 {
			continue
		}
		sources, ok := nativeLogPresentationSources(*coverage)
		if !ok {
			continue
		}
		table := RuntimeMeasurementTable{ObservationID: coverage.ID, View: RuntimeMeasurementMembers,
			Label: text("日志原始记录（按来源及文件行顺序）", "Original log records (source and file-line order)"), DefaultPresentation: true,
			Columns: []string{text("来源 / 原始行", "Source / original lines"), text("格式 / 解析状态", "Format / parse status"), "PID", "TID", "CPU", text("进程名", "Process name"), text("级别 / 标签", "Level / tag"), text("原始时间", "Original time"), text("记录内容", "Recorded text")},
			Notes:   []string{text("各行身份只来自该行；未知字段不沿用邻行。原始时间照录；文件行顺序不等于跨来源时间顺序，未提供已验证的跨来源时钟校准时不能跨来源排序或计算时差。共同标识及相邻事件不证明因果关系。", "Identity belongs to each record; absent fields never inherit from adjacent rows. Timestamps remain verbatim. File order is not cross-source time order: without verified clock calibration, do not order or subtract timestamps across sources. Shared identifiers and nearby events do not prove causality.")}}
		for _, source := range sources {
			table.Notes = append(table.Notes, fmt.Sprintf("%s: %s; source_id=%s; generation=%s; complete=%t%s", text("来源", "Source"), nativeLogSourceLabel(source), source.ID, source.Generation, source.Complete, source.Error))
		}
		if input.RequestModel != nil && input.RequestModel.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
			table.DefaultPresentation = false
			table.Notes = append(table.Notes, text("此日志查询没有已验证的连续时间窗口，仅供额外查看，不代替所请求窗口内的事实。", "This log query has no verified continuous time window; it is supplementary and cannot replace facts for the requested window."))
		}
		ids := map[string]bool{}
		valid := true
		for _, r := range result.Observations {
			if r.Predicate != "log_record" {
				continue
			}
			v, ok := decodeNativeLogPresentation(r)
			source, sourceOK := nativeLogPresentationSource(r, sources)
			if !ok || !sourceOK || NativeRuntimeFactPresentationKind(r) != "log_record" ||
				r.SourceRef.QueryScopeID != coverage.SourceRef.QueryScopeID || r.SourceRef.PayloadRef != coverage.SourceRef.PayloadRef || ids[r.ID] {
				valid = false
				continue
			}
			ids[r.ID] = true
			message := v.MessagePreview
			if message == "" {
				message = v.RawTextPreview
			}
			status := v.Kind + " / " + v.Status
			if v.ParseError != "" {
				status += " / " + v.ParseError
			}
			if v.TextFieldsTruncated {
				status += " / " + text("文本为截断预览，完整记录见原始查询载荷", "text preview truncated; full record is in the query payload")
			}
			known := func(s string) string {
				if s == "" {
					return text("未知", "unknown")
				}
				return s
			}
			identity := func(v *int64) string {
				if v == nil {
					return known("")
				}
				return strconv.FormatInt(*v, 10)
			}
			timestamps := text("民用=", "civil=") + known(v.WallTimestamp) + text("; 启动(ns)=", "; boot(ns)=") + known(v.BootTimestampNS) + text("; 时钟=", "; clock=") + known(v.ClockDomain)
			table.Rows = append(table.Rows, []string{fmt.Sprintf("%s / L%d–%d", nativeLogSourceLabel(source), v.FirstLine, v.LastLine), status,
				identity(v.PID), identity(v.TID), identity(v.CPU), known(v.Comm), known(v.Level) + " / " + known(v.Tag), timestamps, message})
		}
		if !valid || int64(len(table.Rows)) > matched {
			continue
		}
		table.Notes = append(table.Notes, fmt.Sprintf(text("本次查询匹配%d条；此页保留%d条，未在本表展示%d条。查询命中数不证明整个采集完整；没有记录不等于0。", "Query matches: %d; rows retained on this page: %d; matching rows not shown: %d. Query coverage does not prove capture completeness; absence of a record is not a measured zero."), matched, len(table.Rows), matched-int64(len(table.Rows))))
		for _, note := range coverage.RichNotes {
			if filters, ok := strings.CutPrefix(note, "query_filters="); ok {
				table.Notes = append(table.Notes, text("本次查询筛选：", "Query filters: ")+filters)
			}
			if complete, ok := strings.CutPrefix(note, "source_reads_complete="); ok {
				table.Notes = append(table.Notes, text("所选来源读取全部成功：", "All selected source reads succeeded: ")+complete)
			}
		}
		table.Notes = append(table.Notes, text("原始查询载荷：", "Original query payload: ")+coverage.SourceRef.PayloadRef)
		roster := nativeLogDefaultRoster(*coverage, sources, result.Observations, matched)
		table.nativeLogDisplayPopulation = roster.valid && roster.untruncated && table.DefaultPresentation
		encoded, _ := json.Marshal(table)
		if prior, exists := seen[string(encoded)]; exists {
			// Equal rendered previews are not equal physical records. Retain
			// the existing display, but do not let conflicting hidden fields
			// justify suppression or display-completeness qualification.
			if !reflect.DeepEqual(rosters[prior], roster) {
				rosters[prior].valid = false
				tables[prior].nativeLogDisplayPopulation = false
			}
		} else {
			seen[string(encoded)] = len(tables)
			tables = append(tables, table)
			rosters = append(rosters, roster)
		}
	}
	suppressNativeLogSubsetDefaults(tables, rosters)
	return tables
}

// These temporary display receipts never become a member-completion or causal
// credential. Keep every original table selectable; suppress only a redundant
// default when another complete query carries the exact same source records.
type nativeLogDisplayRoster struct {
	filters     string
	sources     map[string]nativeLogSourceReceipt
	rows        map[string]string
	valid       bool
	untruncated bool
}

func nativeLogDefaultRoster(coverage ObservationRecord, sources []nativeLogSourceReceipt, observations []ObservationRecord, matched int64) nativeLogDisplayRoster {
	roster := nativeLogDisplayRoster{sources: map[string]nativeLogSourceReceipt{}, rows: map[string]string{}, untruncated: true}
	if matched <= 0 {
		return roster // Empty results carry useful status, not a substitute roster.
	}
	var rawFilters string
	filterCount, completeCount := 0, 0
	for _, note := range coverage.RichNotes {
		if value, ok := strings.CutPrefix(note, "query_filters="); ok {
			rawFilters, filterCount = value, filterCount+1
		}
		if value, ok := strings.CutPrefix(note, "source_reads_complete="); ok {
			if value != "true" {
				return roster
			}
			completeCount++
		}
	}
	var filters map[string]json.RawMessage
	if filterCount != 1 || completeCount != 1 || !runtimeMeasurementUniqueKeys(rawFilters) || json.Unmarshal([]byte(rawFilters), &filters) != nil || filters == nil {
		return roster
	}
	// Source selection alone may differ; all other current/future query fields,
	// including pagination, remain exact. No text or name matching is involved.
	delete(filters, "source_ids")
	encoded, _ := json.Marshal(struct {
		Filters map[string]json.RawMessage
		Scope   ObservationSourceRef
	}{filters, nativeLogDisplaySourceScope(coverage.SourceRef)})
	roster.filters = string(encoded)
	for _, source := range sources {
		if !source.Complete || source.Error != "" || source.Generation == "" {
			return roster
		}
		roster.sources[source.ID] = source
	}
	for _, record := range observations {
		if record.Predicate != "log_record" {
			continue
		}
		value, ok := decodeNativeLogPresentation(record)
		source, bound := nativeLogPresentationSource(record, sources)
		if !ok || !bound {
			return roster
		}
		roster.untruncated = roster.untruncated && !value.TextFieldsTruncated
		keyJSON, _ := json.Marshal([]string{source.ID, source.Path, source.Generation, value.RecordID})
		key := string(keyJSON)
		if _, exists := roster.rows[key]; exists {
			return roster
		}
		span := record.Span
		span.JSONPointer = "" // Result-array position is not physical identity.
		rowJSON, _ := json.Marshal(struct {
			Value nativeLogPresentation
			Scope ObservationSourceRef
			Span  ObservationSpan
		}{value, nativeLogDisplaySourceScope(record.SourceRef), span})
		roster.rows[key] = string(rowJSON)
	}
	roster.valid = int64(len(roster.rows)) == matched
	return roster
}

func nativeLogDisplaySourceScope(source ObservationSourceRef) ObservationSourceRef {
	source.RawRef, source.PayloadRef, source.QueryScopeID = "", "", ""
	if source.ArtifactKind == "log_query_result" {
		source.ArtifactID = "" // Query identity is intentionally not record identity.
	}
	return source
}

func suppressNativeLogSubsetDefaults(tables []RuntimeMeasurementTable, rosters []nativeLogDisplayRoster) {
	counts := map[string]int{}
	for _, table := range tables {
		counts[table.ObservationID]++
	}
	for i, subset := range rosters {
		if !tables[i].DefaultPresentation || !subset.valid || counts[tables[i].ObservationID] != 1 {
			continue
		}
		for j, superset := range rosters {
			if i == j || !superset.valid || counts[tables[j].ObservationID] != 1 || subset.filters != superset.filters ||
				len(subset.rows) > len(superset.rows) || len(subset.sources) > len(superset.sources) ||
				len(subset.rows) == len(superset.rows) && len(subset.sources) == len(superset.sources) {
				continue
			}
			contained := true
			for id, source := range subset.sources {
				if superset.sources[id] != source {
					contained = false
				}
			}
			for id, row := range subset.rows {
				if superset.rows[id] != row {
					contained = false
				}
			}
			if contained {
				tables[i].DefaultPresentation = false
				break
			}
		}
	}
}

type nativeLogSourceReceipt struct {
	ID         string `json:"source_id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Generation string `json:"generation"`
	Complete   bool   `json:"complete"`
	Error      string `json:"error"`
}

func nativeLogPresentationSources(coverage ObservationRecord) ([]nativeLogSourceReceipt, bool) {
	var raw string
	count := 0
	for _, note := range coverage.RichNotes {
		if value, ok := strings.CutPrefix(note, "source_generations="); ok {
			raw = value
			count++
		}
	}
	var sources []nativeLogSourceReceipt
	if count != 1 || !runtimeMeasurementUniqueKeys(raw) || json.Unmarshal([]byte(raw), &sources) != nil {
		return nil, false
	}
	seen := map[string]bool{}
	for _, s := range sources {
		if s.ID == "" || seen[s.ID] || s.Complete && s.Generation == "" {
			return nil, false
		}
		seen[s.ID] = true
	}
	return sources, len(sources) > 0
}

func nativeLogPresentationSource(r ObservationRecord, sources []nativeLogSourceReceipt) (nativeLogSourceReceipt, bool) {
	var generation string
	count := 0
	for _, note := range r.RichNotes {
		if value, ok := strings.CutPrefix(note, "source_generation="); ok {
			generation = value
			count++
		}
	}
	for _, source := range sources {
		if count == 1 && source.Complete && source.ID == r.SourceRef.ArtifactID && source.Path == r.SourceRef.Path && generation == source.Generation {
			return source, true
		}
	}
	return nativeLogSourceReceipt{}, false
}

func nativeLogSourceLabel(source nativeLogSourceReceipt) string {
	if source.Path != "" {
		return source.Path
	}
	if source.Name != "" {
		return source.Name + " (" + source.ID + ")"
	}
	return source.ID
}

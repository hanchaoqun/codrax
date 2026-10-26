package types

import (
	"fmt"
	"path/filepath"
	"strings"
)

const RuntimeMeasurementSourceTitleTeaching = "For a published source-owned native selector, omit title: the system supplies the source heading and exact original-source note on table, summary and edgeless fact diagram. Accepted user comparison roles and actual query-side roles are distinct; conflicting roles remain unspecified. Keep explanations in separate ordinary blocks. Other block titles are unchanged."

// Endpoint semantics belong to the validated native producer, not the request
// window, member envelope or display notes. Zero remains unknown for legacy
// producers. This private display metadata never grants query/causal authority.
type runtimeMeasurementQueryEnd uint8

const (
	runtimeMeasurementQueryEndUnknown runtimeMeasurementQueryEnd = iota
	runtimeMeasurementQueryEndExcluded
	runtimeMeasurementQueryEndIncluded
)

func runtimeMeasurementNativeQueryEnd(inclusive bool) runtimeMeasurementQueryEnd {
	if inclusive {
		return runtimeMeasurementQueryEndIncluded
	}
	return runtimeMeasurementQueryEndExcluded
}

type runtimeMeasurementSourceWindow struct {
	start, end float64
	known      bool
	endBound   runtimeMeasurementQueryEnd
}

func newRuntimeMeasurementSourceWindow(source ObservationSourceRef, endBound runtimeMeasurementQueryEnd) runtimeMeasurementSourceWindow {
	start, end, known := TraceObservationContinuousQueryWindow(source)
	if !known {
		return runtimeMeasurementSourceWindow{}
	}
	return runtimeMeasurementSourceWindow{start: start, end: end, known: true, endBound: endBound}
}

func (w runtimeMeasurementSourceWindow) format(language string) string {
	if !w.known {
		return ""
	}
	switch w.endBound {
	case runtimeMeasurementQueryEndIncluded:
		return fmt.Sprintf("[%g,%g] s", w.start, w.end)
	case runtimeMeasurementQueryEndExcluded:
		return fmt.Sprintf("[%g,%g) s", w.start, w.end)
	default:
		if strings.HasPrefix(strings.ToLower(language), "zh") {
			return fmt.Sprintf("%g–%g s（端点包含性未知）", w.start, w.end)
		}
		return fmt.Sprintf("%g–%g s (endpoint inclusion unavailable)", w.start, w.end)
	}
}

// All fields stay private and immutable after publication. The current source
// witness is checked by the existing compiler before constructing this value;
// the table digest protects that exact final display, not physical semantics.
type runtimeMeasurementSourcePresentation struct {
	original, queryPath, generation string
	role, roleBasis                 string
	window                          runtimeMeasurementSourceWindow
	selection                       string
	digest                          string
}

// RuntimeMeasurementSourceTitleBasis is a copy for prompt display only. It
// cannot be consumed as a source receipt or used to bind a model selector.
// QueryWindow remains query-local when the other literal fields share storage.
type RuntimeMeasurementSourceTitleBasis struct {
	OriginalSource     string `json:"original_source,omitempty"`
	QuerySource        string `json:"query_source,omitempty"`
	QuerySourceVersion string `json:"query_source_version,omitempty"`
	Role               string `json:"role,omitempty"`
	RoleBasis          string `json:"role_basis,omitempty"`
	QueryWindow        string `json:"query_window,omitempty"`
	QuerySelection     string `json:"query_selection,omitempty"`
}

func (t RuntimeMeasurementTable) NativeSourceTitleBasis() (RuntimeMeasurementSourceTitleBasis, bool) {
	if _, _, valid := t.NativeSourcePresentation(""); !valid {
		return RuntimeMeasurementSourceTitleBasis{}, false
	}
	p := t.nativeSourcePresentation
	return RuntimeMeasurementSourceTitleBasis{
		OriginalSource: p.original, QuerySource: p.queryPath, QuerySourceVersion: p.generation,
		Role: p.role, RoleBasis: p.roleBasis, QueryWindow: p.window.format(""), QuerySelection: p.selection,
	}, true
}

func runtimeMeasurementQuerySourcePresentation(input ObservationLedgerInput, source ObservationSourceRef, original string, replay TraceQueryWindowReplayRef) *runtimeMeasurementSourcePresentation {
	if original == "" {
		return nil
	}
	ref := replay.source
	if replay.preparedSource != nil {
		ref = replay.preparedSource.source
	} else if replay.nativeNavigation != nil {
		ref = replay.nativeNavigation.source
	}
	if ref.generation == nil || ref.path != source.Path {
		return nil
	}
	p := &runtimeMeasurementSourcePresentation{
		original:  RuntimeArtifactComparisonLocalPath(original, input.RepoRoot),
		queryPath: source.Path, generation: ref.identity.CacheToken(),
	}
	p.window = newRuntimeMeasurementSourceWindow(source, runtimeMeasurementQueryEndUnknown)
	if input.RequestModel != nil && input.RequestModel.RuntimeArtifactScopeProfile != nil {
		comparison := input.RequestModel.RuntimeArtifactScopeProfile.Comparison
		if comparison.Valid() {
			for i, side := range comparison.Sides() {
				if RuntimeArtifactComparisonPathEqual(side.Path, original, input.RepoRoot) {
					if p.role != "" {
						p.role, p.roleBasis = "", "ambiguous"
						break
					}
					p.role, p.roleBasis = []string{"baseline", "current"}[i], "accepted_user_comparison"
				}
			}
		}
	}
	return p
}

// Coalescing retains the existing full-publication conflict rule. Source
// presentation is joined only after that succeeds, and never from JSON, model
// titles, file-name words or approximate equal values. A current identical
// publication can supply a missing memo presentation, as with existing private
// display witnesses. Role conflicts only remove a role, never a source.
func qualifyRuntimeMeasurementSourcePresentations(c *RuntimeMeasurementContract, publications []RuntimeMeasurementPublication) {
	type identity struct{ original, queryPath, generation string }
	sources := map[string]*runtimeMeasurementSourcePresentation{}
	conflicts := map[string]bool{}
	roles := map[identity]map[string]bool{}
	for _, publication := range publications {
		p := publication.sourcePresentation
		if p == nil {
			continue
		}
		// The decoder has independently checked the producer's publication
		// seal. Preserve its endpoint semantics after current-source binding,
		// without changing the query coordinates or requested display scope.
		cp := *p
		cp.window = newRuntimeMeasurementSourceWindow(publication.Source, publication.queryWindowEnd)
		p = &cp
		key := publication.ObservationID
		if old := sources[key]; old != nil && (old.original != p.original || old.queryPath != p.queryPath || old.generation != p.generation || old.window != p.window) {
			conflicts[key] = true
		} else if old == nil {
			cp := *p
			sources[key] = &cp
		}
		owner := identity{p.original, p.queryPath, p.generation}
		if roles[owner] == nil {
			roles[owner] = map[string]bool{}
		}
		if p.roleBasis != "" {
			roles[owner][p.roleBasis+"\x00"+p.role] = true
		}
	}
	for i := range c.Tables {
		table := &c.Tables[i]
		p := sources[table.ObservationID]
		if p == nil || conflicts[table.ObservationID] {
			continue
		}
		cp := *p
		if table.nativeEventData && table.EventData != nil {
			// Point lookup selectors are not continuous statistical windows.
			// Use only the producer's executed scan, not the observed envelope
			// or a request window whose half-open point projection is separate.
			cp.window = runtimeMeasurementSourceWindow{}
			cp.selection = runtimeEventQuerySelection(table.EventData.Coverage.ScanScope)
		}
		cp.role, cp.roleBasis = "", ""
		candidates := roles[identity{p.original, p.queryPath, p.generation}]
		// Accepted-user roles have already passed the original-source contract.
		// Actual query roles cannot override them or create a user obligation.
		userRoles := map[string]bool{}
		queryRoles := map[string]bool{}
		ambiguous := false
		for candidate := range candidates {
			basis, role, _ := strings.Cut(candidate, "\x00")
			switch basis {
			case "accepted_user_comparison":
				userRoles[role] = true
			case "query_side":
				queryRoles[role] = true
			case "ambiguous":
				ambiguous = true
			}
		}
		if !ambiguous && len(userRoles) == 1 {
			for role := range userRoles {
				cp.role, cp.roleBasis = role, "accepted_user_comparison"
			}
		} else if !ambiguous && len(userRoles) == 0 && len(queryRoles) == 1 {
			for role := range queryRoles {
				cp.role, cp.roleBasis = role, "query_side"
			}
		}
		cp.digest = runtimeSamplingDigest(*table)
		table.nativeSourcePresentation = &cp
	}
}

// NativeSourcePresentation returns a short source heading and literal source
// notes, never a relation, measured unit or completion claim. The full source
// remains in notes, not as a diagram actor or a long internal identifier.
func (t RuntimeMeasurementTable) NativeSourcePresentation(language string) (string, []string, bool) {
	p := t.nativeSourcePresentation
	if p == nil || p.digest == "" || p.digest != runtimeSamplingDigest(t) || !t.IsValid() {
		return "", nil, false
	}
	zh := strings.HasPrefix(strings.ToLower(language), "zh")
	label := t.Label
	if label == "" {
		label = "Native facts"
		if zh {
			label = "原生事实"
		}
	}
	if p.original == "" {
		return label, nil, true // Current producer-owned multi-source status.
	}
	name := filepath.Base(p.original)
	role := ""
	switch p.role {
	case "baseline":
		role = "Baseline"
		if zh {
			role = "基线侧"
		}
	case "current":
		role = "Current"
		if zh {
			role = "当前侧"
		}
	}
	if role != "" {
		if p.roleBasis == "query_side" {
			if zh {
				role = "本次查询" + role
			} else {
				role = "Query " + role
			}
		}
		name = role + " · " + name
	}
	note := "Original source: " + p.original
	if zh {
		note = "原始来源：" + p.original
	}
	if p.queryPath != "" && p.queryPath != p.original {
		if zh {
			note += "；查询来源：" + p.queryPath
		} else {
			note += "; query source: " + p.queryPath
		}
	}
	if p.generation != "" {
		if zh {
			note += "；查询来源版本：" + p.generation
		} else {
			note += "; query source version: " + p.generation
		}
	}
	if window := p.window.format(language); window != "" {
		if zh {
			note += "；实际查询窗：" + window
		} else {
			note += "; actual query window: " + window
		}
	}
	if p.selection != "" {
		if zh {
			note += "；查询筛选范围：" + p.selection
		} else {
			note += "; query selection: " + p.selection
		}
	}
	return name + " · " + label, []string{note}, true
}

// Display-only disclosure. Applied flags preserve inclusive/one-sided engine
// semantics, including explicit zero and line precedence. This text is never
// parsed to admit evidence, derive rates, or expand request/causal authority.
func runtimeEventQuerySelection(scope *TraceEventSearchScanScope) string {
	if scope == nil {
		return "selection unavailable (legacy producer)"
	}
	selection := "no time or line filter"
	switch {
	case scope.LineStart > 0 && scope.LineEnd > 0:
		selection = fmt.Sprintf("lines [%d,%d] (inclusive; time arguments not applied)", scope.LineStart, scope.LineEnd)
	case scope.LineStart > 0:
		selection = fmt.Sprintf("lines >= %d (time arguments not applied)", scope.LineStart)
	case scope.LineEnd > 0:
		selection = fmt.Sprintf("lines <= %d (time arguments not applied)", scope.LineEnd)
	case scope.TimeStartApplied && scope.TimeEndApplied:
		selection = fmt.Sprintf("time [%g,%g] s (both endpoints included)", scope.TimeStart, scope.TimeEnd)
	case scope.TimeStartApplied:
		selection = fmt.Sprintf("time >= %g s", scope.TimeStart)
	case scope.TimeEndApplied:
		selection = fmt.Sprintf("time <= %g s", scope.TimeEnd)
	}
	if scope.IndexRestricted {
		selection += "; restricted to indexed candidates"
	}
	return selection
}

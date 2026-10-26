package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

type traceCatalogRequestPlanParams struct {
	View string `json:"view"`
}

// This is a bounded, typed proposal, not observations or an execution receipt.
// Physical source coordinates are supplied only by normal catalog discovery.
type traceCatalogRequestQuery struct {
	View        string  `json:"view"`
	PID         int     `json:"pid,omitempty"`
	Thread      string  `json:"thread,omitempty"`
	TargetScope string  `json:"target_scope"`
	TimeStart   float64 `json:"time_start"`
	TimeEnd     float64 `json:"time_end"`
}

type traceCatalogRequestPlanProposal struct {
	Status          string                              `json:"status"`
	NavigationOnly  bool                                `json:"navigation_only"`
	MissingFields   []string                            `json:"missing_fields,omitempty"`
	Queries         []traceCatalogRequestQuery          `json:"queries,omitempty"`
	Representatives *tracecatalog.RepresentativeOptions `json:"representatives,omitempty"`
}

func traceCatalogRequestPlan(rm *types.RequestModel, view string) (traceCatalogRequestPlanProposal, error) {
	p := traceCatalogRequestPlanProposal{Status: "pending", NavigationOnly: true}
	view = strings.TrimSpace(view)
	if view != "" {
		if err := tracequery.ValidateViewName(view); err != nil {
			return p, err
		}
	} else {
		p.MissingFields = append(p.MissingFields, "request_plan.view")
	}
	if rm == nil || !rm.RuntimeArtifactWorkProfile.Valid() {
		p.MissingFields = append(p.MissingFields, "runtime_artifact_work_profile")
		return p, nil
	}
	work := rm.RuntimeArtifactWorkProfile
	if work.PerSpecificationLimit > tracecatalog.MaxRepresentativesPerCluster {
		p.MissingFields = append(p.MissingFields, "per_specification_limit_capacity")
	}
	start, end, windowOK := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow()
	if rm.RuntimeArtifactScopeProfile != nil && rm.RuntimeArtifactScopeProfile.Comparison != nil {
		p.MissingFields = append(p.MissingFields, "unambiguous_common_window_without_comparison")
	} else if !windowOK {
		p.MissingFields = append(p.MissingFields, "single_common_explicit_time_window")
	} else if _, ok := traceCatalogExactNS(start); !ok {
		p.MissingFields = append(p.MissingFields, "exact_nanosecond_window")
	} else if _, ok := traceCatalogExactNS(end); !ok {
		p.MissingFields = append(p.MissingFields, "exact_nanosecond_window")
	}
	if !rm.RuntimeTargetProfile.NamedTarget() {
		p.MissingFields = append(p.MissingFields, "named_user_targets")
		return p, nil
	}
	var queries []traceCatalogRequestQuery
	identities := map[string]string{}
	invalidTarget := false
	for _, target := range rm.RuntimeTargets {
		if strings.TrimSpace(target.Source) != "user_explicit" {
			continue // A tool's exploration cursor is never a task subject.
		}
		query := traceCatalogRequestQuery{View: tracequery.CanonicalViewName(view), TimeStart: start, TimeEnd: end}
		kind := types.NormalizeRuntimeTargetKind(target.Kind)
		switch kind {
		case types.RuntimeTargetKindThread:
			pid, name, ok := threadidentity.QuerySelector(target.PID, target.Thread)
			if !ok || pid == 0 && strings.TrimSpace(name) == "" {
				invalidTarget = true
				continue
			}
			query.PID, query.Thread, query.TargetScope = pid, name, "thread"
		case types.RuntimeTargetKindProcess:
			if target.PID <= 0 || target.PID > types.RuntimeTargetMaxPID {
				invalidTarget = true
				continue
			}
			if view != "" && !traceQueryViewSupportsProcessScope(view) {
				p.MissingFields = append(p.MissingFields, "unsupported_target_scope_for_view")
				return p, nil
			}
			query.PID, query.TargetScope = target.PID, "process"
		default:
			invalidTarget = true
			continue
		}
		identity := fmt.Sprintf("%s:%d", query.TargetScope, query.PID)
		if query.PID == 0 {
			identity += ":" + query.Thread
		}
		if oldName, exists := identities[identity]; exists {
			if oldName != query.Thread {
				invalidTarget = true // Do not elect one of conflicting typed names.
			}
			continue
		}
		identities[identity] = query.Thread
		queries = append(queries, query)
	}
	if invalidTarget {
		p.MissingFields = append(p.MissingFields, "valid_nonconflicting_user_targets")
	}
	if len(queries) == 0 {
		p.MissingFields = append(p.MissingFields, "named_user_targets")
	} else if len(queries) > 16 {
		p.MissingFields = append(p.MissingFields, "query_specification_capacity")
	}
	if len(p.MissingFields) != 0 {
		return p, nil // Never publish a prefix, default subject, or partial plan.
	}
	p.Status, p.Queries = "ready", queries
	if work.Mode == types.RuntimeArtifactWorkRepresentative {
		p.Representatives = &tracecatalog.RepresentativeOptions{PerCluster: work.PerSpecificationLimit, MaxClusters: len(queries), Priority: "unexecuted_first"}
	}
	return p, nil
}

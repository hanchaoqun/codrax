package dataworkflow

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type CoverageLayer string

const (
	CoverageLayerWorkflow     CoverageLayer = "workflow"
	CoverageLayerCurrentBatch CoverageLayer = "current_batch"
	CoverageLayerLastBatch    CoverageLayer = "last_batch"
)

type CoverageContractView struct {
	Layer                      CoverageLayer          `json:"layer,omitempty"`
	RequiredMaterials          []MaterialContractView `json:"required_materials,omitempty"`
	OptionalMaterials          []MaterialContractView `json:"optional_materials,omitempty"`
	RequiredPaths              []string               `json:"required_paths,omitempty"`
	RequiredRunnerInputPaths   []string               `json:"required_runner_input_paths,omitempty"`
	ValidationRuleCount        int                    `json:"validation_rule_count,omitempty"`
	DecisionRecordsRequired    bool                   `json:"decision_records_required,omitempty"`
	RuleCoverageRequired       bool                   `json:"rule_coverage_required,omitempty"`
	ContributionLedgerRequired bool                   `json:"contribution_ledger_required,omitempty"`
	EntityResolutionRequired   bool                   `json:"entity_resolution_required,omitempty"`
	ReconcileRequired          bool                   `json:"reconcile_required,omitempty"`
}

type MaterialContractView struct {
	ID         string `json:"id,omitempty"`
	Path       string `json:"path,omitempty"`
	RunnerPath string `json:"runner_path,omitempty"`
	UseMode    string `json:"use_mode,omitempty"`
	Required   bool   `json:"required,omitempty"`
	Purpose    string `json:"purpose,omitempty"`
}

func CoverageContractViewFor(layer CoverageLayer, contract dataquery.CoverageContract) CoverageContractView {
	view := CoverageContractView{
		Layer:                      layer,
		RequiredPaths:              contract.RequiredPaths(),
		RequiredRunnerInputPaths:   contract.RequiredRunnerInputPaths(),
		ValidationRuleCount:        len(contract.ValidationRules),
		DecisionRecordsRequired:    contract.DecisionRecordsRequired,
		RuleCoverageRequired:       contract.RuleCoverageRequired,
		ContributionLedgerRequired: contract.ContributionLedgerRequired,
		EntityResolutionRequired:   contract.EntityResolutionRequired,
		ReconcileRequired:          contract.ReconcileRequired,
	}
	view.RequiredMaterials = materialContractViews(contract.RequiredMaterials)
	view.OptionalMaterials = materialContractViews(contract.OptionalMaterials)
	return view
}

func materialContractViews(materials []dataquery.CoverageMaterial) []MaterialContractView {
	out := make([]MaterialContractView, 0, len(materials))
	for _, material := range materials {
		path := cleanContractPath(material.Path)
		runnerPath := coverageMaterialRunnerPath(material)
		if path == "" && runnerPath == "" {
			continue
		}
		out = append(out, MaterialContractView{
			ID:         strings.TrimSpace(material.ID),
			Path:       path,
			RunnerPath: runnerPath,
			UseMode:    string(material.UsageMode),
			Required:   material.Required,
			Purpose:    strings.TrimSpace(material.Purpose),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].RunnerPath < out[j].RunnerPath
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func coverageMaterialRunnerPath(material dataquery.CoverageMaterial) string {
	switch material.UsageMode {
	case dataquery.MaterialUseScriptConsumed:
		return cleanContractPath(material.Path)
	case dataquery.MaterialUseTextEvidenceConsumed:
		return cleanContractPath(material.TextEvidencePath)
	default:
		return ""
	}
}

func cleanContractPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	for strings.Contains(value, "//") {
		value = strings.ReplaceAll(value, "//", "/")
	}
	return value
}

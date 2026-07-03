package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

type violationPolicyCoverageRow struct {
	Kind                 types.ViolationKind
	DefaultSeverity      types.Severity
	SoftByDefault        bool
	Promotable           bool
	RuntimeSoftDefault   bool
	DefaultRetryEligible bool
	StrictRetryEligible  bool
	FallbackLocus        types.RepairLocus
	FallbackTarget       FallbackTarget
	Layer                string
	CaveatFamilyID       string
	FixableByAgents      []types.AgentName
	SchemaPreview        bool
	StrictSeverity       types.Severity
	RepairPhase          types.RepairPhase
	AutoRepair           types.AutoRepairLane
}

func buildViolationPolicyCoverageReport() []violationPolicyCoverageRow {
	rows := make([]violationPolicyCoverageRow, 0, len(types.AllViolationKinds()))
	activeSoft := defaultSoftKinds()
	policy := DefaultFallbackPolicy()
	for _, kind := range types.AllViolationKinds() {
		runtimeSoftDefault, hasSoftDefault := activeSoft[kind]
		if !hasSoftDefault {
			runtimeSoftDefault = !types.ViolationProfileFor(kind, false).RetryEligible
		}
		spec, ok := types.ViolKindSpecFor(kind)
		if !ok {
			rows = append(rows, violationPolicyCoverageRow{
				Kind:                 kind,
				RuntimeSoftDefault:   runtimeSoftDefault,
				DefaultRetryEligible: types.ViolationProfileFor(kind, false).RetryEligible,
				StrictRetryEligible:  types.ViolationProfileFor(kind, true).RetryEligible,
				FallbackTarget:       policy[kind],
			})
			continue
		}
		agents := append([]types.AgentName(nil), spec.FixableByAgents...)
		rows = append(rows, violationPolicyCoverageRow{
			Kind:                 kind,
			DefaultSeverity:      spec.DefaultSeverity,
			SoftByDefault:        spec.SoftByDefault,
			Promotable:           spec.Promotable,
			RuntimeSoftDefault:   runtimeSoftDefault,
			DefaultRetryEligible: types.ViolationProfileFor(kind, false).RetryEligible,
			StrictRetryEligible:  types.ViolationProfileFor(kind, true).RetryEligible,
			FallbackLocus:        spec.FallbackLocus,
			FallbackTarget:       policy[kind],
			Layer:                spec.Layer,
			CaveatFamilyID:       spec.CaveatFamilyID,
			FixableByAgents:      agents,
			SchemaPreview:        spec.SchemaDescriptionFragment != "",
			StrictSeverity:       spec.EffectiveStrictSeverity(),
			RepairPhase:          spec.EffectiveRepairPhase(),
			AutoRepair:           spec.AutoRepair,
		})
	}
	return rows
}

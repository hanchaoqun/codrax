package dataworkflow

import (
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestRelationActionScaffoldsUseArtifactSchemaProjection(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:        "base",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"base.json"},
			JSONShape: "array(len=2,item=object(keys=id,name))",
			Fields:    []string{"id", "name"},
		},
		{
			ID:        "lookup",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"lookup.json"},
			JSONShape: "array(len=2,item=object(keys=id,label))",
			Fields:    []string{"id", "label"},
		},
		{
			ID:        "diag",
			NodeClass: ArtifactNodeClassDiagnosticChild,
			Aliases:   []string{"diag.json"},
			Fields:    []string{"id", "debug"},
		},
	}

	scaffolds := RelationActionScaffolds(projections, []string{
		string(dataquery.DataActionEnrichRecords),
		string(dataquery.DataActionJoinRecords),
	}, 8)
	var kinds []string
	for _, scaffold := range scaffolds {
		kinds = append(kinds, scaffold.Kind)
		for _, input := range scaffold.InputPaths {
			if input == "diag.json" {
				t.Fatalf("diagnostic artifact used as relation input: %+v", scaffold)
			}
		}
	}
	if !slices.Contains(kinds, string(dataquery.DataActionEnrichRecords)) {
		t.Fatalf("scaffolds=%+v, want enrich_records scaffold", scaffolds)
	}
	if !slices.Contains(kinds, string(dataquery.DataActionJoinRecords)) {
		t.Fatalf("scaffolds=%+v, want join_records scaffold", scaffolds)
	}
	for _, scaffold := range scaffolds {
		switch scaffold.Kind {
		case string(dataquery.DataActionJoinRecords):
			if !scaffold.Executable {
				t.Fatalf("join scaffold=%+v, want executable concrete scaffold", scaffold)
			}
		case string(dataquery.DataActionEnrichRecords):
			if scaffold.Executable {
				t.Fatalf("enrich scaffold=%+v, want prompt-only scaffold until target field is concrete", scaffold)
			}
		}
	}
}

func TestConservativeContributionLedgerScaffoldBuildsExecutableAuditCount(t *testing.T) {
	scaffolds := ConservativeContributionLedgerScaffolds([]ArtifactSchemaProjection{{
		ID:        "eligible_records",
		Kind:      string(dataquery.DataActionFilterRecords),
		NodeClass: ArtifactNodeClassRecord,
		Aliases:   []string{"eligible_records.json"},
		JSONShape: "array(len=2,item=object(keys=id,status))",
		Fields:    []string{"id", "status"},
	}}, 2)
	if len(scaffolds) != 1 {
		t.Fatalf("scaffolds=%+v, want one conservative contribution scaffold", scaffolds)
	}
	action, ok := ConcreteActionFromScaffold(scaffolds[0])
	if !ok {
		t.Fatalf("ConcreteActionFromScaffold returned ok=false for %+v", scaffolds[0])
	}
	if action.Kind != dataquery.DataActionComputeContribs ||
		!slices.Equal(action.InputPaths, []string{"eligible_records.json"}) ||
		action.Params["operation"] != "count" ||
		action.Params["role"] != "audit" ||
		action.Params["value_field"] != "" {
		t.Fatalf("action=%+v, want executable audit count contribution action", action)
	}
}

func TestBuildActionScaffoldsBuildsGenericTypedActions(t *testing.T) {
	scaffolds := BuildActionScaffolds(ActionScaffoldBuildInput{
		State: WorkflowStateView{
			MaterialCoverageSufficient: true,
			ContributionLedgerRequired: true,
			EntityStageMaterialized:    true,
			NextStage:                  StagePrepareContributionInputs,
			AllowedNextActions: []string{
				string(dataquery.DataActionDeriveFields),
				string(dataquery.DataActionExtractFields),
				string(dataquery.DataActionFilterRecords),
				string(dataquery.DataActionValueDistribution),
				string(dataquery.DataActionComputeContribs),
			},
		},
		Artifacts: []ArtifactSchemaProjection{{
			ID:        "records",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"records.json"},
			JSONShape: "array(len=2,item=object(keys=id,text,status,amount))",
			Fields:    []string{"id", "text", "status", "amount"},
		}},
		Limit: 10,
	})
	var kinds []string
	for _, scaffold := range scaffolds {
		kinds = append(kinds, scaffold.Kind)
		if scaffold.InputPath == "" && len(scaffold.InputPaths) == 0 {
			t.Fatalf("scaffold=%+v, want concrete input path hints", scaffold)
		}
	}
	for _, want := range []string{
		string(dataquery.DataActionDeriveFields),
		string(dataquery.DataActionExtractFields),
		string(dataquery.DataActionFilterRecords),
		string(dataquery.DataActionValueDistribution),
		string(dataquery.DataActionComputeContribs),
	} {
		if !slices.Contains(kinds, want) {
			t.Fatalf("scaffold kinds=%v, want %s", kinds, want)
		}
	}
}

func TestBuildActionScaffoldsSkipsCompletedStage(t *testing.T) {
	got := BuildActionScaffolds(ActionScaffoldBuildInput{
		State: WorkflowStateView{
			NextStage:          StageComplete,
			AllowedNextActions: []string{string(dataquery.DataActionFilterRecords)},
		},
		Artifacts: []ArtifactSchemaProjection{{
			ID:      "records",
			Aliases: []string{"records.json"},
			Fields:  []string{"id"},
		}},
	})
	if len(got) != 0 {
		t.Fatalf("scaffolds=%+v, want none for completed stage", got)
	}
}

func TestBuildActionScaffoldsSkipsUncontractedHistory(t *testing.T) {
	got := BuildActionScaffolds(ActionScaffoldBuildInput{
		State: WorkflowStateView{
			MaterialCoverageSufficient: true,
			NextStage:                  StageComputeContributions,
			AllowedNextActions:         []string{string(dataquery.DataActionFilterRecords)},
		},
		Artifacts: []ArtifactSchemaProjection{{
			ID:        "records",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"records"},
			JSONShape: "array(len=1,item=object(keys=id,status))",
			Fields:    []string{"id", "status"},
		}},
		Limit: 10,
	})
	if len(got) != 0 {
		t.Fatalf("scaffolds=%+v, want none without ledger/output contract or typed violations", got)
	}
}

func TestJoinRecordScaffoldsIgnoreInternalLineageFields(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:        "left",
			Kind:      string(dataquery.DataActionJoinRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"left.json"},
			JSONShape: "array(len=2,item=object(keys=_source,_left_index,value))",
			Fields:    []string{"_source", "_left_index", "value"},
		},
		{
			ID:        "right",
			Kind:      string(dataquery.DataActionJoinRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"right.json"},
			JSONShape: "array(len=2,item=object(keys=_source,_left_index,other))",
			Fields:    []string{"_source", "_left_index", "other"},
		},
	}

	if got := JoinRecordScaffolds(projections, 4); len(got) != 0 {
		t.Fatalf("JoinRecordScaffolds=%+v, want no join on internal lineage fields", got)
	}

	projections[1].Fields = []string{"_source", "_left_index", "value", "other"}
	got := JoinRecordScaffolds(projections, 4)
	if len(got) != 1 {
		t.Fatalf("JoinRecordScaffolds=%+v, want one join on ordinary field", got)
	}
	if strings.Join(got[0].CommonFields, ",") != "value" {
		t.Fatalf("CommonFields=%v, want only ordinary field value", got[0].CommonFields)
	}
}

func TestMappingCandidateScaffoldsBuildConcreteAction(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:        "source",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"source.json"},
			JSONShape: "array(len=2,item=object(keys=raw_label))",
			Fields:    []string{"raw_label"},
		},
		{
			ID:        "reference",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"reference.json"},
			JSONShape: "array(len=2,item=object(keys=code,label))",
			Fields:    []string{"code", "label"},
		},
	}
	scaffolds := MappingCandidateScaffolds(projections, 4)
	if len(scaffolds) == 0 {
		t.Fatal("MappingCandidateScaffolds empty, want candidate scaffold")
	}
	if scaffolds[0].Kind != string(dataquery.DataActionMappingCandidate) || !scaffolds[0].Executable {
		t.Fatalf("scaffold=%+v, want executable mapping_candidate scaffold", scaffolds[0])
	}
	action, ok := ConcreteActionFromScaffold(scaffolds[0])
	if !ok {
		t.Fatalf("ConcreteActionFromScaffold(%+v) ok=false", scaffolds[0])
	}
	if action.Kind != dataquery.DataActionMappingCandidate || len(action.InputPaths) != 2 {
		t.Fatalf("action=%+v, want mapping_candidate over two inputs", action)
	}
	if action.Params["match_mode"] != "exact" {
		t.Fatalf("params=%+v, want default concrete match_mode exact", action.Params)
	}
}

func TestApplyResolutionScaffoldsIgnoreDiagnosticResolutionChildren(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:        "records",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"records.json"},
			JSONShape: "array(len=2,item=object(keys=item_id,name))",
			Fields:    []string{"item_id", "name"},
		},
		{
			ID:                "entity_mappings.json",
			Kind:              string(dataquery.DataActionNormalizeEntities),
			NodeClass:         ArtifactNodeClassArtifact,
			Aliases:           []string{"entity_mappings.json"},
			JSONShape:         "array(len=2,item=object(keys=item_id,source_value,canonical_id,canonical_label))",
			Fields:            []string{"item_id", "source_value", "canonical_id", "canonical_label"},
			SourcePaths:       []string{"records.json", "reference.json"},
			SourceRecordPaths: []string{"records.json"},
			ReferencePaths:    []string{"reference.json"},
		},
		{
			ID:        "entity_mappings.json#entity_resolution_source",
			Kind:      "apply_entity_resolutions/resolution",
			NodeClass: ArtifactNodeClassDiagnosticChild,
			Aliases:   []string{"entity_mappings.json#entity_resolution_source"},
			JSONShape: "array(len=2,item=object(keys=item_id,source_value,canonical_id))",
			Fields:    []string{"item_id", "source_value", "canonical_id"},
		},
	}

	scaffolds := ApplyResolutionScaffolds(projections, 4)
	if len(scaffolds) != 1 {
		t.Fatalf("ApplyResolutionScaffolds=%+v, want exactly one scaffold from real ledger", scaffolds)
	}
	if strings.Join(scaffolds[0].InputPaths, ",") != "records.json,entity_mappings.json" {
		t.Fatalf("InputPaths=%v, want diagnostic child excluded", scaffolds[0].InputPaths)
	}
}

func TestApplyResolutionScaffoldsSkipWorkflowLedgerHandleAndDiagnosticChildren(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:        "records.json#base",
			Kind:      "apply_entity_resolutions/base",
			NodeClass: ArtifactNodeClassDiagnosticChild,
			Aliases:   []string{"records.json#base"},
			JSONShape: "array(len=2,item=object(keys=_source_index,raw_name))",
			Fields:    []string{"_source_index", "raw_name"},
		},
		{
			ID:        "workflow_entity_resolutions",
			Kind:      "workflow_ledger/entity_resolutions",
			NodeClass: ArtifactNodeClassWorkflowLedger,
			Aliases:   []string{"workflow_entity_resolutions"},
			JSONShape: "array(len=2,item=object(keys=item_id,source_value,canonical_id,canonical_label,status))",
			Fields:    []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
		},
		{
			ID:        "records",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"records"},
			JSONShape: "array(len=2,item=object(keys=_source_index,raw_name))",
			Fields:    []string{"_source_index", "raw_name"},
		},
	}

	if got := ApplyResolutionScaffolds(projections, 8); len(got) != 0 {
		t.Fatalf("ApplyResolutionScaffolds=%+v, want no auto scaffold from workflow-wide ledger handle or diagnostic child", got)
	}
}

func TestApplyResolutionScaffoldsSkipAlreadyAppliedLedger(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:          "orders_with_vendor",
			Kind:        string(dataquery.DataActionApplyResolutions),
			NodeClass:   ArtifactNodeClassRecord,
			Aliases:     []string{"orders_with_vendor", "orders_with_vendor.json"},
			JSONShape:   "array(len=2,item=object(keys=_source_index,order_id,vendor_canonical_id,vendor_resolution_status))",
			Fields:      []string{"_source_index", "order_id", "vendor_canonical_id", "vendor_resolution_status"},
			SourcePaths: []string{"orders", "vendor_resolution"},
		},
		{
			ID:                "vendor_resolution",
			Kind:              string(dataquery.DataActionNormalizeEntities),
			NodeClass:         ArtifactNodeClassArtifact,
			Aliases:           []string{"vendor_resolution", "vendor_resolution.json"},
			Fields:            []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
			SourceRecordPaths: []string{"orders"},
			ReferencePaths:    []string{"vendors"},
		},
	}

	for _, scaffold := range ApplyResolutionScaffolds(projections, 4) {
		if len(scaffold.InputPaths) >= 2 &&
			normalizeAccessPath(scaffold.InputPaths[0]) == "orders_with_vendor" &&
			normalizeAccessPath(scaffold.InputPaths[1]) == "vendor_resolution" {
			t.Fatalf("scaffold re-applies already applied ledger: %+v", scaffold)
		}
	}
}

func TestApplyResolutionScaffoldsSkipIncompatibleSourceLineage(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:          "rules_records",
			Kind:        string(dataquery.DataActionExtractRecords),
			NodeClass:   ArtifactNodeClassRecord,
			Aliases:     []string{"rules_records", "rules_records.json"},
			JSONShape:   "array(len=3,item=object(keys=_source_index,text))",
			SourcePaths: []string{"data_rules.md"},
			Fields:      []string{"_source_index", "text"},
		},
		{
			ID:                "vendor_resolution",
			Kind:              string(dataquery.DataActionNormalizeEntities),
			NodeClass:         ArtifactNodeClassArtifact,
			Aliases:           []string{"vendor_resolution", "vendor_resolution.json"},
			Fields:            []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
			SourceRecordPaths: []string{"orders_cleaned"},
			ReferencePaths:    []string{"vendors.csv"},
		},
	}

	if got := ApplyResolutionScaffolds(projections, 4); len(got) != 0 {
		t.Fatalf("ApplyResolutionScaffolds=%+v, want no apply-resolution scaffold for incompatible source lineage", got)
	}
}

func TestApplyResolutionScaffoldsUseStructuredLineageNotSourcePathOrder(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:          "orders_cleaned",
			Kind:        string(dataquery.DataActionExtractRecords),
			NodeClass:   ArtifactNodeClassRecord,
			Aliases:     []string{"orders_cleaned"},
			JSONShape:   "array(len=3,item=object(keys=_source_index,raw_name))",
			SourcePaths: []string{"orders.csv"},
			Fields:      []string{"_source_index", "raw_name"},
		},
		{
			ID:                "vendor_resolution",
			Kind:              string(dataquery.DataActionNormalizeEntities),
			NodeClass:         ArtifactNodeClassArtifact,
			Aliases:           []string{"vendor_resolution"},
			Fields:            []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
			SourcePaths:       []string{"vendors.csv", "orders_cleaned"},
			SourceRecordPaths: []string{"orders_cleaned"},
			ReferencePaths:    []string{"vendors.csv"},
		},
	}

	got := ApplyResolutionScaffolds(projections, 4)
	if len(got) != 1 {
		t.Fatalf("ApplyResolutionScaffolds=%+v, want one scaffold using source_record_paths despite reversed source_paths", got)
	}
	if strings.Join(got[0].InputPaths, ",") != "orders_cleaned,vendor_resolution" {
		t.Fatalf("InputPaths=%v, want base then resolution ledger", got[0].InputPaths)
	}
}

func TestApplyResolutionScaffoldsAddExistingIDReferenceSpec(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{
			ID:          "orders_cleaned",
			Kind:        string(dataquery.DataActionExtractRecords),
			NodeClass:   ArtifactNodeClassRecord,
			Aliases:     []string{"orders_cleaned"},
			JSONShape:   "array(len=3,item=object(keys=_source_index,vendor_id,raw_name))",
			SourcePaths: []string{"orders.csv"},
			Fields:      []string{"_source_index", "vendor_id", "raw_name"},
		},
		{
			ID:        "vendors",
			Kind:      string(dataquery.DataActionExtractRecords),
			NodeClass: ArtifactNodeClassRecord,
			Aliases:   []string{"vendors"},
			JSONShape: "array(len=3,item=object(keys=vendor_id,vendor_name))",
			Fields:    []string{"vendor_id", "vendor_name"},
		},
		{
			ID:                "vendor_resolution",
			Kind:              string(dataquery.DataActionNormalizeEntities),
			NodeClass:         ArtifactNodeClassArtifact,
			Aliases:           []string{"vendor_resolution"},
			Fields:            []string{"item_id", "source_value", "canonical_id", "canonical_label", "status"},
			SourceRecordPaths: []string{"orders_cleaned"},
			ReferencePaths:    []string{"vendors"},
		},
	}

	got := ApplyResolutionScaffolds(projections, 4)
	if len(got) == 0 {
		t.Fatal("ApplyResolutionScaffolds empty, want scaffold")
	}
	spec := got[0].ParamsTemplate["resolution_specs"]
	for _, want := range []string{`"existing_id_field":"vendor_id"`, `"reference_path":"vendors"`, `"reference_id_field":"vendor_id"`, `"reference_label_field":"vendor_name"`} {
		if !strings.Contains(spec, want) {
			t.Fatalf("resolution_specs=%s, want %s", spec, want)
		}
	}
}

func TestPrioritizeConcreteScaffoldsUsesWorkflowStageFacts(t *testing.T) {
	scaffolds := []ActionScaffold{
		{Kind: string(dataquery.DataActionComputeContribs)},
		{Kind: string(dataquery.DataActionJoinRecords)},
		{Kind: string(dataquery.DataActionFilterRecords)},
		{Kind: string(dataquery.DataActionApplyResolutions)},
	}
	facts := StageFacts{
		MaterialCoverageSufficient: true,
		EntityStageMaterialized:    true,
		ContributionLedgerRequired: true,
	}
	got := PrioritizeConcreteScaffolds(scaffolds, facts)
	kinds := []string{got[0].Kind, got[1].Kind, got[2].Kind, got[3].Kind}
	want := []string{
		string(dataquery.DataActionApplyResolutions),
		string(dataquery.DataActionJoinRecords),
		string(dataquery.DataActionFilterRecords),
		string(dataquery.DataActionComputeContribs),
	}
	if !slices.Equal(kinds, want) {
		t.Fatalf("kinds=%v, want %v", kinds, want)
	}

	facts.EntityStageMaterialized = false
	got = PrioritizeConcreteScaffolds(scaffolds, facts)
	kinds = []string{got[0].Kind, got[1].Kind, got[2].Kind, got[3].Kind}
	if !slices.Equal(kinds, []string{
		string(dataquery.DataActionComputeContribs),
		string(dataquery.DataActionJoinRecords),
		string(dataquery.DataActionFilterRecords),
		string(dataquery.DataActionApplyResolutions),
	}) {
		t.Fatalf("kinds=%v, want original order before entity stage materializes", kinds)
	}
}

func TestConcreteActionFromScaffoldAcceptsStructuredSourceFields(t *testing.T) {
	action, ok := ConcreteActionFromScaffold(ActionScaffold{
		Kind:       string(dataquery.DataActionNormalizeEntities),
		Executable: true,
		InputPaths: []string{"records.json", "lookup.json"},
		ParamsTemplate: map[string]string{
			"source_fields":         `["raw_name","alias"]`,
			"reference_name_fields": `["name","aliases"]`,
			"canonical_id_field":    "id",
			"canonical_label_field": "name",
			"match_mode":            "exact|contains|token_set",
		},
	})
	if !ok {
		t.Fatalf("ConcreteActionFromScaffold ok=false, want structured normalize action")
	}
	if action.Kind != dataquery.DataActionNormalizeEntities {
		t.Fatalf("Kind=%q, want normalize_entities", action.Kind)
	}
	if action.Params["source_fields"] != `["raw_name","alias"]` {
		t.Fatalf("source_fields=%q, want structured source field list preserved", action.Params["source_fields"])
	}
	if action.Params["match_mode"] != "exact" {
		t.Fatalf("match_mode=%q, want concrete default", action.Params["match_mode"])
	}
}

func TestConcreteActionFromScaffoldMaterializesJoinFields(t *testing.T) {
	action, ok := ConcreteActionFromScaffold(ActionScaffold{
		Kind:         string(dataquery.DataActionJoinRecords),
		Executable:   true,
		InputPaths:   []string{"left.json", "right.json"},
		CommonFields: []string{"id"},
		ParamsTemplate: map[string]string{
			"join_type": "inner|left",
			"collision": "prefix",
		},
	})
	if !ok {
		t.Fatalf("ConcreteActionFromScaffold ok=false, want join action")
	}
	if action.Params["left_fields"] != `["id"]` || action.Params["right_fields"] != `["id"]` {
		t.Fatalf("params=%+v, want concrete join field arrays", action.Params)
	}
	if action.OutputArtifact == "" || !strings.HasSuffix(action.OutputArtifact, ".json") {
		t.Fatalf("OutputArtifact=%q, want generated json artifact", action.OutputArtifact)
	}
}

func TestConcreteActionFromScaffoldMaterializesValueDistribution(t *testing.T) {
	action, ok := ConcreteActionFromScaffold(ActionScaffold{
		Kind:       string(dataquery.DataActionValueDistribution),
		Executable: true,
		InputPath:  "records.json",
		Fields:     []string{"_source", "status", "group"},
		ParamsTemplate: map[string]string{
			"fields": `["<existing field from fields>"]`,
		},
	})
	if !ok {
		t.Fatalf("ConcreteActionFromScaffold ok=false, want value_distribution action")
	}
	if action.Kind != dataquery.DataActionValueDistribution || strings.Join(action.InputPaths, ",") != "records.json" {
		t.Fatalf("action=%+v, want value_distribution over records.json", action)
	}
	if action.Params["fields"] != `["status","group"]` {
		t.Fatalf("fields param=%q, want concrete field array", action.Params["fields"])
	}
}

func TestConcreteActionFromScaffoldRejectsPromptOnlyTemplate(t *testing.T) {
	_, ok := ConcreteActionFromScaffold(ActionScaffold{
		Kind:         string(dataquery.DataActionJoinRecords),
		InputPaths:   []string{"left.json", "right.json"},
		CommonFields: []string{"id"},
		ParamsTemplate: map[string]string{
			"join_type": "inner",
		},
	})
	if ok {
		t.Fatal("ConcreteActionFromScaffold ok=true, want prompt-only scaffold rejected")
	}
}

func TestConcreteFallbackScaffoldsDoNotReturnToNormalizeAfterEntityStage(t *testing.T) {
	scaffolds := []ActionScaffold{
		{Kind: string(dataquery.DataActionNormalizeEntities)},
		{Kind: string(dataquery.DataActionJoinRecords)},
	}
	got := ConcreteFallbackScaffolds(scaffolds, StageFacts{
		MaterialCoverageSufficient: true,
		EntityStageMaterialized:    true,
		ContributionLedgerRequired: true,
	})
	if len(got) != 1 || got[0].Kind != string(dataquery.DataActionJoinRecords) {
		t.Fatalf("fallback scaffolds=%+v, want normalize filtered after entity stage materializes", got)
	}
}

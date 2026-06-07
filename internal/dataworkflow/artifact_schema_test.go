package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestProjectArtifactSchemasPreservesFullFields(t *testing.T) {
	headers := make([]string, 0, 48)
	for i := 0; i < 40; i++ {
		headers = append(headers, "field_"+string(rune('a'+(i%26))))
	}
	headers = append(headers, "required_field")
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{{
		ID:      "joined_rows",
		Kind:    "record_artifact",
		Headers: headers,
		Fields: map[string]string{
			"json_shape": "array(len=10)",
		},
	}})
	if len(projections) != 1 {
		t.Fatalf("projection count = %d, want 1", len(projections))
	}
	if !containsString(projections[0].Fields, "required_field") {
		t.Fatalf("full contract fields dropped required_field: %#v", projections[0].Fields)
	}
}

func TestProjectArtifactSchemasPreservesLineageRoles(t *testing.T) {
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{{
		ID:                "mapping",
		Kind:              string(dataquery.DataActionNormalizeEntities),
		SourcePaths:       []string{"reference.csv", "items.csv"},
		SourceRecordPaths: []string{"items.csv"},
		ReferencePaths:    []string{"reference.csv"},
		EvidencePaths:     []string{"items.csv:2"},
		Fields: map[string]string{
			"json_shape": "array(len=2)",
		},
	}})
	if len(projections) != 1 {
		t.Fatalf("projection count = %d, want 1", len(projections))
	}
	if got := projections[0]; !containsString(got.SourceRecordPaths, "items.csv") || !containsString(got.ReferencePaths, "reference.csv") || !containsString(got.EvidencePaths, "items.csv:2") {
		t.Fatalf("projection=%+v, want source/reference/evidence lineage roles", got)
	}
}

func TestProjectArtifactSchemasPropagatesTransitiveLineage(t *testing.T) {
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{
		{
			ID:                "entity_mappings.json",
			Kind:              string(dataquery.DataActionNormalizeEntities),
			SourcePaths:       []string{"orders.csv", "lookup.csv"},
			SourceRecordPaths: []string{"orders.csv"},
			ReferencePaths:    []string{"lookup.csv"},
			Headers:           []string{"item_id", "canonical_id"},
			Fields: map[string]string{
				"artifact_aliases": "entity_mappings.json",
				"json_shape":       "array(len=2)",
			},
		},
		{
			ID:          "entity_mapping_records.json",
			Kind:        string(dataquery.DataActionExtractRecords),
			SourcePaths: []string{"entity_mappings.json"},
			Headers:     []string{"item_id", "canonical_id"},
			Fields: map[string]string{
				"artifact_aliases": "entity_mapping_records.json",
				"json_shape":       "array(len=2)",
			},
		},
	})
	if len(projections) != 2 {
		t.Fatalf("projection count=%d, want 2", len(projections))
	}
	extracted := projections[1]
	if !containsString(extracted.SourceRecordPaths, "orders.csv") {
		t.Fatalf("SourceRecordPaths=%v, want transitive source record orders.csv", extracted.SourceRecordPaths)
	}
	if !containsString(extracted.ReferencePaths, "lookup.csv") {
		t.Fatalf("ReferencePaths=%v, want transitive reference lookup.csv", extracted.ReferencePaths)
	}
}

func TestArtifactResolutionLineageCompatibilityUsesNonReferenceSource(t *testing.T) {
	base := ArtifactSchemaProjection{
		ID:          "orders_records.json",
		Aliases:     []string{"orders_records.json"},
		SourcePaths: []string{"orders.csv"},
		Fields:      []string{"item_id", "raw_category"},
	}
	ledger := ArtifactSchemaProjection{
		ID:                "category_resolutions.json",
		Kind:              string(dataquery.DataActionNormalizeEntities),
		Aliases:           []string{"category_resolutions.json"},
		SourcePaths:       []string{"taxonomy.csv", "orders.csv:2", "orders.csv:3"},
		SourceRecordPaths: []string{"taxonomy.csv"},
		ReferencePaths:    []string{"taxonomy.csv"},
		Fields:            []string{"item_id", "source_value", "canonical_id", "status"},
	}
	if !ArtifactResolutionLineageCompatible(base, ledger) {
		t.Fatalf("ledger=%+v should be compatible with base through non-reference source line roots", ledger)
	}
	sources, precise := ResolutionSourceCandidates(ledger)
	if !precise || !containsString(sources, "orders.csv") {
		t.Fatalf("sources=%v precise=%v, want orders.csv as precise source candidate", sources, precise)
	}
}

func TestProjectArtifactSchemasNewestFirstDedupesByAlias(t *testing.T) {
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{
		{
			ID:      "new_artifact",
			Kind:    "record_artifact",
			Headers: []string{"new_field"},
			Fields: map[string]string{
				"artifact_aliases": "shared_alias",
				"json_shape":       "array(len=1)",
			},
		},
		{
			ID:      "old_artifact",
			Kind:    "record_artifact",
			Headers: []string{"old_field"},
			Fields: map[string]string{
				"artifact_aliases": "shared_alias",
				"json_shape":       "array(len=1)",
			},
		},
	})
	if len(projections) != 1 {
		t.Fatalf("projection count = %d, want 1", len(projections))
	}
	if projections[0].ID != "new_artifact" {
		t.Fatalf("kept %q, want newest artifact", projections[0].ID)
	}
	if !containsString(projections[0].Fields, "new_field") {
		t.Fatalf("kept projection does not expose newest fields: %#v", projections[0].Fields)
	}
}

func TestProjectArtifactSchemasIncludesChildren(t *testing.T) {
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{{
		ID:   "parent",
		Kind: "summary",
		Fields: map[string]string{
			"json_shape": "object",
		},
		Children: []dataquery.DataArtifact{{
			ID:      "child_records",
			Kind:    "record_artifact",
			Headers: []string{"child_field"},
			Fields: map[string]string{
				"json_shape": "array(len=2)",
			},
		}},
	}})
	if len(projections) != 2 {
		t.Fatalf("projection count = %d, want 2", len(projections))
	}
	if projections[1].ID != "child_records" {
		t.Fatalf("child projection missing or out of order: %#v", projections)
	}
	if !containsString(projections[1].Fields, "child_field") {
		t.Fatalf("child fields missing: %#v", projections[1].Fields)
	}
}

func TestProjectArtifactSchemasClassifiesWorkflowLedgerAndDiagnosticChildren(t *testing.T) {
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{{
		ID:   "workflow_entity_resolutions",
		Kind: "workflow_ledger/entity_resolutions",
		Fields: map[string]string{
			"workflow_ledger": "true",
			"json_shape":      "array(len=2)",
		},
		Children: []dataquery.DataArtifact{{
			ID:      "records.json#base",
			Kind:    "apply_entity_resolutions/base",
			Headers: []string{"id"},
			Fields:  map[string]string{"json_shape": "array(len=2)"},
		}},
	}})
	if len(projections) != 2 {
		t.Fatalf("projection count=%d, want workflow ledger plus child", len(projections))
	}
	if projections[0].NodeClass != ArtifactNodeClassWorkflowLedger {
		t.Fatalf("workflow ledger node_class=%q", projections[0].NodeClass)
	}
	if projections[1].NodeClass != ArtifactNodeClassDiagnosticChild {
		t.Fatalf("diagnostic child node_class=%q", projections[1].NodeClass)
	}
}

func TestProjectArtifactSchemasClassifiesEntityResolutionSourceChildrenAsDiagnostic(t *testing.T) {
	projections := ProjectArtifactSchemasNewestFirst([]dataquery.DataArtifact{{
		ID:      "entity_mappings.json",
		Kind:    string(dataquery.DataActionNormalizeEntities),
		Headers: []string{"item_id", "source_value", "canonical_id", "canonical_label"},
		Fields:  map[string]string{"json_shape": "array(len=2)"},
		Children: []dataquery.DataArtifact{{
			ID:      "records.json#entity_resolution_source",
			Kind:    "apply_entity_resolutions/resolution",
			Headers: []string{"item_id", "source_value", "canonical_id"},
			Fields:  map[string]string{"json_shape": "array(len=2)"},
		}, {
			ID:      "records.json#entity_resolutions",
			Kind:    "entity_resolution_source",
			Headers: []string{"source_field", "canonical_id_field"},
			Fields:  map[string]string{"json_shape": "array(len=2)"},
		}},
	}})
	if len(projections) != 3 {
		t.Fatalf("projection count=%d, want parent plus two children", len(projections))
	}
	for _, projection := range projections[1:] {
		if projection.NodeClass != ArtifactNodeClassDiagnosticChild {
			t.Fatalf("%s node_class=%q, want diagnostic_child", projection.ID, projection.NodeClass)
		}
		if ArtifactUsableForRecordAction(projection) {
			t.Fatalf("%s should not be usable as record action input", projection.ID)
		}
	}
}

func TestMissingFieldsOnArtifactSchemaUsesAliasContract(t *testing.T) {
	projections := []ArtifactSchemaProjection{{
		ID:      "records",
		Aliases: []string{"records.json", "/tmp/work/records.json"},
		Fields:  []string{"id", "amount", "status"},
	}}

	missing := MissingFieldsOnArtifactSchema(projections, "records.json", []string{"amount", "currency", "status", "currency"})
	if len(missing) != 1 || missing[0] != "currency" {
		t.Fatalf("missing=%v, want currency only", missing)
	}
	if _, ok := ArtifactSchemaByAlias(projections, "/tmp/work/records.json"); !ok {
		t.Fatalf("expected path alias to resolve")
	}
	if missing := MissingFieldsOnArtifactSchema(projections, "unknown.json", []string{"amount"}); len(missing) != 0 {
		t.Fatalf("unknown schema should not hard fail, missing=%v", missing)
	}
}

func TestArtifactUsableForRecordActionUsesNodeClassAndKind(t *testing.T) {
	if !ArtifactUsableForRecordAction(ArtifactSchemaProjection{
		ID:        "records",
		Kind:      string(dataquery.DataActionExtractRecords),
		NodeClass: ArtifactNodeClassRecord,
		Fields:    []string{"id"},
		JSONShape: "array(len=1)",
	}) {
		t.Fatalf("record artifact should be usable for record actions")
	}
	if ArtifactUsableForRecordAction(ArtifactSchemaProjection{
		ID:        "records#base",
		Kind:      string(dataquery.DataActionApplyResolutions) + "/base",
		NodeClass: ArtifactNodeClassDiagnosticChild,
		Fields:    []string{"id"},
		JSONShape: "array(len=1)",
	}) {
		t.Fatalf("diagnostic child should not be usable for record actions")
	}
	if ArtifactUsableForRecordAction(ArtifactSchemaProjection{
		ID:        "workflow_contributions",
		Kind:      "workflow_ledger/contributions",
		NodeClass: ArtifactNodeClassWorkflowLedger,
		Fields:    []string{"group_key"},
		JSONShape: "array(len=1)",
	}) {
		t.Fatalf("workflow ledger should not be used as an ordinary record action base")
	}
	if ArtifactUsableForRecordAction(ArtifactSchemaProjection{
		ID:        "rules",
		Kind:      string(dataquery.DataActionDeriveRules),
		NodeClass: ArtifactNodeClassArtifact,
		Fields:    []string{"rule_id"},
	}) {
		t.Fatalf("derive_rules artifact should not be treated as record-action input")
	}
}

func TestBuildArtifactGraphStateProjectsAliasesLineageDiagnostics(t *testing.T) {
	graph := BuildArtifactGraphState([]dataquery.DataArtifact{{
		ID:                "eligible_records",
		Kind:              string(dataquery.DataActionFilterRecords),
		Headers:           []string{"id", "amount", "status"},
		SourcePaths:       []string{"source.csv"},
		SourceRecordPaths: []string{"source_records.json"},
		ReferencePaths:    []string{"reference.json"},
		EvidencePaths:     []string{"source.csv:2"},
		RowCount:          7,
		Fields: map[string]string{
			"artifact_aliases":   "eligible_records,eligible_records.json",
			"artifact_path":      "/tmp/work/eligible_records.json",
			"json_shape":         "array(len=7,item=object(keys=id,amount,status))",
			"input_rows":         "10",
			"output_rows":        "7",
			"filter_diagnostics": `{"total":10,"combined_match":7}`,
		},
	}}, 8)
	if graph.NodeCount != 1 || graph.Truncated {
		t.Fatalf("graph count/truncated=%d/%v, want 1/false", graph.NodeCount, graph.Truncated)
	}
	if len(graph.Nodes) != 1 {
		t.Fatalf("nodes=%d, want 1", len(graph.Nodes))
	}
	node := graph.Nodes[0]
	if node.ProducerKind != string(dataquery.DataActionFilterRecords) || !node.ExecutableRecordInput {
		t.Fatalf("node producer/executable=%q/%v", node.ProducerKind, node.ExecutableRecordInput)
	}
	if node.RowCount != 7 || !containsString(node.Fields, "amount") {
		t.Fatalf("node row/fields=%d/%v", node.RowCount, node.Fields)
	}
	if !containsString(node.Lineage.SourceRecordPaths, "source_records.json") || !containsString(node.Lineage.ReferencePaths, "reference.json") {
		t.Fatalf("lineage=%+v, want source record and reference roles", node.Lineage)
	}
	if node.Diagnostics["input_rows"] != "10" || node.Diagnostics["output_rows"] != "7" || node.Diagnostics["filter_diagnostics"] == "" {
		t.Fatalf("diagnostics=%+v, want structural runner diagnostics", node.Diagnostics)
	}
	if node.Diagnostics["artifact_path"] != "" || node.Diagnostics["json_shape"] != "" {
		t.Fatalf("diagnostics leaked access metadata: %+v", node.Diagnostics)
	}
	if !containsString(graph.ExecutableRecordAliases, "eligible_records.json") {
		t.Fatalf("executable aliases=%v, want eligible_records.json", graph.ExecutableRecordAliases)
	}
	var found bool
	for _, binding := range graph.AliasIndex {
		if binding.Alias == "eligible_records.json" && binding.NodeID == "eligible_records" && binding.ExecutableRecordInput {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("alias index=%+v, want executable binding for eligible_records.json", graph.AliasIndex)
	}
}

func TestBuildArtifactGraphStateReportsTruncationWithoutChangingCount(t *testing.T) {
	graph := BuildArtifactGraphState([]dataquery.DataArtifact{{
		ID:      "new_records",
		Kind:    string(dataquery.DataActionExtractRecords),
		Headers: []string{"id"},
		Fields:  map[string]string{"artifact_aliases": "new_records", "json_shape": "array(len=1)"},
	}, {
		ID:      "old_records",
		Kind:    string(dataquery.DataActionExtractRecords),
		Headers: []string{"id"},
		Fields:  map[string]string{"artifact_aliases": "old_records", "json_shape": "array(len=1)"},
	}}, 1)
	if graph.NodeCount != 2 || !graph.Truncated {
		t.Fatalf("graph count/truncated=%d/%v, want 2/true", graph.NodeCount, graph.Truncated)
	}
	if len(graph.Nodes) != 1 || graph.Nodes[0].ID != "new_records" {
		t.Fatalf("nodes=%+v, want newest node only", graph.Nodes)
	}
	for _, binding := range graph.AliasIndex {
		if binding.Alias == "old_records" {
			t.Fatalf("alias index included truncated node: %+v", graph.AliasIndex)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

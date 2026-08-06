package dataquery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// A child alias from a multi-material extract retains source-value identity,
// while its numeric positions can overlap locator numbers from unrelated
// resolution rows. Source values must win when no explicit base-key contract
// was declared; otherwise a coincidental row number silently changes values.
func TestApplyEntityResolutionsSourceValuePrecedesImplicitLocatorCollision(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"labels.csv":       "raw_label,canonical_label\nA-one,GroupA\nA-two,GroupA\nBeta,GroupB\nGamma alt,GroupC\n",
		"observations.csv": "record_id,raw_label,value,active\nr1,A-one,10,true\nr2,A-two,7,true\nr3,A-one,3,false\nr4,Beta,4,true\nr5,Gamma alt,5,true\nr6,unmapped,11,true\n",
		"targets.csv":      "target_id,canonical_label\nT1,GroupA\nT2,GroupX\nT3,GroupC\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tempRoot := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	res, err := (ActionRunner{RepoRoot: root, TempRoot: tempRoot}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{
			{
				ID:             "extract_all",
				Kind:           DataActionExtractRecords,
				InputPaths:     []string{"labels.csv", "observations.csv", "targets.csv"},
				OutputArtifact: "csv_records",
			},
			{
				ID:             "normalize",
				Kind:           DataActionNormalizeEntities,
				InputPaths:     []string{"observations.csv#records", "labels.csv#records"},
				OutputArtifact: "entity_mappings",
				Params: map[string]string{
					"source_path":           "observations.csv#records",
					"reference_path":        "labels.csv#records",
					"source_fields":         `["record_id","raw_label"]`,
					"reference_name_fields": `["raw_label","canonical_label"]`,
					"canonical_id_field":    "raw_label",
					"canonical_label_field": "canonical_label",
					"match_mode":            "exact",
				},
			},
			{
				ID:             "apply",
				Kind:           DataActionApplyResolutions,
				InputPaths:     []string{"observations.csv#records", "entity_mappings"},
				OutputArtifact: "resolved_records",
				Params: map[string]string{
					"base_path":          "observations.csv#records",
					"base_filter_mode":   "preserve",
					"preserve_base_rows": "true",
					"resolution_specs": `[{
						"resolution_path":"entity_mappings",
						"resolution_key_fields":["item_id"],
						"target_id_field":"canonical_id",
						"target_label_field":"canonical_label",
						"target_status_field":"resolution_status",
						"unmatched_status":"unmatched"
					}]`,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Artifacts) != 3 {
		t.Fatalf("artifacts=%d, want extract/normalize/apply", len(res.Artifacts))
	}
	artifact := res.Artifacts[2]
	if artifact.Fields["matched_canonical_id"] != "5" || artifact.Fields["unmatched_canonical_id"] != "1" {
		t.Fatalf("apply receipt=%+v, want matched=5 unmatched=1", artifact.Fields)
	}
	raw, err := os.ReadFile(artifact.Fields["artifact_path"])
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 6 {
		t.Fatalf("rows=%d, want 6", len(rows))
	}
	if rows[4]["record_id"] != "r5" || rows[4]["canonical_id"] != "Gamma alt" || rows[4]["canonical_label"] != "GroupC" || rows[4]["resolution_status"] != "resolved" {
		t.Fatalf("Gamma row=%+v, want its source-value mapping", rows[4])
	}
	if rows[5]["record_id"] != "r6" || rows[5]["canonical_id"] != "" || rows[5]["canonical_label"] != "" || rows[5]["resolution_status"] != "unmatched" {
		t.Fatalf("unmapped row=%+v, want unmatched without locator fallback", rows[5])
	}
}

func TestApplyEntityResolutionsExplicitBaseKeyPrecedesSourceValue(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("records.json", []map[string]any{{"row_id": "base-1", "raw_label": "alpha"}})
	writeJSON("resolution.json", []map[string]any{
		{"item_id": "base-1", "source_field": "raw_label", "source_value": "different", "canonical_id": "EXPLICIT", "status": "resolved"},
		{"item_id": "base-2", "source_field": "raw_label", "source_value": "alpha", "canonical_id": "VALUE", "status": "resolved"},
	})
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{{
			ID:             "apply",
			Kind:           DataActionApplyResolutions,
			InputPaths:     []string{"records.json", "resolution.json"},
			OutputArtifact: "resolved",
			Params: map[string]string{
				"base_path":             "records.json",
				"resolution_path":       "resolution.json",
				"base_key_fields":       `["row_id"]`,
				"resolution_key_fields": `["item_id"]`,
				"target_id_field":       "canonical_id",
				"target_status_field":   "resolution_status",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(res.Artifacts[0].Sample[0]), &row); err != nil {
		t.Fatal(err)
	}
	if row["canonical_id"] != "EXPLICIT" || row["resolution_status"] != "resolved" {
		t.Fatalf("row=%+v, explicit base-key contract must remain highest authority", row)
	}
}

// A filtered/materialized artifact keeps the original source index even though
// its local JSON-array ordinal is compacted. normalize_entities and
// apply_entity_resolutions must use the same identity domain; otherwise the
// mapping for the next surviving row can be attached to the current row while
// every downstream contribution and reconcile receipt remains self-consistent.
func TestNormalizeThenApplyEntityResolutionsPreservesFilteredSourceIndex(t *testing.T) {
	root := t.TempDir()
	writeJSON := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("active.json", []map[string]any{
		{"_source_index": 1, "record_id": "r1", "raw_label": "A-one", "value": 10},
		{"_source_index": 2, "record_id": "r2", "raw_label": "A-two", "value": 7},
		{"_source_index": 4, "record_id": "r4", "raw_label": "Beta", "value": 4},
		{"_source_index": 5, "record_id": "r5", "raw_label": "Gamma alt", "value": 5},
	})
	writeJSON("labels.json", []map[string]any{
		{"raw_label": "A-one", "canonical_label": "GroupA"},
		{"raw_label": "A-two", "canonical_label": "GroupA"},
		{"raw_label": "Beta", "canonical_label": "GroupB"},
		{"raw_label": "Gamma alt", "canonical_label": "GroupC"},
	})
	tempRoot := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	res, err := (ActionRunner{RepoRoot: root, TempRoot: tempRoot}).Run(context.Background(), TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputFreeform, ExplanationAllowed: true},
		Actions: []DataAction{
			{
				ID:             "normalize",
				Kind:           DataActionNormalizeEntities,
				InputPaths:     []string{"active.json", "labels.json"},
				OutputArtifact: "entity_mappings",
				Params: map[string]string{
					"source_path":           "active.json",
					"reference_path":        "labels.json",
					"source_field":          "raw_label",
					"reference_name_fields": `["raw_label"]`,
					"canonical_id_field":    "raw_label",
					"canonical_label_field": "canonical_label",
					"match_mode":            "exact",
				},
			},
			{
				ID:             "apply",
				Kind:           DataActionApplyResolutions,
				InputPaths:     []string{"active.json", "entity_mappings"},
				OutputArtifact: "resolved",
				Params: map[string]string{
					"base_path":             "active.json",
					"resolution_path":       "entity_mappings",
					"base_key_fields":       `["_source_index"]`,
					"resolution_key_fields": `["item_id"]`,
					"source_field":          "raw_label",
					"target_id_field":       "canonical_id",
					"target_label_field":    "canonical_label",
					"target_status_field":   "canonical_status",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Artifacts) != 2 {
		t.Fatalf("artifacts=%d, want normalize/apply", len(res.Artifacts))
	}
	if got := res.EntityResolutions[2].ItemID.String(); got != "active.json#4:raw_label" {
		t.Fatalf("Beta item_id=%q, want stable source index 4", got)
	}
	raw, err := os.ReadFile(res.Artifacts[1].Fields["artifact_path"])
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if rows[2]["record_id"] != "r4" || rows[2]["canonical_id"] != "Beta" || rows[2]["canonical_label"] != "GroupB" {
		t.Fatalf("Beta row=%+v, want Beta/GroupB without compacted-index cross-attachment", rows[2])
	}
	if rows[3]["record_id"] != "r5" || rows[3]["canonical_id"] != "Gamma alt" || rows[3]["canonical_label"] != "GroupC" {
		t.Fatalf("Gamma row=%+v, want Gamma alt/GroupC", rows[3])
	}
}

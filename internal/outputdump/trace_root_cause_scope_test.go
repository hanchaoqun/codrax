package outputdump

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1619RootCauseScopeSurvivesBothRealWriters(t *testing.T) {
	start, end := 2.0, 2.020
	profile := &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start, TimeEnd: &end, SourceQuote: "2.000..2.020",
	}
	requested := types.ResolveTraceQueryWindowScope(profile, 2, 2.020)
	exploration := types.ResolveTraceQueryWindowScope(profile, 2, 2.021)
	unknown := types.ResolveTraceQueryWindowScope(profile, 0, 0)
	elected := types.ResolveTraceQueryWindowScope(nil, 2, 2.021)
	for _, tc := range []struct {
		name  string
		scope *types.TraceQueryWindowScope
	}{
		{"requested", &requested},
		{"exploration", &exploration},
		{"unknown_query", &unknown},
		{"elected_without_explicit_request", &elected},
		{"legacy_without_scope", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			explicitJSON := filepath.Join(dir, "api", "root-causes.json")
			explicitMD := filepath.Join(dir, "api", "answer.md")
			withExplicitReport(t, ExplicitReport{RootCauseJSONPath: explicitJSON, MarkdownPath: explicitMD})
			a := explicitReportArgs(filepath.Join(dir, "default"))
			a.HasTrace = true
			a.Answer = "# 原模型分析\n\n模型选择保持原顺序，不由旁路排序。\n\n```mermaid\nsequenceDiagram\n    participant app as \"应用\"\n    participant worker as \"工作线程\"\n    app->>worker: 请求\n    worker-->>app: 完成\n```\n"
			a.RootCauseReport = &types.TraceRootCauseReportV2{
				SchemaVersion: types.TraceRootCauseReportSchemaVersion,
				RootCauses: []*types.TraceRootCauseItemV2{
					{Rank: 1, Category: types.TraceRootCauseCPUSchedulingDelay,
						ThreadName: "app-100", ArtifactLabel: "capture-A.ftrace",
						ImpactSeconds:   floatPointerForSidecarTest(0.000020),
						ImpactCaliber:   types.TraceImpactCaliberEffectiveAttribution,
						CausalQualifier: types.TraceCausalQualifierNotApplicable,
						Summary:         "应用线程调度延迟", Description: "模型保留的第一项说明。",
						Evidence: []string{"原始观测二十微秒"}, WindowScope: tc.scope},
					{Rank: 2, Category: types.TraceRootCauseIOBlocking,
						ThreadName: "worker-200", ArtifactLabel: "capture-B.ftrace",
						ImpactSeconds:   floatPointerForSidecarTest(0.011),
						ImpactCaliber:   types.TraceImpactCaliberWindowProjection,
						CausalQualifier: types.TraceCausalQualifierNotApplicable,
						Summary:         "工作线程 IO 等待", Description: "模型保留的第二项说明。",
						Evidence: []string{"独立捕获中的原始观测"}, WindowScope: &requested},
				},
			}
			before, err := json.Marshal(a.RootCauseReport)
			if err != nil {
				t.Fatal(err)
			}
			originalAnswer, expectedMarkdown := a.Answer, BuildBody(a)
			var priorDefault, priorExplicit []byte
			for attempt := 0; attempt < 2; attempt++ {
				result := WriteResult(a)
				if result.RootCauseJSONError != nil || result.MarkdownPath == "" || result.RootCauseJSONPath != explicitJSON {
					t.Fatalf("both real output sinks must succeed: %+v", result)
				}
				defaultJSON := readFileOrFatal(t, RootCauseJSONPathForMarkdown(result.MarkdownPath))
				explicitJSONBody := readFileOrFatal(t, explicitJSON)
				var defaultArtifact DefaultRootCauseArtifact
				var explicitArtifact ExplicitRootCauseArtifact
				if err := json.Unmarshal(defaultJSON, &defaultArtifact); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(explicitJSONBody, &explicitArtifact); err != nil {
					t.Fatal(err)
				}
				if defaultArtifact.Status != ExplicitRootCauseStatusAvailable || defaultArtifact.ReasonCode != "" ||
					explicitArtifact.Status != ExplicitRootCauseStatusAvailable || explicitArtifact.ReasonCode != "" {
					t.Fatalf("scope is not delivery failure: default=%+v explicit=%+v", defaultArtifact, explicitArtifact)
				}
				for sink, report := range map[string]*types.TraceRootCauseReportV2{
					"default": &defaultArtifact.TraceRootCauseReportV2, "explicit": explicitArtifact.TraceRootCauses,
				} {
					if !reflect.DeepEqual(report, a.RootCauseReport) {
						t.Errorf("%s writer changed scope, impact, model order, or description: got=%+v want=%+v", sink, report, a.RootCauseReport)
					}
				}
				for _, path := range []string{result.MarkdownPath, explicitMD} {
					markdown := string(readFileOrFatal(t, path))
					if markdown != expectedMarkdown || !strings.Contains(markdown, originalAnswer) {
						t.Fatalf("scope sidecar changed original Markdown/model Mermaid: %s", path)
					}
				}
				if attempt > 0 && (string(priorDefault) != string(defaultJSON) || string(priorExplicit) != string(explicitJSONBody)) {
					t.Fatal("repeated writing changed the per-item window scopes")
				}
				priorDefault, priorExplicit = defaultJSON, explicitJSONBody
			}
			after, err := json.Marshal(a.RootCauseReport)
			if err != nil || string(before) != string(after) || a.Answer != originalAnswer {
				t.Fatal("output writers mutated their input report or model answer")
			}
		})
	}
}

func TestB1619UnavailableRootCauseWritersDoNotMintWindowScope(t *testing.T) {
	dir := t.TempDir()
	explicitJSON := filepath.Join(dir, "api", "root-causes.json")
	withExplicitReport(t, ExplicitReport{RootCauseJSONPath: explicitJSON})
	a := explicitReportArgs(filepath.Join(dir, "default"))
	a.HasTrace = true
	a.RootCauseUnavailableReason = RootCauseReasonSelectionRejected
	result := WriteResult(a)
	if result.RootCauseJSONError != nil || result.MarkdownPath == "" || result.RootCauseJSONPath != explicitJSON {
		t.Fatalf("unavailable envelopes must still be written: %+v", result)
	}
	var defaultArtifact DefaultRootCauseArtifact
	var explicitArtifact ExplicitRootCauseArtifact
	defaultJSON := readFileOrFatal(t, RootCauseJSONPathForMarkdown(result.MarkdownPath))
	explicitJSONBody := readFileOrFatal(t, explicitJSON)
	if err := json.Unmarshal(defaultJSON, &defaultArtifact); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(explicitJSONBody, &explicitArtifact); err != nil {
		t.Fatal(err)
	}
	if defaultArtifact.Status != ExplicitRootCauseStatusUnavailable || defaultArtifact.ReasonCode != a.RootCauseUnavailableReason ||
		defaultArtifact.RootCauses == nil || len(defaultArtifact.RootCauses) != 0 ||
		explicitArtifact.Status != ExplicitRootCauseStatusUnavailable || explicitArtifact.ReasonCode != a.RootCauseUnavailableReason ||
		explicitArtifact.TraceRootCauses != nil {
		t.Fatalf("missing selection became a scoped conclusion: default=%s explicit=%s", defaultJSON, explicitJSONBody)
	}
	for _, body := range [][]byte{defaultJSON, explicitJSONBody} {
		if strings.Contains(string(body), "window_scope") {
			t.Fatalf("missing selection must not mint even an empty window_scope object: %s", body)
		}
	}
	if string(readFileOrFatal(t, result.MarkdownPath)) != BuildBody(a) {
		t.Fatal("unavailable sidecar changed the model answer")
	}
}

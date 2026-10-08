package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeIntentTargetValidationIndependentOfPreparation(t *testing.T) {
	confidence := .9
	targets := []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit", Confidence: confidence}}
	cursor := append([]types.RuntimeTarget(nil), targets...)
	cursor[0].Source = types.RuntimeTargetSourceExplicitToolCall
	for _, tc := range []struct {
		name    string
		p       emitRuntimeTargetProfileParam
		targets []types.RuntimeTarget
	}{
		{"no_named_target", emitRuntimeTargetProfileParam{Declaration: "no_named_target"}, nil},
		{"unspecified", emitRuntimeTargetProfileParam{Declaration: "unspecified"}, nil},
		{"named", emitRuntimeTargetProfileParam{Declaration: "named_target", SourceQuote: "thread 101"}, targets},
		{"named_missing_quote", emitRuntimeTargetProfileParam{Declaration: "named_target"}, targets},
		{"named_unanchored_quote", emitRuntimeTargetProfileParam{Declaration: "named_target", SourceQuote: "thread 999"}, targets},
		{"named_missing_roster", emitRuntimeTargetProfileParam{Declaration: "named_target", SourceQuote: "thread 101"}, nil},
		{"cursor_is_not_user", emitRuntimeTargetProfileParam{Declaration: "named_target", SourceQuote: "thread 101"}, cursor},
		{"no_named_conflict", emitRuntimeTargetProfileParam{Declaration: "no_named_target"}, targets},
		{"unspecified_conflict", emitRuntimeTargetProfileParam{Declaration: "unspecified"}, targets},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.p.Confidence = &confidence
			want, wantErr, wantWarnings := parseRuntimeTargetProfile("Inspect thread 101 in the capture directory.", true, &tc.p, tc.targets)
			got, gotErr, gotWarnings := parseRuntimeTargetProfile("Inspect thread 101 in the capture directory.", false, &tc.p, tc.targets)
			if !reflect.DeepEqual(got, want) || gotErr != wantErr || !reflect.DeepEqual(gotWarnings, wantWarnings) {
				t.Fatalf("pending preparation changed target declaration: got=%+v/%q/%v want=%+v/%q/%v", got, gotErr, gotWarnings, want, wantErr, wantWarnings)
			}
		})
	}
	if got, issue, _ := parseRuntimeTargetProfile("thread 101", false, nil, targets); got != nil || !strings.Contains(issue, "object missing") {
		t.Fatalf("typed roster without its declaration was accepted: %+v %q", got, issue)
	}
	legacy, issue, _ := parseRuntimeTargetProfile("explain code", false, &emitRuntimeTargetProfileParam{Declaration: "not_applicable", Confidence: &confidence}, nil)
	if issue != "" || legacy.Declaration != types.RuntimeTargetDeclarationNotApplicable {
		t.Fatalf("non-runtime declaration lost compatibility: %+v %q", legacy, issue)
	}
}

func TestEmitAnalysisRuntimeIntentNoNamedTargetPublic(t *testing.T) {
	for _, declaration := range []string{"no_named_target", "unspecified"} {
		t.Run(declaration, func(t *testing.T) {
			_, payload := runtimeIntentPublicPayload(t)
			const request = "For captures in /capture-set, list allocation events and stacks during 10.000..10.050 seconds."
			delete(payload, "runtime_targets")
			payload["runtime_target_profile"] = map[string]any{"declaration": declaration, "confidence": .95}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			ctx := &types.BusContext{Mutable: types.NewMutableState(request), RepoRoot: t.TempDir()}
			out, err := (&EmitAnalysis{}).Execute(ctx, raw)
			if err != nil || !out.Success {
				t.Fatalf("unprepared directory declaration rejected: %+v %v", out, err)
			}
			rm := ctx.Mutable.RequestModel()
			if rm.RuntimeTargetProfile.Declaration != types.RuntimeTargetDeclaration(declaration) || len(rm.RuntimeTargets) != 0 || !rm.RuntimeQuestionProfile.BoundedFactSet() {
				t.Fatalf("request declaration lost or an identity invented: %+v", rm)
			}
			if _, _, ok := traceSupplementDeriveTarget(ctx); ok {
				t.Fatal("declaration alone invented an answer target")
			}
			// A legitimate exploration cursor remains a cursor, not a user-named
			// identity or permission to run against an arbitrary repository file.
			rm.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 999, Source: types.RuntimeTargetSourceExplicitToolCall}}
			ctx.Mutable.SetRequestModel(*rm)
			if target, source, ok := traceSupplementDeriveTarget(ctx); !ok || source != "cursor" || target.PID != 999 || rm.RuntimeTargetProfile.NamedTarget() {
				t.Fatalf("cursor fallback changed its authority lane: %+v %q %t", target, source, ok)
			}
			if supplement := RunTraceQuerySystemSupplement(ctx); len(supplement.Executed) != 0 || emitAnalysisHasRuntimeArtifactCarrier(ctx) {
				t.Fatalf("request/cursor admitted a source: %+v", supplement)
			}
			path := filepath.Join(ctx.RepoRoot, "unrelated.systrace")
			if err := os.WriteFile(path, []byte("# a repository file is not a current-request source receipt\n"), 0600); err != nil {
				t.Fatal(err)
			}
			p, warning := traceQueryApplyRequestWindow(ctx, traceQueryParams{Source: "path", Path: path, View: "event_search"}, path, "path")
			if p.TimeStart.Set() || p.TimeEnd.Set() || warning != "" {
				t.Fatal("request/cursor bound an unrelated repository path")
			}
		})
	}
}

func TestEmitAnalysisRuntimeIntentRequiresTargetDeclarationPublic(t *testing.T) {
	for _, declaration := range []string{"missing", "not_applicable", "no_named_target"} {
		t.Run(declaration, func(t *testing.T) {
			request, payload := runtimeIntentPublicPayload(t)
			if declaration == "missing" {
				delete(payload, "runtime_target_profile")
				delete(payload, "runtime_targets")
			} else {
				payload["runtime_target_profile"] = map[string]any{"declaration": declaration, "confidence": .95}
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			ctx := &types.BusContext{Mutable: types.NewMutableState(request)}
			out, err := (&EmitAnalysis{}).Execute(ctx, raw)
			if err != nil || out.Success || ctx.Mutable.RequestModel() != nil || !strings.Contains(out.Summary, "runtime_target_profile") {
				t.Fatalf("missing/conflicting target declaration accepted: %+v %v", out, err)
			}
		})
	}
}

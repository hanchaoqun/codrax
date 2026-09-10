package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This exercises the existing permitted in-repository read path. The missing
// feature is a return-navigation hint, not permission to read a derived blob.
func TestB1624bActualQueryGrepDispatchReadReturnNavigation(t *testing.T) {
	b1624bActualReturnNavigation(t, 0, "grep-native")
}

func TestB1624bActualResultNavigationProducerMatrix(t *testing.T) {
	for _, format := range []struct {
		name  string
		index int
	}{{"text", 0}, {"json", 1}} {
		for _, producer := range []string{"grep-native", "grep-streamed", "read_file"} {
			t.Run(format.name+"/"+producer, func(t *testing.T) {
				b1624bActualReturnNavigation(t, format.index, producer)
			})
		}
	}
}

func b1624bActualReturnNavigation(t *testing.T, refIndex int, producer string) {
	t.Helper()
	ctx, published, refs := b1624PublishedResult(t)
	origin := refs[refIndex]
	before, err := os.ReadFile(origin)
	if err != nil {
		t.Fatal(err)
	}
	publishedBefore, _ := json.Marshal(published)
	var derived types.ToolResult
	if producer == "read_file" {
		params, _ := json.Marshal(map[string]any{"path": origin, "line_offset": 1, "limit": 12})
		derived, err = (&ReadFile{}).Execute(ctx, params)
	} else {
		p := map[string]any{"path": origin, "pattern": "."}
		if producer == "grep-native" {
			// The existing line-window branch uses the native bounded search.
			p["line_start"], p["line_end"] = 1, 100000
		}
		params, _ := json.Marshal(p)
		derived, err = (&GrepTool{}).Execute(ctx, params)
	}
	if err != nil || !derived.Success || derived.RawRef == "" || derived.RawRef == origin {
		t.Fatalf("actual successful %s must save a distinct derived result: err=%v result=%+v", producer, err, derived)
	}
	if producer != "read_file" && !strings.Contains(derived.Summary, "decision=broad_result_compacted") {
		t.Fatalf("actual broad result was not exercised: %s", derived.Summary)
	}
	if producer == "grep-streamed" && !strings.Contains(derived.Summary, "relevance=explicit_file=") {
		t.Fatalf("actual streamed compaction branch was not exercised: %s", derived.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(derived)
	if _, ok := ctx.Mutable.ResolveTraceQueryBlobRef(derived.RawRef); ok {
		t.Fatal("derived result must not acquire the original-query read escape permission")
	}
	saved, err := os.ReadFile(derived.RawRef)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(saved), "\n")
	if len(lines) < 5 {
		t.Fatalf("derived result did not retain a real pageable body: %q", saved)
	}
	params, _ := json.Marshal(map[string]any{"path": derived.RawRef, "line_offset": 1, "limit": 3})
	read, err := (&ReadFile{}).Execute(ctx, params)
	if err != nil || !read.Success {
		t.Fatalf("existing permitted derived-result read must remain successful: err=%v result=%+v", err, read)
	}
	if !strings.Contains(read.Summary, renderWithLineGutter(lines[1:4], 2)) {
		t.Fatalf("read must retain the exact requested derived-file lines, not substitute original-query bytes: %s", read.Summary)
	}
	after, _ := os.ReadFile(origin)
	savedAfter, _ := os.ReadFile(derived.RawRef)
	publishedAfter, _ := json.Marshal(published)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(saved, savedAfter) || string(publishedBefore) != string(publishedAfter) {
		t.Fatal("navigation/read changed the source artifact, derived bytes, or original publication")
	}
	b1624bAssertReturnNavigation(t, read, origin)
}

func b1624bAssertReturnNavigation(t *testing.T, result types.ToolResult, origin string) {
	t.Helper()
	// The original path occurs in grep's saved parameter banner. Its mere
	// appearance in page contents is not a navigation instruction.
	const marker = "query_result_return_navigation: "
	quoted, err := json.Marshal(origin)
	if err != nil {
		t.Fatal(err)
	}
	want := marker + "use grep or read_file on the original published query result at " + string(quoted) + ". This is navigation only: derived line numbers are not original trace lines and this link grants no new read or evidence authority. " + types.TraceQueryResultReadRoleGuidance
	var navigation []string
	for _, line := range strings.Split(result.Summary, "\n") {
		if strings.HasPrefix(line, marker) {
			navigation = append(navigation, line)
		}
	}
	if len(navigation) != 1 || navigation[0] != want {
		t.Errorf("missing exact independent return-navigation; got=%q want=%q", navigation, want)
	}
	// These three complete lines never needed a pagination refinement. A
	// return link is separate advice; it must not invent or replace that lane.
	if result.Refinement != nil {
		t.Errorf("return-navigation changed existing page refinement: %+v", result.Refinement)
	}
}

func b1624bPublishedDerivedNavigation(t *testing.T) (*types.BusContext, string, types.ToolResult) {
	t.Helper()
	ctx, _, refs := b1624PublishedResult(t)
	params, _ := json.Marshal(map[string]any{"path": refs[0], "pattern": ".", "line_start": 1, "line_end": 10000})
	derived, err := (&GrepTool{}).Execute(ctx, params)
	if err != nil || !derived.Success || derived.RawRef == "" {
		t.Fatalf("actual derived publication failed: err=%v result=%+v", err, derived)
	}
	ctx.Mutable.AppendDispatchToolResult(derived)
	return ctx, refs[0], derived
}

func TestB1624bReturnNavigationHelperUsesExactDerivedPath(t *testing.T) {
	ctx, origin, derived := b1624bPublishedDerivedNavigation(t)
	rel, err := filepath.Rel(ctx.RepoRoot, derived.RawRef)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{derived.RawRef, rel} {
		b1624bAssertReturnNavigation(t, types.ToolResult{Summary: ArtifactReadReturnNavigationAdvisory(ctx, input)}, origin)
	}
	for _, input := range []string{filepath.Base(derived.RawRef), filepath.Join(ctx.RepoRoot, "elsewhere", filepath.Base(derived.RawRef))} {
		if got := ArtifactReadReturnNavigationAdvisory(ctx, input); got != "" {
			t.Errorf("derived basename must not be guessed into another path: input=%q advice=%q", input, got)
		}
	}
	// Original query refs retain their pre-existing resolver compatibility.
	b1624bAssertReturnNavigation(t, types.ToolResult{Summary: ArtifactReadReturnNavigationAdvisory(ctx, filepath.Base(origin))}, origin)
	if _, ok := ctx.Mutable.ResolveTraceQueryBlobRef(derived.RawRef); ok {
		t.Fatal("navigation helper registered a derived read permission")
	}
}

func TestB1624bPreparedNavigationCannotSurviveReset(t *testing.T) {
	for _, republish := range []bool{false, true} {
		t.Run(map[bool]string{false: "reset", true: "reset-and-republish-same-path"}[republish], func(t *testing.T) {
			ctx, published, refs := b1624PublishedResult(t)
			ticket := prepareArtifactReadNavigation(ctx, refs[0])
			if ticket.InputRef != refs[0] {
				t.Fatalf("actual publication did not prepare a ticket: %+v", ticket)
			}
			output := StoreBlobArtifact(ctx.WorkDir, "read_file", "late-output.txt", "late result body")
			if output == "" {
				t.Fatal("test output was not saved")
			}
			result := types.ToolResult{ToolName: "read_file", Success: true, Summary: "late result body", RawRef: output}
			before := result
			ctx.Mutable.ResetTurnAArtifacts()
			if republish {
				ctx.Mutable.AppendDispatchToolResult(published)
			}
			finishArtifactReadNavigation(ctx, ticket, &result)
			if !reflect.DeepEqual(before, result) {
				t.Fatalf("old in-flight read minted a new-generation link or advice: before=%+v after=%+v", before, result)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			if got := ArtifactReadReturnNavigationAdvisory(ctx, output); got != "" {
				t.Errorf("old read output became navigable after reset: %q", got)
			}
		})
	}
}

func TestB1624bNavigationFailureAndExistingFields(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(map[bool]string{false: "failed-result", true: "successful-result"}[success], func(t *testing.T) {
			ctx, published, refs := b1624PublishedResult(t)
			origin := refs[0]
			ticket := prepareArtifactReadNavigation(ctx, origin)
			const body = "preserve body\nwith original coordinates 4..6\n"
			output := StoreBlobArtifact(ctx.WorkDir, "read_file", "navigation-fields.txt", body)
			if output == "" {
				t.Fatal("test output was not saved")
			}
			// These are independent lane sentinels. Navigation does not decide
			// their authority or rewrite a page cursor into an original trace line.
			result := types.ToolResult{
				ToolName: "read_file", Success: success, Summary: body, RawRef: output,
				Refinement:          &types.ToolRefinementHint{ReasonCode: "existing_page", PreferredNextTool: "read_file", NextCursor: "6", PreferredParams: map[string]string{"path": output, "line_offset": "6"}},
				ReadCoverage:        &types.ToolReadCoverage{Path: "original-read-path", LineStart: 4, LineEnd: 6, TotalLines: 99, RawRef: output},
				RuntimeArtifactRead: &types.ToolRuntimeArtifactRead{RequestedPath: origin, Kind: "blob", TraceQueryBlob: true, LineStart: 4, LineEnd: 6, TotalLines: 99, RawRef: output},
				Observations:        published.Observations,
			}
			before, _ := json.Marshal(result)
			finishArtifactReadNavigation(ctx, ticket, &result)
			navigationOnly := result
			navigationOnly.Refinement = nil
			b1624bAssertReturnNavigation(t, navigationOnly, origin)
			resultWithoutAdvice := result
			resultWithoutAdvice.Summary = body
			after, _ := json.Marshal(resultWithoutAdvice)
			if string(before) != string(after) || !strings.HasPrefix(result.Summary, body) {
				t.Fatalf("navigation changed an existing body/refinement/evidence field: before=%s after=%s", before, after)
			}
			saved, err := os.ReadFile(output)
			if err != nil || string(saved) != body {
				t.Fatalf("navigation changed saved output bytes: err=%v data=%q", err, saved)
			}
			if success {
				if result.ArtifactReadNavigation.InputRef != origin || result.ArtifactReadNavigation.OutputRef != output || result.ArtifactReadNavigation.OriginQueryRef != origin {
					t.Errorf("successful saved output lost its exact navigation receipt: %+v", result.ArtifactReadNavigation)
				}
			} else if result.ArtifactReadNavigation != (types.ToolArtifactReadNavigation{}) {
				t.Errorf("failed result minted output navigation: %+v", result.ArtifactReadNavigation)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			got := ArtifactReadReturnNavigationAdvisory(ctx, output)
			if success {
				b1624bAssertReturnNavigation(t, types.ToolResult{Summary: got}, origin)
			} else if got != "" {
				t.Errorf("failed output was registered as a derived result: %q", got)
			}
			if _, ok := ctx.Mutable.ResolveTraceQueryBlobRef(output); ok {
				t.Fatal("soft navigation expanded direct-result permissions")
			}
		})
	}
}

func TestB1624bReaderSensitiveRefusalPrecedesNavigation(t *testing.T) {
	ctx, _, refs := b1624PublishedResult(t)
	origin := refs[0]
	// This is generated trace-query test data, never a real credential file.
	previous := SensitiveConfigFilePaths()
	SetSensitiveConfigFilePaths([]string{origin})
	t.Cleanup(func() { SetSensitiveConfigFilePaths(previous) })
	params, _ := json.Marshal(map[string]any{"path": origin})
	read, err := (&ReadFile{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	params, _ = json.Marshal(map[string]any{"path": origin, "pattern": "."})
	grep, err := (&GrepTool{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range []types.ToolResult{read, grep} {
		if result.Success || strings.Contains(result.Summary, origin) || strings.Contains(result.Summary, "query_result_return_navigation:") || result.ArtifactReadNavigation != (types.ToolArtifactReadNavigation{}) {
			t.Errorf("early sensitive-file refusal leaked navigation/source identity: %+v", result)
		}
	}
}

func TestB1624bActualDerivedReadAdviceMustNotRequeryDerivedCapture(t *testing.T) {
	for _, reader := range []string{"read_file-default-clamp", "read_file-page-clamp", "grep-native-broad", "grep-streamed-broad"} {
		t.Run(reader, func(t *testing.T) {
			ctx, origin, derived := b1624bPublishedDerivedNavigation(t)
			if got := ArtifactReadReturnNavigationAdvisory(ctx, derived.RawRef); got == "" {
				t.Fatal("actual derived publication must be registered before testing advice parity")
			}
			var result types.ToolResult
			var err error
			isReadFile := strings.HasPrefix(reader, "read_file-")
			if isReadFile {
				p := map[string]any{"path": derived.RawRef}
				if reader == "read_file-page-clamp" {
					p["line_offset"], p["limit"] = 1, 10000
				}
				params, _ := json.Marshal(p)
				result, err = (&ReadFile{}).Execute(ctx, params)
			} else {
				p := map[string]any{"path": derived.RawRef, "pattern": "."}
				if reader == "grep-native-broad" {
					p["line_start"], p["line_end"] = 1, 100000
				}
				params, _ := json.Marshal(p)
				result, err = (&GrepTool{}).Execute(ctx, params)
			}
			if err != nil || !result.Success {
				t.Fatalf("existing derived read failed before advice check: err=%v result=%+v", err, result)
			}
			if isReadFile && (result.Refinement == nil || !result.Refinement.ResultTruncated) {
				t.Fatalf("default read did not reach actual clamp: %+v", result.Refinement)
			}
			if !isReadFile && !strings.Contains(result.Summary, "decision=broad_result_compacted") {
				t.Fatalf("derived broad grep was not exercised: %s", result.Summary)
			}
			if isReadFile {
				wantTool := "grep"
				if reader == "read_file-page-clamp" {
					wantTool = "read_file"
				}
				hint := result.Refinement
				resolved, reject := resolveReadFilePath(ctx, hint.PreferredParams["path"])
				if hint.ReasonCode != "read_file_result_truncated" || hint.PreferredNextTool != wantTool || reject != nil || resolved != derived.RawRef || resolved == origin {
					t.Errorf("continuation must stay on the actual derived file: hint=%+v resolved=%q reject=%+v", hint, resolved, reject)
				}
				if hint.PreferredParams["view"] != "" || hint.PreferredParams["line_start"] != "" || hint.PreferredParams["line_end"] != "" {
					t.Errorf("derived page acquired raw-trace query coordinates: %+v", hint)
				}
				if reader == "read_file-page-clamp" {
					marker := result.RuntimeArtifactRead
					if marker == nil || marker.LineStart != 2 || marker.LineEnd <= marker.LineStart || marker.LineEnd >= marker.TotalLines {
						t.Fatalf("actual explicitly offset derived page was not retained: %+v", marker)
					}
					if hint.NextCursor != strconv.Itoa(marker.LineEnd) || hint.PreferredParams["line_offset"] != hint.NextCursor {
						t.Errorf("cursor must use the next offset of this derived page: hint=%+v read=%+v", hint, marker)
					}
				} else if hint.NextCursor != "" || hint.PreferredParams["line_offset"] != "" {
					t.Errorf("default grep continuation invented a page cursor: %+v", hint)
				}
				adviceOnly := result
				adviceOnly.Refinement = nil
				b1624bAssertReturnNavigation(t, adviceOnly, origin)
			}
			refinement, _ := json.Marshal(result.Refinement)
			t.Logf("origin=%s\nderived=%s\nSUMMARY:\n%s\nREFINEMENT:%s", origin, derived.RawRef, result.Summary, refinement)
			if result.Refinement != nil && result.Refinement.PreferredNextTool == "trace_query" {
				t.Errorf("derived result was mistaken for an original trace-query input: %s", refinement)
			}
			for _, forbidden := range []string{"trace_query_required_soft_advisory=", "Next call: trace_query(", "Switch to trace_query(", "next_shape=trace artifact grep matched too broadly"} {
				if strings.Contains(result.Summary, forbidden) {
					t.Errorf("derived result received contradictory raw-capture advice %q", forbidden)
				}
			}
		})
	}
}

func TestB1624bOriginalCaptureAndUnknownDerivedKeepExistingRefinement(t *testing.T) {
	ctx, _, derived := b1624bPublishedDerivedNavigation(t)
	unknown := ctx.ShallowClone()
	unknown.Mutable = types.NewMutableState("no current derived navigation")
	for _, tc := range []struct {
		name string
		ctx  *types.BusContext
		path string
	}{
		{"original-capture", ctx, ctx.AttachedHitrace},
		{"unknown-derived-in-new-task", unknown, derived.RawRef},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := prepareArtifactReadNavigation(tc.ctx, tc.path); got.InputRef != "" {
				t.Fatalf("negative fixture unexpectedly has current navigation: %+v", got)
			}
			before, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			params, _ := json.Marshal(map[string]any{"path": tc.path})
			result, err := (&ReadFile{}).Execute(tc.ctx, params)
			if err != nil || !result.Success || result.Refinement == nil || !result.Refinement.ResultTruncated {
				t.Fatalf("existing read/clamp was not preserved: err=%v result=%+v", err, result)
			}
			hint := result.Refinement
			resolved, reject := resolveReadFilePath(tc.ctx, hint.PreferredParams["path"])
			if hint.ReasonCode != "read_file_trace_artifact_truncated" || hint.PreferredNextTool != "trace_query" || hint.PreferredParams["view"] != "event_search" || reject != nil || resolved != tc.path {
				t.Errorf("unknown/original target changed its existing continuation policy: hint=%+v resolved=%q reject=%+v", hint, resolved, reject)
			}
			if strings.Contains(result.Summary, "\nquery_result_return_navigation:") || result.ArtifactReadNavigation != (types.ToolArtifactReadNavigation{}) {
				t.Errorf("unknown target acquired a return link: %+v", result.ArtifactReadNavigation)
			}
			after, _ := os.ReadFile(tc.path)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("continuation hint changed the input bytes")
			}
		})
	}
}

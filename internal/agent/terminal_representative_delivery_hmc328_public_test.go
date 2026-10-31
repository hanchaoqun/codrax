package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

type representativePageHMC328 struct {
	Index        string                     `json:"saved_index"`
	IndexSHA     string                     `json:"saved_index_sha256"`
	Selection    string                     `json:"selection_snapshot"`
	SelectionSHA string                     `json:"selection_snapshot_sha256"`
	Calls        map[string]json.RawMessage `json:"query_calls"`
}

// Real public discovery and native SQLite queries; no model/status/operator.
// The terminal entry is the same entry called after the pipeline joins forks.
func representativeTerminalContextHMC328(t *testing.T, selectMembers bool) (*types.AgentContext, representativePageHMC328) {
	t.Helper()
	root, work := t.TempDir(), t.TempDir()
	for i, name := range []string{"a", "b", "c", "d"} {
		fixture := "capture.data"
		if i%2 == 1 {
			fixture = "nested/capture.data"
		}
		body, err := os.ReadFile(filepath.Join("../../eval/fixtures/hmosperf_catalog_objects/captures", fixture))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "captures", name, "capture.data")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &types.AgentContext{RepoRoot: root, WorkDir: work, Language: "en", Mutable: types.NewMutableState("representative resource observations"),
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")})}
	args := map[string]any{"root": "captures", "queries": []any{
		map[string]any{"view": "resource_stack", "target_scope": "thread", "pid": 101, "time_start": 10, "time_end": 10.05},
		map[string]any{"view": "resource_stack", "target_scope": "thread", "pid": 202, "time_start": 10, "time_end": 10.05},
	}}
	if selectMembers {
		args["representatives"] = map[string]any{"per_cluster": 2, "max_clusters": 2}
	}
	raw, _ := json.Marshal(args)
	result, err := (&tool.TraceCatalog{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !result.Success {
		t.Fatalf("discover: %v %s", err, result.Summary)
	}
	var page representativePageHMC328
	if err := json.Unmarshal([]byte(result.Summary), &page); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Mutable.TraceCatalogs()) != 1 || len(ctx.Mutable.TraceCatalogs()[0].Snapshot().Queries) != 8 {
		t.Fatal("fixture did not register eight exact plans")
	}
	if selectMembers && (page.Selection == "" || len(page.Calls) != 4) {
		t.Fatalf("fixture did not select four representatives: %s", result.Summary)
	}
	return ctx, page
}

func representativePairFromAnswerHMC328(t *testing.T, answer string) (tracecatalog.Snapshot, tracecatalog.RepresentativeSnapshot, string, string) {
	t.Helper()
	paths := make([]string, 2)
	for i, label := range []string{"Execution index", "Selection snapshot"} {
		matches := regexp.MustCompile(`\[`+regexp.QuoteMeta(label)+`\]\(<([^>]+)>\)`).FindAllStringSubmatch(answer, -1)
		if len(matches) != 1 {
			t.Fatalf("terminal answer needs exactly one current %s link without an extra status call; got %d: %s", label, len(matches), answer)
		}
		var err error
		paths[i], err = url.PathUnescape(matches[0][1])
		if err != nil {
			t.Fatal(err)
		}
	}
	index, err := tracecatalog.Load(context.Background(), paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var selection struct {
		tracecatalog.RepresentativeSnapshot
		Index    string `json:"saved_index"`
		IndexSHA string `json:"saved_index_sha256"`
	}
	data, err := os.ReadFile(paths[1])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &selection); err != nil {
		t.Fatal(err)
	}
	if selection.Index != paths[0] {
		t.Fatal("terminal selection points at another index generation")
	}
	for i, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		sha := hex.EncodeToString(sum[:])
		if !strings.Contains(answer, sha) {
			t.Fatal("terminal answer omitted the exact delivered hash")
		}
		if i == 0 && selection.IndexSHA != sha {
			t.Fatal("selection does not bind exact index bytes")
		}
	}
	queries := map[string]tracecatalog.QueryRecord{}
	for _, q := range index.Queries {
		queries[q.ID] = q
	}
	for _, group := range selection.Clusters {
		if group.Specification.View != "resource_stack" || group.Specification.Object.Kind != "thread" || group.Specification.Window == nil || group.Specification.Window.StartNS != 10_000_000_000 || group.Specification.Window.EndNS != 10_050_000_000 {
			t.Fatalf("spec changed: %+v", group.Specification)
		}
		for _, member := range group.Members {
			q, ok := queries[member.QueryID]
			if !ok || q.ArtifactID != member.ArtifactID || q.ArtifactRevision != member.ArtifactRevision || q.Outcome != member.Outcome || len(q.Attempts) != member.Attempts {
				t.Fatalf("index/selection mismatch: %+v", member)
			}
			if len(q.Attempts) > 0 {
				got, _ := json.Marshal(member.LatestAttempt)
				want, _ := json.Marshal(q.Attempts[len(q.Attempts)-1])
				if !bytes.Equal(got, want) {
					t.Fatal("selection latest attempt is not this index generation")
				}
			}
		}
	}
	return index, selection.RepresentativeSnapshot, paths[0], paths[1]
}

func TestHMC328RepresentativeTerminalPublishesJoinedQueriesWithoutStatus(t *testing.T) {
	ctx, discovery := representativeTerminalContextHMC328(t, true)
	oldIndex, _ := os.ReadFile(discovery.Index)
	oldSelection, _ := os.ReadFile(discovery.Selection)
	left, right := ctx.ShallowClone(), ctx.ShallowClone()
	left.Mutable, right.Mutable = ctx.Mutable.ForkForExploreDispatch(), ctx.Mutable.ForkForExploreDispatch()
	keys := make([]string, 0, len(discovery.Calls))
	for key := range discovery.Calls {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for i, key := range keys {
		branch := left
		if i%2 != 0 {
			branch = right
		}
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(branch, types.AgentExplorer), discovery.Calls[key])
		if err != nil || !result.Success {
			t.Fatalf("selected query: %v %s", err, result.Summary)
		}
	}
	ctx.Mutable.MergeExploreFork(left.Mutable)
	ctx.Mutable.MergeExploreFork(right.Mutable)
	before := ctx.Mutable.TraceCatalogs()[0].CurrentExecution()
	ledgerBefore, _ := json.Marshal(types.ObservationLedgerInputFromAgentContext(ctx, 0))
	const model = "Only the recorded resource events are compared; other members were not queried."
	answer := FinalizeTraceCatalogExports(ctx, model, "en")
	if !strings.HasPrefix(answer, model+"\n\n") {
		t.Fatal("terminal handoff missing or rewrote model analysis")
	}
	index, selection, indexPath, selectionPath := representativePairFromAnswerHMC328(t, answer)
	if len(index.Queries) != 8 || selection.Coverage.Clusters != 2 || selection.Coverage.Selected != 4 || selection.Coverage.BudgetNotSelected != 4 {
		t.Fatalf("roster changed: %+v", selection.Coverage)
	}
	attempts, queried := 0, 0
	for _, q := range index.Queries {
		attempts += len(q.Attempts)
		if len(q.Attempts) > 0 {
			queried++
		}
	}
	if attempts != 4 || queried != 4 || selection.Coverage.Outcomes[tracecatalog.OutcomeNotExecuted] != 4 {
		t.Fatalf("terminal snapshot missed/created attempts: %d %d %+v", attempts, queried, selection.Coverage)
	}
	if indexPath == discovery.Index || selectionPath == discovery.Selection {
		t.Fatal("terminal returned the discover-time generation")
	}
	if after := ctx.Mutable.TraceCatalogs()[0].CurrentExecution(); !reflect.DeepEqual(before, after) {
		t.Fatal("publication mutated executing catalog")
	}
	ledgerAfter, _ := json.Marshal(types.ObservationLedgerInputFromAgentContext(ctx, 0))
	if !bytes.Equal(ledgerBefore, ledgerAfter) {
		t.Fatal("navigation publication changed native evidence")
	}
	for path, want := range map[string][]byte{discovery.Index: oldIndex, discovery.Selection: oldSelection} {
		got, _ := os.ReadFile(path)
		if !bytes.Equal(got, want) {
			t.Fatal("old immutable generation was rewritten")
		}
	}
	versions := map[string]filegeneration.Identity{}
	for _, path := range []string{indexPath, selectionPath} {
		version, err := filegeneration.FromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		versions[path] = version
	}
	if got := FinalizeTraceCatalogExports(ctx, model, "en"); got != answer {
		t.Fatal("same terminal generation was not idempotent")
	}
	for path, before := range versions {
		after, err := filegeneration.FromPath(path)
		if err != nil || !before.SameVersion(after) {
			t.Fatal("idempotent delivery rewrote an immutable object")
		}
	}
}

func TestHMC328RepresentativeTerminalNoSelectionIsExactNoop(t *testing.T) {
	ctx, page := representativeTerminalContextHMC328(t, false)
	before, _ := os.ReadDir(filepath.Dir(page.Index))
	const model = "No representative selection was requested."
	if got := FinalizeTraceCatalogExports(ctx, model, "en"); got != model {
		t.Fatal("plain catalog acquired representative delivery")
	}
	after, _ := os.ReadDir(filepath.Dir(page.Index))
	if len(before) != len(after) {
		t.Fatal("unselected catalog created extra publications")
	}
}

func TestHMC328RepresentativeTerminalFailureAndEmptyAnswerDoNotMintLinks(t *testing.T) {
	for _, mode := range []string{"cancelled", "revoked", "save_failure", "empty_answer"} {
		t.Run(mode, func(t *testing.T) {
			ctx, page := representativeTerminalContextHMC328(t, true)
			model := "Keep the original explanation."
			switch mode {
			case "cancelled":
				cancelled, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Ctx = cancelled
			case "revoked":
				ctx.RepoRoot = t.TempDir()
				ctx.UserPinnedFiles = nil
			case "save_failure":
				bad := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(bad, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				ctx.WorkDir = bad
			case "empty_answer":
				model = ""
			}
			got := FinalizeTraceCatalogExports(ctx, model, "en")
			if !strings.HasPrefix(got, model) || strings.Contains(got, "](<") || strings.Contains(got, page.Index) || strings.Contains(got, page.Selection) {
				t.Fatalf("failed terminal delivery minted links or rewrote answer: %s", got)
			}
			if mode == "empty_answer" && got != "" {
				t.Fatal("no answer produced delivery side effects")
			}
		})
	}
}

func TestHMC328RepresentativeTerminalKeepsAttemptAfterIntermediateSaveFailure(t *testing.T) {
	ctx, page := representativeTerminalContextHMC328(t, true)
	fork := ctx.ShallowClone()
	fork.Mutable = ctx.Mutable.ForkForExploreDispatch()
	// Block only index persistence. Making the entire WorkDir unreadable
	// would also prevent the native query's original result from being saved.
	fork.WorkDir = t.TempDir()
	bad := filepath.Join(fork.WorkDir, "trace-catalog")
	if err := os.WriteFile(bad, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	var raw json.RawMessage
	for _, call := range page.Calls {
		raw = call
		break
	}
	out, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(fork, types.AgentExplorer), raw)
	if err != nil || !out.Success || !strings.Contains(out.Summary, "Capture index persistence failed:") {
		t.Fatalf("fixture did not preserve real query after failed save: %v %s", err, out.Summary)
	}
	if current := fork.Mutable.TraceCatalogs()[0].CurrentExecution(); current.Coverage.Attempts != 1 || current.Coverage.QueriesWithAttempts != 1 {
		t.Fatalf("fixture did not record the completed native query before the save failure: %+v", current.Coverage)
	}
	ctx.Mutable.MergeExploreFork(fork.Mutable)
	answer := FinalizeTraceCatalogExports(ctx, "One query was actually attempted.", "en")
	index, selection, _, _ := representativePairFromAnswerHMC328(t, answer)
	attempts := 0
	for _, q := range index.Queries {
		attempts += len(q.Attempts)
	}
	if attempts != 1 || selection.Coverage.Selected != 4 || selection.Coverage.BudgetNotSelected != 4 {
		t.Fatalf("terminal save lost the attempt or changed selection: attempts=%d coverage=%+v", attempts, selection.Coverage)
	}
}

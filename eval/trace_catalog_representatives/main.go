// trace_catalog_representatives prepares offline inputs and audits explicitly
// returned navigation artifacts. It never loads providers or invokes a model.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

const question = "请对 captures 目录中的采集做一次抽样对照：按线程和检查窗口归类，每类挑两份代表，比较 10.000 到 10.050 秒内线程 101 和 202 的分配、释放及调用位置，并概括各组的检查进度。"

type fixtureCopy struct{ Original, Destination string }

var copies = []fixtureCopy{
	{"captures/capture.data", "captures/session-a/capture.data"},
	{"captures/nested/capture.data", "captures/session-b/capture.data"},
	{"captures/capture.data", "captures/session-c/capture.data"},
	{"captures/nested/capture.data", "captures/session-d/capture.data"},
}

type expectedQuery struct {
	Path   string `json:"relative_path"`
	TID    int    `json:"tid"`
	Events int    `json:"events"`
	Frames int    `json:"source_frames"`
}

func expectedQueries() []expectedQuery {
	var queries []expectedQuery
	for _, copy := range copies {
		if copy.Original == "captures/capture.data" {
			queries = append(queries, expectedQuery{copy.Destination, 101, 3, 8}, expectedQuery{copy.Destination, 202, 1, 3})
		} else {
			queries = append(queries, expectedQuery{copy.Destination, 101, 1, 3}, expectedQuery{copy.Destination, 202, 0, 0})
		}
	}
	return queries
}

type preparation struct {
	Kind        string            `json:"kind"`
	Workspace   string            `json:"workspace"`
	Question    string            `json:"question"`
	OperatorPID int               `json:"operator_pid"`
	ModelCalls  int               `json:"model_calls"`
	ToolCalls   int               `json:"tool_calls"`
	Copies      []fixtureCopy     `json:"copies"`
	SourceSHA   map[string]string `json:"source_sha256"`
	Expected    []expectedQuery   `json:"expected_queries"`
	GitHEAD     string            `json:"git_head"`
}

type auditInput struct{ Out, Index, Selection, SelectionSHA string }

func main() {
	out := flag.String("out", "", "new absolute result directory; parent must exist")
	fixture := flag.String("fixture", "eval/fixtures/hmosperf_catalog_objects", "existing constructed SQLite fixture directory")
	index := flag.String("audit-index", "", "exact saved_index from current representative_status result")
	selection := flag.String("selection-snapshot", "", "exact selection_snapshot from the same current result")
	digest := flag.String("selection-sha256", "", "selection_snapshot_sha256 from that result")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(2)
	}
	var err error
	if *index != "" || *selection != "" || *digest != "" {
		err = audit(context.Background(), auditInput{*out, *index, *selection, *digest})
	} else {
		_, err = prepare(context.Background(), *out, *fixture)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Preparation does not discover a catalog, choose representatives, prepare a
// trace or execute a trace query. All of those are left to the real CLI run.
func prepare(ctx context.Context, out, fixture string) (preparation, error) {
	var receipt preparation
	if !filepath.IsAbs(out) {
		return receipt, errors.New("-out must be a new absolute directory")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		return receipt, err // Never reuse or clean an earlier result directory.
	}
	out, err := filepath.EvalSymlinks(out)
	if err != nil {
		return receipt, err
	}
	workspace, operator := filepath.Join(out, "workspace"), filepath.Join(out, "operator")
	for _, dir := range []string{workspace, operator, filepath.Join(out, "logs")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return receipt, err
		}
	}
	receipt = preparation{Kind: "constructed_sqlite_copies_no_seed_queries", Workspace: workspace, Question: question,
		OperatorPID: os.Getpid(), Copies: copies, SourceSHA: map[string]string{}, Expected: expectedQueries()}
	for _, copy := range copies {
		body, err := os.ReadFile(filepath.Join(fixture, copy.Original))
		if err != nil {
			return receipt, err
		}
		if len(body) < 16 || string(body[:16]) != "SQLite format 3\x00" {
			return receipt, fmt.Errorf("not the full SQLite fixture: %s", copy.Original)
		}
		path := filepath.Join(workspace, copy.Destination)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return receipt, err
		}
		if err := writeExclusive(path, body, 0400); err != nil {
			return receipt, err
		}
		receipt.SourceSHA[copy.Destination] = hash(body)
		oraclePath := filepath.Join(operator, strings.ReplaceAll(copy.Destination, "/", "_")+".oracle.json")
		if err := verifySQLiteOracle(ctx, path, copy.Destination, oraclePath); err != nil {
			return receipt, err
		}
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "captures"}, {"-c", "user.name=Codrax Eval", "-c", "user.email=eval@invalid", "commit", "-q", "-m", "constructed representative inputs"}} {
		if _, err := git(ctx, workspace, args...); err != nil {
			return receipt, err
		}
	}
	receipt.GitHEAD, err = git(ctx, workspace, "rev-parse", "HEAD")
	if err != nil {
		return receipt, err
	}
	if err := writeExclusive(filepath.Join(out, "question.txt"), []byte(question+"\n"), 0600); err != nil {
		return receipt, err
	}
	return receipt, writeJSON(filepath.Join(out, "preparation.json"), receipt)
}

func verifySQLiteOracle(ctx context.Context, path, rel, destination string) error {
	const countSQL = `SELECT t.tid AS tid, count(*) AS events, sum((SELECT count(*) FROM native_hook_frame f WHERE f.callchain_id=n.callchain_id)) AS frames FROM native_hook n JOIN thread t ON t.itid=n.itid WHERE n.start_ts >= 10000000000 AND n.start_ts < 10050000000 GROUP BY t.tid ORDER BY t.tid;`
	const rowSQL = `SELECT n.rowid AS source_row,n.id AS event_id,n.start_ts,n.end_ts,n.event_type,n.heap_size,n.addr,t.tid,n.callchain_id,f.rowid AS frame_row,f.depth,f.ip,f.symbol_id,f.file_id,f.offset,f.symbol_offset,f.vaddr FROM native_hook n JOIN thread t ON t.itid=n.itid LEFT JOIN native_hook_frame f ON f.callchain_id=n.callchain_id WHERE n.start_ts >= 10000000000 AND n.start_ts < 10050000000 ORDER BY t.tid,n.start_ts,n.rowid,f.rowid;`
	counts, err := exec.CommandContext(ctx, "sqlite3", "-readonly", "-json", path, countSQL).Output()
	if err != nil {
		return fmt.Errorf("independent SQLite counts: %w", err)
	}
	var rows []struct{ TID, Events, Frames int }
	if err := json.Unmarshal(counts, &rows); err != nil {
		return err
	}
	for _, want := range expectedQueries() {
		if want.Path != rel {
			continue
		}
		events, frames := 0, 0
		for _, row := range rows {
			if row.TID == want.TID {
				events, frames = row.Events, row.Frames
			}
		}
		if events != want.Events || frames != want.Frames {
			return fmt.Errorf("independent oracle differs: %s tid=%d", rel, want.TID)
		}
	}
	facts, err := exec.CommandContext(ctx, "sqlite3", "-readonly", "-json", path, rowSQL).Output()
	if err != nil || !json.Valid(facts) {
		return fmt.Errorf("independent SQLite rows unavailable: %v", err)
	}
	return writeJSON(destination, map[string]any{"counts": json.RawMessage(counts), "event_frame_rows": json.RawMessage(facts), "scope": "native_hook.start_ts in [10000000000,10050000000) ns; source frames are rows, not proven complete paths"})
}

type selectionDocument struct {
	Kind string `json:"kind"`
	tracecatalog.RepresentativeSnapshot
}

// This is a structural smoke audit, not serialized-evidence authentication.
// Actual tool selection, raw payload values and final prose require human audit.
func audit(ctx context.Context, input auditInput) error {
	if !filepath.IsAbs(input.Out) || !filepath.IsAbs(input.Index) || !filepath.IsAbs(input.Selection) || len(input.SelectionSHA) != 64 {
		return errors.New("audit requires absolute out/index/selection paths and the exact returned selection SHA256")
	}
	var seed preparation
	body, err := os.ReadFile(filepath.Join(input.Out, "preparation.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, &seed); err != nil {
		return err
	}
	out, err := filepath.EvalSymlinks(input.Out)
	if err != nil || seed.Workspace != filepath.Join(out, "workspace") || !reflect.DeepEqual(seed.Expected, expectedQueries()) || len(seed.SourceSHA) != len(copies) {
		return errors.New("operator receipt does not match this fixture workspace")
	}
	for _, copy := range copies {
		body, err := os.ReadFile(filepath.Join(seed.Workspace, copy.Destination))
		if err != nil || hash(body) != seed.SourceSHA[copy.Destination] {
			return fmt.Errorf("capture changed: %s", copy.Destination)
		}
	}
	head, err := git(ctx, seed.Workspace, "rev-parse", "HEAD")
	if err != nil || head != seed.GitHEAD {
		return errors.New("fixture HEAD changed")
	}
	current, err := tracecatalog.Load(ctx, input.Index)
	if err != nil {
		return err
	}
	body, err = os.ReadFile(input.Selection)
	if err != nil || hash(body) != input.SelectionSHA {
		return errors.New("selection snapshot missing or SHA256 differs from tool return")
	}
	var selection selectionDocument
	if err := json.Unmarshal(body, &selection); err != nil {
		return err
	}
	if err := auditSnapshots(seed, current, selection); err != nil {
		return err
	}
	return writeJSON(filepath.Join(out, "current-index-audit.json"), map[string]any{
		"status": "representatives_smoke_pass_human_pending", "current_index": input.Index,
		"selection_snapshot": input.Selection, "selection_snapshot_sha256": input.SelectionSHA,
		"catalog_id": current.ID, "selection_id": selection.SelectionID, "coverage": selection.Coverage,
		"current_queries": current.Queries, "excluded_query_ids": selection.ExcludedQueryIDs, "new_query_ids": selection.NewQueryIDs,
		"limits": "Navigation-only structural audit: verify actual model selection before queries, unchanged selection IDs across status, current native payloads, all final answer surfaces and permissions against original logs. No serialized field is authenticated as evidence by this audit.",
	})
}

func auditSnapshots(seed preparation, current tracecatalog.Snapshot, selection selectionDocument) error {
	if current.Root != filepath.Join(seed.Workspace, "captures") || len(current.Artifacts) != 4 || len(current.Queries) < 8 || len(current.History) != 0 || !current.Discovery.Complete || current.Discovery.Canceled || current.Discovery.Truncated {
		return errors.New("current catalog differs from the complete four-capture/eight-plan roster")
	}
	if selection.Kind != "query_specification_representatives/v1" || !selection.NavigationOnly || selection.CatalogID != current.ID || selection.SelectionID == "" || selection.Options.PerCluster != 2 || selection.Options.MaxClusters < 2 || len(selection.Clusters) != 2 || len(selection.Unmatched) != 0 || !reflect.DeepEqual(selection.Discovery, current.Discovery) {
		return errors.New("selection does not match this current complete two-group roster")
	}
	if selection.SelectionScope != "explicit_query_ids" && selection.SelectionScope != "all_current_queries" || selection.SelectionScope == "all_current_queries" && len(selection.ExcludedQueryIDs) != 0 {
		return errors.New("selection has an unknown or contradictory scope")
	}
	artifacts := map[string]tracecatalog.Artifact{}
	queries := map[string]tracecatalog.QueryRecord{}
	expectations := map[string]expectedQuery{}
	seenPaths, seenPairs := map[string]bool{}, map[string]bool{}
	attemptIDs, attemptSequences := map[string]bool{}, map[uint64]bool{}
	for _, want := range seed.Expected {
		expectations[filepath.Join(seed.Workspace, want.Path)+"#"+strconv.Itoa(want.TID)] = want
	}
	for _, artifact := range current.Artifacts {
		rel, err := filepath.Rel(seed.Workspace, artifact.Path)
		if err != nil || seed.SourceSHA[rel] == "" || seenPaths[artifact.Path] || artifact.ID == "" || artifacts[artifact.ID].ID != "" || artifact.RevisionID == "" {
			return errors.New("foreign, duplicate or unversioned artifact")
		}
		seenPaths[artifact.Path], artifacts[artifact.ID] = true, artifact
	}
	for _, query := range current.Queries {
		a, found := artifacts[query.ArtifactID]
		if !found || query.ID == "" || queries[query.ID].ID != "" || query.ArtifactRevision != a.RevisionID || !json.Valid(query.Parameters) {
			return errors.New("current query source/identity is mismatched or duplicated")
		}
		if err := auditAttemptHistory(query, attemptIDs, attemptSequences); err != nil {
			return err
		}
		for _, attempt := range query.Attempts {
			if (attempt.Outcome == tracecatalog.OutcomeSuccess || attempt.Outcome == tracecatalog.OutcomeEmpty) && attempt.SourceRevision != a.SourceRevision {
				return errors.New("successful query attempt belongs to a different source revision")
			}
		}
		queries[query.ID] = query
	}
	seenMembers, seenGroups, seenGroupIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	counts := tracecatalog.RepresentativeCoverage{Candidates: 4, Clusters: 2, SelectedClusters: 2,
		RepresentativeMemberCoverage: tracecatalog.RepresentativeMemberCoverage{Outcomes: map[tracecatalog.Outcome]int{}, SelectedOutcomes: map[tracecatalog.Outcome]int{}}}
	for _, group := range selection.Clusters {
		tid, err := strconv.Atoi(group.Specification.Object.ID)
		if err != nil || seenGroups[group.Specification.Object.ID] || group.ID == "" || seenGroupIDs[group.ID] || group.BudgetSkipped || len(group.Members) != 4 || !validSpec(group.Specification.Object, group.Specification.View, group.Specification.Window, group.Specification.Parameters, tid) {
			return errors.New("cluster specification or full membership differs")
		}
		seenGroups[group.Specification.Object.ID] = true
		seenGroupIDs[group.ID] = true
		groupCounts := tracecatalog.RepresentativeMemberCoverage{Outcomes: map[tracecatalog.Outcome]int{}, SelectedOutcomes: map[tracecatalog.Outcome]int{}}
		for _, member := range group.Members {
			q, exists := queries[member.QueryID]
			a := artifacts[q.ArtifactID]
			if !exists || seenMembers[member.QueryID] || q.Object != group.Specification.Object || q.View != group.Specification.View || !reflect.DeepEqual(q.Window, group.Specification.Window) || !jsonEqual(q.Parameters, group.Specification.Parameters) || member.ArtifactID != q.ArtifactID || member.ArtifactRevision != q.ArtifactRevision || member.Path != a.Path || member.SourceRevision != a.SourceRevision || member.SourceStatus != a.Status || member.Outcome != q.Outcome || member.Attempts != len(q.Attempts) {
				return errors.New("selection member is not the exact current source/query/state")
			}
			key := a.Path + "#" + q.Object.ID
			want, expected := expectations[key]
			if !expected || seenPairs[key] || !validSpec(q.Object, q.View, q.Window, q.Parameters, want.TID) {
				return errors.New("core task member source/selector/window is mismatched or duplicated")
			}
			seenPairs[key] = true
			seenMembers[member.QueryID] = true
			// Selection membership and execution are independent. An unselected
			// member may be queried normally; it never becomes a representative.
			if len(q.Attempts) == 0 {
				if member.LatestAttempt != nil {
					return errors.New("unexecuted member carries a fabricated latest attempt")
				}
			} else if member.LatestAttempt == nil || !sameAttempt(*member.LatestAttempt, q.Attempts[len(q.Attempts)-1]) {
				return errors.New("member latest attempt differs from its complete current history")
			}
			outcome := tracecatalog.OutcomeSuccess
			if want.Events == 0 {
				outcome = tracecatalog.OutcomeEmpty
			}
			for _, attempt := range q.Attempts {
				if attempt.Outcome == tracecatalog.OutcomeSuccess || attempt.Outcome == tracecatalog.OutcomeEmpty {
					if attempt.Outcome != outcome || attempt.SourceRevision == "" || attempt.SourceRevision != a.SourceRevision || attempt.PayloadRef == "" || !jsonEqual(q.Parameters, attempt.ExecutedParameters) {
						return errors.New("core effective query/source/payload does not match its exact plan")
					}
				} else if len(attempt.ExecutedParameters) > 0 && !jsonEqual(q.Parameters, attempt.ExecutedParameters) {
					return errors.New("core failed attempt changed the planned parameters")
				}
			}
			if member.Selected {
				if len(q.Attempts) == 0 || q.Outcome != outcome || member.SelectionReason != "soft_priority" {
					return errors.New("selected representative lacks a successful/empty latest current query")
				}
				if q.Attempts[len(q.Attempts)-1].Truncated {
					return errors.New("selected representative latest query is incomplete")
				}
			} else if member.SelectionReason != "per_cluster_budget" {
				return errors.New("unselected member lost its selection status")
			}
			for _, count := range []*tracecatalog.RepresentativeMemberCoverage{&counts.RepresentativeMemberCoverage, &groupCounts} {
				count.Members++
				count.Outcomes[q.Outcome]++
				if member.Selected {
					count.Selected++
					count.SelectedOutcomes[q.Outcome]++
				} else {
					count.BudgetNotSelected++
				}
			}
		}
		if groupCounts.Selected != 2 || !reflect.DeepEqual(groupCounts, group.Coverage) {
			return errors.New("cluster coverage does not equal its exact member states")
		}
	}
	// Reconcile every catalog query, rather than silently filtering extras to
	// make the core matrix pass. The two navigation sets must be disjoint from
	// the complete task selection and from each other, and contain real IDs.
	allIDs := map[string]bool{}
	for id := range seenMembers {
		allIDs[id] = true
	}
	for _, ids := range [][]string{selection.ExcludedQueryIDs, selection.NewQueryIDs} {
		for _, id := range ids {
			if queries[id].ID == "" || allIDs[id] {
				return errors.New("outside/new query roster is unknown, duplicated or overlaps task members")
			}
			allIDs[id] = true
		}
	}
	counts.Excluded, counts.NotInSelection = len(selection.ExcludedQueryIDs), len(selection.NewQueryIDs)
	if len(allIDs) != len(queries) {
		return errors.New("outside/new query roster omitted current work")
	}
	if len(seenMembers) != 8 || len(seenPairs) != 8 || counts.Selected != 4 || counts.BudgetNotSelected != 4 || !reflect.DeepEqual(counts, selection.Coverage) {
		return errors.New("selection totals do not preserve all eight members, four selected and four unselected")
	}
	return nil
}

// Navigation JSON cannot authenticate execution; Load already checks canonical
// IDs. This independent pass also checks the whole visible attempt history and
// current state, including extra queries, without treating failed work as zero.
func auditAttemptHistory(q tracecatalog.QueryRecord, ids map[string]bool, sequences map[uint64]bool) error {
	lastOutcome := tracecatalog.OutcomeNotExecuted
	var previous uint64
	for _, attempt := range q.Attempts {
		if attempt.ID == "" || ids[attempt.ID] || attempt.Sequence == 0 || sequences[attempt.Sequence] || attempt.Sequence <= previous {
			return errors.New("attempt identity/order is missing or duplicated")
		}
		ids[attempt.ID], sequences[attempt.Sequence], previous = true, true, attempt.Sequence
		switch attempt.Outcome {
		case tracecatalog.OutcomeSuccess, tracecatalog.OutcomeEmpty:
			if attempt.SourceRevision == "" || !json.Valid(attempt.ExecutedParameters) || attempt.Error != nil {
				return errors.New("successful attempt lacks source/parameters or carries a failure")
			}
		case tracecatalog.OutcomeFailure, tracecatalog.OutcomeCanceled, tracecatalog.OutcomeStale:
			if attempt.Error == nil || attempt.Error.Code == "" || len(attempt.ExecutedParameters) > 0 && !json.Valid(attempt.ExecutedParameters) {
				return errors.New("failed attempt lacks a recorded failure or has invalid parameters")
			}
		default:
			return errors.New("unknown attempt outcome")
		}
		lastOutcome = attempt.Outcome
	}
	if q.Outcome != lastOutcome {
		return errors.New("current query outcome differs from the latest actual attempt")
	}
	return nil
}

func validSpec(object tracecatalog.Object, view string, window *tracecatalog.Window, parameters json.RawMessage, tid int) bool {
	if (tid != 101 && tid != 202) || object != (tracecatalog.Object{Kind: "thread", ID: strconv.Itoa(tid)}) || view != "resource_stack" || window == nil || window.StartNS != 10_000_000_000 || window.EndNS != 10_050_000_000 || (window.EndInclusive != nil && *window.EndInclusive) {
		return false
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(parameters, &values) != nil || len(values) < 4 {
		return false
	}
	for key := range values {
		if key != "view" && key != "pid" && key != "time_start" && key != "time_end" && key != "target_scope" {
			return false
		}
	}
	var pid int
	var start, end float64
	var queryView, targetScope string
	if json.Unmarshal(values["pid"], &pid) != nil || json.Unmarshal(values["view"], &queryView) != nil || json.Unmarshal(values["time_start"], &start) != nil || json.Unmarshal(values["time_end"], &end) != nil {
		return false
	}
	if raw, ok := values["target_scope"]; ok && (json.Unmarshal(raw, &targetScope) != nil || targetScope != "thread") {
		return false
	}
	return pid == tid && queryView == view && start == 10 && end == 10.05
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	dx, dy := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	dx.UseNumber()
	dy.UseNumber()
	return json.Valid(a) && json.Valid(b) && dx.Decode(&x) == nil && dy.Decode(&y) == nil && reflect.DeepEqual(x, y)
}

func sameAttempt(a, b tracecatalog.Attempt) bool {
	if (len(a.ExecutedParameters) != 0 || len(b.ExecutedParameters) != 0) && !jsonEqual(a.ExecutedParameters, b.ExecutedParameters) {
		return false
	}
	// Indentation of an embedded RawMessage differs in the independently saved
	// catalog and selection documents. All non-JSON fields must still be exact.
	a.ExecutedParameters, b.ExecutedParameters = nil, nil
	return reflect.DeepEqual(a, b)
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fixture git: %w: %s", err, output)
	}
	return strings.TrimSpace(string(output)), nil
}

func hash(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeExclusive(path, append(body, '\n'), 0600)
}
func writeExclusive(path string, body []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(body)
	return errors.Join(writeErr, file.Close())
}

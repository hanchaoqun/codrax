// trace_catalog_resume prepares one bounded operator checkpoint. It never
// loads a provider, starts a model, or copies old results into the next run.
package main

import (
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

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

const question = "继续上次的采集检查，记录在 checkpoint.json。请逐份检查 captures 目录中 10.000 到 10.050 秒内线程 101 和 202 的内存分配、释放及调用位置，并列出本次各文件各线程的完成情况。只分析这些采集文件。"

type expectedQuery struct {
	Path   string `json:"relative_path"`
	TID    int    `json:"tid"`
	Events int    `json:"events"`
	Frames int    `json:"source_frames"`
}

var expected = []expectedQuery{
	{"captures/capture.data", 101, 3, 8},
	{"captures/capture.data", 202, 1, 3},
	{"captures/nested/capture.data", 101, 1, 3},
	{"captures/nested/capture.data", 202, 0, 0},
}

type preparation struct {
	Kind          string                       `json:"kind"`
	Workspace     string                       `json:"workspace"`
	Question      string                       `json:"question"`
	OperatorPID   int                          `json:"operator_pid"`
	ModelCalls    int                          `json:"model_calls"`
	Checkpoint    string                       `json:"checkpoint"`
	CheckpointSHA string                       `json:"checkpoint_sha256"`
	SourceSHA     map[string]string            `json:"source_sha256"`
	Expected      []expectedQuery              `json:"expected_current_queries"`
	SeedOutcomes  map[tracecatalog.Outcome]int `json:"seed_outcomes"`
	OldRawRefs    []string                     `json:"old_result_refs"`
	ExportTarget  string                       `json:"export_target,omitempty"`
}

func main() {
	out := flag.String("out", "", "new absolute evidence directory; parent must exist")
	fixture := flag.String("fixture", "eval/fixtures/hmosperf_catalog_objects", "existing offline constructed fixture directory")
	currentIndex := flag.String("audit-index", "", "optional exact current CLI catalog path; inspect structured status only, not model prose")
	exportTarget := flag.String("export-target", "", "optional explicit package name requested by the natural question; preparation does not export")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments")
		os.Exit(2)
	}
	var err error
	if *currentIndex != "" {
		err = audit(context.Background(), *out, *currentIndex)
	} else {
		_, err = prepareScenario(context.Background(), *out, *fixture, *exportTarget)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func prepare(ctx context.Context, out, fixture string) (preparation, error) {
	return prepareScenario(ctx, out, fixture, "")
}

func prepareScenario(ctx context.Context, out, fixture, exportTarget string) (preparation, error) {
	var receipt preparation
	if exportTarget != "" {
		for _, ch := range exportTarget {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
				return receipt, errors.New("-export-target must be a simple package name")
			}
		}
		if len(exportTarget) > 64 {
			return receipt, errors.New("-export-target exceeds 64 bytes")
		}
	}
	if !filepath.IsAbs(out) {
		return receipt, errors.New("-out must be a new absolute directory")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		return receipt, err // Never reuse, overwrite or clean an earlier run.
	}
	out, err := filepath.EvalSymlinks(out)
	if err != nil {
		return receipt, err
	}
	workspace := filepath.Join(out, "workspace")
	operator := filepath.Join(out, "operator")
	for _, dir := range []string{filepath.Join(workspace, "captures", "nested"), operator, filepath.Join(out, "logs")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return receipt, err
		}
	}
	receipt = preparation{Kind: "constructed_sqlite_operator_checkpoint_not_natural_interruption", Workspace: workspace,
		Question: question, OperatorPID: os.Getpid(), SourceSHA: map[string]string{}, Expected: expected,
		SeedOutcomes: map[tracecatalog.Outcome]int{}, Checkpoint: filepath.Join(workspace, "checkpoint.json"), ExportTarget: exportTarget}
	if exportTarget != "" {
		receipt.Question += " 并将本次检查的证据包导出为 " + exportTarget + "。"
	}
	for _, rel := range []string{"captures/capture.data", "captures/nested/capture.data"} {
		body, err := os.ReadFile(filepath.Join(fixture, rel))
		if err != nil {
			return receipt, err
		}
		if len(body) < 16 || string(body[:16]) != "SQLite format 3\x00" {
			return receipt, fmt.Errorf("fixture is not the expected full SQLite input: %s", rel)
		}
		if err := os.WriteFile(filepath.Join(workspace, rel), body, 0400); err != nil {
			return receipt, err
		}
		receipt.SourceSHA[rel] = hash(body)
		// Independent SQLite oracle: no trace converter/query implementation.
		if err := verifySQLiteOracle(ctx, filepath.Join(workspace, rel), rel, filepath.Join(operator, strings.ReplaceAll(rel, "/", "_")+".oracle.json")); err != nil {
			return receipt, err
		}
	}
	bus := &types.BusContext{Ctx: ctx, RepoRoot: workspace, WorkDir: operator, Mutable: types.NewMutableState(question),
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(operator, "prepared")})}
	discover, err := (&tool.TraceCatalog{}).Execute(bus, json.RawMessage(`{"root":"captures","queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05},{"view":"resource_stack","pid":202,"time_start":10,"time_end":10.05}]}`))
	if err != nil || !discover.Success {
		return receipt, fmt.Errorf("public discover: %v %s", err, discover.Summary)
	}
	if err := writeJSON(filepath.Join(operator, "discover.json"), discover); err != nil {
		return receipt, err
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": filepath.Join(workspace, "captures", "capture.data"), "view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05})
	result, err := (&tool.TraceQuery{}).Execute(bus, params)
	if err != nil || !result.Success {
		return receipt, fmt.Errorf("operator native query: %v %s", err, result.Summary)
	}
	valid := false
	for _, observation := range result.Observations {
		if p, ok := tool.DecodeTraceResourceStack(observation); ok {
			frames := 0
			for _, event := range p.Events {
				frames += len(event.Frames)
			}
			valid = p.TargetPID == 101 && p.MatchedEvents == 3 && p.OmittedEvents == 0 && frames == 8 && p.Window.StartTs == 10 && p.Window.EndTs == 10.05 && !p.Window.EndInclusive
		}
	}
	if !valid {
		return receipt, errors.New("operator query did not produce the exact 3-event/8-frame typed seed")
	}
	if err := writeJSON(filepath.Join(operator, "seed-tool-result.json"), result); err != nil {
		return receipt, err
	}
	catalogs := bus.Mutable.TraceCatalogs()
	if len(catalogs) != 1 {
		return receipt, errors.New("operator expected exactly one public catalog")
	}
	catalog := catalogs[0]
	snapshot := catalog.Snapshot()
	if len(snapshot.Artifacts) != 2 || len(snapshot.Queries) != 4 || !snapshot.Discovery.Complete {
		return receipt, errors.New("operator roster is incomplete")
	}
	for _, query := range snapshot.Queries {
		receipt.SeedOutcomes[query.Outcome]++
		for _, attempt := range query.Attempts {
			receipt.OldRawRefs = append(receipt.OldRawRefs, attempt.RawRef, attempt.PayloadRef)
		}
	}
	if receipt.SeedOutcomes[tracecatalog.OutcomeSuccess] != 1 || receipt.SeedOutcomes[tracecatalog.OutcomeNotExecuted] != 3 {
		return receipt, errors.New("seed must contain one actual success and three unexecuted queries")
	}
	if err := catalog.Save(ctx, receipt.Checkpoint); err != nil {
		return receipt, err
	}
	checkpoint, err := os.ReadFile(receipt.Checkpoint)
	if err != nil {
		return receipt, err
	}
	receipt.CheckpointSHA = hash(checkpoint)
	if err := os.WriteFile(filepath.Join(out, "question.txt"), []byte(receipt.Question+"\n"), 0600); err != nil {
		return receipt, err
	}
	if err := writeJSON(filepath.Join(out, "preparation.json"), receipt); err != nil {
		return receipt, err
	}
	// Root stays identical between the operator and actual CLI; do not run this
	// fixture through run.sh's copy-to-scratch path.
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "captures", "checkpoint.json"}, {"-c", "user.name=Codrax Eval", "-c", "user.email=eval@invalid", "commit", "-q", "-m", "operator checkpoint fixture"}} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = workspace
		if output, err := cmd.CombinedOutput(); err != nil {
			return receipt, fmt.Errorf("fixture git: %w: %s", err, output)
		}
	}
	return receipt, nil
}

func verifySQLiteOracle(ctx context.Context, path, rel, destination string) error {
	const query = `SELECT t.tid AS tid, count(*) AS events, sum((SELECT count(*) FROM native_hook_frame f WHERE f.callchain_id=n.callchain_id)) AS frames FROM native_hook n JOIN thread t ON t.itid=n.itid WHERE n.start_ts >= 10000000000 AND n.start_ts < 10050000000 GROUP BY t.tid ORDER BY t.tid;`
	output, err := exec.CommandContext(ctx, "sqlite3", "-readonly", "-json", path, query).Output()
	if err != nil {
		return fmt.Errorf("independent SQLite oracle: %w", err)
	}
	var rows []struct{ TID, Events, Frames int }
	if err := json.Unmarshal(output, &rows); err != nil {
		return err
	}
	for _, want := range expected {
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
			return fmt.Errorf("independent oracle differs: %s TID %d", rel, want.TID)
		}
	}
	return os.WriteFile(destination, output, 0600)
}

// audit inspects an explicitly selected product index. It never elects an
// index by mtime, reads model prose, executes a query, or grants evidence.
func audit(ctx context.Context, out, index string) error {
	var seed preparation
	body, err := os.ReadFile(filepath.Join(out, "preparation.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, &seed); err != nil {
		return err
	}
	if !filepath.IsAbs(index) {
		return errors.New("audit-index must be an exact absolute current catalog path")
	}
	checkpoint, err := os.ReadFile(seed.Checkpoint)
	if err != nil || hash(checkpoint) != seed.CheckpointSHA {
		return errors.New("operator checkpoint changed")
	}
	for rel, digest := range seed.SourceSHA {
		body, err := os.ReadFile(filepath.Join(seed.Workspace, rel))
		if err != nil || hash(body) != digest {
			return fmt.Errorf("capture changed: %s", rel)
		}
	}
	current, err := tracecatalog.Load(ctx, index)
	if err != nil {
		return err
	}
	if current.Root != filepath.Join(seed.Workspace, "captures") || len(current.Artifacts) != 2 || len(current.Queries) != 4 {
		return errors.New("current catalog differs from the requested input roster")
	}
	paths := map[string]string{}
	for _, artifact := range current.Artifacts {
		paths[artifact.ID] = artifact.Path
	}
	seen := map[string]bool{}
	for _, query := range current.Queries {
		key := paths[query.ArtifactID] + "#" + query.Object.ID
		if seen[key] {
			return errors.New("duplicate current member")
		}
		seen[key] = true
		var want *expectedQuery
		for i := range seed.Expected {
			candidate := &seed.Expected[i]
			if key == filepath.Join(seed.Workspace, candidate.Path)+"#"+strconv.Itoa(candidate.TID) {
				want = candidate
				break
			}
		}
		if want == nil || query.Object.Kind != "thread" || query.View != "resource_stack" || query.Window == nil || query.Window.StartNS != 10_000_000_000 || query.Window.EndNS != 10_050_000_000 || len(query.Attempts) == 0 {
			return errors.New("current member selector/window/attempt is missing or mismatched")
		}
		outcome := tracecatalog.OutcomeSuccess
		if want.Events == 0 {
			outcome = tracecatalog.OutcomeEmpty
		}
		last := query.Attempts[len(query.Attempts)-1]
		if query.Outcome != outcome || last.Outcome != outcome || last.SourceRevision == "" || last.PayloadRef == "" || last.Truncated {
			return fmt.Errorf("current member is not freshly complete: %s", key)
		}
		var declared, executed map[string]any
		if json.Unmarshal(query.Parameters, &declared) != nil || json.Unmarshal(last.ExecutedParameters, &executed) != nil || !reflect.DeepEqual(declared, executed) {
			return fmt.Errorf("effective query differs from the bounded explicit plan: %s", key)
		}
		for _, old := range seed.OldRawRefs {
			if old != "" && (old == last.PayloadRef || old == last.RawRef) {
				return errors.New("current member reused an operator historical result reference")
			}
		}
	}
	return writeJSON(filepath.Join(out, "current-index-audit.json"), map[string]any{
		"status": "manifest_smoke_pass_human_pending", "current_index": index, "snapshot": current,
		"limits": "Navigation only: actual model selection, original payloads, complete answer and source/window authority require independent log/payload/human audit.",
	})
}

func hash(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(body, '\n'))
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

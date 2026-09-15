package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Wire input lets the public persistence/context RED execute against the old
// implementation: an unknown additive field is dropped, not a compile error.
func b1702ExecutionReport(t *testing.T) *ChangeReport {
	t.Helper()
	var report ChangeReport
	const wire = `{"plan_id":"p-current","passed":true,"test_results":[{"assertion_id":"native-pass","passed":true}],"executed_commands":[{"runner":"verification_probe","outcome":"unavailable","probe_execution":{"version":1,"definition_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","invocation_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","execution_id":"execution-current","started_at":"2026-09-15T10:00:00Z","finished_at":"2026-09-15T10:00:01Z","repository_root":"/repo","executable":"/bin/node","args":["probe.js"],"working_dir":"/repo"}}],"verification_diagnostics":[{"source":"pre_suite_verification_probe","category":"probe_execution","reason_code":"verification_probe_runner_unavailable","runner":"verification_probe","outcome":"unavailable","probe_execution_observations":[{"plan_id":"p-current","probe_id":"typescript-check","execution_id":"execution-current","definition_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","invocation_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","output_excerpt":"SyntaxError: Unexpected token :\n  exact  spacing","output_ref":"/outputs/current  probe.txt"}]}]}`
	if err := json.Unmarshal([]byte(wire), &report); err != nil {
		t.Fatal(err)
	}
	return &report
}

func TestB1702ExecutionObservationPublicContextAndFinalReport(t *testing.T) {
	report := b1702ExecutionReport(t)
	pack := WriteContextPackFromChangeReport(report)
	var body string
	for _, item := range pack.Items {
		if item.Kind == "verification_probe_execution_observation" {
			body = item.Text
		}
	}
	for _, want := range []string{"SyntaxError: Unexpected token :", "exact  spacing", "/outputs/current  probe.txt", "execution-current"} {
		if !strings.Contains(body, want) {
			t.Errorf("passed native suite lost independent probe context %q: %s", want, body)
		}
	}
	final := BuildWriteFinalReport(WriteFinalReportInput{Report: report})
	encoded, err := json.Marshal(final)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"probe_execution_observations"`, `"output_excerpt":"SyntaxError: Unexpected token :\n  exact  spacing"`, `"output_ref":"/outputs/current  probe.txt"`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("durable final summary lost exact probe context %s", want)
		}
	}
	if !report.Passed || !final.Verification.Passed || len(report.TestResults) != 1 || !report.TestResults[0].Passed || report.FailureKind != "" {
		t.Fatal("non-authoritative execution output changed verification verdict")
	}
}

func TestB1702ExecutionObservationPublicHandoffPersistence(t *testing.T) {
	report := b1702ExecutionReport(t)
	report.Passed = false
	report.FailureKind = FailureKindTestsFailed
	handoff := BuildVerifyFailureHandoff(report, "batch", 1, "", "")
	encoded, err := json.Marshal(handoff)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"probe_execution_observations"`) || !strings.Contains(string(encoded), `"execution_id":"execution-current"`) {
		t.Fatalf("failure handoff dropped independent probe output: %s", encoded)
	}
}

func TestB1702ExecutionObservationExactMergeAndReceiptBinding(t *testing.T) {
	report := b1702ExecutionReport(t)
	base := report.VerificationDiagnostics[0].ProbeExecutionObservations[0]
	want := []VerificationProbeExecutionObservation{base}
	// Every field belongs to the exact tuple: no whitespace/case/path folding.
	for _, field := range []string{"PlanID", "ProbeID", "ExecutionID", "DefinitionSHA256", "InvocationSHA256", "OutputExcerpt", "OutputRef"} {
		variant := base
		value := reflect.ValueOf(&variant).Elem().FieldByName(field)
		value.SetString(value.String() + " ")
		want = append(want, variant)
	}
	got := MergeVerificationProbeExecutionObservations(want, []VerificationProbeExecutionObservation{base})
	if !reflect.DeepEqual(got, want) || MergeVerificationProbeExecutionObservations() != nil {
		t.Fatal("exact tuple/order changed or empty merge fabricated context")
	}
	got[0].OutputExcerpt = "changed"
	if want[0] != base {
		t.Fatal("merge aliases its input")
	}
	negatives := map[string]func(*ChangeReport){
		"empty_plan":   func(r *ChangeReport) { r.PlanID = "" },
		"foreign_plan": func(r *ChangeReport) { r.PlanID = "p-other" },
		"empty_probe":  func(r *ChangeReport) { r.VerificationDiagnostics[0].ProbeExecutionObservations[0].ProbeID = "" },
		"foreign_execution": func(r *ChangeReport) {
			r.VerificationDiagnostics[0].ProbeExecutionObservations[0].ExecutionID = "other"
		},
		"foreign_definition": func(r *ChangeReport) {
			r.VerificationDiagnostics[0].ProbeExecutionObservations[0].DefinitionSHA256 = strings.Repeat("c", 64)
		},
		"foreign_invocation": func(r *ChangeReport) {
			r.VerificationDiagnostics[0].ProbeExecutionObservations[0].InvocationSHA256 = strings.Repeat("c", 64)
		},
		"no_command":           func(r *ChangeReport) { r.ExecutedCommands = nil },
		"no_receipt":           func(r *ChangeReport) { r.ExecutedCommands[0].ProbeExecution = nil },
		"command_runner":       func(r *ChangeReport) { r.ExecutedCommands[0].Runner = "node" },
		"diagnostic_runner":    func(r *ChangeReport) { r.VerificationDiagnostics[0].Runner = "node" },
		"receipt_version":      func(r *ChangeReport) { r.ExecutedCommands[0].ProbeExecution.Version++ },
		"receipt_not_started":  func(r *ChangeReport) { r.ExecutedCommands[0].ProbeExecution.StartedAt = time.Time{} },
		"receipt_not_finished": func(r *ChangeReport) { r.ExecutedCommands[0].ProbeExecution.FinishedAt = time.Time{} },
		"receipt_root":         func(r *ChangeReport) { r.ExecutedCommands[0].ProbeExecution.RepositoryRoot = "" },
		"receipt_executable":   func(r *ChangeReport) { r.ExecutedCommands[0].ProbeExecution.Executable = "" },
		"receipt_argv":         func(r *ChangeReport) { r.ExecutedCommands[0].ProbeExecution.Args = nil },
		"receipt_cwd":          func(r *ChangeReport) { r.ExecutedCommands[0].ProbeExecution.WorkingDir = "" },
		"malformed_shared_digest": func(r *ChangeReport) {
			r.ExecutedCommands[0].ProbeExecution.DefinitionSHA256 = "not-a-digest"
			r.VerificationDiagnostics[0].ProbeExecutionObservations[0].DefinitionSHA256 = "not-a-digest"
		},
		"receipt_fields_split_across_commands": func(r *ChangeReport) {
			second := r.ExecutedCommands[0]
			receipt := *second.ProbeExecution
			second.ProbeExecution = &receipt
			r.ExecutedCommands[0].ProbeExecution.InvocationSHA256 = strings.Repeat("c", 64)
			second.ProbeExecution.DefinitionSHA256 = strings.Repeat("d", 64)
			r.ExecutedCommands = append(r.ExecutedCommands, second)
		},
	}
	for name, mutate := range negatives {
		t.Run(name, func(t *testing.T) {
			r := b1702ExecutionReport(t)
			mutate(r)
			if got := CurrentReportProbeExecutionObservations(r); len(got) != 0 {
				t.Fatalf("invalid current execution binding gained display: %+v", got)
			}
			if got := BuildWriteFinalReport(WriteFinalReportInput{Report: r}).Verification.ProbeExecutionObservations; len(got) != 0 {
				t.Fatal("durable final projection bypassed receipt binding")
			}
		})
	}
	// Two same-definition invocations in one merged diagnostic stay distinct.
	second := report.ExecutedCommands[0]
	receipt := *second.ProbeExecution
	receipt.ExecutionID = "execution-second"
	second.ProbeExecution = &receipt
	report.ExecutedCommands = append(report.ExecutedCommands, second)
	next := base
	next.ExecutionID, next.OutputExcerpt, next.OutputRef = receipt.ExecutionID, "second exact error", "/outputs/second.txt"
	report.VerificationDiagnostics[0].ProbeExecutionObservations = []VerificationProbeExecutionObservation{base, next, base}
	for _, passed := range []bool{true, false} {
		report.Passed = passed
		if got := CurrentReportProbeExecutionObservations(report); !reflect.DeepEqual(got, []VerificationProbeExecutionObservation{base, next}) {
			t.Fatalf("passed=%v lost distinct actual executions: %+v", passed, got)
		}
	}
	// The output carrier is not borrowed from the comparator failure taxonomy.
	// Valid executed output remains informational even if the diagnostic is
	// merged under a different category/reason or the suite itself passes.
	report.VerificationDiagnostics[0].Category = "probe_authoring"
	report.VerificationDiagnostics[0].ReasonCode = "independent_reason"
	report.VerificationDiagnostics[0].Outcome = "passed"
	if got := CurrentReportProbeExecutionObservations(report); !reflect.DeepEqual(got, []VerificationProbeExecutionObservation{base, next}) {
		t.Fatal("collector conflated output persistence with diagnostic verdict")
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var restored ChangeReport
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(CurrentReportProbeExecutionObservations(&restored), []VerificationProbeExecutionObservation{base, next}) {
		t.Fatal("report JSON replay lost distinct execution identity/output")
	}
	if CurrentReportProbeExecutionObservations(nil) != nil {
		t.Fatal("nil report fabricated current execution")
	}
}

func TestB1702ExecutionObservationAtomicScopeBudgetAndReplay(t *testing.T) {
	report := b1702ExecutionReport(t)
	observation := &report.VerificationDiagnostics[0].ProbeExecutionObservations[0]
	observation.OutputExcerpt = "HEAD-EXACT\n" + strings.Repeat("frame\n", 1000) + "TAIL-EXACT"
	observation.OutputRef = "/" + strings.Repeat("p", 4091) + ".txt"
	pack := WriteContextPackFromChangeReport(report).WithScope("batch", "slice")
	var atom WriteContextItem
	for _, item := range pack.Items {
		if item.Kind == "verification_probe_execution_observation" {
			atom = item
		}
	}
	if atom.ID == "" || atom.SourceID != report.PlanID || atom.SourceStage != "verify" || atom.BatchID != "batch" || atom.SliceID != "slice" {
		t.Fatalf("missing scoped atomic item: %+v", atom)
	}
	crowded := WriteContextPack{PackID: "crowded", BatchID: "batch"}
	for i := 0; i < writeContextPackMaxItems; i++ {
		crowded.Items = append(crowded.Items, WriteContextItem{ID: fmt.Sprint(i), Kind: "background", Priority: WriteContextP3, Text: "background"})
	}
	crowded.Items = append(crowded.Items, atom)
	crowded = NormalizeWriteContextPack(crowded)
	if len(crowded.Items) != writeContextPackMaxItems {
		t.Fatal("changed pack item cap")
	}
	for _, consumer := range []WriteContextConsumer{WriteConsumerPlanner, WriteConsumerController, WriteConsumerVerifier} {
		for _, scope := range []struct {
			batch, slice string
			want         bool
		}{{"batch", "slice", true}, {"other", "slice", false}, {"batch", "other", false}} {
			var body string
			for _, item := range crowded.ViewForScope(consumer, 100, scope.batch, scope.slice).Items {
				if item.Kind == atom.Kind {
					body = item.Text
				}
			}
			if (body != "") != scope.want {
				t.Fatalf("consumer=%s scope=%+v crossed/lost context", consumer, scope)
			}
			if scope.want {
				for _, exact := range []string{"HEAD-EXACT", "TAIL-EXACT", strconv.Quote(observation.OutputRef)} {
					if !strings.Contains(body, exact) {
						t.Fatalf("atomic display lost %q", exact)
					}
				}
			}
		}
	}
	next := b1702ExecutionReport(t)
	next.VerificationDiagnostics[0].ProbeExecutionObservations[0].OutputExcerpt = "new exact observation"
	latest := WriteContextPackFromChangeReport(next).WithScope("batch", "slice")
	merged := MergeWriteContextPacks("batch", "", pack, latest, latest)
	count := 0
	for _, item := range merged.Items {
		if item.Kind == atom.Kind {
			count++
			if !strings.Contains(item.Text, "new exact observation") || strings.Contains(item.Text, "HEAD-EXACT") {
				t.Fatal("same-plan replay mixed generations")
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one current atomic group, got %d", count)
	}
	empty := &ChangeReport{PlanID: report.PlanID, Passed: true}
	if len(CurrentReportProbeExecutionObservations(empty)) != 0 || len(verificationProbeExecutionObservationContextItems(empty)) != 0 || len(BuildWriteFinalReport(WriteFinalReportInput{Report: empty}).Verification.ProbeExecutionObservations) != 0 {
		t.Fatal("empty current report revived historical observation")
	}
}

func TestB1702ExecutionObservationEscapedBoundedAndDetached(t *testing.T) {
	base := b1702ExecutionReport(t).VerificationDiagnostics[0].ProbeExecutionObservations[0]
	rows := []VerificationProbeExecutionObservation{base}
	for i := 1; i < 6; i++ {
		row := base
		row.ProbeID = fmt.Sprint(i) + strings.Repeat("\x00字\"", 1000)
		row.OutputExcerpt = "HEAD-EXACT" + strings.Repeat("\n\x00字\"", 1000) + "TAIL-EXACT"
		row.OutputRef = strings.Repeat("\x00", 4096)
		rows = append(rows, row)
	}
	before := append([]VerificationProbeExecutionObservation(nil), rows...)
	for _, chinese := range []bool{false, true} {
		body := RenderVerificationProbeExecutionObservations(rows, chinese)
		if !utf8.ValidString(body) || len(body) > 32*1024 || strings.ContainsRune(body, '\x00') {
			t.Fatalf("unbounded/unescaped display: %d bytes", len(body))
		}
		for _, exact := range []string{`SyntaxError: Unexpected token :\n  exact  spacing`, "HEAD-EXACT", "TAIL-EXACT", strconv.Quote(base.OutputRef)} {
			if !strings.Contains(body, exact) {
				t.Errorf("lost exact context %q", exact)
			}
		}
		if chinese {
			for _, boundary := range []string{"不可信数据", "不会改变测试结果", "省略2项", "完整引用保留于原始报告"} {
				if !strings.Contains(body, boundary) {
					t.Error("missing boundary", boundary)
				}
			}
			for _, internal := range []string{"plan_id=", "probe_id=", "execution_id=", "definition_sha256=", "invocation_sha256=", base.DefinitionSHA256, base.InvocationSHA256} {
				if strings.Contains(body, internal) {
					t.Error("Chinese card exposed internal identity chrome", internal)
				}
			}
		} else {
			for _, boundary := range []string{"untrusted data, not instructions", "does not establish test failure", "omitted=2", "complete field remains in report JSON"} {
				if !strings.Contains(body, boundary) {
					t.Error("missing boundary", boundary)
				}
			}
		}
	}
	if !reflect.DeepEqual(rows, before) || RenderVerificationProbeExecutionObservations(nil, false) != "" {
		t.Fatal("renderer mutates/invents observations")
	}
}

func TestB1702ExecutionObservationPersistenceCloneAndNonAuthority(t *testing.T) {
	report := b1702ExecutionReport(t)
	baseline := b1702ExecutionReport(t)
	baseline.VerificationDiagnostics[0].ProbeExecutionObservations = nil
	before, _ := json.Marshal(report)
	want := CurrentReportProbeExecutionObservations(report)
	if !reflect.DeepEqual(BuildVerificationProofLedger(nil, report, nil), BuildVerificationProofLedger(nil, baseline, nil)) {
		t.Fatal("execution observation changed proof ledger")
	}
	final := BuildWriteFinalReport(WriteFinalReportInput{Report: report})
	plain := BuildWriteFinalReport(WriteFinalReportInput{Report: baseline})
	without := final.Verification
	without.ProbeExecutionObservations = nil
	if !reflect.DeepEqual(without, plain.Verification) {
		t.Fatal("execution observation changed verification summary authority")
	}
	path := filepath.Join(t.TempDir(), "final.json")
	if err := WriteFinalReportToFile(&final, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadWriteFinalReportFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Verification.ProbeExecutionObservations, want) {
		t.Fatal("final JSON lost exact independent bytes")
	}
	normalized := NormalizeWriteFinalReport(final)
	normalized.Verification.ProbeExecutionObservations[0].OutputExcerpt = "mutated normalized copy"
	if !reflect.DeepEqual(final.Verification.ProbeExecutionObservations, want) {
		t.Fatal("normalizer aliases original final report")
	}
	final.Verification.ProbeExecutionObservations[0].OutputExcerpt = "mutated final copy"
	after, _ := json.Marshal(report)
	if !bytes.Equal(before, after) {
		t.Fatal("final projection mutated raw report")
	}
	report.Passed, baseline.Passed = false, false
	report.FailureKind, baseline.FailureKind = FailureKindTestsFailed, FailureKindTestsFailed
	handoff := BuildVerifyFailureHandoff(report, "batch", 1, "", "")
	plainHandoff := BuildVerifyFailureHandoff(baseline, "batch", 1, "", "")
	if handoff.FailureAuthority != plainHandoff.FailureAuthority || handoff.FailureKind != plainHandoff.FailureKind || handoff.FailureAuthorityReasonCode != plainHandoff.FailureAuthorityReasonCode {
		t.Fatal("observation changed handoff failure authority")
	}
	handoff.Diagnostics[0].ProbeExecutionObservations[0].OutputExcerpt = "mutated handoff"
	if !reflect.DeepEqual(CurrentReportProbeExecutionObservations(report), want) {
		t.Fatal("handoff aliases original diagnostic")
	}
	legacy, _ := json.Marshal(VerificationDiagnostic{})
	if strings.Contains(string(legacy), "probe_execution_observations") {
		t.Fatal("optional field changed legacy JSON")
	}
}

func TestB1702ExecutionObservationPreservesShortEncodedOutput(t *testing.T) {
	for _, encodedBytes := range []int{500, 507, 750, 999, 1000} {
		t.Run(fmt.Sprint(encodedBytes), func(t *testing.T) {
			// A short stack can put its identifying message in the middle. The
			// policy is a generic byte budget, not a search for error keywords.
			raw := strings.Repeat("a", 180) + " exact middle context " + strings.Repeat("z", encodedBytes-2-180-len(" exact middle context "))
			if len(strconv.Quote(raw)) != encodedBytes {
				t.Fatal("invalid encoded-length fixture")
			}
			for _, chinese := range []bool{false, true} {
				body := RenderVerificationProbeExecutionObservations([]VerificationProbeExecutionObservation{{OutputExcerpt: raw}}, chinese)
				if !strings.Contains(body, strconv.Quote(raw)) {
					t.Errorf("%d-byte short output was truncated (Chinese=%v)", encodedBytes, chinese)
				}
			}
		})
	}
}

func TestB1702ExecutionObservationOutputUsesEncodedByteBudget(t *testing.T) {
	for _, unit := range []string{"界", "😀", "\x00\n\"", "a"} {
		for _, chinese := range []bool{false, true} {
			raw := "HEAD-EXACT" + strings.Repeat(unit, 1200) + "TAIL-EXACT"
			var rows []VerificationProbeExecutionObservation
			for i := 0; i < 4; i++ {
				rows = append(rows, VerificationProbeExecutionObservation{PlanID: strings.Repeat("😀", 500), ProbeID: fmt.Sprint(i) + strings.Repeat("😀", 500), ExecutionID: strings.Repeat("😀", 500), DefinitionSHA256: strings.Repeat("😀", 500), InvocationSHA256: strings.Repeat("😀", 500), OutputExcerpt: raw, OutputRef: "/" + strings.Repeat("p", 4095)})
			}
			body := RenderVerificationProbeExecutionObservations(rows, chinese)
			if !utf8.ValidString(body) || len(body) > 32*1024 {
				t.Fatalf("atomic group exceeded byte budget: %d", len(body))
			}
			prefix := "output_excerpt="
			if chinese {
				prefix = "输出摘录="
			}
			count := 0
			for _, line := range strings.Split(body, "\n") {
				if !strings.HasPrefix(line, prefix) {
					continue
				}
				count++
				display := strings.TrimPrefix(line, prefix)
				if len(display) > 1000 || !strings.Contains(display, "HEAD-EXACT") || !strings.Contains(display, "TAIL-EXACT") || strings.ContainsRune(display, '\x00') {
					t.Errorf("output is not escaped, positional and <=1000 encoded bytes: unit=%q Chinese=%v len=%d", unit, chinese, len(display))
				}
			}
			if count != 4 {
				t.Fatalf("missing atomic output rows: %d", count)
			}
		}
	}
}

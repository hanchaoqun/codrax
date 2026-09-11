package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestB1661NormalizedContextPreservesDistinctReceiptBytes(t *testing.T) {
	first := VerificationDiagnostic{
		Source: "junit_invocation_report", Category: "report_binding", ReasonCode: "junit_current_report_bytes", Severity: "info",
		Runner: "java", Framework: "junit", WorkingDir: "module", Command: "mvn test -Dsurefire.reportNameSuffix=current",
		Detail: `report_path="/reports/Case.xml" sha256=0123 reporting_nonce="current"; this receipt records parsed bytes, not additional assertion coverage`,
	}
	second := first
	second.Detail = strings.ReplaceAll(first.Detail, "Case.xml", "case.xml")
	duplicate := first
	duplicate.Detail = " \n" + first.Detail + "\t"
	report := &ChangeReport{Passed: true, VerificationDiagnostics: []VerificationDiagnostic{first, second, duplicate}}
	before, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var restored ChangeReport
	if err := json.Unmarshal(before, &restored); err != nil {
		t.Fatal(err)
	}
	pack := NormalizeWriteContextPack(WriteContextPackFromChangeReport(&restored))
	if len(pack.Items) != 2 {
		t.Errorf("case-distinct source receipts collapsed (or genuine duplicate retained): got %d items %+v", len(pack.Items), pack.Items)
	}
	for _, consumer := range []WriteContextConsumer{WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier} {
		view := pack.View(consumer, 100)
		if len(view.Items) != 2 {
			t.Errorf("%s normalized view lost source identity: %+v", consumer, view.Items)
		}
	}
	// The normal context ID cap still applies; long commands must not truncate
	// away the discriminator and collapse two source files again.
	for i := range restored.VerificationDiagnostics {
		restored.VerificationDiagnostics[i].Command += strings.Repeat(" --long-option=value", 100)
	}
	longPack := NormalizeWriteContextPack(WriteContextPackFromChangeReport(&restored))
	if len(longPack.Items) != 2 {
		t.Errorf("bounded long command erased receipt identity: %+v", longPack.Items)
	}
	for _, consumer := range []WriteContextConsumer{WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier} {
		if view := longPack.View(consumer, 100); len(view.Items) != 2 {
			t.Errorf("%s bounded long-command view lost receipt identity: %+v", consumer, view.Items)
		}
	}
	if restored.VerificationDiagnostics[0].Detail != first.Detail || restored.VerificationDiagnostics[1].Detail != second.Detail || restored.VerificationDiagnostics[2].Detail != duplicate.Detail {
		t.Error("bounded context changed complete diagnostic detail in the report")
	}
	after, err := json.Marshal(report)
	if err != nil || !bytes.Equal(before, after) {
		t.Error("context projection changed the source report")
	}
}

func TestB1661ReceiptIdentityOnlyHashesExistingDetail(t *testing.T) {
	diag := VerificationDiagnostic{Source: "junit_invocation_report", Category: "report_binding", ReasonCode: "junit_current_report_bytes"}
	if got := VerificationDiagnosticReceiptIdentity(diag); got != "" {
		t.Fatalf("missing detail invented a receipt identity: %q", got)
	}
	diag.Detail = " \nNOT parsed or validated: /Case.xml\t"
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(diag.Detail))))
	if got := VerificationDiagnosticReceiptIdentity(diag); got != want || len(got) != 64 {
		t.Fatalf("identity must preserve exact inner bytes in a full stable hash: %q want %q", got, want)
	}
	for _, change := range []func(*VerificationDiagnostic){
		func(d *VerificationDiagnostic) { d.Source = "other" },
		func(d *VerificationDiagnostic) { d.Category = "other" },
		func(d *VerificationDiagnostic) { d.ReasonCode = "other" },
	} {
		copy := diag
		change(&copy)
		if got := VerificationDiagnosticReceiptIdentity(copy); got != "" {
			t.Errorf("unrelated diagnostic got a new identity: %+v => %s", copy, got)
		}
	}
}

func TestB1661OtherDiagnosticContextIdentityStaysUnchanged(t *testing.T) {
	for _, tc := range []struct{ source, category, reason string }{
		{"verification_probe", "report_binding", "junit_current_report_bytes"},
		{"junit_invocation_report", "verification_signal", "junit_current_report_bytes"},
		{"junit_invocation_report", "report_binding", "future_report_bytes"},
		{"JUNIT_INVOCATION_REPORT", "report_binding", "junit_current_report_bytes"},
	} {
		t.Run(tc.source+"/"+tc.category+"/"+tc.reason, func(t *testing.T) {
			first := VerificationDiagnostic{Source: tc.source, Category: tc.category, ReasonCode: tc.reason, Runner: "java", Command: "mvn test", Detail: "first detail"}
			second := first
			second.Detail = "different detail"
			pack := NormalizeWriteContextPack(WriteContextPackFromChangeReport(&ChangeReport{Passed: true, VerificationDiagnostics: []VerificationDiagnostic{first, second}}))
			if len(pack.Items) != 1 {
				t.Fatalf("unrelated diagnostic identity changed: %+v", pack.Items)
			}
			want := writeContextStableID("verification_diagnostic", first.Source, first.Category, first.Severity, first.ReasonCode, first.Runner, first.Framework, first.WorkingDir, first.Command, first.Outcome)
			if pack.Items[0].ID != want {
				t.Errorf("unrelated diagnostic ID changed: got %s want %s", pack.Items[0].ID, want)
			}
		})
	}
}

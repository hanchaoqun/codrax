package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestExportAuditRequiresExactQuestionAndUniqueFlags(t *testing.T) {
	in, _ := prepareExportAudit(t)
	body, err := os.ReadFile(in.Process)
	if err != nil {
		t.Fatal(err)
	}
	var original auditProcess
	if err := json.Unmarshal(body, &original); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"different_question", "duplicate_request", "duplicate_repo", "duplicate_report", "missing_value"} {
		t.Run(name, func(t *testing.T) {
			process := original
			process.Args = append([]string(nil), original.Args...)
			switch name {
			case "different_question":
				process.Args[4] = "other nonempty natural question"
			case "duplicate_request":
				process.Args = append(process.Args, "--request="+original.Args[4])
			case "duplicate_repo":
				process.Args = append(process.Args, "--repo", original.Args[2])
			case "duplicate_report":
				process.Args = append(process.Args, "--report-md="+in.Report)
			case "missing_value":
				process.Args = append(process.Args[:3], "--request")
			}
			data, err := json.Marshal(process)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(in.Process, data, 0600); err != nil {
				t.Fatal(err)
			}
			a, err := inspectExport(context.Background(), in)
			if err != nil || a.Checks["formal_process"].Status != "not_verified" {
				t.Fatalf("formal receipt mismatch was signed: %v %+v", err, a)
			}
		})
	}
}

func TestExportAuditUsesRenderedLinksNotMarkdownLookingText(t *testing.T) {
	const path = "/tmp/capture-review/report.md"
	const link = "[Execution](</tmp/capture-review/report.md>)"
	for _, source := range []string{link, "prefix " + link + " suffix", "[Execution][report]\n\n[report]: /tmp/capture-review/report.md"} {
		if !hasAuditMarkdownLink([]byte(source), path) {
			t.Fatalf("real link was not found: %q", source)
		}
	}
	for _, source := range []string{
		"`" + link + "`",
		"<!-- " + link + " -->",
		"prefix <!-- " + link + " --> suffix",
		"\\[Execution](</tmp/capture-review/report.md>)",
		"!" + link,
		"```markdown\n" + link + "\n```",
		"    " + link,
		"[Execution](</tmp/capture-review/report.md#other>)",
		"[Execution](</tmp/capture-review/report.md?other>)",
		`<a href="/tmp/capture-review/report.md">Execution</a>`,
	} {
		if hasAuditMarkdownLink([]byte(source), path) {
			t.Fatalf("non-delivery text was signed as a link: %q", source)
		}
	}
}

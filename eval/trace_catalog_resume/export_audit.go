package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
	"golang.org/x/net/html"
)

type exportAuditInputs struct {
	Out, Index, Receipt, Process, Answer string
	PriorReport, Report                  string
}

type exportAuditCheck struct {
	Status string   `json:"status"`
	Issues []string `json:"issues,omitempty"`
}

func (c *exportAuditCheck) issue(err error) {
	if err != nil {
		c.Status = "not_verified"
		c.Issues = append(c.Issues, err.Error())
	}
}

type exportPhysicalAudit struct {
	Status      string                      `json:"status"`
	Checks      map[string]exportAuditCheck `json:"checks"`
	InputSHA    map[string]string           `json:"input_sha256"`
	Receipt     tracecatalog.ExportReceipt  `json:"receipt"`
	OutputFiles []tracecatalog.ExportFile   `json:"verified_package_files,omitempty"`
	Limits      string                      `json:"limits"`
}

type auditProcess struct {
	Mode              string   `json:"mode"`
	Started           string   `json:"started_at"`
	Finished          string   `json:"finished_at"`
	ExitCode          *int     `json:"exit_code"`
	Signal            *int     `json:"signal"`
	Args              []string `json:"argv"`
	PriorReportSHA256 string   `json:"prior_report_sha256"`
}

// The audit never exports, queries, repairs, selects a newest file, or signs a
// model interpretation. A successful command means that a new audit was saved;
// its per-axis verdicts, not the command exit, describe the checked delivery.
func auditExport(ctx context.Context, in exportAuditInputs) error {
	a, err := inspectExport(ctx, in)
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(in.Out, "export-delivery-audit.json"), a)
}

func inspectExport(ctx context.Context, in exportAuditInputs) (exportPhysicalAudit, error) {
	a := exportPhysicalAudit{Checks: map[string]exportAuditCheck{}, InputSHA: map[string]string{},
		Limits: "Read-only operator smoke: serialized receipts/navigation create no live authority. Formal-process, file integrity, current execution and report identity are separate checks. Logs, original tool executions, full answer semantics and all-surface human audit remain required. No permanent archival, crash durability, cross-process CAS or persisted Previous receipt is asserted."}
	for label, path := range map[string]string{"out": in.Out, "index": in.Index, "receipt": in.Receipt, "process": in.Process, "answer": in.Answer} {
		if !filepath.IsAbs(path) {
			return a, fmt.Errorf("%s must be an exact absolute path", label)
		}
	}
	if (in.PriorReport == "") != (in.Report == "") || (in.Report != "" && (!filepath.IsAbs(in.PriorReport) || !filepath.IsAbs(in.Report))) {
		return a, errors.New("explicit report audit requires both exact absolute -audit-report-before and -audit-report paths")
	}
	read := func(path string) ([]byte, error) {
		body, err := readAuditFile(path)
		if err == nil {
			a.InputSHA[path] = hash(body)
		}
		return body, err
	}
	seedBytes, err := read(filepath.Join(in.Out, "preparation.json"))
	if err != nil {
		return a, err
	}
	var seed preparation
	if err := json.Unmarshal(seedBytes, &seed); err != nil {
		return a, err
	}
	processCheck := exportAuditCheck{Status: "verified"}
	var process auditProcess
	var processRepo, processRequest, processReport string
	processBytes, err := read(in.Process)
	processCheck.issue(err)
	if err == nil {
		processCheck.issue(json.Unmarshal(processBytes, &process))
		start, startErr := time.Parse(time.RFC3339Nano, process.Started)
		end, endErr := time.Parse(time.RFC3339Nano, process.Finished)
		if process.Mode != "live" || process.ExitCode == nil || *process.ExitCode != 0 || (process.Signal != nil && *process.Signal != 0) || startErr != nil || endErr != nil || end.Before(start) || end.After(time.Now()) {
			processCheck.issue(errors.New("no formal successful completed live CLI process receipt"))
		}
		processRepo, err = auditArg(process.Args, "--repo")
		processCheck.issue(err)
		processRequest, err = auditArg(process.Args, "--request")
		processCheck.issue(err)
		processReport, err = auditArg(process.Args, "--report-md")
		processCheck.issue(err)
		if processRepo != seed.Workspace || seed.Question == "" || processRequest != seed.Question {
			processCheck.issue(errors.New("process receipt does not match this workspace's exact prepared question"))
		}
	}
	a.Checks["formal_process"] = processCheck

	currentCheck := exportAuditCheck{Status: "verified"}
	_, err = read(in.Index)
	currentCheck.issue(err)
	current, currentErr := inspectCurrent(ctx, in.Out, in.Index)
	currentCheck.issue(currentErr)
	a.Checks["current_fixture_execution"] = currentCheck

	packageCheck := exportAuditCheck{Status: "verified"}
	var manifest tracecatalog.ExportManifest
	data, err := read(in.Receipt)
	packageCheck.issue(err)
	if err == nil {
		a.Receipt, err = decodeAuditExportReceipt(data)
		packageCheck.issue(err)
	}
	if packageCheck.Status == "verified" {
		var files []tracecatalog.ExportFile
		manifest, files, err = inspectExportPackage(a.Receipt, seed.ExportTarget)
		packageCheck.issue(err)
		a.OutputFiles = files
	}
	a.Checks["package_integrity"] = packageCheck

	currentMatch := exportAuditCheck{Status: "verified"}
	if packageCheck.Status != "verified" || currentCheck.Status != "verified" {
		currentMatch.issue(errors.New("package/current-index prerequisites are not verified"))
	} else if !sameCurrentNavigation(manifest.Catalog, current) {
		currentMatch.issue(errors.New("valid frozen package differs from final current navigation"))
	}
	a.Checks["current_package_match"] = currentMatch
	material := exportAuditCheck{Status: "verified"}
	if packageCheck.Status != "verified" || !a.Receipt.MaterialComplete || !manifest.MaterialComplete {
		material.issue(errors.New("reference material is not verified complete; this alone does not claim a product defect"))
	}
	a.Checks["material_complete"] = material

	linkCheck := exportAuditCheck{Status: "verified"}
	answer, answerErr := read(in.Answer)
	linkCheck.issue(answerErr)
	if processCheck.Status != "verified" || packageCheck.Status != "verified" {
		linkCheck.issue(errors.New("post-return links require completed live process and valid package"))
	} else {
		for _, path := range []string{a.Receipt.ManifestPath, a.Receipt.ReportPath} {
			if !hasAuditMarkdownLink(answer, path) {
				linkCheck.issue(fmt.Errorf("final Markdown has no exact delivery link to %s", path))
			}
		}
	}
	a.Checks["post_return_delivery_links"] = linkCheck
	reportCheck := exportAuditCheck{Status: "not_applicable"}
	if in.Report != "" {
		reportCheck.Status = "verified"
		prior, priorErr := read(in.PriorReport)
		report, reportErr := read(in.Report)
		reportCheck.issue(priorErr)
		reportCheck.issue(reportErr)
		reportCheck.issue(answerErr)
		if processCheck.Status != "verified" || processReport != in.Report || process.PriorReportSHA256 == "" || process.PriorReportSHA256 != hash(prior) {
			reportCheck.issue(errors.New("explicit report target/prior bytes do not match completed process receipt"))
		}
		if !bytes.Equal(report, answer) || len(answer) == 0 || bytes.Equal(prior, report) {
			reportCheck.issue(errors.New("explicit report was not updated to byte-identical final default Markdown"))
		}
	}
	a.Checks["explicit_report_bytes"] = reportCheck
	a.Status = "physical_delivery_smoke_pass_human_pending"
	for _, check := range a.Checks {
		if check.Status == "not_verified" {
			a.Status = "physical_delivery_smoke_incomplete_human_pending"
		}
	}
	return a, nil
}

func decodeAuditExportReceipt(data []byte) (tracecatalog.ExportReceipt, error) {
	var receipt tracecatalog.ExportReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return receipt, err
	}
	if receipt.ManifestPath == "" {
		var result types.ToolResult
		if err := json.Unmarshal(data, &result); err != nil || !result.Success || result.ToolName != "trace_catalog" {
			return receipt, errors.New("expected exact ExportReceipt or original successful trace_catalog ToolResult")
		}
		if err := json.Unmarshal([]byte(result.Summary), &receipt); err != nil {
			return receipt, err
		}
	}
	if !receipt.PhysicalSuccess || !filepath.IsAbs(receipt.ManifestPath) || !filepath.IsAbs(receipt.ReportPath) {
		return receipt, errors.New("receipt has no successful physical package with absolute paths")
	}
	return receipt, nil
}

func auditArg(args []string, name string) (string, error) {
	var value string
	found := false
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return "", errors.New("audit does not accept positional-argument delimiter in the formal CLI receipt")
		}
		if arg != name && !strings.HasPrefix(arg, name+"=") {
			continue
		}
		if found {
			return "", fmt.Errorf("formal CLI receipt has duplicate %s", name)
		}
		found = true
		if arg == name {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", fmt.Errorf("formal CLI receipt has no value for %s", name)
			}
			i++
			value = args[i]
		} else {
			value = strings.TrimPrefix(arg, name+"=")
		}
		if value == "" {
			return "", fmt.Errorf("formal CLI receipt has an empty %s", name)
		}
	}
	return value, nil
}

func hasAuditMarkdownLink(body []byte, path string) bool {
	// Reuse the real saved-preview Markdown parser/renderer. Text inside code,
	// raw HTML and comments must not become links merely by containing syntax.
	rendered, err := preview.RenderMarkdownHTML(body)
	if err != nil {
		return false
	}
	tokens := html.NewTokenizer(strings.NewReader(rendered))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken:
			token := tokens.Token()
			if token.Data != "a" {
				continue
			}
			for _, attr := range token.Attr {
				if attr.Key != "href" {
					continue
				}
				link, err := url.Parse(attr.Val)
				if err == nil && link.Scheme == "" && link.Host == "" && link.RawQuery == "" && link.Fragment == "" && link.Path == path {
					return true
				}
			}
		}
	}
}

func readAuditFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64<<20 {
		return nil, fmt.Errorf("audit input must be a bounded regular file: %s", path)
	}
	return os.ReadFile(path)
}

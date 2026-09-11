package types

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// VerificationDiagnosticReceiptIdentity distinguishes existing JUnit report-byte
// diagnostics for deduplication only. It neither parses nor verifies the detail,
// and grants no assertion authority. Other diagnostic identities stay unchanged.
func VerificationDiagnosticReceiptIdentity(diag VerificationDiagnostic) string {
	if strings.TrimSpace(diag.Source) != "junit_invocation_report" ||
		strings.TrimSpace(diag.Category) != "report_binding" ||
		strings.TrimSpace(diag.ReasonCode) != "junit_current_report_bytes" {
		return ""
	}
	detail := strings.TrimSpace(diag.Detail)
	if detail == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(detail)))
}

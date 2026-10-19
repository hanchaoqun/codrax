// Package reportartifact separates immutable verification report originals from
// the legacy mutable current view. References identify material, never authority.
package reportartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/immutablefile"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Receipt is process-local publication/read metadata. Report is a detached
// snapshot; changing it invalidates the receipt. The two digests have distinct
// domains: exact published bytes and json.Marshal(report), respectively.
// No permission, grant, workflow completion or test execution is created here.
type Receipt struct {
	ReportID        string
	PayloadSHA256   string
	CanonicalSHA256 string
	Report          *types.ChangeReport
	payload         []byte
	planID          string
	reportID        string
	canonicalSHA256 string
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func safeStem(planID string) string {
	var b strings.Builder
	for _, c := range []byte(planID) {
		if b.Len() == 120 {
			break
		}
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "report"
	}
	return b.String()
}

func exactID(planID, payloadSHA string) string {
	return safeStem(planID) + "." + payloadSHA + ".report.json"
}

// IsExactID recognizes only the generated, bounded leaf shape. Actual reads
// additionally validate complete bytes and exact producer PlanID.
func IsExactID(id string) bool {
	if !strings.HasSuffix(id, ".report.json") {
		return false
	}
	parts := strings.Split(strings.TrimSuffix(id, ".report.json"), ".")
	if len(parts) != 2 || len(parts[0]) == 0 || len(parts[0]) > 120 || safeStem(parts[0]) != parts[0] {
		return false
	}
	decoded, err := hex.DecodeString(parts[1])
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(parts[1]) == parts[1]
}

func validLegacyID(id string) bool {
	return id != ".report.json" && strings.HasSuffix(id, ".report.json") && !strings.ContainsAny(id, "/\\\x00") && !IsExactID(id)
}

// PublishSnapshot freezes complete producer data. The caller supplies PlanID
// and GeneratedAt; this function never generates a new execution/time identity.
// Normalization happens on a private clone, never on the producer's report.
func PublishSnapshot(root string, report *types.ChangeReport) (*Receipt, error) {
	if report == nil || strings.TrimSpace(report.PlanID) == "" {
		return nil, fmt.Errorf("publish report: missing report or plan identity")
	}
	canonical, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("freeze report: %w", err)
	}
	var snapshot types.ChangeReport
	if err := json.Unmarshal(canonical, &snapshot); err != nil {
		return nil, fmt.Errorf("freeze report: %w", err)
	}
	snapshot.EnsureVerificationStatus()
	wire, err := json.MarshalIndent(&snapshot, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("serialize report: %w", err)
	}
	id := exactID(snapshot.PlanID, digest(wire))
	if _, err := immutablefile.Publish(root, id, wire); err != nil {
		return nil, fmt.Errorf("publish report original: %w", err)
	}
	return receipt(id, wire, &snapshot)
}

func receipt(id string, wire []byte, report *types.ChangeReport) (*Receipt, error) {
	canonical, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	return &Receipt{ReportID: id, PayloadSHA256: digest(wire), CanonicalSHA256: digest(canonical), Report: report,
		payload: append([]byte(nil), wire...), planID: report.PlanID, reportID: id, canonicalSHA256: digest(canonical)}, nil
}

// ReceiptMatchesReport compares the exact normalized report generation, not
// just PlanID or outcome. It does not normalize, mutate, save or authorize it.
func ReceiptMatchesReport(r *Receipt, report *types.ChangeReport) bool {
	if r == nil || report == nil || r.Report == nil || r.planID == "" || report.PlanID != r.planID || r.Report.PlanID != r.planID || len(r.payload) == 0 || digest(r.payload) != r.PayloadSHA256 || r.ReportID != r.reportID || r.CanonicalSHA256 != r.canonicalSHA256 {
		return false
	}
	canonical, err := json.Marshal(report)
	if err != nil || digest(canonical) != r.CanonicalSHA256 {
		return false
	}
	current, err := json.Marshal(r.Report)
	return err == nil && digest(current) == r.CanonicalSHA256
}

// PublishCurrentView retains the old full-JSON format. It only projects a
// revalidated exact original and cannot overwrite any content-addressed leaf.
func PublishCurrentView(root, legacyID string, r *Receipt) error {
	if !validLegacyID(legacyID) || r == nil || !IsExactID(r.ReportID) || !ReceiptMatchesReport(r, r.Report) {
		return fmt.Errorf("publish current report: invalid leaf or stale receipt")
	}
	loaded, err := LoadExact(root, r.ReportID, r.planID)
	if err != nil {
		return err
	}
	if loaded.PayloadSHA256 != r.PayloadSHA256 || loaded.CanonicalSHA256 != r.CanonicalSHA256 {
		return fmt.Errorf("publish current report: original differs from receipt")
	}
	return immutablefile.ReplaceCurrent(root, legacyID, loaded.payload)
}

// LoadExact never falls back to the current report. Its generated name, full
// payload digest and expected PlanID must all agree before a snapshot is exposed.
func LoadExact(root, id, expectedPlanID string) (*Receipt, error) {
	if !IsExactID(id) || strings.TrimSpace(expectedPlanID) == "" {
		return nil, fmt.Errorf("load exact report: invalid reference or missing plan identity")
	}
	wire, err := immutablefile.Read(root, id)
	if err != nil {
		return nil, fmt.Errorf("load exact report: %w", err)
	}
	if exactID(expectedPlanID, digest(wire)) != id {
		return nil, fmt.Errorf("load exact report: content address or plan mismatch")
	}
	report, err := decode(wire, expectedPlanID)
	if err != nil {
		return nil, err
	}
	return receipt(id, wire, report)
}

// LoadLegacy is an explicit compatibility read, not a historical fallback. An
// expected canonical digest binds old attempts; without it the caller has only
// an unverified current view. Legacy bytes/status are not silently normalized.
func LoadLegacy(root, id, expectedPlanID, expectedCanonicalSHA256 string) (*Receipt, error) {
	if !validLegacyID(id) || strings.TrimSpace(expectedPlanID) == "" {
		return nil, fmt.Errorf("load legacy report: invalid reference or missing plan identity")
	}
	wire, err := immutablefile.Read(root, id)
	if err != nil {
		return nil, fmt.Errorf("load legacy report: %w", err)
	}
	report, err := decode(wire, expectedPlanID)
	if err != nil {
		return nil, err
	}
	r, err := receipt(id, wire, report)
	if err != nil {
		return nil, err
	}
	if expectedCanonicalSHA256 != "" && expectedCanonicalSHA256 != r.CanonicalSHA256 {
		return nil, fmt.Errorf("load legacy report: canonical report digest mismatch")
	}
	return r, nil
}

func decode(wire []byte, expectedPlanID string) (*types.ChangeReport, error) {
	var report types.ChangeReport
	if err := json.Unmarshal(wire, &report); err != nil {
		return nil, fmt.Errorf("decode report: %w", err)
	}
	if report.PlanID != expectedPlanID {
		return nil, fmt.Errorf("decode report: plan identity mismatch")
	}
	return &report, nil
}

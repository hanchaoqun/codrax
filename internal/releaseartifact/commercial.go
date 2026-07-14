package releaseartifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	traceStreamerProvenancePath = "third_party/trace_streamer/PROVENANCE.json"
	traceStreamerEvidenceDir    = "third_party/trace_streamer/evidence/"
	hmtraceRepository           = "https://gitcode.com/diting/hmtrace.git"
	hmtraceRef                  = "7fb4eabae01f310beccecf339403aca4e9660131"
	hmtraceLicenseFile          = "third_party/hmtrace/LICENSE.hmtrace.txt"
	hmtraceLicenseSHA256        = "c5accbbd8546e94c34aed24afe689a617627d18eed5a6c48277e48db57c23851"
	hmtraceLicenseDisclaimer    = "The hmtrace repository declares Apache-2.0 for hmtrace. This does not assert that Apache-2.0 is the sole or concluded license of the bundled trace_streamer binary or its statically linked dependencies."
)

type commercialProvenance struct {
	SchemaVersion             int                       `json:"schema_version"`
	Acquisition               provenanceAcquisition     `json:"acquisition"`
	LicenseConcluded          string                    `json:"license_concluded"`
	RedistributionStatus      string                    `json:"redistribution_status"`
	CommercialReleaseEvidence commercialReleaseEvidence `json:"commercial_release_evidence"`
	Assets                    []provenanceAsset         `json:"assets"`
}

type provenanceAcquisition struct {
	Repository              string `json:"repository"`
	Ref                     string `json:"ref"`
	RepositoryLicense       string `json:"repository_license"`
	RepositoryLicenseFile   string `json:"repository_license_file"`
	RepositoryLicenseSHA256 string `json:"repository_license_sha256"`
	Disclaimer              string `json:"disclaimer"`
}

type provenanceAsset struct {
	GOOS           string `json:"goos"`
	GOARCH         string `json:"goarch"`
	SourcePath     string `json:"source_path"`
	RepositoryPath string `json:"repository_path"`
	SHA256         string `json:"sha256"`
	SizeBytes      int64  `json:"size_bytes"`
	CopyMethod     string `json:"copy_method"`
	MinimumGlibc   string `json:"minimum_glibc"`
}

type commercialReleaseEvidence struct {
	PayloadScopedLegalApproval *legalApprovalEvidence `json:"payload_scoped_legal_approval"`
	SBOM                       *fileEvidence          `json:"sbom"`
	DependencyLicenseBundle    *fileEvidence          `json:"dependency_license_bundle"`
	ThirdPartyNotices          *fileEvidence          `json:"third_party_notices"`
	SourceBuildAttestation     *fileEvidence          `json:"source_build_attestation"`
}

type fileEvidence struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type legalApprovalEvidence struct {
	Path           string   `json:"path"`
	SHA256         string   `json:"sha256"`
	ApprovalID     string   `json:"approval_id"`
	ApprovedBy     string   `json:"approved_by"`
	ApprovedAt     string   `json:"approved_at"`
	PayloadSHA256s []string `json:"payload_sha256s"`
}

type legalApprovalDocument struct {
	SchemaVersion    int                        `json:"schema_version"`
	Decision         string                     `json:"decision"`
	LicenseConcluded string                     `json:"license_concluded"`
	ApprovalID       string                     `json:"approval_id"`
	ApprovedBy       string                     `json:"approved_by"`
	ApprovedAt       string                     `json:"approved_at"`
	PayloadSHA256s   []string                   `json:"payload_sha256s"`
	Evidence         commercialEvidenceBindings `json:"evidence"`
}

type commercialEvidenceBindings struct {
	SBOM                    fileEvidence `json:"sbom"`
	DependencyLicenseBundle fileEvidence `json:"dependency_license_bundle"`
	ThirdPartyNotices       fileEvidence `json:"third_party_notices"`
	SourceBuildAttestation  fileEvidence `json:"source_build_attestation"`
}

// VerifyCommercialTraceStreamerRelease is the pre-build authority for formal
// release targets that carry an embedded trace_streamer. Development builds
// may use an audited-but-legally-blocked payload; commercial releases may not.
func VerifyCommercialTraceStreamerRelease(repoRoot string) error {
	_, err := verifiedCommercialRepositoryPayloads(repoRoot)
	return err
}

func verifiedCommercialRepositoryPayloads(repoRoot string) ([]repositoryPayload, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return nil, fmt.Errorf("repository root is required")
	}
	provenanceBody, err := readRepositoryEvidence(repoRoot, traceStreamerProvenancePath, "trace_streamer provenance")
	if err != nil {
		return nil, err
	}
	var provenance commercialProvenance
	if err := decodeStrictJSON(provenanceBody, &provenance); err != nil {
		return nil, fmt.Errorf("decode %s: %w", traceStreamerProvenancePath, err)
	}
	if err := validateProvenanceAcquisition(repoRoot, provenance); err != nil {
		return nil, err
	}

	payloads := make([]repositoryPayload, 0, len(platformContracts))
	for _, contract := range platformContracts {
		payload, err := loadRepositoryPayload(repoRoot, contract)
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, payload)
	}
	if err := validateProvenanceAssets(provenance, payloads); err != nil {
		return nil, err
	}
	for _, payload := range payloads {
		if payload.contract.goos == "linux" {
			baseline, err := minimumRequiredGlibc(payload.payloadBody)
			if err != nil {
				return nil, fmt.Errorf("verify %s minimum glibc: %w", payload.name, err)
			}
			if baseline != payload.manifest.Platforms[0].MinimumGlibc {
				return nil, fmt.Errorf("%s payload minimum glibc=%q want manifest=%q", payload.name, baseline, payload.manifest.Platforms[0].MinimumGlibc)
			}
		}
		if payload.manifest.UpstreamRef != provenance.Acquisition.Ref {
			return nil, fmt.Errorf("%s manifest upstream_ref=%q want provenance ref=%q", payload.name, payload.manifest.UpstreamRef, provenance.Acquisition.Ref)
		}
		if payload.manifest.AcquisitionRepoLicense != provenance.Acquisition.RepositoryLicense {
			return nil, fmt.Errorf("%s manifest acquisition_repo_license=%q want=%q", payload.name, payload.manifest.AcquisitionRepoLicense, provenance.Acquisition.RepositoryLicense)
		}
		if payload.manifest.LicenseConcluded != provenance.LicenseConcluded {
			return nil, fmt.Errorf("%s manifest license_concluded=%q want provenance=%q", payload.name, payload.manifest.LicenseConcluded, provenance.LicenseConcluded)
		}
		if payload.manifest.RedistributionStatus != provenance.RedistributionStatus {
			return nil, fmt.Errorf("%s manifest redistribution_status=%q want provenance=%q", payload.name, payload.manifest.RedistributionStatus, provenance.RedistributionStatus)
		}
	}

	if provenance.RedistributionStatus != "approved" || strings.EqualFold(provenance.LicenseConcluded, "NOASSERTION") {
		return nil, fmt.Errorf(
			"trace_streamer commercial redistribution is blocked: license_concluded=%q redistribution_status=%q; payload-scoped legal approval, SBOM, dependency license bundle, third-party notices, and source/build attestation are required",
			provenance.LicenseConcluded,
			provenance.RedistributionStatus,
		)
	}
	if err := validateCommercialReleaseEvidence(repoRoot, provenance, payloads); err != nil {
		return nil, err
	}
	return payloads, nil
}

func validateProvenanceAcquisition(repoRoot string, provenance commercialProvenance) error {
	if provenance.SchemaVersion != 1 {
		return fmt.Errorf("trace_streamer provenance schema_version=%d want=1", provenance.SchemaVersion)
	}
	acquisition := provenance.Acquisition
	if acquisition.Repository != hmtraceRepository {
		return fmt.Errorf("trace_streamer acquisition repository=%q want=%q", acquisition.Repository, hmtraceRepository)
	}
	if acquisition.Ref != hmtraceRef {
		return fmt.Errorf("trace_streamer acquisition ref=%q want pinned %s", acquisition.Ref, hmtraceRef)
	}
	if acquisition.RepositoryLicense != "Apache-2.0" {
		return fmt.Errorf("trace_streamer acquisition repository_license=%q want Apache-2.0", acquisition.RepositoryLicense)
	}
	if acquisition.RepositoryLicenseFile != hmtraceLicenseFile || acquisition.RepositoryLicenseSHA256 != hmtraceLicenseSHA256 {
		return fmt.Errorf("trace_streamer acquisition license witness does not match the pinned hmtrace license")
	}
	if acquisition.Disclaimer != hmtraceLicenseDisclaimer {
		return fmt.Errorf("trace_streamer acquisition disclaimer must preserve the payload-license limitation verbatim")
	}
	license, err := readRepositoryEvidence(repoRoot, acquisition.RepositoryLicenseFile, "hmtrace repository license")
	if err != nil {
		return err
	}
	if got := sha256Hex(license); got != acquisition.RepositoryLicenseSHA256 {
		return fmt.Errorf("hmtrace repository license sha256=%s want=%s", got, acquisition.RepositoryLicenseSHA256)
	}
	concluded := strings.TrimSpace(provenance.LicenseConcluded)
	if concluded == "" {
		return fmt.Errorf("trace_streamer provenance license_concluded is required")
	}
	if concluded != provenance.LicenseConcluded {
		return fmt.Errorf("trace_streamer provenance license_concluded must be canonical without surrounding whitespace")
	}
	if provenance.RedistributionStatus != "blocked" && provenance.RedistributionStatus != "approved" {
		return fmt.Errorf("trace_streamer provenance redistribution_status must be blocked or approved")
	}
	return nil
}

func validateProvenanceAssets(provenance commercialProvenance, payloads []repositoryPayload) error {
	if len(provenance.Assets) != len(payloads) {
		return fmt.Errorf("trace_streamer provenance asset count=%d want=%d", len(provenance.Assets), len(payloads))
	}
	assets := make(map[string]provenanceAsset, len(provenance.Assets))
	for _, asset := range provenance.Assets {
		key := asset.GOOS + "/" + asset.GOARCH
		if _, duplicate := assets[key]; duplicate {
			return fmt.Errorf("trace_streamer provenance contains duplicate asset %s", key)
		}
		assets[key] = asset
	}
	for _, payload := range payloads {
		contract := payload.contract
		key := contract.goos + "/" + contract.goarch
		asset, ok := assets[key]
		if !ok {
			return fmt.Errorf("trace_streamer provenance lacks asset %s", key)
		}
		wantSource := "assets/trace_streamer/" + map[PayloadExpectation]string{
			PayloadLinux:   "linux-x86_64/trace_streamer",
			PayloadWindows: "windows-x86_64/trace_streamer.exe",
		}[contract.name]
		wantRepository := filepath.ToSlash(filepath.Join("internal", "hitraceconv", "embedded_trace_streamer", contract.directory, contract.binaryName))
		if asset.SourcePath != wantSource || asset.RepositoryPath != wantRepository || asset.CopyMethod != "verbatim_unmodified" {
			return fmt.Errorf("trace_streamer provenance asset %s has unapproved source/repository/copy identity", key)
		}
		if err := validateCanonicalSHA256(asset.SHA256); err != nil {
			return fmt.Errorf("trace_streamer provenance asset %s: %w", key, err)
		}
		if asset.SizeBytes != int64(len(payload.payloadBody)) || asset.SHA256 != sha256Hex(payload.payloadBody) {
			return fmt.Errorf("trace_streamer provenance asset %s does not match repository payload bytes", key)
		}
		platform := payload.manifest.Platforms[0]
		if asset.SizeBytes != platform.SizeBytes || asset.SHA256 != platform.SHA256 {
			return fmt.Errorf("trace_streamer provenance asset %s does not match platform manifest", key)
		}
		if asset.MinimumGlibc != platform.MinimumGlibc {
			return fmt.Errorf("trace_streamer provenance asset %s minimum_glibc=%q want manifest=%q", key, asset.MinimumGlibc, platform.MinimumGlibc)
		}
		wantSourceURL := "https://gitcode.com/diting/hmtrace/tree/" + hmtraceRef + "/" + strings.TrimSuffix(asset.SourcePath, "/"+contract.binaryName)
		if payload.manifest.SourceURL != wantSourceURL {
			return fmt.Errorf("trace_streamer manifest %s source_url=%q want=%q", key, payload.manifest.SourceURL, wantSourceURL)
		}
	}
	return nil
}

func validateCommercialReleaseEvidence(repoRoot string, provenance commercialProvenance, payloads []repositoryPayload) error {
	evidence := provenance.CommercialReleaseEvidence
	if evidence.PayloadScopedLegalApproval == nil {
		return fmt.Errorf("commercial trace_streamer release lacks payload_scoped_legal_approval")
	}
	for name, item := range map[string]*fileEvidence{
		"sbom":                      evidence.SBOM,
		"dependency_license_bundle": evidence.DependencyLicenseBundle,
		"third_party_notices":       evidence.ThirdPartyNotices,
		"source_build_attestation":  evidence.SourceBuildAttestation,
	} {
		if item == nil {
			return fmt.Errorf("commercial trace_streamer release lacks %s", name)
		}
	}
	legal := evidence.PayloadScopedLegalApproval
	if strings.TrimSpace(legal.ApprovalID) == "" || strings.TrimSpace(legal.ApprovedBy) == "" || strings.TrimSpace(legal.ApprovedAt) == "" {
		return fmt.Errorf("payload_scoped_legal_approval requires approval_id, approved_by, and approved_at")
	}
	if _, err := time.Parse(time.RFC3339, legal.ApprovedAt); err != nil {
		return fmt.Errorf("payload_scoped_legal_approval approved_at must be RFC3339: %w", err)
	}
	wantPayloads := make([]string, 0, len(payloads))
	for _, payload := range payloads {
		wantPayloads = append(wantPayloads, sha256Hex(payload.payloadBody))
	}
	sort.Strings(wantPayloads)
	gotPayloads := append([]string(nil), legal.PayloadSHA256s...)
	sort.Strings(gotPayloads)
	if strings.Join(gotPayloads, "\n") != strings.Join(wantPayloads, "\n") {
		return fmt.Errorf("payload_scoped_legal_approval hashes=%v want exact payload set=%v", gotPayloads, wantPayloads)
	}

	paths := map[string]string{}
	type physicalEvidence struct {
		name string
		info fs.FileInfo
	}
	physical := make([]physicalEvidence, 0, 5)
	legalFile := fileEvidence{Path: legal.Path, SHA256: legal.SHA256}
	var legalBody []byte
	for name, item := range map[string]*fileEvidence{
		"payload_scoped_legal_approval": &legalFile,
		"sbom":                          evidence.SBOM,
		"dependency_license_bundle":     evidence.DependencyLicenseBundle,
		"third_party_notices":           evidence.ThirdPartyNotices,
		"source_build_attestation":      evidence.SourceBuildAttestation,
	} {
		if prior, duplicate := paths[item.Path]; duplicate {
			return fmt.Errorf("commercial release evidence %s reuses path %q already owned by %s", name, item.Path, prior)
		}
		if !strings.HasPrefix(item.Path, traceStreamerEvidenceDir) {
			return fmt.Errorf("commercial release evidence %s path %q must stay under %s", name, item.Path, traceStreamerEvidenceDir)
		}
		paths[item.Path] = name
		body, info, err := verifyRepositoryEvidenceFile(repoRoot, name, *item)
		if err != nil {
			return err
		}
		for _, prior := range physical {
			if os.SameFile(prior.info, info) {
				return fmt.Errorf("commercial release evidence %s reuses the physical file owned by %s", name, prior.name)
			}
		}
		physical = append(physical, physicalEvidence{name: name, info: info})
		if name == "payload_scoped_legal_approval" {
			legalBody = body
		}
	}
	var document legalApprovalDocument
	if err := decodeStrictJSON(legalBody, &document); err != nil {
		return fmt.Errorf("decode payload-scoped legal approval: %w", err)
	}
	documentPayloads := append([]string(nil), document.PayloadSHA256s...)
	sort.Strings(documentPayloads)
	wantBindings := commercialEvidenceBindings{
		SBOM:                    *evidence.SBOM,
		DependencyLicenseBundle: *evidence.DependencyLicenseBundle,
		ThirdPartyNotices:       *evidence.ThirdPartyNotices,
		SourceBuildAttestation:  *evidence.SourceBuildAttestation,
	}
	if document.SchemaVersion != 1 || document.Decision != "approved_for_commercial_redistribution" ||
		document.LicenseConcluded != provenance.LicenseConcluded || document.ApprovalID != legal.ApprovalID ||
		document.ApprovedBy != legal.ApprovedBy || document.ApprovedAt != legal.ApprovedAt ||
		strings.Join(documentPayloads, "\n") != strings.Join(wantPayloads, "\n") || document.Evidence != wantBindings {
		return fmt.Errorf("payload-scoped legal approval document does not bind the approved status, concluded license, approval identity, exact payload hash set, and complete evidence path/hash set")
	}
	return nil
}

func verifyRepositoryEvidenceFile(repoRoot, name string, evidence fileEvidence) ([]byte, fs.FileInfo, error) {
	if err := validateCanonicalSHA256(evidence.SHA256); err != nil {
		return nil, nil, fmt.Errorf("commercial release evidence %s: %w", name, err)
	}
	resolved, err := resolveRepositoryEvidencePath(repoRoot, evidence.Path, "commercial release evidence "+name)
	if err != nil {
		return nil, nil, err
	}
	body, info, err := readRegularFileSnapshotInfo(resolved, "commercial release evidence "+name)
	if err != nil {
		return nil, nil, err
	}
	if got := sha256Hex(body); got != evidence.SHA256 {
		return nil, nil, fmt.Errorf("commercial release evidence %s sha256=%s want=%s", name, got, evidence.SHA256)
	}
	return body, info, nil
}

func readRepositoryEvidence(repoRoot, relative, label string) ([]byte, error) {
	resolved, err := resolveRepositoryEvidencePath(repoRoot, relative, label)
	if err != nil {
		return nil, err
	}
	return readRegularFileSnapshot(resolved, label)
}

func resolveRepositoryEvidencePath(repoRoot, relative, label string) (string, error) {
	if filepath.IsAbs(relative) || strings.TrimSpace(relative) == "" {
		return "", fmt.Errorf("%s path must be repository-relative", label)
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.ToSlash(clean) != relative {
		return "", fmt.Errorf("%s path must be a clean repository-relative path: %q", label, relative)
	}
	realRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root for %s: %w", label, err)
	}
	realRoot, err = filepath.Abs(realRoot)
	if err != nil {
		return "", fmt.Errorf("make repository root absolute for %s: %w", label, err)
	}
	target := filepath.Join(realRoot, clean)
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("resolve %s path: %w", label, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("make %s path absolute: %w", label, err)
	}
	rel, err := filepath.Rel(realRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s resolves outside the repository", label)
	}
	if resolved != target {
		return "", fmt.Errorf("%s path contains a symlink or junction", label)
	}
	return resolved, nil
}

func decodeStrictJSON(body []byte, target any) error {
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return err
	}
	if err := validateExactJSONFieldNames(body, target); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
